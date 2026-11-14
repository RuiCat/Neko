/**
 * K25：agent 工具 `qqbot_bots` — 列出运行中 QQ 机器人及其主会话 / QQ 子会话 id。
 *
 * 数据源与 `GET /qqbot/bots` 同源（`BotCoordinator.list()` + `qqSessionsOf()`），
 * 但**只暴露非敏感目录字段**（botId/label/entity/mainSessionId/sessions），
 * 不含 appId/appSecret/appSecretRef。拿到会话 id 后可配合 `astrbox_relay`
 * 的 `target=session-…` 做跨会话投递。
 *
 * 注册模式（与 dsh-astrbox 桥接一致）：先立即尝试 `ctx.get('tools')`；
 * 冷启动时 tools 尚未就绪则 `ctx.inject(['tools'])` 延迟注册；只注册一次，
 * 失败/缺失只 warn，不影响插件其它功能。
 */
import type { Context } from '@deepseek-ai/cordis';
import type { ContentBlock } from '@deepseek-ai/dsh-llm';
import type { QqSessionWire } from '../shared/bind-protocol.ts';
import { normalizeEntityId } from '../configBots.ts';
import type { Logger } from '../types.ts';

/** 工具名（qqbot 前缀避免与其它通道冲突）。 */
export const BOTS_TOOL_NAME = 'qqbot_bots';

const DESCRIPTION =
  'List the running QQ bots and their sessions (the bot main session id plus private/group QQ '
  + 'session ids). Use this to discover a session id when you need cross-session delivery, for '
  + 'example calling astrbox_relay with target="session-…". Pass bot_id to filter one bot; omit it '
  + 'for all running bots. The result never contains credentials.';

/** tools 服务最小接口（与 send-file/vision 工具一致）。 */
interface ToolsRegistryLike {
  register(definition: unknown): unknown;
}

/** 目录快照所需的机器人形状（`BotCoordinator.list()` 的元素结构上满足）。 */
export interface BotsToolBot {
  readonly id: string;
  readonly label: string;
  /** 生效配置（只读其中非敏感的 entity） */
  readonly config: { readonly entity?: string };
  /** 会话管理器（主会话 id 可能尚未就绪） */
  readonly manager: { readonly mainSessionId?: string };
}

/** `qqbot_bots` 的数据源（与 `/qqbot/bots` 同源，避免两套事实）。 */
export interface BotsToolSource {
  /** 运行中的机器人（顺序 = 启动顺序） */
  list(): readonly BotsToolBot[];
  /** 该 bot 的 QQ 子会话（与 `/qqbot/bots` 的 sessions 字段同源） */
  qqSessionsOf(botId: string): ReadonlyArray<QqSessionWire> | undefined;
}

/** 工具返回的单个 bot 目录项（不含任何凭据/机密字段）。 */
export interface BotsToolSnapshot {
  /** 配置里的稳定 bot id */
  botId: string;
  /** 展示名 */
  label: string;
  /** 绑定的 NPC 实体 id（未绑定时省略） */
  entity?: string;
  /** 主会话 id（未创建/未就绪时省略） */
  mainSessionId?: string;
  /** QQ 子会话（私聊/群聊），无历史会话时为空数组 */
  sessions: QqSessionWire[];
}

/**
 * 生成工具快照（纯函数）：只挑非敏感字段，`bot_id` 过滤走精确匹配。
 * @param source - 机器人与会话数据源
 * @param botId - 可选过滤的 bot id
 * @returns 目录项数组（保持启动顺序）
 */
export function snapshotBots(source: BotsToolSource, botId?: string): BotsToolSnapshot[] {
  return source.list()
    .filter(bot => botId === undefined || bot.id === botId)
    .map((bot) => {
      const entity = normalizeEntityId(bot.config.entity);
      const mainSessionId = bot.manager.mainSessionId;
      const sessions = source.qqSessionsOf(bot.id) ?? [];
      return {
        botId: bot.id,
        label: bot.label,
        ...(entity === undefined ? {} : { entity }),
        ...(mainSessionId === undefined ? {} : { mainSessionId }),
        sessions: sessions.map(session => ({ id: session.id, scope: session.scope, peerId: session.peerId })),
      };
    });
}

/** 工具返回值的渲染文本（模型可读的一行摘要）。 */
function renderBots(value: unknown): ContentBlock[] {
  const bots = (value as { bots?: BotsToolSnapshot[] } | undefined)?.bots ?? [];
  if (bots.length === 0) return [{ type: 'text', text: '没有运行中的 QQ 机器人' }];
  const lines = bots.map((bot) => {
    const head = `${bot.botId}（${bot.label}${bot.entity === undefined ? '' : `，实体 ${bot.entity}`}）`;
    const main = bot.mainSessionId === undefined ? '主会话未就绪' : `主会话 ${bot.mainSessionId}`;
    const sessions = bot.sessions.length === 0
      ? '无 QQ 子会话'
      : bot.sessions.map(session => `${session.scope}:${session.peerId}=${session.id}`).join('，');
    return `${head}：${main}；${sessions}`;
  });
  return [{ type: 'text', text: lines.join('\n') }];
}

