/**
 * 弹幕读取：拉 `comment.bilibili.com/<cid>.xml` 并解析成带时间的文本。
 *
 * 两个实测细节写进实现：
 * 1. 该接口返回 `content-encoding: deflate`，必须让运行时自动解压（Node 内置 fetch 会）；
 *    自己手解析压缩字节会得到乱码——所以这里用 `response.text()` 而不是读 arrayBuffer。
 * 2. 弹幕正文里有 XML 实体（`&amp;` `&lt;` 等），必须解码，否则模型看到的是转义串。
 */

const DANMAKU_API = "https://comment.bilibili.com";
const ENTRY = /<d p="([^"]*)">([\s\S]*?)<\/d>/g;
const ENTITIES = { amp: "&", lt: "<", gt: ">", quot: '"', apos: "'" };

/** 解码 XML 实体（够用即可，不求完整 XML 解析）。 */
export function decodeEntities(text) {
  return String(text || "").replace(/&(#x?[0-9a-fA-F]+|[a-zA-Z]+);/g, (match, body) => {
    if (body.startsWith("#x") || body.startsWith("#X")) return String.fromCodePoint(Number.parseInt(body.slice(2), 16));
    if (body.startsWith("#")) return String.fromCodePoint(Number.parseInt(body.slice(1), 10));
    return ENTITIES[body] ?? match;
  });
}

/**
 * 下载弹幕 XML 原文。
 * @param {Function} fetchImpl
 * @param {number|string} cid
 * @param {{userAgent?:string, timeoutMs?:number}} [options]
 * @returns {Promise<string>}
 */
export async function fetchDanmakuXml(fetchImpl, cid, options = {}) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), options.timeoutMs ?? 15000);
  try {
    const response = await fetchImpl(`${DANMAKU_API}/${cid}.xml`, {
      headers: {
        "user-agent": options.userAgent || "Mozilla/5.0",
        referer: "https://www.bilibili.com/",
        accept: "text/xml,application/xml",
      },
      signal: controller.signal,
    });
    if (!response.ok) throw new Error(`弹幕接口 HTTP ${response.status}`);
    return await response.text();
  } finally {
    clearTimeout(timer);
  }
}

/**
 * 解析弹幕。
 * `p` 字段：出现时间,模式,字号,颜色,时间戳,池,用户哈希,行号,…（只取前两个与颜色）
 * @param {string} xml
 * @param {{limit?:number, keyword?:string}} [options]
 * @returns {{total:number, items:Array<{time:number, mode:number, color:number, text:string}>}}
 */
export function parseDanmaku(xml, options = {}) {
  const limit = Number.isFinite(options.limit) && options.limit > 0 ? Math.floor(options.limit) : 200;
  const keyword = String(options.keyword || "").trim();
  const items = [];
  let total = 0;
  for (const match of String(xml || "").matchAll(ENTRY)) {
    total += 1;
    const parts = match[1].split(",");
    const text = decodeEntities(match[2]).trim();
    if (!text) continue;
    if (keyword && !text.includes(keyword)) continue;
    items.push({
      time: Number.parseFloat(parts[0]) || 0,
      mode: Number.parseInt(parts[1], 10) || 0,
      color: Number.parseInt(parts[3], 10) || 0,
      text,
    });
    if (items.length >= limit) break;
  }
  return { total, items };
}

/** 秒 → `mm:ss`。 */
export function clockText(seconds) {
  const total = Math.max(0, Math.floor(Number(seconds) || 0));
  return `${String(Math.floor(total / 60)).padStart(2, "0")}:${String(total % 60).padStart(2, "0")}`;
}

/**
 * 渲染弹幕文本（滚动弹幕与顶部/底部弹幕用模式号区分，这里只保留时间前缀）。
 * @param {{total:number, items:Array<object>}} parsed
 * @param {{keyword?:string, maxChars?:number}} [options]
 */
export function renderDanmaku(parsed, options = {}) {
  const maxChars = Number.isFinite(options.maxChars) ? options.maxChars : 8000;
  const scope = options.keyword ? `含「${options.keyword}」的` : "";
  const head = `弹幕总数 ${parsed.total} 条，取到${scope} ${parsed.items.length} 条：`;
  const body = parsed.items.map((item) => `[${clockText(item.time)}] ${item.text}`).join("\n");
  const text = `${head}\n\n${body}`;
  return text.length > maxChars ? `${text.slice(0, maxChars)}\n…（已截断）` : text;
}
