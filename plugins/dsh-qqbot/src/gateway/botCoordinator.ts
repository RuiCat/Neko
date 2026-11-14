/**
 * 多机器人运行时协调器（P2）。
 *
 * 为每个启用的 bot 装配独立运行时（createBotRuntime），并保证全局资源只注册一次：
 * - `qqbot_send_file` / `qqbot_describe_image` 工具名全局唯一，执行时按调用会话的
 *   agent → SessionManager 归属解析到具体机器人；
 * - 视觉 settings.yaml 模态补写按 (provider, model) 去重后各执行一次；
 * - 富媒体清理在同一插件根下只启动一份（P4 再按 bot 分目录）。
 */
import type { Context } from '@deepseek-ai/cordis';
import type { EffectiveBot } from '../configBots.ts';
import type { ImQQBotConfig } from '../config.ts';
import type { DshAgentRegistry } from '../session/index.ts';
import type { DshAgent } from '../session/index.ts';
import type { Logger } from '../types.ts';
import { createBotRuntime, type BotRuntimeHandle } from './bootstrap.ts';
import { provideQQBotBots, snapshotBindings, type QQBotBotsBinding } from './qqbotBotsService.ts';
import { refreshBridge } from '../shared/astrboxBridge.ts';
import type { QqSessionWire } from '../shared/bind-protocol.ts';
import { registerSendFileTool } from '../media/send-file-tool.ts';
import { registerBotsTool } from './botsTool.ts';
import { ensureVisionInputModal, registerDescribeImageTool } from '../media/vision-tool.ts';
import { startMediaCleanup } from '../media/media-cleaner.ts';

/** 协调器持有的一个机器人：运行时句柄 + 配置身份。 */
export interface CoordinatedBot extends BotRuntimeHandle {
  /** 配置里的稳定 bot id */
  id: string;
  /** 展示名 */
  label: string;
  /** true = 旧版顶层单 bot 配置（workspace 缺省保持旧全局目录，见 shared/dirs.ts） */
  legacy: boolean;
}

/** startBotCoordinator 的可测试注入点。 */
export interface StartBotOptions {
  /** 替换真实 QQBot 运行时工厂（单测用；默认 createBotRuntime） */
  createRuntime?: (config: ImQQBotConfig, botId: string, legacy: boolean, workspaceTitle?: string) => BotRuntimeHandle;
}

/** 工作区显示名：绑定实体名 > bot label > 插件旧默认名（与 K10 一致）。 */
function workspaceTitleOf(bot: EffectiveBot): string {
  const entity = bot.config.entity?.trim();
  if (entity !== undefined && entity !== '') return entity;
  return bot.label.trim() !== '' ? bot.label : 'QQBot';
}

/** 多机器人查询与归属解析（工具执行时使用）。 */
export class BotCoordinator {
  private readonly bots: CoordinatedBot[] = [];

  /**
   * @param logger - 插件日志器
   */
  public constructor(private readonly logger: Logger) {}

  /**
   * 登记一个已启动的机器人。
   * @param bot - 机器人运行时
   * @throws 当 id 重复
   */
  public add(bot: CoordinatedBot): void {
    if (this.bots.some(existing => existing.id === bot.id)) {
      throw new Error(`im-qqbot: 机器人 id 重复: ${bot.id}`);
    }
    this.bots.push(bot);
    this.logger.info(`im-qqbot: 机器人 ${bot.id} 已登记 (appId=${bot.config.appId})`);
  }

  /** 已登记的机器人（顺序 = 启动顺序）。 */
  public list(): readonly CoordinatedBot[] {
    return this.bots;
  }

  /** 已登记数量。 */
  public get size(): number {
    return this.bots.length;
  }

  /**
   * 该 bot 的平台账号昵称（网关 READY 后可用；连接前/未知时 undefined）。
   * @param botId - 配置里的稳定 bot id
   * @returns 平台昵称
   */
  public nameOf(botId: string): string | undefined {
    return this.bots.find(bot => bot.id === botId)?.account.name;
  }

  /**
   * 按 agent 找归属机器人（工具执行上下文里只有 agent）。
   * @param agent - dsh agent 对象
   * @returns 归属机器人；不属于任何 QQ 会话时为 undefined
   */
  public byAgent(agent: unknown): CoordinatedBot | undefined {
    if (agent === undefined || agent === null) return undefined;
    for (const bot of this.bots) {
      if (bot.manager.findByAgent(agent as DshAgent) !== undefined) return bot;
    }
    return undefined;
  }

  /**
   * 按 sessionId 找归属机器人。
   * @param sessionId - 会话 id
   * @returns 归属机器人；不属于任何 QQ 会话时为 undefined
   */
  public bySession(sessionId: string): CoordinatedBot | undefined {
    for (const bot of this.bots) {
      if (bot.manager.findBySessionId(sessionId) !== undefined) return bot;
    }
    return undefined;
  }

  /** 默认机器人（启动顺序第一个）：仅用于无法按会话定位时的显式 target 兜底。
   * @returns 默认机器人；一个都没启动时为 undefined
   */
  public fallback(): CoordinatedBot | undefined {
    return this.bots[0];
  }

  /** 按 bot id 查找运行中的机器人。 */
  private find(botId: string): CoordinatedBot | undefined {
    return this.bots.find(bot => bot.id === botId);
  }

