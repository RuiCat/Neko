/**
 * K8 会话黑板订阅状态：`{"subscriptions": {sessionId: {workspace: {since}, global: {since}}}}`
 * 的加载、订阅/退订、游标推进与原子保存。
 *
 * 默认路径 `~/.dsh-astrbox/board-subscriptions.json`（config `boardSubscriptionStatePath`
 * 非空时覆盖）。加载缺失/损坏/条目非法 → 空表（fail-soft，只 warn）；保存走
 * `<path>.tmp` + rename，失败只 warn、内存值继续生效（下一轮重复注入 = 至少一次投递）。
 * 文件读写同步、零依赖；测试用 `os.tmpdir()` 注入路径，不写真实用户目录。
 *
 * 游标语义（K8/C5）：`since` 只由 pre-step 在「差异已注入（或确认无需注入）」后推进；
 * `subscribe` 的初值 = 当前板 version（不灌历史）。
 */
import { mkdirSync, readFileSync, renameSync, writeFileSync } from 'node:fs'
import { homedir } from 'node:os'
import { dirname, join } from 'node:path'

/** 允许的黑板 scope（K1）。 */
export const BOARD_SCOPES = Object.freeze(['workspace', 'global'])

/** 默认状态目录名（位于用户主目录下）。 */
export const BOARD_SUBSCRIPTION_STATE_DIR = join('.dsh-neko', 'state')

/** 默认状态文件名。 */
export const BOARD_SUBSCRIPTION_STATE_FILE = 'board-subscriptions.json'

/** 订阅表最多保留的会话数（防御异常增长；超出按插入序淘汰最旧）。 */
const MAX_SESSIONS = 512

/** @param {unknown} value 任意值 @returns {boolean} 是否合法 scope */
export function isValidBoardScope(value) {
  return value === 'workspace' || value === 'global'
}

/** @param {unknown} value 任意值 @returns {boolean} 是否非负整数版本 */
function isVersion(value) {
  return Number.isInteger(value) && value >= 0
}

/**
 * 默认状态文件路径：`<home>/.dsh-astrbox/board-subscriptions.json`。
 * @returns {string} 绝对路径
 */
export function defaultBoardSubscriptionPath() {
  return join(homedir(), BOARD_SUBSCRIPTION_STATE_DIR, BOARD_SUBSCRIPTION_STATE_FILE)
}

/**
 * @param {unknown} error 原始错误
 * @returns {string} 可读错误文本
 */
function errorMessage(error) {
  return error instanceof Error ? error.message : String(error)
}

/**
 * 建黑板订阅存储（K8）。
 * @param {{path?: string, logger?: object}} [options] `path` 空/缺省用默认路径；`logger.warnOnce` 可选
 * @returns {object} 存储（path/get/subscribe/unsubscribe/setCursor/list/snapshot）
 */
