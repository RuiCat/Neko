/**
 * Observation v2 客户端渲染（契约 §4）：把控制面 `/v2/observations` 响应渲染成稳定文本。
 *
 * 纯函数、无 I/O、无状态：同一 `payload` + 同一 `options` 逐字节相同；不含时间戳或随机量。
 * 过滤/合并/排序/预算判定都以服务端返回为准，本模块只做「格式化 + 按字符上限裁行」。
 */

/** 变化推送的默认字符上限（§4.2：options.maxChars → payload.budget.maxChars → 1200）。 */
export const DEFAULT_DELTA_MAX_CHARS = 1200

/** 单个值格式化后的默认字符上限（§4.1）。 */
export const DEFAULT_VALUE_MAX_CHARS = 200

/** 现状卡的默认条目上限（§4.3）。 */
export const DEFAULT_STATE_MAX_ITEMS = 8

/** 现状卡的默认字符上限（§4.3）。 */
export const DEFAULT_STATE_MAX_CHARS = 1200

/** 变化推送的头行前缀（§4.2）。 */
export const DELTA_PREFIX = '[astrbox-observation]'

/** 现状卡的头行前缀（§4.3）。 */
export const STATE_PREFIX = '[astrbox-state]'

/** 变化推送的正文行前缀。 */
const DELTA_LINE_PREFIX = '- '

/** 实体 scope 前缀（§1.2：`entity:<id>`）。 */
const ENTITY_SCOPE_PREFIX = 'entity:'

