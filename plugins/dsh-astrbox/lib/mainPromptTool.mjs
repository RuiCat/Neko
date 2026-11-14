/**
 * K29 `astrbox_main_prompt` 工具：让智慧体（主会话）自行维护按实体的主会话提示词。
 *
 * - 读：`entity` 缺省 = 调用方主会话绑定的实体；任意会话可读（子会话只读）。
 * - 写：只有主会话（会话 id 命中某绑定的 `mainSessionId`）可写，且可写任意实体
 *   （猫猫替梦梦改提示词是真实场景）；`prompt` 空串 = 清除该实体。
 * - 列：`list: true` → 各实体与其字数。
 *
 * 与 K28 面板、路由共用同一个存储（主会话提示词 JSON）：写入后下一轮由 qqbot 主会话作为
 * systemPrompt 段注入，无需重启。所有失败返回值、不抛异常（fail-soft）。
 */
import { MAIN_SESSION_PROMPT_MAX_CHARS } from './mainSessionPromptStore.mjs'

/** 工具名（K29 冻结）。 */
export const MAIN_PROMPT_TOOL_NAME = 'astrbox_main_prompt'

/**
 * 调用方会话 id（`exec.agent.session.id`，兜底 `agent.id`）。
 * @param {object|undefined} exec 工具执行上下文
 * @returns {string} 会话 id（未知时为空串）
 */
function callerSessionId(exec) {
  const agent = exec?.agent
  if (typeof agent?.session?.id === 'string') return agent.session.id
  return typeof agent?.id === 'string' ? agent.id : ''
}

/**
 * 会话 id 命中的主会话绑定实体（K29：判定调用方是否主会话、缺省读写哪个实体）。
 * @param {Array<object>} bindings 规范化绑定
 * @param {string} sessionId 调用方会话 id
 * @returns {string|undefined} 绑定实体；不是主会话时为 undefined
 */
function entityOfSession(bindings, sessionId) {
  if (sessionId === '') return undefined
  for (const binding of bindings) {
    if (typeof binding?.mainSessionId !== 'string' || binding.mainSessionId !== sessionId) continue
    return typeof binding.entity === 'string' && binding.entity.trim() !== '' ? binding.entity.trim() : undefined
  }
  return undefined
}

/**
 * 实体 → 字数快照；读取异常回空数组（fail-soft）。
 * @param {object} store 提示词存储
 * @returns {Array<{entity: string, chars: number}>} 实体列表
 */
function entityListOf(store) {
  try {
    const list = typeof store?.list === 'function' ? store.list() : {}
    if (list === null || typeof list !== 'object' || Array.isArray(list)) return []
    return Object.entries(list).map(([entity, text]) => ({ entity, chars: typeof text === 'string' ? text.length : 0 }))
  } catch {
    return []
  }
}

/**
 * @param {unknown} error 原始错误
 * @returns {string} 可读错误文本
 */
function errorMessage(error) {
  return error instanceof Error ? error.message : String(error)
}

/**
 * 渲染工具结果（output.render 必须纯函数且不抛错）。
 * @param {unknown} value execute 返回的规范值
 * @returns {string} 渲染文本
 */
function renderMainPromptResult(value) {
  const result = value !== null && typeof value === 'object' ? value : {}
  if (result.ok !== true) return `主提示词：未完成（${String(result.reason ?? 'unknown')}）`
  if (Array.isArray(result.entities)) {
    const items = result.entities.map((item) => `${String(item?.entity ?? '')} ${String(item?.chars ?? 0)} 字`).join('、')
    return `主提示词：共 ${result.entities.length} 个实体${items === '' ? '' : `（${items}）`}`
  }
  if (result.cleared === true) return `已清除 ${String(result.entity ?? '')} 的主提示词`
  const text = typeof result.prompt === 'string' ? result.prompt : ''
  return `主提示词（${String(result.entity ?? '')}，${text.length} 字）：\n${text}`
}

/**
 * 建 `astrbox_main_prompt` 工具（K29）。
 * @param {{store: object, getBindings: () => Array<object>, logger?: object}} options 依赖
 * @returns {{definition: object, execute: Function}} 工具
 */
