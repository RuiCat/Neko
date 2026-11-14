/**
 * K6 绑定解析纯函数：把 `qqbotBots.bindings()` 的原始数组规范化成 `BotBinding[]`，
 * 再按 agent 会话 cwd 解析生效实体（目录边界安全、最长前缀优先、解析确定）。
 *
 * 本模块不读服务、不缓存：缓存「最近一次非空绑定」由调用方（plugin.mjs）持有。
 */

/**
 * 取非空字符串；空白串与非字符串一律视为非法。
 * @param {unknown} value 原始值
 * @returns {string | undefined} 原字符串（保留首尾空白的原样），非法返回 undefined
 */
function nonEmptyString(value) {
  if (typeof value !== 'string' || value.trim() === '') return undefined
  return value
}

/**
 * 把 cwd 规范成可比较的目录：去掉首尾空白与结尾分隔符，根目录统一记为 `/`。
 * @param {unknown} value 原始 cwd
 * @returns {string | undefined} 规范化目录；非法返回 undefined
 */
export function normalizeCwd(value) {
  const cwd = nonEmptyString(value)
  if (cwd === undefined) return undefined
  const stripped = cwd.trim().replace(/[\\/]+$/, '')
  return stripped === '' ? '/' : stripped
}

/**
 * 规范化单条绑定：`botId`/`cwd` 必填，`entity`/`label`/`mainSessionId` 只在非空时保留。
 * @param {unknown} raw 原始条目
 * @returns {{botId: string, cwd: string, entity?: string, label?: string, mainSessionId?: string} | undefined} 规范化条目
 */
function normalizeBinding(raw) {
  if (typeof raw !== 'object' || raw === null || Array.isArray(raw)) return undefined
  const botId = nonEmptyString(raw.botId)
  const cwd = nonEmptyString(raw.cwd)
  if (botId === undefined || cwd === undefined) return undefined
  const binding = { botId, cwd }
  const entity = nonEmptyString(raw.entity)
  if (entity !== undefined) binding.entity = entity
  const label = nonEmptyString(raw.label)
  if (label !== undefined) binding.label = label
  const mainSessionId = nonEmptyString(raw.mainSessionId)
  if (mainSessionId !== undefined) binding.mainSessionId = mainSessionId
  return binding
}

/**
 * 规范化绑定数组：非法条目丢弃；同 cwd（去尾分隔符后相同）只保留数组序第一条（K6 确定性）。
 * @param {unknown} raw 原始 bindings（任何形状都不抛错）
 * @returns {Array<{botId: string, cwd: string, entity?: string, label?: string, mainSessionId?: string}>} 规范化绑定
 */
export function normalizeBindings(raw) {
  const list = Array.isArray(raw) ? raw : []
  const seenCwd = new Set()
  const bindings = []
  for (const item of list) {
    const binding = normalizeBinding(item)
    if (binding === undefined) continue
    const cwd = normalizeCwd(binding.cwd)
    if (seenCwd.has(cwd)) continue
    seenCwd.add(cwd)
    bindings.push(binding)
  }
  return bindings
}

/**
 * 按目录边界判断 `target` 是否在 `base` 内（`<base>-old` 不算）。
 * @param {string} target 规范化后的目标目录
 * @param {string} base 规范化后的绑定目录
 * @returns {boolean} 是否命中
 */
function isInside(target, base) {
  if (base === '/') return true
  return target === base || target.startsWith(`${base}/`) || target.startsWith(`${base}\\`)
}

/**
 * 解析 cwd 命中的绑定：目录边界安全、最长前缀优先、同长度保留数组序第一条（K6）。
 * @param {ReadonlyArray<object>} bindings 规范化绑定（未规范化的条目会被跳过）
 * @param {unknown} cwd agent 会话工作目录
 * @returns {object | undefined} 命中的绑定（返回入参数组里的原对象）；未命中返回 undefined
 */
export function resolveEntityForCwd(bindings, cwd) {
  const target = normalizeCwd(cwd)
  if (target === undefined) return undefined
  const list = Array.isArray(bindings) ? bindings : []
  let best
  let bestLength = -1
  for (const item of list) {
    if (nonEmptyString(item?.botId) === undefined) continue
    const base = normalizeCwd(item?.cwd)
    if (base === undefined) continue
    if (!isInside(target, base)) continue
    if (base.length > bestLength) {
      best = item
      bestLength = base.length
    }
  }
  return best
}
