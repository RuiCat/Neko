/**
 * 极简 cookie 罐：只为绕过 B 站对「无 cookie 直连 API」的风控（实测 view 接口返回 412）。
 *
 * 做法与浏览器一致：先访问一次首页，收集 `set-cookie` 里的 name=value，再带上去请求 API。
 * 不做域名/路径/过期管理——我们只发往两个固定主机，且每次进程启动只引导一次。
 */

const BOOTSTRAP_URL = "https://www.bilibili.com/";
const DEFAULT_UA =
  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36";

/**
 * 取响应里的全部 set-cookie 头（兼容只有 getSetCookie 的运行时）。
 * @param {Response} response
 * @returns {string[]}
 */
export function setCookieHeaders(response) {
  if (typeof response.headers.getSetCookie === "function") return response.headers.getSetCookie();
  const single = response.headers.get("set-cookie");
  return single ? [single] : [];
}

/**
 * 把 set-cookie 行折成 `name=value; name2=value2`。
 * @param {string[]} headers
 * @returns {string}
 */
export function toCookieHeader(headers) {
  const pairs = [];
  for (const header of headers) {
    const first = String(header).split(";")[0].trim();
    if (!first.includes("=")) continue;
    const name = first.slice(0, first.indexOf("="));
    if (!name || /^(path|domain|expires|max-age|samesite|secure|httponly)$/i.test(name)) continue;
    pairs.push(first);
  }
  return pairs.join("; ");
}

/**
 * 建 cookie 罐：首次需要时访问首页并缓存，后续复用。
 * @param {{fetchImpl?:Function, userAgent?:string}} [options]
 * @returns {{header:()=>Promise<string>}}
 */
export function createCookieJar(options = {}) {
  const fetchImpl = options.fetchImpl || globalThis.fetch;
  const userAgent = options.userAgent || DEFAULT_UA;
  let cached = "";

  return {
    /** 当前 cookie 串（必要时先引导）。 */
    async header() {
      if (cached) return cached;
      if (typeof fetchImpl !== "function") return "";
      try {
        const response = await fetchImpl(BOOTSTRAP_URL, {
          headers: { "user-agent": userAgent, accept: "text/html" },
          redirect: "follow",
        });
        cached = toCookieHeader(setCookieHeaders(response));
      } catch {
        cached = "";
      }
      return cached;
    },
  };
}
