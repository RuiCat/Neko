/**
 * SessionManager.getOrCreate 会话 id 迁移/铸造路径的回归用例
 *
 * - E1：resume 失败只在确认「会话不存在」时才首次创建，其余故障 warn 后向上抛
 * - E2：0.5.x 确定性 id 不进入新建路径；同 key 并发 getOrCreate 合并为一次
 * - E3 连带：prefs 损坏导致映射丢失时必须留下可见日志
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import type { Context } from '@deepseek-ai/cordis';
import { ConfigSchema, type ImQQBotConfig } from '../config.ts';
import type { Logger, ReplyTarget } from '../types.ts';
import type { DshAgent, DshAgentHandle, DshAgentRegistry } from './types.ts';

const env = vi.hoisted(() => ({ home: '' }));

vi.mock('../shared/dirs.ts', () => ({
  get PLUGIN_HOME() { return env.home; },
  resolveAgentCwd: (cwd: string | undefined) => cwd || `${env.home}/workspace`,
}));

const { SessionManager } = await import('./session-manager.ts');
const { PrefsStore } = await import('../model/prefs-store.ts');
const { legacyDerivedSessionId } = await import('./session-id.ts');

const APP_ID = 'app1';
const PEER = 'user1';
const KEY = `qqbot:${APP_ID}:c2c:${PEER}`;
const MAIN_KEY = `qqbot:${APP_ID}:main`;
const LEGACY_ID = legacyDerivedSessionId(KEY);
const MINTED = /^session-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const REPLY: ReplyTarget = { scope: 'c2c', targetId: PEER, msgId: 'm1' };

/** agents.resume 对不存在会话的报错 */
const notFound = (id: string): Error =>
  Object.assign(new Error(`session "${id}" not found`), { name: 'SessionPersistenceNotFoundError' });
/** 非「不存在」类失败：日志损坏 / 后端 IO / 同 id 未释放等 */
const corrupt = (id: string): Error => new Error(`EIO: corrupt session log for "${id}"`);

/**
 * 宿主 persistence 替身。
 *
 * 签名对齐宿主 `SessionPersistence.stat`：会话不存在返回 `undefined`（不抛），
 * 存在则返回快照。替身必须跟真实契约一致——此前替身带了宿主根本没有的
 * `load()`，用例全绿但线上直接 TypeError。
 */
function persistenceCtx(stat: (id: string) => Promise<{ header?: { agentPreset?: string } } | undefined>): Context {
  return {
    get: (name: string) => (name === 'sessionPersistence' ? { stat } : undefined),
    on: vi.fn(),
  } as unknown as Context;
}

interface Deferred<T> {
  promise: Promise<T>;
  resolve(value: T): void;
  reject(err: unknown): void;
}

function deferred<T>(): Deferred<T> {
  let resolve!: (value: T) => void;
  let reject!: (err: unknown) => void;
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej; });
  return { promise, resolve, reject };
}

function fakeAgent(id: string): DshAgent {
  return {
    id,
    ctx: {} as Context,
    status: 'idle',
    session: { id, snapshotEvents: () => [] },
    cancel: vi.fn(),
    followup: vi.fn(),
    whenIdle: vi.fn(async () => {}),
    runMaintenance: vi.fn(async (task: (signal: AbortSignal) => Promise<unknown>) => task(new AbortController().signal)),
  } as unknown as DshAgent;
}

function fakeHandle(id: string): DshAgentHandle {
  return { agent: fakeAgent(id), dispose: vi.fn(async () => {}) };
}

type ResumeImpl = (options: { resumeSessionId: string }) => Promise<DshAgentHandle>;

function createRegistry(resumeImpl: ResumeImpl) {
  const handles: DshAgentHandle[] = [];
  const registry = {
    get: vi.fn(() => undefined),
    resume: vi.fn(resumeImpl),
    create: vi.fn(async (options: { sessionId: string }) => {
      const handle = fakeHandle(options.sessionId);
      handles.push(handle);
      return handle;
    }),
  };
  return { registry, handles };
}

function createLogger(): Logger {
  return { info: vi.fn(), warn: vi.fn(), error: vi.fn(), debug: vi.fn() } as unknown as Logger;
}

/** 所有日志调用参数拼接（断言某错误是否被记录） */
function allLogText(logger: Logger): string {
  return (['info', 'warn', 'error', 'debug'] as const)
    .flatMap((level) => vi.mocked(logger[level]).mock.calls.flat())
    .map(String)
    .join('\n');
}

const managers: Array<InstanceType<typeof SessionManager>> = [];

/** 无任何宿主服务的 ctx（无 sessionPersistence，存在性无法求证） */
function emptyCtx(): Context {
  return { get: () => undefined, on: vi.fn() } as unknown as Context;
}

/** 默认替身：磁盘上没有任何会话（stat 一律 undefined） */
function absentCtx(): Context {
  return persistenceCtx(async () => undefined);
}

/** 替身：指定 id 的会话在盘上 */
function onDiskCtx(...ids: string[]): Context {
  return persistenceCtx(async (id) => (ids.includes(id) ? { header: {} } : undefined));
}

/**
 * 替身：磁盘上没有会话 + workspaceRegistry 的归档集合可写。
 * `archived` 数组按引用共享，用例可在两次调用之间把会话「归档」。
 */
function archivedCtx(archived: string[]): Context {
  return {
    get: (name: string) => {
      if (name === 'sessionPersistence') return { stat: async () => undefined };
      if (name === 'workspaceRegistry') {
        return {
          create: vi.fn(async (path: string) => ({ path, attachSession: vi.fn(async () => {}) })),
          archivedSessionIds: archived,
        };
      }
      return undefined;
    },
    on: vi.fn(),
  } as unknown as Context;
}

function createManagerWithCtx(
  registry: Pick<DshAgentRegistry, 'get' | 'resume' | 'create'>,
  ctx: Context,
  logger: Logger = createLogger(),
) {
  const config = ConfigSchema({
    appId: APP_ID,
    appSecret: 'secret',
    provider: 'p',
    model: 'm',
    cwd: join(env.home, 'workspace'),
    sessionIdleTimeout: 0,
  } as ImQQBotConfig);
  const manager = new SessionManager(ctx, registry as DshAgentRegistry, config, logger);
  managers.push(manager);
  return manager;
}

function createManager(registry: Pick<DshAgentRegistry, 'get' | 'resume' | 'create'>, logger: Logger = createLogger()) {
  return createManagerWithCtx(registry, absentCtx(), logger);
}

/** 模拟重启后读盘：prefs 中记录的 sessionId */
function persistedId(): string | undefined {
  return new PrefsStore().getSessionId(KEY);
}

/**
 * QQ 会话的 create 调用 id 列表。
 *
 * K9：getOrCreate 会按需补建主会话，主会话的 create 调用也落在这个 mock 上；
 * 主会话 id 由 adopt() 同步写入 prefs 的 MAIN_KEY，这里按它过滤。
 */
