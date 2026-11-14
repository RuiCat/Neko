/**
 * K7 面板数据路由：`GET /astrbox/blackboard`（挂在 DSH `webServer` 上的具名路由）。
 *
 * - 无参数 → `{ok:true, primary, paths, boards:[{entity, board}]}`（实体升序，来自
 *   `astrboxBridge.blackboards()`；`primary`/`paths` 是快照条元数据，即生效实体与白名单路径）
 * - `?entity=<id>` → `{ok:true, entity, board}`（无黑板时 `board:null`，不返回 404）
 * - 非 GET/HEAD → 405；数据源异常 → 500（面板据此显示「黑板不可用」）
 *
 * 只读、无副作用；路由注册失败由调用方兜底（bridge 缺少 webServer 时整段跳过）。
 */

/** 黑板面板路由路径（K7）。 */
export const BLACKBOARD_PATH = '/astrbox/blackboard'

/**
 * 写一条 JSON 响应。
 * @param {object} res 响应对象（http.ServerResponse）
 * @param {number} status 状态码
 * @param {unknown} body 响应体
 */
function sendJson(res, status, body) {
  res.writeHead(status, {
    'content-type': 'application/json; charset=utf-8',
    'cache-control': 'no-store',
  })
  res.end(JSON.stringify(body))
}

/**
 * 从请求 URL 取 `entity` 查询参数；非法/缺失返回 undefined。
 * @param {unknown} requestUrl 请求 URL
 * @returns {string | undefined} 实体 id
 */
function entityOf(requestUrl) {
  try {
    const url = new URL(typeof requestUrl === 'string' && requestUrl !== '' ? requestUrl : '/', 'http://localhost')
    const entity = url.searchParams.get('entity')
    return entity === null || entity.trim() === '' ? undefined : entity
  } catch {
    return undefined
  }
}

/** 快照条可用路径条数上限（与面板侧展示上限同口径，防止异常配置把响应撑大）。 */
const MAX_META_PATHS = 32

/**
 * 面板元数据（K7b）：生效实体 id + 快照白名单路径；数据源缺失或非法时返回空对象。
 * 只做形状收敛，不抛错（元数据坏掉只应让面板少显示快照条，不应让黑板不可用）。
 * @param {(() => {primary?: unknown, paths?: unknown}) | undefined} getMeta 元数据数据源
 * @returns {{primary?: string, paths?: string[]}} 收敛后的元数据（可能为空对象）
 */
function panelMetaOf(getMeta) {
  if (typeof getMeta !== 'function') return {}
  let raw
  try {
    raw = getMeta()
  } catch {
    return {}
  }
  if (raw === null || typeof raw !== 'object') return {}
  const meta = {}
  if (typeof raw.primary === 'string' && raw.primary.trim() !== '') meta.primary = raw.primary.trim()
  if (Array.isArray(raw.paths)) {
    const paths = []
    for (const path of raw.paths) {
      if (typeof path !== 'string' || path.trim() === '') continue
      paths.push(path.trim())
      if (paths.length >= MAX_META_PATHS) break
    }
    meta.paths = paths
  }
  return meta
}

/**
 * 处理一次黑板请求（导出供路由测试直接调用）；getMeta 为可选快照元数据，不抛错。
 * @param {{method?: string, url?: string}} req 请求 @param {object} res 响应
 * @param {() => Array<{entity: string, board: object|null}>} getBoards 黑板数据源
 * @returns {void}
 */
export function handleBlackboardRequest(req, res, getBoards, getMeta) {
  const method = typeof req?.method === 'string' ? req.method : 'GET'
  if (method !== 'GET' && method !== 'HEAD') {
    sendJson(res, 405, { ok: false, error: { code: 'method_not_allowed', message: '只支持 GET' } })
    return
  }
  let boards
  try {
    boards = typeof getBoards === 'function' ? getBoards() : []
  } catch {
    sendJson(res, 500, { ok: false, error: { code: 'internal', message: 'board source failed' } })
    return
  }
  const list = Array.isArray(boards) ? boards : []
  const entity = entityOf(req?.url)
  if (entity !== undefined) {
    const found = list.find((item) => item !== null && typeof item === 'object' && item.entity === entity)
    sendJson(res, 200, { ok: true, entity, board: found?.board ?? null })
    return
  }
  sendJson(res, 200, { ok: true, ...panelMetaOf(getMeta), boards: list })
}

/**
 * 在 webServer 上注册黑板路由（getBoards 黑板源 / getMeta 快照元数据）。
 * @param {{register: Function}} webServer `ctx.webServer`
 * @param {{getBoards: Function, getMeta?: Function, logger?: object}} options 数据源与日志
 * @returns {() => void} 注销函数（注册失败返回空函数）
 */
export function registerBlackboardRoute(webServer, options) {
  const { getBoards, getMeta, logger } = options ?? {}
  if (webServer === undefined || webServer === null || typeof webServer.register !== 'function') {
    return () => {}
  }
  try {
    const dispose = webServer.register({
      kind: 'exact',
      path: BLACKBOARD_PATH,
      handler: (req, res) => {
        handleBlackboardRequest(req, res, getBoards, getMeta)
      },
    })
    try {
      logger?.info?.(`事件黑板路由已就绪: ${BLACKBOARD_PATH}`)
    } catch {
      // 日志失败不影响路由。
    }
    return typeof dispose === 'function' ? dispose : () => {}
  } catch (error) {
    try {
      logger?.warnOnce?.(`blackboard-route`, `事件黑板路由注册失败（面板黑板不可用）：${error instanceof Error ? error.message : String(error)}`)
    } catch {
      // 记录注册失败时不得再抛错。
    }
    return () => {}
  }
}
