/**
 * 单测辅助：临时目录里的 UDS HTTP 服务（模拟 K1 控制面）与 Cordis 替身上下文。
 *
 * 只被 `*Test.mjs` 使用；不进入插件运行时，也不依赖真实 Go 服务（m3 验收要求）。
 */
import http from 'node:http'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

/**
 * 启动一个监听 Unix socket 的 HTTP 服务。
 * @param {(req: object, res: object, record: object) => void} handler 请求处理器（可异步延迟响应）
 * @returns {Promise<{socketPath: string, requests: object[], close: Function}>} 服务句柄
 */
export async function startControlServer(handler) {
  const dir = await mkdtemp(join(tmpdir(), 'astrbox-test-'))
  const socketPath = join(dir, 'control.sock')
  const requests = []
  const sockets = new Set()
  const server = http.createServer((req, res) => {
    res.on('error', () => {
      // 客户端在响应前断开（dispose / 超时用例）：忽略迟到的写错误。
    })
    let body = ''
    req.setEncoding('utf8')
    req.on('data', (chunk) => {
      body += chunk
    })
    req.on('end', () => {
      const record = {
        method: req.method,
        url: req.url,
        body: body === '' ? undefined : JSON.parse(body),
      }
      requests.push(record)
      try {
        handler(req, res, record)
      } catch {
        writeJson(res, 500, { ok: false, error: { code: 'internal', message: 'test handler failed' } })
      }
    })
  })
  server.on('connection', (socket) => {
    sockets.add(socket)
    socket.on('close', () => sockets.delete(socket))
  })
  await new Promise((resolve, reject) => {
    server.once('error', reject)
    server.listen(socketPath, resolve)
  })
  return {
    socketPath,
    requests,
    async close() {
      for (const socket of sockets) socket.destroy()
      await new Promise((resolve) => server.close(resolve))
      await rm(dir, { recursive: true, force: true })
    },
  }
}

/**
 * 写一条 JSON 响应。
 * @param {object} res 响应对象
 * @param {number} status 状态码
 * @param {unknown} body 响应体
 */
export function writeJson(res, status, body) {
  res.statusCode = status
  res.setHeader('content-type', 'application/json')
  res.end(JSON.stringify(body))
}

/** 等到下一轮宏任务（让 fire-and-forget 的请求落地）。 */
export function nextMacroTask() {
  return new Promise((resolve) => setTimeout(resolve, 0))
}

/** 轮询等待条件成立，避免固定 sleep。 */
export async function waitFor(check, timeoutMs = 2000) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    if (await check()) return
    await nextMacroTask()
  }
  throw new Error('waitFor: condition not met before timeout')
}

/**
 * 建一个 Cordis 替身 ctx：记录 section / 监听器 / interval / effect，并模拟
 * `ctx.get`（读替身服务）、`ctx.provide`（记录本插件服务）与 `ctx.inject`（依赖齐时回调）。
 * @param {{services?: Record<string, unknown>}} [options] 初始服务替身表
 * @returns {object} 替身句柄
 */
export function createStubContext(options = {}) {
  const sections = []
  const handlers = new Map()
  const intervals = []
  const effects = []
  const logs = []
  const provided = new Map()
  const services = new Map(Object.entries(options.services ?? {}))
  const ctx = {
    systemPrompt: {
      section(section) {
        sections.push(section)
        return () => {
          section.disposed = true
        }
      },
    },
    on(event, handler) {
      const list = handlers.get(event) ?? []
      list.push(handler)
      handlers.set(event, list)
      return () => {}
    },
    interval(callback, delay) {
      const entry = { callback, delay, active: true }
      intervals.push(entry)
      return () => {
        entry.active = false
      }
    },
    effect(execute) {
      const disposer = execute()
      effects.push(disposer)
      return () => {}
    },
    /** 模拟 Cordis 的服务读取：只返回注入表里的替身，缺失返回 undefined。 */
    get(name) {
      return services.get(name)
    },
    /** 模拟 Cordis 的 `provide`：记录提供的服务值并返回注销函数（重复提供与 Cordis 一样抛错）。 */
    provide(name, value) {
      if (provided.has(name)) throw new Error(`service "${name}" has been registered`)
      provided.set(name, value)
      return () => {
        provided.delete(name)
      }
    },
    /**
     * 模拟 Cordis 的可选注入：依赖都在替身服务表里时立即用同一替身上下文调用回调；
     * 依赖缺失时回调不运行（与宿主「服务未就绪插件照常加载」一致）。
     */
    inject(deps, callback) {
      if (!Array.isArray(deps) || !deps.every((name) => services.has(name))) return undefined
      return callback(ctx)
    },
    logger() {
      return {
        info: (message) => logs.push(['info', message]),
        warn: (message) => logs.push(['warn', message]),
        error: (message) => logs.push(['error', message]),
        debug: (message) => logs.push(['debug', message]),
      }
    },
  }
  return {
    ctx,
    sections,
    handlers,
    intervals,
    effects,
    logs,
    /** 本插件通过 `ctx.provide` 提供的服务（name → value）。 */
    provided,
    /** 注入/替换一个 `ctx.get` 可见的服务替身（模拟 qqbotBots 等上游服务）。 */
    setService(name, value) {
      services.set(name, value)
    },
    /** 删除服务替身（模拟服务随 fiber 卸载）。 */
    clearService(name) {
      services.delete(name)
    },
    /** @returns {Function | undefined} 某事件的第一个监听器 */
    handler(event) {
      return handlers.get(event)?.[0]
    },
    /** 模拟 fiber 卸载：按注册逆序执行 effect 清理。 */
    dispose() {
      for (const disposer of effects.slice().reverse()) {
        if (typeof disposer === 'function') disposer()
      }
    },
  }
}

/**
 * 用替身 decision 调 pre-step。
 * @param {Function} handler pre-step 监听器
 * @param {{agent?: object, turn?: number, step?: number, messages?: object[]}} payload 载荷
 * @returns {Promise<object>} decision
 */
export async function runPreStep(handler, payload) {
  const messages = payload.messages ?? []
  return handler({ messages, ...payload }, async () => ({ kind: 'enter', messages }))
}
