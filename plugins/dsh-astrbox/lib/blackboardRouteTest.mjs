/**
 * K7 路由：`GET /astrbox/blackboard` 的注册、全部黑板 / 单实体两种响应、方法错误、
 * 同步与异步数据源、数据源异常。
 * 用假 webServer 与假 req/res，不启动 HTTP 服务。
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { BLACKBOARD_PATH, handleBlackboardRequest, registerBlackboardRoute } from './blackboardRoute.mjs'

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

const BOARD = { entity: '梦梦', version: 12, updated_at: '2026-11-06T16:20:00+08:00', entries: [] }

test('注册：exact 路由与冻结路径（K7）', () => {
  const webServer = fakeWebServer()
  const logger = { info: (message) => logger.infos.push(message), infos: [] }
  const dispose = registerBlackboardRoute(webServer, { getBoards: () => [], logger })
  assert.equal(webServer.routes.length, 1)
  assert.equal(webServer.routes[0].kind, 'exact')
  assert.equal(webServer.routes[0].path, BLACKBOARD_PATH)
  assert.equal(BLACKBOARD_PATH, '/astrbox/blackboard')
  assert.ok(logger.infos.some((message) => message.includes(BLACKBOARD_PATH)))
  dispose()
  assert.equal(webServer.routes[0].disposed, true)
})

test('无参数：{ok:true, boards:[{entity, board}]}', async () => {
  const webServer = fakeWebServer()
  registerBlackboardRoute(webServer, { getBoards: () => [{ entity: '梦梦', board: BOARD }, { entity: '猫猫', board: null }] })
  const res = fakeResponse()
  await webServer.routes[0].handler({ method: 'GET', url: '/astrbox/blackboard' }, res)
  assert.equal(res.statusCode, 200)
  assert.equal(res.headers['cache-control'], 'no-store')
  assert.deepEqual(res.body, { ok: true, boards: [{ entity: '梦梦', board: BOARD }, { entity: '猫猫', board: null }] })
})

test('数据源可为异步函数：getBoards / getMeta 都 await 后再写响应（v2 供数）', async () => {
  const webServer = fakeWebServer()
  registerBlackboardRoute(webServer, {
    getBoards: async () => [{ entity: '猫猫', board: BOARD }],
    getMeta: async () => ({ primary: '猫猫', paths: ['状态.身体'] }),
  })
  const res = fakeResponse()
  await webServer.routes[0].handler({ method: 'GET', url: '/astrbox/blackboard' }, res)
  assert.deepEqual(res.body, {
    ok: true,
    primary: '猫猫',
    paths: ['状态.身体'],
    boards: [{ entity: '猫猫', board: BOARD }],
  })
})

test('K7b getMeta：无参数响应带 primary/paths（快照条元数据），黑板字段不变', async () => {
  const webServer = fakeWebServer()
  registerBlackboardRoute(webServer, {
    getBoards: () => [{ entity: '猫猫', board: BOARD }],
    getMeta: () => ({ primary: ' 猫猫 ', paths: ['状态.情绪', ' 身体.体力 ', '', 42] }),
  })
  const res = fakeResponse()
  await webServer.routes[0].handler({ method: 'GET', url: '/astrbox/blackboard' }, res)
  assert.equal(res.statusCode, 200)
  assert.deepEqual(res.body, {
    ok: true,
    primary: '猫猫',
    paths: ['状态.情绪', '身体.体力'],
    boards: [{ entity: '猫猫', board: BOARD }],
  })
})

test('K7b getMeta 异常/非法形状/缺省 → 响应不带元数据字段，黑板照常', async () => {
  const cases = [
    { getMeta: undefined },
    { getMeta: () => undefined },
    { getMeta: () => null },
    { getMeta: () => 'bad' },
    { getMeta: () => ({ primary: '', paths: 'bad' }) },
    {
      getMeta: () => {
        throw new Error('boom')
      },
    },
    { getMeta: async () => { throw new Error('async boom') } },
  ]
  for (const { getMeta } of cases) {
    const webServer = fakeWebServer()
    registerBlackboardRoute(webServer, { getBoards: () => [{ entity: '猫猫', board: BOARD }], getMeta })
    const res = fakeResponse()
    await webServer.routes[0].handler({ method: 'GET', url: '/astrbox/blackboard' }, res)
    assert.equal(res.statusCode, 200)
    assert.deepEqual(res.body, { ok: true, boards: [{ entity: '猫猫', board: BOARD }] })
  }
})

test('K7b paths 截断到 32 条（异常配置不撑大响应）', async () => {
  const webServer = fakeWebServer()
  const many = Array.from({ length: 40 }, (_, index) => `字段.${index}`)
  registerBlackboardRoute(webServer, { getBoards: () => [], getMeta: () => ({ primary: '猫猫', paths: many }) })
  const res = fakeResponse()
  await webServer.routes[0].handler({ method: 'GET', url: '/astrbox/blackboard' }, res)
  assert.equal(res.body.paths.length, 32)
})

test('?entity=：单块返回；未知实体返回 board:null（不返回 404）', async () => {
  const webServer = fakeWebServer()
  registerBlackboardRoute(webServer, { getBoards: async () => [{ entity: '梦梦', board: BOARD }] })
  const route = webServer.routes[0]

  const hit = fakeResponse()
  await route.handler({ method: 'GET', url: '/astrbox/blackboard?entity=%E6%A2%A6%E6%A2%A6' }, hit)
  assert.equal(hit.statusCode, 200)
  assert.deepEqual(hit.body, { ok: true, entity: '梦梦', board: BOARD })

  const miss = fakeResponse()
  await route.handler({ method: 'GET', url: '/astrbox/blackboard?entity=unknown' }, miss)
  assert.deepEqual(miss.body, { ok: true, entity: 'unknown', board: null })

  const empty = fakeResponse()
  await route.handler({ method: 'GET', url: '/astrbox/blackboard?entity=' }, empty)
  assert.equal(empty.body.ok, true)
  assert.ok(Array.isArray(empty.body.boards))
})

test('非 GET/HEAD → 405；HEAD 放行', async () => {
  const res = fakeResponse()
  await handleBlackboardRequest({ method: 'POST', url: '/astrbox/blackboard' }, res, () => [])
  assert.equal(res.statusCode, 405)
  assert.equal(res.body.ok, false)
  assert.equal(res.body.error.code, 'method_not_allowed')

  const head = fakeResponse()
  await handleBlackboardRequest({ method: 'HEAD', url: '/astrbox/blackboard' }, head, () => [])
  assert.equal(head.statusCode, 200)
})

test('数据源异常 → 500；非数组 → 空列表；非法 URL 按无参数处理', async () => {
  const failed = fakeResponse()
  await handleBlackboardRequest(
    { method: 'GET', url: '/astrbox/blackboard' },
    failed,
    () => {
      throw new Error('boom')
    },
  )
  assert.equal(failed.statusCode, 500)
  assert.equal(failed.body.ok, false)

  const asyncFailed = fakeResponse()
  await handleBlackboardRequest({ method: 'GET', url: '/astrbox/blackboard' }, asyncFailed, async () => {
    throw new Error('async boom')
  })
  assert.equal(asyncFailed.statusCode, 500)

  const notArray = fakeResponse()
  await handleBlackboardRequest({ method: 'GET', url: '/astrbox/blackboard' }, notArray, () => ({ bad: true }))
  assert.deepEqual(notArray.body, { ok: true, boards: [] })

  const malformed = fakeResponse()
  await handleBlackboardRequest({ method: 'GET', url: 'http://[bad' }, malformed, () => [{ entity: '梦梦', board: null }])
  assert.equal(malformed.statusCode, 200)
  assert.ok(Array.isArray(malformed.body.boards))
})

test('webServer 缺失/形状不对：跳过注册且不抛错', () => {
  for (const webServer of [undefined, null, {}, { register: 'nope' }]) {
    const dispose = registerBlackboardRoute(webServer, { getBoards: () => [] })
    assert.equal(typeof dispose, 'function')
    assert.doesNotThrow(() => dispose())
  }
})

test('register 抛错：记录 warnOnce 并返回空注销函数', () => {
  const messages = []
  const logger = {
    warnOnce: (key, message) => messages.push([key, message]),
    info: () => {},
  }
  const webServer = {
    register() {
      throw new Error('route conflict')
    },
  }
  const dispose = registerBlackboardRoute(webServer, { getBoards: () => [], logger })
  assert.equal(typeof dispose, 'function')
  assert.doesNotThrow(() => dispose())
  assert.equal(messages.length, 1)
  assert.ok(messages[0][1].includes('route conflict'))
})
