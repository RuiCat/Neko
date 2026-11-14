/**
 * 凭据绑定编排 — setup/ 模块入口
 *
 * apply() 检测到凭据缺失时调用 ensureCredentials：
 * 1. 创建 BindingService（单例接管，HMR 双 fiber 安全）
 * 2. Web 宿主（dsh web / Desktop）注册端内绑定页路由
 *    —— 经 ctx.inject(['webServer'], ...) 可选注入：服务未就绪时插件
 *       照常加载（CLI profile），就绪/替换时自动（重）注册路由
 * 3. 按宿主形态挂呈现策略（终端 / 浏览器降级）
 * 4. 生命周期与 ctx fiber 绑定（dispose 时 abort 轮询、注销路由）
 *
 * 凭据已配置的 fiber 调用 registerBoundCredentialRoutes 注册常驻路由
 * （bound 状态回显 + 手动编辑），与扫码流程共用持久化路径。
 */
import type { Context } from '@deepseek-ai/cordis';
import type { Logger } from '../types.ts';
import { BindingService } from './binding-service.ts';
import { BotBindingRegistry, type BotBindingServiceFactory } from './botBindingRegistry.ts';
import {
  registerBindRoutes,
  registerBotsListRoute,
  type ManualSubmitResult,
  type UnbindResult,
  type WebServerLike,
} from './bind-routes.ts';
import { botBindPaths, type QqSessionWire } from '../shared/bind-protocol.ts';
import { createBrowserPresenter, createTerminalPresenter, isDesktopHost } from './presenters.ts';
import {
  addBotToProfile,
  persistBotCredentialRefToProfile,
  persistBotCredentialsToProfile,
  removeBotCredentialsFromProfile,
  removeBotFromProfile,
  updateBotProfileConfig,
} from './persistBots.ts';
import { persistCredentialsToProfile, type SetupCredentials } from './persist.ts';
import {
  botSecretRef,
  isSecretRef,
  removeBotSecret,
  storeBotSecret,
  type CredentialsLike,
} from './credentialStore.ts';
import { getProfileDir } from '../shared/index.ts';

export type { SetupCredentials } from './persist.ts';
export { persistCredentialsToProfile } from './persist.ts';
export { isDesktopHost } from './presenters.ts';

/** 进程内活动绑定服务（HMR 接管用） */
let activeService: BindingService | null = null;

/** ensureCredentials 参数 */
export interface EnsureCredentialsOptions {
  logger: Logger;
  /** 二维码 URL 的 source 标识 */
  source?: string;
  /** 当前生效配置（apply 时 loader 合并值；config 路由 GET 回显用） */
  config: Record<string, unknown>;
}

/**
 * 执行扫码绑定编排，返回凭据就绪的 Promise；失败/取消 resolve null。
 *
 * 【同步契约】函数体必须保持无 await——绑定编排（服务接管、路由注册、
 * 呈现策略、ctx 生命周期挂载）在调用后的同步阶段全部完成，随后返回
 * `service.start()` 的挂起 Promise。调用方（插件 apply）因此可以不等待
 * 扫码结果直接完成加载：boot 的 `loader.await()` 会等所有插件 fiber
 * settle，若 apply 无限期等待用户扫码，Host ready 会被阻塞
 * （Desktop 界面无法进入、Web URL 延迟输出）。
 *
 * 凭据持久化由调用方在 Promise 的 then 链中执行——成功写入
 * cordis.patch.yml 会触发 HMR 热重载，新 fiber 以凭据启动网关。
 */
