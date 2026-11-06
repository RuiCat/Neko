/**
 * 字幕：选轨、下载 JSON、格式化成带时间戳的纯文本。
 *
 * 实测：未登录时 `player/v2` 的 `subtitles` 常为空数组（AI 字幕只对登录态开放），
 * 所以「没有字幕」是正常结果而不是错误——调用方必须能把它讲清楚，而不是抛异常。
 */

const DEFAULT_MAX_CHARS = 20000;

/**
 * 把 `//host/path` 或相对地址补成 https 绝对地址。
 * @param {string} url
 * @returns {string}
 */
export function normalizeUrl(url) {
  const text = String(url || "").trim();
  if (!text) return "";
  if (text.startsWith("//")) return `https:${text}`;
  if (text.startsWith("http://")) return `https://${text.slice("http://".length)}`;
  return text;
}

/**
 * 选字幕轨：优先语言精确匹配，其次中文，最后第一条。
 * @param {Array<{lan:string, lan_doc:string, subtitle_url:string}>} subtitles
 * @param {string} [lang] 语言代码（zh-CN / en-US …）
 * @returns {object|null}
 */
export function pickSubtitle(subtitles, lang) {
  const list = Array.isArray(subtitles) ? subtitles.filter((item) => item?.subtitle_url) : [];
  if (!list.length) return null;
  if (lang) {
    const exact = list.find((item) => item.lan?.toLowerCase() === String(lang).toLowerCase());
    if (exact) return exact;
  }
  return list.find((item) => /zh/i.test(item.lan || "")) || list[0];
}

/**
 * 下载并解析字幕 JSON。
 * @param {Function} fetchImpl
 * @param {string} url 字幕地址
 * @param {{userAgent?:string, timeoutMs?:number}} [options]
 * @returns {Promise<Array<{from:number,to:number,content:string}>>}
 */
export async function fetchSubtitleLines(fetchImpl, url, options = {}) {
  const target = normalizeUrl(url);
  if (!target) throw new Error("字幕地址为空");
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), options.timeoutMs ?? 15000);
  try {
    const response = await fetchImpl(target, {
      headers: {
        "user-agent": options.userAgent || "Mozilla/5.0",
        referer: "https://www.bilibili.com/",
        accept: "application/json",
      },
      signal: controller.signal,
    });
    if (!response.ok) throw new Error(`字幕下载 HTTP ${response.status}`);
    const payload = await response.json();
    const body = Array.isArray(payload?.body) ? payload.body : [];
    return body.map((line) => ({
      from: Number(line.from) || 0,
      to: Number(line.to) || 0,
      content: String(line.content || "").trim(),
    }));
  } finally {
    clearTimeout(timer);
  }
}

/** 秒 → `mm:ss`。 */
export function formatTime(seconds) {
  const total = Math.max(0, Math.floor(Number(seconds) || 0));
  const minutes = String(Math.floor(total / 60)).padStart(2, "0");
  const rest = String(total % 60).padStart(2, "0");
  return `${minutes}:${rest}`;
}

/**
 * 渲染字幕文本（合并空行、按上限截断并标注）。
 * @param {Array<{from:number,content:string}>} lines
 * @param {number} [maxChars]
 * @returns {{text:string, truncated:boolean}}
 */
export function renderSubtitleLines(lines, maxChars = DEFAULT_MAX_CHARS) {
  const text = lines
    .filter((line) => line.content)
    .map((line) => `[${formatTime(line.from)}] ${line.content}`)
    .join("\n");
  if (text.length <= maxChars) return { text, truncated: false };
  return { text: text.slice(0, maxChars), truncated: true };
}
