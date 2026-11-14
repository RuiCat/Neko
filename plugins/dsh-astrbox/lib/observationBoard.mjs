/**
 * 面板投影：把 `/v2/observations/state` 响应折叠成 `GET /astrbox/blackboard` 的 board 形状。
 *
 * 纯函数、无 I/O：面板只读视图与 v2 数据源的唯一转换点；字段缺失/非法时退化为 null 或空数组。
 */

/** 面板快照条可展示的路径条数上限（防止异常响应把路由响应撑大）。 */
export const DEFAULT_PANEL_PATH_LIMIT = 32

/** @param {unknown} value 任意值 @returns {boolean} 是否为普通对象 */
function isPlainObject(value) {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

/**
 * 取条目里最新（按 `at`）的时间戳文本；无法解析时回落第一个非空 `at`。
 * @param {Array<{updated_at: string}>} rows 投影后的条目
 * @returns {string} 时间戳文本（可能为空串）
 */
function newestUpdatedAt(rows) {
  let newest = ''
  let newestTime = Number.NEGATIVE_INFINITY
  for (const row of rows) {
    const at = row.updated_at
    if (at === '') continue
    if (newest === '') newest = at
    const time = Date.parse(at)
    if (!Number.isNaN(time) && time > newestTime) {
      newestTime = time
      newest = at
    }
  }
  return newest
}

/**
 * 把一条 state 响应投影成面板 board；无可用条目返回 null（面板显示「无黑板」）。
 *
 * board 形如 `{entity, version, updated_at, entries:[{path, from, to, updated_at}]}`：
 * `to` 取自 state 的 `value`，`from` 取自 `from`，`updated_at` 取自条目的 `at`。
 * @param {unknown} entity 实体 id（来自桥接的生效实体，而不是响应体推断）
 * @param {unknown} payload `GET /v2/observations/state` 响应体
 * @returns {{entity: string, version: number, updated_at: string, entries: Array<object>} | null} 面板 board
 */
export function boardFromObservationState(entity, payload) {
  if (typeof entity !== 'string' || entity.trim() === '') return null
  if (!isPlainObject(payload) || payload.ok !== true) return null
  const entries = Array.isArray(payload.entries) ? payload.entries : []
  const rows = []
  for (const entry of entries) {
    if (!isPlainObject(entry)) continue
    const path = typeof entry.path === 'string' ? entry.path.trim() : ''
    if (path === '') continue
    rows.push({
      path,
      from: entry.from === undefined ? null : entry.from,
      to: entry.value === undefined ? null : entry.value,
      updated_at: typeof entry.at === 'string' ? entry.at : '',
    })
  }
  if (rows.length === 0) return null
  return {
    entity: entity.trim(),
    version: Number.isInteger(payload.version) ? payload.version : 0,
    updated_at: newestUpdatedAt(rows),
    entries: rows,
  }
}

/**
 * state 响应里的路径清单（面板快照条元数据）；按服务端顺序、去重、截断到上限。
 * @param {unknown} payload `GET /v2/observations/state` 响应体
 * @param {number} [limit] 条数上限（默认 32）
 * @returns {string[]} 路径列表
 */
export function statePathsOf(payload, limit = DEFAULT_PANEL_PATH_LIMIT) {
  const max = Number.isInteger(limit) && limit >= 1 ? limit : DEFAULT_PANEL_PATH_LIMIT
  const paths = []
  const seen = new Set()
  const entries = isPlainObject(payload) && Array.isArray(payload.entries) ? payload.entries : []
  for (const entry of entries) {
    if (!isPlainObject(entry)) continue
    const path = typeof entry.path === 'string' ? entry.path.trim() : ''
    if (path === '' || seen.has(path)) continue
    seen.add(path)
    paths.push(path)
    if (paths.length >= max) break
  }
  return paths
}
