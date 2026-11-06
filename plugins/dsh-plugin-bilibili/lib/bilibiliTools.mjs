/**
 * B 站工具面：`bilibili_video_info`（视频详情）与 `bilibili_subtitles`（字幕正文）。
 *
 * 两个工具都只读，不下载视频、不做登录态写操作；cookie 只用于绕过风控与取登录可见的字幕。
 */
import { createBilibiliApi } from "./bilibiliApi.mjs";
import { fetchSubtitleLines, pickSubtitle, renderSubtitleLines } from "./bilibiliSubtitle.mjs";
import { makeTool } from "./toolDef.mjs";

const INFO_PARAMETERS = {
  type: "object",
  properties: {
    input: { type: "string", description: "视频链接、BV 号或 av 号" },
    page: { type: "integer", description: "只看某个分 P（1 起），默认 1" },
  },
  required: ["input"],
  additionalProperties: false,
};

const SUBTITLE_PARAMETERS = {
  type: "object",
  properties: {
    input: { type: "string", description: "视频链接、BV 号或 av 号" },
    page: { type: "integer", description: "分 P 号（1 起），默认 1" },
    lang: { type: "string", description: "字幕语言代码，例如 zh-CN / ai-zh / en-US；默认中文优先" },
    maxChars: { type: "integer", description: "返回字幕的最大字符数，默认 20000" },
  },
  required: ["input"],
  additionalProperties: false,
};

/** 秒数 → `h:mm:ss`。 */
function durationText(seconds) {
  const total = Math.max(0, Math.floor(Number(seconds) || 0));
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const rest = total % 60;
  return hours > 0
    ? `${hours}:${String(minutes).padStart(2, "0")}:${String(rest).padStart(2, "0")}`
    : `${minutes}:${String(rest).padStart(2, "0")}`;
}

/** 时间戳 → 本地日期串。 */
function dateText(seconds) {
  const value = Number(seconds);
  if (!Number.isFinite(value) || value <= 0) return "-";
  return new Date(value * 1000).toISOString().slice(0, 10);
}

/**
 * 构造 B 站工具。
 * @param {{cookie?:string, timeoutMs?:number, fetchImpl?:Function, maxChars?:number}} [config]
 * @returns {object[]}
 */
export function buildBilibiliTools(config = {}) {
  const api = createBilibiliApi(config);
  const fetchImpl = config.fetchImpl || globalThis.fetch;

  const infoTool = makeTool({
    name: "bilibili_video_info",
    description:
      "抓取 B 站视频的公开信息：标题、UP 主、时长、发布时间、简介、分 P 清单与播放数据。" +
      "只读，不下载视频。需要登录或风控拦截时会返回明确错误。",
    parameters: INFO_PARAMETERS,
    async run(args) {
      const info = await api.videoInfo(args.input);
      const pages = Array.isArray(info.pages) ? info.pages : [];
      const lines = [
        `标题：${info.title}`,
        `编号：${info.bvid || "-"} / av${info.aid ?? "-"}`,
        `UP 主：${info.owner?.name || "-"}`,
        `时长：${durationText(info.duration)}　发布：${dateText(info.pubdate)}`,
        `播放：${info.stat?.view ?? "-"}　弹幕：${info.stat?.danmaku ?? "-"}　点赞：${info.stat?.like ?? "-"}`,
      ];
      if (pages.length > 1) {
        lines.push("", `分 P（${pages.length}）：`);
        for (const page of pages) {
          lines.push(`  P${page.page} ${page.part}（cid ${page.cid}，${durationText(page.duration)}）`);
        }
      }
      if (info.desc) lines.push("", "简介：", String(info.desc).slice(0, 2000));
      return lines.join("\n");
    },
  });

  const subtitleTool = makeTool({
    name: "bilibili_subtitles",
    description:
      "抓取 B 站视频某个分 P 的字幕并输出带时间戳的正文，用于总结/检索视频内容。" +
      "该稿件没有可用字幕（未登录时 AI 字幕常不可见）时会明确说明原因。",
    parameters: SUBTITLE_PARAMETERS,
    async run(args) {
      const page = Number.isFinite(args.page) ? args.page : 1;
      const { bvid, cid, title } = await api.resolveCid(args.input, page);
      if (!bvid) throw new Error("分 P 列表可用但拿不到 bvid，无法查询字幕");
      const player = await api.playerInfo(bvid, cid);
      const subtitles = player?.subtitle?.subtitles;
      const picked = pickSubtitle(subtitles, args.lang);
      if (!picked) {
        const count = Array.isArray(subtitles) ? subtitles.length : 0;
        const head = `${bvid}（${title || `P${page}`}）`;
        if (player?.need_login_subtitle === true) {
          return [
            `${head} 有字幕，但接口标记 need_login_subtitle=true：只有登录态才能取。`,
            "配置 cookie（插件 config.cookie 或环境变量 DSH_BILIBILI_COOKIE，值形如 SESSDATA=...）后重试。",
          ].join("\n");
        }
        return [
          `${head} 没有字幕轨：接口返回 ${count} 条字幕轨。`,
          "常见原因：该稿件没有字幕、字幕是 AI 生成且只对登录用户开放。",
          "可以配置 cookie（插件 config.cookie 或环境变量 DSH_BILIBILI_COOKIE）后重试。",
        ].join("\n");
      }
      const lines = await fetchSubtitleLines(fetchImpl, picked.subtitle_url, {
        userAgent: config.userAgent,
        timeoutMs: config.timeoutMs,
      });
      if (!lines.length) return `字幕轨 ${picked.lan}（${picked.lan_doc}）是空的。`;
      const maxChars = Number.isFinite(args.maxChars) ? args.maxChars : config.maxChars ?? 20000;
      const rendered = renderSubtitleLines(lines, maxChars);
      return [
        `${bvid}（${title || `P${page}`}）字幕轨：${picked.lan} ${picked.lan_doc}`,
        `共 ${lines.length} 行${rendered.truncated ? `（已截断到 ${maxChars} 字符）` : ""}：`,
        "",
        rendered.text,
      ].join("\n");
    },
  });

  return [infoTool, subtitleTool];
}
