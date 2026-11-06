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

/** 默认人设路径（K5；4 项基础人设 + `人格` 子集，默认不含 R18 字段）。 */
export const DEFAULT_PERSONA_PATHS = Object.freeze([
  '种族',
  '体型',
  '毛色',
  '自称',
  '对主人称呼',
  '人格.MBTI',
  '人格.气质类型',
])

/** K2 冻结的默认配置。 */
export const DEFAULT_CONFIG = Object.freeze({
  socketPath: resolve(REPO_ROOT, 'data', 'dsh-control.sock'),
  entity: '猫猫',
  paths: DEFAULT_PATHS,
  personaPaths: DEFAULT_PERSONA_PATHS,
  personaMaxChars: 600,
  promptMaxChars: 1200,
  promptRefreshMs: 5000,
  tickEveryMs: 60000,
  tickScope: '',
  /** 活跃窗口内每 tick 推进的世界时钟分钟数（0 = 关闭时钟）。 */
  clockStepMinutes: 10,
  /** 空闲时每 tick 推进的世界时钟分钟数（默认 1 = 空闲也慢速周期推进；0 = 暂停）。 */
  clockIdleStepMinutes: 1,
  /** 最近多久内有会话活动算「活跃」（毫秒）。 */
  clockActiveWindowMs: 600000,
  /** 事件黑板订阅总开关（K4）：false = 不推主会话、不注入子会话。 */
  subscriptionEnabled: true,
  /** 主会话主动读取黑板的最小间隔（毫秒，K5）。 */
  subscriptionMainIntervalMs: 600000,
  /** 单条订阅文本上限（字符，超限截断并追加 `…（截断）`，K4）。 */
  subscriptionMaxChars: 2000,
  /** 对话摘要每条 user/assistant 文本的字符上限（K10）。 */
  dialogExcerptChars: 160,
  /** 每实体保留的最近对话条数（K10）。 */
  dialogFeedSize: 20,
  /** 订阅间隔覆盖持久化路径（K17）；空串 = `~/.dsh-astrbox/subscription.json`。 */
  subscriptionStatePath: '',
  workspacePrefix: REPO_ROOT,
  functionName: 'dsh_on_event',
  requestTimeoutMs: 2000,
  tickTimeoutMs: 15000,
  warningAfterMs: 30000,
})

/** 整数键 → 允许的最小值（0 表示允许 0，如 tickEveryMs=0 关闭定时器）。 */
const INTEGER_KEYS = Object.freeze({
  promptMaxChars: 1,
  personaMaxChars: 1,
  promptRefreshMs: 0,
  tickEveryMs: 0,
  clockStepMinutes: 0,
  clockIdleStepMinutes: 0,
  clockActiveWindowMs: 0,
  subscriptionMainIntervalMs: 0,
  subscriptionMaxChars: 1,
  dialogExcerptChars: 1,
  dialogFeedSize: 1,
  requestTimeoutMs: 1,
  tickTimeoutMs: 1,
  warningAfterMs: 0,
})

/** 布尔键（K4 订阅开关）。 */
const BOOLEAN_KEYS = Object.freeze(['subscriptionEnabled'])

/** 必须为非空字符串的键。 */
const NON_EMPTY_STRING_KEYS = Object.freeze(['entity', 'functionName'])

/** 允许空串的字符串键（空串有特殊语义：tickScope=全量；subscriptionStatePath=默认路径）。 */
const STRING_KEYS = Object.freeze(['tickScope', 'subscriptionStatePath'])

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
 * 校验并规范化点分路径数组：元素必须是非空字符串；空数组合法（`personaPaths=[]` 关闭人设）。
 * @param {string} key 配置键名（错误信息与元素下标都用它）
 * @param {unknown} value 原始值
 * @returns {string[]} 冻结的路径数组
 */
function checkDotPaths(key, value) {
  if (!Array.isArray(value)) invalid(key, `expected an array of dot paths, got ${describeValue(value)}`)
  const paths = value.map((item, index) => {
    if (typeof item !== 'string' || item.trim() === '') {
      invalid(`${key}[${index}]`, `expected a non-empty dot path, got ${describeValue(item)}`)
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

  const config = { ...DEFAULT_CONFIG, paths: [...DEFAULT_PATHS], personaPaths: [...DEFAULT_PERSONA_PATHS] }
  for (const key of PATH_KEYS) {
    if (source[key] !== undefined) config[key] = checkPath(key, source[key])
  }
  for (const key of NON_EMPTY_STRING_KEYS) {
    if (source[key] !== undefined) config[key] = checkNonEmptyString(key, source[key])
  }
  for (const key of STRING_KEYS) {
    if (source[key] === undefined) continue
    if (typeof source[key] !== 'string') invalid(key, `expected a string, got ${describeValue(source[key])}`)
    config[key] = source[key]
  }
  if (source.paths !== undefined) config.paths = checkDotPaths('paths', source.paths)
  if (source.personaPaths !== undefined) config.personaPaths = checkDotPaths('personaPaths', source.personaPaths)
  for (const key of BOOLEAN_KEYS) {
    if (source[key] !== undefined && typeof source[key] !== 'boolean') {
      invalid(key, `expected a boolean, got ${describeValue(source[key])}`)
    }
    if (source[key] !== undefined) config[key] = source[key]
  }
  for (const [key, min] of Object.entries(INTEGER_KEYS)) {
    if (source[key] !== undefined) config[key] = checkInteger(key, source[key], min)
  }
  return Object.freeze(config)
}
