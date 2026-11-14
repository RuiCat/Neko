/**
 * 限频日志：控制面失败只记一条，`warningAfterMs` 内不重复（K2 错误语义 / C16）。
 *
 * 优先走宿主 `ctx.logger('astrbox-bridge')`；宿主没有日志服务时退回 console。
 * 任何日志失败都不得影响宿主流程。
 */
const LOGGER_NAME = 'astrbox-bridge'

/**
 * 建一个限频 logger。
 * @param {{ctx?: object, warningAfterMs?: number, now?: () => number, sink?: (level: string, line: string) => void}} [options]
 * @returns {{info: Function, warn: Function, debug: Function, warnOnce: Function}} logger
 */
export function createLogger(options = {}) {
  const { ctx, warningAfterMs = 30000, now = Date.now, sink } = options
  const lastWarnAt = new Map()

  /** 真正写出一条日志。 */
  function emit(level, message) {
    const line = `[${LOGGER_NAME}] ${message}`
    try {
      if (typeof sink === 'function') {
        sink(level, line)
        return
      }
      const logger = typeof ctx?.logger === 'function' ? ctx.logger(LOGGER_NAME) : undefined
      const method = logger?.[level]
      if (typeof method === 'function') {
        method.call(logger, message)
        return
      }
      const fallback = level === 'warn' ? console.warn : level === 'error' ? console.error : console.info
      fallback(line)
    } catch {
      // 日志失败（例如宿主 logger 已 dispose）不得冒泡到宿主。
    }
  }

  /** 同一 key 在 warningAfterMs 内只警告一次。 */
  function warnOnce(key, message) {
    const at = now()
    const previous = lastWarnAt.get(key)
    if (previous !== undefined && at - previous < warningAfterMs) return
    lastWarnAt.set(key, at)
    emit('warn', message)
  }

  return {
    info: (message) => emit('info', message),
    warn: (message) => emit('warn', message),
    debug: (message) => emit('debug', message),
    warnOnce,
  }
}
