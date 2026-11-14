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

/** 假控制面默认的 Observation v2 预算（契约 §2）。 */
const OBSERVATION_BUDGET = Object.freeze({ maxItems: 12, maxChars: 1200, coalesceMs: 60000 })

/**
 * 建一个模拟 K1 控制面 + Observation v2 的 UDS 服务（插件测试共用）。
 *
 * 支持 `/tick`、`/session-event`、`/entities`、`/state`、`/turn-context`、
 * `/v2/observations`、`/v2/observations/state`；其余路径 404。`control.observations` / `control.observationStates`
 * 按实体提供夹具（`{version, events|entries}`）。
 * @param {{stateBody?: object}} [options] 初始 `/state` 响应
 * @returns {Promise<{server: object, control: object}>} 服务与服务端开关
 */
export async function startBridgeServer(options = {}) {
  const control = {
    stateMode: 'ok',
    stateBody: options.stateBody ?? { ok: true, entity: '猫猫', values: {}, missing: [], bytes: 0 },
    stateByEntity: null,
    entitiesBody: { entities: [] },
    tickStatus: 200,
    tickDelayMs: 5,
    ticksCompleted: 0,
    /** 变化流夹具：entity → `{version, events}`；缺失 = 空流（version 0）。 */
    observations: {},
    /** 现状夹具：entity → `{version, entries}`；缺失 = 无现状。 */
    observationStates: {},
    /** 补偿包夹具：`{ok, compensation, gate}`；默认无补偿包。 */
    turnContext: { ok: true, compensation: null },
    /** `fail` 时两个 v2 端点都返回 500（模拟控制面不可用）。 */
    observationMode: 'ok',
  }
  const server = await startControlServer((req, res) => {
    const url = new URL(req.url, 'http://localhost')
    if (url.pathname === '/tick') {
      setTimeout(() => {
        try {
          if (control.tickStatus === 409) {
            writeJson(res, 409, { ok: false, error: { code: 'tick_busy', message: 'busy' } })
          } else {
            writeJson(res, 200, { ok: true, tickId: 't', status: 'ok', transitions: 0, conflicts: 0, errors: [] })
          }
        } catch {
          // 客户端已断开（dispose 场景）：忽略。
        } finally {
          control.ticksCompleted += 1
        }
      }, control.tickDelayMs)
      return
    }
    if (url.pathname === '/turn-context') {
      writeJson(res, 200, control.turnContext)
      return
    }
    if (url.pathname === '/session-event') {
      writeJson(res, 200, { ok: true, applied: 'logged' })
      return
    }
    if (url.pathname === '/entities') {
      writeJson(res, 200, control.entitiesBody)
      return
    }
    if (url.pathname === '/state') {
      if (control.stateMode === 'fail') {
        writeJson(res, 500, { ok: false, error: { code: 'internal', message: 'boom' } })
        return
      }
      const entity = url.searchParams.get('entity')
      const perEntity = control.stateByEntity !== null && entity !== null ? control.stateByEntity[entity] : undefined
      writeJson(res, 200, perEntity ?? control.stateBody)
      return
    }
    if (url.pathname === '/v2/observations' || url.pathname === '/v2/observations/state') {
      if (control.observationMode === 'fail') {
        writeJson(res, 500, { ok: false, error: { code: 'internal', message: 'observation boom' } })
        return
      }
      const scope = url.searchParams.get('scope') ?? ''
      if (scope.trim() === '') {
        writeJson(res, 400, { ok: false, error: { code: 'bad_request', message: 'scope 必填' } })
        return
      }
      const entity = scope.startsWith('entity:') ? scope.slice('entity:'.length) : ''
      if (url.pathname === '/v2/observations/state') {
        const source = control.observationStates[entity] ?? { version: 0, entries: [] }
        writeJson(res, 200, {
          ok: true,
          scope,
          version: Number.isInteger(source.version) ? source.version : 0,
          entries: Array.isArray(source.entries) ? source.entries : [],
          truncated: false,
          budget: OBSERVATION_BUDGET,
        })
        return
      }
      const since = Number(url.searchParams.get('since') ?? 0)
      const source = control.observations[entity] ?? { version: 0, events: [] }
      const version = Number.isInteger(source.version) ? source.version : 0
      const events = (Array.isArray(source.events) ? source.events : []).filter((event) => (event.seq ?? 0) > (Number.isFinite(since) ? since : 0))
      writeJson(res, 200, {
        ok: true,
        scope,
        version,
        since: Number.isFinite(since) ? since : 0,
        cursor: events.length > 0 ? events.at(-1).seq : version,
        events,
        truncated: false,
        dropped: 0,
        budget: OBSERVATION_BUDGET,
        policyVersion: 2,
      })
      return
    }
    writeJson(res, 404, { ok: false, error: { code: 'not_found', message: url.pathname } })
  })
  return { server, control }
}

