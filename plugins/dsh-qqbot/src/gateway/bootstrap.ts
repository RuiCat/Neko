/**
 * 网关组装 — 创建 bot、编排中间件、注册事件、出站、生命周期
 *
 * 将 QQ 消息平台作为 dsh 前端协议驱动：入站消息 → handleInbound → dsh agent，
 * dsh session/event → createOutboundHandler → QQ 出站。
 */
import type { Context } from '@deepseek-ai/cordis';
import { QQBot } from '@tencent-connect/qqbot-nodejs';
import type { GatewayDisconnect, InteractionEvent, MiddlewareContext } from '@tencent-connect/qqbot-nodejs';
import { SessionManager, type DshAgentRegistry } from '../session/index.ts';
import { handleInbound, createOutboundHandler, createTurnTracker } from '../transport/index.ts';
import { QqInputRegistry, inputOriginTtlMs } from '../transport/qq-input-origin.ts';
import { parseStreamFrame } from '../transport/events.ts';
import { ReplyLimiter } from '../transport/reply-limiter.ts';
import { cacheMsgId, cacheEventId } from '../transport/msgid-cache.ts';
import { resolveReplyTarget, sendResolvedMarkdown } from '../transport/reply-target.ts';
import type { ToolsRegistryLike } from '../transport/tool-presenter.ts';
import type { QQBotSender } from '../transport/outbound-buffer.ts';
import { QuestionChannel } from '../features/question-channel.ts';
import { ApprovalChannel, type ApprovalChannelContext } from '../features/approval-channel.ts';
import { decodeButtonData } from '../features/button-utils.ts';
import { buildUserAgent } from '../shared/index.ts';
import type { ImQQBotConfig } from '../config.ts';
import type { ChatScope, Logger } from '../types.ts';
import { setupMiddlewares } from './middleware-setup.ts';
import { readAccountInfo, type BotAccountInfo } from './accountInfo.ts';
import type { MediaSenderLike } from '../media/send-media.ts';
import { DeliverablesDeliverer } from '../transport/deliverables.ts';
import type { DeliveryTargetResolver } from '../transport/turn-tracker.ts';
import { resolveMediaRoot } from '../shared/dirs.ts';
import { ensureBotWorkspace } from '../workspace/index.ts';
import { ensureBridgeEntity } from '../shared/astrboxBridge.ts';
import { migrateBotWorkspace } from '../workspace/migrate.ts';
import { pruneOrphanSessions } from '../session/pruneSessions.ts';

/**
 * interaction 统一分发器：按 button_data 的顶层 `t` 判别字段，路由到对应
 * channel（question / approval）。QQ 只有一个 interaction 入口，集中路由
 * 避免多通道各自 try 导致误判。
 */
function dispatchInteraction(event: InteractionEvent, manager: SessionManager): boolean {
  const raw = event.data?.resolved?.button_data;
  if (!raw) return false;
  const button = decodeButtonData(raw);
  if (!button) return false;
  if (button.t === 'question') return manager.questionChannel?.handleInteraction(event) ?? false;
  if (button.t === 'approval') return manager.approvalChannel?.handleInteraction(event) ?? false;
  return false;
}

/** 一个已装配机器人的运行时句柄（BotCoordinator 收集并用于工具归属解析）。 */
export interface BotRuntimeHandle {
  config: ImQQBotConfig;
  manager: SessionManager;
  /** 回合投递目标解析（qqbot_send_file 等工具用它定位当前会话） */
  targets: DeliveryTargetResolver;
  /** 媒体发送器（附件/图片/视频/语音） */
  mediaSender: MediaSenderLike;
  /** markdown 发送器（K15 `qqbotBots.say` 主动直发复用同一通道/限额包装） */
  sender: QQBotSender;
  /** 该 bot 的富媒体下载根目录（多 bot 时按 bots/<id>/media 隔离） */
  mediaRoot: string;
  /** 平台账号身份（README/列表展示；网关 READY 后填充，连接前为空对象） */
  account: BotAccountInfo;
}

/**
 * 迁移后更新侧边栏注册表：删掉旧 cwd 的工作区项、登记新 cwd（fail-soft）。
 * @param ctx - 插件上下文
 * @param from - 旧 cwd（无记录时 undefined）
 * @param to - 新 cwd
 * @param title - 工作区标题
 * @param logger - 日志器
 */
