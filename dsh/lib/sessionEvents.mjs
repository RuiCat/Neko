/**
 * 会话事件同步：`session/created` 与 `agent/turn-stopping` → `POST /session-event`。
 *
 * C2 硬约束：
 * - `cwd` 按目录边界匹配 `workspacePrefix`（`<prefix>-old` 不命中）；
 * - 同一 `(kind, sessionId, turn)` 只发一次，去重表按会话上限 256；
 * - 监听器整体 try/catch，绝不向宿主同步抛错；发送是 fire-and-forget。
 */
export const DEDUPE_LIMIT = 256

/**
 * 按目录边界判断 `cwd` 是否在 `prefix` 内（不把 `<prefix>-old` 当成命中）。
 * @param {unknown} cwd 会话工作目录
 * @param {unknown} prefix 配置的目录前缀
 * @returns {boolean} 是否命中
 */
export function matchesWorkspace(cwd, prefix) {
  if (typeof cwd !== 'string' || cwd === '') return false
  if (typeof prefix !== 'string' || prefix === '') return false
  const base = prefix.replace(/[\\/]+$/, '')
  if (base === '') return true
  return cwd === base || cwd.startsWith(`${base}/`) || cwd.startsWith(`${base}\\`)
}

/**
 * 建会话事件发送器。
 * @param {{client: object, config: object, logger: object, isActive?: () => boolean}} options 依赖
 * @returns {{sessionCreated: Function, turnStopping: Function, dispose: Function, dedupeSize: Function}} 发送器
 */
export function createSessionEventSender(options) {
  const { client, config, logger, isActive = () => true } = options
  const seen = new Map()
  let disposed = false

  /** 记录并判断该事件是否已发送过；超上限时淘汰最早的一条。 */
  function alreadySent(kind, sessionId, turn) {
    let table = seen.get(sessionId)
    if (table === undefined) {
      table = new Set()
      seen.set(sessionId, table)
    }
    const key = `${kind}:${turn ?? ''}`
    if (table.has(key)) return true
    table.add(key)
    while (table.size > DEDUPE_LIMIT) {
      const oldest = table.values().next().value
      if (oldest === undefined) break
      table.delete(oldest)
    }
    return false
  }

  /** 记一条失败日志；logger 缺失时静默（真实运行时一定有 logger）。 */
  function warn(message) {
    if (typeof logger?.warnOnce === 'function') logger.warnOnce('session-event', message)
  }

  /** fire-and-forget 发送；同步异常与异步拒绝都在此收口。 */
  function send(event) {
    if (disposed || !isActive()) return
    try {
      // K1 新增可选字段：指定服务端调用的函数名（缺省 dsh_on_event），让 K2 的 functionName 生效。
      const pending = client.postSessionEvent({ ...event, function: config.functionName })
      Promise.resolve(pending).catch((error) => {
        warn(`控制面 /session-event 失败（只记日志）：${error instanceof Error ? error.message : String(error)}`)
      })
    } catch (error) {
      warn(`控制面 /session-event 同步失败（只记日志）：${error instanceof Error ? error.message : String(error)}`)
    }
  }

  return {
    /** 处理 `session/created`：只上报 cwd 命中前缀的会话。 */
    sessionCreated(session) {
      try {
        const cwd = session?.header?.cwd
        if (!matchesWorkspace(cwd, config.workspacePrefix)) return
        const sessionId = session?.id
        if (typeof sessionId !== 'string' || sessionId === '') return
        if (alreadySent('session-created', sessionId, undefined)) return
        send({ kind: 'session-created', sessionId, cwd })
      } catch (error) {
        warn(`session/created 处理失败（已忽略）：${error instanceof Error ? error.message : String(error)}`)
      }
    },

    /** 处理 `agent/turn-stopping`：上报 turn-end，turn 必须是整数。 */
    turnStopping(payload) {
      try {
        const session = payload?.agent?.session
        const cwd = session?.header?.cwd
        const turn = payload?.turn
        if (!Number.isInteger(turn)) return
        if (!matchesWorkspace(cwd, config.workspacePrefix)) return
        const sessionId = session?.id
        if (typeof sessionId !== 'string' || sessionId === '') return
        if (alreadySent('turn-end', sessionId, turn)) return
        send({ kind: 'turn-end', sessionId, cwd, turn })
      } catch (error) {
        warn(`agent/turn-stopping 处理失败（已忽略）：${error instanceof Error ? error.message : String(error)}`)
      }
    },

    /** 卸载清理：不再发送，清空去重表。 */
    dispose() {
      disposed = true
      seen.clear()
    },

    /** @param {string} sessionId 会话 id @returns {number} 该会话去重表大小（测试用） */
    dedupeSize(sessionId) {
      return seen.get(sessionId)?.size ?? 0
    },
  }
}
