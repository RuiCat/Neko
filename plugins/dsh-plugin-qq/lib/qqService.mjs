/**
 * QQ 服务层：轮询收信 → 去重入队；发送 → 切段 + 白名单 + 干跑。
 *
 * 安全默认（上游共识）：`enabled` 默认 false、`dryRun` 默认 true —— 没手动打开之前，
 * 本插件不发任何网络消息，也不起轮询定时器。白名单默认「只能回给配置里监听过的会话」。
 */
import { createInbox } from "./inbox.mjs";
import { normalizeMessage, splitLongText } from "./messageText.mjs";
import { createOneBotClient } from "./oneBotClient.mjs";
import { createForwarder } from "./forwarder.mjs";

const DEFAULT_POLL_INTERVAL_MS = 4000;
const DEFAULT_HISTORY_COUNT = 20;
const DEFAULT_SEGMENT_DELAY_MS = 400;
const DEFAULT_MAX_TEXT_CHARS = 1500;

/** 睡一会儿（切段间隔用）。 */
function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/**
 * 建 QQ 服务。
 * @param {object} [config] 见 README 的配置表
 */
export function createQqService(config = {}) {
  const client = createOneBotClient(config);
  const inbox = createInbox({ maxQueue: config.maxQueue });
  const enabled = config.enabled === true;
  const dryRun = config.dryRun !== false;
  const pollIntervalMs = Number.isFinite(config.pollIntervalMs) ? config.pollIntervalMs : DEFAULT_POLL_INTERVAL_MS;
  const historyCount = Number.isFinite(config.historyCount) ? config.historyCount : DEFAULT_HISTORY_COUNT;
  const segmentDelayMs = Number.isFinite(config.segmentDelayMs) ? config.segmentDelayMs : DEFAULT_SEGMENT_DELAY_MS;
  const maxTextChars = Number.isFinite(config.maxTextChars) ? config.maxTextChars : DEFAULT_MAX_TEXT_CHARS;
  const watchGroups = (config.watchGroups || []).map(String).filter(Boolean);
  const watchUsers = (config.watchUsers || []).map(String).filter(Boolean);
  const allowAnyTarget = config.allowAnyTarget === true;
  const forwarder = createForwarder({
    agents: config.agents,
    sessionId: config.forwardSessionId,
    enabled: Boolean(config.forwardSessionId),
  });
  let timer = null;
  let polling = false;
  let lastError = "";
  let lastPollAt = 0;

  /** 目标是否被允许（白名单默认生效）。 */
  function targetAllowed(kind, id) {
    if (allowAnyTarget) return true;
    return kind === "group" ? watchGroups.includes(String(id)) : watchUsers.includes(String(id));
  }

  /**
   * 一条新消息的落点：先按 message_id 认领（去重），转发器可用时投递给会话，
   * 投不出去（未配置/会话不在）则留在本机队列等工具来取。
   * @param {object[]} messages
   */
  function intake(messages) {
    for (const message of messages) {
      if (!inbox.claim(message)) continue;
      if (forwarder.active() && forwarder.forward([message]).forwarded > 0) continue;
      inbox.enqueue(message);
    }
  }

  return {
    /** 拉取一轮历史消息并入队。 */
    async pollOnce() {
      if (polling) return { skipped: true };
      polling = true;
      const errors = [];
      try {
        for (const groupId of watchGroups) {
          try {
            const data = await client.call("get_group_msg_history", { group_id: Number(groupId), count: historyCount });
            intake((data?.messages || []).map((raw) => normalizeMessage(raw, "group")));
          } catch (error) {
            errors.push(`群 ${groupId}：${error.message}`);
          }
        }
        for (const userId of watchUsers) {
          try {
            const data = await client.call("get_friend_msg_history", { user_id: Number(userId), count: historyCount });
            intake((data?.messages || []).map((raw) => normalizeMessage(raw, "private")));
          } catch (error) {
            errors.push(`好友 ${userId}：${error.message}`);
          }
        }
        lastPollAt = Date.now();
        lastError = errors.join("；");
        return { errors, stats: inbox.stats() };
      } finally {
        polling = false;
      }
    },

    /** 起轮询（未 enabled 时不动）。 */
    start() {
      if (!enabled || timer) return false;
      timer = setInterval(() => {
        this.pollOnce().catch((error) => {
          lastError = error instanceof Error ? error.message : String(error);
        });
      }, pollIntervalMs);
      if (typeof timer.unref === "function") timer.unref();
      return true;
    },

    /** 停轮询。 */
    stop() {
      if (!timer) return false;
      clearInterval(timer);
      timer = null;
      return true;
    },

    /**
     * 发一条文本。
     * @param {"group"|"user"} kind 群还是私聊
     * @param {string} id 群号/QQ 号
     * @param {string} text 文本
     */
    async send(kind, id, text) {
      if (!enabled) return { ok: false, reason: "插件未启用（config.enabled=false）" };
      if (kind !== "group" && kind !== "user") throw new Error(`target 只能是 group 或 user：${kind}`);
      if (!targetAllowed(kind, id)) {
        return { ok: false, reason: `目标 ${id} 不在白名单（watchGroups/watchUsers），如需放开请配置 allowAnyTarget` };
      }
      const chunks = splitLongText(text, maxTextChars);
      if (!chunks.length) return { ok: false, reason: "文本为空" };
      if (dryRun) {
        return {
          ok: true,
          dryRun: true,
          chunks: chunks.length,
          reason: `干跑：本应分 ${chunks.length} 段发往 ${kind === "group" ? "群" : "好友"} ${id}`,
        };
      }
      const action = kind === "group" ? "send_group_msg" : "send_private_msg";
      const key = kind === "group" ? "group_id" : "user_id";
      const ids = [];
      for (let index = 0; index < chunks.length; index += 1) {
        const data = await client.call(action, { [key]: Number(id), message: chunks[index] });
        ids.push(data?.message_id ?? null);
        if (index < chunks.length - 1) await sleep(segmentDelayMs);
      }
      return { ok: true, chunks: chunks.length, messageIds: ids };
    },

    /** 取回并清空入站队列。 */
    drain(limit) {
      return inbox.drain(limit);
    },

    /** 只看不移除。 */
    peek(limit) {
      return inbox.peek(limit);
    },

    /** 状态快照（不主动联网）。 */
    status() {
      return {
        enabled,
        dryRun,
        polling: Boolean(timer),
        endpoint: client.endpoint(),
        watchGroups: watchGroups.length,
        watchUsers: watchUsers.length,
        allowAnyTarget,
        lastPollAt,
        lastError,
        ...inbox.stats(),
      };
    },

    /** 探测后端登录态（会发一次请求；仅显式调用时使用）。 */
    async loginInfo() {
      if (!enabled) return null;
      return client.call("get_login_info", {});
    },
  };
}