function createdIds(registry: { create: { mock: { calls: Array<[{ sessionId: string }]> } } }): string[] {
  const mainId = new PrefsStore().getSessionId(MAIN_KEY);
  return registry.create.mock.calls
    .map(([options]) => String(options.sessionId))
    .filter((sessionId) => sessionId !== mainId);
}

beforeEach(() => {
  env.home = mkdtempSync(join(tmpdir(), 'im-qqbot-session-'));
  mkdirSync(join(env.home, 'workspace'), { recursive: true });
});

afterEach(async () => {
  await Promise.all(managers.splice(0).map((m) => m.disposeAll()));
  rmSync(env.home, { recursive: true, force: true });
});

describe('deliveryAddress', () => {
  it('内存里没有会话时，只按 prefs 返回 QQ 地址，不唤醒 agent', () => {
    const sessionId = 'session-live';
    new PrefsStore().setSessionId(KEY, sessionId);
    const { registry } = createRegistry(async () => { throw new Error('should not resume'); });

    const address = createManager(registry).deliveryAddress(sessionId);

    expect(address).toEqual({ scope: 'c2c', targetId: PEER });
    expect(registry.resume).not.toHaveBeenCalled();
    expect(registry.create).not.toHaveBeenCalled();
    expect(registry.get).not.toHaveBeenCalled();
  });

  it('未知 session 返回 undefined', () => {
    const { registry } = createRegistry(async () => { throw new Error('should not resume'); });

    expect(createManager(registry).deliveryAddress('session-dead')).toBeUndefined();
    expect(registry.resume).not.toHaveBeenCalled();
    expect(registry.create).not.toHaveBeenCalled();
  });

  it('对端已经换成另一条活会话时，不为旧 id 返回地址', async () => {
    const { registry } = createRegistry(async ({ resumeSessionId }) => { throw notFound(resumeSessionId); });
    const manager = createManager(registry);
    const live = await manager.getOrCreate('c2c', PEER, PEER, REPLY);
    new PrefsStore().setSessionId(KEY, 'session-old');

    expect(manager.deliveryAddress('session-old')).toBeUndefined();
    expect(manager.deliveryAddress(live.sessionId)).toEqual(REPLY);
  });
});

describe('getOrCreate — 对照组（迁移主路径行为正确）', () => {
  it('全新用户：legacy 不存在 → 铸造 session- 前缀 id 创建并落盘', async () => {
    const { registry } = createRegistry(async ({ resumeSessionId }) => { throw notFound(resumeSessionId); });

    const record = await createManager(registry).getOrCreate('c2c', PEER, PEER, REPLY);

    expect(registry.resume).toHaveBeenCalledWith(expect.objectContaining({ resumeSessionId: LEGACY_ID }));
    expect(record.sessionId).toMatch(MINTED);
    expect(createdIds(registry)).toEqual([record.sessionId]);
    expect(persistedId()).toBe(record.sessionId);
  });

  it('0.5.x 老用户：legacy 会话 resume 成功 → 沿用并落盘，不创建', async () => {
    const { registry } = createRegistry(async ({ resumeSessionId }) => fakeHandle(resumeSessionId));

    const record = await createManager(registry).getOrCreate('c2c', PEER, PEER, REPLY);

    expect(record.sessionId).toBe(LEGACY_ID);
    // K9：主会话可能按需创建，但 QQ 会话本身不得新建
    expect(createdIds(registry)).not.toContain(LEGACY_ID);
    expect(persistedId()).toBe(LEGACY_ID);
  });

  it('prefs 已记录：resume 该 id 成功 → 不再探测 legacy', async () => {
    new PrefsStore().setSessionId(KEY, 'session-current');
    const { registry } = createRegistry(async ({ resumeSessionId }) => fakeHandle(resumeSessionId));

    const record = await createManager(registry).getOrCreate('c2c', PEER, PEER, REPLY);

    expect(record.sessionId).toBe('session-current');
    expect(registry.resume).toHaveBeenCalledTimes(1);
    expect(createdIds(registry)).not.toContain('session-current');
  });

  it('session 当初的 preset 锁定：优先 header.agentPreset，而非当前 effective preset', async () => {
    const prefs = new PrefsStore();
    prefs.setSessionId(KEY, 'session-current');
    prefs.setPreset(KEY, 'current-preset'); // 当前 effective 已被 /preset 换过
    const { registry } = createRegistry(async ({ resumeSessionId }) => fakeHandle(resumeSessionId));
    const presets = {
      resolve: vi.fn(async (id?: string) => ({ id: id ?? 'default' })),
      mount: vi.fn(async () => {}),
      list: vi.fn(async () => []),
    };
    const ctx = {
      get: (name: string) => {
        if (name === 'sessionPersistence') {
          return { stat: async (id: string) => (id === 'session-current' ? { header: { agentPreset: 'locked-preset' } } : undefined) };
        }
        return name === 'agentPresets' ? presets : undefined;
      },
      on: vi.fn(),
    } as unknown as Context;

    const record = await createManagerWithCtx(registry, ctx).getOrCreate('c2c', PEER, PEER, REPLY);

    // 此前 header 读取一直失败（调用了宿主没有的 load），preset 锁定语义实际失效
    expect(presets.resolve).toHaveBeenCalledWith('locked-preset');
    expect(record.agentPreset).toBe('locked-preset');
  });
});

