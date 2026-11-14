/**
 * K15 会话黑板面板路由：`GET /astrbox/boards`、`GET /astrbox/boards/workspaces`、
 * `POST /astrbox/boards/panel`（挂在 DSH `webServer` 上的 exact 路由）。
 *
 * 方向：面板浏览器半 → 本路由 → Go 控制面（Unix socket）。三条路由只做转发与轻量参数校验：
 * - 读路由透传控制面 body 与状态码（`readBoard` / `listBoardWorkspaces`）；
 * - 面板写路由转发 `{scope,workspace,region,ops}`，`by` 由控制面固定为 `面板`（K13）；
 * - 非法 JSON/参数 → `400 {ok:false,error:{code:'bad_request',message}}`；方法不符 → 405；
 *   控制面不可达（传输错误）→ `500 control_unavailable`；控制面 4xx/5xx 信封按原状态透传。
 *
 * 路由不做鉴权（同源面板，与 K21/C8 同一信任边界），不缓存、无副作用。
 */

/** 三条会话黑板面板路由路径（K15 冻结）。 */
export const SESSION_BOARD_ROUTES = Object.freeze({
  boards: '/astrbox/boards',
  workspaces: '/astrbox/boards/workspaces',
  panel: '/astrbox/boards/panel',
})

/** 请求体大小上限（K13 的 ops 上限反推 < 64 KiB，这里留余量）。 */
const MAX_BODY_BYTES = 256 * 1024

/** JSON 解析失败标记。 */
const INVALID_BODY = Symbol('invalid-body')

/**
 * @param {unknown} error 原始错误
 * @returns {string} 可读错误文本
 */
function errorMessage(error) {
  return error instanceof Error ? error.message : String(error)
}

/**
 * 写一条 JSON 响应。
 * @param {object} res http.ServerResponse
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
 * 桥接自身错误响应（K15 信封）。
 * @param {object} res 响应
 * @param {number} status 状态码
 * @param {string} code 稳定错误码
 * @param {string} message 可读原因
 */
function sendFailure(res, status, code, message) {
  sendJson(res, status, { ok: false, error: { code, message } })
}

/**
 * 控制面失败 → 响应：有 HTTP 状态时按原状态透传控制面信封，传输错误归 `control_unavailable`。
 * @param {object} res 响应
 * @param {unknown} error ControlError（可能带 status/body）或任意异常
 */
function sendControlFailure(res, error) {
  const status = Number.isInteger(error?.status) && error.status >= 400 && error.status <= 599 ? error.status : 0
  if (status === 0) {
    sendFailure(res, 500, 'control_unavailable', errorMessage(error))
    return
  }
  const body = error?.body
  if (body !== null && typeof body === 'object' && body.ok === false) {
    sendJson(res, status, body)
    return
  }
  sendFailure(res, status, typeof error?.code === 'string' ? error.code : `http_${status}`, errorMessage(error))
}

/**
 * 读并解析 JSON 请求体；空体返回 `{}`，非法 JSON/非对象返回 INVALID_BODY。
 * @param {object} req http.IncomingMessage
 * @returns {Promise<object|symbol>} 请求体或 INVALID_BODY
 */
async function readJsonBody(req) {
  let size = 0
  const chunks = []
  try {
    for await (const chunk of req) {
      size += chunk.length
      if (size > MAX_BODY_BYTES) return INVALID_BODY
      chunks.push(chunk)
    }
  } catch {
    return INVALID_BODY
  }
  if (size === 0) return {}
  try {
    const parsed = JSON.parse(Buffer.concat(chunks).toString('utf8'))
    return parsed !== null && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed : INVALID_BODY
  } catch {
    return INVALID_BODY
  }
}

/**
 * 解析 `GET /astrbox/boards` 查询参数（K4/K15）：`scope` 白名单、workspace 板必须有
 * `workspace`、`since` 为非负整数。
 * @param {unknown} requestUrl 请求 URL
 * @returns {{query?: object, error?: string}} 查询参数或错误
 */
function boardQueryOf(requestUrl) {
  let url
  try {
    url = new URL(typeof requestUrl === 'string' && requestUrl !== '' ? requestUrl : '/', 'http://localhost')
  } catch {
    return { error: '请求 URL 非法' }
  }
  const scope = url.searchParams.get('scope')
  if (scope !== 'workspace' && scope !== 'global') return { error: 'scope 必须是 workspace 或 global' }
  const query = { scope }
  const workspace = url.searchParams.get('workspace')
  if (workspace !== null && workspace.trim() !== '') query.workspace = workspace
  else if (scope === 'workspace') return { error: 'workspace 板需要 workspace 参数' }
  const since = url.searchParams.get('since')
  if (since !== null) {
    if (!/^\d+$/.test(since.trim())) return { error: 'since 必须是非负整数' }
    query.since = Number(since.trim())
  }
  return { query }
}

/**
 * 校验并收敛 `POST /astrbox/boards/panel` 请求体（K13）：scope 白名单、workspace 必填、
 * workspace 板必须有 region、跨工作区板显式 region 必须等于 workspace、ops 非空对象数组。
 * @param {object} body 请求体
 * @returns {{payload?: object, error?: string}} 转发请求体或错误
 */
