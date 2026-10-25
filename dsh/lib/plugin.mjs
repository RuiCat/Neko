/**
 * dsh-astrbox 桥接插件入口。
 *
 * 只做四件事（K2 冻结）：
 * ① 注册常量系统提示段 `astrbox-usage`（C1：动态状态绝不进入系统提示段）；
 * ② `agent/pre-step` 每轮至多追加一条 `[astrbox]` 状态快照 user 消息（C12）；
 * ③ `ctx.interval` 每 `tickEveryMs` 发 `POST /tick`（单飞，409 跳过）；
 * ④ `session/created` / `agent/turn-stopping` 发 `POST /session-event`（C2）。
 *
 * 零依赖、零构建：只用 node 内置模块。配置与监听器失败只记日志，不向宿主抛错。
 */
import { normalizeConfig } from './config.mjs'
import { createControlClient } from './controlClient.mjs'
import { createLogger } from './logger.mjs'
import { STATIC_USAGE_TEXT, composeSnapshot, createSnapshotMessage } from './prompt.mjs'
import { createSessionEventSender } from './sessionEvents.mjs'

export const name = 'astrbox-bridge'

/** 必须在 systemPrompt 服务就绪后注册提示段。 */
export const inject = ['systemPrompt']

/** 提示段固定元数据（K2）。 */
const SECTION = Object.freeze({ name: 'astrbox-usage', order: 2500, interpolate: false })

/** 连续失败达到该次数后本轮不注入动态快照。 */
const STATE_FAILURE_LIMIT = 3

/** 同一 ctx 只保留一个运行时：重复 apply / HMR 不叠加 interval。 */
const runtimes = new WeakMap()

/**
 * 挂载插件。
 * @param {object} ctx Cordis 上下文（已注入 systemPrompt）
 * @param {object} [rawConfig] 行配置；非法时抛出并指名键（C11）
 * @returns {{dispose: Function}} 运行时句柄
 */
export function apply(ctx, rawConfig = {}) {
  const config = normalizeConfig(rawConfig)
  runtimes.get(ctx)?.dispose()
  const logger = createLogger({ ctx, warningAfterMs: config.warningAfterMs })
  const client = createControlClient({
    socketPath: config.socketPath,
    requestTimeoutMs: config.requestTimeoutMs,
    tickTimeoutMs: config.tickTimeoutMs,
  })
  const runtime = createRuntime({ ctx, config, client, logger })
  runtimes.set(ctx, runtime)
  ctx.effect(
    () => () => {
      runtime.dispose()
      if (runtimes.get(ctx) === runtime) runtimes.delete(ctx)
    },
    'astrbox-bridge',
  )
  return runtime
}

/**
 * 建一次插件运行时：注册提示段与监听器，启动 Tick 定时器。
 * @param {{ctx: object, config: object, client: object, logger: object}} options 依赖
 * @returns {{dispose: Function}} 运行时句柄
 */