describe('getOrCreate — E1 resume 失败按会话是否存在分流', () => {
  it('E1-1 会话在盘上但 resume 失败 → 不创建，错误记录后向上抛', async () => {
    new PrefsStore().setSessionId(KEY, 'session-current');
    const { registry } = createRegistry(async ({ resumeSessionId }) => { throw corrupt(resumeSessionId); });
    const logger = createLogger();
    // 宿主自身（agent-loop restoreOrCreateConfigured）只在确认磁盘不存在时才 create
    const manager = createManagerWithCtx(registry, onDiskCtx('session-current'), logger);

    await expect(manager.getOrCreate('c2c', PEER, PEER, REPLY)).rejects.toThrow(/EIO/);

    // K9：主会话可能按需创建，但 QQ 会话不得重建
    expect(createdIds(registry)).not.toContain('session-current');
    expect(persistedId()).toBe('session-current'); // 映射不被改写，故障恢复后仍指向原会话
    expect(allLogText(logger)).toContain('EIO');
  });

  it('E1-2 0.5.x 老会话在盘上但 resume 遇瞬时错误 → 不弃用，故障恢复后仍能恢复', async () => {
    const first = createRegistry(async ({ resumeSessionId }) => { throw corrupt(resumeSessionId); });

    await expect(
      createManagerWithCtx(first.registry, onDiskCtx(LEGACY_ID)).getOrCreate('c2c', PEER, PEER, REPLY),
    ).rejects.toThrow(/EIO/);
    expect(createdIds(first.registry)).not.toContain(LEGACY_ID);
    expect(persistedId()).toBeUndefined();

    // 重启：瞬时故障已恢复，legacy 会话正常 resume
    const second = createRegistry(async ({ resumeSessionId }) => {
      if (resumeSessionId === LEGACY_ID) return fakeHandle(resumeSessionId);
      throw notFound(resumeSessionId);
    });
    const record = await createManager(second.registry).getOrCreate('c2c', PEER, PEER, REPLY);

    expect(record.sessionId).toBe(LEGACY_ID); // 老会话历史仍可恢复
    expect(persistedId()).toBe(LEGACY_ID);
  });

  it('E1-3 存在性求证本身失败（持久化后端故障）→ 按存在处理，不重建', async () => {
    new PrefsStore().setSessionId(KEY, 'session-current');
    const { registry } = createRegistry(async ({ resumeSessionId }) => { throw notFound(resumeSessionId); });
    const logger = createLogger();
    const flaky = persistenceCtx(async () => { throw new Error('EIO: persistence backend unavailable'); });

    await expect(
      createManagerWithCtx(registry, flaky, logger).getOrCreate('c2c', PEER, PEER, REPLY),
    ).rejects.toThrow(/not found/); // 向上抛原 resume 错误，而非求证错误
    expect(createdIds(registry)).not.toContain('session-current');
    expect(allLogText(logger)).toContain('existence unknown');
  });

  it('E1-4 确认磁盘无此会话 → 即使 resume 报的是 IO 错误也允许首次创建', async () => {
    const { registry } = createRegistry(async ({ resumeSessionId }) => { throw corrupt(resumeSessionId); });

    const record = await createManager(registry).getOrCreate('c2c', PEER, PEER, REPLY);

    expect(record.sessionId).toMatch(MINTED);
    expect(createdIds(registry)).toEqual([record.sessionId]);
  });

  it('E1-5 宿主无 sessionPersistence.stat → 无求证手段，按 resume 结果创建并留痕', async () => {
    const { registry } = createRegistry(async ({ resumeSessionId }) => { throw notFound(resumeSessionId); });
    const logger = createLogger();

    const record = await createManagerWithCtx(registry, emptyCtx(), logger).getOrCreate('c2c', PEER, PEER, REPLY);

    expect(record.sessionId).toMatch(MINTED);
    expect(createdIds(registry)).toEqual([record.sessionId]);
    expect(allLogText(logger)).toContain('host lacks sessionPersistence.stat');
  });

  it('E1-6 宿主 persistence 形态不含 stat → 不卡住全新会话的创建', async () => {
    // 回归：此前按不存在的 load() 求证，TypeError 被当成「后端故障」→ 拒绝创建，
    // 全新用户的第一条消息直接收到「处理消息时出现异常」
    const { registry } = createRegistry(async ({ resumeSessionId }) => { throw notFound(resumeSessionId); });
    const legacyShape = {
      get: (name: string) => (name === 'sessionPersistence' ? { open: async () => ({}) } : undefined),
      on: vi.fn(),
    } as unknown as Context;

    const record = await createManagerWithCtx(registry, legacyShape).getOrCreate('c2c', PEER, PEER, REPLY);

    expect(record.sessionId).toMatch(MINTED);
    expect(createdIds(registry)).toEqual([record.sessionId]);
  });
});

describe('getOrCreate — E2 确定性 id 不进入新建路径', () => {
  it('E2-1 resume(legacy) 期间 /new 写入新 id → 采用 prefs 最新 id 新建，内存与 prefs 一致', async () => {
    const pending = deferred<DshAgentHandle>();
    const { registry } = createRegistry(() => pending.promise);
    const manager = createManager(registry);

    const creating = manager.getOrCreate('c2c', PEER, PEER, REPLY);
    await vi.waitFor(() => expect(registry.resume).toHaveBeenCalled());
    await manager.remove('c2c', PEER); // /new：同步把 prefs 写为新铸 id
    pending.reject(notFound(LEGACY_ID));
    const record = await creating;

    expect(record.sessionId).toMatch(MINTED);
    expect(record.sessionId).not.toBe(LEGACY_ID);
    expect(createdIds(registry)).toEqual([record.sessionId]);
    expect(persistedId()).toBe(record.sessionId); // 重启后仍是同一会话
  });

  it('E2-2 /compact 拉起与普通消息并发 getOrCreate → singleflight 合并为一次 resume/create，无 agent 泄漏', async () => {
    const resumes: Array<Deferred<DshAgentHandle>> = [];
    const { registry, handles } = createRegistry(() => {
      const d = deferred<DshAgentHandle>();
      resumes.push(d);
      return d.promise;
    });
    const manager = createManager(registry);

    // /compact 走 slashCommand（在 concurrencyGuard 之前），与普通消息不串行
    const compacting = manager.compact('c2c', PEER, { senderId: PEER, replyTarget: REPLY });
    const messaging = manager.getOrCreate('c2c', PEER, PEER, REPLY);
    await vi.waitFor(() => expect(resumes).toHaveLength(1));
    for (const d of resumes) d.reject(notFound(LEGACY_ID));
    const [, record] = await Promise.all([compacting, messaging]);

    expect(resumes).toHaveLength(1);
    expect(createdIds(registry)).toEqual([record.sessionId]);
    expect(record.sessionId).toMatch(MINTED);
    // 主会话（K9 按需创建）带自己的句柄；QQ 会话不得产生多余 agent
    const mainId = new PrefsStore().getSessionId(MAIN_KEY);
    const qqHandles = handles.filter(handle => handle.agent.id !== mainId);
    expect(qqHandles).toHaveLength(1);
    expect(manager.getSessionRecord('c2c', PEER)?.handle).toBe(qqHandles[0]);
  });

  it('E2-3 singleflight 失败时两个调用方都收到同一错误，且不残留在途记录', async () => {
    new PrefsStore().setSessionId(KEY, 'session-current');
    const { registry } = createRegistry(async ({ resumeSessionId }) => { throw corrupt(resumeSessionId); });
    // 会话在盘上 → resume 失败不重建，整次调用以失败告终
    const manager = createManagerWithCtx(registry, onDiskCtx('session-current'));

    const first = manager.getOrCreate('c2c', PEER, PEER, REPLY);
    const second = manager.getOrCreate('c2c', PEER, PEER, REPLY);

    await expect(first).rejects.toThrow(/EIO/);
    await expect(second).rejects.toThrow(/EIO/);
    expect(registry.resume).toHaveBeenCalledTimes(1);

    // 在途记录已清理：下一次调用重新发起 resume
    await expect(manager.getOrCreate('c2c', PEER, PEER, REPLY)).rejects.toThrow(/EIO/);
    expect(registry.resume).toHaveBeenCalledTimes(2);
  });
});

