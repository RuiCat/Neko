/**
 * 评论区读取：主楼列表与楼中楼回复。
 *
 * 只读，不需要登录（登录后能看到更多/更全，但匿名也能取到公开评论）。
 * 字段按**接口实际返回**取名：`member.uname`、`content.message`、`like`、`rcount`、`ctime`。
 */

const REPLY_API = "https://api.bilibili.com/x/v2/reply";
const SUB_REPLY_API = "https://api.bilibili.com/x/v2/reply/reply";

/**
 * 归一一条评论。
 * @param {object} raw
 */
export function normalizeReply(raw) {
  return {
    rpid: String(raw?.rpid_str || raw?.rpid || ""),
    user: String(raw?.member?.uname || "匿名"),
    message: String(raw?.content?.message || "").trim(),
    like: Number(raw?.like) || 0,
    replyCount: Number(raw?.rcount) || 0,
    ctime: Number(raw?.ctime) || 0,
  };
}

/**
 * 取主楼评论。
 * @param {{getJson:Function}} http
 * @param {{aid:number, page?:number, pageSize?:number, sort?:number}} options
 *   sort: 0=按时间 1=按点赞 2=按回复数（接口语义）
 * @returns {Promise<{total:number, page:number, replies:object[], top:object[]}>}
 */
export async function listComments(http, options) {
  const page = Number.isFinite(options.page) ? options.page : 1;
  const pageSize = Number.isFinite(options.pageSize) ? options.pageSize : 20;
  const sort = Number.isFinite(options.sort) ? options.sort : 1;
  const data = await http.getJson(
    `${REPLY_API}?type=1&oid=${options.aid}&pn=${page}&ps=${pageSize}&sort=${sort}`,
  );
  const top = [data?.top?.upper, data?.top?.admin].filter(Boolean).map(normalizeReply);
  return {
    total: Number(data?.page?.acount) || 0,
    page,
    replies: (data?.replies || []).map(normalizeReply),
    top,
  };
}

/**
 * 取某条评论下的楼中楼。
 * @param {{getJson:Function}} http
 * @param {{aid:number, root:string|number, page?:number, pageSize?:number}} options
 */
export async function listReplies(http, options) {
  const page = Number.isFinite(options.page) ? options.page : 1;
  const pageSize = Number.isFinite(options.pageSize) ? options.pageSize : 10;
  const data = await http.getJson(
    `${SUB_REPLY_API}?type=1&oid=${options.aid}&root=${options.root}&pn=${page}&ps=${pageSize}`,
  );
  return { replies: (data?.replies || []).map(normalizeReply) };
}

/** 渲染评论为给模型看的文本。 */
export function renderComments(result) {
  const lines = [`共 ${result.total} 条评论，当前第 ${result.page} 页取到 ${result.replies.length} 条。`];
  if (result.top.length) {
    lines.push("", "置顶：");
    for (const reply of result.top) lines.push(`- ${reply.user}：${reply.message}`);
  }
  lines.push("");
  for (const reply of result.replies) {
    const when = reply.ctime ? new Date(reply.ctime * 1000).toISOString().slice(0, 16).replace("T", " ") : "-";
    const suffix = reply.replyCount ? `（${reply.replyCount} 条回复，rpid ${reply.rpid}）` : `（rpid ${reply.rpid}）`;
    lines.push(`- [${when}] ${reply.user}（赞 ${reply.like}）：${reply.message}${suffix}`);
  }
  return lines.join("\n");
}