export async function ensureCredentials(
  ctx: Context,
  options: EnsureCredentialsOptions,
): Promise<SetupCredentials | null> {
  const logger = options.logger;
  const source = options.source ?? 'dsh-qqbot';

  // 单例接管：HMR 双 fiber 窗口下，新实例终止旧实例的轮询
  const service = new BindingService({ source, logger });
  if (activeService !== null && activeService !== service) {
    activeService.dispose();
  }
  activeService = service;

  // 生命周期：fiber dispose 时 abort 轮询
  (ctx as unknown as { effect(fn: () => () => void, name?: string): void }).effect(() => () => {
    service.dispose();
    if (activeService === service) activeService = null;
  }, 'im-qqbot.binding');

  // 呈现策略：
  // - 终端通道（CLI / dsh web 运营者）；Desktop 无可见终端，跳过
  // - 浏览器降级通道（QQBOT_BIND_OPEN_BROWSER=1 显式启用）
  if (!isDesktopHost()) {
    service.attach(createTerminalPresenter());
  }
  if (process.env.QQBOT_BIND_OPEN_BROWSER === '1') {
    service.attach(createBrowserPresenter(logger));
  }

  // 端内绑定页路由：webServer 可选注入。
  // cordis 访问守卫要求服务经 inject 声明后方可访问；deps 未就绪时
  // 回调不运行但插件照常加载（CLI profile），服务就绪/替换时回调
  // 重跑（自动清理旧路由、注册新路由），与 webserver 的注入生命周期对齐。
  (ctx as unknown as {
    inject(deps: readonly string[], callback: (scoped: Context) => void): unknown;
  }).inject(['webServer'], (scoped) => {
    const webServer = (scoped as unknown as { webServer?: unknown }).webServer as WebServerLike | undefined;
    if (webServer === undefined || typeof webServer.register !== 'function') return;
    const disposeRoutes = registerBindRoutes(webServer, service, logger, {
      // 未绑定流程中的手动提交：终止扫码轮询，走同一持久化路径
      onManualCredentials: (credentials) => {
        service.dispose();
        return persistCredentials(credentials, logger);
      },
      currentConfig: () => options.config,
    });
    // 回调作用域（服务卸载/替换、插件卸载）连带注销路由
    (scoped as unknown as { effect(fn: () => () => void, name?: string): void }).effect(() => {
      return disposeRoutes;
    }, 'im-qqbot.bind-routes');
  });

  return service.start();
}

/**
 * 凭据已配置的 fiber 注册常驻路由（状态回显 + 手动编辑/更换）。
 *
 * 在 apply 的 bootstrapGateway 之前调用；写入经同一 persist 路径，
 * 保存后触发 HMR 重载、新 fiber 以新凭据重启网关。
 */
export function registerBoundCredentialRoutes(ctx: Context, logger: Logger, appId: string, config: Record<string, unknown>): void {
  (ctx as unknown as {
    inject(deps: readonly string[], callback: (scoped: Context) => void): unknown;
  }).inject(['webServer'], (scoped) => {
    const webServer = (scoped as unknown as { webServer?: unknown }).webServer as WebServerLike | undefined;
    if (webServer === undefined || typeof webServer.register !== 'function') return;
    const disposeRoutes = registerBindRoutes(webServer, null, logger, {
      boundAppId: () => appId,
      onManualCredentials: (credentials) => persistCredentials(credentials, logger),
      currentConfig: () => config,
    });
    (scoped as unknown as { effect(fn: () => () => void, name?: string): void }).effect(() => {
      return disposeRoutes;
    }, 'im-qqbot.bind-routes');
  });
}

/** 多机器人路由目标：每个 bot 一组 /qqbot/bots/<id>/... 路由。 */
export interface BotRouteTarget {
  /** 配置里的稳定 bot id */
  id: string;
  /** 展示名 */
  label?: string;
  /** 当前 appId（状态回显用，非机密；未绑定为空串） */
  appId: string;
  /** 配置里的启用状态（false = 未绑定凭据或被显式停用，不启动运行时） */
  enabled: boolean;
  /** 凭据库引用（bots[] 声明了 appSecretRef 时存在；解绑时一并清理） */
  appSecretRef?: string;
  /** 当前生效配置（config GET 回显；appSecret 由路由层剔除） */
  config: Record<string, unknown>;
  /** 平台账号昵称（网关 READY 后可用） */
  accountName?: () => string | undefined;
  /** 绑定的 NPC 实体 id（配置推导；未绑定时省略该字段，K4） */
  entity?: string;
  /** agent 工作目录（配置推导，与 SessionManager 同源，K4） */
  cwd?: string;
  /** 主会话 id（运行中且已就绪时返回；否则 undefined → 省略字段） */
  mainSessionId?: () => string | undefined;
  /** 该 bot 的 QQ 子会话（运行中返回数组；未运行返回 undefined → 省略字段） */
  qqSessions?: () => ReadonlyArray<QqSessionWire> | undefined;
}

/** 多机器人路由的注入点（扫码服务工厂 / 凭据库 / 实体目录来源）。 */
export interface BotCredentialRouteOptions {
  /** 替换扫码服务工厂（单测用） */
  createService?: BotBindingServiceFactory;
  /** 凭据库适配层（未安装时 undefined） */
  credentials?: CredentialsLike;
  /** NPC 实体目录来源（桥接服务可用时返回数组；不可用返回 undefined → 省略字段，K8） */
  entities?: () => ReadonlyArray<string> | undefined;
}

