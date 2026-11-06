/**
 * 视频引用解析：接受 BV 号、av 号、完整/短链 URL，抽出接口需要的 bvid/aid 与分 P 号。
 *
 * 只做纯字符串解析（不联网），因此可以在自测里穷举各种链接形态而不依赖网络。
 */

const BV_PATTERN = /BV[0-9A-Za-z]{10}/;
const AV_PATTERN = /(?:^|\/|av)(\d+)/i;

/**
 * 解析用户给的视频引用。
 * @param {string} input BV 号 / av 号 / bilibili 链接
 * @returns {{bvid?:string, aid?:number, page:number}} 至少含 bvid 或 aid
 */
export function parseVideoRef(input) {
  const text = String(input || "").trim();
  if (!text) throw new Error("需要 bvid/av 号或视频链接");
  const page = extractPage(text);
  const bvid = BV_PATTERN.exec(text)?.[0];
  if (bvid) return { bvid, page };
  const aid = extractAid(text, /[?&]aid=(\d+)/) ?? extractAid(text, AV_PATTERN);
  if (aid) return { aid, page };
  throw new Error(`无法从输入里认出 BV 号或 av 号：${text.slice(0, 120)}`);
}

/** 从 `?p=2` 或路径里取分 P 号（默认 1）。 */
export function extractPage(text) {
  const match = /[?&]p=(\d+)/.exec(text);
  const value = match ? Number.parseInt(match[1], 10) : 1;
  return Number.isFinite(value) && value > 0 ? value : 1;
}

/** 取 av 号的数字部分。 */
function extractAid(text, pattern) {
  const match = pattern.exec(text);
  if (!match) return null;
  const value = Number.parseInt(match[match.length - 1], 10);
  return Number.isFinite(value) && value > 0 ? value : null;
}

/** 按引用拼查询串（bvid 优先）。 */
export function videoQuery(ref) {
  if (ref.bvid) return `bvid=${encodeURIComponent(ref.bvid)}`;
  return `aid=${ref.aid}`;
}
