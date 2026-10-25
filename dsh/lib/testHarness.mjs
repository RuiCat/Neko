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
 * 建一个 Cordis 替身 ctx：记录 section / 监听器 / interval / effect。
 * @returns {object} 替身句柄
 */
export function createStubContext() {
  const sections = []
  const handlers = new Map()
  const intervals = []
  const effects = []
  const logs = []
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
