/**
 * K15 会话黑板面板路由：三条 exact 路由的注册/注销、读路由透传、工作区列表、
 * panel 写转发体、参数/方法/JSON 错误（400/405）、控制面不可达（500）与 4xx 信封透传。
 * 用假 webServer/假 req/res 的单元用例 + 临时目录 UDS 伪控制面（`testHarness`）的联调用例。
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { ControlError, createControlClient } from './controlClient.mjs'
import {
  SESSION_BOARD_ROUTES,
  handleSessionBoardRequest,
  registerSessionBoardRoute,
} from './sessionBoardRoute.mjs'
import { startControlServer, writeJson } from './testHarness.mjs'

/** 假响应：记录状态码、响应头与解析后的 JSON 体。 */
function fakeResponse() {
  return {
    statusCode: 0,
    headers: {},
    body: undefined,
    writeHead(status, headers) {
      this.statusCode = status
      this.headers = headers
    },
    end(text) {
      this.body = text === undefined ? undefined : JSON.parse(text)
    },
  }
}

/** 假 webServer：记录注册的路由并提供注销。 */
function fakeWebServer() {
  const routes = []
  return {
    routes,
    register(route) {
      routes.push(route)
      return () => {
        route.disposed = true
      }
    },
  }
}

/** 假请求：POST 体按 async iterable 提供（与真实 IncomingMessage 同形状）。 */
function fakePost(body) {
  const text = typeof body === 'string' ? body : JSON.stringify(body)
  return {
    method: 'POST',
    async *[Symbol.asyncIterator]() {
      yield Buffer.from(text, 'utf8')
    },
  }
}

/** 假控制面客户端：记录调用参数；可在 overrides 里替换返回值或抛错。 */
function fakeClient(overrides = {}) {
  const calls = []
  return {
    calls,
    async readBoard(query) {
      calls.push({ method: 'readBoard', query })
      return overrides.readBoard ? overrides.readBoard(query) : { ok: true, board: { scope: query.scope, version: 1, regions: [] } }
    },
    async listBoardWorkspaces() {
      calls.push({ method: 'listBoardWorkspaces' })
      return overrides.listBoardWorkspaces ? overrides.listBoardWorkspaces() : { ok: true, workspaces: [] }
    },
    async writePanelBoard(body) {
      calls.push({ method: 'writePanelBoard', body })
      return overrides.writePanelBoard ? overrides.writePanelBoard(body) : { ok: true, changed: true, version: 2 }
    },
  }
}

/** 取一条已注册路由的处理器。 */
function handlerOf(webServer, path) {
  const route = webServer.routes.find((item) => item.path === path)
  assert.ok(route !== undefined, `缺少路由 ${path}`)
  return route.handler
}

/** 用假客户端注册三条路由，返回 {webServer, client}。 */
function registered(overrides = {}) {
  const client = fakeClient(overrides)
  const webServer = fakeWebServer()
  registerSessionBoardRoute(webServer, { client })
  return { client, webServer }
}

test('注册：三条 exact 路由与冻结路径（K15）', () => {
  const webServer = fakeWebServer()
  const logger = { infos: [], info(message) { this.infos.push(message) } }
  const dispose = registerSessionBoardRoute(webServer, { client: fakeClient(), logger })
  assert.deepEqual(
    webServer.routes.map((route) => route.path).sort(),
    ['/astrbox/boards', '/astrbox/boards/panel', '/astrbox/boards/workspaces'],
  )
  assert.equal(webServer.routes.every((route) => route.kind === 'exact'), true)
  assert.equal(SESSION_BOARD_ROUTES.boards, '/astrbox/boards')
  assert.equal(SESSION_BOARD_ROUTES.workspaces, '/astrbox/boards/workspaces')
  assert.equal(SESSION_BOARD_ROUTES.panel, '/astrbox/boards/panel')
  assert.ok(logger.infos.some((message) => message.includes('/astrbox/boards')))
  dispose()
  assert.equal(webServer.routes.every((route) => route.disposed === true), true)
})

