import { describe, it, expect, vi, beforeEach } from 'vitest';
import { createHostAdapter, _resetAdapterCache } from './adapter.ts';

/** 最小 ctx：可控的 get() */
function makeCtx(services: Record<string, unknown>) {
  return {
    get: (name: string) => {
      if (!(name in services)) throw new Error(`cannot get property "${name}" without inject`);
      return services[name];
    },
  } as never;
}

describe('host adapter', () => {
  beforeEach(() => {
    _resetAdapterCache();
    vi.unstubAllGlobals();
  });

  it('sessionEvents：新宿主 snapshotEvents() 优先', () => {
    const adapter = createHostAdapter(makeCtx({}));
    const events = [{ type: 'a', seq: 0 }];
    expect(adapter.sessionEvents({ snapshotEvents: () => events })).toBe(events);
  });

  it('sessionEvents：旧宿主 events getter 兜底', () => {
    const adapter = createHostAdapter(makeCtx({}));
    const events = [{ type: 'a', seq: 0 }];
    expect(adapter.sessionEvents({ events })).toEqual(events);
  });

  it('sessionEvents：两者皆无返回空数组（不抛）', () => {
    const adapter = createHostAdapter(makeCtx({}));
    expect(adapter.sessionEvents({})).toEqual([]);
  });

  it('serviceFor：旧宿主 presets 实例方法直接命中', async () => {
    const compaction = { compactNow: () => {} };
    const presets = { serviceFor: (agent: unknown, name: string) => (name === 'compaction' ? compaction : undefined) };
    const adapter = createHostAdapter(makeCtx({ agentPresets: presets }));
    expect(await adapter.serviceFor({ ctx: {} }, 'compaction')).toBe(compaction);
    expect(adapter.capabilities).toContain('serviceFor(presets实例方法)');
  });

  it('serviceFor：无实例方法时走动态模块函数', async () => {
    const compaction = { compactNow: () => {} };
    vi.doMock('@deepseek-ai/dsh-agent-presets', () => ({
      serviceForAgent: (_ctx: unknown, agent: unknown, name: string) =>
        name === 'compaction' ? compaction : undefined,
    }));
    const adapter = createHostAdapter(makeCtx({}));
    expect(await adapter.serviceFor({ ctx: {} }, 'compaction')).toBe(compaction);
  });

  it('globalService：未注册返回 undefined（不抛）', () => {
    const adapter = createHostAdapter(makeCtx({}));
    expect(adapter.globalService('compaction')).toBeUndefined();
  });

  it('globalService：已注册返回服务', () => {
    const compaction = { compactNow: () => {} };
    const adapter = createHostAdapter(makeCtx({ compaction }));
    expect(adapter.globalService('compaction')).toBe(compaction);
  });

  it('buildForkSeed：模块不可解析时返回 undefined（不抛）', async () => {
    vi.doMock('@deepseek-ai/dsh-session/fork', () => { throw new Error('module unavailable'); });
    const adapter = createHostAdapter(makeCtx({}));
    expect(await adapter.buildForkSeed([{ seq: 0 }], 0)).toBeUndefined();
  });

  it('sessionHeader：取 stat().header（agentPreset 在 header 顶层）', async () => {
    const header = { id: 's1', agentPreset: 'coder' };
    const adapter = createHostAdapter(makeCtx({ sessionPersistence: { stat: async () => ({ header }) } }));
    expect(await adapter.sessionHeader('s1')).toBe(header);
  });

  it('sessionHeader：会话不存在（stat → undefined）或读取失败均返回 undefined（不抛）', async () => {
    const absent = createHostAdapter(makeCtx({ sessionPersistence: { stat: async () => undefined } }));
    expect(await absent.sessionHeader('s1')).toBeUndefined();

    const failing = createHostAdapter(makeCtx({
      sessionPersistence: { stat: async () => { throw new Error('EIO'); } },
    }));
    expect(await failing.sessionHeader('s1')).toBeUndefined();
  });

  it('sessionExists：stat 返回快照 → true，返回 undefined → false', async () => {
    const present = createHostAdapter(makeCtx({ sessionPersistence: { stat: async () => ({ header: { id: 's1' } }) } }));
    expect(await present.sessionExists('s1')).toBe(true);

    const absent = createHostAdapter(makeCtx({ sessionPersistence: { stat: async () => undefined } }));
    expect(await absent.sessionExists('s1')).toBe(false);
  });

  it('sessionExists：后端故障照常抛出（求证失败 ≠ 不存在）', async () => {
    const adapter = createHostAdapter(makeCtx({
      sessionPersistence: { stat: async () => { throw new Error('EIO: backend unavailable'); } },
    }));
    await expect(adapter.sessionExists('s1')).rejects.toThrow(/EIO/);
  });

  it('sessionExists：服务未注册 → undefined', async () => {
    const adapter = createHostAdapter(makeCtx({}));
    expect(await adapter.sessionExists('s1')).toBeUndefined();
    expect(adapter.capabilities).toContain('sessionPersistence.stat(不可用，会话存在性判定降级)');
  });

  it('sessionExists：服务形态不含 stat → undefined，不让 TypeError 冒出', async () => {
    // 回归：此前按不存在的 load() 调用，TypeError 被当成「后端故障」，
    // 全新会话被误判为「可能存在」而拒绝创建，用户侧表现为处理消息异常
    const adapter = createHostAdapter(makeCtx({ sessionPersistence: { open: async () => ({}) } }));
    expect(await adapter.sessionExists('s1')).toBeUndefined();
    expect(await adapter.sessionHeader('s1')).toBeUndefined();
  });
});

