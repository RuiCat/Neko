/**
 * K17 `astrbox_subscription` 工具：主体（主会话）按自身需要设置订阅推送间隔。
 *
 * - 无参 / `{}` → 读取当前生效间隔；
 * - `minutes` 1..1440 → 为调用方自己的主会话写覆盖（持久化）；
 * - `reset: true` → 清除覆盖，回到全局 `subscriptionMainIntervalMs`。
 *
 * 权限：只有主会话（调用方 sessionId ∈ 某实体 bindings 的 `mainSessionId`）可读可写；
 * 其余会话返回 `{ok:false, reason:'not_main_session'}`。非法分钟数返回 `invalid_minutes`。
 * 工具永不抛异常；读取/写入失败只影响持久化，内存覆盖仍生效（K17 fail-soft）。
 */
import { MAX_INTERVAL_MINUTES, MIN_INTERVAL_MINUTES, isValidIntervalMinutes } from './subscriptionInterval.mjs'

/** 工具名（K17 冻结）。 */
export const SUBSCRIPTION_TOOL_NAME = 'astrbox_subscription'

/** 一分钟的毫秒数。 */
const MINUTE_MS = 60000

/** 默认生效间隔（分钟），仅当配置缺失/非法时兜底。 */
const FALLBACK_DEFAULT_INTERVAL_MS = 600000

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
 * 调用方是否是某实体的主会话（sessionId 命中任一绑定的 `mainSessionId`）。
 * @param {string} sessionId 会话 id
 * @param {Array<object>} bindings 绑定列表
 * @returns {boolean} 是否主会话
 */
function isMainSession(sessionId, bindings) {
  if (sessionId === '') return false
  return bindings.some((binding) => typeof binding?.mainSessionId === 'string' && binding.mainSessionId === sessionId)
}

/**
 * 生效间隔：覆盖分钟数优先，否则全局 `subscriptionMainIntervalMs`。
 * @param {{get: Function}} store 覆盖存储
 * @param {number} defaultIntervalMs 全局间隔（毫秒）
 * @param {string} sessionId 主会话 id
 * @returns {{minutes: number, intervalMs: number, source: 'override'|'default'}} 生效值
 */
function effectiveIntervalOf(store, defaultIntervalMs, sessionId) {
  const override = typeof store?.get === 'function' ? store.get(sessionId) : undefined
  if (isValidIntervalMinutes(override)) {
    return { minutes: override, intervalMs: override * MINUTE_MS, source: 'override' }
  }
  const fallbackMs = Number.isInteger(defaultIntervalMs) && defaultIntervalMs > 0 ? defaultIntervalMs : FALLBACK_DEFAULT_INTERVAL_MS
  const minutes = Math.max(MIN_INTERVAL_MINUTES, Math.min(MAX_INTERVAL_MINUTES, Math.round(fallbackMs / MINUTE_MS)))
  return { minutes, intervalMs: fallbackMs, source: 'default' }
}

/**
 * 渲染工具结果（output.render 必须纯函数且不抛错）。
 * @param {unknown} value execute 返回的规范值
 * @returns {string} 渲染文本
 */
function renderSubscriptionResult(value) {
  const result = value !== null && typeof value === 'object' ? value : {}
  if (result.ok !== true) return `订阅间隔：未生效（${String(result.reason ?? 'unknown')}）`
  const minutes = String(result.minutes ?? '?')
  const source = result.source === 'override' ? '自定义' : '全局默认'
  return `订阅间隔：每 ${minutes} 分钟（${source}，${String(result.intervalMs ?? '?')} ms）`
}

/**
 * 建 `astrbox_subscription` 工具（K17）。
 * @param {{store: object, defaultIntervalMs: number, getBindings: () => Array<object>, logger?: object}} options 依赖
 * @returns {{definition: object, execute: Function}} 工具
 */
export function createSubscriptionTool(options) {
  const { store, defaultIntervalMs, getBindings = () => [], logger } = options ?? {}

  /** @returns {Array<object>} 当前绑定（异常视为无绑定）。 */
  function bindings() {
    try {
      const list = getBindings()
      return Array.isArray(list) ? list : []
    } catch {
      return []
    }
  }

  /**
   * 执行一次读取/设置/重置。
   * @param {unknown} rawArgs 模型参数 `{minutes?, reset?}`
   * @param {{agent?: object}} [exec] 执行上下文
   * @returns {Promise<object>} `{ok, minutes, intervalMs, sessionId, source}` 或 `{ok:false, reason}`
   */
  async function execute(rawArgs, exec) {
    try {
      const args = rawArgs !== null && typeof rawArgs === 'object' ? rawArgs : {}
      const sessionId = callerSessionId(exec)
      if (!isMainSession(sessionId, bindings())) return { ok: false, reason: 'not_main_session' }
      if (args.reset === true) {
        store.reset(sessionId)
      } else if (args.minutes !== undefined) {
        if (!isValidIntervalMinutes(args.minutes)) return { ok: false, reason: 'invalid_minutes' }
        store.set(sessionId, args.minutes)
      }
      const effective = effectiveIntervalOf(store, defaultIntervalMs, sessionId)
      return { ok: true, ...effective, sessionId }
    } catch (error) {
      try {
        logger?.warnOnce?.('subscription-tool', `astrbox_subscription 执行失败（已忽略）：${error instanceof Error ? error.message : String(error)}`)
      } catch {
        // 日志失败不影响返回值。
      }
      return { ok: false, reason: 'internal_error' }
    }
  }

  const definition = {
    name: SUBSCRIPTION_TOOL_NAME,
    description:
      '读取或设置自己（主会话）的事件黑板订阅推送间隔。无参数读取当前生效值；'
      + 'minutes 为 1..1440 的整数（分钟），设置后持久化；reset=true 清除自定义、回到全局默认。'
      + '只有主会话可以设置/读取，子会话调用会返回 not_main_session。',
    parameters: {
      type: 'object',
      properties: {
        minutes: {
          type: 'integer',
          minimum: MIN_INTERVAL_MINUTES,
          maximum: MAX_INTERVAL_MINUTES,
          description: '订阅推送间隔（分钟，1..1440）。',
        },
        reset: {
          type: 'boolean',
          description: 'true = 清除自定义间隔，回到全局默认。',
        },
      },
      additionalProperties: false,
    },
    output: {
      schema: {
        type: 'object',
        properties: {
          ok: { type: 'boolean' },
          minutes: { type: 'integer' },
          intervalMs: { type: 'integer' },
          sessionId: { type: 'string' },
          source: { type: 'string' },
          reason: { type: 'string' },
        },
        required: ['ok'],
        additionalProperties: false,
      },
      render: (_args, value) => [{ type: 'text', text: renderSubscriptionResult(value) }],
    },
    execute: (args, exec) => execute(args, exec),
  }

  return { definition, execute }
}