/**
 * 读取实体目录：来源异常按「服务不可用」处理（K8：省略字段，路由仍 200）。
 * @param source - 实体目录来源
 * @returns 实体 id 数组；不可用时 undefined
 */
function readEntitiesSafe(source?: () => ReadonlyArray<string> | undefined): ReadonlyArray<string> | undefined {
  if (source === undefined) return undefined;
  try {
    return source();
  } catch {
    return undefined;
  }
}

/**
 * 注册多机器人常驻路由：每 bot 一组「状态 / 扫码 / 手动凭据 / 解绑 / 配置」端点。
 * 未绑定的 bot（appId 为空）也会注册，扫码服务由 BotBindingRegistry 首次轮询时懒创建。
 * @param ctx - 插件上下文
 * @param logger - 日志
 * @param bots - 配置里的全部机器人（含未绑定/停用项）
 * @param options - 测试注入点（扫码服务工厂）、凭据库适配层与实体目录来源
 * @returns void
 */
// 多机器人路由：路径按 bot id 参数化，写入都落在该 bot 的 bots[] 条目上。
export function registerBotCredentialRoutes(
  ctx: Context,
  logger: Logger,
  bots: readonly BotRouteTarget[],
  options: BotCredentialRouteOptions = {},
): void {
  if (bots.length === 0) return;
  (ctx as unknown as {
    inject(deps: readonly string[], callback: (scoped: Context) => void): unknown;
  }).inject(['webServer'], (scoped) => {
    const webServer = (scoped as unknown as { webServer?: unknown }).webServer as WebServerLike | undefined;
    if (webServer === undefined || typeof webServer.register !== 'function') return;
    const store = options.credentials;
    const registry = new BotBindingRegistry({
      logger,
      persist: async (botId, credentials) => {
        const target = bots.find(bot => bot.id === botId);
        const result = await persistBotCredentials(botId, credentials, logger, store, target?.appSecretRef);
        return result.persisted;
      },
      ...(options.createService === undefined ? {} : { createService: options.createService }),
    });
    const disposers: Array<() => void> = [
      // K4/K8：只增字段（entity/cwd/mainSessionId/sessions + 顶层 entities），旧字段不变、不含密钥。
      registerBotsListRoute(webServer, logger, () => {
        const entities = readEntitiesSafe(options.entities);
        return {
          bots: bots.map((bot) => {
            const name = bot.accountName?.();
            const mainSessionId = bot.mainSessionId?.();
            const qqSessions = bot.qqSessions?.();
            return {
              id: bot.id,
              label: bot.label ?? bot.id,
              appId: bot.appId,
              enabled: bot.enabled,
              ...(name === undefined ? {} : { name }),
              ...(bot.entity === undefined || bot.entity === '' ? {} : { entity: bot.entity }),
              ...(bot.cwd === undefined || bot.cwd === '' ? {} : { cwd: bot.cwd }),
              ...(mainSessionId === undefined ? {} : { mainSessionId }),
              ...(qqSessions === undefined ? {} : { sessions: [...qqSessions] }),
            };
          }),
          ...(entities === undefined ? {} : { entities: [...entities] }),
        };
      }, {
        onCreate: (id, label) => persistNewBot(id, label, logger),
        onDelete: (id) => deleteBot(id, logger),
      }),
    ];
    disposers.push(...bots.map((bot) => registerBindRoutes(webServer, () => (bot.appId === '' ? registry.serviceFor(bot.id) : null), logger, {
      boundAppId: () => (bot.appId === '' ? null : bot.appId),
      currentConfig: () => bot.config,
      onManualCredentials: (credentials) => persistBotCredentials(bot.id, credentials, logger, store, bot.appSecretRef),
      onUnbind: () => removeBotCredentials(bot.id, logger, store, bot.appSecretRef),
      ...(bot.accountName === undefined ? {} : { accountName: bot.accountName }),
      unavailableReason: () => (bot.appId !== '' && !bot.enabled
        ? '机器人已停用或凭据未加载（检查 appSecretRef 与凭据库）'
        : undefined),
      onUpdateConfig: (config) => updateBotProfileConfig(bot.id, config, getProfileDir() ?? undefined, logger),
      paths: botBindPaths(bot.id),
    })));
    (scoped as unknown as { effect(fn: () => () => void, name?: string): void }).effect(() => {
      return () => { registry.dispose(); for (const dispose of disposers) dispose(); };
    }, 'im-qqbot.bot-bind-routes');
  });
}

