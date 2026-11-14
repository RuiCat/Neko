/**
 * K10 `astrbox_board` 工具：会话读写工作区/跨工作区会话黑板，并管理差异订阅。
 *
 * - `op=read`：读板（`since` 可选；带 since 返回差异，不带返回当前 live 视图）；
 * - `op=write`：写本会话/本工作区自己的区域（`set`/`delete`/`clear`，控制面二次校验）；
 * - `op=subscribe` / `unsubscribe`：订阅/退订 pre-step 差异注入（游标初值 = 当前板版本）；
 * - `op=subscriptions`：查看自己的订阅与游标。
 *
 * 身份只从 `exec.agent` 解析（K7）：sessionId = `session.id`（兜底 `agent.id`），
 * workspaceKey = 规范化 cwd；工具参数 schema 里**没有** `session`/`workspace` 字段。
 * 工具永不抛异常，失败返回 `{ok:false, reason}`（`invalid_args|no_workspace|
 * control_unavailable|forbidden|bad_response|internal_error`）。
 */
import { normalizeCwd, resolveEntityForCwd } from './entityBindings.mjs'

/** 工具名（K10 冻结）。 */
export const BOARD_TOOL_NAME = 'astrbox_board'

/** 单次写入批的 ops 上限（K5/K9）。 */
export const BOARD_WRITE_MAX_OPS = 16

/** 写入 key 字符上限（K5/K9）。 */
export const BOARD_KEY_MAX_CHARS = 128

/** 写入值 JSON 编码字节上限（K5/K9）。 */
export const BOARD_VALUE_MAX_BYTES = 2048

/** 非法 key 字符：换行与控制字符（K5/K9）。 */
const BAD_KEY_PATTERN = /[\u0000-\u001f\u007f]/

/** 读板结果里最多渲染的条目行数（超出只报剩余条数）。 */
const BOARD_RENDER_MAX_LINES = 30

/** 读板结果里单条值的渲染字符上限（对象先转 JSON 再截断）。 */
const BOARD_VALUE_RENDER_CHARS = 160

/**
 * @param {unknown} value 任意值
 * @returns {boolean} 是否为普通对象
 */
