/**
 * K10 对话摘要 Feed：从一轮 Session 事件里抽取 user/assistant 摘要，按实体保留最近 N 条。
 *
 * 纯内存、不落盘（重启清空，K10）；采集是只读扫描 `snapshotEvents()` 的尾部窗口，
 * 任何形状异常都返回 undefined，绝不向宿主抛错（桥接插件统一约束）。
 * `feedVersion` 每新增一条 +1，供 K12/K13 的推送/注入门控使用。
 */

/** 反向扫描 Session 事件窗口的上限（条），与 turnSummary 同口径。 */
export const DIALOG_SCAN_LIMIT = 200

/** 默认每条 user/assistant 摘要的字符上限（K10：dialogExcerptChars）。 */
export const DEFAULT_DIALOG_EXCERPT_CHARS = 160

/** 默认每实体保留的最近对话条数（K10：dialogFeedSize）。 */
export const DEFAULT_DIALOG_FEED_SIZE = 20

/** 桥接自己注入的文本前缀：这些 user 消息不算「用户说的话」（避免订阅/转达回灌）。 */
const INJECTED_PREFIXES = Object.freeze(['[astrbox]', '[astrbox-subscription]', '[astrbox-relay]', '[context-guard]'])

/**
 * 取一条消息的纯文本（text 块拼接；空白视为无文本）。
 * @param {unknown} message user/assistant 消息
 * @returns {string} 文本
 */
export function dialogMessageText(message) {
  const content = message?.content
  if (!Array.isArray(content)) return ''
  const parts = []
  for (const block of content) {
    if (block !== null && typeof block === 'object' && block.type === 'text' && typeof block.text === 'string') parts.push(block.text)
  }
  return parts.join('\n').trim()
}

/**
 * 按字符（码点）截断摘要，超限追加 `…`。
 * @param {string} text 原文
 * @param {number} maxChars 字符上限（整数 ≥1）
 * @returns {string} 截断后的文本
 */
export function truncateDialogExcerpt(text, maxChars) {
  const limit = Number.isInteger(maxChars) && maxChars >= 1 ? maxChars : DEFAULT_DIALOG_EXCERPT_CHARS
  const chars = Array.from(text)
  if (chars.length <= limit) return text
  return `${chars.slice(0, limit).join('')}…`
}

/**
 * 是否为本插件注入的 user 文本（K10 只收用户本人说的话）。
 * @param {string} text 文本
 * @returns {boolean} 是否跳过
 */
function isInjectedText(text) {
  return INJECTED_PREFIXES.some((prefix) => text.startsWith(prefix))
}

/**
 * 从一轮事件窗口取最后一条真实用户文本与最后一条助手文本（各按 excerptChars 截断）。
 * 没有真实用户文本（只有助手/注入文本）的轮次返回 undefined，避免订阅/relay 触发的 NPC 回复回灌。
 * @param {object|undefined} session 带 snapshotEvents 的 Session；turn 轮次；excerptChars 上限（默认 160）
 * @returns {{userText: string, assistantText: string, seq: number}|undefined} 摘要（无真实用户文本时 undefined）
 */
export function turnDialogExcerpt(session, turn, excerptChars = DEFAULT_DIALOG_EXCERPT_CHARS) {
  if (typeof session?.snapshotEvents !== 'function') return undefined
  if (!Number.isInteger(turn)) return undefined
  let events
  try {
    events = session.snapshotEvents()
  } catch {
    return undefined
  }
  if (!Array.isArray(events) || events.length === 0) return undefined
  const start = Math.max(0, events.length - DIALOG_SCAN_LIMIT)
  // user/message 的 payload 没有 turn 字段：先定位本轮 turn/start，再正向扫描到本轮结束。
  let begin = start
  for (let index = events.length - 1; index >= start; index -= 1) {
    const event = events[index]
    if (event?.type === 'turn/start' && event.data?.turn === turn) {
      begin = index + 1
      break
    }
  }
  let userText = ''
  let assistantText = ''
  let seq = 0
  for (let index = begin; index < events.length; index += 1) {
    const event = events[index]
    if (event === null || typeof event !== 'object' || typeof event.type !== 'string') continue
    if (event.type === 'turn/start' && event.data?.turn !== turn) break
    if (event.type === 'turn/end') break
    if (event.type === 'user/message') {
      const message = event.data
      if (message?.source?.kind !== 'user') continue
      const text = dialogMessageText(message)
      if (text === '' || isInjectedText(text)) continue
      userText = text
      seq = typeof event.seq === 'number' ? event.seq : seq
      continue
    }
    if (event.type === 'assistant/message') {
      const message = event.data?.message ?? event.data
      const text = dialogMessageText(message)
      if (text === '') continue
      assistantText = text
      seq = typeof event.seq === 'number' ? event.seq : seq
    }
  }
  // K10 修复：只有真实用户文本的轮次才生成摘要；assistant-only（订阅/relay 触发的 NPC 回复）
  // 不记 entry，否则「最近对话」会把自己触发的回复反复带回并再次推送。
  if (userText === '') return undefined
  return {
    userText: truncateDialogExcerpt(userText, excerptChars),
    assistantText: truncateDialogExcerpt(assistantText, excerptChars),
    seq,
  }
}

/**
 * K10 对话归属：会话 id 命中该实体的主会话 → `主体`，否则 `QQ`。
 * @param {string} sessionId 会话 id
 * @param {Set<string>|string[]|undefined} mainSessionIds 该实体的主会话 id 集合
 * @returns {'主体'|'QQ'} 归属标签
 */
