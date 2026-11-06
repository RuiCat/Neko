/**
 * 发评论：对视频主楼（或某条评论下）发表评论 / 回复。
 *
 * 这是**对外写操作**，所以过了三道闸才发：
 * 1. 必须登录（cookie 里有 `SESSDATA` 与 `bili_jct`，后者就是 csrf）；
 * 2. 配置必须显式 `allowPost: true` 且 `dryRun: false`；
 * 3. 调用方必须传 `confirm: true`（模型先说清要发什么，再由用户确认）。
 *
 * 任何一道不过都返回「未发送 + 原因」，不会静默降级成假成功。
 */
import { csrfOf, parseCookie } from "./bilibiliCredential.mjs";

const ADD_API = "https://api.bilibili.com/x/v2/reply/add";
const DEFAULT_UA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/126.0";

/**
 * 发表评论。
 * @param {{fetchImpl?:Function, cookie:string, aid:number, message:string,
 *   root?:string|number, parent?:string|number, timeoutMs?:number}} options
 * @returns {Promise<{rpid:string}>}
 */
export async function postComment(options) {
  const cookie = String(options.cookie || "");
  if (!parseCookie(cookie).SESSDATA) throw new Error("未登录：先用 bilibili_login 扫码登录");
  const csrf = csrfOf(cookie);
  if (!csrf) throw new Error("cookie 里没有 bili_jct（csrf），请重新扫码登录");
  const message = String(options.message || "").trim();
  if (!message) throw new Error("评论内容为空");
  if (message.length > 1000) throw new Error("评论超过 1000 字，B 站会拒收");

  const body = new URLSearchParams({ type: "1", oid: String(options.aid), message, plat: "1", csrf });
  if (options.root) body.set("root", String(options.root));
  if (options.parent) body.set("parent", String(options.parent));

  const fetchImpl = options.fetchImpl || globalThis.fetch;
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), options.timeoutMs ?? 15000);
  try {
    const response = await fetchImpl(ADD_API, {
      method: "POST",
      headers: {
        "user-agent": DEFAULT_UA,
        referer: "https://www.bilibili.com/",
        origin: "https://www.bilibili.com",
        "content-type": "application/x-www-form-urlencoded",
        cookie,
      },
      body: body.toString(),
      signal: controller.signal,
    });
    const payload = await response.json();
    if (payload.code !== 0) throw new Error(`发表失败：code=${payload.code} ${payload.message || ""}`);
    return { rpid: String(payload.data?.rpid_str || payload.data?.rpid || "") };
  } finally {
    clearTimeout(timer);
  }
}
