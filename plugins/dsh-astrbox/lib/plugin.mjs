/**
 * dsh-astrbox 桥接插件入口。
 *
 * 主链路（Observation v2 冻结，contract §4/§5）：
 * ① 注册常量系统提示段 `astrbox-usage`（C1：动态状态绝不进入系统提示段）；
 * ② `agent/pre-step`：每轮至多追加一条 `[astrbox-state]` 现状卡（拉 `/v2/observations/state` 后
 *    用 `renderState` 渲染；按 agent 会话 cwd 解析生效实体，K6）；
 * ③ `ctx.interval` 每 `tickEveryMs` 发 `POST /tick`（单飞，409 跳过）；
 * ④ `session/created` / `agent/turn-stopping` 发 `POST /session-event`（C2；绑定命中的 cwd 带 entity）；
 * ⑤ `ctx.provide('astrboxBridge', host)` 提供 NPC 实体目录/人设服务（K2），
 *    以及 K28 主会话提示词（`mainSessionPrompt(entityId)`）；
 * ⑥ tick 成功后主会话按**实体游标**拉 `/v2/observations` delta，`renderDelta` 渲染后推送
 *    （对话摘要随同一条消息附带）；`astrboxBridge.blackboards()` 与 `GET /astrbox/blackboard`
 *    由 `/v2/observations/state` 投影供数；
 *    另注册 `astrbox_relay`（K14 跨会话转达）、`astrbox_subscription`（K17 订阅间隔）、
 *    `astrbox_main_prompt`（K29 自行维护主提示词）与 `astrbox_board`（K30 会话黑板）工具。
 *
 * 零依赖、零构建：只用 node 内置模块。配置与监听器失败只记日志，不向宿主抛错。
 */
import { randomUUID } from 'node:crypto'
import { createHostBridge } from './astrboxService.mjs'
import { registerBlackboardRoute } from './blackboardRoute.mjs'
import { registerControlRoutes } from './controlRoutes.mjs'
import { boardFromObservationState, statePathsOf } from './observationBoard.mjs'
import { renderDelta, renderState, renderStateDelta, stateValuesOf } from './observationRender.mjs'
import { createDialogFeed, dialogKind, formatDialogLine, turnDialogExcerpt } from './dialogFeed.mjs'
import { createRelayTool } from './relayTool.mjs'
import { createIntervalStore, isValidIntervalMinutes } from './subscriptionInterval.mjs'
import { createSubscriptionTool } from './subscriptionTool.mjs'
import { createMainSessionPromptStore } from './mainSessionPromptStore.mjs'
import { createBoardSubscriptionStore } from './sessionBoardSubscription.mjs'
import { createSessionBoardTool, callerOf } from './sessionBoardTool.mjs'
import { planBoardInjection } from './sessionBoardText.mjs'
import { registerMainSessionPromptRoute } from './mainSessionPromptRoute.mjs'
import { registerSessionBoardRoute } from './sessionBoardRoute.mjs'
import { createMainPromptTool } from './mainPromptTool.mjs'
import { existsSync, readFileSync } from 'node:fs'
import { DEFAULT_CONFIG_PATH, normalizeConfig } from './config.mjs'
import { createControlClient } from './controlClient.mjs'
import { normalizeBindings, resolveEntityForCwd } from './entityBindings.mjs'
import { createLogger } from './logger.mjs'
import { STATIC_USAGE_TEXT, composeCompensation, compensationSignature, truncateText } from './prompt.mjs'
import { createSessionEventSender } from './sessionEvents.mjs'

export const name = 'astrbox-bridge'

/**
 * 必须等 systemPrompt 与 timer 服务就绪：前者注册提示段，后者提供 ctx.interval。
 * `qqbotBots` 是 K1 可选上游，必须用 `ctx.get` 惰性读取，**不得**进本列表（C6）。
 */
export const inject = ['systemPrompt', 'timer']

/** 提示段固定元数据（K2）。 */
const SECTION = Object.freeze({ name: 'astrbox-usage', order: 2500, interpolate: false })

/** 会话级缓存条目上限：超出后淘汰最早建立的会话，防止长跑内存只增不减。 */
const SESSION_CACHE_LIMIT = 512

/** 单会话「已推送记忆」上限。 */
const PUSHED_MEMORY_LIMIT = 256

/** 某实体现状连续失败达到该次数后本轮不注入现状卡。 */
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
/**
 * 行配置 = 文件配置（configPath 指向的 JSON）覆盖默认值，再被行内字段覆盖。
 * 这样 bundle patch 只负责插件加载与启动参数，运行时配置落在 .dsh-neko/Neko/。
 * @param {object} [rawConfig] Cordis 行配置
 * @returns {object} 合并后的原始配置
 */