function isPlainObject(value) {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

/**
 * cwd 的最后一段（作为未命中绑定时的 label）。
 * @param {string} workspaceKey 规范化工作区键
 * @returns {string} 标签（根目录记 `/`）
 */
function basenameOf(workspaceKey) {
  if (workspaceKey === '') return ''
  if (workspaceKey === '/') return '/'
  const segments = workspaceKey.split(/[\\/]/).filter((segment) => segment !== '')
  return segments.length === 0 ? '/' : segments[segments.length - 1]
}

/**
 * K7 调用方身份：只从 `exec.agent` 取，工具参数不参与。
 * @param {{agent?: object}} exec 工具执行上下文
 * @param {() => Array<object>} getBindings qqbot 绑定（可选）
 * @returns {{sessionId: string, workspaceKey: string, label: string}} 调用方
 */
export function callerOf(exec, getBindings = () => []) {
  const agent = exec?.agent
  const sessionId = typeof agent?.session?.id === 'string' && agent.session.id !== ''
    ? agent.session.id
    : typeof agent?.id === 'string' ? agent.id : ''
  const workspaceKey = normalizeCwd(agent?.session?.header?.cwd) ?? ''
  let label = ''
  if (workspaceKey !== '') {
    try {
      const bindings = typeof getBindings === 'function' ? getBindings() : []
      const binding = resolveEntityForCwd(Array.isArray(bindings) ? bindings : [], workspaceKey)
      label = typeof binding?.label === 'string' && binding.label !== ''
        ? binding.label
        : typeof binding?.entity === 'string' && binding.entity !== '' ? binding.entity : ''
    } catch {
      label = ''
    }
    if (label === '') label = basenameOf(workspaceKey)
  }
  return { sessionId, workspaceKey, label }
}

/**
 * 校验一个写入 key（K5：非空、≤128 字符、禁换行/控制字符）。
 * @param {unknown} key 原始 key
 * @returns {string} 合法 key；非法返回空串
 */
function validKey(key) {
  if (typeof key !== 'string' || key.trim() === '') return ''
  if (key.length > BOARD_KEY_MAX_CHARS) return ''
  if (BAD_KEY_PATTERN.test(key)) return ''
  return key
}

/**
 * 把工具参数规范成 K5 的写请求 ops；非法返回 `{error: 'invalid_args'}`。
 * @param {object} args 工具参数
 * @returns {{ops?: object[], error?: string}} 规范化结果
 */
export function normalizeWriteOps(args) {
  const clear = args.clear === true
  const set = isPlainObject(args.set) ? args.set : undefined
  const del = Array.isArray(args.delete) ? args.delete : undefined
  if (args.set !== undefined && set === undefined) return { error: 'invalid_args' }
  if (args.delete !== undefined && del === undefined) return { error: 'invalid_args' }
  if (clear && (set !== undefined || del !== undefined)) return { error: 'invalid_args' }
  if (!clear && set === undefined && del === undefined) return { error: 'invalid_args' }
  if (clear) return { ops: [{ op: 'clear' }] }
  const ops = []
  for (const [key, value] of Object.entries(set ?? {})) {
    const valid = validKey(key)
    if (valid === '') return { error: 'invalid_args' }
    const encoded = JSON.stringify(value)
    if (encoded === undefined || Buffer.byteLength(encoded, 'utf8') > BOARD_VALUE_MAX_BYTES) {
      return { error: 'invalid_args' }
    }
    ops.push({ op: 'set', key: valid, value })
  }
  for (const key of del ?? []) {
    const valid = validKey(key)
    if (valid === '') return { error: 'invalid_args' }
    ops.push({ op: 'delete', key: valid })
  }
  if (ops.length === 0 || ops.length > BOARD_WRITE_MAX_OPS) return { error: 'invalid_args' }
  return { ops }
}

/**
 * 控制面错误 → K10 reason（词表固定）。
 * @param {object} error ControlError
 * @returns {string} reason
 */
function reasonOf(error) {
  const code = typeof error?.code === 'string' ? error.code : ''
  if (code === 'forbidden') return 'forbidden'
  if (code === 'bad_response') return 'bad_response'
  // 控制面 400（参数/批/键/值/容量）是调用方问题，归 invalid_args；
  // 不映射会落到默认分支被误报成 bad_response（真机实测踩到）。
  if (code === 'bad_request' || code === 'board_too_large' || code === 'workspace_limit'
    || code === 'store_too_large' || code === 'region_limit') return 'invalid_args'
  if (code === 'timeout' || code === 'network_error' || code === 'ECONNREFUSED' || code === 'ENOENT') return 'control_unavailable'
  if (code === 'internal' || code === 'http_500' || code === 'persist_failed') return 'internal_error'
  return 'bad_response'
}

/**
 * 把 K1 板里的 live 条目拍平成工具返回的条目视图。
 * @param {object} board K1 板
 * @returns {Array<object>} 条目
 */
function flattenLiveEntries(board) {
  const entries = []
  if (!isPlainObject(board) || !Array.isArray(board.regions)) return entries
  for (const region of board.regions) {
    if (!isPlainObject(region) || !Array.isArray(region.entries)) continue
    for (const entry of region.entries) {
      if (!isPlainObject(entry) || entry.deleted === true) continue
      entries.push({
        region: typeof region.id === 'string' ? region.id : '',
        key: entry.key,
        from: entry.from ?? null,
        to: entry.to ?? null,
        by: typeof entry.by === 'string' ? entry.by : '',
        version: Number.isInteger(entry.version) ? entry.version : 0,
      })
    }
  }
  return entries
}

/**
 * 把 K4 响应里的 deltas 拍平成工具返回的条目视图（保留 tombstone）。
 * @param {unknown} deltas deltas 数组
 * @returns {Array<object>} 条目
 */
function flattenDeltas(deltas) {
  const entries = []
  if (!Array.isArray(deltas)) return entries
  for (const entry of deltas) {
    if (!isPlainObject(entry)) continue
    entries.push({
      // K4 契约字段是 region_id；region 仅作旧测试夹具兼容。
      region: typeof entry.region_id === 'string' ? entry.region_id
        : (typeof entry.region === 'string' ? entry.region : ''),
      key: entry.key,
      from: entry.from ?? null,
      to: entry.to ?? null,
      by: typeof entry.by === 'string' ? entry.by : '',
      version: Number.isInteger(entry.version) ? entry.version : 0,
      deleted: entry.deleted === true,
    })
  }
  return entries
}

/**
 * 单条黑板值 → 内联文本：对象走 JSON，换行压成空格，超长截断。
 * @param {unknown} value 值
 * @returns {string} 内联文本
 */
function inlineValue(value) {
  const raw = value === undefined || value === null
    ? 'null'
    : (typeof value === 'string' ? value : JSON.stringify(value))
  const flat = String(raw === undefined ? 'null' : raw).replace(/\s+/g, ' ').trim()
  return flat.length <= BOARD_VALUE_RENDER_CHARS ? flat : `${flat.slice(0, BOARD_VALUE_RENDER_CHARS)}…`
}

/**
 * 一条读板条目行：`- [区域] key：from → to`（跨工作区板带区域前缀）。
 * @param {object} entry 条目视图
 * @param {unknown} scope 板 scope
 * @returns {string} 行文本
 */
function entryRenderLine(entry, scope) {
  const key = typeof entry.key === 'string' ? entry.key : String(entry.key ?? '')
  const region = typeof entry.region === 'string' ? entry.region : ''
  const prefix = scope === 'global' && region !== '' ? `[${region}] ` : ''
  return `- ${prefix}${key}：${inlineValue(entry.from)} → ${inlineValue(entry.to)}`
}

/**
 * 工具结果渲染（output.render 必须纯函数且不抛错，K10：简短中文摘要）。
 * @param {unknown} value execute 返回的规范值
 * @returns {string} 渲染文本
 */
export function renderBoardResult(value) {
  const result = isPlainObject(value) ? value : {}
  if (result.ok !== true) return `会话黑板：未成功（${String(result.reason ?? 'unknown')}）`
  const scopeLabel = result.scope === 'global' ? '跨工作区黑板' : '工作区黑板'
  if (result.op === 'read') {
    const entries = Array.isArray(result.entries) ? result.entries : []
    const count = Number.isInteger(result.count) ? result.count : entries.length
    const kind = result.since === undefined ? '条目' : '差异'
    const lines = [`读取${scopeLabel}：版本 ${String(result.version ?? 0)}，${kind} ${count} 条${result.truncated === true ? '（历史已裁剪）' : ''}`]
    const shown = entries.slice(0, BOARD_RENDER_MAX_LINES)
    for (const entry of shown) {
      if (!isPlainObject(entry)) continue
      lines.push(entryRenderLine(entry, result.scope))
    }
    if (entries.length > shown.length) lines.push(`…（还有 ${entries.length - shown.length} 条）`)
    return lines.join('\n')
  }
  if (result.op === 'write') {
    const parts = [`写入${scopeLabel}：${result.changed === true ? '已更新' : '无净变化'}，版本 ${String(result.version ?? 0)}`]
    if (Array.isArray(result.keys) && result.keys.length > 0) parts.push(`键 ${result.keys.join('、')}`)
    if (Array.isArray(result.trimmed) && result.trimmed.length > 0) parts.push(`裁剪 ${result.trimmed.length} 条`)
    if (Array.isArray(result.evicted) && result.evicted.length > 0) parts.push(`淘汰 ${result.evicted.length} 个区域`)
    return parts.join('；')
  }
  if (result.op === 'subscribe') return `已订阅${scopeLabel}：从版本 ${String(result.since ?? 0)} 起收差异`
  if (result.op === 'unsubscribe') return `已退订${scopeLabel}`
  if (result.op === 'subscriptions') {
    const list = Array.isArray(result.subscriptions) ? result.subscriptions : []
    if (list.length === 0) return '会话黑板订阅：无'
    return `会话黑板订阅：${list.map((item) => `${item.scope === 'global' ? '跨工作区' : '工作区'}(since ${String(item.since ?? 0)})`).join('；')}`
  }
  return '会话黑板：完成'
}

/**
 * 建 `astrbox_board` 工具（K10）。
 * @param {{client: object, store: object, getBindings?: Function, logger?: object}} options 依赖
 * @returns {{definition: object, execute: Function}} 工具
 */
export function createSessionBoardTool(options) {
  const { client, store, getBindings = () => [], logger } = options ?? {}

  /**
   * @param {string} message 日志
   * @returns {void}
   */
  function warn(message) {
    try {
      logger?.warnOnce?.('board-tool', message)
    } catch {
      // 日志失败不影响返回。
    }
  }

  /**
   * 读控制面板（K4）；响应非法时抛 `bad_response`。
   * @param {{scope: string, workspaceKey: string, since?: number}} request 读请求
   * @returns {Promise<object>} `{board, deltas?, truncated?, since?}`
   */
  async function readBoard(request) {
    const body = await client.readBoard({
      scope: request.scope,
      ...(request.scope === 'workspace' ? { workspace: request.workspaceKey } : {}),
      ...(Number.isInteger(request.since) ? { since: request.since } : {}),
    })
    if (!isPlainObject(body) || body.ok !== true || !isPlainObject(body.board)) {
      const error = new Error('control /boards response is not a board envelope')
      error.code = 'bad_response'
      throw error
    }
    return body
  }

  /**
   * 校验 scope；workspace 板还要求调用方有工作区键（K7 no_workspace）。
   * @param {object} args 工具参数
   * @param {{sessionId: string, workspaceKey: string}} caller 调用方
   * @returns {{scope: string} | {reason: string}} 结果
   */
  function resolveScope(args, caller) {
    const scope = args.scope
    if (scope !== 'workspace' && scope !== 'global') return { reason: 'invalid_args' }
    if (scope === 'workspace' && caller.workspaceKey === '') return { reason: 'no_workspace' }
    return { scope }
  }

  /**
   * `op=read`：读板（可选 `since`）。
   * @param {object} args 参数
   * @param {object} caller 调用方
   * @returns {Promise<object>} 工具结果
   */
  async function executeRead(args, caller) {
    const resolved = resolveScope(args, caller)
    if (resolved.reason !== undefined) return { ok: false, reason: resolved.reason }
    if (args.since !== undefined && !(Number.isInteger(args.since) && args.since >= 0)) {
      return { ok: false, reason: 'invalid_args' }
    }
    const response = await readBoard({ scope: resolved.scope, workspaceKey: caller.workspaceKey, since: args.since })
    const board = response.board
    const entries = args.since === undefined ? flattenLiveEntries(board) : flattenDeltas(response.deltas)
    return {
      ok: true,
      op: 'read',
      scope: resolved.scope,
      version: Number.isInteger(board.version) ? board.version : 0,
      updated_at: typeof board.updated_at === 'string' ? board.updated_at : '',
      min_retained_version: Number.isInteger(board.min_retained_version) ? board.min_retained_version : 0,
      truncated: response.truncated === true,
      count: entries.length,
      entries,
      ...(args.since === undefined ? {} : { since: args.since }),
    }
  }

  /**
   * `op=write`：写本会话/本工作区自己的区（K5）。
   * @param {object} args 参数
   * @param {object} caller 调用方
   * @returns {Promise<object>} 工具结果
   */
  async function executeWrite(args, caller) {
    const resolved = resolveScope(args, caller)
    if (resolved.reason !== undefined) return { ok: false, reason: resolved.reason }
    if (caller.sessionId === '') return { ok: false, reason: 'no_workspace' }
    const normalized = normalizeWriteOps(args)
    if (normalized.ops === undefined) return { ok: false, reason: 'invalid_args' }
    // K5：workspace 在两种 scope 下都必须带——global 板要靠它推导本工作区的区
    // （Go 侧 resolveTarget 对 global 要求非空 workspaceKey）。
    const body = await client.writeBoard({
      scope: resolved.scope,
      workspace: caller.workspaceKey,
      session: caller.sessionId,
      ops: normalized.ops,
    })
    if (!isPlainObject(body) || body.ok !== true) return { ok: false, reason: 'bad_response' }
    return {
      ok: true,
      op: 'write',
      scope: resolved.scope,
      changed: body.changed === true,
      version: Number.isInteger(body.version) ? body.version : 0,
      updated_at: typeof body.updated_at === 'string' ? body.updated_at : '',
      trimmed: Array.isArray(body.trimmed) ? body.trimmed : [],
      evicted: Array.isArray(body.evicted) ? body.evicted : [],
      keys: normalized.ops.map((op) => op.key).filter((key) => typeof key === 'string' && key !== ''),
    }
  }

  /**
   * `op=subscribe`：订阅差异注入，游标初值 = 当前板版本（不灌历史）。
   * @param {object} args 参数
   * @param {object} caller 调用方
   * @returns {Promise<object>} 工具结果
   */
  async function executeSubscribe(args, caller) {
    const resolved = resolveScope(args, caller)
    if (resolved.reason !== undefined) return { ok: false, reason: resolved.reason }
    if (caller.sessionId === '') return { ok: false, reason: 'no_workspace' }
    const response = await readBoard({ scope: resolved.scope, workspaceKey: caller.workspaceKey })
    const version = Number.isInteger(response.board.version) ? response.board.version : 0
    const persisted = store.subscribe(caller.sessionId, resolved.scope, version)
    return { ok: true, op: 'subscribe', scope: resolved.scope, since: version, version, subscribed: true, persisted }
  }

  /**
   * `op=unsubscribe`：退订（给定 scope 退该 scope，否则退整个会话）。
   * @param {object} args 参数
   * @param {object} caller 调用方
   * @returns {Promise<object>} 工具结果
   */
  async function executeUnsubscribe(args, caller) {
    if (caller.sessionId === '') return { ok: false, reason: 'no_workspace' }
    const scope = args.scope
    if (scope !== undefined && scope !== 'workspace' && scope !== 'global') return { ok: false, reason: 'invalid_args' }
    const persisted = store.unsubscribe(caller.sessionId, scope)
    return { ok: true, op: 'unsubscribe', ...(scope === undefined ? {} : { scope }), subscribed: false, persisted }
  }

  /**
   * `op=subscriptions`：列出本会话的订阅与游标。
   * @param {object} _args 参数（未使用）
   * @param {object} caller 调用方
   * @returns {Promise<object>} 工具结果
   */
  async function executeSubscriptions(_args, caller) {
    if (caller.sessionId === '') return { ok: false, reason: 'no_workspace' }
    const record = store.get(caller.sessionId) ?? {}
    const subscriptions = ['workspace', 'global']
      .filter((scope) => record[scope] !== undefined)
      .map((scope) => ({ scope, since: record[scope].since }))
    return { ok: true, op: 'subscriptions', subscriptions }
  }

  /**
   * 执行一次工具调用（永不抛异常）。
   * @param {unknown} rawArgs 模型参数
   * @param {{agent?: object}} [exec] 执行上下文
   * @returns {Promise<object>} 工具结果
   */
  async function execute(rawArgs, exec) {
    try {
      const args = isPlainObject(rawArgs) ? rawArgs : {}
      if (typeof args.op !== 'string' || args.op === '') return { ok: false, reason: 'invalid_args' }
      const caller = callerOf(exec, getBindings)
      // K7：缺 sessionId 或 cwd 一律 no_workspace，不发请求。
      if (caller.sessionId === '' || caller.workspaceKey === '') return { ok: false, reason: 'no_workspace' }
      if (args.op === 'read') return await executeRead(args, caller)
      if (args.op === 'write') return await executeWrite(args, caller)
      if (args.op === 'subscribe') return await executeSubscribe(args, caller)
      if (args.op === 'unsubscribe') return await executeUnsubscribe(args, caller)
      if (args.op === 'subscriptions') return await executeSubscriptions(args, caller)
      return { ok: false, reason: 'invalid_args' }
    } catch (error) {
      if (error?.code === 'bad_response') return { ok: false, reason: 'bad_response' }
      warn(`astrbox_board 执行失败（已忽略）：${error instanceof Error ? error.message : String(error)}`)
      return { ok: false, reason: reasonOf(error) }
    }
  }

  const definition = {
    name: BOARD_TOOL_NAME,
    description:
      '读写会话黑板（工作区/跨工作区）并管理差异订阅。op=read 读板（since 返回差异）；'
      + 'write 写 set/delete/clear 到本会话（工作区板）或本工作区（跨工作区板）自己的区域；'
      + 'subscribe/unsubscribe/subscriptions 管理每轮开头的 [astrbox-board] 差异注入。'
      + '身份由桥接从执行上下文注入，参数里没有 session/workspace。',
    parameters: {
      type: 'object',
      properties: {
        op: {
          type: 'string',
          enum: ['read', 'write', 'subscribe', 'unsubscribe', 'subscriptions'],
          description: '操作：read/write/subscribe/unsubscribe/subscriptions。',
        },
        scope: {
          type: 'string',
          enum: ['workspace', 'global'],
          description: '板 scope：workspace=同工作区会话共享；global=跨工作区只读他区、只写本区。',
        },
        since: { type: 'integer', minimum: 0, description: 'read 的游标；给出时只返回差异。' },
        set: { type: 'object', description: 'write：按键写入（key → value）。' },
        delete: { type: 'array', items: { type: 'string' }, description: 'write：要删除的 key 列表。' },
        clear: { type: 'boolean', description: 'write：清空本会话/本工作区自己的区（不可与 set/delete 同批）。' },
      },
      required: ['op'],
      additionalProperties: false,
    },
    output: {
      schema: {
        type: 'object',
        properties: {
          ok: { type: 'boolean' },
          reason: { type: 'string' },
          op: { type: 'string' },
          scope: { type: 'string' },
          version: { type: 'integer' },
          updated_at: { type: 'string' },
          min_retained_version: { type: 'integer' },
          truncated: { type: 'boolean' },
          since: { type: 'integer' },
          count: { type: 'integer' },
          changed: { type: 'boolean' },
          keys: { type: 'array' },
          trimmed: { type: 'array' },
          evicted: { type: 'array' },
          subscribed: { type: 'boolean' },
          persisted: { type: 'boolean' },
          subscriptions: { type: 'array' },
          entries: { type: 'array' },
        },
        required: ['ok'],
        additionalProperties: false,
      },
      render: (_args, value) => [{ type: 'text', text: renderBoardResult(value) }],
    },
    execute: (args, exec) => execute(args, exec),
  }

  return { definition, execute }
}
