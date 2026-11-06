/**
 * QQ 工具面：`qq_send`（发）、`qq_inbox`（收）、`qq_status`（查）。
 *
 * 三个工具都不决定「要不要发」——是否真发由服务层的 enabled/dryRun/白名单决定，
 * 因此模型即便乱调工具也伤不到外部世界（默认干跑）。
 */
import { makeTool } from "./toolDef.mjs";

const SEND_PARAMETERS = {
  type: "object",
  properties: {
    target: { type: "string", description: "group=群聊，user=私聊（只能是这两个值）" },
    id: { type: "string", description: "群号或 QQ 号" },
    text: { type: "string", description: "要发送的纯文本；超长会按平台上限自动切段" },
  },
  required: ["target", "id", "text"],
  additionalProperties: false,
};

const INBOX_PARAMETERS = {
  type: "object",
  properties: {
    limit: { type: "integer", description: "最多取多少条，默认 20" },
    peek: { type: "boolean", description: "true 表示只看不移除（默认移除）" },
  },
  required: [],
  additionalProperties: false,
};

const STATUS_PARAMETERS = {
  type: "object",
  properties: {
    checkLogin: { type: "boolean", description: "true 时额外请求后端确认登录的 QQ 号" },
  },
  required: [],
  additionalProperties: false,
};

/** 渲染一条入站消息。 */
function renderMessage(message) {
  const where = message.kind === "group" ? `群 ${message.conversation}` : `私聊 ${message.conversation}`;
  const when = message.time ? new Date(message.time * 1000).toLocaleString("zh-CN") : "-";
  return `[${where}] ${message.sender}（${when}）：${message.text}`;
}

/**
 * 构造 QQ 工具。
 * @param {object} service createQqService 的返回值
 * @returns {object[]}
 */
export function buildQqTools(service) {
  const sendTool = makeTool({
    name: "qq_send",
    description:
      "通过 OneBot/NapCat 后端把一段文本发到 QQ 群或好友。受插件配置约束：未启用或干跑时不会真的发出，" +
      "目标不在白名单会被拒绝。不要用它发未经用户确认的内容。",
    parameters: SEND_PARAMETERS,
    async run(args) {
      const result = await service.send(String(args.target), String(args.id), String(args.text));
      if (!result.ok) return `未发送：${result.reason}`;
      if (result.dryRun) return `${result.reason}（dryRun=true，未真正上网）`;
      return `已发送 ${result.chunks} 段，message_id：${(result.messageIds || []).join(", ") || "-"}`;
    },
  });

  const inboxTool = makeTool({
    name: "qq_inbox",
    description:
      "取出（或只看）QQ 入站消息队列。默认取出后清空队列；peek=true 则保留。" +
      "队列由插件按配置轮询群/好友历史填充，已按 message_id 去重。",
    parameters: INBOX_PARAMETERS,
    run(args) {
      const limit = Number.isFinite(args.limit) ? args.limit : 20;
      const messages = args.peek === true ? service.peek(limit) : service.drain(limit);
      const stats = service.status();
      if (!messages.length) {
        return `收件箱为空（enabled=${stats.enabled} polling=${stats.polling} 监听群 ${stats.watchGroups} 个、好友 ${stats.watchUsers} 个）`;
      }
      const head = `取到 ${messages.length} 条（队列剩余 ${stats.queued} 条，历史收到 ${stats.received} 条）：`;
      return `${head}\n${messages.map(renderMessage).join("\n")}`;
    },
  });

  const statusTool = makeTool({
    name: "qq_status",
    description: "查看 QQ 插件状态：是否启用/干跑、后端地址、监听会话数、队列长度与最近一次错误。",
    parameters: STATUS_PARAMETERS,
    async run(args) {
      const stats = service.status();
      const lines = [
        `启用：${stats.enabled ? "是" : "否（config.enabled=false）"}`,
        `干跑：${stats.dryRun ? "是（不会真正发消息）" : "否"}`,
        `轮询：${stats.polling ? "运行中" : "未运行"}`,
        `后端：${stats.endpoint}`,
        `监听：群 ${stats.watchGroups} 个 / 好友 ${stats.watchUsers} 个（allowAnyTarget=${stats.allowAnyTarget}）`,
        `队列：${stats.queued} 条待取，累计收到 ${stats.received} 条，溢出丢弃 ${stats.dropped} 条`,
        `最近错误：${stats.lastError || "无"}`,
      ];
      if (args.checkLogin === true) {
        try {
          const info = await service.loginInfo();
          lines.push(`登录中的 QQ：${info?.user_id ? `${info.nickname}（${info.user_id}）` : "未登录/未启用"}`);
        } catch (error) {
          lines.push(`登录检查失败：${error instanceof Error ? error.message : String(error)}`);
        }
      }
      return lines.join("\n");
    },
  });

  return [sendTool, inboxTool, statusTool];
}
