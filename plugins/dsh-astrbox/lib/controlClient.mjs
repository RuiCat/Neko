/**
 * K1 控制面客户端：HTTP/1.1 over Unix domain socket。
 *
 * 必须用 `node:http` 的 `socketPath`：`fetch` 不支持 UDS（K1 冻结约定）。
 * 本模块只做请求与错误分类，不记日志——失败由调用方限频记录，避免刷屏。
 * `dispose()` 会中止所有在飞请求，供插件卸载时清理（K2 dispose 语义）。
 */
import http from 'node:http'

const MAX_RESPONSE_BYTES = 4 * 1024 * 1024

/** 控制面错误：`code` 对应 K1 错误码或本地失败类别；HTTP 响应错误另带
 * `status`（原状态码）与 `body`（原响应体），供 K15 面板路由按原状态透传。 */
export class ControlError extends Error {
  /**
   * @param {string} code 稳定错误码
   * @param {string} message 可读原因
   */
  constructor(code, message) {
    super(`${code}: ${message}`)
    this.name = 'ControlError'
    this.code = code
  }
}

/**
 * 把底层错误归类成 ControlError。
 * @param {unknown} error 原始错误
 * @returns {ControlError} 归类后的错误
 */
function toControlError(error) {
  if (error instanceof ControlError) return error
  const code = typeof error?.code === 'string' ? error.code : 'network_error'
  const message = error instanceof Error ? error.message : String(error)
  return new ControlError(code, message)
}

/**
 * 从非 200 响应体提取 K1 错误；`status`/`body` 保留原始响应，
 * 供 K15 面板路由按原状态透传控制面信封（传输错误没有这两个字段）。
 * @param {{status: number, body: unknown}} response 响应
 * @returns {ControlError} 错误
 */
function errorFromResponse(response) {
  const payload = typeof response.body === 'object' && response.body !== null ? response.body : {}
  const detail = typeof payload.error === 'object' && payload.error !== null ? payload.error : {}
  const code = typeof detail.code === 'string' ? detail.code : `http_${response.status}`
  const message = typeof detail.message === 'string' ? detail.message : `unexpected status ${response.status}`
  const error = new ControlError(code, message)
  error.status = response.status
  error.body = response.body
  return error
}

/**
 * 建控制面客户端。
 * @param {{socketPath: string, requestTimeoutMs?: number, tickTimeoutMs?: number}} options 配置
 * @returns {object} 客户端（getState / fetchObservations / fetchObservationState / postTick / dispose）
 */