export function dialogKind(sessionId, mainSessionIds) {
  if (typeof sessionId !== 'string' || sessionId === '') return 'QQ'
  const ids = mainSessionIds instanceof Set ? mainSessionIds : Array.isArray(mainSessionIds) ? mainSessionIds : []
  const main = ids instanceof Set ? ids.has(sessionId) : ids.includes(sessionId)
  return main ? '主体' : 'QQ'
}

/**
 * 建每实体对话摘要 Feed（K10）：内存态、按实体独立、版本单调递增。
 * @param {{maxEntries?: number, excerptChars?: number, now?: () => number}} [options]
 *   `maxEntries` = dialogFeedSize；`excerptChars` 仅用于截断（record 前由调用方截断）；`now` 仅测试用
 * @returns {{record: Function, version: Function, dialogs: Function, size: Function, clear: Function}} Feed
 */
export function createDialogFeed(options = {}) {
  const { maxEntries, excerptChars, now = Date.now } = options
  const limit = Number.isInteger(maxEntries) && maxEntries >= 1 ? maxEntries : DEFAULT_DIALOG_FEED_SIZE
  const excerpt = Number.isInteger(excerptChars) && excerptChars >= 1 ? excerptChars : DEFAULT_DIALOG_EXCERPT_CHARS
  const feeds = new Map()

  /**
   * @param {string} entity 实体 id
   * @returns {{entries: Array<object>, versions: number[], version: number}} 该实体的 Feed
   */
  function feedOf(entity) {
    let feed = feeds.get(entity)
    if (feed === undefined) {
      feed = { entries: [], versions: [], version: 0 }
      feeds.set(entity, feed)
    }
    return feed
  }

  return {
    /**
     * 记一条对话摘要；无真实用户文本/非法实体或会话不记。`label` 缺省按 kind 回退 主体/QQ。
     * @param {string} entity 实体 id
     * @param {{sessionId: string, kind: '主体'|'QQ', label?: unknown, userText?: unknown,
     *   assistantText?: unknown, at?: unknown, seq?: unknown}} entry 条目
     * @returns {boolean} 是否记录
     */
    record(entity, entry) {
      if (typeof entity !== 'string' || entity === '') return false
      if (entry === null || typeof entry !== 'object') return false
      const sessionId = entry.sessionId
      if (typeof sessionId !== 'string' || sessionId === '') return false
      const kind = entry.kind === '主体' ? '主体' : 'QQ'
      const label = typeof entry.label === 'string' && entry.label.trim() !== '' ? entry.label.trim() : kind
      const userText = typeof entry.userText === 'string' ? truncateDialogExcerpt(entry.userText.trim(), excerpt) : ''
      const assistantText = typeof entry.assistantText === 'string' ? truncateDialogExcerpt(entry.assistantText.trim(), excerpt) : ''
      if (userText === '') return false
      const feed = feedOf(entity)
      feed.entries.push({
        sessionId,
        label,
        kind,
        userText,
        assistantText,
        at: Number.isFinite(entry.at) ? entry.at : now(),
        seq: Number.isInteger(entry.seq) ? entry.seq : 0,
      })
      feed.versions.push(feed.version + 1)
      while (feed.entries.length > limit) {
        feed.entries.shift()
        feed.versions.shift()
      }
      feed.version += 1
      return true
    },

    /**
     * @param {string} entity 实体 id
     * @returns {number} 该实体 Feed 版本（每条 +1；0 = 从未记录）
     */
    version(entity) {
      return typeof entity === 'string' ? feeds.get(entity)?.version ?? 0 : 0
    },

    /**
     * 最近对话（按时间正序，最早的在前）；可排除某会话并只取最近 limit 条。
     * @param {string} entity 实体 id
     * @param {{excludeSessionId?: string, limit?: number}} [query] 过滤
     * @returns {Array<object>} 条目副本
     */
    dialogs(entity, query = {}) {
      const entries = typeof entity === 'string' ? feeds.get(entity)?.entries ?? [] : []
      const exclude = typeof query.excludeSessionId === 'string' && query.excludeSessionId !== '' ? query.excludeSessionId : undefined
      const picked = exclude === undefined ? entries.slice() : entries.filter((entry) => entry.sessionId !== exclude)
      if (Number.isInteger(query.limit) && query.limit >= 0) return picked.slice(-query.limit)
      return picked
    },

    /**
     * K12 附带段：只取 `feedVersion > sinceVersion` 的新增对话（已被上限淘汰的不返回）。
     * @param {string} entity 实体 id
     * @param {number} sinceVersion 上次已推送的 feedVersion
     * @returns {Array<object>} 新增条目（按时间正序）
     */
    dialogsSince(entity, sinceVersion) {
      const feed = typeof entity === 'string' ? feeds.get(entity) : undefined
      if (feed === undefined) return []
      const since = Number.isInteger(sinceVersion) ? sinceVersion : 0
      const picked = []
      for (let index = 0; index < feed.entries.length; index += 1) {
        if ((feed.versions[index] ?? 0) > since) picked.push(feed.entries[index])
      }
      return picked
    },

    /**
     * @param {string} entity 实体 id
     * @returns {number} 该实体当前保留的条目数
     */
    size(entity) {
      return typeof entity === 'string' ? feeds.get(entity)?.entries.length ?? 0 : 0
    },

    /** 清空全部 Feed（dispose 用）。 */
    clear() {
      feeds.clear()
    },
  }
}