test('缺 webServer/client：返回空函数、不注册、不抛错', () => {
  assert.equal(typeof registerSessionBoardRoute(undefined, { client: fakeClient() }), 'function')
  assert.equal(typeof registerSessionBoardRoute({}, { client: fakeClient() }), 'function')
  const webServer = fakeWebServer()
  assert.equal(typeof registerSessionBoardRoute(webServer, {}), 'function')
  assert.equal(typeof registerSessionBoardRoute(webServer, { client: null }), 'function')
  assert.equal(webServer.routes.length, 0)
})

test('GET /astrbox/boards：scope=global 透传控制面 body 与状态码', async () => {
  const board = { scope: 'global', version: 3, regions: [] }
  const { client, webServer } = registered({ readBoard: () => ({ ok: true, board }) })
  const res = fakeResponse()
  await handlerOf(webServer, SESSION_BOARD_ROUTES.boards)({ method: 'GET', url: '/astrbox/boards?scope=global' }, res)
  assert.equal(res.statusCode, 200)
  assert.equal(res.headers['cache-control'], 'no-store')
  assert.deepEqual(res.body, { ok: true, board })
  assert.deepEqual(client.calls, [{ method: 'readBoard', query: { scope: 'global' } }])
})

test('GET /astrbox/boards：workspace 板带 workspace 与 since', async () => {
  const { client, webServer } = registered()
  const res = fakeResponse()
  await handlerOf(webServer, SESSION_BOARD_ROUTES.boards)(
    { method: 'GET', url: '/astrbox/boards?scope=workspace&workspace=%2Fhome%2Fneko&since=7' },
    res,
  )
  assert.equal(res.statusCode, 200)
  assert.deepEqual(client.calls[0].query, { scope: 'workspace', workspace: '/home/neko', since: 7 })
})

test('GET /astrbox/boards：非法参数 400 bad_request 且不发请求', async () => {
  const urls = [
    '/astrbox/boards',
    '/astrbox/boards?scope=bad',
    '/astrbox/boards?scope=workspace',
    '/astrbox/boards?scope=workspace&workspace=%20',
    '/astrbox/boards?scope=global&since=abc',
    '/astrbox/boards?scope=global&since=-1',
    '/astrbox/boards?scope=global&since=1.5',
  ]
  for (const url of urls) {
    const { client, webServer } = registered()
    const res = fakeResponse()
    await handlerOf(webServer, SESSION_BOARD_ROUTES.boards)({ method: 'GET', url }, res)
    assert.equal(res.statusCode, 400, url)
    assert.equal(res.body.ok, false)
    assert.equal(res.body.error.code, 'bad_request')
    assert.equal(client.calls.length, 0, url)
  }
})

test('GET /astrbox/boards/workspaces：透传工作区列表（K14）', async () => {
  const workspaces = [{ workspace: '/home/neko', version: 4, regions: 2, updated_at: '2026-11-10T15:00:00+08:00' }]
  const { client, webServer } = registered({ listBoardWorkspaces: () => ({ ok: true, workspaces }) })
  const res = fakeResponse()
  await handlerOf(webServer, SESSION_BOARD_ROUTES.workspaces)({ method: 'GET' }, res)
  assert.equal(res.statusCode, 200)
  assert.deepEqual(res.body, { ok: true, workspaces })
  assert.deepEqual(client.calls, [{ method: 'listBoardWorkspaces' }])
})

test('方法错误：三条路由都回 405', async () => {
  const { webServer } = registered()
  const cases = [
    [SESSION_BOARD_ROUTES.boards, { method: 'POST' }],
    [SESSION_BOARD_ROUTES.workspaces, { method: 'POST' }],
    [SESSION_BOARD_ROUTES.panel, { method: 'GET' }],
  ]
  for (const [path, request] of cases) {
    const res = fakeResponse()
    await handlerOf(webServer, path)(request, res)
    assert.equal(res.statusCode, 405, path)
    assert.equal(res.body.error.code, 'method_not_allowed')
  }
})

