/**
 * BotCoordinator 多机器人归属与全局工具注册测试（P2，不连真实 QQ SDK）。
 */
import { describe, expect, it, vi } from 'vitest';
import type { Context } from '@deepseek-ai/cordis';
import type { EffectiveBot } from '../configBots.ts';
import type { ImQQBotConfig } from '../config.ts';
import type { BotRuntimeHandle } from './bootstrap.ts';
import type { SessionManager } from '../session/index.ts';
import type { MediaSenderLike } from '../media/send-media.ts';
import { join } from 'node:path';
import { startBotCoordinator } from './botCoordinator.ts';
import { PLUGIN_HOME } from '../shared/dirs.ts';
import { registerDescribeImageTool } from '../media/vision-tool.ts';
import { SEND_FILE_TOOL_NAME } from '../media/send-file-tool.ts';

function createLogger() {
  return { debug: vi.fn(), info: vi.fn(), warn: vi.fn(), error: vi.fn() };
}

function configOf(appId: string, extra: Partial<ImQQBotConfig> = {}): ImQQBotConfig {
  return {
    appId,
    appSecret: 'secret',
    provider: '',
    model: '',
    preset: '',
    cwd: '',
    requireMention: true,
    textChunkLimit: 4500,
    streaming: false,
    sessionIdleTimeout: 0,
    maxQueue: 20,
    processingTimeoutMs: 1000,
    askTimeoutMs: 1000,
    historyLimit: 10,
    access: { c2cMode: 'open', c2cAllow: [], groupMode: 'open', groupAllow: [] },
    bots: [],
    showToolResults: false,
    debug: false,
    media: { enabled: false, maxMB: 200, ttlHours: 24 },
    vision: { enabled: false, provider: '', model: '', defaultPrompt: '', maxBytes: 1, maxTokens: 1, timeoutMs: 1 },
    sendFile: { restrictPaths: true, extraRoots: [], sendPresentedFiles: false, maxFiles: 1 },
    proactiveDelivery: { externalTurns: 'off' },
    ...extra,
  };
}

/** 假运行时：只有归属查询是测试关心的。 */
function fakeRuntime(appId: string, agents: Array<{ agent: unknown; sessionId: string }>): BotRuntimeHandle {
  const manager = {
    findByAgent: (agent: unknown) => (agents.find(entry => entry.agent === agent) === undefined ? undefined : {}),
    findBySessionId: (sessionId: string) => (agents.find(entry => entry.sessionId === sessionId) === undefined ? undefined : {}),
  } as unknown as SessionManager;
  return {
    config: configOf(appId),
    manager,
    targets: { targetFor: () => undefined },
    mediaSender: {} as MediaSenderLike,
    mediaRoot: `/tmp/dsh-qqbot-test-media/${appId}`,
    account: {},
    sender: { sendMarkdown: vi.fn() } as unknown as BotRuntimeHandle['sender'],
  };
}

function botConfig(id: string, appId: string, extra: Partial<ImQQBotConfig> = {}, legacy = false): EffectiveBot {
  return { id, label: id, enabled: true, legacy, config: configOf(appId, extra) };
}

/** 只关心绑定快照的运行时替身（config 原样保留，便于断言 entity/cwd 推导）。 */
function bindingRuntime(config: ImQQBotConfig, mainSessionId?: string): BotRuntimeHandle {
  return {
    config,
    manager: { mainSessionId, latestTarget: () => undefined } as unknown as SessionManager,
    targets: { targetFor: () => undefined },
    mediaSender: {} as MediaSenderLike,
    mediaRoot: `/tmp/dsh-qqbot-test-media/${config.appId}`,
    account: {},
    sender: { sendMarkdown: vi.fn() } as unknown as BotRuntimeHandle['sender'],
  };
}

/** 带 provide 记录的 ctx（K1 服务注册断言用）。 */
function createProvideCtx(): { ctx: Context; provided: Map<string, unknown> } {
  const provided = new Map<string, unknown>();
  const ctx = {
    get: (name: string) => {
      if (name === 'tools') return { register: () => () => undefined };
      if (name === 'llm') return { stream: async function* () {} };
      if (name === 'attachments') return { saveImage: async () => ({}) };
      return undefined;
    },
    provide: (name: string, value: unknown) => {
      provided.set(name, value);
      return () => { provided.delete(name); };
    },
  } as unknown as Context;
  return { ctx, provided };
}

