/**
 * K21 控制路由：/astrbox/control/* 的转发体、方法/输入校验、控制面失败映射与注销。
 * 用假 webServer + 假控制面客户端，不启动 HTTP 服务。
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { CONTROL_PATHS, registerControlRoutes } from './controlRoutes.mjs'

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

/**
 * 假请求：可异步迭代的 body（route 用 for await 读）。
 * @param {{method?: string, url?: string, body?: unknown}} input 请求
 * @returns {object} 假 req
 */
function fakeRequest(input = {}) {
  const text = input.body === undefined ? undefined : typeof input.body === 'string' ? input.body : JSON.stringify(input.body)
  const chunks = text === undefined ? [] : [Buffer.from(text, 'utf8')]
  return {
    method: input.method ?? 'GET',
    url: input.url ?? '/',
    async *[Symbol.asyncIterator]() {
      for (const chunk of chunks) yield chunk
    },
  }
}

/** 假 webServer：记录路由并支持注销。 */
function fakeWebServer(options = {}) {
  const routes = []
  return {
    routes,
    register(route) {
      if (options.failPath === route.path) throw new Error('route conflict')
      routes.push(route)
      return () => {
        route.disposed = true
      }
    },
  }
}

/**
 * 假控制面客户端：记录调用并返回固定/可覆盖结果。
 * @param {object} [overrides] 覆盖方法或返回值
 * @returns {object} 客户端与调用记录
 */
function fakeClient(overrides = {}) {
  const calls = []
  const client = {
    calls,
    async postTick(request) {
      calls.push(['tick', request])
      return overrides.tick ?? { status: 200, busy: false, body: { ok: true, tickId: 't1' } }
    },
    async setClock(body) {
      calls.push(['clock', body])
      return { ok: true, worldTime: '2026-11-07T17:34:00+08:00', month: 11, phase: '傍晚' }
    },
    async getEntities() {
      calls.push(['entities'])
      return { ok: true, entities: [{ id: '猫猫', type: 'NPC' }, { id: '场景', type: '场景' }] }
    },
    async getState(query) {
      calls.push(['getState', query])
      return { ok: true, entity: query.entity, values: {}, missing: [], bytes: 0 }
    },
    async setState(body) {
      calls.push(['setState', body])
      return { ok: true, entity: body.entity, paths: [body.path ?? Object.keys(body.values ?? {}).join(',')] }
    },
    async deleteEntity(entity) {
      calls.push(['deleteEntity', entity])
      return { ok: true, deleted: entity }
    },
  }
  for (const [key, value] of Object.entries(overrides)) {
    if (typeof value === 'function') client[key] = value
  }
  return client
}

/**
 * 注册路由并返回按 path 查找与调用的小工具。
 * @param {object} client 假客户端
 * @param {object} [webServer] 假 webServer
 * @returns {{routes: Array<object>, call: Function, dispose: Function}} 注册结果
 */
function setup(client, webServer = fakeWebServer()) {
  const dispose = registerControlRoutes(webServer, { client })
  return {
    routes: webServer.routes,
    dispose,
    call: async (path, input) => {
      const route = webServer.routes.find((item) => item.path === path)
      assert.ok(route, `路由不存在：${path}`)
      const res = fakeResponse()
      await route.handler(fakeRequest(input), res)
      return res
    },
  }
}

test('注册 5 条 exact 控制路由（6 个端点，state 的 GET/POST 共用路径，K21）', () => {
  const client = fakeClient()
  const { routes, dispose } = setup(client)
  assert.deepEqual(routes.map((route) => route.path).sort(), Object.values(CONTROL_PATHS).sort())
  assert.equal(Object.keys(CONTROL_PATHS).length, 5)
  for (const route of routes) assert.equal(route.kind, 'exact')
  dispose()
  for (const route of routes) assert.equal(route.disposed, true)
})

test('tick：POST 转发 advanceMinutes；409 原样回传', async () => {
  const client = fakeClient()
  const { call } = setup(client)
  const ok = await call(CONTROL_PATHS.tick, { method: 'POST', body: { advanceMinutes: 15 } })
  assert.equal(ok.statusCode, 200)
  assert.deepEqual(ok.body, { ok: true, tickId: 't1' })
  assert.deepEqual(client.calls.at(-1), ['tick', { scope: '', advanceMinutes: 15 }])

  const empty = await call(CONTROL_PATHS.tick, { method: 'POST' })
  assert.equal(empty.body.ok, true)
  assert.deepEqual(client.calls.at(-1), ['tick', { scope: '', advanceMinutes: 0 }])

  const busyClient = fakeClient({ tick: { status: 409, busy: true, body: { ok: false, error: { code: 'tick_busy', message: 'busy' } } } })
  const busySetup = setup(busyClient)
  const busyRes = await busySetup.call(CONTROL_PATHS.tick, { method: 'POST' })
  assert.equal(busyRes.statusCode, 409)
  assert.equal(busyRes.body.error.code, 'tick_busy')

  const wrong = await call(CONTROL_PATHS.tick, { method: 'GET' })
  assert.equal(wrong.statusCode, 405)
  assert.equal(wrong.body.error.code, 'method_not_allowed')
})

test('clock：minutes/time 二选一；缺参数 400；非法 JSON 400', async () => {
  const client = fakeClient()
  const { call } = setup(client)

  const byMinutes = await call(CONTROL_PATHS.clock, { method: 'POST', body: { minutes: 60 } })
  assert.equal(byMinutes.statusCode, 200)
  assert.equal(byMinutes.body.ok, true)
  assert.deepEqual(client.calls.at(-1), ['clock', { minutes: 60 }])

  const byTime = await call(CONTROL_PATHS.clock, { method: 'POST', body: { time: '2026-11-07T18:00:00+08:00' } })
  assert.equal(byTime.statusCode, 200)
  assert.deepEqual(client.calls.at(-1), ['clock', { time: '2026-11-07T18:00:00+08:00' }])

  const missing = await call(CONTROL_PATHS.clock, { method: 'POST', body: {} })
  assert.equal(missing.statusCode, 400)
  assert.equal(missing.body.error.code, 'invalid_request')

  const invalid = await call(CONTROL_PATHS.clock, { method: 'POST', body: 'not json' })
  assert.equal(invalid.statusCode, 400)
})