test('POST /astrbox/boards/panel：转发体原样（scope/workspace/region/ops）并透传结果', async () => {
  const { client, webServer } = registered({ writePanelBoard: () => ({ ok: true, changed: true, version: 9, updated_at: '2026-11-10T15:01:00+08:00', trimmed: [], evicted: [] }) })
  const payload = { scope: 'global', workspace: '/home/neko', region: '/home/neko', ops: [{ op: 'set', key: '项目.状态', value: '进行中' }] }
  const res = fakeResponse()
  await handlerOf(webServer, SESSION_BOARD_ROUTES.panel)(fakePost(payload), res)
  assert.equal(res.statusCode, 200)
  assert.equal(res.body.ok, true)
  assert.equal(res.body.version, 9)
  assert.deepEqual(client.calls, [{ method: 'writePanelBoard', body: payload }])
})

test('POST /astrbox/boards/panel：非法 JSON/字段 400 且不发请求', async () => {
  const bodies = [
    '{bad json',
    '[]',
    { ops: [{ op: 'clear' }] },
    { scope: 'bad', workspace: '/w', ops: [{ op: 'clear' }] },
    { scope: 'global', ops: [{ op: 'clear' }] },
    { scope: 'workspace', workspace: '/w', ops: [{ op: 'clear' }] },
    { scope: 'workspace', workspace: '/w', region: 'r', ops: [] },
    { scope: 'workspace', workspace: '/w', region: 'r', ops: [null] },
    { scope: 'global', workspace: '/w', region: 'other', ops: [{ op: 'clear' }] },
  ]
  for (const body of bodies) {
    const { client, webServer } = registered()
    const res = fakeResponse()
    await handlerOf(webServer, SESSION_BOARD_ROUTES.panel)(fakePost(body), res)
    assert.equal(res.statusCode, 400, JSON.stringify(body))
    assert.equal(res.body.error.code, 'bad_request')
    assert.equal(client.calls.length, 0)
  }
})

test('控制面传输错误：500 control_unavailable', async () => {
  const { webServer } = registered({
    readBoard() {
      throw new ControlError('network_error', 'connect ENOENT')
    },
  })
  const res = fakeResponse()
  await handlerOf(webServer, SESSION_BOARD_ROUTES.boards)({ method: 'GET', url: '/astrbox/boards?scope=global' }, res)
  assert.equal(res.statusCode, 500)
  assert.equal(res.body.ok, false)
  assert.equal(res.body.error.code, 'control_unavailable')
  assert.ok(res.body.error.message.includes('ENOENT'))
})

test('控制面 4xx：按原状态透传控制面信封', async () => {
  const envelope = { ok: false, error: { code: 'forbidden', message: '无权写该区域' } }
  const { webServer } = registered({
    writePanelBoard() {
      const error = new ControlError('forbidden', 'forbidden: 无权写该区域')
      error.status = 403
      error.body = envelope
      throw error
    },
  })
  const res = fakeResponse()
  await handlerOf(webServer, SESSION_BOARD_ROUTES.panel)(
    fakePost({ scope: 'global', workspace: '/w', region: '/w', ops: [{ op: 'clear' }] }),
    res,
  )
  assert.equal(res.statusCode, 403)
  assert.deepEqual(res.body, envelope)
})

test('注册抛错：warnOnce 且返回空函数、不抛错', () => {
  const warnings = []
  const webServer = { register() { throw new Error('boom') } }
  const logger = { warnOnce: (key, message) => warnings.push([key, message]), info() {} }
  const dispose = registerSessionBoardRoute(webServer, { client: fakeClient(), logger })
  assert.equal(typeof dispose, 'function')
  assert.equal(warnings.length, 3)
  assert.ok(warnings.every(([key]) => key === 'session-board-route'))
  dispose()
})

