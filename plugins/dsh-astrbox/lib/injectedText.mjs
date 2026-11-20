/**
 * 注入消息识别：桥接自己推给会话的 user 消息不是「用户说的话」，也不是「用户输入」。
 *
 * 轮次摘要（turnSummary）和最近对话（dialogFeed）共用这一张前缀表，避免两处口径漂移：
 * 漏一个前缀，补偿包/现状卡就会被当成主人输入回灌成记忆，再被下一轮召回。
 */

/** 桥接注入文本的前缀（`[astrbox-compensation]` 漏过就会自我回灌）。 */
export const INJECTED_PREFIXES = Object.freeze([
  '[astrbox-observation]',
  '[astrbox-state]',
  '[astrbox-compensation]',
  '[astrbox-board]',
  '[astrbox-relay]',
  '[context-guard]',
])

/**
 * 判断一条 user 消息是否为注入内容。
 *
 * 先看消息来源标记（宿主注入是 runtime-context；桥接自己的注入标记为 astrbox），
 * 再按文本前缀兜底——`createInjectedMessage` 把 source.kind 设成 'user'，前缀是唯一可靠的判据。
 * @param {unknown} message user/message 的消息体（可缺）
 * @param {string} text 消息纯文本
 * @returns {boolean} 是否注入内容
 */
export function isInjectedUserText(message, text) {
  const kind = message?.source?.kind
  if (kind === 'runtime-context' || kind === 'astrbox') return true
  if (typeof text !== 'string' || text === '') return false
  return INJECTED_PREFIXES.some((prefix) => text.startsWith(prefix))
}