test('entities：GET 透传；POST 405', async () => {
  const client = fakeClient()
  const { call } = setup(client)
  const res = await call(CONTROL_PATHS.entities, { method: 'GET' })
  assert.equal(res.statusCode, 200)
  assert.deepEqual(client.calls.at(-1), ['entities'])
  assert.equal(res.body.entities.length, 2)
  assert.equal((await call(CONTROL_PATHS.entities, { method: 'POST', body: {} })).statusCode, 405)
})

test('state：GET entity+重复 path；POST path/value 与 values；缺 entity 400', async () => {
  const client = fakeClient()
  const { call } = setup(client)

  const got = await call(CONTROL_PATHS.state, {
    method: 'GET',
    url: '/astrbox/control/state?entity=%E7%8C%AB%E7%8C%AB&path=%E7%8A%B6%E6%80%81.%E6%83%85%E7%BB%AA&path=%E8%BA%AB%E4%BD%93.%E4%BD%93%E5%8A%9B',
  })
  assert.equal(got.statusCode, 200)
  assert.deepEqual(client.calls.at(-1), ['getState', { entity: '猫猫', paths: ['状态.情绪', '身体.体力'] }])

  const noEntity = await call(CONTROL_PATHS.state, { method: 'GET', url: '/astrbox/control/state' })
  assert.equal(noEntity.statusCode, 400)

  const wrote = await call(CONTROL_PATHS.state, {
    method: 'POST',
    body: { entity: '猫猫', path: '状态.情绪', value: '开心' },
  })
  assert.equal(wrote.statusCode, 200)
  assert.deepEqual(client.calls.at(-1), ['setState', { entity: '猫猫', path: '状态.情绪', value: '开心' }])

  const wroteValues = await call(CONTROL_PATHS.state, {
    method: 'POST',
    body: { entity: ' 猫猫 ', values: { '状态.情绪': '平静' } },
  })
  assert.equal(wroteValues.statusCode, 200)
  assert.deepEqual(client.calls.at(-1), ['setState', { entity: '猫猫', values: { '状态.情绪': '平静' } }])

  const missing = await call(CONTROL_PATHS.state, { method: 'POST', body: { entity: '猫猫' } })
  assert.equal(missing.statusCode, 400)
  const badJson = await call(CONTROL_PATHS.state, { method: 'POST', body: '{' })
  assert.equal(badJson.statusCode, 400)
  const noEntityPost = await call(CONTROL_PATHS.state, { method: 'POST', body: { path: 'a', value: 1 } })
  assert.equal(noEntityPost.statusCode, 400)
})

test('entity-delete：POST 转发并透传；缺 entity 400；DELETE 405', async () => {
  const client = fakeClient()
  const { call } = setup(client)
  const res = await call(CONTROL_PATHS.entityDelete, { method: 'POST', body: { entity: '猫猫' } })
  assert.equal(res.statusCode, 200)
  assert.deepEqual(res.body, { ok: true, deleted: '猫猫' })
  assert.deepEqual(client.calls.at(-1), ['deleteEntity', '猫猫'])
  assert.equal((await call(CONTROL_PATHS.entityDelete, { method: 'POST', body: {} })).statusCode, 400)
  assert.equal((await call(CONTROL_PATHS.entityDelete, { method: 'DELETE' })).statusCode, 405)
})

test('控制面失败 → 500 {ok:false, error{code,message}}', async () => {
  class FakeControlError extends Error {
    constructor(code, message) {
      super(`${code}: ${message}`)
      this.code = code
    }
  }
  const client = fakeClient({
    postTick: async () => {
      throw new FakeControlError('network_error', 'connect ENOENT')
    },
    setState: async () => {
      throw new FakeControlError('not_found', 'entity not found')
    },
  })
  const { call } = setup(client)
  const tick = await call(CONTROL_PATHS.tick, { method: 'POST' })
  assert.equal(tick.statusCode, 500)
  assert.equal(tick.body.ok, false)
  assert.equal(tick.body.error.code, 'network_error')
  assert.ok(tick.body.error.message.includes('ENOENT'))

  const write = await call(CONTROL_PATHS.state, { method: 'POST', body: { entity: '猫猫', path: 'a', value: 1 } })
  assert.equal(write.statusCode, 500)
  assert.equal(write.body.error.code, 'not_found')
})

test('webServer 缺失/单条注册失败：可安全调用与注销', () => {
  for (const webServer of [undefined, null, {}, { register: 'nope' }]) {
    const dispose = registerControlRoutes(webServer, { client: fakeClient() })
    assert.equal(typeof dispose, 'function')
    assert.doesNotThrow(() => dispose())
  }
  const messages = []
  const logger = { warnOnce: (key, message) => messages.push([key, message]), info: () => {} }
  const failing = fakeWebServer({ failPath: CONTROL_PATHS.clock })
  const dispose = registerControlRoutes(failing, { client: fakeClient(), logger })
  assert.equal(failing.routes.length, 4, '其余路由仍注册')
  assert.equal(messages.length, 1)
  assert.ok(messages[0][1].includes('/astrbox/control/clock'))
  dispose()
  for (const route of failing.routes) assert.equal(route.disposed, true)
})