async function cleanupMigratedWorkspace(
  ctx: Context,
  from: string | undefined,
  to: string,
  title: string,
  logger: Logger,
): Promise<void> {
  try {
    const accessor = (ctx as unknown as { get?: (name: string) => unknown }).get;
    const registry = (typeof accessor === 'function' ? accessor.call(ctx, 'workspaceRegistry') : undefined) as {
      list?: () => ReadonlyArray<{ id?: string; path?: string }>;
      delete?: (id: string) => Promise<boolean>;
      create?: (path: string, title?: string) => Promise<unknown>;
    } | undefined;
    if (registry?.list !== undefined && typeof registry.delete === 'function' && from !== undefined) {
      const old = registry.list().find(item => item.path === from);
      if (old?.id !== undefined) await registry.delete(old.id);
    }
    if (typeof registry?.create === 'function') await registry.create(to, title);
  } catch (error) {
    logger.warn(`im-qqbot: 工作区注册表更新失败（继续）: ${error instanceof Error ? error.message : String(error)}`);
  }
}

/**
 * 装配单个机器人：QQBot SDK + 中间件 + 入站/出站 + 问答/审批通道 + 生命周期。
 * 只负责一个 bot 的运行时；全局工具注册由 BotCoordinator 统一完成（多机器人只注册一次）。
 */