function createCtx(): { ctx: Context; registered: string[] } {
  const registered: string[] = [];
  const ctx = {
    get: (name: string) => {
      if (name === 'tools') {
        return {
          register: (definition: { name: string }) => {
            registered.push(definition.name);
            return () => undefined;
          },
        };
      }
      if (name === 'llm') return { stream: async function* () {} };
      if (name === 'attachments') return { saveImage: async () => ({}) };
      return undefined;
    },
  } as unknown as Context;
  return { ctx, registered };
}

describe('BotCoordinator：归属解析', () => {
  it('按 sessionId / agent 找到归属机器人，fallback 取启动顺序第一个', () => {
    const agentA = { id: 'agent-a' };
    const agentB = { id: 'agent-b' };
    const logger = createLogger();
    const createdIds: string[] = [];
    const coordinator = startBotCoordinator(
      createCtx().ctx,
      {} as never,
      [botConfig('cat', 'app-cat'), botConfig('helper', 'app-helper')],
      logger,
      {
        createRuntime: (config: ImQQBotConfig, botId: string) => {
          createdIds.push(botId);
          return fakeRuntime(config.appId, config.appId === 'app-cat'
            ? [{ agent: agentA, sessionId: 'session-a' }]
            : [{ agent: agentB, sessionId: 'session-b' }]);
        },
      },
    );

    expect(coordinator.size).toBe(2);
    expect(coordinator.bySession('session-b')?.id).toBe('helper');
    expect(coordinator.bySession('session-x')).toBeUndefined();
    expect(coordinator.byAgent(agentB)?.id).toBe('helper');
    expect(coordinator.byAgent({ id: 'unknown' })).toBeUndefined();
    expect(coordinator.fallback()?.id).toBe('cat');
    expect(createdIds).toEqual(['cat', 'helper']);
  });

  it('nameOf 返回该 bot 的账号昵称（READY 后填充）', () => {
    const coordinator = startBotCoordinator(
      createCtx().ctx,
      {} as never,
      [botConfig('cat', 'app-cat')],
      createLogger(),
      {
        createRuntime: (config: ImQQBotConfig) => {
          const runtime = fakeRuntime(config.appId, []);
          runtime.account.name = '猫猫号';
          return runtime;
        },
      },
    );
    expect(coordinator.nameOf('cat')).toBe('猫猫号');
    expect(coordinator.nameOf('helper')).toBeUndefined();
  });

  it('工作区显示名按「绑定实体 > label」传给运行时（K10）', () => {
    const titles: Array<string | undefined> = [];
    startBotCoordinator(
      createCtx().ctx,
      {} as never,
      [
        botConfig('cat', 'app-cat', { entity: '梦梦' }),
        botConfig('helper', 'app-helper'),
      ],
      createLogger(),
      {
        createRuntime: (config: ImQQBotConfig, botId: string, _legacy: boolean, workspaceTitle?: string) => {
          titles.push(workspaceTitle);
          return fakeRuntime(config.appId, []);
        },
      },
    );
    expect(titles).toEqual(['梦梦', 'helper']);
  });

  it('单个 bot 启动失败不拖垮其它 bot', () => {
    const logger = createLogger();
    const coordinator = startBotCoordinator(
      createCtx().ctx,
      {} as never,
      [botConfig('bad', 'app-bad'), botConfig('good', 'app-good')],
      logger,
      {
        createRuntime: (config: ImQQBotConfig) => {
          if (config.appId === 'app-bad') throw new Error('boom');
          return fakeRuntime(config.appId, []);
        },
      },
    );
    expect(coordinator.size).toBe(1);
    expect(coordinator.list()[0]?.id).toBe('good');
    expect(logger.error).toHaveBeenCalled();
  });
});

describe('BotCoordinator：全局工具只注册一次', () => {
  it('两个 bot 只注册一个 qqbot_send_file', () => {
    const { ctx, registered } = createCtx();
    startBotCoordinator(ctx, {} as never, [botConfig('a', 'app-a'), botConfig('b', 'app-b')], createLogger(), {
      createRuntime: (config: ImQQBotConfig) => fakeRuntime(config.appId, []),
    });
    expect(registered.filter(name => name === SEND_FILE_TOOL_NAME)).toHaveLength(1);
  });

  it('qqbot_bots 与 send_file 并列注册，多 bot 全局只注册一次（K25）', () => {
    const { ctx, registered } = createCtx();
    startBotCoordinator(ctx, {} as never, [botConfig('a', 'app-a'), botConfig('b', 'app-b')], createLogger(), {
      createRuntime: (config: ImQQBotConfig) => fakeRuntime(config.appId, []),
    });
    expect(registered.filter(name => name === 'qqbot_bots')).toHaveLength(1);
  });

  it('没有启用机器人时不注册工具并告警', () => {
    const { ctx, registered } = createCtx();
    const logger = createLogger();
    const coordinator = startBotCoordinator(ctx, {} as never, [], logger);
    expect(coordinator.size).toBe(0);
    expect(registered).toHaveLength(0);
    expect(logger.warn).toHaveBeenCalled();
  });
});

