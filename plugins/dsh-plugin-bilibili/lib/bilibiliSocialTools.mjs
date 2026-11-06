/**
 * 登录与互动工具：`bilibili_login` / `bilibili_comments` / `bilibili_danmaku` /
 * `bilibili_play_urls` / `bilibili_comment_post`。
 *
 * 读操作（评论/弹幕/播放地址）不需要登录；写操作（发评论）要过三道闸，见 `bilibiliPost.mjs`。
 */
import { listComments, listReplies, renderComments } from "./bilibiliComments.mjs";
import { fetchDanmakuXml, parseDanmaku, renderDanmaku } from "./bilibiliDanmaku.mjs";
import { normalizePlayUrl, renderPlayUrl } from "./bilibiliPlayUrl.mjs";
import { postComment } from "./bilibiliPost.mjs";
import { makeTool } from "./toolDef.mjs";
import { parseVideoRef, videoQuery } from "./bilibiliInput.mjs";

const LOGIN_PARAMETERS = {
  type: "object",
  properties: {
    action: { type: "string", description: "generate=生成二维码链接，poll=轮询扫码结果，status=看当前登录态" },
    qrcodeKey: { type: "string", description: "generate 返回的 key；poll 时可省略（默认用最近一次生成）" },
  },
  required: ["action"],
  additionalProperties: false,
};

const COMMENTS_PARAMETERS = {
  type: "object",
  properties: {
    input: { type: "string", description: "视频链接、BV 号或 av 号" },
    root: { type: "string", description: "给了就取这条评论（rpid）下的楼中楼" },
    page: { type: "integer", description: "第几页，默认 1" },
    pageSize: { type: "integer", description: "每页条数，默认 20（楼中楼默认 10）" },
    sort: { type: "integer", description: "0=按时间 1=按点赞 2=按回复数" },
  },
  required: ["input"],
  additionalProperties: false,
};

const DANMAKU_PARAMETERS = {
  type: "object",
  properties: {
    input: { type: "string", description: "视频链接、BV 号或 av 号" },
    page: { type: "integer", description: "分 P 号，默认 1" },
    limit: { type: "integer", description: "最多取多少条弹幕，默认 200" },
    keyword: { type: "string", description: "只看包含该关键词的弹幕" },
  },
  required: ["input"],
  additionalProperties: false,
};

const PLAY_PARAMETERS = {
  type: "object",
  properties: {
    input: { type: "string", description: "视频链接、BV 号或 av 号" },
    page: { type: "integer", description: "分 P 号，默认 1" },
    qn: { type: "integer", description: "清晰度号：80=1080P 64=720P 32=480P；未登录通常最高 720P" },
  },
  required: ["input"],
  additionalProperties: false,
};

const POST_PARAMETERS = {
  type: "object",
  properties: {
    input: { type: "string", description: "视频链接、BV 号或 av 号" },
    message: { type: "string", description: "评论内容（≤1000 字）" },
    root: { type: "string", description: "回复某条评论时给它的 rpid" },
    confirm: { type: "boolean", description: "必须显式传 true 才真的发送" },
  },
  required: ["input", "message"],
  additionalProperties: false,
};

/**
 * 从引用解析出 aid（评论接口要 aid 而不是 bvid）。
 * 详情接口被风控时退回更轻的 `archive/stat`（只取 aid），避免整条能力不可用。
 */
async function aidOf(runtime, input) {
  try {
    const info = await runtime.info(input);
    return { aid: Number(info.aid) || 0, title: String(info.title || "") };
  } catch (error) {
    if (!/412/.test(String(error?.message))) throw error;
    const ref = parseVideoRef(input);
    const stat = await runtime.api.raw.http.getJson(
      `https://api.bilibili.com/x/web-interface/archive/stat?${videoQuery(ref)}`,
    );
    return { aid: Number(stat.aid) || 0, title: "（详情被风控，标题未知）" };
  }
}

/**
 * 从引用解析出 bvid 与 cid。
 * 详情被风控时退回 `pagelist`（只取分 P 与 cid）。
 */
async function mediaOf(runtime, input, page) {
  const wanted = Number.isFinite(page) ? page : 1;
  try {
    const info = await runtime.info(input);
    const target = (info.pages || []).find((item) => item.page === wanted) || (info.pages || [])[0];
    return { bvid: info.bvid, cid: target?.cid, title: target?.part || info.title, pages: (info.pages || []).length };
  } catch (error) {
    if (!/412/.test(String(error?.message))) throw error;
    const pages = await runtime.api.pageList(input);
    const target = pages.find((item) => item.page === wanted) || pages[0];
    return {
      bvid: parseVideoRef(input).bvid,
      cid: target?.cid,
      title: target?.part || "（详情被风控）",
      pages: pages.length,
    };
  }
}

/**
 * 构造登录与互动工具。
 * @param {object} runtime createBilibiliRuntime 的返回值
 * @returns {object[]}
 */
