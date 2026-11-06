/**
 * QQ 消息的文本规范化与长消息切段。
 *
 * 两个都必须做对，否则「发出去的东西」和「模型看到的」不是一回事：
 * - 收：OneBot 的 message 可能是字符串，也可能是 segment 数组（text/at/image/face…）；
 * - 发：平台对单条长度有限制，长文本要切段并留间隔，且切段后不能因重试重复发同一段。
 */

const DEFAULT_MAX_CHARS = 1500;

/**
 * 把 OneBot 的 message 字段折成纯文本。
 * @param {string|Array<object>} message
 * @returns {string}
 */
export function messageToText(message) {
  if (typeof message === "string") return message.trim();
  if (!Array.isArray(message)) return "";
  const parts = [];
  for (const segment of message) {
    const type = segment?.type;
    const data = segment?.data || {};
    if (type === "text") parts.push(String(data.text || ""));
    else if (type === "at") parts.push(`@${data.qq === "all" ? "全体成员" : data.qq}`);
    else if (type === "image") parts.push("[图片]");
    else if (type === "face") parts.push("[表情]");
    else if (type === "record") parts.push("[语音]");
    else if (type === "video") parts.push("[视频]");
    else if (type === "file") parts.push(`[文件 ${data.name || ""}]`);
    else if (type === "reply") continue;
    else parts.push(`[${type || "未知"}]`);
  }
  return parts.join("").trim();
}

/**
 * 长文本切段：优先在换行/句末断开，避免把一句话劈成两半。
 * @param {string} text
 * @param {number} [maxChars]
 * @returns {string[]}
 */
export function splitLongText(text, maxChars = DEFAULT_MAX_CHARS) {
  const source = String(text || "");
  if (!source) return [];
  const limit = Math.max(1, Math.floor(maxChars));
  if (source.length <= limit) return [source];
  const chunks = [];
  let rest = source;
  while (rest.length > limit) {
    const window = rest.slice(0, limit);
    const breakAt = Math.max(
      window.lastIndexOf("\n"),
      window.lastIndexOf("。"),
      window.lastIndexOf("！"),
      window.lastIndexOf("？"),
      window.lastIndexOf(" "),
    );
    const cut = breakAt > limit * 0.5 ? breakAt + 1 : limit;
    chunks.push(rest.slice(0, cut).trim());
    rest = rest.slice(cut);
  }
  if (rest.trim()) chunks.push(rest.trim());
  return chunks.filter(Boolean);
}

/**
 * 从 history 接口的一条原始消息里抽出统一形态。
 * @param {object} raw OneBot 消息对象
 * @param {"group"|"private"} kind
 * @returns {{id:string, kind:string, conversation:string, sender:string, senderId:string,
 *   time:number, text:string}}
 */
export function normalizeMessage(raw, kind) {
  const conversation = kind === "group" ? String(raw?.group_id ?? "") : String(raw?.user_id ?? "");
  const senderId = String(raw?.sender?.user_id ?? raw?.user_id ?? "");
  return {
    id: String(raw?.message_id ?? ""),
    kind,
    conversation,
    sender: String(raw?.sender?.card || raw?.sender?.nickname || senderId || "未知"),
    senderId,
    time: Number(raw?.time) || 0,
    text: messageToText(raw?.message ?? raw?.raw_message ?? ""),
  };
}