test('handleSessionBoardRequest 直接调用也可用（未知路由按 panel 兜底且方法校验先行）', async () => {
  const client = fakeClient()
  const res = fakeResponse()
  await handleSessionBoardRequest({ method: 'PUT' }, res, 'panel', client)
  assert.equal(res.statusCode, 405)
  assert.equal(client.calls.length, 0)
})

test('伪控制面联调：读透传 / workspaces / panel 写转发体 / 4xx 透传（真实 UDS + controlClient）', async (t) => {
  const server = await startControlServer((req, res, record) => {
    const url = new URL(record.url, 'http://localhost')
    if (url.pathname === '/boards/workspaces') {
      writeJson(res, 200, { ok: true, workspaces: [{ workspace: '/w1', version: 2, regions: 1, updated_at: '2026-11-10T15:00:00+08:00' }] })
      return
    }
    if (url.pathname === '/boards' && req.method === 'GET') {
      writeJson(res, 200, { ok: true, board: { scope: url.searchParams.get('scope'), version: 2, regions: [] } })
      return
    }
    if (url.pathname === '/boards/panel' && req.method === 'POST') {
      if (record.body?.region === 'reject') {
        writeJson(res, 400, { ok: false, error: { code: 'invalid_region', message: 'bad region' } })
        return
      }
      writeJson(res, 200, { ok: true, changed: true, version: 3, updated_at: '2026-11-10T15:02:00+08:00' })
      return
    }
    writeJson(res, 404, { ok: false, error: { code: 'not_found', message: 'unknown' } })
  })
  t.after(() => server.close())
  const client = createControlClient({ socketPath: server.socketPath, requestTimeoutMs: 2000 })
  t.after(() => client.dispose())
  const webServer = fakeWebServer()
  registerSessionBoardRoute(webServer, { client })
  const boards = handlerOf(webServer, SESSION_BOARD_ROUTES.boards)
  const workspaces = handlerOf(webServer, SESSION_BOARD_ROUTES.workspaces)
  const panel = handlerOf(webServer, SESSION_BOARD_ROUTES.panel)

  const read = fakeResponse()
  await boards({ method: 'GET', url: '/astrbox/boards?scope=global&since=1' }, read)
  assert.equal(read.statusCode, 200)
  assert.equal(read.body.board.scope, 'global')
  assert.equal(server.requests[0].method, 'GET')
  assert.equal(server.requests[0].url, '/boards?scope=global&since=1')

  const list = fakeResponse()
  await workspaces({ method: 'GET' }, list)
  assert.equal(list.statusCode, 200)
  assert.equal(list.body.workspaces[0].workspace, '/w1')
  assert.equal(server.requests[1].url, '/boards/workspaces')

  const payload = { scope: 'global', workspace: '/w1', region: '/w1', ops: [{ op: 'set', key: '项目.状态', value: '进行中' }] }
  const write = fakeResponse()
  await panel(fakePost(payload), write)
  assert.equal(write.statusCode, 200)
  assert.equal(write.body.ok, true)
  assert.deepEqual(server.requests[2].body, payload)

  const rejected = fakeResponse()
  await panel(fakePost({ scope: 'workspace', workspace: '/w1', region: 'reject', ops: [{ op: 'clear' }] }), rejected)
  assert.equal(rejected.statusCode, 400)
  assert.deepEqual(rejected.body, { ok: false, error: { code: 'invalid_region', message: 'bad region' } })
})

test('控制面不可达：真实 UDS 缺 socket → 500 control_unavailable', async (t) => {
  const socketPath = join(tmpdir(), `neko-missing-board-${process.pid}-${Date.now()}.sock`)
  const client = createControlClient({ socketPath, requestTimeoutMs: 1000 })
  t.after(() => client.dispose())
  const webServer = fakeWebServer()
  registerSessionBoardRoute(webServer, { client })
  const res = fakeResponse()
  await handlerOf(webServer, SESSION_BOARD_ROUTES.workspaces)({ method: 'GET' }, res)
  assert.equal(res.statusCode, 500)
  assert.equal(res.body.error.code, 'control_unavailable')
})
