/**
 * dsh-im-qqbot — QQ Bot IM channel plugin for deepseek-harness
 *
 * Cordis 插件入口。将 QQ 消息平台作为 dsh 的前端协议驱动。
 * 网关组装（中间件编排 + 事件 + 出站 + 生命周期）见 src/gateway/；
 * 凭据绑定编排（扫码状态机 + 端内绑定页路由）见 src/setup/。
 */
import type { Context } from '@deepseek-ai/cordis';
import { ConfigSchema, type ImQQBotConfig } from './config.ts';
import { startBotCoordinator } from './gateway/index.ts';
import type { DshAgentRegistry } from './session/index.ts';
import { getProfileDir, resolveAgentCwd } from './shared/index.ts';
import { readBridgeEntities } from './shared/astrboxBridge.ts';
import { addLegacyBotToProfile } from './setup/persistBots.ts';
import { normalizeBots, type EffectiveBot } from './configBots.ts';
import {
  ensureCredentials,
  persistCredentialsToProfile,
  registerBotCredentialRoutes,
  registerBoundCredentialRoutes,
} from './setup/index.ts';
import { credentialsFrom, resolveConfigSecrets } from './setup/credentialStore.ts';
import type { Logger } from './types.ts';
import { loadFileConfig } from './fileConfig.ts';

// ── Cordis 插件元数据 ──
export const name = 'im-qqbot';
// credentials 是 dsh-base 的必备 seam：静态声明确保 apply 运行时凭据库已就绪（冷启动解析 appSecretRef）。
export const inject = ['agents', 'credentials'];
export const Config = ConfigSchema;

export type { ImQQBotConfig } from './config.ts';

