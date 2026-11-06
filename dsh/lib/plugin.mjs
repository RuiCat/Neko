/**
 * dsh-astrbox 桥接插件入口。
 *
 * 只做六件事（K2/K4–K7 冻结）：
 * ① 注册常量系统提示段 `astrbox-usage`（C1：动态状态绝不进入系统提示段）；
 * ② `agent/pre-step` 每轮至多追加一条 `[astrbox]` 状态快照 user 消息（C12），按 agent 会话 cwd 解析生效实体（K6）；
 * ③ `ctx.interval` 每 `tickEveryMs` 发 `POST /tick`（单飞，409 跳过）；
 * ④ `session/created` / `agent/turn-stopping` 发 `POST /session-event`（C2；绑定命中的 cwd 带 entity）；
 * ⑤ `ctx.provide('astrboxBridge', host)` 提供 NPC 实体目录/人设服务（K2）；
 * ⑥ 事件黑板订阅（K4–K7）：tick 成功后主会话按版本+间隔 `followup` 订阅文本；
 *    pre-step 对绑定实体的子会话注入一次；`astrboxBridge.blackboards()` 与 `GET /astrbox/blackboard` 供面板读取；
 *    另注册 `astrbox_relay`（K14 跨会话转达）与 `astrbox_subscription`（K17 订阅间隔）工具。
 *
 * 零依赖、零构建：只用 node 内置模块。配置与监听器失败只记日志，不向宿主抛错。
 */
import { randomUUID } from 'node:crypto'
import { createHostBridge } from './astrboxService.mjs'
import { registerBlackboardRoute } from './blackboardRoute.mjs'
import { formatSubscriptionText } from './blackboardText.mjs'
import { createDialogFeed, dialogKind, turnDialogExcerpt } from './dialogFeed.mjs'
import { createRelayTool } from './relayTool.mjs'
import { createIntervalStore, isValidIntervalMinutes } from './subscriptionInterval.mjs'
import { createSubscriptionTool } from './subscriptionTool.mjs'
import { normalizeConfig } from './config.mjs'
import { createControlClient } from './controlClient.mjs'
import { normalizeBindings, resolveEntityForCwd } from './entityBindings.mjs'
import { createLogger } from './logger.mjs'
import { STATIC_USAGE_TEXT, composePackage, composeSnapshot, createSnapshotMessage } from './prompt.mjs'
import { createSessionEventSender } from './sessionEvents.mjs'

export const name = 'astrbox-bridge'

/**
 * 必须等 systemPrompt 与 timer 服务就绪：前者注册提示段，后者提供 ctx.interval。
 * `qqbotBots` 是 K1 可选上游，必须用 `ctx.get` 惰性读取，**不得**进本列表（C6）。
 */
export const inject = ['systemPrompt', 'timer']

/** 提示段固定元数据（K2）。 */
const SECTION = Object.freeze({ name: 'astrbox-usage', order: 2500, interpolate: false })

/** 某实体连续失败达到该次数后本轮不注入动态快照。 */
const STATE_FAILURE_LIMIT = 3

/** 桥接回落 agent 缓存的条数上限（K5：`agents` 注册表不可用时按会话 id 索引）。 */
const SEEN_AGENT_LIMIT = 64

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
 * 建一次插件运行时：注册提示段与监听器，启动 Tick 定时器，提供 `astrboxBridge` 服务。
 * @param {{ctx: object, config: object, client: object, logger: object}} options 依赖
 * @returns {{dispose: Function, tickInFlight: boolean}} 运行时句柄
 */