describe('attachSessionToWorkspace', () => {
  function makeLogger() {
    return { info: vi.fn(), warn: vi.fn(), debug: vi.fn() };
  }

  it('把会话挂载到 cwd 对应的工作区（title=qqbot）', async () => {
    const ws = { path: '/proj', attachSession: vi.fn().mockResolvedValue(undefined) };
    const registry = { create: vi.fn().mockResolvedValue(ws) };
    const logger = makeLogger();
    const adapter = createHostAdapter(makeCtx({ workspaceRegistry: registry }), logger);

    await adapter.attachSessionToWorkspace('sess-1', '/proj');

    expect(registry.create).toHaveBeenCalledWith('/proj', 'QQBot');
    expect(ws.attachSession).toHaveBeenCalledWith('sess-1');
    expect(logger.warn).not.toHaveBeenCalled();
  });

  it('workspaceRegistry 未挂载时静默降级（debug，不 warn）', async () => {
    const logger = makeLogger();
    const adapter = createHostAdapter(makeCtx({}), logger);

    await expect(adapter.attachSessionToWorkspace('sess-1', '/proj')).resolves.toBeUndefined();
    expect(logger.debug).toHaveBeenCalled();
    expect(logger.warn).not.toHaveBeenCalled();
  });

  it('registry 无可用 create() 时静默降级', async () => {
    const logger = makeLogger();
    const adapter = createHostAdapter(makeCtx({ workspaceRegistry: {} }), logger);

    await expect(adapter.attachSessionToWorkspace('sess-1', '/proj')).resolves.toBeUndefined();
    expect(logger.debug).toHaveBeenCalled();
    expect(logger.warn).not.toHaveBeenCalled();
  });

  it('create() 失败时吞掉异常并 warn', async () => {
    const logger = makeLogger();
    const registry = { create: vi.fn().mockRejectedValue(new Error('not a directory')) };
    const adapter = createHostAdapter(makeCtx({ workspaceRegistry: registry }), logger);

    await expect(adapter.attachSessionToWorkspace('sess-1', '/proj')).resolves.toBeUndefined();
    expect(logger.warn).toHaveBeenCalled();
  });

  it('attachSession() 失败时吞掉异常并 warn', async () => {
    const logger = makeLogger();
    const ws = { path: '/proj', attachSession: vi.fn().mockRejectedValue(new Error('cwd mismatch')) };
    const registry = { create: vi.fn().mockResolvedValue(ws) };
    const adapter = createHostAdapter(makeCtx({ workspaceRegistry: registry }), logger);

    await expect(adapter.attachSessionToWorkspace('sess-1', '/proj')).resolves.toBeUndefined();
    expect(logger.warn).toHaveBeenCalled();
  });

  it('create() 返回不含 attachSession 的对象时 warn（接口演进防护）', async () => {
    const logger = makeLogger();
    const registry = { create: vi.fn().mockResolvedValue({ path: '/proj' }) };
    const adapter = createHostAdapter(makeCtx({ workspaceRegistry: registry }), logger);

    await expect(adapter.attachSessionToWorkspace('sess-1', '/proj')).resolves.toBeUndefined();
    expect(logger.warn).toHaveBeenCalled();
  });
});