describe('getOrCreate — E3 连带（prefs 损坏导致映射丢失）', () => {
  it('E3-4 prefs 文件损坏 → 回落 legacy 探测，且映射丢失留下可见日志', async () => {
    writeFileSync(join(env.home, 'model-prefs.json'), `{"overrides":{},"sessionIds":{"${KEY}":"session-cur`, 'utf8');
    const { registry } = createRegistry(async ({ resumeSessionId }) => fakeHandle(resumeSessionId));
    const logger = createLogger();

    const record = await createManager(registry, logger).getOrCreate('c2c', PEER, PEER, REPLY);

    expect(registry.resume).toHaveBeenCalledWith(expect.objectContaining({ resumeSessionId: LEGACY_ID }));
    expect(record.sessionId).toBe(LEGACY_ID);
    expect(allLogText(logger)).toMatch(/model-prefs\.json/); // 不再静默
  });
});

describe('getOrCreate — 会话创建/恢复后挂载到对应工作区', () => {
  /** 带 workspaceRegistry 的 ctx（sessionPersistence 缺省为「磁盘无会话」） */
  function workspaceCtx(workspaceRegistry: unknown): Context {
    return {
      get: (name: string) => {
        if (name === 'sessionPersistence') return { stat: async () => undefined };
        if (name === 'workspaceRegistry') return workspaceRegistry;
        return undefined;
      },
      on: vi.fn(),
    } as unknown as Context;
  }

  it('创建会话后用 workspaceDir() 挂载（cwd 与 meta.cwd 同源，避免 attachSession 校验失败）', async () => {
    const ws = { path: join(env.home, 'workspace'), attachSession: vi.fn().mockResolvedValue(undefined) };
    const workspaceRegistry = { create: vi.fn().mockResolvedValue(ws) };
    const { registry } = createRegistry(async ({ resumeSessionId }) => { throw notFound(resumeSessionId); });
    const manager = createManagerWithCtx(registry, workspaceCtx(workspaceRegistry));

    const record = await manager.getOrCreate('c2c', PEER, PEER, REPLY);

    // attach 是 fire-and-forget，等待其落到工作区
    await vi.waitFor(() => expect(ws.attachSession).toHaveBeenCalledWith(record.sessionId), { timeout: 3000 });
    // 关键回归：cwd 必须等于 meta.cwd（resolveAgentCwd(config.cwd)），而非 process.cwd()
    expect(workspaceRegistry.create).toHaveBeenCalledWith(join(env.home, 'workspace'), 'QQBot');
  });

  it('恢复会话时同样挂载（升级后老会话在下一次创建记录时补挂）', async () => {
    new PrefsStore().setSessionId(KEY, 'session-current');
    const ws = { path: join(env.home, 'workspace'), attachSession: vi.fn().mockResolvedValue(undefined) };
    const workspaceRegistry = { create: vi.fn().mockResolvedValue(ws) };
    const { registry } = createRegistry(async ({ resumeSessionId }) => fakeHandle(resumeSessionId));
    const manager = createManagerWithCtx(registry, workspaceCtx(workspaceRegistry));

    const record = await manager.getOrCreate('c2c', PEER, PEER, REPLY);

    expect(record.sessionId).toBe('session-current');
    await vi.waitFor(() => expect(ws.attachSession).toHaveBeenCalledWith('session-current'), { timeout: 3000 });
    expect(workspaceRegistry.create).toHaveBeenCalledWith(join(env.home, 'workspace'), 'QQBot');
  });
});