function createRuntime(options) {
  const { ctx, config, client, logger } = options
  let disposed = false
  let stopTicker
  let disposeSection
  let disposeProvide
  let tickInFlight = false
  /** 最近一次会话活动时间（pre-step / turn-stopping）；0 = 从未活跃。 */
  let lastActivityAt = 0
  /** 最近一次非空合法绑定（K6：服务缺失/HMR 空窗继续用它）。 */
  let lastBindings = []
  /** 每实体快照缓存：entity → { text, checkedAt, failures, inflight }（K6/C4）。 */
  const snapshots = new Map()
  const lastTurnByAgent = new WeakMap()
  /** 主会话推送状态：`entity\0sessionId` → { boardVersion, feedVersion, pushedAt }（K12）。 */
  const mainPushes = new Map()
  /** 子会话注入状态：sessionId → 最近注入过的 { boardVersion, feedVersion }（K13）。 */
  const lastInjected = new Map()
  /** 每实体对话摘要 Feed（K10，内存态）。 */
  const dialogFeed = createDialogFeed({
    maxEntries: config.dialogFeedSize,
    excerptChars: config.dialogExcerptChars,
  })
  /** 桥接见过的 agent 缓存：sessionId → agent（K5 回落）。 */
  const seenAgentCache = new Map()
  /** 每实体黑板缓存：entity → { board, checkedAt, failures, inflight }（K7）。 */
  const boardCache = new Map()
  let disposeBlackboardRoute
  let relayTool
  let disposeRelay
  let subscriptionTool
  let disposeSubscription
  /** K17：订阅间隔覆盖（内存 + 持久化，路径由 `subscriptionStatePath` 决定）。 */
  const intervalStore = createIntervalStore({ path: config.subscriptionStatePath, logger })

  /** 惰性读取 `qqbotBots.bindings()`（K1）：服务缺失、抛错或形状不对都视为无绑定。 */
  function currentBindings() {
    try {
      const service = typeof ctx.get === 'function' ? ctx.get('qqbotBots') : undefined
      const raw = typeof service?.bindings === 'function' ? service.bindings() : undefined
      const bindings = normalizeBindings(raw)
      if (bindings.length > 0) lastBindings = bindings
      return lastBindings
    } catch {
      return lastBindings
    }
  }

  /**
   * cwd 命中的绑定实体 id；未命中/绑定无 entity 返回 undefined（K6）。
   * @param {unknown} cwd 会话工作目录
   * @returns {string | undefined} 实体 id
   */
  function entityOfCwd(cwd) {
    try {
      const entity = resolveEntityForCwd(currentBindings(), cwd)?.entity
      return typeof entity === 'string' && entity !== '' ? entity : undefined
    } catch {
      return undefined
    }
  }

  /**
   * agent 会话生效实体：绑定命中 → 该实体；未命中 → 全局 `config.entity`（K6 回落）。
   * @param {object | undefined} agent pre-step 的 agent
   * @returns {string} 实体 id
   */
  function resolveEntity(agent) {
    try {
      return entityOfCwd(agent?.session?.header?.cwd) ?? config.entity
    } catch {
      return config.entity
    }
  }

  /**
   * 记录桥接见过的 agent（K5 回落：`agents` 注册表不可用时按会话 id 索引）。
   * @param {object | undefined} agent pre-step / turn-stopping 的 agent
   */
  function rememberAgent(agent) {
    try {
      const sessionId = agent?.session?.id ?? agent?.id
      if (typeof sessionId !== 'string' || sessionId === '') return
      seenAgentCache.delete(sessionId)
      seenAgentCache.set(sessionId, agent)
      if (seenAgentCache.size > SEEN_AGENT_LIMIT) {
        const oldest = seenAgentCache.keys().next()
        if (!oldest.done) seenAgentCache.delete(oldest.value)
      }
    } catch {
      // agent 缓存只服务回落路径，失败忽略。
    }
  }

  /**
   * 按会话 id 取活跃 agent：优先 DSH `agents` 注册表，回落桥接见过的 agent 缓存（K5）。
   * @param {string} sessionId 会话 id
   * @returns {object | undefined} agent
   */
  function agentOf(sessionId) {
    if (typeof sessionId !== 'string' || sessionId === '') return undefined
    try {
      const registry = typeof ctx.get === 'function' ? ctx.get('agents') : undefined
      const agent = typeof registry?.get === 'function' ? registry.get(sessionId) : undefined
      if (agent !== undefined && agent !== null) return agent
    } catch {
      // agents 注册表未安装/未就绪/已卸载：回落到见过的 agent 缓存。
    }
    return seenAgentCache.get(sessionId)
  }

  /**
   * 取（或建）某实体的黑板缓存条目（K7）。
   * @param {string} entity 实体 id
   * @returns {{board: object|null, checkedAt: number, failures: number, inflight: Promise<void> | undefined}} 缓存条目
   */
  function boardEntry(entity) {
    let entry = boardCache.get(entity)
    if (entry === undefined) {
      entry = { board: null, checkedAt: 0, failures: 0, inflight: undefined }
      boardCache.set(entity, entry)
    }
    return entry
  }

  /**
   * 校验并规范化 `/blackboard` 响应里的 board（K1 形状；非法/空返回 null）。
   * @param {unknown} raw 响应体里的 board
   * @returns {object | null} 黑板
   */
  function normalizeBoard(raw) {
    if (typeof raw !== 'object' || raw === null || Array.isArray(raw)) return null
    if (typeof raw.entity !== 'string' || raw.entity.trim() === '') return null
    return {
      ...raw,
      entity: raw.entity,
      version: Number.isInteger(raw.version) ? raw.version : 0,
      entries: Array.isArray(raw.entries) ? raw.entries : [],
    }
  }

  /**
   * 拉一次某实体的黑板；失败保留上一版并限频告警（K7）。
   * @param {string} entity 实体 id
   * @param {object} entry 缓存条目
   */
  async function loadBoard(entity, entry) {
    try {
      const response = await client.getBlackboard({ entity })
      if (disposed) return
      entry.board = normalizeBoard(response?.board)
      entry.failures = 0
    } catch (error) {
      entry.failures += 1
      logger.warnOnce(
        `board:${entity}`,
        `控制面 /blackboard 拉取失败 ${entry.failures} 次（实体 ${entity}，只记日志）：${errorMessage(error)}`,
      )
    }
  }

  /**
   * 取某实体当前黑板：窗口内复用缓存；并发复用同一在飞请求；`force` 忽略窗口（主会话推送用）。
   * @param {string} entity 实体 id
   * @param {boolean} [force] 是否强制刷新
   * @returns {Promise<object | null>} 黑板（无黑板/失败时为 null 或上一版）
   */
  async function currentBoard(entity, force = false) {
    if (disposed) return null
    const entry = boardEntry(entity)
    if (force) {
      await loadBoard(entity, entry)
      return entry.board
    }
    const at = Date.now()
    if (entry.checkedAt !== 0 && at - entry.checkedAt < config.promptRefreshMs) return entry.board
    entry.checkedAt = at
    if (entry.inflight === undefined) {
      entry.inflight = Promise.resolve()
        .then(() => loadBoard(entity, entry))
        .finally(() => {
          entry.inflight = undefined
        })
    }
    await entry.inflight
    return entry.board
  }

  /**
   * 服务/面板可见的实体集合：绑定实体 + 全局 `config.entity`，去重升序（K7）。
   * @returns {string[]} 实体 id
   */
  function boardEntityIds() {
    const ids = new Set()
    try {
      if (typeof config.entity === 'string' && config.entity !== '') ids.add(config.entity)
      for (const binding of currentBindings()) {
        if (typeof binding.entity === 'string' && binding.entity !== '') ids.add(binding.entity)
      }
    } catch {
      // 绑定服务异常：只保留全局实体。
    }
    return [...ids].sort((left, right) => (left < right ? -1 : left > right ? 1 : 0))
  }

  /** `astrboxBridge.blackboards()`（K7）：同步返回缓存快照，后台按刷新窗口更新。 */
  function blackboardList() {
    try {
      return boardEntityIds().map((entity) => {
        void currentBoard(entity).catch(() => {})
        return { entity, board: boardEntry(entity).board }
      })
    } catch {
      return []
    }
  }

  /**
   * 主会话推送键：同一实体可能有多个 bot 主会话（K5 状态按实体+主会话隔离）。
   * @param {string} entity 实体 id
   * @param {string} sessionId 主会话 id
   * @returns {string} 键
   */
  function mainPushKey(entity, sessionId) {
    return `${entity}\u0000${sessionId}`
  }

  /**
   * 主会话推送目标：带 `entity` + `mainSessionId` 的绑定（去重、保持绑定顺序，K5）。
   * @returns {Array<{entity: string, sessionId: string, key: string}>} 目标
   */
  function mainPushTargets() {
    const targets = []
    const seen = new Set()
    try {
      for (const binding of currentBindings()) {
        const entity = binding.entity
        const sessionId = binding.mainSessionId
        if (typeof entity !== 'string' || entity === '') continue
        if (typeof sessionId !== 'string' || sessionId === '') continue
        const key = mainPushKey(entity, sessionId)
        if (seen.has(key)) continue
        seen.add(key)
        targets.push({ entity, sessionId, key })
      }
    } catch {
      // 绑定服务异常：无目标（全局 config.entity 没有主会话 id，按 K5 跳过）。
    }
    return targets
  }

  /**
   * 造一条订阅 user 消息（形状与宿主 `createUserMessage` 一致；本插件零依赖）。
   * @param {string} text 订阅文本
   * @returns {object} user 消息
   */
  function createSubscriptionMessage(text) {
    return {
      id: randomUUID(),
      role: 'user',
      content: [{ type: 'text', text }],
      source: { kind: 'user' },
    }
  }

  /**
   * K17：某主会话的生效推送间隔（毫秒）——覆盖分钟数优先，否则全局 subscriptionMainIntervalMs。
   * @param {string} sessionId 主会话 id
   * @returns {number} 间隔毫秒
   */
  function effectiveInterval(sessionId) {
    const override = intervalStore.get(sessionId)
    return isValidIntervalMinutes(override) ? override * 60000 : config.subscriptionMainIntervalMs
  }

  /**
   * 黑板占位：控制面没有黑板但对话 feed 有变化时，仍按 K5 头部渲染订阅文本。
   * @param {string} entity 实体 id
   * @returns {object} 形状与 K1 黑板一致的占位对象
   */
  function emptyBoard(entity) {
    return { entity, version: 0, updated_at: '', tick_id: '', entries: [] }
  }

  /**
   * 主会话主动读取（K12 扩展 K5）：tick 成功后**仅当黑板版本前进**才推送给主会话
   * （用户 20:30 补充：黑板没有变化则不推送到主体）。对话 feed 不单独触发推送，
   * 只在推送时附带 `feedVersion > 上次推送 feedVersion` 的新增条目。
   * 成功才推进 board 版本、`lastMainFeedVer=feedVersion` 与时间；失败/agent 不在线只 debug。
   * @returns {Promise<void>} 完成信号
   */
  async function pushMainSubscriptions() {
    if (disposed || !config.subscriptionEnabled) return
    for (const target of mainPushTargets()) {
      if (disposed) return
      const pushed = mainPushes.get(target.key)
      if (pushed !== undefined && Date.now() - pushed.pushedAt < effectiveInterval(target.sessionId)) continue
      try {
        const agent = agentOf(target.sessionId)
        if (agent === undefined || typeof agent.followup !== 'function') {
          logger.debug(`主会话订阅跳过（agent 不在线）：${target.entity}/${target.sessionId}`)
          continue
        }
        const board = await currentBoard(target.entity, true)
        const boardVersion = board === null ? 0 : Number.isInteger(board.version) ? board.version : 0
        // K12 收紧：只有黑板变化才推送主体；对话 feed 仅作附带内容。
        if (board === null || boardVersion <= (pushed?.boardVersion ?? 0)) continue
        const feedVersion = dialogFeed.version(target.entity)
        const dialogs = dialogFeed.dialogsSince(target.entity, pushed?.feedVersion ?? 0)
        const text = formatSubscriptionText(board, config.subscriptionMaxChars, dialogs)
        if (text === '') continue
        agent.followup(createSubscriptionMessage(text))
        mainPushes.set(target.key, { boardVersion, feedVersion, pushedAt: Date.now() })
      } catch (error) {
        logger.debug(`主会话订阅推送失败（下个 tick 重试）：${errorMessage(error)}`)
      }
    }
  }

  /**
   * 该实体绑定的主会话 id 集合（K6：主会话走 K5，避免双份）。
   * @param {string} entity 实体 id
   * @returns {Set<string>} 主会话 id
   */
  function mainSessionIdsOf(entity) {
    const ids = new Set()
    try {
      for (const binding of currentBindings()) {
        if (binding.entity !== entity) continue
        if (typeof binding.mainSessionId === 'string' && binding.mainSessionId !== '') ids.add(binding.mainSessionId)
      }
    } catch {
      // 绑定服务异常：主会话未知 → 照常注入。
    }
    return ids
  }

  /**
   * 子会话订阅文本（K6）：cwd 命中绑定的非主会话，黑板版本前进时返回一次；否则 `''`。
   * @param {object | undefined} agent pre-step 的 agent
   * @param {string | undefined} boundEntity cwd 命中的绑定实体（未命中返回 undefined）
   * @returns {Promise<string>} 订阅文本（不注入时 `''`）
   */
  async function subscriptionTextFor(agent, boundEntity) {
    try {
      if (!config.subscriptionEnabled || boundEntity === undefined) return ''
      const sessionId = agent?.session?.id
      if (typeof sessionId !== 'string' || sessionId === '') return ''
      if (mainSessionIdsOf(boundEntity).has(sessionId)) return ''
      const board = await currentBoard(boundEntity)
      const boardVersion = board === null ? 0 : Number.isInteger(board.version) ? board.version : 0
      const feedVersion = dialogFeed.version(boundEntity)
      const last = lastInjected.get(sessionId)
      const boardChanged = board !== null && boardVersion > (last?.boardVersion ?? 0)
      const feedChanged = feedVersion > (last?.feedVersion ?? 0)
      if (!boardChanged && !feedChanged) return ''
      const dialogs = dialogFeed.dialogs(boundEntity, { excludeSessionId: sessionId })
      const text = formatSubscriptionText(board ?? emptyBoard(boundEntity), config.subscriptionMaxChars, dialogs)
      if (text === '') return ''
      lastInjected.set(sessionId, { boardVersion, feedVersion })
      return text
    } catch (error) {
      logger.warnOnce('subscription', `订阅注入失败（已忽略，不影响本轮）：${errorMessage(error)}`)
      return ''
    }
  }

  /**
   * 取（或建）某实体的快照缓存条目：TTL、失败计数、在飞请求都按实体隔离（C4）。
   * @param {string} entity 实体 id
   * @returns {{text: string, checkedAt: number, failures: number, inflight: Promise<void> | undefined}} 缓存条目
   */
  function snapshotEntry(entity) {
    let entry = snapshots.get(entity)
    if (entry === undefined) {
      entry = { text: '', checkedAt: 0, failures: 0, inflight: undefined }
      snapshots.set(entity, entry)
    }
    return entry
  }

  /** 拉一次某实体的补偿包；失败或无包时回退 `/state`（K2 旧引擎兼容）。 */
  async function loadSnapshot(entity, entry) {
    try {
      const context = await client.getTurnContext({ entity })
      if (disposed) return
      if (context?.ok === true && typeof context.compensation === 'object' && context.compensation !== null) {
        entry.text = composePackage({
          entity,
          timestamp: new Date().toISOString(),
          compensation: context.compensation,
          active: Array.isArray(context.gate?.active) ? context.gate.active : [],
          paths: config.paths,
          maxChars: config.promptMaxChars,
        })
        entry.failures = 0
        return
      }
    } catch {
      // 旧引擎没有 /turn-context：回退到 /state，不额外告警。
    }
    try {
      const state = await client.getState({ entity, paths: config.paths })
      if (disposed) return
      entry.text = composeSnapshot({
        entity,
        timestamp: new Date().toISOString(),
        paths: config.paths,
        values: state?.values ?? {},
        maxChars: config.promptMaxChars,
      })
      entry.failures = 0
    } catch (error) {
      entry.failures += 1
      if (entry.failures >= STATE_FAILURE_LIMIT) {
        entry.text = ''
        logger.warnOnce(
          `state:${entity}`,
          `控制面 /state 连续失败 ${entry.failures} 次（实体 ${entity}），本轮不注入状态快照：${errorMessage(error)}`,
        )
      }
    }
  }

  /**
   * 取某实体当前快照文本：窗口内复用缓存；同实体并发复用同一在飞请求。
   * @param {string} entity 实体 id
   * @returns {Promise<string>} 快照文本（失败/抑制时为 ''）
   */
  async function currentSnapshot(entity) {
    if (disposed) return ''
    const entry = snapshotEntry(entity)
    const at = Date.now()
    // 最多每 promptRefreshMs 拉一次；失败也占一个刷新窗口，避免失败时每轮打服务端。
    if (entry.checkedAt !== 0 && at - entry.checkedAt < config.promptRefreshMs) return entry.text
    entry.checkedAt = at
    if (entry.inflight === undefined) {
      entry.inflight = Promise.resolve()
        .then(() => loadSnapshot(entity, entry))
        .finally(() => {
          entry.inflight = undefined
        })
    }
    await entry.inflight
    return entry.text
  }

  const sender = createSessionEventSender({
    client,
    config,
    logger,
    isActive: () => !disposed,
    // K6：绑定命中的 cwd 跳过 workspacePrefix 门控并带 entity；未命中走旧行为。
    entityOfCwd,
  })
  const host = createHostBridge({ client, config, logger, getBindings: currentBindings })
  // K7：黑板只读投影与实体目录/人设挂在同一个 `astrboxBridge` 服务对象上。
  host.blackboards = () => blackboardList()

  /**
   * K7：可选挂载 `GET /astrbox/blackboard`（`webServer` 缺失时整段跳过，插件照常工作）。
   * 路由随 `webServer` 服务生命周期自动注销；运行时 dispose 再兜一层。
   */
  function installBlackboardRoute() {
    if (typeof ctx.inject !== 'function') return
    try {
      ctx.inject(['webServer'], (scoped) => {
        if (disposed) return
        const webServer = scoped?.webServer ?? (typeof scoped?.get === 'function' ? scoped.get('webServer') : undefined)
        if (webServer === undefined || typeof webServer.register !== 'function') return
        const disposeRoutes = registerBlackboardRoute(webServer, { getBoards: blackboardList, logger })
        disposeBlackboardRoute = disposeRoutes
        try {
          if (typeof scoped?.effect === 'function') scoped.effect(() => disposeRoutes, 'astrbox-bridge.blackboard-route')
        } catch (error) {
          logger.warnOnce('blackboard-route', `黑板路由生命周期注册失败（只记日志）：${errorMessage(error)}`)
        }
      })
    } catch (error) {
      logger.warnOnce('blackboard-route', `黑板路由注入失败（面板黑板不可用）：${errorMessage(error)}`)
    }
  }

  /**
   * K10：会话标题作为对话行标记（qqbot 已把 `私聊(…)`/`群聊(…)` 写进 sessionTitle，K14）。
   * 服务缺失/会话未 live/抛错都返回 `''`（只影响标记，不影响记录）。
   * @param {object|undefined} session Session
   * @returns {string} 标题（可能为空串）
   */
  function sessionTitleOf(session) {
    try {
      const service = typeof ctx.get === 'function' ? ctx.get('sessionTitle') : undefined
      const snapshot = typeof service?.get === 'function' ? service.get(session) : undefined
      return typeof snapshot?.title === 'string' ? snapshot.title.trim() : ''
    } catch {
      return ''
    }
  }

  /**
   * K10：记录一轮的对话摘要到实体 feed（`agent/turn-stopping` 时调用；异常只记日志）。
   * 只收 cwd 命中实体绑定、且有真实用户文本的轮次（订阅/relay 触发的 NPC 回复不回灌）；
   * label = 主会话 `主体`，QQ 会话用 sessionTitle（如 `私聊(85D47757)`），取不到回退 `QQ`。
   * @param {{agent?: object, turn?: number}} payload `agent/turn-stopping` 载荷
   */
  function rememberDialog(payload) {
    try {
      const session = payload?.agent?.session
      const sessionId = session?.id
      const turn = payload?.turn
      if (typeof sessionId !== 'string' || sessionId === '' || !Number.isInteger(turn)) return
      const entity = entityOfCwd(session?.header?.cwd)
      if (entity === undefined) return
      const excerpt = turnDialogExcerpt(session, turn, config.dialogExcerptChars)
      if (excerpt === undefined) return
      const kind = dialogKind(sessionId, mainSessionIdsOf(entity))
      const label = kind === '主体' ? '主体' : sessionTitleOf(session) || 'QQ'
      dialogFeed.record(entity, {
        sessionId,
        kind,
        label,
        userText: excerpt.userText,
        assistantText: excerpt.assistantText,
        at: Date.now(),
        seq: excerpt.seq,
      })
    } catch (error) {
      logger.warnOnce('dialog', `对话摘要记录失败（已忽略）：${errorMessage(error)}`)
    }
  }

  /**
   * K14/K17：把 `astrbox_relay` 与 `astrbox_subscription` 注册到 tools 服务。
   * 服务缺失只 warn 一次，插件照常工作；两个工具各自注册失败互不影响。
   */
  function installTools() {
    let tools
    try {
      tools = typeof ctx.get === 'function' ? ctx.get('tools') : undefined
    } catch {
      tools = undefined
    }
    if (tools === undefined || tools === null || typeof tools.register !== 'function') {
      logger.warnOnce('tools', 'tools 服务不可用，astrbox_relay/astrbox_subscription 未注册（插件继续工作）')
      return
    }
    try {
      relayTool = createRelayTool({
        getBindings: currentBindings,
        agentOf,
        defaultEntity: config.entity,
        logger,
        qqbotService: () => {
          try {
            return typeof ctx.get === 'function' ? ctx.get('qqbotBots') : undefined
          } catch {
            return undefined
          }
        },
      })
      disposeRelay = tools.register(relayTool.definition)
    } catch (error) {
      logger.warnOnce('relay-tool', `astrbox_relay 注册失败（插件继续工作）：${errorMessage(error)}`)
    }
    try {
      subscriptionTool = createSubscriptionTool({
        store: intervalStore,
        defaultIntervalMs: config.subscriptionMainIntervalMs,
        getBindings: currentBindings,
        logger,
      })
      disposeSubscription = tools.register(subscriptionTool.definition)
    } catch (error) {
      logger.warnOnce('subscription-tool', `astrbox_subscription 注册失败（插件继续工作）：${errorMessage(error)}`)
    }
  }

  /** 记录一次会话活动，供世界时钟判断「活跃窗口」。 */
  function markActivity() {
    lastActivityAt = Date.now()
  }

  /**
   * 本 tick 要推进的世界时钟分钟数（K12）：活跃窗口内用 clockStepMinutes，
   * 空闲时用 clockIdleStepMinutes（默认 0 = 暂停）。两者都为 0 即关闭时钟。
   * @returns {number} 非负整数；0 表示不推进
   */
  function clockAdvanceMinutes() {
    const activeStep = Number.isInteger(config.clockStepMinutes) ? config.clockStepMinutes : 0
    const idleStep = Number.isInteger(config.clockIdleStepMinutes) ? config.clockIdleStepMinutes : 0
    if (activeStep <= 0 && idleStep <= 0) return 0
    const windowMs = Number.isInteger(config.clockActiveWindowMs) ? config.clockActiveWindowMs : 0
    const active = lastActivityAt !== 0 && Date.now() - lastActivityAt <= windowMs
    const step = active ? activeStep : idleStep
    return step > 0 ? step : 0
  }

  /** `agent/pre-step`：每轮（step 1）至多一条快照 + 一条订阅（快照在前）；异常都不得打断宿主。 */
  async function onPreStep(payload, next) {
    markActivity()
    const decision = await next()
    try {
      if (disposed || decision === undefined || decision === null || decision.kind === 'reject') return decision
      const { agent, step, turn } = payload ?? {}
      if (step !== 1) return decision
      rememberAgent(agent)
      if (agent !== undefined && lastTurnByAgent.get(agent) === turn) return decision
      const boundEntity = entityOfCwd(agent?.session?.header?.cwd)
      const snapshotText = await currentSnapshot(resolveEntity(agent))
      const subscriptionText = await subscriptionTextFor(agent, boundEntity)
      if (snapshotText === '' && subscriptionText === '') return decision
      if (agent !== undefined) lastTurnByAgent.set(agent, turn)
      const messages = [...decision.messages]
      if (snapshotText !== '') messages.push(createSnapshotMessage(snapshotText))
      if (subscriptionText !== '') messages.push(createSubscriptionMessage(subscriptionText))
      return { ...decision, messages }
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
      markActivity()
      rememberAgent(payload?.agent)
      rememberDialog(payload)
      sender.turnStopping(payload)
    } catch (error) {
      logger.warnOnce('turn-stopping', `agent/turn-stopping 处理失败（已忽略）：${errorMessage(error)}`)
    }
  }

  /** 发一次 `/tick`；409 记为跳过，失败只记日志，不改间隔；成功后按 K5 推送主会话订阅。 */
  async function runTick() {
    try {
      const result = await client.postTick({ scope: config.tickScope, advanceMinutes: clockAdvanceMinutes() })
      if (result.busy) {
        logger.debug('tick_busy：已有 tick 在飞，本次跳过')
        return
      }
      await pushMainSubscriptions()
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
  // K2：向 qqbot 等消费方提供服务；注册失败只记日志，注入/回写不受影响。
  if (typeof ctx.provide === 'function') {
    try {
      disposeProvide = ctx.provide('astrboxBridge', host)
    } catch (error) {
      logger.warnOnce('provide', `astrboxBridge 服务注册失败（插件继续工作）：${errorMessage(error)}`)
    }
  }
  installBlackboardRoute()
  installTools()

  return {
    /** 卸载清理：停止定时器、注销提示段与服务、中止在飞请求与事件发送。 */
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
      try {
        disposeProvide?.()
      } catch {
        // 服务可能已随 fiber 释放：忽略。
      }
      try {
        disposeBlackboardRoute?.()
      } catch {
        // 路由可能已随 webServer fiber 释放：忽略。
      }
      try {
        disposeRelay?.()
        relayTool?.dispose()
      } catch {
        // 工具可能已随 tools fiber 释放：忽略。
      }
      try {
        disposeSubscription?.()
      } catch {
        // 工具可能已随 tools fiber 释放：忽略。
      }
      dialogFeed.clear()
      host.dispose()
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