/** @param {unknown} value 任意值 @returns {boolean} 是否为普通对象 */
function isPlainObject(value) {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

/**
 * 按字符上限截断并追加 `…`（不切断代理对）。
 * @param {string} text 原文
 * @param {number} maxChars 字符上限（整数 ≥1）
 * @returns {string} 截断后的文本
 */
function truncateWithEllipsis(text, maxChars) {
  if (text.length <= maxChars) return text
  if (maxChars <= 1) return '…'.slice(0, maxChars)
  let head = text.slice(0, maxChars - 1)
  if (/[\uD800-\uDBFF]$/.test(head)) head = head.slice(0, -1)
  return `${head}…`
}

/**
 * 从响应体取实体 id（`scope` 的 `entity:` 之后）；取不到返回空串。
 * @param {unknown} payload 响应体
 * @returns {string} 实体 id
 */
function entityOf(payload) {
  const scope = isPlainObject(payload) && typeof payload.scope === 'string' ? payload.scope : ''
  const entity = scope.startsWith(ENTITY_SCOPE_PREFIX) ? scope.slice(ENTITY_SCOPE_PREFIX.length) : scope
  return entity.trim()
}

/**
 * 取 `options.maxChars`，再回落 `payload.budget.maxChars`，再回落默认值（§4.2）。
 * @param {unknown} options 渲染选项
 * @param {unknown} payload 响应体
 * @returns {number} 字符上限
 */
function deltaMaxCharsOf(options, payload) {
  const fromOptions = isPlainObject(options) ? options.maxChars : undefined
  if (Number.isInteger(fromOptions) && fromOptions >= 1) return fromOptions
  const fromBudget = isPlainObject(payload) && isPlainObject(payload.budget) ? payload.budget.maxChars : undefined
  if (Number.isInteger(fromBudget) && fromBudget >= 1) return fromBudget
  return DEFAULT_DELTA_MAX_CHARS
}

/**
 * 把任意 JSON 值格式化成一行文本（§4.1）。
 * - `null` / `undefined` → `null`（调用方据此决定省略/占位）
 * - 字符串原样；布尔 → `真` / `假`；数字不带多余小数
 * - 对象/数组 → 紧凑 JSON
 * - 超过 `maxChars` 截断并追加 `…`
 * @param {unknown} value 值
 * @param {number} [maxChars] 字符上限（默认 200）
 * @returns {string | null} 文本；空值为 null
 */
export function formatValue(value, maxChars = DEFAULT_VALUE_MAX_CHARS) {
  if (value === null || value === undefined) return null
  const limit = Number.isInteger(maxChars) && maxChars >= 1 ? maxChars : DEFAULT_VALUE_MAX_CHARS
  if (typeof value === 'string') return truncateWithEllipsis(value, limit)
  if (typeof value === 'boolean') return value ? '真' : '假'
  if (typeof value === 'number') return String(value)
  let encoded
  try {
    encoded = JSON.stringify(value)
  } catch {
    encoded = undefined
  }
  return truncateWithEllipsis(encoded === undefined ? 'null' : encoded, limit)
}

/**
 * 一条变化行：`- <path>: <from> → <to>`；`from` 为 null/缺失时只写当前值（§4.2）。
 * @param {unknown} event ObservationEvent
 * @returns {string} 行文本；path 非法时返回空串
 */
function deltaLine(event) {
  if (!isPlainObject(event)) return ''
  const path = typeof event.path === 'string' ? event.path.trim() : ''
  if (path === '') return ''
  const to = formatValue(event.to)
  const toText = to === null ? 'null' : to
  const hasFrom = Object.hasOwn(event, 'from') && event.from !== null && event.from !== undefined
  if (!hasFrom) return `${DELTA_LINE_PREFIX}${path}: ${toText}`
  const from = formatValue(event.from)
  return `${DELTA_LINE_PREFIX}${path}: ${from === null ? 'null' : from} → ${toText}`
}

/**
 * 按字符预算从尾部丢行，超限时追加 `另有 <N> 条未显示`；正文行保持原顺序。
 * @param {string} header 头行
 * @param {string[]} lines 正文行
 * @param {number} maxChars 字符上限
 * @param {number} baseHidden 进入字符预算前已隐藏的条数
 * @returns {string} 文本（总长 ≤ maxChars，除非连头行都放不下）
 */
function fitBlock(header, lines, maxChars, baseHidden = 0) {
  const full = [header, ...lines].join('\n')
  if (baseHidden === 0 && full.length <= maxChars) return full
  for (let keep = lines.length; keep >= 0; keep -= 1) {
    const hidden = baseHidden + (lines.length - keep)
    const parts = [header, ...lines.slice(0, keep)]
    if (hidden > 0) parts.push(`另有 ${hidden} 条未显示`)
    const candidate = parts.join('\n')
    if (candidate.length <= maxChars) return candidate
  }
  return truncateWithEllipsis([header, `另有 ${baseHidden + lines.length} 条未显示`].join('\n'), maxChars)
}

/**
 * 渲染变化推送（§4.2）。
 *
 * 头行 `[astrbox-observation] <entity> · <N> 处变化`，N = 本次可推变化总数（含未显示）；
 * `events` 为空返回空串（调用方不推送）。超字符预算时从尾部（normal 段）丢行。
 * @param {unknown} payload `GET /v2/observations` 响应体
 * @param {{maxChars?: number}} [options] 渲染选项
 * @returns {string} 推送文本；无可推变化时为空串
 */
export function renderDelta(payload, options = {}) {
  if (!isPlainObject(payload)) return ''
  const entity = entityOf(payload)
  if (entity === '') return ''
  const events = Array.isArray(payload.events) ? payload.events.filter(isPlainObject).filter((event) => deltaLine(event) !== '') : []
  if (events.length === 0) return ''
  const header = `${DELTA_PREFIX} ${entity} · ${events.length} 处变化`
  const lines = events.map((event) => deltaLine(event))
  return fitBlock(header, lines, deltaMaxCharsOf(options, payload))
}

/**
 * 一条现状行：`<path>: <value>`（§4.3 无行首 `-`）。
 * @param {unknown} entry state 条目
 * @returns {string} 行文本；path 非法时返回空串
 */
function stateLine(entry) {
  if (!isPlainObject(entry)) return ''
  const path = typeof entry.path === 'string' ? entry.path.trim() : ''
  if (path === '') return ''
  const value = formatValue(entry.value)
  return `${path}: ${value === null ? 'null' : value}`
}

/**
 * 渲染现状卡（§4.3）：`[astrbox-state] <entity>` + 每 path 一行当前值。
 *
 * `maxItems` 缺省 8：超出条目上限的部分计入 `另有 <N> 条未显示`；`maxChars` 缺省 1200。
 * `entries` 为空返回空串（调用方不注入）。
 * @param {unknown} payload `GET /v2/observations/state` 响应体
 * @param {{maxItems?: number, maxChars?: number}} [options] 渲染选项
 * @returns {string} 现状卡文本；无可用条目时为空串
 */
export function renderState(payload, options = {}) {
  if (!isPlainObject(payload)) return ''
  const entity = entityOf(payload)
  if (entity === '') return ''
  const entries = Array.isArray(payload.entries) ? payload.entries.filter((entry) => stateLine(entry) !== '') : []
  if (entries.length === 0) return ''
  const optionsObject = isPlainObject(options) ? options : {}
  const maxItems = Number.isInteger(optionsObject.maxItems) && optionsObject.maxItems >= 1
    ? optionsObject.maxItems
    : DEFAULT_STATE_MAX_ITEMS
  const maxChars = Number.isInteger(optionsObject.maxChars) && optionsObject.maxChars >= 1
    ? optionsObject.maxChars
    : DEFAULT_STATE_MAX_CHARS
  const shown = entries.slice(0, maxItems)
  const lines = shown.map((entry) => stateLine(entry))
  return fitBlock(`${STATE_PREFIX} ${entity}`, lines, maxChars, entries.length - shown.length)
}