describe('QQ prompt 注册生命周期（per-agent 安装 / 释放 / live 复用重装）', () => {
  /**
   * 带 systemPrompt 替身的 agent：section/context 注册进 Map，disposer 从 Map 删除，
   * 用于观察 QQ prompt sections/contexts 的安装与释放。
   */
  function promptAgent(id: string, opts: { failSection?: boolean } = {}) {
    const sections = new Map<string, { name: string }>();
    const contexts = new Map<string, { name: string }>();
    const service = {
      section: (s: { name: string }) => {
        if (opts.failSection) throw new Error('boom: section registration rejected');
        sections.set(s.name, s);
        return () => { sections.delete(s.name); };
      },
      context: (c: { name: string }) => {
        contexts.set(c.name, c);
        return () => { contexts.delete(c.name); };
      },
    };
    const agent = {
      ...fakeAgent(id),
      ctx: { get: (name: string) => (name === 'systemPrompt' ? service : undefined) },
    } as unknown as DshAgent;
    return { agent, sections, contexts };
  }

  /** registry：create 产出带 prompt 替身的 agent，并让后续 get() 能复用它（live 路径） */
  function promptRegistry(opts: { failSection?: boolean } = {}) {
    let live: DshAgent | undefined;
    const created: Array<{ agent: DshAgent; sections: Map<string, unknown>; contexts: Map<string, unknown> }> = [];
    const registry = {
      get: vi.fn(() => live),
      resume: vi.fn(async ({ resumeSessionId }: { resumeSessionId: string }) => { throw notFound(resumeSessionId); }),
      create: vi.fn(async (options: { sessionId: string }) => {
        const { agent, sections, contexts } = promptAgent(options.sessionId, opts);
        created.push({ agent, sections, contexts });
        live = agent;
        return { agent, dispose: vi.fn(async () => {}) } as DshAgentHandle;
      }),
    };
    return { registry, created };
  }

  it('会话建立即注册 3 个 section + 2 个 context；remove() 全部释放', async () => {
    const { registry, created } = promptRegistry();
    const manager = createManager(registry);

    // K9：主会话也会用同一 registry.create 建 agent（不装 QQ prompt），按 agent 定位 QQ 子会话
    const record = await manager.getOrCreate('c2c', PEER, PEER, REPLY);
    const child = created.find(entry => entry.agent === record.agent);
    expect(child).toBeDefined();
    expect([...child!.sections.keys()].sort()).toEqual(['qqbot:extra', 'qqbot:persona', 'qqbot:rules']);
    expect([...child!.contexts.keys()].sort()).toEqual(['qqbot:background', 'qqbot:turn-meta']);

    await manager.remove('c2c', PEER);
    expect(child!.sections.size).toBe(0);
    expect(child!.contexts.size).toBe(0);
  });

  it('extra section 惰性读取 config：改 directPrompt 即时生效，无需重装', async () => {
    const { registry, created } = promptRegistry();
    const config = ConfigSchema({
      appId: APP_ID, appSecret: 'secret', provider: 'p', model: 'm',
      cwd: join(env.home, 'workspace'), sessionIdleTimeout: 0,
      groupPrompt: 'OLD_GROUP', directPrompt: 'OLD_DIRECT',
    } as ImQQBotConfig);
    const manager = new SessionManager(absentCtx(), registry as unknown as DshAgentRegistry, config, createLogger());
    managers.push(manager);

    const record = await manager.getOrCreate('c2c', PEER, PEER, REPLY);
    const child = created.find(entry => entry.agent === record.agent);
    const extra = child?.sections.get('qqbot:extra') as { text: () => string };
    expect(extra.text()).toBe('OLD_DIRECT'); // c2c 会话读 directPrompt

    config.directPrompt = 'NEW_DIRECT'; // 热更新配置（Schema 代理可写）
    expect(extra.text()).toBe('NEW_DIRECT');
  });

  it('live 复用：释放后的 agent 再次被 get() 命中时重新安装 prompt（WeakSet 实现下会永久丢失）', async () => {
    const { registry, created } = promptRegistry();
    const manager = createManager(registry);

    // 第一次建立：create 产出 agent A 并注册 prompt（主会话 agent 另算）
    const first = await manager.getOrCreate('c2c', PEER, PEER, REPLY);
    const agent = first.agent;
    const child = created.find(entry => entry.agent === agent);
    expect(child).toBeDefined();

    // 回收（releasePrompt 释放注册，但 registry 中 agent A 仍存活——宿主未必同步销毁）
    await manager.remove('c2c', PEER);
    expect(child!.sections.size).toBe(0);

    // 第二次 getOrCreate：prefs 指向 remove 时铸造的新 id，registry.get 命中 A → live 复用
    const record = await manager.getOrCreate('c2c', PEER, PEER, REPLY);
    expect(record.agent).toBe(agent);
    // 未重建 QQ agent，纯复用（主会话 K9 按需创建不重复）
    const mainId = new PrefsStore().getSessionId(MAIN_KEY);
    expect(registry.create.mock.calls.filter(([options]) => options.sessionId !== mainId)).toHaveLength(1);
    // 关键回归：prompt 必须重新注册，而不是因"已安装"标记被跳过
    expect(child!.sections.size).toBe(3);
    expect(child!.contexts.size).toBe(2);
  });

  it('disposeAll 同样释放 prompt 注册项', async () => {
    const { registry, created } = promptRegistry();
    const manager = createManager(registry);
    const record = await manager.getOrCreate('c2c', PEER, PEER, REPLY);
    const child = created.find(entry => entry.agent === record.agent);

    await manager.disposeAll();
    expect(child!.sections.size).toBe(0);
    expect(child!.contexts.size).toBe(0);
  });

  it('注册失败 fail-soft：会话仍可用，warn 留痕，下次同 agent 重试', async () => {
    const { registry, created } = promptRegistry({ failSection: true });
    const logger = createLogger();
    const manager = createManagerWithCtx(registry, absentCtx(), logger);

    const record = await manager.getOrCreate('c2c', PEER, PEER, REPLY);
    expect(record.agent).toBeDefined(); // 安装失败不阻断会话建立
    expect(allLogText(logger)).toContain('qq prompt install failed');

    // 失败不入表：同 agent 再次触发安装时重试（此处模拟再次 remove→复用）
    const child = created.find(entry => entry.agent === record.agent);
    expect(manager.remove('c2c', PEER)).resolves.toBeUndefined();
    expect(child!.sections.size).toBe(0);
  });

  /** 带桥接服务替身的 ctx（K7：personaCard 来自 astrboxBridge.persona）。 */
  function bridgeCtx(persona: (entityId: string) => string): Context {
    const bridge = { entities: () => ['猫猫'], persona };
    return {
      get: (name: string) => {
        if (name === 'sessionPersistence') return { stat: async () => undefined };
        if (name === 'astrboxBridge') return bridge;
        return undefined;
      },
      on: vi.fn(),
    } as unknown as Context;
  }

  /** 取 QQ 子会话的 persona section spec。 */
  function personaSection(created: Array<{ agent: DshAgent; sections: Map<string, unknown> }>, agent: DshAgent) {
    const child = created.find(entry => entry.agent === agent);
    return child?.sections.get('qqbot:persona') as { text: () => string; interpolate?: boolean };
  }

  it('persona 段读取桥接服务：传本 bot 的 entity、同步返回服务人设、interpolate=false（K7）', async () => {
    const persona = vi.fn((entityId: string) => `# ${entityId}\n喜欢 {{猫薄荷}}`);
    const { registry, created } = promptRegistry();
    const config = ConfigSchema({
      appId: APP_ID, appSecret: 'secret', provider: 'p', model: 'm',
      cwd: join(env.home, 'workspace'), sessionIdleTimeout: 0, entity: ' 猫猫 ',
    } as ImQQBotConfig);
    const manager = new SessionManager(bridgeCtx(persona), registry as unknown as DshAgentRegistry, config, createLogger());
    managers.push(manager);

    const record = await manager.getOrCreate('c2c', PEER, PEER, REPLY);
    const section = personaSection(created, record.agent);
    // section 文本惰性求值：调用 text() 才读桥接（每步组装现读，热更新生效）
    const text = section.text();
    expect(persona).toHaveBeenCalledWith('猫猫'); // trim 后传给桥接
    expect(section.interpolate).toBe(false);
    expect(text).toContain('# 猫猫');
  });

  it('无桥接服务或未绑定实体时 persona 段为空串（K7 错误语义）', async () => {
    const noService = promptRegistry();
    const managerA = createManager(noService.registry);
    const recordA = await managerA.getOrCreate('c2c', PEER, PEER, REPLY);
    expect(personaSection(noService.created, recordA.agent).text()).toBe('');

    const unbound = promptRegistry();
    const config = ConfigSchema({
      appId: APP_ID, appSecret: 'secret', provider: 'p', model: 'm',
      cwd: join(env.home, 'workspace'), sessionIdleTimeout: 0,
    } as ImQQBotConfig);
    const managerB = new SessionManager(bridgeCtx(() => 'SHOULD_NOT_RENDER'), unbound.registry as unknown as DshAgentRegistry, config, createLogger());
    managers.push(managerB);
    const recordB = await managerB.getOrCreate('c2c', PEER, PEER, REPLY);
    expect(personaSection(unbound.created, recordB.agent).text()).toBe('');
  });

  it('桥接 persona 抛错时人设段为空串，不影响会话建立（K7 fail-soft）', async () => {
    const { registry, created } = promptRegistry();
    const config = ConfigSchema({
      appId: APP_ID, appSecret: 'secret', provider: 'p', model: 'm',
      cwd: join(env.home, 'workspace'), sessionIdleTimeout: 0, entity: '猫猫',
    } as ImQQBotConfig);
    const manager = new SessionManager(bridgeCtx(() => { throw new Error('bridge down'); }), registry as unknown as DshAgentRegistry, config, createLogger());
    managers.push(manager);

    const record = await manager.getOrCreate('c2c', PEER, PEER, REPLY);
    expect(record.agent).toBeDefined();
    expect(personaSection(created, record.agent).text()).toBe('');
  });

  it('K26：单会话提示词命中 scope+peerId 时追加在场景词后，未命中不注入', async () => {
    const config = ConfigSchema({
      appId: APP_ID, appSecret: 'secret', provider: 'p', model: 'm',
      cwd: join(env.home, 'workspace'), sessionIdleTimeout: 0,
      directPrompt: 'DIRECT_BASE', groupPrompt: 'GROUP_BASE',
      conversationPrompts: [
        { scope: 'c2c', peerId: PEER, prompt: 'C2C_ONLY' },
        { scope: 'group', peerId: 'g1', prompt: 'GROUP_ONLY' },
      ],
    } as ImQQBotConfig);

    /** 每个场景独立 manager（promptRegistry 的 get() 替身总是返回最近创建的 agent）。 */
    const extraTextFor = async (scope: 'c2c' | 'group', peerId: string): Promise<string> => {
      const { registry, created } = promptRegistry();
      const manager = new SessionManager(absentCtx(), registry as unknown as DshAgentRegistry, config, createLogger());
      managers.push(manager);
      const record = await manager.getOrCreate(scope, peerId, 'sender', { scope, targetId: peerId, msgId: 'm1' });
      const section = created.find(entry => entry.agent === record.agent)?.sections.get('qqbot:extra') as { text: () => string };
      return section.text();
    };

    expect(await extraTextFor('c2c', PEER)).toBe('DIRECT_BASE\n\nC2C_ONLY'); // 命中私聊追加
    expect(await extraTextFor('c2c', 'other-peer')).toBe('DIRECT_BASE'); // 未命中不注入
    expect(await extraTextFor('group', 'g1')).toBe('GROUP_BASE\n\nGROUP_ONLY'); // 命中群聊追加
    expect(await extraTextFor('group', 'other-group')).toBe('GROUP_BASE');
  });

  it('K27：配置 owner 时逐轮注入身份行（主人/非主人），发送者随最新消息更新', async () => {
    const config = ConfigSchema({
      appId: APP_ID, appSecret: 'secret', provider: 'p', model: 'm',
      cwd: join(env.home, 'workspace'), sessionIdleTimeout: 0,
      owner: { openid: PEER, name: '喵喵' },
    } as ImQQBotConfig);
    const { registry, created } = promptRegistry();
    const manager = new SessionManager(absentCtx(), registry as unknown as DshAgentRegistry, config, createLogger());
    managers.push(manager);

    const record = await manager.getOrCreate('c2c', PEER, PEER, REPLY);
    const turnMeta = created.find(entry => entry.agent === record.agent)
      ?.contexts.get('qqbot:turn-meta') as { text: () => string };
    expect(turnMeta.text()).toContain('对方身份：主人（喵喵）');

    // 同一会话下一轮换成非 owner 发送者 → 身份行随之变化（senderId 每条消息更新）
    const again = await manager.getOrCreate('c2c', PEER, 'stranger', REPLY);
    expect(again.agent).toBe(record.agent);
    expect(turnMeta.text()).toContain('对方身份：非主人');
  });

  it('K27：未配置 owner → turn-meta 不注入身份行（旧行为不变）', async () => {
    const { registry, created } = promptRegistry();
    const manager = createManager(registry);

    const record = await manager.getOrCreate('c2c', PEER, PEER, REPLY);
    const turnMeta = created.find(entry => entry.agent === record.agent)
      ?.contexts.get('qqbot:turn-meta') as { text: () => string };
    expect(turnMeta.text()).toBe('');
  });
});

