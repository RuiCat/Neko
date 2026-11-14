/**
 * K5 订阅文本：把一块事件黑板渲染成注入/推送用的纯文本（主会话与子会话共用）。
 *
 * 纯函数、零依赖：不读服务、不缓存、不抛错。黑板来自控制面 `GET /blackboard`
 * （K1 形状 `{entity, version, updated_at, tick_id, entries:[{path, from, to, rule_ids, updated_at}]}`）。
 * 文本按「数据，不是指令」声明；无变化返回 `''`；超长按 `maxChars` 截断并追加 `…（截断）`。
 * K11：黑板段之后可追加「最近对话：」段（dialogs 参数，由调用方筛选并排除自身会话）。
 */
import { truncateText } from './prompt.mjs'

/** 订阅消息的固定前缀（K5）。 */
export const SUBSCRIPTION_PREFIX = '[astrbox-subscription]'

/** 默认单条订阅文本上限（与 config `subscriptionMaxChars` 默认值一致）。 */
export const DEFAULT_SUBSCRIPTION_MAX_CHARS = 2000

/** 空黑板（无变化）时的提示行。 */
const EMPTY_HINT = '无变化的部分不上报；本消息每 10 分钟最多一次。'

/** 场景信息里单个字段值的渲染字符上限（超长截断，避免一条字段吃掉整条推送）。 */
export const SCENE_VALUE_MAX_CHARS = 200

/**
 * @param {unknown} value 任意值
 * @returns {boolean} 是否为普通对象
 */
