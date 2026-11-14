/**
 * K25 `qqbot_bots` 工具单测：快照形状/过滤/无敏感字段、立即注册、延迟注入注册、无 tools 降级与释放。
 */
import { describe, expect, it, vi } from 'vitest';
import type { Context } from '@deepseek-ai/cordis';
import type { QqSessionWire } from '../shared/bind-protocol.ts';
import type { Logger } from '../types.ts';
import {
  BOTS_TOOL_NAME,
  registerBotsTool,
  snapshotBots,
  type BotsToolBot,
  type BotsToolSource,
} from './botsTool.ts';

/** 日志替身。 */
function createLogger(): Logger {
  return { debug: vi.fn(), info: vi.fn(), warn: vi.fn(), error: vi.fn() } as unknown as Logger;
}

/** 机器人目录项替身。 */
function bot(
  id: string,
  opts: { label?: string; entity?: string; mainSessionId?: string } = {},
): BotsToolBot {
  return {
    id,
    label: opts.label ?? id,
    config: opts.entity === undefined ? {} : { entity: opts.entity },
    manager: { mainSessionId: opts.mainSessionId },
  };
}

/** 数据源替身：会话表按 botId 给。 */
function sourceOf(
  bots: readonly BotsToolBot[],
  sessions: Record<string, QqSessionWire[] | undefined> = {},
): BotsToolSource {
  return {
    list: () => bots,
    qqSessionsOf: (botId: string) => sessions[botId],
  };
}

/** tools 服务替身：记录注册的定义与返回的 disposer。 */
function toolsService() {
  const registered: Array<Record<string, unknown>> = [];
  const disposers: Array<() => void> = [];
  return {
    registered,
    disposers,
    tools: {
      register: (definition: unknown) => {
        registered.push(definition as Record<string, unknown>);
        const dispose = vi.fn();
        disposers.push(dispose);
        return dispose;
      },
    },
  };
}

/** 立即路径的 ctx：get('tools') 命中 + effect 收集清理函数。 */
function immediateCtx(tools: unknown): { ctx: Context; cleanups: Array<() => void> } {
  const cleanups: Array<() => void> = [];
  const ctx = {
    get: (name: string) => (name === 'tools' ? tools : undefined),
    effect: (fn: () => () => void) => { cleanups.push(fn()); },
  } as unknown as Context;
  return { ctx, cleanups };
}

describe('snapshotBots（K25 返回形状）', () => {
  const sessions: Record<string, QqSessionWire[] | undefined> = {
    cat: [
      { id: 'session-c2c', scope: 'c2c', peerId: 'u1' },
      { id: 'session-group', scope: 'group', peerId: 'g1' },
    ],
    helper: undefined,
  };

  it('含主会话与 QQ 子会话；无会话的 bot sessions 为空数组', () => {
    const source = sourceOf([
      bot('cat', { label: '猫猫', entity: ' 梦梦 ', mainSessionId: 'session-main' }),
      bot('helper', { label: '助手' }),
    ], sessions);

    expect(snapshotBots(source)).toEqual([
      {
        botId: 'cat',
        label: '猫猫',
        entity: '梦梦',
        mainSessionId: 'session-main',
        sessions: [
          { id: 'session-c2c', scope: 'c2c', peerId: 'u1' },
          { id: 'session-group', scope: 'group', peerId: 'g1' },
        ],
      },
      { botId: 'helper', label: '助手', sessions: [] },
    ]);
  });

  it('bot_id 精确过滤；未知 bot → 空数组（仍 ok 语义由 execute 保证）', () => {
    const source = sourceOf([bot('cat'), bot('helper')], sessions);
    expect(snapshotBots(source, 'cat').map(item => item.botId)).toEqual(['cat']);
    expect(snapshotBots(source, 'nope')).toEqual([]);
  });

  it('不含 appId/appSecret/appSecretRef 等敏感字段', () => {
    const source = sourceOf([
      { ...bot('cat'), appId: 'app-secret-id', appSecret: 'SECRET-VALUE', appSecretRef: 'QQBOT_CAT_SECRET' } as unknown as BotsToolBot,
    ], sessions);
    const serialized = JSON.stringify(snapshotBots(source));
    expect(serialized).not.toContain('SECRET');
    expect(serialized).not.toContain('appId');
  });

  it('数据源抛错时由注册器兜住（工具不外泄异常）；快照本身不做隐藏捕获', () => {
    const source: BotsToolSource = { list: () => { throw new Error('gone'); }, qqSessionsOf: () => undefined };
    expect(() => snapshotBots(source)).toThrow('gone');
  });
});

