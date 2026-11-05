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

/**
 * 汇总一轮的可见结果；没有可用事件时返回 undefined。
 * @param {object|undefined} session Session（带 snapshotEvents）
 * @param {number} turn 轮次
 * @returns {{assistantDigest: string, toolCalls: number, steps: number}|undefined} 摘要
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
  let digest = ''
  let userDigest = ''
  let toolCalls = 0
  let steps = 0
  for (let index = events.length - 1; index >= start; index -= 1) {
    const event = events[index]
    if (!event || typeof event.type !== 'string') continue
    const data = event.data
    if (event.type === 'turn/start' && data?.turn === turn) break
    // `user/message` 的 payload 就是消息本身，没有 turn 字段；反向扫描停在本轮
    // turn/start 处，所以此前遇到的用户消息都属于本轮。
    if (event.type === 'user/message') {
      if (userDigest === '') {
        const text = messageText(data)
        if (text !== '') userDigest = text
      }
      continue
    }
    if (data?.turn !== turn) continue
    if (event.type === 'assistant/message') {
      const text = messageText(data.message)
      // 反向扫描：第一条命中的就是本轮最后一条助手消息。
      if (digest === '' && text !== '') digest = text
    } else if (event.type === 'user/message') {
      const text = messageText(data.message)
      if (userDigest === '' && text !== '') userDigest = text
    } else if (event.type === 'tool/call') {
      toolCalls += 1
    } else if (event.type === 'step/start') {
      steps += 1
    }
  }
  if (digest === '' && userDigest === '' && toolCalls === 0 && steps === 0) return undefined
  const summary = { assistantDigest: truncate(digest), toolCalls, steps }
  if (userDigest !== '') summary.userDigest = truncate(userDigest)
  return summary
}
