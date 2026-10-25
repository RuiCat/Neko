/**
 * astrbox-bridge 配置规范化与手写校验（C11/C15）。
 *
 * 零依赖、不引入 schemastery：未知键、类型错、负数、路径无分隔符一律抛错并指名键；
 * 合法配置补齐 K2 冻结的默认值。默认值只在此处维护。
 */
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

/** 仓库根目录：本文件位于 <repo>/dsh/lib/config.mjs。 */
export const REPO_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..')

/** 默认 8 条白名单路径（C12；均不含 R18 字段）。 */
export const DEFAULT_PATHS = Object.freeze([
  '状态.情绪',
  '身体.体力',
  '心情.当前',
  '关系.亲密',
  '位置.场景',
  '睡眠.疲劳',
  '社交.互动需求',
  '任务.当前任务',
])

/** K2 冻结的默认配置。 */
export const DEFAULT_CONFIG = Object.freeze({
  socketPath: resolve(REPO_ROOT, 'data', 'dsh-control.sock'),
  entity: '猫猫',
  paths: DEFAULT_PATHS,
  promptMaxChars: 1200,
  promptRefreshMs: 5000,
  tickEveryMs: 60000,
  tickScope: '',
  workspacePrefix: REPO_ROOT,
  functionName: 'dsh_on_event',
  requestTimeoutMs: 2000,
  tickTimeoutMs: 15000,
  warningAfterMs: 30000,
})

/** 整数键 → 允许的最小值（0 表示允许 0，如 tickEveryMs=0 关闭定时器）。 */
const INTEGER_KEYS = Object.freeze({
  promptMaxChars: 1,
  promptRefreshMs: 0,
  tickEveryMs: 0,
  requestTimeoutMs: 1,
  tickTimeoutMs: 1,
  warningAfterMs: 0,
})

/** 必须为非空字符串的键（tickScope 允许空串，单独处理）。 */
const NON_EMPTY_STRING_KEYS = Object.freeze(['entity', 'functionName'])

/** 必须是绝对或相对路径且含分隔符的键。 */
const PATH_KEYS = Object.freeze(['socketPath', 'workspacePrefix'])

const KNOWN_KEYS = new Set(Object.keys(DEFAULT_CONFIG))

/**
 * 抛出一条指名键的配置错误。
 * @param {string} key 配置键（paths 元素写成 `paths[0]`）
 * @param {string} reason 原因
 */
function invalid(key, reason) {
  throw new Error(`astrbox-bridge: invalid config key ${key}: ${reason}`)
}

/** @param {unknown} value @returns {boolean} 是否为普通对象 */
function isPlainObject(value) {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

/** @param {unknown} value @returns {string} 供错误信息使用的类型描述 */
function describeValue(value) {
  if (typeof value === 'string') return `string ${JSON.stringify(value)}`
  if (typeof value === 'number' || typeof value === 'boolean') return `${typeof value} ${String(value)}`
  if (value === null) return 'null'
  if (Array.isArray(value)) return 'array'
  return typeof value
}

/** @param {string} key @param {unknown} value @returns {string} 校验通过的路径 */
function checkPath(key, value) {
  if (typeof value !== 'string' || value.trim() === '') {
    invalid(key, `expected a non-empty path string, got ${describeValue(value)}`)
  }
  if (!/[/\\]/.test(value)) invalid(key, 'must contain a path separator')
  return value
}

/** @param {string} key @param {unknown} value @returns {string} 校验通过的非空字符串 */
function checkNonEmptyString(key, value) {
  if (typeof value !== 'string' || value.trim() === '') {
    invalid(key, `expected a non-empty string, got ${describeValue(value)}`)
  }
  return value
}

/** @param {string} key @param {unknown} value @param {number} min @returns {number} 校验通过的整数 */
function checkInteger(key, value, min) {
  if (typeof value !== 'number' || !Number.isInteger(value)) {
    invalid(key, `expected an integer, got ${describeValue(value)}`)
  }
  if (value < min) invalid(key, `must be >= ${min}, got ${value}`)
  return value
}

/**
 * 校验并规范化 `paths`：字符串数组，每个元素是点分路径。
 * @param {unknown} value 原始值
 * @returns {string[]} 冻结的路径数组
 */
function checkPaths(value) {
  if (!Array.isArray(value)) invalid('paths', `expected an array of dot paths, got ${describeValue(value)}`)
  const paths = value.map((item, index) => {
    const key = `paths[${index}]`
    if (typeof item !== 'string' || item.trim() === '') {
      invalid(key, `expected a non-empty dot path, got ${describeValue(item)}`)
    }
    return item
  })
  return Object.freeze(paths)
}

/**
 * 把行配置规范化为带默认值的完整配置；非法输入抛错并指名键（C11）。
 * @param {object | undefined} raw 行配置
 * @returns {Readonly<object>} 冻结的完整配置
 */
export function normalizeConfig(raw = {}) {
  const source = raw === undefined || raw === null ? {} : raw
  if (!isPlainObject(source)) {
    throw new Error(`astrbox-bridge: invalid config: expected an object, got ${describeValue(source)}`)
  }
  for (const key of Object.keys(source)) {
    if (!KNOWN_KEYS.has(key)) invalid(key, 'unknown key')
  }

  const config = { ...DEFAULT_CONFIG, paths: [...DEFAULT_PATHS] }
  for (const key of PATH_KEYS) {
    if (source[key] !== undefined) config[key] = checkPath(key, source[key])
  }
  for (const key of NON_EMPTY_STRING_KEYS) {
    if (source[key] !== undefined) config[key] = checkNonEmptyString(key, source[key])
  }
  if (source.tickScope !== undefined) {
    if (typeof source.tickScope !== 'string') {
      invalid('tickScope', `expected a string, got ${describeValue(source.tickScope)}`)
    }
    config.tickScope = source.tickScope
  }
  if (source.paths !== undefined) config.paths = checkPaths(source.paths)
  for (const [key, min] of Object.entries(INTEGER_KEYS)) {
    if (source[key] !== undefined) config[key] = checkInteger(key, source[key], min)
  }
  return Object.freeze(config)
}
