/**
 * OneBot 11 HTTP 客户端：只做「调一个 action，拿回 data」这件事。
 *
 * 约定来自上游实测（Yuuyuko-uu/dsh-qq-bridge）：
 * - 插件 → 后端的**正向 HTTP**（`POST {apiBase}/{action}`），不需要 WS；
 * - 令牌为空时**不发** Authorization 头，非空时是标准 `Bearer <token>`；
 * - 干跑（dryRun）默认开启：没有显式关闭前，任何发送都不上网。
 */

const DEFAULT_TIMEOUT_MS = 10000;
const DEFAULT_API_BASE = "http://127.0.0.1:3000";

/**
 * 建客户端。
 * @param {{apiBase?:string, token?:string, timeoutMs?:number, dryRun?:boolean, fetchImpl?:Function}} [config]
 */
export function createOneBotClient(config = {}) {
  const apiBase = String(config.apiBase || DEFAULT_API_BASE).replace(/\/+$/, "");
  const token = String(config.token || "");
  const timeoutMs = Number.isFinite(config.timeoutMs) ? config.timeoutMs : DEFAULT_TIMEOUT_MS;
  const dryRun = config.dryRun !== false;
  const fetchImpl = config.fetchImpl || globalThis.fetch;

  /** 组装请求头：令牌为空时不带 Authorization。 */
  function headers() {
    const base = { "content-type": "application/json" };
    return token ? { ...base, authorization: `Bearer ${token}` } : base;
  }

  return {
    /** 当前实际使用的后端地址（状态展示用）。 */
    endpoint() {
      return apiBase;
    },

    /** 是否干跑。 */
    isDryRun() {
      return dryRun;
    },

    /**
     * 调一个 OneBot action。
     * @param {string} action 例如 send_group_msg
     * @param {object} params 该 action 的参数
     * @returns {Promise<any>} 响应里的 data
     */
    async call(action, params = {}) {
      const controller = new AbortController();
      const timer = setTimeout(() => controller.abort(), timeoutMs);
      let response;
      try {
        response = await fetchImpl(`${apiBase}/${action}`, {
          method: "POST",
          headers: headers(),
          body: JSON.stringify(params),
          signal: controller.signal,
        });
      } catch (error) {
        if (error?.name === "AbortError") throw new Error(`OneBot 请求超时（${timeoutMs}ms）：${action}`);
        throw new Error(`连不上 OneBot 后端 ${apiBase}：${error instanceof Error ? error.message : String(error)}`);
      } finally {
        clearTimeout(timer);
      }
      const text = await response.text();
      if (!response.ok) throw new Error(`OneBot ${action} HTTP ${response.status}：${text.slice(0, 200)}`);
      let payload;
      try {
        payload = JSON.parse(text);
      } catch {
        throw new Error(`OneBot ${action} 返回的不是 JSON：${text.slice(0, 200)}`);
      }
      if (payload.status === "failed" || (payload.retcode !== undefined && payload.retcode !== 0)) {
        throw new Error(`OneBot ${action} 失败：retcode=${payload.retcode} ${payload.wording || payload.message || ""}`);
      }
      return payload.data;
    },
  };
}
