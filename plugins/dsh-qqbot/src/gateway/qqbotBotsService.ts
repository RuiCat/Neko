/**
 * cordis 服务 `qqbotBots`（契约 K1）— 向 dsh-astrbox 桥接暴露机器人→实体绑定。
 *
 * 桥接侧用 `ctx.get('qqbotBots')` 惰性读取（不进静态 `inject`）：
 * - `bindings()` 每次返回新数组，元素 `{botId,label,entity?,cwd,mainSessionId?}`；
 * - `cwd` 与 SessionManager 的 workspace 解析同源（`resolveAgentCwd`，legacy 不传 botId）；
 * - 任何内部异常都回落为 `[]`，绝不向桥接抛错；
 * - 服务由提供方的 fiber 持有，插件卸载/HMR 时自动失效。
 */
import type { Context } from '@deepseek-ai/cordis';
import type { ImQQBotConfig } from '../config.ts';
import { normalizeEntityId } from '../configBots.ts';
import { resolveAgentCwd } from '../shared/dirs.ts';
import type { Logger } from '../types.ts';
import { sayToBot, type QQBotSayRequest, type QQBotSayResult, type SayableBot } from './botSay.ts';

/** 生成绑定快照所需的最小机器人形状（`BotCoordinator.list()` 的元素）。 */
export interface BindableBot {
  /** 配置里的稳定 bot id */
  readonly id: string;
  /** 展示名 */
  readonly label: string;
  /** true = 旧版顶层单 bot 配置（cwd 缺省保持旧全局目录） */
  readonly legacy: boolean;
  /** 该 bot 的生效配置（含归一化后的 entity） */
  readonly config: ImQQBotConfig;
  /** 会话管理器（主会话 id 可能尚未就绪） */
  readonly manager: { readonly mainSessionId?: string };
}

/** K1 v1 绑定项。 */
export interface QQBotBotsBinding {
  /** 配置里的稳定 bot id */
  botId: string;
  /** 展示名 */
  label: string;
  /** 绑定的 NPC 实体 id（未绑定/空串时省略） */
  entity?: string;
  /** agent 工作目录（与 SessionManager 同源） */
  cwd: string;
  /** 主会话 id（未创建/创建失败时省略） */
  mainSessionId?: string;
}

/** K1 服务运行时元素：绑定快照所需（BindableBot）+ K15 直发所需（SayableBot）。 */
export type QQBotBotsRuntime = BindableBot & SayableBot;

/** K1 v1 服务投影（K15 追加只读 `say`，不改变 `bindings` 形状）。 */
export interface QQBotBotsService {
  /** 当前运行中机器人的绑定快照（新数组） */
  bindings(): ReadonlyArray<QQBotBotsBinding>;
  /** K15：主动直发一条 QQ 消息（不触发模型回合；失败返回 reason，不抛错） */
  say(request: QQBotSayRequest): Promise<QQBotSayResult>;
}

/**
 * 生成绑定快照（纯函数）：实体归一化、cwd 与主会话 id 按 K1 语义填充。
 * @param bots - 运行中的机器人（顺序 = 启动顺序）
 * @returns 绑定项数组（每次调用新数组）
 */
export function snapshotBindings(bots: readonly BindableBot[]): ReadonlyArray<QQBotBotsBinding> {
  return bots.map((bot) => {
    const entity = normalizeEntityId(bot.config.entity);
    const mainSessionId = bot.manager.mainSessionId;
    return {
      botId: bot.id,
      label: bot.label,
      ...(entity === undefined ? {} : { entity }),
      cwd: resolveAgentCwd(bot.config.cwd, bot.legacy ? undefined : bot.id, entity),
      ...(mainSessionId === undefined ? {} : { mainSessionId }),
    };
  });
}

/** 服务名（K1）。 */
export const QQBOT_BOTS_SERVICE = 'qqbotBots';

/** 在当前 fiber 上注册 `qqbotBots` 服务（fail-soft；同 scope 冲突时覆盖服务值）。 */
export function provideQQBotBots(
  ctx: Context,
  listBots: () => readonly QQBotBotsRuntime[],
  logger?: Logger,
): () => void {
  // 细节：ctx 缺少 provide（单测替身/非 cordis 上下文）时静默跳过；同 scope 已提供
  //（重复启动）时改用 set 覆盖服务值，避免桥接读到旧快照；返回的 disposer 便于单测清理。
  const service: QQBotBotsService = {
    bindings: () => {
      try {
        return snapshotBindings(listBots());
      } catch {
        return [];
      }
    },
    // K15：直发失败一律转成 reason（listBots 同步抛错也在这里兜住）。
    say: (request) => {
      try {
        return sayToBot(listBots(), request);
      } catch {
        return Promise.resolve({ ok: false, reason: 'send_failed' });
      }
    },
  };
  const reflect = ctx as unknown as {
    provide?: (name: string, value: unknown) => unknown;
    set?: (name: string, value: unknown) => void;
  };
  if (typeof reflect.provide !== 'function') return () => {};
  try {
    const disposer = reflect.provide(QQBOT_BOTS_SERVICE, service);
    return typeof disposer === 'function' ? disposer as () => void : () => {};
  } catch (err) {
    // HMR 同 fiber 重复启动 / 其它插件已占用该名字：尽力覆盖，失败不阻塞启动。
    try {
      reflect.set?.(QQBOT_BOTS_SERVICE, service);
    } catch {
      logger?.warn(`im-qqbot: ${QQBOT_BOTS_SERVICE} 服务注册冲突: ${err instanceof Error ? err.message : String(err)}`);
    }
    return () => {};
  }
}