/** 构造工具定义（definition 形状与 send-file/vision 工具一致）。 */
function createDefinition(source: BotsToolSource): Record<string, unknown> {
  return {
    name: BOTS_TOOL_NAME,
    description: DESCRIPTION,
    parameters: {
      type: 'object',
      properties: {
        bot_id: {
          type: 'string',
          description: 'Optional bot id to filter the listing; omit to list all running bots.',
        },
      },
      additionalProperties: false,
    },
    output: {
      schema: {
        type: 'object',
        properties: {
          ok: { type: 'boolean' },
          bots: {
            type: 'array',
            items: {
              type: 'object',
              properties: {
                botId: { type: 'string' },
                label: { type: 'string' },
                entity: { type: 'string' },
                mainSessionId: { type: 'string' },
                sessions: {
                  type: 'array',
                  items: {
                    type: 'object',
                    properties: {
                      id: { type: 'string' },
                      scope: { type: 'string', enum: ['c2c', 'group'] },
                      peerId: { type: 'string' },
                    },
                    required: ['id', 'scope', 'peerId'],
                    additionalProperties: false,
                  },
                },
              },
              required: ['botId', 'label', 'sessions'],
              additionalProperties: false,
            },
          },
        },
        required: ['ok', 'bots'],
        additionalProperties: false,
      },
      render: (_args: unknown, value: unknown): ContentBlock[] => renderBots(value),
    },
    async execute(args: unknown): Promise<Record<string, unknown>> {
      const rawBotId = (args as { bot_id?: unknown } | null | undefined)?.bot_id;
      if (rawBotId !== undefined && typeof rawBotId !== 'string') {
        throw new Error('qqbot_bots: `bot_id` must be a string when provided');
      }
      const botId = rawBotId === undefined || rawBotId.trim() === '' ? undefined : rawBotId.trim();
      return { ok: true, bots: snapshotBots(source, botId) };
    },
  };
}

/** 读取 tools 服务（任何异常都视为不可用）。 */
function readTools(ctx: Context): ToolsRegistryLike | undefined {
  try {
    const getter = (ctx as unknown as { get?: (name: string) => unknown }).get;
    const tools = typeof getter === 'function' ? getter.call(ctx, 'tools') : undefined;
    const candidate = tools as ToolsRegistryLike | undefined;
    return candidate !== undefined && typeof candidate.register === 'function' ? candidate : undefined;
  } catch {
    return undefined;
  }
}

/** 注册 `qqbot_bots` 工具（立即尝试 + `ctx.inject(['tools'])` 延迟注册；缺失只 warn；返回可重复调用的释放函数）。 */
export function registerBotsTool(ctx: Context, source: BotsToolSource, logger: Logger): () => void {
  let registered = false;
  let disposed = false;
  let disposeTool: (() => void) | undefined;
  let toolScope: { dispose?: () => unknown } | undefined;

  const disposeAll = (): void => {
    if (disposed) return;
    disposed = true;
    try {
      disposeTool?.();
    } catch {
      // 工具可能已随 tools fiber 释放：忽略。
    }
    try {
      void toolScope?.dispose?.();
    } catch {
      // 注入作用域可能已随父 fiber 释放：忽略。
    }
  };

  const tryRegister = (tools: ToolsRegistryLike | undefined): boolean => {
    if (registered || disposed || tools === undefined) return registered;
    try {
      const disposer = tools.register(createDefinition(source));
      // 只有注册成功才置位：register 抛错时仍可走延迟注入路径重试一次。
      disposeTool = typeof disposer === 'function' ? disposer as () => void : undefined;
      registered = true;
      logger.info('im-qqbot: qqbot_bots 工具已注册');
      return true;
    } catch (err) {
      logger.warn(`im-qqbot: qqbot_bots 工具注册失败（插件继续工作）: ${err instanceof Error ? err.message : String(err)}`);
      return false;
    }
  };

  // 1) 立即尝试：热重载/常规启动时 tools 通常已就绪。
  tryRegister(readTools(ctx));

  // 2) 冷启动：tools 尚未就绪时等它出现再注册（只注册一次）。
  if (!registered && !disposed) {
    const inject = (ctx as unknown as {
      inject?: (deps: readonly string[], callback: (scoped: Context) => void) => unknown;
    }).inject;
    if (typeof inject === 'function') {
      try {
        const fiber = inject.call(ctx, ['tools'], (scoped: Context) => {
          const scopedTools = (scoped as unknown as { tools?: unknown }).tools;
          tryRegister((scopedTools as ToolsRegistryLike | undefined) ?? readTools(scoped));
        });
        toolScope = fiber as unknown as { dispose?: () => unknown };
      } catch (err) {
        logger.warn(`im-qqbot: tools 服务注入失败，qqbot_bots 工具未注册（插件继续工作）: ${err instanceof Error ? err.message : String(err)}`);
      }
    }
    if (!registered) {
      logger.warn('im-qqbot: tools 服务不可用，qqbot_bots 工具未注册（插件继续工作）');
    }
  }

  // 3) 随插件 fiber 卸载释放（自定义环境无 effect 时由调用方使用返回的 disposer）。
  const effect = (ctx as unknown as { effect?: (fn: () => () => void, name?: string) => void }).effect;
  if (typeof effect === 'function') {
    try {
      effect.call(ctx, () => disposeAll, 'im-qqbot.bots-tool');
    } catch {
      // effect 注册失败不阻断：注册已成功时工具仍可用，卸载回收由 tools 服务兜底。
    }
  }
  return disposeAll;
}
