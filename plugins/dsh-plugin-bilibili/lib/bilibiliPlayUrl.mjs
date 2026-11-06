/**
 * 播放地址：把 `x/player/wbi/playurl` 的 dash 响应归一成「能播的地址 + 清晰度清单」。
 *
 * 注意两件事，否则「拿到地址」也用不了：
 * 1. 媒体 CDN 会校验 `Referer: https://www.bilibili.com/`——播放器必须带；
 * 2. 地址里带签名与过期时间，**有时效**；返回里显式写出来，别让调用方缓存。
 *
 * 本模块只归一与渲染，不做下载（下载属于另一个能力，且涉及大文件与版权）。
 */

/**
 * 归一 dash 响应。
 * @param {object} data playurl 的 data
 * @returns {{quality:number, acceptQuality:number[], videos:Array<object>, audios:Array<object>, durl:Array<object>}}
 */
export function normalizePlayUrl(data) {
  const dash = data?.dash || {};
  return {
    quality: Number(data?.quality) || 0,
    acceptQuality: (data?.accept_quality || []).map(Number),
    videos: (dash.video || []).map((item) => ({
      id: Number(item.id) || 0,
      codecs: String(item.codecs || ""),
      width: Number(item.width) || 0,
      height: Number(item.height) || 0,
      bandwidth: Number(item.bandwidth) || 0,
      url: String(item.baseUrl || item.base_url || ""),
    })),
    audios: (dash.audio || []).map((item) => ({
      id: Number(item.id) || 0,
      codecs: String(item.codecs || ""),
      bandwidth: Number(item.bandwidth) || 0,
      url: String(item.baseUrl || item.base_url || ""),
    })),
    durl: (data?.durl || []).map((item) => ({ length: Number(item.length) || 0, url: String(item.url || "") })),
  };
}

/** 清晰度号 → 人类可读（只列常见值）。 */
export function qualityName(qn) {
  const names = {
    16: "360P", 32: "480P", 64: "720P", 74: "720P60", 80: "1080P", 112: "1080P+",
    116: "1080P60", 120: "4K", 125: "HDR", 126: "杜比视界", 127: "8K",
  };
  return names[qn] || `qn=${qn}`;
}

/**
 * 渲染播放地址。
 * @param {object} normalized
 * @param {{loginNote?:boolean}} [options]
 */
export function renderPlayUrl(normalized, options = {}) {
  const lines = [
    `当前清晰度：${qualityName(normalized.quality)}；可选：${normalized.acceptQuality.map(qualityName).join(" / ") || "-"}`,
    "媒体地址需要请求头 `Referer: https://www.bilibili.com/`，且带时效签名，不要长期缓存。",
  ];
  if (options.loginNote) lines.push("提示：未登录时通常最高只有 720P；要 1080P 及以上请先 bilibili_login。");
  const videos = normalized.videos;
  if (videos.length) {
    lines.push("", "视频流：");
    for (const item of videos) {
      lines.push(`- ${item.id} ${item.width}x${item.height} ${item.codecs}（${Math.round(item.bandwidth / 1024)} kb/s）`);
      lines.push(`  ${item.url}`);
    }
  }
  if (normalized.audios.length) {
    lines.push("", "音频流：");
    for (const item of normalized.audios) {
      lines.push(`- ${item.id} ${item.codecs}（${Math.round(item.bandwidth / 1024)} kb/s）`);
      lines.push(`  ${item.url}`);
    }
  }
  if (normalized.durl.length) {
    lines.push("", "合并流（durl）：");
    for (const item of normalized.durl) lines.push(`- ${item.length}s ${item.url}`);
  }
  if (!videos.length && !normalized.durl.length) lines.push("", "（没有取到可播放地址：可能需要登录或该视频受限）");
  return lines.join("\n");
}
