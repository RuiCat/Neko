/**
 * 轮次摘要：从 DSH Session 的有界事件窗口里提取助手最后一句话、工具调用数与步数。
 *
 * 只读 `snapshotEvents()` 的尾部窗口（上限 SCAN_LIMIT 条），不复制整段日志；
 * 任何形状异常都返回 undefined，绝不向宿主抛错（桥接插件的统一约束）。
 */
/** 反向扫描 Session 事件窗口的上限（条）。 */
export const SCAN_LIMIT = 200

/** 摘要文本的最大字符数。 */
export const DIGEST_MAX_CHARS = 1200

/**
 * 提取一条 assistant/message 的纯文本内容。
 * @param {unknown} message assistant 消息
 * @returns {string} 拼接后的文本，空串表示没有可见文本
 */
function messageText(message) {
  const content = message?.content
  if (!Array.isArray(content)) return ''
  const parts = []
  for (const block of content) {
    if (block?.type === 'text' && typeof block.text === 'string') parts.push(block.text)
  }
  return parts.join('\n').trim()
}

/**
 * 截断摘要，按字符（不切断代理对）。
 * @param {string} text 原始文本
 * @returns {string} 不超过 DIGEST_MAX_CHARS 的文本
 */
function truncate(text) {
  const chars = Array.from(text)
  if (chars.length <= DIGEST_MAX_CHARS) return text
  return chars.slice(0, DIGEST_MAX_CHARS).join('') + '…'
}

/** 判断一条 user/message 是否为注入内容（不是用户本人输入）。 */
function injectedUserText(message, text) {
  const kind = message?.source?.kind
  if (kind === 'runtime-context' || kind === 'astrbox') return true
  return text.startsWith('[context-guard]')
}

/**
 * 汇总一轮的可见结果；没有可用事件时返回 undefined。
 * @param {object|undefined} session Session（带 snapshotEvents）
 * @param {number} turn 轮次
 * @returns {{assistantDigest: string, toolCalls: number, steps: number, userDigest?: string}|undefined} 摘要
 */
export function summarizeTurn(session, turn) {
  if (typeof session?.snapshotEvents !== 'function') return undefined
  let events
  try {
    events = session.snapshotEvents()
  } catch {
    return undefined
  }
  if (!Array.isArray(events) || events.length === 0) return undefined
  const start = Math.max(0, events.length - SCAN_LIMIT)
  // 用户消息的 payload 没有 turn 字段，只能先定位本轮 turn/start 再正向扫描。
  let begin = start
  for (let index = events.length - 1; index >= start; index -= 1) {
    const event = events[index]
    if (event?.type === 'turn/start' && event.data?.turn === turn) {
      begin = index + 1
      break
    }
  }
  let digest = ''
  let userDigest = ''
  let toolCalls = 0
  let steps = 0
  for (let index = begin; index < events.length; index += 1) {
    const event = events[index]
    if (!event || typeof event.type !== 'string') continue
    const data = event.data
    if (event.type === 'turn/start' && data?.turn !== turn) break
    if (event.type === 'turn/end') break
    if (event.type === 'user/message') {
      if (userDigest !== '') continue
      const text = messageText(data)
      if (text === '' || injectedUserText(data, text)) continue
      userDigest = truncate(text)
      continue
    }
    if (data?.turn !== turn) continue
    if (event.type === 'assistant/message') {
      const text = messageText(data.message)
      // 正向扫描：最后一条命中的就是本轮最后一条助手消息。
      if (text !== '') digest = text
    } else if (event.type === 'tool/call') {
      toolCalls += 1
    } else if (event.type === 'step/start') {
      steps += 1
    }
  }
  if (digest === '' && userDigest === '' && toolCalls === 0 && steps === 0) return undefined
  const summary = { assistantDigest: truncate(digest), toolCalls, steps }
  if (userDigest !== '') summary.userDigest = userDigest
  return summary
}

