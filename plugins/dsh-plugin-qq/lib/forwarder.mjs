/**
 * 入站转发：把 QQ 消息投递进一个 DSH 会话（"接入会话"）。
 *
 * 默认关闭：只有显式配置 `forwardSessionId` 才启用。投递走 Agent 的 `followup(message)`——
 * 它把消息排进该 agent 的下一个轮次（`send` 需要目标与唤醒语义，`inject` 只做上下文注入，
 * 都不适合"外部来了一条用户消息"）。消息对象手工构造，形状与 dsh-llm 的 UserMessage 一致。
 *
 * 注意：本模块只构造消息并调用该方法，不做重试/排队——重试由调用方（轮询）下一轮自然覆盖。
 */
import { randomUUID } from "node:crypto";

/**
 * 造一条 QQ 来源的 user 消息。
 * @param {object} message normalizeMessage 的结果
 * @returns {object} UserMessage 形状
 */
export function toUserMessage(message) {
  const where = message.kind === "group" ? `群 ${message.conversation}` : `私聊 ${message.conversation}`;
  const text = `[QQ ${where}] ${message.sender}：${message.text}`;
  return {
    id: randomUUID(),
    role: "user",
    content: [{ type: "text", text }],
    source: { kind: "user" },
  };
}

/**
 * 建转发器。
 * @param {{agents?:object, sessionId?:string, enabled?:boolean}} [options]
 */
export function createForwarder(options = {}) {
  const agents = options.agents;
  const sessionId = options.sessionId ? String(options.sessionId) : "";
  const enabled = options.enabled === true && Boolean(sessionId);

  return {
    /** 是否真的会转发。 */
    active() {
      return enabled && typeof agents?.get === "function";
    },

    /**
     * 转发一批消息。
     * @param {object[]} messages
     * @returns {{forwarded:number, reason?:string}}
     */
    forward(messages) {
      if (!enabled) return { forwarded: 0, reason: "未配置 forwardSessionId" };
      if (typeof agents?.get !== "function") return { forwarded: 0, reason: "宿主没有 agents 服务" };
      const agent = agents.get(sessionId);
      if (!agent) return { forwarded: 0, reason: `找不到会话 ${sessionId}` };
      if (typeof agent.followup !== "function") return { forwarded: 0, reason: "该会话不支持 followup" };
      let forwarded = 0;
      for (const message of messages || []) {
        if (!message?.text) continue;
        agent.followup(toUserMessage(message));
        forwarded += 1;
      }
      return { forwarded };
    },
  };
}
