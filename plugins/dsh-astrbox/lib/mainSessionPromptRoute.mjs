/**
 * K28 主会话提示词路由：`/astrbox/main-session-prompts`（挂在 DSH `webServer` 上的具名路由）。
 *
 * - `GET` → `{ok:true, prompts:{<entity>: <text>}, maxChars}`（按实体返回当前提示词）
 * - `POST {entity, prompt}` → 写入（`prompt` 空串 = 清除该实体），回写后的 `prompts`
 * - 非 GET/POST → 405；JSON/字段非法 → 400 `{ok:false,error:{code:'invalid_request'}}`；
 *   存储异常 → 500（面板据此显示可读错误）
 *
 * 只读写桥接侧的提示词文件，不触碰控制面；路由注册失败由调用方兜底（无 `webServer` 时整段跳过）。
 */
import { MAIN_SESSION_PROMPT_MAX_CHARS, normalizePromptEntity, normalizePromptText } from './mainSessionPromptStore.mjs'

/** 主会话提示词路由路径（K28）。 */
export const MAIN_SESSION_PROMPT_PATH = '/astrbox/main-session-prompts'

/** 请求体大小上限（20k 字符提示词全中文 UTF-8 约 60KB，留足余量）。 */
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
 * @param {object} res 响应对象（http.ServerResponse）
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
 * 读并解析 JSON 请求体；空体返回 `{}`，非法 JSON/超上限返回 INVALID_BODY。
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
 * 400 错误响应。
 * @param {object} res 响应
 * @param {string} message 原因
 */
function badRequest(res, message) {
  sendJson(res, 400, { ok: false, error: { code: 'invalid_request', message } })
}

/**
 * 当前配置快照（实体 → 提示词）；读取异常回空对象，避免面板整块失败。
 * @param {object} store 提示词存储
 * @returns {Record<string, string>} 提示词快照
 */
function snapshotOf(store) {
  try {
    const list = store?.list?.()
    return list !== null && typeof list === 'object' && !Array.isArray(list) ? list : {}
  } catch {
    return {}
  }
}

/**
 * 处理一次主会话提示词请求（导出供路由测试直接调用）。
 * @param {{method?: string, url?: string}} req 请求 @param {object} res 响应
 * @param {object} store 提示词存储（get/list/set/remove）
 * @returns {Promise<void>} 完成（不抛错）
 */
export async function handleMainSessionPromptRequest(req, res, store) {
  const method = typeof req?.method === 'string' ? req.method : 'GET'
  if (method === 'GET' || method === 'HEAD') {
    sendJson(res, 200, { ok: true, prompts: snapshotOf(store), maxChars: MAIN_SESSION_PROMPT_MAX_CHARS })
    return
  }
  if (method !== 'POST') {
    sendJson(res, 405, { ok: false, error: { code: 'method_not_allowed', message: '只支持 GET/POST' } })
    return
  }
  const body = await readJsonBody(req)
  if (body === INVALID_BODY) return badRequest(res, '请求体不是合法 JSON')
  let entity
  let prompt
  try {
    entity = normalizePromptEntity(body.entity)
    prompt = normalizePromptText(body.prompt)
  } catch (error) {
    return badRequest(res, errorMessage(error))
  }
  try {
    if (typeof store?.set !== 'function') throw new Error('store unavailable')
    store.set(entity, prompt)
  } catch (error) {
    sendJson(res, 500, { ok: false, error: { code: 'store_failed', message: errorMessage(error) } })
    return
  }
  sendJson(res, 200, { ok: true, entity, prompts: snapshotOf(store), maxChars: MAIN_SESSION_PROMPT_MAX_CHARS })
}

/**
 * 在 webServer 上注册主会话提示词路由。
 * @param {{register: Function}} webServer `ctx.webServer`
 * @param {{store: object, logger?: object}} options 存储与日志
 * @returns {() => void} 注销函数（注册失败返回空函数）
 */
export function registerMainSessionPromptRoute(webServer, options) {
  const { store, logger } = options ?? {}
  if (webServer === undefined || webServer === null || typeof webServer.register !== 'function') return () => {}
  if (store === undefined || store === null) return () => {}
  try {
    const dispose = webServer.register({
      kind: 'exact',
      path: MAIN_SESSION_PROMPT_PATH,
      handler: (req, res) => handleMainSessionPromptRequest(req, res, store),
    })
    try {
      logger?.info?.(`主会话提示词路由已就绪: ${MAIN_SESSION_PROMPT_PATH}`)
    } catch {
      // 日志失败不影响路由。
    }
    return typeof dispose === 'function' ? dispose : () => {}
  } catch (error) {
    try {
      logger?.warnOnce?.('main-session-prompt-route', `主会话提示词路由注册失败（面板编辑不可用）：${errorMessage(error)}`)
    } catch {
      // 记录注册失败时不得再抛错。
    }
    return () => {}
  }
}
