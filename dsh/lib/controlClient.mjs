/**
 * K1 控制面客户端：HTTP/1.1 over Unix domain socket。
 *
 * 必须用 `node:http` 的 `socketPath`：`fetch` 不支持 UDS（K1 冻结约定）。
 * 本模块只做请求与错误分类，不记日志——失败由调用方限频记录，避免刷屏。
 * `dispose()` 会中止所有在飞请求，供插件卸载时清理（K2 dispose 语义）。
 */
import http from 'node:http'

const MAX_RESPONSE_BYTES = 4 * 1024 * 1024

/** 控制面错误：`code` 对应 K1 错误码或本地失败类别。 */
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
 * 从非 200 响应体提取 K1 错误。
 * @param {{status: number, body: unknown}} response 响应
 * @returns {ControlError} 错误
 */
function errorFromResponse(response) {
  const payload = typeof response.body === 'object' && response.body !== null ? response.body : {}
  const detail = typeof payload.error === 'object' && payload.error !== null ? payload.error : {}
  const code = typeof detail.code === 'string' ? detail.code : `http_${response.status}`
  const message = typeof detail.message === 'string' ? detail.message : `unexpected status ${response.status}`
  return new ControlError(code, message)
}

/**
 * 建控制面客户端。
 * @param {{socketPath: string, requestTimeoutMs?: number, tickTimeoutMs?: number}} options 配置
 * @returns {object} 客户端（getState / postTick / postSessionEvent / dispose）
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
     * `POST /tick`：单飞由服务端 409 兜底；409 作为“跳过”结果返回而不抛错。
     * @param {{scope?: string}} [request] tick 参数
     * @returns {Promise<{status: number, busy: boolean, body: any}>} 结果
     */
    async postTick(request = {}) {
      const response = await send({
        method: 'POST',
        path: '/tick',
        body: { scope: request.scope ?? '' },
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