export function createMainPromptTool(options) {
  const { store, getBindings = () => [], logger } = options ?? {}

  /** @returns {Array<object>} 当前绑定（异常视为无绑定）。 */
  function bindings() {
    try {
      const list = getBindings()
      return Array.isArray(list) ? list : []
    } catch {
      return []
    }
  }

  /** 限频告警；logger 缺失/抛错都静默。 */
  function warnOnce(message) {
    try {
      logger?.warnOnce?.('main-prompt-tool', message)
    } catch {
      // 记录日志失败不影响工具返回值。
    }
  }

  /**
   * 执行一次读 / 写 / 列表。
   * @param {unknown} rawArgs 模型参数 `{entity?, prompt?, list?}`
   * @param {{agent?: object}} [exec] 执行上下文
   * @returns {Promise<object>} 结果对象（永不抛错）
   */
  async function execute(rawArgs, exec) {
    try {
      const args = rawArgs !== null && typeof rawArgs === 'object' ? rawArgs : {}
      const sessionId = callerSessionId(exec)
      const ownEntity = entityOfSession(bindings(), sessionId)
      if (args.list === true) {
        return { ok: true, entities: entityListOf(store), maxChars: MAIN_SESSION_PROMPT_MAX_CHARS }
      }
      if (args.entity !== undefined && typeof args.entity !== 'string') return { ok: false, reason: 'invalid_entity' }
      const entity = typeof args.entity === 'string' && args.entity.trim() !== '' ? args.entity.trim() : ownEntity
      if (entity === undefined || entity === '') return { ok: false, reason: 'need_entity' }
      if (args.prompt === undefined) {
        const prompt = typeof store?.get === 'function' ? store.get(entity) : ''
        return { ok: true, entity, prompt: typeof prompt === 'string' ? prompt : '', maxChars: MAIN_SESSION_PROMPT_MAX_CHARS }
      }
      if (ownEntity === undefined) return { ok: false, reason: 'not_main_session', entity }
      if (typeof args.prompt !== 'string') return { ok: false, reason: 'invalid_prompt', entity }
      const text = args.prompt.trim()
      if (text.length > MAIN_SESSION_PROMPT_MAX_CHARS) return { ok: false, reason: 'prompt_too_long', entity }
      const persisted = store.set(entity, text) !== false
      return { ok: true, entity, prompt: text, cleared: text === '', persisted, maxChars: MAIN_SESSION_PROMPT_MAX_CHARS }
    } catch (error) {
      warnOnce(`astrbox_main_prompt 执行失败（已忽略）：${errorMessage(error)}`)
      return { ok: false, reason: 'internal_error' }
    }
  }

  const definition = {
    name: MAIN_PROMPT_TOOL_NAME,
    description:
      '读取、写入或列出按实体的主会话提示词（Neko 面板的「主会话提示词」同一份存储）。'
      + '省略 prompt = 读取（entity 缺省取自己主会话绑定的实体）；给 prompt = 写入，空串 = 清除该实体。'
      + 'list=true 列出全部实体与字数。写入只允许主会话（子会话返回 not_main_session）；写入后下一轮生效。',
    parameters: {
      type: 'object',
      properties: {
        entity: {
          type: 'string',
          description: '实体 id（梦梦/猫猫…）；省略 = 自己主会话绑定的实体。读取任意实体，写入需要主会话。',
        },
        prompt: {
          type: 'string',
          description: '要写入的提示词（给参数即写入，空串 = 清除该实体）；省略 = 读取。',
        },
        list: {
          type: 'boolean',
          description: 'true = 列出全部实体与各自字数。',
        },
      },
      additionalProperties: false,
    },
    output: {
      schema: {
        type: 'object',
        properties: {
          ok: { type: 'boolean' },
          entity: { type: 'string' },
          prompt: { type: 'string' },
          cleared: { type: 'boolean' },
          persisted: { type: 'boolean' },
          maxChars: { type: 'integer' },
          entities: {
            type: 'array',
            items: {
              type: 'object',
              properties: { entity: { type: 'string' }, chars: { type: 'integer' } },
              additionalProperties: false,
            },
          },
          reason: { type: 'string' },
        },
        required: ['ok'],
        additionalProperties: false,
      },
      render: (_args, value) => [{ type: 'text', text: renderMainPromptResult(value) }],
    },
    execute: (args, exec) => execute(args, exec),
  }

  return { definition, execute }
}