export function createControlClient(options) {
  const { socketPath, requestTimeoutMs = 2000, tickTimeoutMs = 15000 } = options
  const inflight = new Set()
  let disposed = false

  /**
   * 发一个请求并解析 JSON 响应。
   * @param {{method: string, path: string, body?: unknown, timeoutMs: number}} request 请求参数
   * @returns {Promise<{status: number, body: any}>} 响应
   */
  function send(request) {
    return new Promise((resolve, reject) => {
      if (disposed) {
        reject(new ControlError('disposed', 'control client is disposed'))
        return
      }
      const bodyText = request.body === undefined ? undefined : JSON.stringify(request.body)
      const headers = { accept: 'application/json' }
      if (bodyText !== undefined) {
        headers['content-type'] = 'application/json'
        headers['content-length'] = String(Buffer.byteLength(bodyText))
      }
      const req = http.request(
        { socketPath, method: request.method, path: request.path, headers, agent: false },
        (res) => {
          const chunks = []
          let size = 0
          res.on('data', (chunk) => {
            size += chunk.length
            if (size > MAX_RESPONSE_BYTES) {
              req.destroy(new ControlError('response_too_large', 'control response exceeds 4 MiB'))
              return
            }
            chunks.push(chunk)
          })
          res.on('end', () => {
            const text = Buffer.concat(chunks).toString('utf8')
            let parsed = {}
            if (text.trim() !== '') {
              try {
                parsed = JSON.parse(text)
              } catch {
                reject(new ControlError('bad_response', 'control response is not valid JSON'))
                return
              }
            }
            resolve({ status: res.statusCode ?? 0, body: parsed })
          })
          res.on('error', (error) => reject(toControlError(error)))
        },
      )
      inflight.add(req)
      req.on('close', () => inflight.delete(req))
      req.on('error', (error) => reject(toControlError(error)))
      req.setTimeout(request.timeoutMs, () => {
        req.destroy(new ControlError('timeout', `${request.method} ${request.path} timed out after ${request.timeoutMs}ms`))
      })
      if (bodyText !== undefined) req.write(bodyText)
      req.end()
    })
  }

  return {
    /**
     * `GET /state`：按实体与白名单路径取当前值（只读）。
     * @param {{entity: string, paths: string[]}} query 查询参数
     * @returns {Promise<any>} 响应体
     */
    async getState(query) {
      const params = new URLSearchParams()
      params.set('entity', query.entity)
      for (const path of query.paths) params.append('path', path)
      const response = await send({ method: 'GET', path: `/state?${params.toString()}`, timeoutMs: requestTimeoutMs })
      if (response.status !== 200) throw errorFromResponse(response)
      return response.body
    },

    /**
     * `GET /entities`：只读 NPC 实体目录（K3）。
     * @returns {Promise<any>} 响应体 `{entities: [{id, type}]}`（id 升序，不含 data）
     */
    async getEntities() {
      const response = await send({ method: 'GET', path: '/entities', timeoutMs: requestTimeoutMs })
      if (response.status !== 200) throw errorFromResponse(response)
      return response.body
    },

    /**
     * `GET /v2/observations`：按 scope 拉 Observation v2 变化流（契约 §3.1）。
     * @param {string} scope 例如 `entity:猫`
     * @param {number} [since] 只返回 `seq > since` 的事件；缺省/0 表示不限
     * @returns {Promise<any>} 响应体 `{ok, scope, version, cursor, events, budget, …}`
     */
    async fetchObservations(scope, since) {
      const params = new URLSearchParams({ scope })
      if (Number.isInteger(since) && since > 0) params.set('since', String(since))
      const response = await send({ method: 'GET', path: `/v2/observations?${params.toString()}`, timeoutMs: requestTimeoutMs })
      if (response.status !== 200) throw errorFromResponse(response)
      return response.body
    },

    /**
     * `GET /v2/observations/state`：把事件流折叠成每个 path 的当前值（契约 §3.2）。
     * @param {string} scope 例如 `entity:猫`
     * @returns {Promise<any>} 响应体 `{ok, scope, version, entries, budget}`
     */
    async fetchObservationState(scope) {
      const params = new URLSearchParams({ scope })
      const response = await send({
        method: 'GET',
        path: `/v2/observations/state?${params.toString()}`,
        timeoutMs: requestTimeoutMs,
      })
      if (response.status !== 200) throw errorFromResponse(response)
      return response.body
    },

    /**
     * `GET /turn-context`：取该实体的补偿包（记忆/约束/情境/激活专家）。
     * @param {{entity: string}} query 查询
     * @returns {Promise<any>} 响应体 `{ok, compensation, gate}`
     */
    async getTurnContext(query) {
      const params = new URLSearchParams({ entity: query.entity })
      const response = await send({ method: 'GET', path: `/turn-context?${params.toString()}`, timeoutMs: requestTimeoutMs })
      if (response.status !== 200) throw errorFromResponse(response)
      return response.body
    },

    /**
     * `POST /state`：写实体状态（K19；`path+value` 与 `values` 二选一，由控制面校验）。
     * @param {{entity: string, path?: string, value?: unknown, values?: object}} body 写请求
     * @returns {Promise<any>} 响应体 `{ok, entity, paths}`
     */
    async setState(body) {
      const response = await send({ method: 'POST', path: '/state', body, timeoutMs: requestTimeoutMs })
      if (response.status !== 200) throw errorFromResponse(response)
      return response.body
    },

    /**
     * `POST /clock`：设定/推进世界时钟（K19；`minutes` 与 `time` 二选一）。
     * @param {{minutes?: number, time?: string}} body 时钟请求
     * @returns {Promise<any>} 响应体 `{ok, worldTime, month, phase}`
     */
    async setClock(body) {
      const response = await send({ method: 'POST', path: '/clock', body, timeoutMs: requestTimeoutMs })
      if (response.status !== 200) throw errorFromResponse(response)
      return response.body
    },

    /**
     * `POST /entities/create`：确保实体存在（不存在则由模板链创建）。
     * @param {string} entity 实体 id
     * @param {string} [type] 模板类型，缺省 NPC
     * @returns {Promise<any>} 响应体 `{ok, entity, type, created}`
     */
    async createEntity(entity, type = 'NPC') {
      const response = await send({
        method: 'POST',
        path: '/entities/create',
        body: { entity, type },
        timeoutMs: requestTimeoutMs,
      })
      if (response.status !== 200) throw errorFromResponse(response)
      return response.body
    },

    /**
     * `POST /entities/delete`：删除实体（K19；控制面同时清该实体黑板）。
     * @param {string} entity 实体 id
     * @returns {Promise<any>} 响应体 `{ok, deleted}`
     */
    async deleteEntity(entity) {
      const response = await send({
        method: 'POST',
        path: '/entities/delete',
        body: { entity },
        timeoutMs: requestTimeoutMs,
      })
      if (response.status !== 200) throw errorFromResponse(response)
      return response.body
    },

    /**
     * `GET /boards`：读会话黑板（K4）。`since` 缺省时响应不带 `deltas`（全量 live 视图）。
     * @param {{scope: 'workspace'|'global', workspace?: string, since?: number}} query 查询参数
     * @returns {Promise<any>} 响应体 `{ok, since?, truncated?, board, deltas?}`
     */
    async readBoard(query) {
      const params = new URLSearchParams()
      params.set('scope', query.scope)
      if (typeof query.workspace === 'string' && query.workspace !== '') params.set('workspace', query.workspace)
      if (Number.isInteger(query.since) && query.since >= 0) params.set('since', String(query.since))
      const response = await send({ method: 'GET', path: `/boards?${params.toString()}`, timeoutMs: requestTimeoutMs })
      if (response.status !== 200) throw errorFromResponse(response)
      return response.body
    },

    /**
     * `POST /boards`：写会话黑板（K5）。`session`/`workspace` 由桥接填写，模型不可传（K7）。
     * @param {{scope: 'workspace'|'global', workspace?: string, session: string, ops: object[]}} body 写请求
     * @returns {Promise<any>} 响应体 `{ok, changed, version, updated_at, trimmed?, evicted?}`
     */
    async writeBoard(body) {
      const response = await send({ method: 'POST', path: '/boards', body, timeoutMs: requestTimeoutMs })
      if (response.status !== 200) throw errorFromResponse(response)
      return response.body
    },

    /**
     * `GET /boards/workspaces`：工作区板摘要列表（K14，只读，按 workspace 升序）。
     * @returns {Promise<any>} 响应体 `{ok, workspaces:[{workspace,version,regions,updated_at}]}`
     */
    async listBoardWorkspaces() {
      const response = await send({ method: 'GET', path: '/boards/workspaces', timeoutMs: requestTimeoutMs })
      if (response.status !== 200) throw errorFromResponse(response)
      return response.body
    },

    /**
     * `POST /boards/panel`：面板管理员写（K13）。`by` 由控制面固定为 `面板`，调用方不传身份。
     * @param {{scope: 'workspace'|'global', workspace: string, region?: string, ops: object[]}} body 写请求
     * @returns {Promise<any>} 响应体 `{ok, changed, version, updated_at, trimmed?, evicted?}`
     */
    async writePanelBoard(body) {
      const response = await send({ method: 'POST', path: '/boards/panel', body, timeoutMs: requestTimeoutMs })
      if (response.status !== 200) throw errorFromResponse(response)
      return response.body
    },

    /**
     * `POST /tick`：单飞由服务端 409 兜底；409 作为“跳过”结果返回而不抛错。
     * @param {{scope?: string}} [request] tick 参数
     * @returns {Promise<{status: number, busy: boolean, body: any}>} 结果
     */
    async postTick(request = {}) {
      // 可选的世界时钟推进：只有活跃会话期间桥接才会带上（0 不带字段，保持旧协议）。
      const advanceMinutes = Number.isInteger(request.advanceMinutes) && request.advanceMinutes > 0
        ? request.advanceMinutes
        : 0
      const response = await send({
        method: 'POST',
        path: '/tick',
        body: advanceMinutes > 0
          ? { scope: request.scope ?? '', advanceMinutes }
          : { scope: request.scope ?? '' },
        timeoutMs: tickTimeoutMs,
      })
      if (response.status === 409) return { status: 409, busy: true, body: response.body }
      if (response.status !== 200) throw errorFromResponse(response)
      return { status: 200, busy: false, body: response.body }
    },

    /**
     * `POST /session-event`：会话生命周期事件。
     * @param {{kind: string, sessionId: string, cwd: string, turn?: number}} event 事件
     * @returns {Promise<any>} 响应体
     */
    async postSessionEvent(event) {
      const response = await send({ method: 'POST', path: '/session-event', body: event, timeoutMs: requestTimeoutMs })
      if (response.status !== 200) throw errorFromResponse(response)
      return response.body
    },

    /** 卸载清理：标记已销毁并中止在飞请求。 */
    dispose() {
      if (disposed) return
      disposed = true
      for (const req of inflight) {
        req.destroy(new ControlError('disposed', 'control client is disposed'))
      }
      inflight.clear()
    },

    /** @returns {number} 在飞请求数（测试与诊断用） */
    get inflightCount() {
      return inflight.size
    },

    /** @returns {boolean} 是否已销毁 */
    get disposed() {
      return disposed
    },
  }
}