describe('listQqSessions（K4：/qqbot/bots 的 sessions 字段）', () => {
  it('按本 bot appId 过滤并解析 scope/peer；只读、不唤醒会话', () => {
    const prefs = new PrefsStore();
    prefs.setSessionId(`qqbot:${APP_ID}:c2c:u1`, 'session-c2c');
    prefs.setSessionId(`qqbot:${APP_ID}:group:g1`, 'session-group');
    prefs.setSessionId('qqbot:other:c2c:u9', 'session-other'); // 其它 bot，不出现
    prefs.setSessionId(MAIN_KEY, 'session-main'); // 主会话不是 QQ 子会话
    const { registry } = createRegistry(async () => { throw new Error('should not resume'); });

    const sessions = createManager(registry).listQqSessions();

    expect(sessions).toEqual([
      { id: 'session-c2c', scope: 'c2c', peerId: 'u1' },
      { id: 'session-group', scope: 'group', peerId: 'g1' },
    ]);
    expect(registry.resume).not.toHaveBeenCalled();
    expect(registry.create).not.toHaveBeenCalled();
  });

  it('无历史会话时返回空数组', () => {
    const { registry } = createRegistry(async () => { throw new Error('should not resume'); });
    expect(createManager(registry).listQqSessions()).toEqual([]);
  });
});

describe('latestTarget（K15 主动直发目标）', () => {
  it('按 scope 取最近活跃记录的 replyTarget；同 scope 更新活跃者胜出', async () => {
    const { registry } = createRegistry(async ({ resumeSessionId }) => { throw notFound(resumeSessionId); });
    const manager = createManager(registry);

    const c2c = await manager.getOrCreate('c2c', 'u1', 'u1', { scope: 'c2c', targetId: 'u1', msgId: 'm1' });
    const group = await manager.getOrCreate('group', 'g1', 's1', { scope: 'group', targetId: 'g1', msgId: 'm2' });
    c2c.lastActivity = 100;
    group.lastActivity = 200;

    expect(manager.latestTarget('c2c')).toEqual(c2c.replyTarget);
    expect(manager.latestTarget('group')).toEqual(group.replyTarget);

    const c2c2 = await manager.getOrCreate('c2c', 'u2', 'u2', { scope: 'c2c', targetId: 'u2', msgId: 'm3' });
    c2c2.lastActivity = 300;
    expect(manager.latestTarget('c2c')).toEqual(c2c2.replyTarget);
  });

  it('无会话 / 会话被回收后返回 undefined；只读不唤醒 agent', async () => {
    const { registry } = createRegistry(async () => { throw new Error('should not resume'); });
    const manager = createManager(registry);

    expect(manager.latestTarget('c2c')).toBeUndefined();
    expect(manager.latestTarget('group')).toBeUndefined();
    expect(registry.resume).not.toHaveBeenCalled();
    expect(registry.create).not.toHaveBeenCalled();

    await manager.getOrCreate('c2c', 'u1', 'u1', { scope: 'c2c', targetId: 'u1', msgId: 'm1' });
    await manager.remove('c2c', 'u1');
    expect(manager.latestTarget('c2c')).toBeUndefined();
  });
});