  /**
   * 该 bot 的主会话 id（未启动/未就绪时 undefined；`/qqbot/bots` 用）。
   * @param botId - 配置里的稳定 bot id
   * @returns 主会话 id 或 undefined
   */
  public mainSessionIdOf(botId: string): string | undefined {
    return this.find(botId)?.manager.mainSessionId;
  }

  /**
   * 该 bot 的 QQ 子会话（K4；未运行的 bot 返回 undefined → wire 省略字段）。
   * @param botId - 配置里的稳定 bot id
   * @returns 子会话描述数组；bot 未运行时 undefined
   */
  public qqSessionsOf(botId: string): ReadonlyArray<QqSessionWire> | undefined {
    const bot = this.find(botId);
    if (bot === undefined) return undefined;
    try {
      return bot.manager.listQqSessions();
    } catch {
      return undefined;
    }
  }

  /**
   * 运行中机器人的实体绑定快照（K1；内部异常回落空数组）。
   * @returns 绑定项新数组
   */
  public bindings(): ReadonlyArray<QQBotBotsBinding> {
    try {
      return snapshotBindings(this.bots);
    } catch {
      return [];
    }
  }
}

/**
 * 注册一次性全局工具：send_file 按归属机器人发送，describe_image 按归属机器人取视觉配置。
 * @param ctx - 插件上下文
 * @param coordinator - 已登记运行时
 * @param logger - 日志器
 * @returns void
 */
function registerBotTools(ctx: Context, coordinator: BotCoordinator, logger: Logger): void {
  registerSendFileTool(ctx, {
    byAgent: agent => coordinator.byAgent(agent),
    fallback: () => coordinator.fallback(),
  }, logger);

  registerDescribeImageTool(ctx, {
    byAgent: agent => coordinator.byAgent(agent)?.config.vision,
    fallback: () => coordinator.fallback()?.config.vision,
    anyEnabled: () => coordinator.list().some(bot => bot.config.vision.enabled),
  }, logger);

  // K25：机器人与会话目录工具（与 GET /qqbot/bots 同源；全局只注册一次）。
  registerBotsTool(ctx, {
    list: () => coordinator.list(),
    qqSessionsOf: botId => coordinator.qqSessionsOf(botId),
  }, logger);
}

/**
 * 注册一次性后台资源：视觉模态补写按模型去重、媒体清理只启一份。
 * @param ctx - 插件上下文
 * @param coordinator - 已登记运行时
 * @param logger - 日志器
 * @returns void
 */
function registerSharedBackground(ctx: Context, coordinator: BotCoordinator, logger: Logger): void {
  const seenVision = new Set<string>();
  for (const bot of coordinator.list()) {
    const vision = bot.config.vision;
    if (!vision.enabled || !vision.provider || !vision.model) continue;
    const key = `${vision.provider}/${vision.model}`;
    if (seenVision.has(key)) continue;
    seenVision.add(key);
    ensureVisionInputModal(vision, logger);
  }

  // 媒体清理：每个 bot 各自根目录各起一份（legacy 单 bot 仍是旧全局目录）
  for (const bot of coordinator.list()) {
    if (bot.config.media.enabled) {
      startMediaCleanup(ctx, bot.config.media.ttlHours, logger, bot.mediaRoot);
    }
  }
}

/**
 * 启动一组机器人：逐个装配运行时、登记归属，然后只注册一次全局工具与后台资源。
 * 单个 bot 启动失败只记 error，不拖垮其它 bot。
 * @param ctx - 插件上下文
 * @param agents - dsh agent 注册表
 * @param bots - 生效机器人列表（通常已过滤 enabled）
 * @param logger - 日志器
 * @param options - 测试注入点
 * @returns 已启动机器人的协调器
 */
// 启动入口：单个 bot 失败隔离，全局工具与后台资源只注册一次。
export function startBotCoordinator(
  ctx: Context,
  agents: DshAgentRegistry,
  bots: readonly EffectiveBot[],
  logger: Logger,
  options: StartBotOptions = {},
): BotCoordinator {
  const coordinator = new BotCoordinator(logger);
  const createRuntime = options.createRuntime
    ?? ((config: ImQQBotConfig, botId: string, legacy: boolean, workspaceTitle?: string) =>
      createBotRuntime(ctx, agents, config, logger, botId, legacy, workspaceTitle));

  for (const bot of bots) {
    if (!bot.enabled) {
      logger.info(`im-qqbot: 机器人 ${bot.id} 未启用，跳过`);
      continue;
    }
    try {
      const runtime = createRuntime(bot.config, bot.id, bot.legacy, workspaceTitleOf(bot));
      coordinator.add({ id: bot.id, label: bot.label, legacy: bot.legacy, ...runtime });
    } catch (error) {
      logger.error(`im-qqbot: 机器人 ${bot.id} 启动失败: ${error instanceof Error ? error.message : String(error)}`);
    }
  }

  // K1：向桥接暴露绑定快照；没有可启动机器人也提供服务（bindings 为空数组而非旧值）。
  provideQQBotBots(ctx, () => coordinator.list(), logger);

  if (coordinator.size === 0) {
    logger.warn('im-qqbot: 没有可启动的机器人（检查 bots[]/appId 或被禁用）');
    return coordinator;
  }

  registerBotTools(ctx, coordinator, logger);
  registerSharedBackground(ctx, coordinator, logger);
  // K3：运行时启动后预热人设缓存（fire-and-forget，失败静默，不阻塞启动）。
  refreshBridge(ctx);
  return coordinator;
}