describe('registerBotsTool：立即注册', () => {
  it('tools 已就绪 → 直接注册一次；可执行返回目录；effect 释放注销', async () => {
    const service = toolsService();
    const { ctx, cleanups } = immediateCtx(service.tools);
    const logger = createLogger();

    const dispose = registerBotsTool(ctx, sourceOf([
      bot('cat', { label: '猫猫', mainSessionId: 'session-main' }),
    ], { cat: [{ id: 'session-c2c', scope: 'c2c', peerId: 'u1' }] }), logger);

    expect(service.registered).toHaveLength(1);
    const definition = service.registered[0]!;
    expect(definition.name).toBe(BOTS_TOOL_NAME);
    expect(JSON.stringify(definition)).not.toContain('appSecret');

    const execute = definition.execute as (args: unknown) => Promise<Record<string, unknown>>;
    expect(await execute({})).toEqual({
      ok: true,
      bots: [{
        botId: 'cat',
        label: '猫猫',
        mainSessionId: 'session-main',
        sessions: [{ id: 'session-c2c', scope: 'c2c', peerId: 'u1' }],
      }],
    });
    expect(await execute({ bot_id: 'cat' })).toMatchObject({ ok: true, bots: [{ botId: 'cat' }] });
    expect(await execute({ bot_id: 'nope' })).toEqual({ ok: true, bots: [] });

    const render = (definition.output as { render: (a: unknown, v: unknown) => Array<{ text: string }> }).render;
    expect(render(undefined, { bots: [] })[0]?.text).toContain('没有运行中的');
    expect(render(undefined, {
      bots: [{ botId: 'cat', label: '猫猫', mainSessionId: 'session-main', sessions: [] }],
    })[0]?.text).toContain('session-main');

    expect(cleanups).toHaveLength(1);
    cleanups[0]?.();
    expect(service.disposers[0]).toHaveBeenCalledTimes(1);
    cleanups[0]?.(); // 幂等
    expect(service.disposers[0]).toHaveBeenCalledTimes(1);
    void dispose;
  });

  it('bot_id 非字符串 → execute 抛参数错误', async () => {
    const service = toolsService();
    const { ctx } = immediateCtx(service.tools);
    registerBotsTool(ctx, sourceOf([bot('cat')]), createLogger());
    const execute = service.registered[0]!.execute as (args: unknown) => Promise<unknown>;
    await expect(execute({ bot_id: 42 })).rejects.toThrow('bot_id');
  });
});

describe('registerBotsTool：延迟注册与降级', () => {
  it('tools 未就绪 → ctx.inject 等它出现后只注册一次；释放时注销工具与注入 scope', () => {
    const service = toolsService();
    let injectCallback: ((scoped: Context) => void) | undefined;
    const fiberDispose = vi.fn();
    const ctx = {
      get: () => undefined,
      inject: (_deps: readonly string[], callback: (scoped: Context) => void) => {
        injectCallback = callback;
        return { dispose: fiberDispose };
      },
    } as unknown as Context;

    const dispose = registerBotsTool(ctx, sourceOf([bot('cat')]), createLogger());
    expect(service.registered).toHaveLength(0);

    injectCallback?.({ tools: service.tools } as unknown as Context);
    expect(service.registered).toHaveLength(1);
    injectCallback?.({ tools: service.tools } as unknown as Context); // 服务替换重跑：不重复注册
    expect(service.registered).toHaveLength(1);

    dispose();
    expect(service.disposers[0]).toHaveBeenCalledTimes(1);
    expect(fiberDispose).toHaveBeenCalledTimes(1);
  });

  it('无 tools 服务且无 inject → 只 warn，不抛；返回的 disposer 可安全调用', () => {
    const logger = createLogger();
    const ctx = {} as Context;
    let dispose: (() => void) | undefined;
    expect(() => { dispose = registerBotsTool(ctx, sourceOf([bot('cat')]), logger); }).not.toThrow();
    expect(logger.warn).toHaveBeenCalled();
    expect(() => dispose?.()).not.toThrow();
  });

  it('inject 注册回调收到 scoped 的 tools 前抛错 → warn 且插件继续', () => {
    const logger = createLogger();
    const ctx = {
      get: () => undefined,
      inject: () => { throw new Error('registry unavailable'); },
    } as unknown as Context;
    expect(() => registerBotsTool(ctx, sourceOf([bot('cat')]), logger)).not.toThrow();
    expect(String(vi.mocked(logger.warn).mock.calls.flat())).toContain('tools');
  });
});
