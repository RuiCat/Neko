/**
 * K14 `astrbox_relay` 工具：把一句话转达给另一个 Neko 实体/会话（跨会话交互）。
 *
 * - `target` = 实体 id（梦梦/猫猫）→ `followup` 到该实体主会话，正文前缀
 *   `[astrbox-relay] <来源实体>·<会话标题>（<sessionId 前 8>） 对你说：`（K18）；
 * - `target` = `私聊`/`群聊` → 用 qqbot 服务 `say({botId, scope, text})` 直发 QQ（K15，不触发模型回合）；
 * - `target` = `主体` → 调用方所属 bot 的主会话。
 *
 * 限流：同一调用方（会话 id）每分钟最多 6 次，超限返回 `{delivered:false, reason:'rate_limited'}`；
 * 不自动回传（收到 relay 的一方想回话需自己再调用工具）。所有失败都返回值、不抛异常。
 */
import { randomUUID } from 'node:crypto'
import { resolveEntityForCwd } from './entityBindings.mjs'

/** 工具名（K14 冻结）。 */
export const RELAY_TOOL_NAME = 'astrbox_relay'

/** 正文上限（字/码点，K14：≤1000 字）。 */
export const RELAY_TEXT_MAX_CHARS = 1000

/** 同一调用方每分钟最多转达次数（K14）。 */
export const RELAY_RATE_LIMIT = 6

/** 限流窗口（毫秒）。 */
export const RELAY_RATE_WINDOW_MS = 60000

/** 转达正文的固定前缀（K14）。 */
export const RELAY_PREFIX = '[astrbox-relay]'

/** 限流表最多保留的调用方数量（防御异常增长）。 */
const RELAY_CALLER_LIMIT = 64

/**
 * 造一条转达用 user 消息（形状与宿主 `createUserMessage` 一致；本插件零依赖）。
 * @param {string} text 正文
 * @returns {object} user 消息
 */
function createRelayMessage(text) {
  return {
    id: randomUUID(),
    role: 'user',
    content: [{ type: 'text', text }],
    source: { kind: 'user' },
  }
}

/**
 * 转达工具的渲染文本（output.render 必须纯函数且不抛错）。
 * @param {unknown} value execute 返回的规范值
 * @returns {string} 渲染文本
 */
function renderRelayResult(value) {
  const result = value !== null && typeof value === 'object' ? value : {}
  if (result.delivered === true) {
    return result.kind === 'qq' ? `已转达到 QQ ${String(result.scope ?? '')}` : `已转达到 ${String(result.entity ?? '')} 主会话`
  }
  return `未送达：${String(result.reason ?? 'unknown')}`
}

/**
 * 建 `astrbox_relay` 工具（K14/K18；注册形状参考 qqbot 的 send-file-tool）。
 * @param {{getBindings: Function, agentOf: Function, qqbotService?: Function,
 *   sessionTitleOf?: Function, defaultEntity?: string, logger?: object, now?: Function}} options 依赖
 * @returns {{definition: object, execute: Function, dispose: Function}} 工具
 */
