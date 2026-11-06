/**
 * K17 订阅间隔覆盖存储：`{"intervals": {mainSessionId: minutes}}` 的加载/原子保存。
 *
 * 默认路径 `~/.dsh-astrbox/subscription.json`（config `subscriptionStatePath` 非空时覆盖）。
 * 加载缺失/损坏 → 空表（fail-soft，只 warn）；保存走 `<path>.tmp` + rename，失败只 warn、
 * 内存值继续生效（工具仍可用）。文件读写同步、零依赖，供 `subscriptionTool` 与插件门控共用。
 */
import { mkdirSync, readFileSync, renameSync, writeFileSync } from 'node:fs'
import { homedir } from 'node:os'
import { dirname, join } from 'node:path'

/** 覆盖间隔允许的分钟下限（K17：1..1440）。 */
export const MIN_INTERVAL_MINUTES = 1

/** 覆盖间隔允许的分钟上限（K17：1..1440）。 */
export const MAX_INTERVAL_MINUTES = 1440

/** 默认状态目录名（位于用户主目录下）。 */
export const SUBSCRIPTION_STATE_DIR = '.dsh-astrbox'

/** 默认状态文件名。 */
export const SUBSCRIPTION_STATE_FILE = 'subscription.json'

/**
 * @param {unknown} value 任意值
 * @returns {boolean} 是否为 1..1440 的整数
 */
export function isValidIntervalMinutes(value) {
  return Number.isInteger(value) && value >= MIN_INTERVAL_MINUTES && value <= MAX_INTERVAL_MINUTES
}

/**
 * 默认状态文件路径：`<home>/.dsh-astrbox/subscription.json`。
 * @returns {string} 绝对路径
 */
export function defaultSubscriptionPath() {
  return join(homedir(), SUBSCRIPTION_STATE_DIR, SUBSCRIPTION_STATE_FILE)
}

/**
 * @param {unknown} error 原始错误
 * @returns {string} 可读错误文本
 */
function errorMessage(error) {
  return error instanceof Error ? error.message : String(error)
}

/**
 * 建订阅间隔存储（K17）。
 * @param {{path?: string, logger?: object}} [options] `path` 空/缺省用默认路径；`logger.warnOnce` 可选
 * @returns {{path: string, get: Function, set: Function, reset: Function, snapshot: Function}} 存储
 */
export function createIntervalStore(options = {}) {
  const { path, logger } = options
  const statePath = typeof path === 'string' && path.trim() !== '' ? path.trim() : defaultSubscriptionPath()
  /** mainSessionId → minutes。 */
  const intervals = new Map()

  /** 限频告警；logger 缺失/抛错都静默（fail-soft）。 */
  function warnOnce(message) {
    try {
      logger?.warnOnce?.('subscription-store', message)
    } catch {
      // 记录失败不影响存储语义。
    }
  }

  /** 读文件并填充内存表；缺失/损坏/非法条目都只跳过（K17 fail-soft）。 */
  function load() {
    let text
    try {
      text = readFileSync(statePath, 'utf8')
    } catch (error) {
      if (error?.code !== 'ENOENT') warnOnce(`订阅间隔状态读取失败（按空处理）：${errorMessage(error)}`)
      return
    }
    try {
      const parsed = JSON.parse(text)
      const table = parsed !== null && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed.intervals : undefined
      if (table === null || typeof table !== 'object' || Array.isArray(table)) return
      for (const [sessionId, minutes] of Object.entries(table)) {
        if (typeof sessionId === 'string' && sessionId !== '' && isValidIntervalMinutes(minutes)) intervals.set(sessionId, minutes)
      }
    } catch (error) {
      warnOnce(`订阅间隔状态损坏（按空处理）：${errorMessage(error)}`)
    }
  }

  /** 原子保存：写 `.tmp` 再 rename；失败只 warn（内存值继续生效）。 */
  function save() {
    try {
      mkdirSync(dirname(statePath), { recursive: true })
      const tmpPath = `${statePath}.tmp`
      writeFileSync(tmpPath, `${JSON.stringify({ intervals: Object.fromEntries(intervals) }, null, 2)}\n`, 'utf8')
      renameSync(tmpPath, statePath)
      return true
    } catch (error) {
      warnOnce(`订阅间隔状态保存失败（内存生效）：${errorMessage(error)}`)
      return false
    }
  }

  load()

  return {
    /** @type {string} 实际状态文件路径（诊断/测试用） */
    path: statePath,

    /**
     * 读某主会话的覆盖分钟数。
     * @param {string} sessionId 主会话 id
     * @returns {number|undefined} 覆盖值；无覆盖/非法返回 undefined
     */
    get(sessionId) {
      return typeof sessionId === 'string' && sessionId !== '' ? intervals.get(sessionId) : undefined
    },

    /**
     * 写覆盖并持久化；非法输入不改表。
     * @param {string} sessionId 主会话 id
     * @param {number} minutes 1..1440 整数
     * @returns {boolean} 是否保存成功（内存值总会生效）
     */
    set(sessionId, minutes) {
      if (typeof sessionId !== 'string' || sessionId === '' || !isValidIntervalMinutes(minutes)) return false
      intervals.set(sessionId, minutes)
      return save()
    },

    /**
     * 清除覆盖并持久化。
     * @param {string} sessionId 主会话 id
     * @returns {boolean} 是否保存成功（无覆盖时直接 true）
     */
    reset(sessionId) {
      if (typeof sessionId !== 'string' || sessionId === '') return false
      if (!intervals.delete(sessionId)) return true
      return save()
    },

    /**
     * 全部覆盖的快照副本（测试/诊断用）。
     * @returns {{intervals: Record<string, number>}} 快照
     */
    snapshot() {
      return { intervals: Object.fromEntries(intervals) }
    },
  }
}