export function createBoardSubscriptionStore(options = {}) {
  const { path, logger } = options
  const statePath = typeof path === 'string' && path.trim() !== '' ? path.trim() : defaultBoardSubscriptionPath()
  /** sessionId → {workspace?: {since}, global?: {since}}。 */
  const subscriptions = new Map()

  /** 限频告警；logger 缺失/抛错都静默（fail-soft）。 */
  function warnOnce(message) {
    try {
      logger?.warnOnce?.('board-subscription', message)
    } catch {
      // 记录失败不影响存储语义。
    }
  }

  /**
   * 规范化一条 scope 游标记录。
   * @param {unknown} raw 原始值
   * @returns {{since: number} | undefined} 规范化游标
   */
  function normalizeCursor(raw) {
    if (raw === null || typeof raw !== 'object' || Array.isArray(raw)) return undefined
    return isVersion(raw.since) ? { since: raw.since } : undefined
  }

  /** 读盘；缺失/损坏/非法条目一律跳过（只 warn 一次）。 */
  function load() {
    let text
    try {
      text = readFileSync(statePath, 'utf8')
    } catch (error) {
      // ENOENT 是首次启动的正常形态，不告警。
      if (error?.code !== 'ENOENT') warnOnce(`黑板订阅状态读取失败（按空表处理）：${errorMessage(error)}`)
      return
    }
    let parsed
    try {
      parsed = JSON.parse(text)
    } catch (error) {
      warnOnce(`黑板订阅状态不是合法 JSON（按空表处理）：${errorMessage(error)}`)
      return
    }
    const table = parsed !== null && typeof parsed === 'object' ? parsed.subscriptions : undefined
    if (table === null || typeof table !== 'object' || Array.isArray(table)) {
      warnOnce('黑板订阅状态缺少 subscriptions 对象（按空表处理）')
      return
    }
    for (const [sessionId, scopes] of Object.entries(table)) {
      if (typeof sessionId !== 'string' || sessionId.trim() === '') continue
      if (scopes === null || typeof scopes !== 'object' || Array.isArray(scopes)) continue
      const record = {}
      for (const scope of BOARD_SCOPES) {
        const cursor = normalizeCursor(scopes[scope])
        if (cursor !== undefined) record[scope] = cursor
      }
      if (Object.keys(record).length > 0) subscriptions.set(sessionId, record)
    }
  }

  /**
   * 原子保存：写 `<path>.tmp` 再 rename；失败只 warn（内存值继续生效）。
   * @returns {boolean} 是否落盘成功
   */
  function save() {
    try {
      mkdirSync(dirname(statePath), { recursive: true })
      const tmpPath = `${statePath}.tmp`
      const data = { subscriptions: Object.fromEntries(subscriptions) }
      writeFileSync(tmpPath, `${JSON.stringify(data, null, 2)}\n`, 'utf8')
      renameSync(tmpPath, statePath)
      return true
    } catch (error) {
      warnOnce(`黑板订阅状态保存失败（内存生效，下一轮可能重复注入）：${errorMessage(error)}`)
      return false
    }
  }

  /**
   * 取某会话的订阅记录副本。
   * @param {string} sessionId 会话 id
   * @returns {Record<string, {since: number}> | undefined} 记录副本
   */
  function get(sessionId) {
    if (typeof sessionId !== 'string' || sessionId === '') return undefined
    const record = subscriptions.get(sessionId)
    if (record === undefined) return undefined
    return Object.fromEntries(Object.entries(record).map(([scope, cursor]) => [scope, { since: cursor.since }]))
  }

  load()

  return {
    /** @type {string} 实际状态文件路径（诊断/测试用） */
    path: statePath,

    /**
     * 读某会话的订阅记录副本。
     * @param {string} sessionId 会话 id
     * @returns {Record<string, {since: number}> | undefined} 记录副本
     */
    get,

    /**
     * 建立/覆盖某会话某 scope 的订阅，游标设为 `since`（初值 = 当前板 version）。
     * @param {string} sessionId 会话 id
     * @param {'workspace'|'global'} scope 板 scope
     * @param {number} since 游标
     * @returns {boolean} 是否保存成功（内存值总会生效）；入参非法返回 false
     */
    subscribe(sessionId, scope, since) {
      if (typeof sessionId !== 'string' || sessionId === '') return false
      if (!isValidBoardScope(scope) || !isVersion(since)) return false
      const record = subscriptions.get(sessionId) ?? {}
      record[scope] = { since }
      subscriptions.delete(sessionId)
      subscriptions.set(sessionId, record)
      if (subscriptions.size > MAX_SESSIONS) {
        const oldest = subscriptions.keys().next()
        if (!oldest.done && oldest.value !== sessionId) subscriptions.delete(oldest.value)
      }
      return save()
    },

    /**
     * 退订：给 scope 只退该 scope，省略 scope 退整个会话。
     * @param {string} sessionId 会话 id
     * @param {'workspace'|'global'} [scope] 板 scope
     * @returns {boolean} 是否保存成功（无记录时直接 true）；入参非法返回 false
     */
    unsubscribe(sessionId, scope) {
      if (typeof sessionId !== 'string' || sessionId === '') return false
      if (scope !== undefined && !isValidBoardScope(scope)) return false
      const record = subscriptions.get(sessionId)
      if (record === undefined) return true
      if (scope === undefined) subscriptions.delete(sessionId)
      else {
        if (record[scope] === undefined) return true
        delete record[scope]
        if (Object.keys(record).length === 0) subscriptions.delete(sessionId)
      }
      return save()
    },

    /**
     * 推进游标（只对已订阅的会话/scope 生效；用于 pre-step 注入后续传）。
     * @param {string} sessionId 会话 id
     * @param {'workspace'|'global'} scope 板 scope
     * @param {number} since 新游标
     * @returns {{ok: boolean, saved: boolean}} `ok:false` = 未订阅/入参非法
     */
    setCursor(sessionId, scope, since) {
      if (typeof sessionId !== 'string' || sessionId === '') return { ok: false, saved: false }
      if (!isValidBoardScope(scope) || !isVersion(since)) return { ok: false, saved: false }
      const record = subscriptions.get(sessionId)
      if (record === undefined || record[scope] === undefined) return { ok: false, saved: false }
      if (record[scope].since === since) return { ok: true, saved: true }
      record[scope] = { since }
      return { ok: true, saved: save() }
    },

    /**
     * 全部订阅的快照副本（工具 `subscriptions` 与诊断用）。
     * @returns {{subscriptions: Record<string, Record<string, {since: number}>>}} 快照
     */
    list() {
      return { subscriptions: Object.fromEntries([...subscriptions].map(([sessionId, record]) => [sessionId, Object.fromEntries(Object.entries(record).map(([scope, cursor]) => [scope, { since: cursor.since }]))])) }
    },

    /**
     * 存储快照（等价 `list`，测试/诊断用）。
     * @returns {{subscriptions: object}} 快照
     */
    snapshot() {
      return { subscriptions: Object.fromEntries([...subscriptions].map(([sessionId, record]) => [sessionId, Object.fromEntries(Object.entries(record).map(([scope, cursor]) => [scope, { since: cursor.since }]))])) }
    },
  }
}