/**
 * 机器人凭据持久化：凭据库可用时只写引用（密钥不落 patch），否则回退明文。
 * @param botId - 配置里的稳定 bot id
 * @param credentials - 扫码/手动提交的凭据
 * @param logger - 日志
 * @param store - 凭据库适配层（未安装时 undefined）
 * @param appSecretRef - 该 bot 已声明的引用（缺省用 QQBOT_<ID>_SECRET）
 * @returns 可展示的持久化结果
 */
async function persistBotCredentials(
  botId: string,
  credentials: SetupCredentials,
  logger: Logger,
  store: CredentialsLike | undefined,
  appSecretRef: string | undefined,
): Promise<ManualSubmitResult> {
  const profileDir = getProfileDir() ?? undefined;
  const ref = appSecretRef !== undefined && isSecretRef(appSecretRef) ? appSecretRef : botSecretRef(botId);
  if (store !== undefined && await storeBotSecret(store, ref, credentials.appSecret)) {
    const persisted = persistBotCredentialRefToProfile(botId, {
      appId: credentials.appId,
      appSecretRef: ref,
      ...(credentials.userOpenid === undefined ? {} : { userOpenid: credentials.userOpenid }),
    }, profileDir, logger);
    if (persisted) return { persisted: true };
    // patch 写失败：清掉刚写入的引用，回退明文路径（避免留下孤儿密钥）
    await removeBotSecret(store, ref);
  } else if (store !== undefined) {
    logger.warn(`im-qqbot: 机器人 ${botId} 凭据库写入失败，回退 profile patch 明文存储`);
  }
  const persisted = persistBotCredentialsToProfile(botId, credentials, profileDir, logger);
  return persisted
    ? { persisted: true }
    : { persisted: false, error: '无法定位 profile 目录或写入失败，请改用环境变量配置' };
}

/**
 * 机器人解绑：删除 patch 里的凭据并清理凭据库引用。
 * @param botId - 配置里的稳定 bot id
 * @param logger - 日志
 * @param store - 凭据库适配层（未安装时 undefined）
 * @param appSecretRef - 该 bot 已声明的引用（与默认引用一并清理）
 * @returns 可展示的解绑结果
 */
async function removeBotCredentials(
  botId: string,
  logger: Logger,
  store: CredentialsLike | undefined,
  appSecretRef: string | undefined,
): Promise<UnbindResult> {
  const persisted = removeBotCredentialsFromProfile(botId, getProfileDir() ?? undefined, logger);
  if (!persisted) {
    return { persisted: false, error: '无法定位 profile 目录或写入失败，请手动编辑 cordis.patch.yml' };
  }
  if (store !== undefined) {
    const refs = new Set<string>([botSecretRef(botId)]);
    if (appSecretRef !== undefined && isSecretRef(appSecretRef)) refs.add(appSecretRef);
    for (const ref of refs) {
      if (!(await removeBotSecret(store, ref))) {
        logger.warn(`im-qqbot: 解绑时未能清理凭据库引用 ${ref}（不影响机器人下线）`);
      }
    }
  }
  return { persisted: true };
}

/** 新增机器人条目（空凭据，等待手动绑定/配置） */
function persistNewBot(botId: string, label: string | undefined, logger: Logger): { ok: boolean; error?: string } {
  const added = addBotToProfile(botId, label, getProfileDir() ?? undefined, logger);
  return added ? { ok: true } : { ok: false, error: `机器人 ${botId} 已存在或写入失败` };
}

/** 删除机器人条目（运行时由 HMR 重载下线） */
function deleteBot(botId: string, logger: Logger): { ok: boolean; error?: string } {
  const removed = removeBotFromProfile(botId, getProfileDir() ?? undefined, logger);
  return removed ? { ok: true } : { ok: false, error: `机器人 ${botId} 不存在或写入失败` };
}

/** 手动凭据持久化（失败时返回可展示的错误） */
function persistCredentials(credentials: SetupCredentials, logger: Logger): { persisted: boolean; error?: string } {
  const persisted = persistCredentialsToProfile(credentials, getProfileDir() ?? undefined, logger);
  return persisted
    ? { persisted: true }
    : { persisted: false, error: '无法定位 profile 目录或写入失败，请改用环境变量配置' };
}
