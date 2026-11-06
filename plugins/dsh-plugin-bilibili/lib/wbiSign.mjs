/**
 * B 站 WBI 签名：给 `/x/player/wbi/v2` 这类接口生成 `wts` + `w_rid`。
 *
 * 为什么要它：上游实测（CZX2244/dsh-bilibili）指出未签名的 `/x/player/v2` 对已登录会话不稳定，
 * 所以签名版优先、明文版只做兜底。算法是公开的：nav 取 img/sub 两把 key → 重排成 32 位 mixinKey
 * → 参数按 key 排序拼查询串 → md5(查询串 + mixinKey)。
 *
 * 出处：B 站 web 端 wbi 算法与上述仓库的实现（本文件为独立零依赖实现，未复制其代码）。
 */
import { createHash } from "node:crypto";

const MIXIN_TAB = [
  46, 47, 18, 2, 53, 8, 23, 32, 15, 50, 10, 31, 58, 3, 45, 35, 27, 43, 5, 49, 33, 9, 42, 19, 29, 28,
  14, 39, 12, 38, 41, 13, 37, 48, 7, 16, 24, 55, 40, 61, 26, 17, 0, 1, 60, 51, 30, 4, 22, 25, 54, 21,
  56, 59, 6, 63, 57, 62, 11, 36, 20, 34, 44, 52,
];

const NAV_API = "https://api.bilibili.com/x/web-interface/nav";

/**
 * 从图片地址里取 key（`.../7cd084941338484aae1ad9425b84077c.png` → 文件名）。
 * @param {string} url
 * @returns {string}
 */
export function keyFromUrl(url) {
  const file = String(url || "").split("/").pop() || "";
  return file.replace(/\.[A-Za-z0-9]+$/, "");
}

/**
 * 把 imgKey+subKey 重排成 32 位 mixinKey。
 * @param {string} imgKey
 * @param {string} subKey
 * @returns {string}
 */
export function mixinKey(imgKey, subKey) {
  const raw = `${imgKey}${subKey}`;
  return MIXIN_TAB.map((index) => raw[index] || "").join("").slice(0, 32);
}

/** WBI 的 md5 十六进制。 */
export function wbiMd5(text) {
  return createHash("md5").update(text).digest("hex");
}

/**
 * 给参数签名：返回带 wts 与 w_rid 的新对象。
 * @param {Record<string,string|number>} params
 * @param {string} mixin
 * @param {number} [nowSeconds]
 * @returns {Record<string,string|number>}
 */
export function signParams(params, mixin, nowSeconds = Math.round(Date.now() / 1000)) {
  const withTs = { ...params, wts: nowSeconds };
  const query = Object.keys(withTs)
    .sort()
    .map((key) => `${encodeURIComponent(key)}=${encodeURIComponent(String(withTs[key]).replace(/[!'()*]/g, ""))}`)
    .join("&");
  return { ...withTs, w_rid: wbiMd5(query + mixin) };
}

/**
 * 把签名参数拼成查询串。
 * @param {Record<string,string|number>} params
 * @returns {string}
 */
export function toQuery(params) {
  return Object.keys(params)
    .map((key) => `${encodeURIComponent(key)}=${encodeURIComponent(String(params[key]))}`)
    .join("&");
}

/**
 * 建签名器：nav 取 key 并缓存；失败返回 null（调用方降级到明文接口）。
 * @param {{getJson:(url:string)=>Promise<any>}} http
 * @returns {{sign:(params:object)=>Promise<object|null>}}
 */
export function createWbiSigner(http) {
  let cachedMixin = "";
  return {
    async sign(params) {
      if (!cachedMixin) {
        let nav;
        try {
          // nav 未登录时 code=-101，但 data.wbi_img 仍在；必须取完整响应体而不是严格 getJson。
          nav = await http.getPayload(NAV_API);
        } catch {
          return null;
        }
        const imgKey = keyFromUrl(nav?.data?.wbi_img?.img_url);
        const subKey = keyFromUrl(nav?.data?.wbi_img?.sub_url);
        if (!imgKey || !subKey) return null;
        cachedMixin = mixinKey(imgKey, subKey);
      }
      return signParams(params, cachedMixin);
    },
  };
}