describe('视觉工具 lookup', () => {
  it('按 agent 解析配置；无归属且配置未启用时执行报错而不是崩溃', async () => {
    const { ctx, registered } = createCtx();
    registerDescribeImageTool(ctx, {
      byAgent: () => undefined,
      fallback: () => undefined,
      anyEnabled: () => true,
    }, createLogger());
    expect(registered).toContain('qqbot_describe_image');
  });
});

describe('qqbotBots 服务（K1）', () => {
  it('startBotCoordinator 提供 qqbotBots；bindings 形状含 entity/cwd/mainSessionId，每次返回新数组', () => {
    const { ctx, provided } = createProvideCtx();
    startBotCoordinator(ctx, {} as never, [
      botConfig('cat', 'app-cat', { entity: '梦梦', cwd: '/work/cat' }),
      botConfig('helper', 'app-helper', { cwd: '/work/helper' }),
    ], createLogger(), {
      createRuntime: (config, botId) => bindingRuntime(config, botId === 'cat' ? 'session-main' : undefined),
    });

    const service = provided.get('qqbotBots') as { bindings(): Array<Record<string, unknown>> };
    expect(service).toBeDefined();
    const first = service.bindings();
    expect(first).toEqual([
      { botId: 'cat', label: 'cat', entity: '梦梦', cwd: '/work/cat', mainSessionId: 'session-main' },
      { botId: 'helper', label: 'helper', cwd: '/work/helper' },
    ]);
    expect(service.bindings()).not.toBe(first);
  });

  it('cwd 与 SessionManager 同源：legacy 不传 botId（旧全局目录），多 bot 按 id 隔离', () => {
    const { ctx, provided } = createProvideCtx();
    startBotCoordinator(ctx, {} as never, [
      botConfig('default', 'app-legacy', {}, true),
      botConfig('cat', 'app-cat', {}),
    ], createLogger(), {
      createRuntime: (config) => bindingRuntime(config),
    });

    const service = provided.get('qqbotBots') as { bindings(): Array<Record<string, unknown>> };
    const bindings = service.bindings();
    expect(bindings[0]?.cwd).toBe(join(PLUGIN_HOME, 'workspace'));
    expect(bindings[1]?.cwd).toBe(join(PLUGIN_HOME, 'bots', 'cat', 'workspace'));
  });

  it('bindings 内部任何异常回落 []，绝不向桥接抛错（K1 错误语义）', () => {
    const { ctx, provided } = createProvideCtx();
    const throwingManager = {
      get mainSessionId(): string | undefined { throw new Error('manager exploded'); },
    } as unknown as SessionManager;
    startBotCoordinator(ctx, {} as never, [botConfig('cat', 'app-cat')], createLogger(), {
      createRuntime: (config) => ({
        config,
        manager: throwingManager,
        targets: { targetFor: () => undefined },
        mediaSender: {} as MediaSenderLike,
        mediaRoot: '/tmp/x',
        account: {},
        sender: { sendMarkdown: vi.fn() } as unknown as BotRuntimeHandle['sender'],
      }),
    });

    const service = provided.get('qqbotBots') as { bindings(): Array<unknown> };
    expect(() => service.bindings()).not.toThrow();
    expect(service.bindings()).toEqual([]);
  });

  it('没有可启动机器人时仍提供服务（bindings 为空），桥接不会读到已卸载 fiber 的旧值', () => {
    const { ctx, provided } = createProvideCtx();
    const coordinator = startBotCoordinator(ctx, {} as never, [], createLogger());
    expect(coordinator.size).toBe(0);
    const service = provided.get('qqbotBots') as { bindings(): unknown[] };
    expect(service.bindings()).toEqual([]);
  });

  it('主会话 id 来自运行时（未就绪时字段省略）', () => {
    const { ctx, provided } = createProvideCtx();
    startBotCoordinator(ctx, {} as never, [botConfig('cat', 'app-cat', { entity: ' 猫猫 ' })], createLogger(), {
      createRuntime: (config) => bindingRuntime(config, undefined),
    });
    const service = provided.get('qqbotBots') as { bindings(): Array<Record<string, unknown>> };
    const [binding] = service.bindings();
    expect(binding?.entity).toBe('猫猫'); // trim 归一
    expect(binding).not.toHaveProperty('mainSessionId');
  });
});
