/**
 * K21 控制路由：把 `/astrbox/control/*` 代理到本地控制面（Unix socket，K19 契约）。
 *
 * 只做转发与轻量输入校验：方法错误 → 405；JSON/字段非法 → 400；控制面不可达或返回错误 → 500
 * `{ok:false,error}`；成功原样透传控制面 body。控制面写语义全部由 Go 侧实现，本模块不缓存。
 */

/** 控制路由路径（K21 冻结）。 */
export const CONTROL_PATHS = Object.freeze({
  tick: '/astrbox/control/tick',
  clock: '/astrbox/control/clock',
  entities: '/astrbox/control/entities',
  state: '/astrbox/control/state',
  entityDelete: '/astrbox/control/entity-delete',
})

/** 请求体大小上限（控制请求都是小 JSON）。 */
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
 * 读并解析 JSON 请求体；空体返回 `{}`，非法 JSON 返回 INVALID_BODY。
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
 * 方法校验；不匹配时回 405 并返回 false。
 * @param {object} req 请求
 * @param {object} res 响应
 * @param {'GET'|'POST'} method 期望方法
 * @returns {boolean} 是否放行
 */
function allowMethod(req, res, method) {
  if (req.method === method) return true
  sendJson(res, 405, { ok: false, error: { code: 'method_not_allowed', message: `只支持 ${method}` } })
  return false
}

/**
 * 400 错误响应。
 * @param {object} res 响应
 * @param {string} message 原因
 */
function badRequest(res, message) {
  sendJson(res, 400, { ok: false, error: { code: 'invalid_request', message } })
}

/**
 * 把一次转发包成处理器：任何控制面异常都转成 500（控制面不可达/写失败）。
 * @param {(req: object, res: object) => Promise<void>} forward 实际转发
 * @returns {(req: object, res: object) => Promise<void>} 处理器
 */
function guarded(forward) {
  return async (req, res) => {
    try {
      await forward(req, res)
    } catch (error) {
      sendJson(res, 500, {
        ok: false,
        error: { code: error?.code ?? 'control_unavailable', message: errorMessage(error) },
      })
    }
  }
}

/**
 * 注册控制路由（同一个 webServer 注入路径，K21）。
 * @param {{register: Function}} webServer `ctx.webServer` 局部投影
 * @param {{client: object, logger?: object}} options 控制面客户端与日志
 * @returns {() => void} 注销函数（注册失败返回空函数）
 */
export function registerControlRoutes(webServer, options) {
  const { client, logger } = options ?? {}
  if (webServer === undefined || webServer === null || typeof webServer.register !== 'function') return () => {}
  if (client === undefined || client === null) return () => {}
  const disposers = []
  let failed = false

  /** @param {string} path 路由路径 @param {object} handler 处理器 */
  function addRoute(path, handler) {
    try {
      const dispose = webServer.register({ kind: 'exact', path, handler })
      if (typeof dispose === 'function') disposers.push(dispose)
    } catch (error) {
      failed = true
      try {
        logger?.warnOnce?.('control-routes', `控制路由注册失败（${path}）：${errorMessage(error)}`)
      } catch {
        // 日志失败不影响其余路由。
      }
    }
  }

  // POST /astrbox/control/tick：advanceMinutes 透传；409（单飞占用）原样回给面板。
  addRoute(CONTROL_PATHS.tick, guarded(async (req, res) => {
    if (!allowMethod(req, res, 'POST')) return
    const body = await readJsonBody(req)
    if (body === INVALID_BODY) return badRequest(res, '请求体不是合法 JSON')
    const result = await client.postTick({
      scope: '',
      advanceMinutes: Number.isInteger(body.advanceMinutes) ? body.advanceMinutes : 0,
    })
    if (result.busy) {
      sendJson(res, 409, result.body ?? { ok: false, error: { code: 'tick_busy', message: '已有 tick 在飞' } })
      return
    }
    sendJson(res, 200, result.body)
  }))

  // POST /astrbox/control/clock：minutes 或 time 二选一（控制面再校验范围/格式）。
  addRoute(CONTROL_PATHS.clock, guarded(async (req, res) => {
    if (!allowMethod(req, res, 'POST')) return
    const body = await readJsonBody(req)
    if (body === INVALID_BODY) return badRequest(res, '请求体不是合法 JSON')
    const hasMinutes = Number.isInteger(body.minutes)
    const hasTime = typeof body.time === 'string' && body.time !== ''
    if (!hasMinutes && !hasTime) return badRequest(res, '需要 minutes（1..1440）或 time（RFC3339）')
    sendJson(res, 200, await client.setClock(hasMinutes ? { minutes: body.minutes } : { time: body.time }))
  }))

  // GET /astrbox/control/entities：实体目录。
  addRoute(CONTROL_PATHS.entities, guarded(async (req, res) => {
    if (!allowMethod(req, res, 'GET')) return
    sendJson(res, 200, await client.getEntities())
  }))

  // GET /astrbox/control/state?entity=&path=…：路径可重复。
  addRoute(CONTROL_PATHS.state, guarded(async (req, res) => {
    if (req.method === 'POST') {
      const body = await readJsonBody(req)
      if (body === INVALID_BODY) return badRequest(res, '请求体不是合法 JSON')
      if (typeof body.entity !== 'string' || body.entity.trim() === '') return badRequest(res, 'entity 必须是非空字符串')
      const validValues = body.values !== null && typeof body.values === 'object' && !Array.isArray(body.values)
      const validPair = typeof body.path === 'string' && body.path.trim() !== '' && body.value !== undefined
      if (!validValues && !validPair) return badRequest(res, '需要 entity +（path/value 或 values）')
      const payload = { entity: body.entity.trim() }
      if (validValues) payload.values = body.values
      else {
        payload.path = body.path.trim()
        payload.value = body.value
      }
      sendJson(res, 200, await client.setState(payload))
      return
    }
    if (!allowMethod(req, res, 'GET')) return
    const url = new URL(typeof req.url === 'string' && req.url !== '' ? req.url : '/', 'http://localhost')
    const entity = url.searchParams.get('entity')
    if (entity === null || entity.trim() === '') return badRequest(res, '缺少 entity 查询参数')
    sendJson(res, 200, await client.getState({ entity, paths: url.searchParams.getAll('path') }))
  }))

  // POST /astrbox/control/entity-delete：删除实体。
  addRoute(CONTROL_PATHS.entityDelete, guarded(async (req, res) => {
    if (!allowMethod(req, res, 'POST')) return
    const body = await readJsonBody(req)
    if (body === INVALID_BODY) return badRequest(res, '请求体不是合法 JSON')
    if (typeof body.entity !== 'string' || body.entity.trim() === '') return badRequest(res, 'entity 必须是非空字符串')
    sendJson(res, 200, await client.deleteEntity(body.entity.trim()))
  }))

  if (failed && disposers.length === 0) return () => {}
  try {
    logger?.info?.('astrbox-bridge: 控制路由已就绪（/astrbox/control/*）')
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