describe('主会话（Neko 主体）与子会话父指针', () => {
  it('ensureMainSession 创建并持久化主会话，二次调用复用同一条', async () => {
    const { registry } = createRegistry(async () => { throw notFound('none'); });
    const manager = createManager(registry);

    const id = await manager.ensureMainSession();
    expect(id).toMatch(MINTED);
    expect(manager.mainSessionId).toBe(id);

    const calls = registry.create.mock.calls as unknown as Array<[{ sessionId: string; meta?: Record<string, unknown> }]>;
    expect(calls).toHaveLength(1);
    expect(calls[0]![0].meta?.cwd).toBe(join(env.home, 'workspace'));
    expect(calls[0]![0].meta?.parentSession).toBeUndefined();
    expect(calls[0]![0].meta?.origin).toBeUndefined();
    expect(new PrefsStore().getSessionId(MAIN_KEY)).toBe(id);

    expect(await manager.ensureMainSession()).toBe(id);
    expect(registry.create).toHaveBeenCalledTimes(1);
  });

  it('QQ 会话是独立顶层会话：不带 parentSession / origin=subagent（工作区可见）', async () => {
    const { registry } = createRegistry(async () => { throw notFound('none'); });
    const manager = createManager(registry);

    await manager.ensureMainSession();
    const record = await manager.getOrCreate('c2c', PEER, PEER, REPLY);

    const calls = registry.create.mock.calls as unknown as Array<[{ sessionId: string; meta?: Record<string, unknown> }]>;
    const child = calls.find(([options]) => options.sessionId === record.sessionId);
    expect(child?.[0].meta?.cwd).toBe(join(env.home, 'workspace'));
    expect(child?.[0].meta?.parentSession).toBeUndefined();
    expect(child?.[0].meta?.origin).toBeUndefined();
  });

  it('主会话首次创建失败后，getOrCreate 按需重试一次（K9）', async () => {
    const { registry } = createRegistry(async () => { throw notFound('none'); });
    const createImpl = registry.create.getMockImplementation();
    let first = true;
    registry.create.mockImplementation(async (options: { sessionId: string }) => {
      if (first) { first = false; throw new Error('preset mount failed'); }
      return createImpl?.(options as never) as never;
    });
    const logger = createLogger();
    const manager = createManagerWithCtx(registry, absentCtx(), logger);

    expect(await manager.ensureMainSession()).toBeNull(); // 启动时创建失败
    const record = await manager.getOrCreate('c2c', PEER, PEER, REPLY); // K9：消息进入时重试

    expect(manager.mainSessionId).toBeDefined();
    expect(allLogText(logger)).toContain('main session init failed');
    const calls = registry.create.mock.calls as unknown as Array<[{ sessionId: string; meta?: Record<string, unknown> }]>;
    const child = calls.find(([options]) => options.sessionId === record.sessionId);
    // 顶层会话：主会话只作为 NPC 主体单独存在，不再是 QQ 会话的父节点
    expect(child?.[0].meta?.parentSession).toBeUndefined();
    expect(child?.[0].meta?.origin).toBeUndefined();
  });

  it('主会话重试仍失败：只告警，QQ 会话照常创建（无父指针）', async () => {
    const { registry } = createRegistry(async () => { throw notFound('none'); });
    const createImpl = registry.create.getMockImplementation();
    let attempts = 0;
    registry.create.mockImplementation(async (options: { sessionId: string }) => {
      // 前两次都是主会话（启动 ensure + K9 重试），按调用顺序失败；第三次是 QQ 子会话
      attempts += 1;
      if (attempts <= 2) throw new Error('preset mount failed');
      return createImpl?.(options as never) as never;
    });
    const logger = createLogger();
    const manager = createManagerWithCtx(registry, absentCtx(), logger);

    expect(await manager.ensureMainSession()).toBeNull();
    const record = await manager.getOrCreate('c2c', PEER, PEER, REPLY);

    expect(record.sessionId).toMatch(MINTED);
    expect(manager.mainSessionId).toBeUndefined();
    expect(attempts).toBe(3); // 证明 getOrCreate 触发了第二次主会话尝试
    expect(allLogText(logger)).toContain('main session init failed');
    const calls = registry.create.mock.calls as unknown as Array<[{ sessionId: string; meta?: Record<string, unknown> }]>;
    const child = calls.find(([options]) => options.sessionId === record.sessionId);
    expect(child?.[0].meta?.parentSession).toBeUndefined();
  });
});

describe('会话标题命名：主体 / 私聊(xxx) / 群聊(xxx)', () => {
  /** 带 sessionTitle 替身的 ctx：记录 rename 请求。 */
  function titleCtx(titles: Array<{ session: unknown; title: string }>, current?: { title: string; source: { kind: string } }): Context {
    return {
      get: (name: string) => {
        if (name === 'sessionPersistence') return { stat: async () => undefined };
        if (name === 'sessionTitle') {
          return {
            get: () => current,
            rename: (session: unknown, title: string) => { titles.push({ session, title }); return { title }; },
          };
        }
        return undefined;
      },
      on: vi.fn(),
    } as unknown as Context;
  }

  it('主会话标题为「主体」，QQ 私聊/群聊为 私聊(openid前8) / 群聊(group前8)', async () => {
    const renames: Array<{ session: unknown; title: string }> = [];
    const { registry } = createRegistry(async () => { throw notFound('none'); });
    const manager = createManagerWithCtx(registry, titleCtx(renames));

    await manager.ensureMainSession();
    await manager.getOrCreate('c2c', '85D47757A89F71E06CD008AAC76C57F8', 'u1', REPLY);
    await manager.getOrCreate('group', '1EC36362906D509645E352607FA92685', 'u1', { scope: 'group', targetId: 'g1', msgId: 'm2' });

    const byTitle = renames.map((entry) => entry.title);
    expect(byTitle).toContain('主体');
    expect(byTitle).toContain('私聊(85D47757)');
    expect(byTitle).toContain('群聊(1EC36362)');
  });

  it('置空前，sessionTitle 替身放进 ctx 也能工作（无服务时静默）', async () => {
    const { registry } = createRegistry(async () => { throw notFound('none'); });
    const manager = createManagerWithCtx(registry, absentCtx());
    await expect(manager.ensureMainSession()).resolves.toMatch(MINTED);
    await expect(manager.getOrCreate('c2c', PEER, PEER, REPLY)).resolves.toBeDefined();
  });
});

