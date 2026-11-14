/**
 * K28 路由：`/astrbox/main-session-prompts` 的注册、GET 快照、POST 写入/清除、
 * 方法错误、非法 JSON/字段与存储异常。用假 webServer 与假 req/res，不启动 HTTP 服务。
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { mkdtempSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { MAIN_SESSION_PROMPT_MAX_CHARS, createMainSessionPromptStore } from './mainSessionPromptStore.mjs'
import { MAIN_SESSION_PROMPT_PATH, handleMainSessionPromptRequest, registerMainSessionPromptRoute } from './mainSessionPromptRoute.mjs'

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

/** 内存存储替身（隔离真实文件）。 */
function memoryStore(initial = {}) {
  const prompts = new Map(Object.entries(initial))
  return {
    prompts,
    list: () => Object.fromEntries(prompts),
    get: (entity) => prompts.get(entity) ?? '',
    set(entity, text) {
      if (text === '') prompts.delete(entity)
      else prompts.set(entity, text)
      return true
    },
    remove(entity) {
      prompts.delete(entity)
      return true
    },
  }
}

test('注册：exact 路由与冻结路径（K28）', () => {
  const webServer = fakeWebServer()
  const logger = { info: (message) => logger.infos.push(message), infos: [] }
  const dispose = registerMainSessionPromptRoute(webServer, { store: memoryStore(), logger })
  assert.equal(webServer.routes.length, 1)
  assert.equal(webServer.routes[0].kind, 'exact')
  assert.equal(webServer.routes[0].path, MAIN_SESSION_PROMPT_PATH)
  assert.equal(MAIN_SESSION_PROMPT_PATH, '/astrbox/main-session-prompts')
  assert.ok(logger.infos.some((message) => message.includes(MAIN_SESSION_PROMPT_PATH)))
  dispose()
  assert.equal(webServer.routes[0].disposed, true)
})

test('缺 webServer/store：注册返回空函数且不抛错', () => {
  assert.equal(typeof registerMainSessionPromptRoute(undefined, { store: memoryStore() }), 'function')
  assert.equal(typeof registerMainSessionPromptRoute({}, { store: memoryStore() }), 'function')
  const webServer = fakeWebServer()
  assert.equal(typeof registerMainSessionPromptRoute(webServer, { store: undefined }), 'function')
  assert.equal(webServer.routes.length, 0)
})

test('GET：返回按实体的提示词与长度上限', async () => {
  const store = memoryStore({ 猫猫: 'A', 梦梦: 'B' })
  const webServer = fakeWebServer()
  registerMainSessionPromptRoute(webServer, { store })
  const res = fakeResponse()
  await webServer.routes[0].handler({ method: 'GET', url: MAIN_SESSION_PROMPT_PATH }, res)
  assert.equal(res.statusCode, 200)
  assert.equal(res.headers['cache-control'], 'no-store')
  assert.deepEqual(res.body, { ok: true, prompts: { 猫猫: 'A', 梦梦: 'B' }, maxChars: MAIN_SESSION_PROMPT_MAX_CHARS })
})

test('POST：写入后回写最新快照', async () => {
  const store = memoryStore()
  const webServer = fakeWebServer()
  registerMainSessionPromptRoute(webServer, { store })
  const res = fakeResponse()
  await webServer.routes[0].handler(fakePost({ entity: ' 猫猫 ', prompt: '  你是猫猫  ' }), res)
  assert.equal(res.statusCode, 200)
  assert.equal(res.body.ok, true)
  assert.equal(res.body.entity, '猫猫')
  assert.equal(res.body.prompts['猫猫'], '你是猫猫')
  assert.equal(store.prompts.get('猫猫'), '你是猫猫')
})

test('POST 空 prompt：清除该实体条目', async () => {
  const store = memoryStore({ 猫猫: 'OLD' })
  const webServer = fakeWebServer()
  registerMainSessionPromptRoute(webServer, { store })
  const res = fakeResponse()
  await webServer.routes[0].handler(fakePost({ entity: '猫猫', prompt: '   ' }), res)
  assert.equal(res.statusCode, 200)
  assert.deepEqual(res.body.prompts, {})
})

test('POST 非法输入：400（JSON / entity / prompt / 超长）', async () => {
  const store = memoryStore()
  const webServer = fakeWebServer()
  registerMainSessionPromptRoute(webServer, { store })
  const handler = webServer.routes[0].handler
  for (const body of ['{bad json', { prompt: 'x' }, { entity: '猫猫', prompt: 42 }, { entity: '猫猫', prompt: 'x'.repeat(MAIN_SESSION_PROMPT_MAX_CHARS + 1) }]) {
    const res = fakeResponse()
    await handler(fakePost(body), res)
    assert.equal(res.statusCode, 400, `body=${String(body).slice(0, 20)}`)
    assert.equal(res.body.ok, false)
    assert.equal(res.body.error.code, 'invalid_request')
  }
})

test('方法错误：405；HEAD 与 GET 同响应', async () => {
  const webServer = fakeWebServer()
  registerMainSessionPromptRoute(webServer, { store: memoryStore() })
  const handler = webServer.routes[0].handler
  const res405 = fakeResponse()
  await handler({ method: 'PUT' }, res405)
  assert.equal(res405.statusCode, 405)
  const resHead = fakeResponse()
  await handler({ method: 'HEAD' }, resHead)
  assert.equal(resHead.statusCode, 200)
})

test('存储异常：500 且不抛错', async () => {
  const store = {
    list: () => ({}),
    set() { throw new Error('disk gone') },
  }
  const res = fakeResponse()
  await handleMainSessionPromptRequest(fakePost({ entity: '猫猫', prompt: 'x' }), res, store)
  assert.equal(res.statusCode, 500)
  assert.equal(res.body.error.code, 'store_failed')
})

test('真实存储往返：GET 读到 POST 写入的文本', async () => {
  const dir = mkdtempSync(join(tmpdir(), 'neko-msp-route-'))
  try {
    const store = createMainSessionPromptStore({ path: join(dir, 'p.json') })
    const webServer = fakeWebServer()
    registerMainSessionPromptRoute(webServer, { store })
    const handler = webServer.routes[0].handler
    const saved = fakeResponse()
    await handler(fakePost({ entity: '猫猫', prompt: '主会话专用提示词' }), saved)
    assert.equal(saved.statusCode, 200)
    const read = fakeResponse()
    await handler({ method: 'GET' }, read)
    assert.equal(read.body.prompts['猫猫'], '主会话专用提示词')
  } finally {
    rmSync(dir, { recursive: true, force: true })
  }
})