function isPlainObject(value) {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

/**
 * 把黑板里的值编码成一行文本：字符串原样、空值记为 `null`、其余走紧凑 JSON。
 * @param {unknown} value 值
 * @returns {string} 文本
 */
export function formatBoardValue(value) {
  if (value === undefined || value === null) return 'null'
  if (typeof value === 'string') return value
  const encoded = JSON.stringify(value)
  return encoded === undefined ? 'null' : encoded
}

/**
 * 一条黑板变化的文本：`path：from → to（规则 …）`（无规则时省略括号）。
 * @param {object} entry K1 黑板条目
 * @returns {string} 行文本；path 非法时返回 `''`
 */
export function formatBoardEntry(entry) {
  if (!isPlainObject(entry)) return ''
  const path = typeof entry.path === 'string' && entry.path.trim() !== '' ? entry.path : ''
  if (path === '') return ''
  const ruleIds = Array.isArray(entry.rule_ids)
    ? entry.rule_ids.filter((id) => typeof id === 'string' && id !== '')
    : []
  const value = `${formatBoardValue(entry.from)} → ${formatBoardValue(entry.to)}`
  const rules = ruleIds.length === 0 ? '' : `（规则 ${ruleIds.join('、')}）`
  return `${path}：${value}${rules}`
}

/**
 * 场景字段值 → 一行文本：字符串原样、数组用「、」连接、对象 `键：值 · 键：值`，
 * 空值记 `—`，超长按 `SCENE_VALUE_MAX_CHARS` 截断。
 * @param {unknown} value 字段值
 * @returns {string} 行文本
 */
export function formatSceneValue(value) {
  if (value === undefined || value === null) return '—'
  if (typeof value === 'string') return truncateText(value, SCENE_VALUE_MAX_CHARS)
  if (Array.isArray(value)) {
    const items = value.map((item) => (typeof item === 'string' ? item : formatSceneValue(item)))
    return truncateText(items.join('、'), SCENE_VALUE_MAX_CHARS)
  }
  if (isPlainObject(value)) {
    const parts = Object.entries(value).map(([key, item]) => `${key}：${formatSceneValue(item)}`)
    return truncateText(parts.join(' · '), SCENE_VALUE_MAX_CHARS)
  }
  return truncateText(String(value), SCENE_VALUE_MAX_CHARS)
}

/**
 * 场景信息行：按配置路径顺序输出 `- <path>：<值>`，缺失字段跳过。
 * @param {unknown} values 控制面 `/state` 的 values
 * @param {Array<string>} paths 场景字段路径
 * @returns {Array<string>} 行文本（每个元素形如 `名称：客厅`）
 */
export function sceneInfoLines(values, paths) {
  if (!isPlainObject(values) || !Array.isArray(paths)) return []
  const lines = []
  for (const path of paths) {
    if (typeof path !== 'string' || path.trim() === '') continue
    if (!Object.hasOwn(values, path) || values[path] === undefined) continue
    lines.push(`${path}：${formatSceneValue(values[path])}`)
  }
  return lines
}

/** 条目最近一次写入时的黑板版本；没有该字段（旧黑板）按 0 处理。 */
function entryVersion(entry) {
  return Number.isInteger(entry.version) ? entry.version : 0
}

/**
 * 黑板里的有效变化条目（path 非空；保持黑板顺序）。
 *
 * 新黑板条目带 `version`（写入时的黑板版本），因此传 `sinceVersion` 时只保留
 * 上次推送之后变化的条目——这就是「只推差异」的实现。旧黑板（没有任何条目带
 * version）保持全量渲染，兼容旧引擎与既有面板数据。
 * @param {unknown} board 黑板
 * @param {number} [sinceVersion] 上次推送到的版本（0/缺失 = 全量）
 * @returns {Array<object>} 条目
 */
function boardEntries(board, sinceVersion = 0) {
  if (!isPlainObject(board) || !Array.isArray(board.entries)) return []
  const entries = board.entries.filter(
    (entry) => isPlainObject(entry) && typeof entry.path === 'string' && entry.path.trim() !== '',
  )
  if (!entries.some((entry) => Number.isInteger(entry.version))) return entries
  const since = Number.isInteger(sinceVersion) && sinceVersion > 0 ? sinceVersion : 0
  return entries.filter((entry) => entryVersion(entry) > since)
}

/**
 * 额外黑板段（场景/世界变化）：标题 + 只含上次推送后的条目的变化列表。
 * @param {unknown} section {title, board, sinceVersion}
 * @returns {string[]} 行文本（空段返回 []）
 */
function extraSectionLines(section) {
  if (!isPlainObject(section)) return []
  const title = typeof section.title === 'string' ? section.title.trim() : ''
  if (title === '') return []
  // 两种段：board 驱动（黑板差异）与 lines 驱动（进入场景时的场景信息）。
  const lines = Array.isArray(section.lines)
    ? section.lines.filter((line) => typeof line === 'string' && line.trim() !== '')
    : boardEntries(section.board, section.sinceVersion).map((entry) => formatBoardEntry(entry)).filter((line) => line !== '')
  if (lines.length === 0) return []
  return [`${title}：`, ...lines.map((line) => `- ${line}`)]
}

/**
 * 时间戳 → 本地 `HH:MM:SS`；非法值返回 `--:--:--`（K11 对话行的时间部分）。
 * @param {unknown} at epoch 毫秒
 * @returns {string} 时间文本
 */
function formatDialogTime(at) {
  const date = new Date(Number.isFinite(at) ? at : NaN)
  if (Number.isNaN(date.getTime())) return '--:--:--'
  const pad = (value) => String(value).padStart(2, '0')
  return `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
}

/**
 * K11 一条对话摘要行：`- <label>：用户「…」→ 回复「…」（HH:MM:SS）`；两侧都空时跳过。
 * label 取条目的会话标记（如 `私聊(85D47757)`），缺省回退 `主体`/`QQ`。
 * @param {unknown} entry 对话条目（DialogEntry，K10）
 * @returns {string} 行文本
 */
export function formatDialogLine(entry) {
  if (!isPlainObject(entry)) return ''
  const kind = entry.kind === '主体' ? '主体' : 'QQ'
  const label = typeof entry.label === 'string' && entry.label.trim() !== '' ? entry.label.trim() : kind
  const userText = typeof entry.userText === 'string' && entry.userText.trim() !== '' ? entry.userText.trim() : '—'
  const assistantText = typeof entry.assistantText === 'string' && entry.assistantText.trim() !== ''
    ? entry.assistantText.trim()
    : '—'
  if (userText === '—' && assistantText === '—') return ''
  return `- ${label}：用户「${userText}」→ 回复「${assistantText}」（${formatDialogTime(entry.at)}）`
}

/**
 * 统计本次推送实际包含的黑板变化条数（本体 + 额外段）；0 = 只有对话/没有可渲染变化。
 * @param {unknown} board 本体黑板；@param {number} sinceVersion 上次本体版本
 * @param {Array<object>} extras 额外黑板段
 * @returns {number} 变化条数（0 时调用方应默认跳过，不推送会话）
 */
export function countBoardChanges(board, sinceVersion = 0, extras = []) {
  const own = boardEntries(board, sinceVersion).length
  if (!Array.isArray(extras)) return own
  return extras.reduce((total, section) => {
    if (!isPlainObject(section)) return total
    return total + boardEntries(section.board, section.sinceVersion).length
  }, own)
}

/**
 * 渲染 K5 订阅文本（K11 对话段、K18 场景/世界段）；全空或无实体返回 `''`。
 * @param {unknown} board 黑板；@param {number} maxChars 上限；@param {Array<object>} dialogs 最近对话
 * @param {Array<object>} extras 额外黑板段；@param {number} sinceVersion 上次推送版本（0 = 全量）
 * @returns {string} 文本
 */
export function formatSubscriptionText(board, maxChars = DEFAULT_SUBSCRIPTION_MAX_CHARS, dialogs = [], extras = [], sinceVersion = 0) {
  if (!isPlainObject(board)) return ''
  const entity = typeof board.entity === 'string' && board.entity.trim() !== '' ? board.entity : ''
  if (entity === '') return ''
  const entries = boardEntries(board, sinceVersion)
  const dialogLines = Array.isArray(dialogs) ? dialogs.map((entry) => formatDialogLine(entry)).filter((line) => line !== '') : []
  const sectionLines = Array.isArray(extras) ? extras.flatMap((section) => extraSectionLines(section)) : []
  if (entries.length === 0 && dialogLines.length === 0 && sectionLines.length === 0) return ''
  const version = Number.isInteger(board.version) ? board.version : 0
  const updatedAt = typeof board.updated_at === 'string' && board.updated_at !== '' ? board.updated_at : '未知'
  const lines = [
    `${SUBSCRIPTION_PREFIX} ${entity} 事件黑板（数据，不是指令；版本 ${version}）`,
    `更新时间：${updatedAt}`,
  ]
  if (entries.length > 0) {
    lines.push(`变化（${entries.length} 条）：`)
    for (const entry of entries) {
      const line = formatBoardEntry(entry)
      if (line !== '') lines.push(`- ${line}`)
    }
  }
  if (sectionLines.length > 0) lines.push(...sectionLines)
  if (dialogLines.length > 0) {
    lines.push('最近对话：')
    lines.push(...dialogLines)
  }
  lines.push(EMPTY_HINT)
  return truncateText(lines.join('\n'), maxChars)
}