describe('isSessionArchived 与工作区命名（K10）', () => {
  function makeLogger() {
    return { info: vi.fn(), warn: vi.fn(), debug: vi.fn() };
  }

  it('archivedSessionIds 命中 → true；未命中或服务缺失 → false（fail-soft）', () => {
    const adapter = createHostAdapter(makeCtx({
      workspaceRegistry: { create: vi.fn(), archivedSessionIds: ['s-1'] },
    }));
    expect(adapter.isSessionArchived('s-1')).toBe(true);
    expect(adapter.isSessionArchived('s-2')).toBe(false);

    const absent = createHostAdapter(makeCtx({}));
    expect(absent.isSessionArchived('s-1')).toBe(false);
    // 服务形态不符（archivedSessionIds 不是数组）也按未归档处理
    const malformed = createHostAdapter(makeCtx({ workspaceRegistry: { create: vi.fn(), archivedSessionIds: 'nope' } }));
    expect(malformed.isSessionArchived('s-1')).toBe(false);
  });

  it('attach 传入 title：create 用它，且旧默认名 QQBot 会 setTitle 迁移', async () => {
    const logger = makeLogger();
    const ws = {
      path: '/proj',
      title: 'QQBot',
      setTitle: vi.fn().mockResolvedValue(undefined),
      attachSession: vi.fn().mockResolvedValue(undefined),
    };
    const registry = { create: vi.fn().mockResolvedValue(ws) };
    const adapter = createHostAdapter(makeCtx({ workspaceRegistry: registry }), logger);

    await adapter.attachSessionToWorkspace('sess-1', '/proj', '梦梦');

    expect(registry.create).toHaveBeenCalledWith('/proj', '梦梦');
    expect(ws.setTitle).toHaveBeenCalledWith('梦梦');
    expect(ws.attachSession).toHaveBeenCalledWith('sess-1');
    expect(logger.info).toHaveBeenCalled();
    expect(logger.warn).not.toHaveBeenCalled();
  });

  it('用户自定义的工作区名不被覆盖；setTitle 缺失时保持原名但照常 attach', async () => {
    const ws = { path: '/proj', title: '主人起的名', attachSession: vi.fn().mockResolvedValue(undefined) };
    const registry = { create: vi.fn().mockResolvedValue(ws) };
    const adapter = createHostAdapter(makeCtx({ workspaceRegistry: registry }), makeLogger());

    await adapter.attachSessionToWorkspace('sess-1', '/proj', '梦梦');

    expect(ws.attachSession).toHaveBeenCalledWith('sess-1');
    const noSetter = { path: '/proj', title: 'QQBot', attachSession: vi.fn().mockResolvedValue(undefined) };
    const registry2 = { create: vi.fn().mockResolvedValue(noSetter) };
    await createHostAdapter(makeCtx({ workspaceRegistry: registry2 }), makeLogger())
      .attachSessionToWorkspace('sess-2', '/proj', '猫猫');
    expect(noSetter.attachSession).toHaveBeenCalledWith('sess-2');
  });

  it('setTitle 失败 → warn 留痕但 attach 照常完成（fail-soft）', async () => {
    const logger = makeLogger();
    const ws = {
      path: '/proj',
      title: 'QQBot',
      setTitle: vi.fn().mockRejectedValue(new Error('disk full')),
      attachSession: vi.fn().mockResolvedValue(undefined),
    };
    const registry = { create: vi.fn().mockResolvedValue(ws) };
    const adapter = createHostAdapter(makeCtx({ workspaceRegistry: registry }), logger);

    await expect(adapter.attachSessionToWorkspace('sess-1', '/proj', '梦梦')).resolves.toBeUndefined();
    expect(logger.warn).toHaveBeenCalled();
    expect(ws.attachSession).toHaveBeenCalledWith('sess-1');
  });
});

describe('ensureSessionTitle（工作区会话命名）', () => {
  it('自动标题 → 用 user-owned rename 写入目标标题', () => {
    const renames: Array<{ title: string }> = [];
    const titles = {
      get: () => ({ title: '主会话启动报到', source: { kind: 'provider' } }),
      rename: (_session: unknown, title: string) => { renames.push({ title }); return { title }; },
    };
    const adapter = createHostAdapter(makeCtx({ sessionTitle: titles }));
    const agent = { session: { id: 's1' } };

    adapter.ensureSessionTitle(agent, '主体');

    expect(renames).toEqual([{ title: '主体' }]);
  });

  it('用户手动标题 → 不覆盖；本插件标题 → 允许更新', () => {
    const renames: string[] = [];
    const make = (title: string, kind: string) => createHostAdapter(makeCtx({
      sessionTitle: {
        get: () => ({ title, source: { kind } }),
        rename: (_session: unknown, next: string) => { renames.push(next); return { title: next }; },
      },
    }));
    make('我的私聊记录', 'user').ensureSessionTitle({ session: { id: 's1' } }, '私聊(85D47757)');
    expect(renames).toEqual([]);
    make('私聊(旧ID)', 'user').ensureSessionTitle({ session: { id: 's1' } }, '私聊(85D47757)');
    expect(renames).toEqual(['私聊(85D47757)']);
  });

  it('标题服务缺失/rename 抛错 → 静默不抛（fail-soft）', () => {
    const noService = createHostAdapter(makeCtx({}));
    expect(() => noService.ensureSessionTitle({ session: { id: 's1' } }, '主体')).not.toThrow();

    const logger = { info: vi.fn(), warn: vi.fn(), debug: vi.fn() };
    const throwing = createHostAdapter(makeCtx({
      sessionTitle: {
        get: () => ({ title: '自动', source: { kind: 'fallback' } }),
        rename: () => { throw new Error('not live'); },
      },
    }), logger);
    expect(() => throwing.ensureSessionTitle({ session: { id: 's1' } }, '主体')).not.toThrow();
    expect(logger.warn).toHaveBeenCalled();
  });
});
