/**
 * 扫码登录：生成二维码链接 → 轮询状态 → 成功后从 `set-cookie` 收集登录 cookie。
 *
 * 为什么要自己请求而不是复用 http 客户端：`poll` 的登录态在**响应头**（set-cookie）里，
 * 而通用客户端只返回 body。这里只多要一个 headers 能力。
 *
 * 凭据只交给调用方（插件入口）落盘，本模块不写文件、不回显 cookie。
 */

const GENERATE_API = "https://passport.bilibili.com/x/passport-login/web/qrcode/generate";
const POLL_API = "https://passport.bilibili.com/x/passport-login/web/qrcode/poll";
const LOGIN_COOKIE_KEYS = ["SESSDATA", "bili_jct", "DedeUserID", "buvid3", "buvid4", "DedeUserID__ckMd5"];
const DEFAULT_UA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/126.0";

/** 轮询状态码 → 语义。 */
export const QR_STATUS = {
  0: "success",
  86101: "waiting",
  86090: "scanned",
  86038: "expired",
};

/** 从 set-cookie 头里挑出登录相关的 name=value。 */
export function pickLoginCookies(headers) {
  const pairs = [];
  for (const header of headers) {
    const first = String(header).split(";")[0].trim();
    const name = first.slice(0, first.indexOf("="));
    if (!LOGIN_COOKIE_KEYS.includes(name)) continue;
    if (!first.includes("=")) continue;
    pairs.push(first);
  }
  return pairs.join("; ");
}

/**
 * 生成二维码。
 * @param {{fetchImpl?:Function, userAgent?:string}} [options]
 * @returns {Promise<{url:string, key:string}>} url 是二维码内容，key 用于轮询
 */
export async function generateQr(options = {}) {
  const fetchImpl = options.fetchImpl || globalThis.fetch;
  const response = await fetchImpl(GENERATE_API, {
    headers: { "user-agent": options.userAgent || DEFAULT_UA, accept: "application/json" },
  });
  const payload = await response.json();
  if (payload.code !== 0) throw new Error(`生成二维码失败：code=${payload.code} ${payload.message || ""}`);
  return { url: payload.data.url, key: payload.data.qrcode_key };
}

/**
 * 轮询一次二维码状态。
 * @param {{fetchImpl?:Function, userAgent?:string}} options
 * @param {string} key generateQr 返回的 key
 * @returns {Promise<{status:string, code:number, message:string, cookie?:string}>}
 *   status: waiting | scanned | expired | success；success 时带 cookie
 */
export async function pollQr(options, key) {
  const fetchImpl = options.fetchImpl || globalThis.fetch;
  const url = `${POLL_API}?qrcode_key=${encodeURIComponent(key)}&source=main-fe-header`;
  const response = await fetchImpl(url, {
    headers: { "user-agent": options.userAgent || DEFAULT_UA, accept: "application/json", referer: "https://www.bilibili.com/" },
  });
  const payload = await response.json();
  const status = QR_STATUS[payload.data?.code] || "unknown";
  const result = {
    status,
    code: payload.data?.code ?? payload.code,
    message: payload.data?.message || payload.message || "",
  };
  if (status !== "success") return result;
  const headers = typeof response.headers.getSetCookie === "function" ? response.headers.getSetCookie() : [];
  const cookie = pickLoginCookies(headers);
  if (!cookie.includes("SESSDATA")) {
    return { status: "success-no-cookie", code: result.code, message: "扫码已确认，但响应没带 SESSDATA，请重试" };
  }
  return { ...result, cookie };
}