export function createBotRuntime(
  ctx: Context,
  agents: DshAgentRegistry,
  config: ImQQBotConfig,
  logger: Logger,
  botId?: string,
  legacy = false,
  workspaceTitle?: string,
): BotRuntimeHandle {
  // legacy 单 bot 不传 botId：workspace 保持旧全局目录（与 K1 qqbotBots.bindings 的
  // cwd 公式同源，见 shared/dirs.ts）。多机器人按 botId 隔离到 bots/<id>/workspace。
  // workspaceTitle 让侧边栏工作区按机器人名/绑定实体名显示（缺省回落 QQBot）。
  // 新增/加载机器人：按模板初始化工作区（幂等；legacy 单 bot 保持旧行为不初始化）。
  if (botId !== undefined && !legacy) {
    // 实体/工作目录变化时先重命名旧工作区与会话存储，再按最终 cwd 建模板（不留孤儿目录）。
    const migration = migrateBotWorkspace({ botId, cwdConfig: config.cwd, entity: config.entity, logger });
    if (migration.moved) {
      void cleanupMigratedWorkspace(ctx, migration.from, migration.to, workspaceTitle?.trim() || 'QQBot', logger);
    }
    try {
      const init = ensureBotWorkspace(botId, { cwdConfig: config.cwd, entity: config.entity });
      if (init.workspace.created.length > 0 || init.neko.deployed.length > 0) {
        logger.info(
          `im-qqbot: 机器人 ${botId} 工作区 ${init.workspaceDir} 就绪`
            + `（新建 ${init.workspace.created.length} 项，Neko 部署 ${init.neko.deployed.length} 项）`,
        );
      }
      if (init.neko.missing.length > 0) {
        logger.warn(`im-qqbot: 机器人 ${botId} Neko 部署缺项: ${init.neko.missing.join(', ')}`);
      }
    } catch (error) {
      logger.warn(`im-qqbot: 机器人 ${botId} 工作区初始化失败（继续启动）: ${error instanceof Error ? error.message : String(error)}`);
    }
  }
  // K33：实体名称即访问路径，组合类型由内容层 `@实例` 定义；绑定后按名称幂等确保存在。
  if (config.entity !== undefined && config.entity.trim() !== '') {
    void ensureBridgeEntity(ctx, config.entity, logger);
  }
  const manager = new SessionManager(ctx, agents, config, logger, legacy ? undefined : botId, workspaceTitle);
  // 主会话（Neko 主体）：每 bot 一个常驻父会话，QQ 私聊/群聊子会话挂到它下面。
  // 主会话就绪后再清理该工作区里不再被引用的旧会话（等 prefs/主会话都落定）。
  void manager.ensureMainSession().then(() => {
    try {
      pruneOrphanSessions({
        workspaceDir: manager.workspaceDirectory,
        keep: new Set(manager.knownSessionIds()),
        logger,
      });
    } catch (error) {
      logger.warn(`im-qqbot: 遗弃会话清理失败（继续）: ${error instanceof Error ? error.message : String(error)}`);
    }
  });
  const mediaRoot = resolveMediaRoot(botId, legacy, config.entity);

  // ── 回合归属：入站登记 + 回合跟踪，出站/提问/审批/发文件共享同一投递目标解析 ──
  const qqInputs = new QqInputRegistry({ ttlMs: inputOriginTtlMs(config.processingTimeoutMs) });
  const turnTracker = createTurnTracker(manager, config, logger, qqInputs);

  // ── 初始化 QQ Bot SDK ──
  const userAgent = buildUserAgent();
  // 账号身份：READY 事件到达后填充（列表/绑定面板据此显示平台昵称）
  const account: BotAccountInfo = {};
  const bot = new QQBot({
    appId: config.appId,
    appSecret: config.appSecret,
    transport: 'websocket',
    baseUrl: process.env.QQBOT_BASE_URL?.replace(/\/+$/, '') || 'https://api.bot.qq.com',
    tokenBaseUrl: process.env.QQBOT_TOKEN_BASE_URL?.replace(/\/+$/, '') || 'https://api.bot.qq.com',
    userAgent,
    logger,
  } as ConstructorParameters<typeof QQBot>[0]);
  logger.info(`QQBot SDK initialized (UA: ${userAgent})`);

  // ── 中间件链 ──
  setupMiddlewares(bot, config, manager, logger, mediaRoot);

  // ── 入站：经过中间件链后的消息交给 dsh agent ──
  bot.on('message', async (mCtx: MiddlewareContext) => {
    const msg = mCtx.message;
    // 记录最近 msgId，供无上下文的出站发送（文件发送显式 target、主动推送等）获取被动回复目标
    cacheMsgId(mCtx.replyTarget.scope, mCtx.replyTarget.targetId, mCtx.replyTarget.msgId);
    if (config.debug) {
      logger.debug(`← message (post-middleware): ${JSON.stringify(msg, null, 2).slice(0, 500)}`);
    }
    await handleInbound(mCtx, manager, config, logger, qqInputs);
  });

  // ── 出站：dsh session/event → QQ 消息 ──
  // 获取 tools 服务（工具结果结构化展示），可选
  let toolsRegistry: ToolsRegistryLike | undefined;
  try {
    toolsRegistry = ctx.get('tools') as ToolsRegistryLike | undefined;
  } catch {
    toolsRegistry = undefined;
  }

  // 发送适配器：将 QQBot 实例适配为 QQBotSender（openStream 参数形态不同）。
  // sendMarkdown 统一做被动回复限额管控：同一 msgId 超限/过期时降级主动推送。
  const replyLimiter = new ReplyLimiter({ limit: 4 });
  const sender: QQBotSender = {
    sendMarkdown: (target, content, opts) => sendResolvedMarkdown(bot, resolveReplyTarget(target, replyLimiter, true), content, opts),
    openStream: (target) => bot.openStream({
      target: {
        scope: target.scope,
        targetId: target.targetId,
        msgId: target.msgId as string,
      },
    }),
  };

  // ── 附件发送适配器（qqbot_send_file 工具与交付物投递共用） ──
  // 包装 bot：让文件发送也走 msgId 兜底 + 被动回复限额管控。
  // 注意：SDK 文件发送暂不支持 event_id，故 allowEvent=false，跳过 event 候选。
  const mediaSender: MediaSenderLike = {
    sendImage: (target, source) => bot.sendImage(resolveReplyTarget(target, replyLimiter, false), source),
    sendVideo: (target, source) => bot.sendVideo(resolveReplyTarget(target, replyLimiter, false), source),
    sendVoice: (target, source) => bot.sendVoice(resolveReplyTarget(target, replyLimiter, false), source),
    sendFile: (target, source, opts) => bot.sendFile(resolveReplyTarget(target, replyLimiter, false), source, opts),
  };

  // 交付物投递：present 声明的文件在轮末发送到当前会话（outbound 路由触发）
  const deliverablesDeliverer = new DeliverablesDeliverer(sender, mediaSender, config, logger, botId, mediaRoot);

  const outbound = createOutboundHandler(
    manager, sender, config, logger, toolsRegistry, deliverablesDeliverer, turnTracker,
  );
  // session/event 按会话作用域派发。根作用域监听不带 global 收不到
  // 桌面恢复出来的 agent，主动回合的 user/message 会直接丢。
  // 全局监听下非 QQ 会话在 TurnTracker.begin 即被过滤，不进入回合状态表。
  (ctx as unknown as { on(event: string, handler: (...args: unknown[]) => void, options?: { global?: boolean }): void })
    .on('session/event', outbound.routeSessionEvent as (...args: unknown[]) => void, { global: true });

  // ── 出站流式：agent/assistant-stream → 流式帧路由 ──
  // 实时流式走瞬态 agent/assistant-stream（chunk 不进 session log）。
  // 该事件按 agent 作用域派发，根作用域监听器不带 global 会被过滤（同
  // approval/request），帧到达后自行按 agent.id 路由，非 QQ 会话在 router 内忽略。
  (ctx as unknown as { on(event: string, handler: (...args: unknown[]) => void, options?: { global?: boolean }): void })
    .on('agent/assistant-stream', (payload: unknown) => {
      const parsed = parseStreamFrame(payload);
      if (parsed !== undefined) outbound.routeStreamFrame(parsed.agentId, parsed.delta);
    }, { global: true });

  // ── 问答通道（ask_user_question → QQ 文本 + 按钮） ──
  const questionChannel = new QuestionChannel(manager, sender, config, logger, turnTracker);
  manager.questionChannel = questionChannel;
  questionChannel.install(ctx);

  // ── 审批通道（approval/request → QQ 两按钮审批） ──
  const approvalChannel = new ApprovalChannel(manager, sender, config, logger, turnTracker);
  manager.approvalChannel = approvalChannel;
  approvalChannel.install(ctx as unknown as ApprovalChannelContext);

  // ── 按钮点击回调（interaction）：统一分发到问答/审批通道 ──
  bot.on('interaction', async (_iCtx: unknown, event: InteractionEvent) => {
    // scope/targetId 推导对齐消息事件（inbound.ts）：按 scene 判定类型，群用 group_openid，私聊用 user_openid
    const scope: ChatScope = event.scene === 'group' ? 'group' : 'c2c';
    const targetId = scope === 'group' ? (event.group_openid ?? event.user_openid) : event.user_openid;
    // 记录互动事件 id，作为后续被动回复候选（event_id）
    cacheEventId(scope, targetId, event.id);

    const matched = dispatchInteraction(event, manager);
    await bot.acknowledgeInteraction(event.id, matched ? 0 : 3).catch(() => {});
  });

  bot.on('error', (err: unknown) => {
    logger.error(`bot error: ${err instanceof Error ? err.message : String(err)}`);
  });

  // 掉线可观测：willReconnect=false 表示不会再重连（随后 bot.start() 的 Promise 会失败）。
  // disconnected 事件为 SDK 1.0.5+ 新增：旧版 handlers 表缺该键，bot.on 会抛
  // TypeError（profiles 增量安装可能残留旧版 SDK）——降级为跳过监听而非崩溃
  try {
    bot.on('disconnected', (event: GatewayDisconnect) => {
      const detail = `code=${event.code}${event.reason ? ` reason=${event.reason}` : ''}`;
      if (event.willReconnect) {
        logger.warn(`Bot disconnected (${detail}), reconnecting...`);
      } else {
        logger.error(`Bot disconnected permanently (${detail}), giving up`);
      }
    });
  } catch (err) {
    logger.warn(`qqbot-nodejs SDK 不支持 disconnected 事件（请升级 @tencent-connect/qqbot-nodejs 至 1.0.5+）: ${err instanceof Error ? err.message : String(err)}`);
  }

  bot.on('ready', (data: unknown) => {
    Object.assign(account, readAccountInfo(data));
    console.log(`[im-qqbot] Bot ready! appId=${config.appId}${account.name === undefined ? '' : ` name=${account.name}`}`);
  });

  // ── 生命周期 ──
  (ctx as unknown as { effect(fn: () => (() => Promise<void>) | void, name?: string): void })
    .effect(() => {
      logger.info(`Starting bot (appId=${config.appId})`);
      bot.start().catch((err: unknown) => {
        logger.error(`Bot start failed: ${err instanceof Error ? err.message : String(err)}`);
      });

      return async () => {
        logger.info('Shutting down');
        await manager.disposeAll();
        bot.stop();
      };
    }, 'im-qqbot.lifecycle');

  return { config, manager, targets: turnTracker, mediaSender, mediaRoot, account, sender };
}