function createRuntime(options) {
  const { ctx, config, client, logger } = options
  const sender = createSessionEventSender({ client, config, logger, isActive: () => !disposed })
  const lastTurnByAgent = new WeakMap()
  let disposed = false
  let stopTicker
  let disposeSection
  let stateText = ''
  let stateFailures = 0
  let stateCheckedAt = 0
  let tickInFlight = false

  /** 拉一次 `/state` 并渲染快照；失败保留上一版，连续失败达到上限则本轮不注入。 */
  async function currentSnapshot() {
    if (disposed) return ''
    const at = Date.now()
    // 最多每 promptRefreshMs 拉一次；失败也占一个刷新窗口，避免失败时每轮打服务端。
    if (stateCheckedAt !== 0 && at - stateCheckedAt < config.promptRefreshMs) return stateText
    stateCheckedAt = at
    try {
      const state = await client.getState({ entity: config.entity, paths: config.paths })
      stateText = composeSnapshot({
        entity: config.entity,
        timestamp: new Date().toISOString(),
        paths: config.paths,
        values: state?.values ?? {},
        maxChars: config.promptMaxChars,
      })
      stateFailures = 0
      return stateText
    } catch (error) {
      stateFailures += 1
      if (stateFailures >= STATE_FAILURE_LIMIT) {
        logger.warnOnce(
          'state',
          `控制面 /state 连续失败 ${stateFailures} 次，本轮不注入状态快照：${errorMessage(error)}`,
        )
        return ''
      }
      return stateText
    }
  }

  /** `agent/pre-step`：每轮（step 1）至多一条快照消息；任何异常都不得打断宿主。 */
  async function onPreStep(payload, next) {
    const decision = await next()
    try {
      if (disposed || decision === undefined || decision === null || decision.kind === 'reject') return decision
      const { agent, step, turn } = payload ?? {}
      if (step !== 1) return decision
      if (agent !== undefined && lastTurnByAgent.get(agent) === turn) return decision
      const text = await currentSnapshot()
      if (text === '') return decision
      if (agent !== undefined) lastTurnByAgent.set(agent, turn)
      return { ...decision, messages: [...decision.messages, createSnapshotMessage(text)] }
    } catch (error) {
      logger.warnOnce('pre-step', `状态快照注入失败（已忽略）：${errorMessage(error)}`)
      return decision
    }
  }

  /** `session/created`：处理器内部已 try/catch，这里再兜一层同步异常。 */
  function onSessionCreated(session) {
    try {
      sender.sessionCreated(session)
    } catch (error) {
      logger.warnOnce('session-created', `session/created 处理失败（已忽略）：${errorMessage(error)}`)
    }
  }

  /** `agent/turn-stopping`：同上。 */
  function onTurnStopping(payload) {
    try {
      sender.turnStopping(payload)
    } catch (error) {
      logger.warnOnce('turn-stopping', `agent/turn-stopping 处理失败（已忽略）：${errorMessage(error)}`)
    }
  }

  /** 发一次 `/tick`；409 记为跳过，失败只记日志，不改间隔。 */
  async function runTick() {
    try {
      const result = await client.postTick({ scope: config.tickScope })
      if (result.busy) logger.debug('tick_busy：已有 tick 在飞，本次跳过')
    } catch (error) {
      logger.warnOnce('tick', `控制面 /tick 失败（只记日志，不改变间隔）：${errorMessage(error)}`)
    }
  }

  /** 启动单飞 Tick 定时器；tickEveryMs=0 不注册。 */
  function startTicker() {
    if (config.tickEveryMs <= 0) return
    stopTicker = ctx.interval(() => {
      if (disposed || tickInFlight) return
      tickInFlight = true
      void runTick().finally(() => {
        tickInFlight = false
      })
    }, config.tickEveryMs)
  }

  if (typeof ctx.systemPrompt?.section !== 'function') {
    throw new Error('astrbox-bridge: systemPrompt service is unavailable; add "systemPrompt" to inject')
  }
  disposeSection = ctx.systemPrompt.section({
    name: SECTION.name,
    order: SECTION.order,
    interpolate: SECTION.interpolate,
    text: () => STATIC_USAGE_TEXT,
  })
  ctx.on('agent/pre-step', onPreStep)
  ctx.on('session/created', onSessionCreated)
  ctx.on('agent/turn-stopping', onTurnStopping)
  startTicker()

  return {
    /** 卸载清理：停止定时器、注销提示段、中止在飞请求与事件发送。 */
    dispose() {
      if (disposed) return
      disposed = true
      try {
        stopTicker?.()
      } catch {
        // 定时器可能已随 fiber 释放：忽略。
      }
      try {
        disposeSection?.()
      } catch {
        // 提示段可能已随 fiber 释放：忽略。
      }
      sender.dispose()
      client.dispose()
    },

    /** @returns {boolean} 是否有 /tick 在飞（单飞诊断/测试用） */
    get tickInFlight() {
      return tickInFlight
    },
  }
}

/** @param {unknown} error @returns {string} 可读错误文本 */
function errorMessage(error) {
  return error instanceof Error ? error.message : String(error)
}