function loadRowConfig(rawConfig = {}) {
  const inline = rawConfig ?? {}
  const explicit = typeof inline.configPath === 'string' && inline.configPath.trim() !== ''
  const path = explicit ? inline.configPath : DEFAULT_CONFIG_PATH
  if (!existsSync(path)) {
    if (explicit) {
      throw new Error(`astrbox-bridge: 指定的配置文件不存在: ${path}`)
    }
    return inline
  }
  let fileConfig
  try {
    fileConfig = JSON.parse(readFileSync(path, 'utf8'))
  } catch (err) {
    throw new Error(`astrbox-bridge: 配置文件不是合法 JSON: ${path}: ${err.message}`)
  }
  return { ...fileConfig, ...inline }
}

export function apply(ctx, rawConfig = {}) {
  const config = normalizeConfig(loadRowConfig(rawConfig))
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
  /** 每实体现状卡缓存：entity → { text, structured, values, checkedAt, failures, inflight }（Observation v2）。 */
  const observationStates = new Map()
  /** 每会话上次注入的现状值表：sessionId → Map(path → 值)，下一轮只推与它相比的变化。 */
  const lastStateValues = new Map()
  /** 每 (entity, session) 补偿包缓存：键 `entity\0sessionId`，服务端按会话过滤记忆。 */
  const compensationPackages = new Map()
  /** 每会话上次注入的补偿包正文签名：正文相同则跳过（头行时间戳不算变化）。 */
  const lastCompensationSignatures = new Map()
  /** 每会话已经推送过的记忆片段 id：同一会话不再重复收到同一条记忆。 */
  const pushedMemoryIds = new Map()
  const lastTurnByAgent = new WeakMap()
  /** 主会话推送游标：`entity\0sessionId` → { cursor, feedVersion, pushedAt }（Observation v2）。 */
  const mainPushes = new Map()
  /** 每实体对话摘要 Feed（K10，内存态）。 */
  const dialogFeed = createDialogFeed({
    maxEntries: config.dialogFeedSize,
    excerptChars: config.dialogExcerptChars,
  })
  /** 桥接见过的 agent 缓存：sessionId → agent（K5 回落）。 */
  const seenAgentCache = new Map()
  /** 每实体面板投影缓存：entity → { board, paths, checkedAt, failures, inflight }（Observation v2）。 */
  const panelBoards = new Map()
  let disposeBlackboardRoute
  let disposeControlRoutes
  let disposeMainSessionPromptRoute
  let disposeSessionBoardRoute
  let relayTool
  let disposeRelay
  let disposeToolsScope
  let subscriptionTool
  let disposeSubscription
  let mainPromptTool
  let disposeMainPrompt
  let boardTool
  let disposeBoardTool
  /** K17：订阅间隔覆盖（内存 + 持久化，路径由 `subscriptionStatePath` 决定）。 */
  const intervalStore = createIntervalStore({ path: config.subscriptionStatePath, logger })
  /** K28：按实体的主会话提示词（qqbot 主会话通过服务读取；面板经路由编辑）。 */
  const mainSessionPromptStore = createMainSessionPromptStore({ path: config.mainSessionPromptStatePath, logger })
  /** K8/K11：会话黑板订阅游标（`boardEnabled=false` 时不建存储）。 */
  const boardSubscriptionStore = config.boardEnabled
    ? createBoardSubscriptionStore({ path: config.boardSubscriptionStatePath, logger })
    : undefined

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
   * 取（或建）某实体的面板投影缓存条目（Observation v2）。
   * @param {string} entity 实体 id
   * @returns {{board: object|null, paths: string[], checkedAt: number, failures: number, inflight: Promise<void> | undefined}} 缓存条目
   */
  function panelBoardEntry(entity) {
    let entry = panelBoards.get(entity)
    if (entry === undefined) {
      entry = { board: null, paths: [], checkedAt: 0, failures: 0, inflight: undefined }
      panelBoards.set(entity, entry)
    }
    return entry
  }

  /**
   * 拉一次某实体的现状并投影成面板 board；失败保留上一版并限频告警。
   * @param {string} entity 实体 id
   * @param {object} entry 缓存条目
   */
  async function loadPanelBoard(entity, entry) {
    try {
      const payload = await client.fetchObservationState(`entity:${entity}`)
      if (disposed) return
      entry.board = boardFromObservationState(entity, payload)
      entry.paths = statePathsOf(payload)
      entry.failures = 0
    } catch (error) {
      entry.failures += 1
      logger.warnOnce(
        `observation-state:${entity}`,
        `控制面 /v2/observations/state 拉取失败 ${entry.failures} 次（实体 ${entity}，只记日志）：${errorMessage(error)}`,
      )
    }
  }

  /**
   * 取某实体当前面板投影：窗口内复用缓存；并发复用同一在飞请求；`force` 忽略窗口。
   * @param {string} entity 实体 id
   * @param {boolean} [force] 是否强制刷新
   * @returns {Promise<object | null>} 面板 board（无数据/失败时为 null 或上一版）
   */
  async function currentPanelBoard(entity, force = false) {
    if (disposed) return null
    const entry = panelBoardEntry(entity)
    if (force) {
      await loadPanelBoard(entity, entry)
      return entry.board
    }
    const at = Date.now()
    if (entry.checkedAt !== 0 && at - entry.checkedAt < config.promptRefreshMs) return entry.board
    entry.checkedAt = at
    if (entry.inflight === undefined) {
      entry.inflight = Promise.resolve()
        .then(() => loadPanelBoard(entity, entry))
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

  /**
   * 面板数据源（异步）：先按刷新窗口更新全部实体的投影，再返回缓存。
   * @returns {Promise<Array<{entity: string, board: object|null}>>} 面板数据
   */
  async function blackboardListAsync() {
    const ids = boardEntityIds()
    await Promise.all(ids.map((entity) => currentPanelBoard(entity)))
    return ids.map((entity) => ({ entity, board: panelBoardEntry(entity).board }))
  }

  /** `astrboxBridge.blackboards()`：同步返回缓存快照，后台按刷新窗口更新。 */
  function blackboardList() {
    try {
      return boardEntityIds().map((entity) => {
        void currentPanelBoard(entity).catch(() => {})
        return { entity, board: panelBoardEntry(entity).board }
      })
    } catch {
      return []
    }
  }

  /**
   * 面板快照条元数据（K7b）：生效实体 + 观测现状里的路径（最多 32 条）。
   * 同步、不抛错；配置异常由路由侧再兜一层。
   * @returns {{primary: string, paths: string[]}} 元数据
   */
  function panelMeta() {
    return { primary: config.entity, paths: [...panelBoardEntry(config.entity).paths] }
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
   * 写一条会话级缓存；超过上限时淘汰最早建立的会话。
   * @param {Map<string, unknown>} map 会话级 Map
   * @param {string} key 键
   * @param {unknown} value 值
   */
  function setSessionEntry(map, key, value) {
    map.delete(key)
    map.set(key, value)
    if (map.size > SESSION_CACHE_LIMIT) {
      const oldest = map.keys().next()
      if (!oldest.done) map.delete(oldest.value)
    }
  }

  /**
   * 某会话已推送记忆 id 集合（懒建、有上限）。
   * @param {string} sessionId 会话 id
   * @returns {Set<string>} 已推送 id
   */
  function pushedMemoryOf(sessionId) {
    let seen = pushedMemoryIds.get(sessionId)
    if (seen === undefined) {
      seen = new Set()
      setSessionEntry(pushedMemoryIds, sessionId, seen)
    }
    return seen
  }

  /**
   * 造一条注入 user 消息（现状卡 / 变化推送共用；形状与宿主 `createUserMessage` 一致）。
   * @param {string} text 注入文本
   * @returns {object} user 消息
   */
  function createInjectedMessage(text) {
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
   * 主会话推送文本：renderDelta 正文 + 新增对话段（同一条消息附带，不再独立链路）。
   * @param {string} deltaText renderDelta 正文
   * @param {Array<object>} dialogs 上次推送之后新增的对话条目
   * @returns {string} 推送文本
   */
  function composePushText(deltaText, dialogs) {
    const lines = []
    for (const entry of dialogs) {
      const line = formatDialogLine(entry)
      if (line !== '') lines.push(line)
    }
    if (lines.length === 0) return deltaText
    return `${deltaText}\n最近对话：\n${lines.join('\n')}`
  }

  /**
   * 从 `/v2/observations` 响应取下一次的 `since`：优先 `cursor`（服务端跳过噪声），
   * 再回落 `version`（契约 §3.1）。
   * @param {unknown} payload 响应体
   * @returns {number} 下一次 since
   */
  function cursorOf(payload) {
    if (payload === null || typeof payload !== 'object') return 0
    if (Number.isInteger(payload.cursor) && payload.cursor >= 0) return payload.cursor
    if (Number.isInteger(payload.version) && payload.version >= 0) return payload.version
    return 0
  }

  /**
   * 主会话变化推送（Observation v2）：tick 成功后按实体游标拉 delta 并 followup。
   *
   * 首次见到某目标时只以当前 `version` 建立游标、**不回放历史**；之后只推 `seq > cursor` 的变化。
   * 对话 feed 不单独触发，只在有变化时作为同一条消息的「最近对话」段附带。
   * 成功才推进游标与 `feedVersion`/时间；agent 不在线、拉取失败、followup 抛错都下个 tick 重试。
   * @returns {Promise<void>} 完成信号
   */
  async function pushMainSubscriptions() {
    if (disposed || !config.subscriptionEnabled) return
    for (const target of mainPushTargets()) {
      if (disposed) return
      const entry = mainPushes.get(target.key)
      if (entry !== undefined && Date.now() - entry.pushedAt < effectiveInterval(target.sessionId)) continue
      try {
        const agent = agentOf(target.sessionId)
        if (agent === undefined || typeof agent.followup !== 'function') {
          logger.debug(`主会话推送跳过（agent 不在线）：${target.entity}/${target.sessionId}`)
          continue
        }
        const scope = `entity:${target.entity}`
        if (entry === undefined) {
          // 首次：用现状响应的 version 作为起点，不回放历史（不发 since=0 拉全量事件流）。
          const baseline = await client.fetchObservationState(scope)
          if (disposed) return
          mainPushes.set(target.key, {
            cursor: cursorOf(baseline),
            feedVersion: dialogFeed.version(target.entity),
            pushedAt: 0,
          })
          continue
        }
        const payload = await client.fetchObservations(scope, entry.cursor)
        if (disposed) return
        const cursor = cursorOf(payload)
        const feedVersion = dialogFeed.version(target.entity)
        const deltaText = renderDelta(payload)
        if (deltaText === '') {
          // 没有可推变化（noise/阈值/无事件）：只推进游标，不打扰会话。
          mainPushes.set(target.key, { ...entry, cursor })
          continue
        }
        const dialogs = dialogFeed.dialogsSince(target.entity, entry.feedVersion)
        const text = truncateText(composePushText(deltaText, dialogs), config.subscriptionMaxChars)
        agent.followup(createInjectedMessage(text))
        mainPushes.set(target.key, { cursor, feedVersion, pushedAt: Date.now() })
      } catch (error) {
        logger.debug(`主会话变化推送失败（下个 tick 重试）：${errorMessage(error)}`)
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
   * 取（或建）某实体的现状缓存条目：TTL、失败计数、在飞请求都按实体隔离（Observation v2）。
   * @param {string} entity 实体 id
   * @returns {{text: string, structured: object|undefined, values: Map<string, string>|undefined, checkedAt: number, failures: number, inflight: Promise<void> | undefined}} 缓存条目
   */
  function stateEntry(entity) {
    let entry = observationStates.get(entity)
    if (entry === undefined) {
      entry = { text: '', structured: undefined, values: undefined, checkedAt: 0, failures: 0, inflight: undefined }
      observationStates.set(entity, entry)
    }
    return entry
  }

  /**
   * 拉一次某实体的现状并渲染成现状卡；连续失败达上限后本轮不注入（Observation v2）。
   * @param {string} entity 实体 id
   * @param {object} entry 缓存条目
   */
  async function loadObservationState(entity, entry) {
    try {
      const payload = await client.fetchObservationState(`entity:${entity}`)
      if (disposed) return
      entry.text = renderState(payload, { maxChars: config.promptMaxChars })
      entry.structured = { scope: payload?.scope, entries: Array.isArray(payload?.entries) ? payload.entries : [] }
      entry.values = stateValuesOf(payload)
      entry.failures = 0
    } catch (error) {
      entry.failures += 1
      if (entry.failures >= STATE_FAILURE_LIMIT) {
        entry.text = ''
        logger.warnOnce(
          `observation-state:${entity}`,
          `控制面 /v2/observations/state 连续失败 ${entry.failures} 次（实体 ${entity}），本轮不注入现状卡：${errorMessage(error)}`,
        )
      }
    }
  }

  /**
   * 取某实体当前现状卡文本：窗口内复用缓存；同实体并发复用同一在飞请求。
   * 失败也占一个刷新窗口，避免失败时每轮打服务端。
   * @param {string} entity 实体 id
   * @returns {Promise<{text: string, structured: object|undefined, values: Map<string, string>|undefined}>} 现状快照
   */
  async function currentObservationState(entity) {
    if (disposed) return { text: '', structured: undefined, values: undefined }
    const entry = stateEntry(entity)
    const at = Date.now()
    if (entry.checkedAt !== 0 && at - entry.checkedAt < config.promptRefreshMs) return { text: entry.text, structured: entry.structured, values: entry.values }
    entry.checkedAt = at
    if (entry.inflight === undefined) {
      entry.inflight = Promise.resolve()
        .then(() => loadObservationState(entity, entry))
        .finally(() => {
          entry.inflight = undefined
        })
    }
    await entry.inflight
    return { text: entry.text, structured: entry.structured, values: entry.values }
  }

  /**
   * 取（或建）某会话的补偿包缓存条目：服务端按 session 过滤本会话来源，
   * 所以缓存键必须是 (entity, sessionId)，不能只按实体。
   * @param {string} entity 实体 id
   * @param {string} sessionId 会话 id（可为空串）
   * @returns {{text: string, checkedAt: number, failures: number, inflight: Promise<void> | undefined}} 缓存条目
   */
  function compensationEntry(entity, sessionId) {
    const key = `${entity}\u0000${sessionId ?? ''}`
    let entry = compensationPackages.get(key)
    if (entry === undefined) {
      entry = { text: '', ids: [], checkedAt: 0, failures: 0, inflight: undefined }
      setSessionEntry(compensationPackages, key, entry)
    }
    return entry
  }

  /**
   * 拉一次某实体的补偿包并渲染；连续失败达上限后本轮不注入。
   * @param {string} entity 实体 id
   * @param {object} entry 缓存条目
   */
  async function loadCompensation(entity, entry, sessionId) {
    try {
      const context = await client.getTurnContext({ entity, sessionId })
      if (disposed) return
      const compensation = context?.compensation
      if (typeof compensation === 'object' && compensation !== null) {
        const allMemory = Array.isArray(compensation.memory) ? compensation.memory : []
        const seen = pushedMemoryOf(sessionId)
        const memory = allMemory.filter((item) => {
          const id = typeof item?.source_snapshot_id === 'string' ? item.source_snapshot_id : ''
          return id === '' || !seen.has(id)
        })
        entry.ids = memory.map((item) => item?.source_snapshot_id).filter((id) => typeof id === 'string' && id !== '')
        entry.text = composeCompensation({
          entity,
          timestamp: new Date().toISOString(),
          compensation: { ...compensation, memory },
          active: Array.isArray(context?.gate?.active) ? context.gate.active : [],
          maxChars: config.promptMaxChars,
          omitEmptyMemory: allMemory.length > 0 && memory.length === 0,
        })
        // 只有头行、没有正文（记忆全被过滤且无约束/情境/专家）→ 没有可注入内容。
        if (!entry.text.includes('\n')) entry.text = ''
      } else {
        entry.text = ''
        entry.ids = []
      }
      entry.failures = 0
    } catch (error) {
      entry.failures += 1
      if (entry.failures >= STATE_FAILURE_LIMIT) {
        entry.text = ''
        logger.warnOnce(
          `compensation:${entity}`,
          `控制面 /turn-context 连续失败 ${entry.failures} 次（实体 ${entity}），本轮不注入补偿包：${errorMessage(error)}`,
        )
      }
    }
  }

  /**
   * 取某实体当前补偿包文本：窗口内复用缓存；同实体并发复用同一在飞请求。
   * @param {string} entity 实体 id
   * @param {string} sessionId 会话 id（可为空串）
   * @returns {Promise<{text: string, ids: string[]}>} 补偿包文本与其记忆片段 id
   */
  async function currentCompensation(entity, sessionId) {
    if (disposed) return { text: '', ids: [] }
    const entry = compensationEntry(entity, sessionId)
    const at = Date.now()
    if (entry.checkedAt !== 0 && at - entry.checkedAt < config.promptRefreshMs) return { text: entry.text, ids: entry.ids ?? [] }
    entry.checkedAt = at
    if (entry.inflight === undefined) {
      entry.inflight = Promise.resolve()
        .then(() => loadCompensation(entity, entry, sessionId))
        .finally(() => {
          entry.inflight = undefined
        })
    }
    await entry.inflight
    return { text: entry.text, ids: entry.ids ?? [] }
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
  // K28：主会话提示词（同步只读；未配置的实体回空串，qqbot 据此决定是否安装提示段）。
  host.mainSessionPrompt = (entityId) => mainSessionPromptStore.get(entityId)

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
        const disposeRoutes = registerBlackboardRoute(webServer, { getBoards: blackboardListAsync, getMeta: panelMeta, logger })
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
   * K21：把 `/astrbox/control/*` 挂到同一个 webServer（缺失时整段跳过，插件照常工作）。
   * 控制面写语义由 Go 侧实现；桥接只做转发与输入校验。
   */
  function installControlRoutes() {
    if (typeof ctx.inject !== 'function') return
    try {
      ctx.inject(['webServer'], (scoped) => {
        if (disposed) return
        const webServer = scoped?.webServer ?? (typeof scoped?.get === 'function' ? scoped.get('webServer') : undefined)
        if (webServer === undefined || typeof webServer.register !== 'function') return
        const disposeRoutes = registerControlRoutes(webServer, { client, logger })
        disposeControlRoutes = disposeRoutes
        try {
          if (typeof scoped?.effect === 'function') scoped.effect(() => disposeRoutes, 'astrbox-bridge.control-routes')
        } catch (error) {
          logger.warnOnce('control-route', `控制路由生命周期注册失败（只记日志）：${errorMessage(error)}`)
        }
      })
    } catch (error) {
      logger.warnOnce('control-route', `控制路由注入失败（面板控制不可用）：${errorMessage(error)}`)
    }
  }

  /**
   * K28：把 `/astrbox/main-session-prompts` 挂到同一个 webServer（缺失时整段跳过，插件照常工作）。
   * 读写给面板编辑主会话提示词；写入作用于桥接侧存储文件，不触碰控制面。
   */
  function installMainSessionPromptRoute() {
    if (typeof ctx.inject !== 'function') return
    try {
      ctx.inject(['webServer'], (scoped) => {
        if (disposed) return
        const webServer = scoped?.webServer ?? (typeof scoped?.get === 'function' ? scoped.get('webServer') : undefined)
        if (webServer === undefined || typeof webServer.register !== 'function') return
        const disposeRoutes = registerMainSessionPromptRoute(webServer, { store: mainSessionPromptStore, logger })
        disposeMainSessionPromptRoute = disposeRoutes
        try {
          if (typeof scoped?.effect === 'function') scoped.effect(() => disposeRoutes, 'astrbox-bridge.main-session-prompt-route')
        } catch (error) {
          logger.warnOnce('main-session-prompt-route', `主会话提示词路由生命周期注册失败（只记日志）：${errorMessage(error)}`)
        }
      })
    } catch (error) {
      logger.warnOnce('main-session-prompt-route', `主会话提示词路由注入失败（面板编辑不可用）：${errorMessage(error)}`)
    }
  }

  /**
   * K15：把 `/astrbox/boards`、`/astrbox/boards/workspaces`、`/astrbox/boards/panel`
   * 挂到同一个 webServer（缺失时整段跳过，插件照常工作）。
   * 读路由透传控制面 body/状态；面板写路由由 Go 侧固定 `by=面板`。
   */
  function installSessionBoardRoute() {
    if (typeof ctx.inject !== 'function') return
    try {
      ctx.inject(['webServer'], (scoped) => {
        if (disposed) return
        const webServer = scoped?.webServer ?? (typeof scoped?.get === 'function' ? scoped.get('webServer') : undefined)
        if (webServer === undefined || typeof webServer.register !== 'function') return
        const disposeRoutes = registerSessionBoardRoute(webServer, { client, logger })
        disposeSessionBoardRoute = disposeRoutes
        try {
          if (typeof scoped?.effect === 'function') scoped.effect(() => disposeRoutes, 'astrbox-bridge.session-board-route')
        } catch (error) {
          logger.warnOnce('session-board-route', `会话黑板路由生命周期注册失败（只记日志）：${errorMessage(error)}`)
        }
      })
    } catch (error) {
      logger.warnOnce('session-board-route', `会话黑板路由注入失败（面板会话黑板不可用）：${errorMessage(error)}`)
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
   *
   * 冷启动时插件 apply 可能早于 tools 服务就绪：先走 `ctx.inject(['tools'], …)`
   * 等它可用（服务已就绪则立即回调）；ctx.inject 不可用（单测替身/旧宿主）时
   * 按 `ctx.get('tools')` 立即注册一次。注册只做一次；服务缺失只 warn。
   */
  function installTools() {
    let registered = false
    const register = (tools) => {
      if (registered || disposed) return
      if (tools === undefined || tools === null || typeof tools.register !== 'function') return
      registered = true
      registerToolDefinitions(tools)
    }
    // 1) 立即尝试（插件热重载时 tools 通常已就绪）。
    let immediate
    try {
      immediate = typeof ctx.get === 'function' ? ctx.get('tools') : undefined
    } catch {
      immediate = undefined
    }
    register(immediate)
    if (registered) return
    // 2) 冷启动：tools 尚未就绪时等它出现再注册（只注册一次）。
    if (typeof ctx.inject === 'function') {
      try {
        disposeToolsScope = ctx.inject(['tools'], (scoped) => {
          const tools = scoped?.tools ?? (typeof scoped?.get === 'function' ? scoped.get('tools') : undefined)
          register(tools)
        })
      } catch (error) {
        logger.warnOnce('tools', `tools 服务注入失败：${errorMessage(error)}`)
      }
    }
    if (!registered) {
      logger.warnOnce('tools', 'tools 服务不可用，astrbox_relay/astrbox_subscription/astrbox_main_prompt 未注册（插件继续工作）')
    }
  }

  /**
   * 实际注册两个工具（K14 relay / K17 subscription）；各自失败互不影响。
   * @param {object} tools `tools` 服务（需有 `register`）
   */
  function registerToolDefinitions(tools) {
    try {
      relayTool = createRelayTool({
        getBindings: currentBindings,
        agentOf,
        // K18：来源标签需要会话标题（主体 / 私聊(…) / 群聊(…)），缺失时工具内部回退「会话」。
        sessionTitleOf,
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
    try {
      mainPromptTool = createMainPromptTool({
        store: mainSessionPromptStore,
        getBindings: currentBindings,
        logger,
      })
      disposeMainPrompt = tools.register(mainPromptTool.definition)
    } catch (error) {
      logger.warnOnce('main-prompt-tool', `astrbox_main_prompt 注册失败（插件继续工作）：${errorMessage(error)}`)
    }
    // K30/K11：会话黑板工具（总开关关闭时整段跳过）。
    if (config.boardEnabled && boardSubscriptionStore !== undefined) {
      try {
        boardTool = createSessionBoardTool({
          client,
          store: boardSubscriptionStore,
          getBindings: currentBindings,
          logger,
        })
        disposeBoardTool = tools.register(boardTool.definition)
      } catch (error) {
        logger.warnOnce('board-tool', `astrbox_board 注册失败（插件继续工作）：${errorMessage(error)}`)
      }
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

  /**
   * K8：会话黑板差异文本（显式订阅 + pre-step 续传；workspace 段在前、global 段在后）。
   *
   * 每个 scope 独立读取：某 scope 失败只跳过该 scope（warnOnce），不影响本轮。
   * 无差异 → `''`（不注入）；游标只在文本已交给本轮 decision 后推进（至少一次投递：
   * 落盘失败时下一轮会重复注入，不丢失）；`truncated`/`since > board.version` 走重同步。
   * @param {object | undefined} agent pre-step 的 agent
   * @returns {Promise<string>} 注入文本（不注入时 `''`）
   */
  async function sessionBoardTextFor(agent) {
    try {
      if (!config.boardEnabled || disposed || boardSubscriptionStore === undefined) return ''
      const caller = callerOf({ agent }, currentBindings)
      if (caller.sessionId === '') return ''
      const record = boardSubscriptionStore.get(caller.sessionId)
      if (record === undefined) return ''
      const sections = []
      const scopes = ['workspace', 'global']
      for (const scope of scopes) {
        const cursor = record[scope]
        if (cursor === undefined) continue
        // K8：no_workspace 时跳过 workspace 板，global 照常。
        if (scope === 'workspace' && caller.workspaceKey === '') continue
        try {
          const response = await client.readBoard({
            scope,
            ...(scope === 'workspace' ? { workspace: caller.workspaceKey } : {}),
            since: cursor.since,
          })
          if (disposed) return ''
          if (response === null || typeof response !== 'object' || response.ok !== true || response.board === null || typeof response.board !== 'object') {
            logger.warnOnce('board-pre-step', `控制面 /boards（${scope}）响应非法，本轮跳过该板`)
            continue
          }
          sections.push({
            scope,
            label: caller.label,
            board: response.board,
            deltas: response.deltas,
            since: cursor.since,
            truncated: response.truncated === true,
          })
        } catch (error) {
          logger.warnOnce('board-pre-step', `控制面 /boards（${scope}）读取失败，本轮跳过该板：${errorMessage(error)}`)
        }
      }
      if (sections.length === 0) return ''
      const maxChars = config.boardSubscriptionMaxChars
      const plan = planBoardInjection({ sections, callerSessionId: caller.sessionId, maxChars })
      if (plan.invalidScopes.length > 0) {
        logger.warnOnce('board-pre-step', `会话黑板（${plan.invalidScopes.join('、')}）差异形状非法，本轮不注入、不推进游标`)
      }
      for (const [scope, since] of Object.entries(plan.cursors)) {
        boardSubscriptionStore.setCursor(caller.sessionId, scope, since)
      }
      return plan.text
    } catch (error) {
      logger.warnOnce('board-pre-step', `会话黑板注入失败（已忽略，不影响本轮）：${errorMessage(error)}`)
      return ''
    }
  }

  /**
   * `agent/pre-step`：step 1 注入 Observation v2 现状卡（现状未变默认跳过）+ 补偿包
   *（记忆/约束/情境/专家）+ 会话黑板差异（顺序固定：现状卡 → 补偿包 → 会话黑板）；
   * 异常都不得打断宿主。
   */
  async function onPreStep(payload, next) {
    markActivity()
    const decision = await next()
    try {
      if (disposed || decision === undefined || decision === null || decision.kind === 'reject') return decision
      const { agent, step, turn } = payload ?? {}
      if (step !== 1) return decision
      rememberAgent(agent)
      if (agent !== undefined && lastTurnByAgent.get(agent) === turn) return decision
      const entity = resolveEntity(agent)
      const sessionId = typeof agent?.session?.id === 'string' ? agent.session.id : ''
      const state = await currentObservationState(entity)
      const compensation = await currentCompensation(entity, sessionId)
      const compensationText = compensation.text
      const boardText = await sessionBoardTextFor(agent)
      // 首轮给完整现状卡；之后只给与上次相比变化的字段（无变化就不注入）。
      const previousState = lastStateValues.get(sessionId)
      const stateText = previousState === undefined
        ? state.text
        : renderStateDelta(state.structured, previousState, { maxChars: config.promptMaxChars })
      const stateDue = stateText !== ''
      // 头行带每次刷新的时间戳：按正文签名比较，否则内容没变也会被当成新包反复注入。
      const compensationKey = compensationSignature(compensationText)
      const compensationDue = compensationText !== '' && compensationKey !== lastCompensationSignatures.get(sessionId)
      if (!stateDue && !compensationDue && boardText === '') return decision
      if (agent !== undefined) lastTurnByAgent.set(agent, turn)
      const messages = [...decision.messages]
      if (stateDue) {
        messages.push(createInjectedMessage(stateText))
        if (state.values instanceof Map) setSessionEntry(lastStateValues, sessionId, state.values)
      }
      if (compensationDue) {
        messages.push(createInjectedMessage(compensationText))
        lastCompensationSignatures.set(sessionId, compensationKey)
        if (compensation.ids.length > 0) {
          const seen = pushedMemoryOf(sessionId)
          for (const id of compensation.ids) {
            if (seen.has(id)) continue
            seen.add(id)
            if (seen.size > PUSHED_MEMORY_LIMIT) {
              const oldest = seen.values().next()
              if (!oldest.done) seen.delete(oldest.value)
            }
          }
        }
      }
      if (boardText !== '') messages.push(createInjectedMessage(boardText))
      return { ...decision, messages }
    } catch (error) {
      logger.warnOnce('pre-step', `pre-step 注入失败（已忽略）：${errorMessage(error)}`)
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
  installControlRoutes()
  installMainSessionPromptRoute()
  installSessionBoardRoute()
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
        disposeControlRoutes?.()
      } catch {
        // 路由可能已随 webServer fiber 释放：忽略。
      }
      try {
        disposeMainSessionPromptRoute?.()
      } catch {
        // 路由可能已随 webServer fiber 释放：忽略。
      }
      try {
        disposeSessionBoardRoute?.()
      } catch {
        // 路由可能已随 webServer fiber 释放：忽略。
      }
      try {
        disposeToolsScope?.()
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
      try {
        disposeMainPrompt?.()
      } catch {
        // 工具可能已随 tools fiber 释放：忽略。
      }
      try {
        disposeBoardTool?.()
      } catch {
        // 工具可能已随 tools fiber 释放：忽略。
      }
      dialogFeed.clear()
      lastStateValues.clear()
      pushedMemoryIds.clear()
      lastCompensationSignatures.clear()
      observationStates.clear()
      panelBoards.clear()
      mainPushes.clear()
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
