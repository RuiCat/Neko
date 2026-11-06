/**
 * B 站 HTTP 客户端：统一 UA/Referer、cookie 引导、超时、412 风控重试与错误归一。
 *
 * 实测结论写进实现：无 cookie 直连 `x/web-interface/view` 会被风控（HTTP 412 + HTML 错误页），
 * 先访问一次首页拿到 buvid3 等 cookie 后同一请求返回 200 JSON。因此这里把「引导一次再重试」
 * 做成客户端内建行为，而不是让每个调用点自己处理。
 */
import { createCookieJar } from "./cookieJar.mjs";

const DEFAULT_UA =
  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36";
const DEFAULT_TIMEOUT_MS = 15000;
const REFERER = "https://www.bilibili.com/";

/**
 * 建一个 B 站 JSON 客户端。
 * @param {{cookie?:string, userAgent?:string, timeoutMs?:number, fetchImpl?:Function}} [config]
 * @returns {{getJson:(url:string)=>Promise<any>}}
 */
export function createBilibiliHttp(config = {}) {
  const fetchImpl = config.fetchImpl || globalThis.fetch;
  const userAgent = config.userAgent || DEFAULT_UA;
  const timeoutMs = Number.isFinite(config.timeoutMs) ? config.timeoutMs : DEFAULT_TIMEOUT_MS;
  // cookie 可以是字符串，也可以是「每次请求时取当前值」的函数（扫码登录后要即时生效）。
  const cookieSource = typeof config.cookie === "function" ? config.cookie : () => config.cookie || "";
  const jar = config.cookie ? null : createCookieJar({ fetchImpl, userAgent });

  /** 发一次请求（cookie 在请求前就绪；被风控时不重试，重试只会延长封禁）。 */
  async function request(url) {
    const cookie = cookieSource() || (jar ? await jar.header() : "");
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), timeoutMs);
    let response;
    try {
      response = await fetchImpl(url, {
        headers: {
          "user-agent": userAgent,
          referer: REFERER,
          accept: "application/json, text/plain, */*",
          ...(cookie ? { cookie } : {}),
        },
        signal: controller.signal,
      });
    } catch (error) {
      if (error?.name === "AbortError") throw new Error(`请求超时（${timeoutMs}ms）：${url}`);
      throw new Error(`请求失败：${error instanceof Error ? error.message : String(error)}`);
    } finally {
      clearTimeout(timer);
    }
    if (response.status === 412) {
      throw new Error(
        "B站风控（HTTP 412）：请求太频繁或该 IP 被临时限制。稍后再试，或先 bilibili_login 用登录 cookie；" +
          "不要连续重试——越试封得越久。",
      );
    }
    return response;
  }

  return {
    /**
     * GET 一个 JSON 接口，返回**完整响应体**（不因 code 非 0 抛错）。
     *
     * 有些接口在未登录时仍返回有用数据：`nav` 的 `code=-101` 里就带着 WBI 密钥，
     * 所以取密钥不能走「code 必须为 0」的严格路径。
     * @param {string} url
     * @returns {Promise<{code:number, message?:string, data?:any}>}
     */
    async getPayload(url) {
      const response = await request(url);
      const text = await response.text();
      if (!response.ok) {
        throw new Error(`B站接口 HTTP ${response.status}：${text.slice(0, 120).replace(/\s+/g, " ")}`);
      }
      try {
        return JSON.parse(text);
      } catch {
        throw new Error(
          `B站返回的不是 JSON（多半被风控或需要登录 cookie）：${text.slice(0, 120).replace(/\s+/g, " ")}`,
        );
      }
    },

    /**
     * GET 一个 JSON 接口；`code !== 0` 归一等错误。
     * @param {string} url
     * @returns {Promise<any>} 接口的 data 字段
     */
    async getJson(url) {
      const payload = await this.getPayload(url);
      if (payload.code !== 0) {
        const hint = payload.code === -101 ? "（需要登录 cookie：先用 bilibili_login 扫码，或配置 cookie）" : "";
        throw new Error(`B站接口 code=${payload.code} ${payload.message || ""}${hint}`);
      }
      return payload.data;
    },
  };
}