// ── 插件主体 ──
export async function apply(ctx: Context, rawConfig: ImQQBotConfig): Promise<void> {
  // patch 只声明加载；运行配置缺省时从 ~/.dsh-neko/Neko/data/qqbot.config.json 读取。
  const config = loadFileConfig('qqbot', rawConfig);
  const agents = (ctx as unknown as Record<string, unknown>).agents as DshAgentRegistry;
  const logger: Logger = ((ctx as unknown as Record<string, unknown>).logger as Logger) ?? console;

  // 凭据库适配层（credentials 已在静态 inject 中；形状不符时兜底 undefined，读取回退进程环境变量）。
  const credentials = credentialsFrom(ctx);

  // 配置归一化：旧顶层单 bot（可回退环境变量）与 bots[] 统一成「生效机器人」列表。
  // bots[] 条目可以只写 appSecretRef：先从凭据库解析出密钥，再交给归一化层判断可用性。
  const secretResolution = await resolveConfigSecrets(
    credentials,
    Array.isArray(config.bots) ? config.bots : [],
  );
  for (const warning of secretResolution.warnings) {
    logger.warn(`im-qqbot: ${warning}`);
  }
  const normalized = normalizeBots({ ...config, bots: secretResolution.bots }, {
    envAppId: process.env.QQBOT_APPID,
    envAppSecret: process.env.QQBOT_SECRET,
  });
  for (const warning of normalized.warnings) {
    logger.warn(`im-qqbot: ${warning}`);
  }

  // 多机器人：未绑定/停用的 bot 也注册路由（设置页需要它们用于扫码/手动绑定），
  // 但只有 enabled 的 bot 启动网关运行时。
  if (!normalized.legacy) {
    const configured = normalized.bots;
    const runnable = configured.filter(bot => bot.enabled);
    if (runnable.length === 0) {
      logger.warn('im-qqbot: bots[] 中没有可启动的机器人（检查 enabled 与 appId）；仍注册设置页路由以便绑定凭据');
    }
    const coordinator = startBotCoordinator(ctx, agents, runnable, logger);
    // 每 bot 一组 /qqbot/bots/<id>/... 路由（扫码/手动凭据/解绑/配置编辑）。
    registerBotCredentialRoutes(ctx, logger, configured.map(bot => ({
      id: bot.id,
      label: bot.label,
      appId: bot.config.appId,
      enabled: bot.enabled,
      appSecretRef: bot.appSecretRef,
      config: { ...bot.config } as unknown as Record<string, unknown>,
      accountName: () => coordinator.nameOf(bot.id),
      // K4：配置层推导的实体绑定与工作目录（停用/未运行的 bot 也会出现）。
      entity: bot.config.entity,
      cwd: resolveAgentCwd(bot.config.cwd, bot.legacy ? undefined : bot.id, bot.config.entity),
      // K4：运行态字段（未启动的 bot 回调返回 undefined → wire 省略字段）。
      mainSessionId: () => coordinator.mainSessionIdOf(bot.id),
      qqSessions: () => coordinator.qqSessionsOf(bot.id),
    })), {
      ...(credentials === undefined ? {} : { credentials }),
      // K8：NPC 实体目录只在桥接服务（astrboxBridge）可用时出现；否则省略字段。
      entities: () => readBridgeEntities(ctx),
    });
    return;
  }

  const legacyBot = normalized.bots[0];
  if (legacyBot === undefined) {
    logger.error('im-qqbot: 配置归一化没有产出可用机器人');
    return;
  }
  const appId = legacyBot.config.appId;
  const appSecret = legacyBot.config.appSecret;

  // ── 凭据缺失时唤起扫码绑定 ──
  if (!appId || !appSecret) {
    logger.info('凭据未配置，启动扫码绑定...');
    // K34：Web 设置页统一走多机器人视图。还没有任何机器人时返回空列表：
    // 客户端显示「+ 新增机器人」，不预置一个"默认机器人"，也不回退成单 bot 扫码页。
    // 终端扫码流程仍然保留（无 Web 时使用）。
    registerBotCredentialRoutes(ctx, logger, [], {
      entities: () => readBridgeEntities(ctx),
    });

    // 关键：不 await 扫码结果。boot 会等 Loader 树稳定（loader.await()），
    // 无限期等待用户扫码会阻塞 Host ready——Desktop 界面无法进入、
    // Web 的 URL 延迟输出。绑定编排（路由/呈现/生命周期）在
    // ensureCredentials 的同步阶段即完成，凭据就绪后由异步链接管。
    void ensureCredentials(ctx, { logger, config: { ...config } })
      .then((credentials): void => {
        if (!credentials) {
          // null 包括：扫码取消、手动提交（已另行持久化）、流程失败
          logger.info('扫码绑定流程结束（未获取凭据或已手动配置）');
          return;
        }

        // 持久化到 profile：成功则等待热更新重载，失败则用凭据直接启动。
        // 注意：不写入 process.env——resolveEnv 的 env 回退属于用户显式
        // export 的配置语义，运行时写入会与解绑清理不对齐（解绑后 HMR
        // 重载的新 fiber 仍从 env 读到旧凭据，状态停留 bound）
        const persisted = persistCredentialsToProfile(credentials, getProfileDir() ?? undefined, logger);
        if (persisted) {
          // 写入 cordis.patch.yml 会触发 dsh 热更新，自动重新加载本插件。
          logger.info('配置已保存，等待热更新重新加载...');
          return;
        }
        logger.warn('凭据未能持久化，本次进程将使用环境变量凭据启动（重启后需重新绑定）');
        // 本 fiber 已激活（ctx 有效），直接以凭据启动单 bot 运行时
        try {
          const bot: EffectiveBot = {
            id: legacyBot.id,
            label: legacyBot.label,
            enabled: true,
            legacy: true,
            config: { ...legacyBot.config, appId: credentials.appId, appSecret: credentials.appSecret },
          };
          startBotCoordinator(ctx, agents, [bot], logger);
        } catch (err) {
          logger.error(`网关启动失败: ${err instanceof Error ? err.message : String(err)}`);
        }
      })
      .catch((err: unknown) => {
        // 全兜：unhandled rejection 会被宿主 fail-loud 判致命
        logger.error(`扫码绑定流程异常: ${err instanceof Error ? err.message : String(err)}`);
      });

    // apply 立即完成，插件 fiber 进入 ACTIVE，Host ready 不再被阻塞
    return;
  }

  // 已绑定：注册常驻路由（状态回显 + 手动编辑/更换凭据 + 配置编辑）
  registerBoundCredentialRoutes(ctx, logger, appId, { ...config });

  // K34：把旧单 bot 幂等迁移进 bots[]（只写一次）。多机器人 UI 的配置/绑定写入
  // 都以 bots[] 条目为落点；先迁移可避免首次保存/绑定时找不到条目而失败。
  addLegacyBotToProfile(appId, appSecret, getProfileDir() ?? undefined, logger);

  const coordinator = startBotCoordinator(ctx, agents, [{ ...legacyBot, enabled: true }], logger);
  // K34：legacy 也暴露多机器人路由，否则设置页探测不到 /qqbot/bots 会回退成
  // 「手动填写凭据」单 bot 界面，永远没有新增入口。首次新增时把当前单 bot
  // 迁移进 bots[]（legacyBot 选项），旧机器人不会因新增而消失。
  registerBotCredentialRoutes(ctx, logger, [{
    id: legacyBot.id,
    label: legacyBot.label,
    appId,
    enabled: true,
    config: { ...legacyBot.config } as unknown as Record<string, unknown>,
    accountName: () => coordinator.nameOf(legacyBot.id),
    entity: legacyBot.config.entity,
    cwd: resolveAgentCwd(legacyBot.config.cwd, undefined, legacyBot.config.entity),
    mainSessionId: () => coordinator.mainSessionIdOf(legacyBot.id),
    qqSessions: () => coordinator.qqSessionsOf(legacyBot.id),
  }], {
    ...(credentials === undefined ? {} : { credentials }),
    entities: () => readBridgeEntities(ctx),
    legacyBot: { appId, appSecret },
  });
}