export function createRelayTool(options) {
  const {
    getBindings = () => [],
    agentOf = () => undefined,
    qqbotService = () => undefined,
    sessionTitleOf = () => '',
    defaultEntity = '',
    logger,
    now = Date.now,
  } = options ?? {}
  /** 调用方 sessionId → 最近调用时间戳数组（滑动窗口）。 */
  const calls = new Map()

  /** @returns {Array<object>} 当前绑定（任何异常都视为无绑定）。 */
  function bindings() {
    try {
      const list = getBindings()
      return Array.isArray(list) ? list : []
    } catch {
      return []
    }
  }

  /**
   * 调用方上下文：会话 id、cwd 命中的绑定、来源实体（严格）与 `target=主体` 用的回退实体。
   * K18：来源实体**不**回落 defaultEntity（未命中即空，标签写「未绑定会话」）；
   * `target=主体` 仍用 `targetEntity`（含 defaultEntity 回退，K14 语义不变）。
   * @param {object|undefined} exec 工具执行上下文（exec.agent）
   * @returns {{sessionId: string, binding: object|undefined, entity: string,
   *   targetEntity: string, title: string}} 调用方
   */
  function callerOf(exec) {
    const agent = exec?.agent
    const sessionId = typeof agent?.session?.id === 'string' ? agent.session.id : typeof agent?.id === 'string' ? agent.id : ''
    let binding
    try {
      binding = resolveEntityForCwd(bindings(), agent?.session?.header?.cwd)
    } catch {
      binding = undefined
    }
    const boundEntity = typeof binding?.entity === 'string' && binding.entity !== '' ? binding.entity : ''
    const fallback = typeof defaultEntity === 'string' ? defaultEntity : ''
    let title
    try {
      const resolved = sessionTitleOf(agent?.session)
      title = typeof resolved === 'string' ? resolved.trim() : ''
    } catch {
      title = ''
    }
    return { sessionId, binding, entity: boundEntity, targetEntity: boundEntity !== '' ? boundEntity : fallback, title }
  }

  /**
   * K18 来源标签：`<来源实体>·<会话标题>（<sessionId 前 8>）`；
   * 实体未命中写「未绑定会话」，标题缺失回退「会话」。
   * @param {{sessionId: string, entity: string, title: string}} caller 调用方
   * @returns {string} 来源标签
   */
  function sourceLabel(caller) {
    const entity = caller.entity !== '' ? caller.entity : '未绑定会话'
    const title = caller.title !== '' ? caller.title : '会话'
    return `${entity}·${title}（${shortSessionId(caller.sessionId)}）`
  }

  /**
   * 会话 id 短标识：去掉宿主固定的 `session-` 前缀后取前 8 位（否则前 8 位恒为 `session-`）。
   * @param {string} sessionId 会话 id
   * @returns {string} 短标识（空/非法回退「未知」）
   */
  function shortSessionId(sessionId) {
    if (typeof sessionId !== 'string' || sessionId === '') return '未知'
    const stripped = sessionId.startsWith('session-') ? sessionId.slice('session-'.length) : sessionId
    return stripped.slice(0, 8) || '未知'
  }

  /**
   * 给工具结果附加来源信息（K18：成功/失败都带）。
   * @param {object} result 工具结果
   * @param {object} caller 调用方
   * @returns {object} 带 source 的结果
   */
  function withSource(result, caller) {
    return {
      ...result,
      source: {
        sessionId: caller.sessionId,
        entity: caller.entity,
        label: sourceLabel(caller),
      },
    }
  }

  /**
   * 同一调用方滑动窗口限流：每分钟最多 RELAY_RATE_LIMIT 次。
   * @param {string} sessionId 调用方会话 id（空串归入 anonymous）
   * @returns {boolean} 是否允许
   */
  function allowCaller(sessionId) {
    const key = sessionId === '' ? 'anonymous' : sessionId
    const at = now()
    const history = (calls.get(key) ?? []).filter((time) => at - time < RELAY_RATE_WINDOW_MS)
    if (history.length >= RELAY_RATE_LIMIT) {
      calls.set(key, history)
      return false
    }
    history.push(at)
    calls.delete(key)
    calls.set(key, history)
    if (calls.size > RELAY_CALLER_LIMIT) {
      const oldest = calls.keys().next()
      if (!oldest.done) calls.delete(oldest.value)
    }
    return true
  }

  /**
   * 找某实体的主会话绑定（有 `mainSessionId` 的第一条）。
   * @param {string} entity 实体 id
   * @returns {object|undefined} 绑定
   */
  function mainBindingOf(entity) {
    if (typeof entity !== 'string' || entity === '') return undefined
    for (const binding of bindings()) {
      if (binding?.entity !== entity) continue
      if (typeof binding.mainSessionId === 'string' && binding.mainSessionId !== '') return binding
    }
    return undefined
  }

  /**
   * 唯一 bot id（调用方未命中绑定时回退用）；不唯一返回 undefined（交给 qqbot 解析）。
   * @returns {string|undefined} bot id
   */
  function uniqueBotId() {
    const ids = new Set()
    for (const binding of bindings()) {
      if (typeof binding?.botId === 'string' && binding.botId !== '') ids.add(binding.botId)
    }
    return ids.size === 1 ? [...ids][0] : undefined
  }

  /**
   * 转达到某实体主会话（followup；不触发自动回传）。
   * @param {string} entity 目标实体 id
   * @param {string} text 正文
   * @param {{sessionId: string, binding: object|undefined, entity: string}} caller 调用方
   * @returns {object} 规范结果
   */
  function deliverToMain(entity, text, caller) {
    const binding = mainBindingOf(entity)
    if (binding === undefined) return { delivered: false, reason: 'unknown_target' }
    const agent = agentOf(binding.mainSessionId)
    if (agent === undefined || typeof agent.followup !== 'function') return { delivered: false, reason: 'agent_unavailable' }
    try {
      agent.followup(createRelayMessage(`${RELAY_PREFIX} ${sourceLabel(caller)} 对你说：${text}`))
    } catch (error) {
      try {
        logger?.warnOnce?.('relay-tool', `astrbox_relay followup 失败：${error instanceof Error ? error.message : String(error)}`)
      } catch {
        // 日志失败不影响返回值。
      }
      return { delivered: false, reason: 'agent_unavailable' }
    }
    return { delivered: true, kind: 'main', entity, sessionId: binding.mainSessionId }
  }

  /**
   * 转达到 QQ 私聊/群聊（K15 `qqbotBots.say`；不触发模型回合）。
   * @param {'c2c'|'group'} scope 会话类型
   * @param {string} text 正文
   * @param {{sessionId: string, binding: object|undefined, entity: string}} caller 调用方
   * @returns {Promise<object>} 规范结果
   */
  async function deliverToQq(scope, text, caller) {
    let service
    try {
      service = qqbotService()
    } catch {
      service = undefined
    }
    const say = typeof service?.say === 'function' ? service.say : undefined
    if (say === undefined) return { delivered: false, reason: 'qqbot_unavailable' }
    const botId = typeof caller.binding?.botId === 'string' && caller.binding.botId !== '' ? caller.binding.botId : uniqueBotId()
    try {
      const result = await say({ ...(botId === undefined ? {} : { botId }), scope, text })
      if (result?.ok === true) {
        const out = { delivered: true, kind: 'qq', scope }
        if (typeof result.botId === 'string' && result.botId !== '') out.botId = result.botId
        else if (botId !== undefined) out.botId = botId
        if (typeof result.targetId === 'string') out.targetId = result.targetId
        return out
      }
      const reason = typeof result?.reason === 'string' && result.reason !== '' ? result.reason : 'qq_send_failed'
      return { delivered: false, reason }
    } catch (error) {
      try {
        logger?.warnOnce?.('relay-tool', `astrbox_relay say 失败：${error instanceof Error ? error.message : String(error)}`)
      } catch {
        // 日志失败不影响返回值。
      }
      return { delivered: false, reason: 'qq_send_failed' }
    }
  }

  /**
   * 执行一次转达。
   * @param {unknown} rawArgs 模型参数 `{target, text}`
   * @param {{agent?: object, signal?: AbortSignal}} [exec] 执行上下文
   * @returns {Promise<object>} `{delivered, ...}`（失败也给值、不抛错）
   */
  async function execute(rawArgs, exec) {
    const args = rawArgs !== null && typeof rawArgs === 'object' ? rawArgs : {}
    const target = typeof args.target === 'string' ? args.target.trim() : ''
    const text = typeof args.text === 'string' ? args.text.trim() : ''
    const caller = callerOf(exec)
    if (text === '' || Array.from(text).length > RELAY_TEXT_MAX_CHARS) {
      return withSource({ delivered: false, reason: 'invalid_text' }, caller)
    }
    if (target === '') return withSource({ delivered: false, reason: 'unknown_target' }, caller)
    if (!allowCaller(caller.sessionId)) return withSource({ delivered: false, reason: 'rate_limited' }, caller)
    if (target === '私聊') return withSource(await deliverToQq('c2c', text, caller), caller)
    if (target === '群聊') return withSource(await deliverToQq('group', text, caller), caller)
    if (target === '主体') {
      const entity = caller.targetEntity
      if (entity === '') return withSource({ delivered: false, reason: 'unknown_target' }, caller)
      return withSource(deliverToMain(entity, text, caller), caller)
    }
    if (target === '对方主体') {
      // 兼容写法：唯一「另一个实体」的主会话（多实体时无法判定，返回 unknown_target）。
      const others = new Set()
      for (const binding of bindings()) {
        if (typeof binding?.entity === 'string' && binding.entity !== '' && binding.entity !== caller.targetEntity) others.add(binding.entity)
      }
      if (others.size !== 1) return withSource({ delivered: false, reason: 'unknown_target' }, caller)
      return withSource(deliverToMain([...others][0], text, caller), caller)
    }
    return withSource(deliverToMain(target, text, caller), caller)
  }

  const definition = {
    name: RELAY_TOOL_NAME,
    description:
      '把一句话转达给另一个 Neko 实体或会话（跨会话交互）。target 可为实体 id（如 梦梦/猫猫）、'
      + '主体（自己所属 bot 的主会话）、私聊/群聊（经 QQ 机器人直发）。不会自动收到对方回复，'
      + '需要回话时再调用一次。正文最多 1000 字，同一会话每分钟最多 6 次。',
    parameters: {
      type: 'object',
      properties: {
        target: {
          type: 'string',
          description: '目标：实体 id（梦梦/猫猫）、对方主体、主体、私聊、群聊。',
        },
        text: {
          type: 'string',
          description: '要转达的正文（非空，最多 1000 字）。',
        },
      },
      required: ['target', 'text'],
      additionalProperties: false,
    },
    output: {
      schema: {
        type: 'object',
        properties: {
          delivered: { type: 'boolean' },
          kind: { type: 'string' },
          entity: { type: 'string' },
          sessionId: { type: 'string' },
          scope: { type: 'string' },
          botId: { type: 'string' },
          targetId: { type: 'string' },
          reason: { type: 'string' },
          source: {
            type: 'object',
            properties: {
              sessionId: { type: 'string' },
              entity: { type: 'string' },
              label: { type: 'string' },
            },
            required: ['sessionId', 'entity', 'label'],
            additionalProperties: false,
          },
        },
        required: ['delivered'],
        additionalProperties: false,
      },
      render: (_args, value) => [{ type: 'text', text: renderRelayResult(value) }],
    },
    execute: (args, exec) => execute(args, exec),
  }

  return {
    definition,
    execute,
    /** 卸载清理：清空限流表。 */
    dispose() {
      calls.clear()
    },
  }
}