export function buildSocialTools(runtime) {
  const loginTool = makeTool({
    name: "bilibili_login",
    description:
      "B 站扫码登录：generate 给出二维码内容链接（用手机 B 站 App 扫码），poll 轮询直到成功，" +
      "status 查看当前是否已登录。登录态只存本机 ~/.dsh/bilibili-state.json（0600），不回显 cookie。",
    parameters: LOGIN_PARAMETERS,
    async run(args) {
      const action = String(args.action || "").toLowerCase();
      if (action === "status") {
        const state = runtime.describe();
        const lines = [
          `登录态：${state.hasSessData ? "已登录" : "未登录"}`,
          `CSRF（bili_jct）：${state.hasCsrf ? "有" : "无"}`,
          `账号 ID：${state.userId || "-"}`,
          `凭据文件：${state.statePath}${state.savedAt ? `（保存于 ${state.savedAt}）` : ""}`,
        ];
        return lines.join("\n");
      }
      if (action === "generate") {
        const qr = await runtime.loginGenerate();
        return [
          "二维码内容（把它转成二维码或用支持扫码的客户端打开）：",
          qr.url,
          "",
          `qrcode_key：${qr.key}`,
          "用手机 B 站 App 扫码并在手机上确认，然后调用 bilibili_login action=poll。",
        ].join("\n");
      }
      if (action === "poll") {
        const result = await runtime.loginPoll(args.qrcodeKey);
        const labels = {
          success: "登录成功，凭据已保存到本机",
          "success-no-cookie": "扫码已确认，但没拿到登录 cookie，请重新生成二维码",
          waiting: "还没扫码",
          scanned: "已扫码，等待手机确认",
          expired: "二维码已过期，请重新 generate",
          unknown: `未知状态（code=${result.code}）`,
        };
        return `${labels[result.status] || result.status}${result.message ? `：${result.message}` : ""}`;
      }
      return `不认识的 action：${action}（只支持 generate / poll / status）`;
    },
  });

  const commentsTool = makeTool({
    name: "bilibili_comments",
    description:
      "读 B 站视频的评论区：主楼按时间/点赞/回复数排序，或给 root（rpid）读某条评论的楼中楼。只读。",
    parameters: COMMENTS_PARAMETERS,
    async run(args) {
      const { aid, title } = await aidOf(runtime, args.input);
      if (!aid) throw new Error("拿不到 aid，无法读评论");
      const head = `${title}（aid ${aid}）`;
      if (args.root) {
        const result = await listReplies(runtime.api.raw.http, { aid, root: args.root, page: args.page, pageSize: args.pageSize });
        if (!result.replies.length) return `${head} 的评论 ${args.root} 下没有回复。`;
        return `${head} 的评论 ${args.root} 下 ${result.replies.length} 条回复：\n${renderComments({ ...result, total: result.replies.length, page: 1, top: [] }).split("\n").slice(1).join("\n")}`;
      }
      const result = await listComments(runtime.api.raw.http, { aid, page: args.page, pageSize: args.pageSize, sort: args.sort });
      return `${head}\n${renderComments(result)}`;
    },
  });

  const danmakuTool = makeTool({
    name: "bilibili_danmaku",
    description: "读 B 站视频某个分 P 的弹幕（按时间排序的文本，可只看含关键词的弹幕）。只读。",
    parameters: DANMAKU_PARAMETERS,
    async run(args) {
      const media = await mediaOf(runtime, args.input, args.page);
      if (!media.cid) throw new Error("拿不到 cid，无法读弹幕");
      const xml = await fetchDanmakuXml(runtime.fetchImpl, media.cid, {
        userAgent: runtime.userAgent,
        timeoutMs: runtime.timeoutMs,
      });
      const parsed = parseDanmaku(xml, { limit: args.limit, keyword: args.keyword });
      return `${media.title || media.bvid}（cid ${media.cid}）\n${renderDanmaku(parsed, { keyword: args.keyword })}`;
    },
  });

  const playTool = makeTool({
    name: "bilibili_play_urls",
    description:
      "取 B 站视频的播放地址（dash 视频/音频分离流）与清晰度清单。地址有时效且必须带 Referer；本工具不下载视频。",
    parameters: PLAY_PARAMETERS,
    async run(args) {
      const media = await mediaOf(runtime, args.input, args.page);
      if (!media.cid || !media.bvid) throw new Error("拿不到 bvid/cid，无法取播放地址");
      const data = await runtime.api.playUrl(media.bvid, media.cid, { qn: args.qn });
      const normalized = normalizePlayUrl(data);
      const loggedIn = runtime.describe().hasSessData;
      return `${media.title || media.bvid}（cid ${media.cid}）\n${renderPlayUrl(normalized, { loginNote: !loggedIn })}`;
    },
  });

  const postTool = makeTool({
    name: "bilibili_comment_post",
    description:
      "在 B 站视频下发评论（或回复某条评论）。这是对外写操作：必须已登录、插件配置 allowPost=true 且 dryRun=false，" +
      "并且本次调用显式 confirm=true 才会真的发出；否则只返回将发送的内容预览。",
    parameters: POST_PARAMETERS,
    async run(args) {
      const message = String(args.message || "").trim();
      const { aid, title } = await aidOf(runtime, args.input);
      if (!aid) throw new Error("拿不到 aid，无法发评论");
      const state = runtime.describe();
      const head = `目标：${title}（aid ${aid}）${args.root ? `，回复 rpid ${args.root}` : ""}`;
      const preview = `${head}\n内容：${message}`;
      if (!state.hasSessData) return `未发送：未登录（先 bilibili_login 扫码）。\n${preview}`;
      if (!state.hasCsrf) return `未发送：cookie 缺少 bili_jct。\n${preview}`;
      if (!runtime.allowPost) return `未发送：插件配置 allowPost=false（默认）。\n${preview}`;
      if (runtime.dryRun) return `未发送：插件处于干跑（dryRun=true，默认）。\n${preview}`;
      if (args.confirm !== true) return `未发送：需要显式 confirm=true。\n${preview}`;
      const result = await postComment({
        fetchImpl: runtime.fetchImpl,
        cookie: runtime.holder.cookie,
        aid,
        message,
        root: args.root,
        timeoutMs: runtime.timeoutMs,
      });
      return `已发表，rpid：${result.rpid || "-"}（${title}）`;
    },
  });

  return [loginTool, commentsTool, danmakuTool, playTool, postTool];
}