describe('主会话 kickoff（K13）：空主会话主动发第一条消息', () => {
  it('主会话创建后 followup 一次 kickoff 提示词；二次 ensure 不重复发送', async () => {
    const { registry, handles } = createRegistry(async () => { throw notFound('none'); });
    const config = ConfigSchema({
      appId: APP_ID, appSecret: 'secret', provider: 'p', model: 'm',
      cwd: join(env.home, 'workspace'), sessionIdleTimeout: 0,
      mainSessionKickoff: '请向主人报到一句',
    } as ImQQBotConfig);
    const manager = new SessionManager(absentCtx(), registry as unknown as DshAgentRegistry, config, createLogger());
    managers.push(manager);

    const mainId = await manager.ensureMainSession();
    const mainHandle = handles.find((handle) => (handle.agent.session as { id: string }).id === mainId);
    expect(mainHandle?.agent.followup).toHaveBeenCalledTimes(1);
    const message = vi.mocked(mainHandle!.agent.followup).mock.calls[0]![0] as { content: Array<{ text: string }> };
    expect(message.content[0]!.text).toContain('请向主人报到一句');

    await manager.ensureMainSession();
    expect(mainHandle?.agent.followup).toHaveBeenCalledTimes(1);
  });

  it('kickoff 为空 → 不发送（安静模式）', async () => {
    const { registry, handles } = createRegistry(async () => { throw notFound('none'); });
    const config = ConfigSchema({
      appId: APP_ID, appSecret: 'secret', provider: 'p', model: 'm',
      cwd: join(env.home, 'workspace'), sessionIdleTimeout: 0, mainSessionKickoff: '',
    } as ImQQBotConfig);
    const manager = new SessionManager(absentCtx(), registry as unknown as DshAgentRegistry, config, createLogger());
    managers.push(manager);

    const mainId = await manager.ensureMainSession();
    const mainHandle = handles.find((handle) => (handle.agent.session as { id: string }).id === mainId);
    expect(mainHandle?.agent.followup).not.toHaveBeenCalled();
  });

  it('主会话已有 turn/start 历史 → 不再 kickoff（避免每次重启打扰）', async () => {
    const persistedMain = 'session-main-with-history';
    new PrefsStore().setSessionId(MAIN_KEY, persistedMain);
    const agent = fakeAgent(persistedMain);
    (agent.session as unknown as { snapshotEvents: () => unknown[] }).snapshotEvents = () => [{ type: 'turn/start' }];
    const registry = {
      get: vi.fn(() => undefined),
      resume: vi.fn(async () => ({ agent, dispose: vi.fn(async () => {}) })),
      create: vi.fn(async () => { throw new Error('must not create'); }),
    };
    const config = ConfigSchema({
      appId: APP_ID, appSecret: 'secret', provider: 'p', model: 'm',
      cwd: join(env.home, 'workspace'), sessionIdleTimeout: 0, mainSessionKickoff: '报到',
    } as ImQQBotConfig);
    const manager = new SessionManager(
      persistenceCtx(async () => ({ header: {} })),
      registry as unknown as DshAgentRegistry,
      config,
      createLogger(),
    );
    managers.push(manager);

    expect(await manager.ensureMainSession()).toBe(persistedMain);
    expect(agent.followup).not.toHaveBeenCalled();
  });
});

describe('归档会话（K10）：不得 resume，必须新建会话', () => {
  it('prefs 指向已归档会话 → 改写映射并创建新会话，不向归档 id 发 resume', async () => {
    const archivedId = 'session-archived-child';
    new PrefsStore().setSessionId(KEY, archivedId);
    const resumedIds: string[] = [];
    const { registry } = createRegistry(async ({ resumeSessionId }) => {
      resumedIds.push(resumeSessionId);
      throw notFound(resumeSessionId);
    });

    const record = await createManagerWithCtx(registry, archivedCtx([archivedId]))
      .getOrCreate('c2c', PEER, PEER, REPLY);

    expect(record.sessionId).not.toBe(archivedId);
    expect(resumedIds).not.toContain(archivedId);
    expect(persistedId()).toBe(record.sessionId);
    expect(createdIds(registry)).toEqual([record.sessionId]);
  });

  it('prefs 指向旧版 subagent 子会话 → 重铸为顶层会话（工作区可见）', async () => {
    const legacyChildId = 'session-legacy-subagent-child';
    new PrefsStore().setSessionId(KEY, legacyChildId);
    const ctx: Context = {
      get: (name: string) => {
        if (name === 'sessionPersistence') {
          return { stat: async (id: string) => (id === legacyChildId ? { header: { origin: 'subagent', parentSession: 'session-main' } } : undefined) };
        }
        if (name === 'workspaceRegistry') {
          return { create: vi.fn(async (path: string) => ({ path, attachSession: vi.fn(async () => {}) })), archivedSessionIds: [] };
        }
        return undefined;
      },
      on: vi.fn(),
    } as unknown as Context;
    const resumedIds: string[] = [];
    const { registry } = createRegistry(async ({ resumeSessionId }) => {
      resumedIds.push(resumeSessionId);
      throw notFound(resumeSessionId);
    });

    const record = await createManagerWithCtx(registry, ctx).getOrCreate('c2c', PEER, PEER, REPLY);

    expect(record.sessionId).not.toBe(legacyChildId);
    expect(resumedIds).not.toContain(legacyChildId);
    expect(persistedId()).toBe(record.sessionId);
    const calls = registry.create.mock.calls as unknown as Array<[{ sessionId: string; meta?: Record<string, unknown> }]>;
    const created = calls.find(([options]) => options.sessionId === record.sessionId);
    expect(created?.[0].meta?.origin).toBeUndefined();
    expect(created?.[0].meta?.parentSession).toBeUndefined();
  });

  it('内存中的会话被归档后再次来消息 → 丢弃旧记录并新建会话', async () => {
    const archived: string[] = [];
    const { registry } = createRegistry(async ({ resumeSessionId }) => { throw notFound(resumeSessionId); });
    const manager = createManagerWithCtx(registry, archivedCtx(archived));

    const first = await manager.getOrCreate('c2c', PEER, PEER, REPLY);
    archived.push(first.sessionId);
    const second = await manager.getOrCreate('c2c', PEER, PEER, REPLY);

    expect(second.sessionId).not.toBe(first.sessionId);
    expect(persistedId()).toBe(second.sessionId);
    expect(createdIds(registry)).toContain(second.sessionId);
  });

  it('主会话被归档 → ensureMainSession 重建新主会话（不复用归档 id）', async () => {
    const archivedMain = 'session-archived-main';
    new PrefsStore().setSessionId(MAIN_KEY, archivedMain);
    const { registry } = createRegistry(async ({ resumeSessionId }) => { throw notFound(resumeSessionId); });

    const manager = createManagerWithCtx(registry, archivedCtx([archivedMain]));
    const id = await manager.ensureMainSession();

    expect(id).not.toBeNull();
    expect(id).not.toBe(archivedMain);
    expect(new PrefsStore().getSessionId(MAIN_KEY)).toBe(id);
    expect(registry.create).toHaveBeenCalledWith(expect.objectContaining({ sessionId: id }));
  });

  it('未归档时行为不变：内存记录直接复用，不新建', async () => {
    const { registry } = createRegistry(async ({ resumeSessionId }) => { throw notFound(resumeSessionId); });
    const manager = createManagerWithCtx(registry, archivedCtx([]));

    const first = await manager.getOrCreate('c2c', PEER, PEER, REPLY);
    const second = await manager.getOrCreate('c2c', PEER, PEER, REPLY);

    expect(second.sessionId).toBe(first.sessionId);
  });
});
