/**
 * K28 主会话提示词存储：按 NPC 实体 id 保存「主会话提示词」，供桥接注入机器人主会话。
 *
 * 机器人主会话（`主体`）没有 QQ 回复目标，qqbot 不安装 QQ 通道提示词；本存储补上这条
 * 按实体的补充提示词，由 `astrboxBridge.mainSessionPrompt(entityId)` 暴露给 qqbot。
 *
 * 默认路径 `~/.dsh-astrbox/main-session-prompts.json`（config `mainSessionPromptStatePath`
 * 非空时覆盖）。加载缺失/损坏 → 空表（fail-soft，只 warn）；保存走 `<path>.tmp` + rename，
 * 失败只 warn、内存值继续生效。文件读写同步、零依赖，供路由与桥接服务共用。
 */
import { mkdirSync, readFileSync, renameSync, writeFileSync } from 'node:fs'
import { homedir } from 'node:os'
import { dirname, join } from 'node:path'

/** 单条主会话提示词长度上限（字符）；与面板输入框的服务端校验同口径。 */
export const MAIN_SESSION_PROMPT_MAX_CHARS = 20_000

/** 默认状态目录（统一插件数据根下，位于用户主目录）。 */
export const PROMPT_STATE_DIR = join('.dsh-neko', 'state')

/** 默认状态文件名。 */
export const PROMPT_STATE_FILE = 'main-session-prompts.json'

/**
 * 默认状态文件路径：`<home>/.dsh-astrbox/main-session-prompts.json`。
 * @returns {string} 绝对路径
 */
export function defaultMainSessionPromptPath() {
  return join(homedir(), PROMPT_STATE_DIR, PROMPT_STATE_FILE)
}

/**
 * @param {unknown} error 原始错误
 * @returns {string} 可读错误文本
 */
function errorMessage(error) {
  return error instanceof Error ? error.message : String(error)
}

/**
 * 校验并归一化实体 id（fail-fast；路由层据此回 400）。
 * @param {unknown} entity 实体 id
 * @returns {string} trim 后的实体 id
 */
export function normalizePromptEntity(entity) {
  if (typeof entity !== 'string' || entity.trim() === '') throw new TypeError('entity 必须是非空字符串')
  return entity.trim()
}

/**
 * 校验并归一化提示词文本（fail-fast；空串合法 = 清除该实体的提示词）。
 * @param {unknown} prompt 提示词文本
 * @returns {string} trim 后的文本
 */
export function normalizePromptText(prompt) {
  if (typeof prompt !== 'string') throw new TypeError('prompt 必须是字符串')
  const text = prompt.trim()
  if (text.length > MAIN_SESSION_PROMPT_MAX_CHARS) {
    throw new TypeError(`prompt 超长（${text.length} > ${MAIN_SESSION_PROMPT_MAX_CHARS} 字符）`)
  }
  return text
}

/**
 * 建主会话提示词存储（K28）。
 * @param {{path?: string, logger?: object}} [options] `path` 空/缺省用默认路径；`logger.warnOnce` 可选
 * @returns {{path: string, get: Function, list: Function, set: Function, remove: Function, snapshot: Function}} 存储
 */
export function createMainSessionPromptStore(options = {}) {
  const { path, logger } = options
  const statePath = typeof path === 'string' && path.trim() !== '' ? path.trim() : defaultMainSessionPromptPath()
  /** 实体 id → 提示词文本。 */
  const prompts = new Map()

  /** 限频告警；logger 缺失/抛错都静默（fail-soft）。 */
  function warnOnce(message) {
    try {
      if (typeof logger?.warnOnce === 'function') logger.warnOnce('main-session-prompt', message)
      else if (typeof logger?.warn === 'function') logger.warn(message)
    } catch {
      // 记录告警失败不得影响存储。
    }
  }

  /** 读盘；缺失/损坏/项非法一律降级为空表（只记一次 warn）。 */
  function load() {
    let raw
    try {
      raw = readFileSync(statePath, 'utf8')
    } catch (error) {
      // ENOENT 是首次启动的正常形态，不告警。
      if (error?.code !== 'ENOENT') warnOnce(`主会话提示词状态读取失败（按空表处理）：${errorMessage(error)}`)
      return
    }
    let parsed
    try {
      parsed = JSON.parse(raw)
    } catch (error) {
      warnOnce(`主会话提示词状态不是合法 JSON（按空表处理）：${errorMessage(error)}`)
      return
    }
    const entries = parsed !== null && typeof parsed === 'object' ? parsed.prompts : undefined
    if (entries === null || typeof entries !== 'object' || Array.isArray(entries)) {
      warnOnce('主会话提示词状态缺少 prompts 对象（按空表处理）')
      return
    }
    for (const [entity, text] of Object.entries(entries)) {
      if (typeof entity !== 'string' || entity.trim() === '') continue
      if (typeof text !== 'string') continue
      const trimmed = text.trim()
      if (trimmed === '') continue
      prompts.set(entity.trim(), trimmed.slice(0, MAIN_SESSION_PROMPT_MAX_CHARS))
    }
  }

  /**
   * 落盘（`<path>.tmp` + rename，原子替换）。
   * @returns {boolean} 是否落盘成功（失败只 warn，内存值仍生效）
   */
  function save() {
    try {
      mkdirSync(dirname(statePath), { recursive: true })
      const tmpPath = `${statePath}.tmp`
      writeFileSync(tmpPath, `${JSON.stringify({ prompts: Object.fromEntries(prompts) }, null, 2)}\n`, 'utf8')
      renameSync(tmpPath, statePath)
      return true
    } catch (error) {
      warnOnce(`主会话提示词状态保存失败（内存生效）：${errorMessage(error)}`)
      return false
    }
  }

  load()

  return {
    /** @type {string} 实际状态文件路径（诊断/测试用） */
    path: statePath,

    /**
     * 读某实体的主会话提示词（同步、永不抛错；非法入参/未配置返回空串）。
     * @param {string} entityId 实体 id
     * @returns {string} 提示词文本（可能为空串）
     */
    get(entityId) {
      try {
        const entity = typeof entityId === 'string' ? entityId.trim() : ''
        return entity === '' ? '' : (prompts.get(entity) ?? '')
      } catch {
        return ''
      }
    },

    /**
     * 全部提示词的副本（实体 id → 文本）。
     * @returns {Record<string, string>} 只读快照
     */
    list() {
      return Object.fromEntries(prompts)
    },

    /**
     * 写入某实体的提示词并持久化；空文本 = 删除该条目。入参须先经 normalize 校验。
     * @param {string} entityId 实体 id
     * @param {string} text 提示词文本（空串 = 清除）
     * @returns {boolean} 是否落盘成功（内存值总会生效）
     */
    set(entityId, text) {
      const entity = normalizePromptEntity(entityId)
      const prompt = normalizePromptText(text)
      if (prompt === '') {
        if (!prompts.delete(entity)) return true
        return save()
      }
      prompts.set(entity, prompt)
      return save()
    },

    /**
     * 删除某实体的提示词并持久化。
     * @param {string} entityId 实体 id
     * @returns {boolean} 是否落盘成功（无条目时直接 true）
     */
    remove(entityId) {
      const entity = normalizePromptEntity(entityId)
      if (!prompts.delete(entity)) return true
      return save()
    },

    /**
     * 存储快照（测试/诊断用）。
     * @returns {{prompts: Record<string, string>}} 快照
     */
    snapshot() {
      return { prompts: Object.fromEntries(prompts) }
    },
  }
}
