/**
 * K15：`qqbotBots.say({botId?, scope, text})` — 主动直发一条 QQ 消息。
 *
 * 不触发任何 agent 模型回合：
 * - 解析 bot（省略 botId 时要求唯一运行中 bot）；
 * - 在该 bot 的 SessionManager 里取该 scope 最近活跃会话的 replyTarget；
 * - 用该 bot 的 markdown 发送器、以主动推送语义（`forceProactive`，不复用被动窗口）直发。
 * 任何失败都返回 `{ok:false, reason}`，绝不抛给桥接。
 */
import type { ChatScope, ReplyTarget } from '../types.ts';
import type { QQBotSender } from '../transport/outbound-buffer.ts';

/** `text` 长度上限（字符数，与 K15 一致：空/超 1000 → invalid_text）。 */
export const SAY_TEXT_MAX_CHARS = 1000;

/** 直发失败原因（K15 冻结枚举）。 */
export type QQBotSayReason =
  | 'bot_not_found'
  | 'ambiguous_bot'
  | 'no_session'
  | 'send_failed'
  | 'invalid_text';

/** 直发请求（K15）。 */
export interface QQBotSayRequest {
  /** 目标 bot id；省略/空串时要求恰好一个运行中 bot */
  botId?: string;
  /** 私聊 / 群聊（决定取哪个 scope 的最近活跃会话） */
  scope: ChatScope;
  /** 消息正文（markdown；1–1000 字符） */
  text: string;
}

/** 直发结果（K15）。 */
export type QQBotSayResult =
  | { ok: true; botId: string; scope: ChatScope; targetId: string }
  | { ok: false; reason: QQBotSayReason };

/** 直发所需的最小 bot 形状（`BotCoordinator.list()` 的元素结构上满足）。 */
export interface SayableBot {
  readonly id: string;
  readonly manager: {
    /** 该 scope 最近活跃会话的回复目标（无会话时 undefined） */
    latestTarget(scope: ChatScope): ReplyTarget | undefined;
  };
  readonly sender: QQBotSender;
}

/** 是否为合法的 QQ 会话 scope。 */
function isChatScope(value: unknown): value is ChatScope {
  return value === 'c2c' || value === 'group';
}

/**
 * 解析目标 bot：显式 botId 精确匹配；省略/空串时要求唯一运行中 bot。
 * @param bots - 运行中的机器人（启动顺序）
 * @param botId - 请求里的 botId
 * @returns 命中的 bot，或失败原因
 */
function pickBot(bots: readonly SayableBot[], botId: string | undefined): SayableBot | QQBotSayReason {
  if (botId !== undefined && botId !== '') {
    if (typeof botId !== 'string') return 'bot_not_found';
    return bots.find(bot => bot.id === botId) ?? 'bot_not_found';
  }
  if (bots.length === 0) return 'bot_not_found';
  if (bots.length > 1) return 'ambiguous_bot';
  return bots[0] as SayableBot;
}

/**
 * 执行一次主动直发（K15）。
 * @param bots - 运行中的机器人
 * @param request - `{botId?, scope, text}`
 * @returns 成功 `{ok:true,botId,scope,targetId}`；失败 `{ok:false,reason}`（不抛）
 */
export async function sayToBot(
  bots: readonly SayableBot[],
  request: QQBotSayRequest,
): Promise<QQBotSayResult> {
  try {
    if (typeof request !== 'object' || request === null) return { ok: false, reason: 'invalid_text' };
    const { botId, scope, text } = request;
    if (typeof text !== 'string' || text.trim() === '' || text.length > SAY_TEXT_MAX_CHARS) {
      return { ok: false, reason: 'invalid_text' };
    }
    if (!isChatScope(scope)) return { ok: false, reason: 'no_session' };

    const picked = pickBot(bots, botId);
    if (typeof picked === 'string') return { ok: false, reason: picked };

    const latest = picked.manager.latestTarget(scope);
    if (latest === undefined) return { ok: false, reason: 'no_session' };

    // 主动推送：去掉被动回复窗口（不携带 msgId/eventId），由发送器自行限额。
    await picked.sender.sendMarkdown({ scope, targetId: latest.targetId, forceProactive: true }, text);
    return { ok: true, botId: picked.id, scope, targetId: latest.targetId };
  } catch {
    return { ok: false, reason: 'send_failed' };
  }
}