function panelPayloadOf(body) {
  if (body === null || typeof body !== 'object' || Array.isArray(body)) return { error: '请求体必须是 JSON 对象' }
  const scope = body.scope
  if (scope !== 'workspace' && scope !== 'global') return { error: 'scope 必须是 workspace 或 global' }
  const workspace = typeof body.workspace === 'string' ? body.workspace.trim() : ''
  if (workspace === '') return { error: 'workspace 必须是非空字符串' }
  const region = typeof body.region === 'string' ? body.region.trim() : ''
  if (scope === 'workspace' && region === '') return { error: 'workspace 板需要 region' }
  if (scope === 'global' && region !== '' && region !== workspace) return { error: '跨工作区板的 region 必须等于 workspace' }
  const ops = body.ops
  if (!Array.isArray(ops) || ops.length === 0) return { error: 'ops 必须是非空数组' }
  for (const op of ops) {
    if (op === null || typeof op !== 'object' || Array.isArray(op)) return { error: 'ops 每项必须是对象' }
  }
  const payload = { scope, workspace, ops }
  if (region !== '') payload.region = region
  return { payload }
}

/**
 * 读一块板（K4）：`GET /astrbox/boards`。
 * @param {object} req 请求
 * @param {object} res 响应
 * @param {object} client 控制面客户端
 * @returns {Promise<void>} 无
 */
async function handleBoardsRead(req, res, client) {
  if (req?.method !== 'GET') {
    sendFailure(res, 405, 'method_not_allowed', '只支持 GET')
    return
  }
  const parsed = boardQueryOf(req?.url)
  if (parsed.error !== undefined) {
    sendFailure(res, 400, 'bad_request', parsed.error)
    return
  }
  sendJson(res, 200, await client.readBoard(parsed.query))
}

/**
 * 列工作区板摘要（K14）：`GET /astrbox/boards/workspaces`。
 * @param {object} req 请求
 * @param {object} res 响应
 * @param {object} client 控制面客户端
 * @returns {Promise<void>} 无
 */
async function handleWorkspacesRead(req, res, client) {
  if (req?.method !== 'GET') {
    sendFailure(res, 405, 'method_not_allowed', '只支持 GET')
    return
  }
  sendJson(res, 200, await client.listBoardWorkspaces())
}

/**
 * 面板管理员写（K13）：`POST /astrbox/boards/panel`，转发 `{scope,workspace,region,ops}`。
 * @param {object} req 请求
 * @param {object} res 响应
 * @param {object} client 控制面客户端
 * @returns {Promise<void>} 无
 */
async function handlePanelWrite(req, res, client) {
  if (req?.method !== 'POST') {
    sendFailure(res, 405, 'method_not_allowed', '只支持 POST')
    return
  }
  const body = await readJsonBody(req)
  if (body === INVALID_BODY) {
    sendFailure(res, 400, 'bad_request', '请求体不是合法 JSON 对象')
    return
  }
  const parsed = panelPayloadOf(body)
  if (parsed.error !== undefined) {
    sendFailure(res, 400, 'bad_request', parsed.error)
    return
  }
  sendJson(res, 200, await client.writePanelBoard(parsed.payload))
}

/**
 * 处理一次会话黑板面板请求（导出供路由测试直接调用）。
 * @param {object} req 请求 @param {object} res 响应
 * @param {string} route 路由种类（boards|workspaces|panel）
 * @param {object} client 控制面客户端
 */
export async function handleSessionBoardRequest(req, res, route, client) {
  try {
    if (route === 'boards') await handleBoardsRead(req, res, client)
    else if (route === 'workspaces') await handleWorkspacesRead(req, res, client)
    else await handlePanelWrite(req, res, client)
  } catch (error) {
    sendControlFailure(res, error)
  }
}

/**
 * 在 webServer 上注册三条会话黑板面板路由（K15）。
 * @param {{register: Function}} webServer `ctx.webServer`
 * @param {{client: object, logger?: object}} options 控制面客户端与日志
 * @returns {() => void} 注销函数（注册失败返回空函数）
 */
export function registerSessionBoardRoute(webServer, options) {
  const { client, logger } = options ?? {}
  if (webServer === undefined || webServer === null || typeof webServer.register !== 'function') return () => {}
  if (client === undefined || client === null) return () => {}
  const disposers = []
  let failed = false

  /** @param {'boards'|'workspaces'|'panel'} route 路由种类 */
  function addRoute(route) {
    const path = SESSION_BOARD_ROUTES[route]
    try {
      const dispose = webServer.register({
        kind: 'exact',
        path,
        handler: (req, res) => handleSessionBoardRequest(req, res, route, client),
      })
      if (typeof dispose === 'function') disposers.push(dispose)
    } catch (error) {
      failed = true
      try {
        logger?.warnOnce?.('session-board-route', `会话黑板路由注册失败（${path}）：${errorMessage(error)}`)
      } catch {
        // 日志失败不影响其余路由。
      }
    }
  }

  addRoute('boards')
  addRoute('workspaces')
  addRoute('panel')

  if (failed && disposers.length === 0) return () => {}
  try {
    logger?.info?.('astrbox-bridge: 会话黑板路由已就绪（/astrbox/boards*）')
  } catch {
    // 日志失败不影响路由。
  }
  return () => {
    for (const dispose of disposers.splice(0)) {
      try {
        dispose()
      } catch {
        // 路由可能已随 webServer fiber 释放：忽略。
      }
    }
  }
}
