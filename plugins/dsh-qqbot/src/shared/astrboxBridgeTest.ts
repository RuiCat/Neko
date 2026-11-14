/**
 * astrboxBridge 只读访问层单测：K2 服务缺失/形状不符/抛错时全部 fail-soft（不抛给调用方）。
 */
import { describe, expect, it, vi } from 'vitest';
import type { Context } from '@deepseek-ai/cordis';
import {
  getAstrboxBridge,
  readBridgeEntities,
  readBridgeMainSessionPrompt,
  readBridgePersona,
  readBridgePersonaStatus,
  refreshBridge,
} from './astrboxBridge.ts';

/** 构造只暴露一个服务名的 ctx 替身。 */
function ctxWith(name: string, value: unknown): Context {
  return {
    get: (key: string) => (key === name ? value : undefined),
  } as unknown as Context;
}

/** 空上下文（无任何服务）。 */
function emptyCtx(): Context {
  return { get: () => undefined } as unknown as Context;
}

describe('getAstrboxBridge：形状校验', () => {
  it('无 get / 服务为 null / 形状不符 → undefined', () => {
    expect(getAstrboxBridge({} as unknown as Context)).toBeUndefined();
    expect(getAstrboxBridge(ctxWith('astrboxBridge', null))).toBeUndefined();
    expect(getAstrboxBridge(ctxWith('astrboxBridge', {}))).toBeUndefined();
    expect(getAstrboxBridge(ctxWith('astrboxBridge', { entities: () => [], persona: 'nope' }))).toBeUndefined();
  });

  it('get 本身抛错时返回 undefined（不向调用方抛）', () => {
    const ctx = {
      get: () => { throw new Error('proxy denied'); },
    } as unknown as Context;
    expect(() => getAstrboxBridge(ctx)).not.toThrow();
    expect(getAstrboxBridge(ctx)).toBeUndefined();
  });

  it('合法服务返回投影', () => {
    const bridge = { entities: () => ['猫猫'], persona: () => '# 猫猫' };
    expect(getAstrboxBridge(ctxWith('astrboxBridge', bridge))).toBe(bridge);
  });
});

describe('readBridgeEntities（K8）', () => {
  it('服务缺失 → undefined（wire 省略字段）', () => {
    expect(readBridgeEntities(emptyCtx())).toBeUndefined();
  });

  it('返回字符串数组；非字符串项被过滤', () => {
    const bridge = {
      entities: () => ['猫猫', '', 42, null, '梦梦'] as unknown as string[],
      persona: () => '',
    };
    expect(readBridgeEntities(ctxWith('astrboxBridge', bridge))).toEqual(['猫猫', '梦梦']);
  });

  it('返回值不是数组 → undefined', () => {
    const bridge = { entities: () => undefined as unknown as string[], persona: () => '' };
    expect(readBridgeEntities(ctxWith('astrboxBridge', bridge))).toBeUndefined();
  });

  it('entities() 抛错 → undefined', () => {
    const bridge = { entities: () => { throw new Error('down'); }, persona: () => '' };
    expect(readBridgeEntities(ctxWith('astrboxBridge', bridge))).toBeUndefined();
  });
});

describe('readBridgePersona（K7）', () => {
  it('未绑定实体（undefined/空串）→ 空串且不调用服务', () => {
    const persona = vi.fn(() => '# 猫猫');
    const ctx = ctxWith('astrboxBridge', { entities: () => [], persona });
    expect(readBridgePersona(ctx, undefined)).toBe('');
    expect(readBridgePersona(ctx, '')).toBe('');
    expect(persona).not.toHaveBeenCalled();
  });

  it('服务缺失 → 空串', () => {
    expect(readBridgePersona(emptyCtx(), '猫猫')).toBe('');
  });

  it('有服务 → 同步返回服务人设文本（含实体参数）', () => {
    const persona = vi.fn((entityId: string) => `# ${entityId}`);
    const ctx = ctxWith('astrboxBridge', { entities: () => [], persona });
    expect(readBridgePersona(ctx, '梦梦')).toBe('# 梦梦');
    expect(persona).toHaveBeenCalledWith('梦梦');
  });

  it('服务抛错或返回非字符串 → 空串（不向 system prompt 组装抛错）', () => {
    const throwing = ctxWith('astrboxBridge', { entities: () => [], persona: () => { throw new Error('down'); } });
    expect(readBridgePersona(throwing, '猫猫')).toBe('');
    const wrongType = ctxWith('astrboxBridge', { entities: () => [], persona: () => 42 as unknown as string });
    expect(readBridgePersona(wrongType, '猫猫')).toBe('');
  });
});

describe('readBridgePersonaStatus（K7 首轮未就绪可见性）', () => {
  it('有非空人设 → ready=true 且 text 一致', () => {
    const ctx = ctxWith('astrboxBridge', { entities: () => ['梦梦'], persona: () => '# 梦梦' });
    expect(readBridgePersonaStatus(ctx, '梦梦')).toEqual({ text: '# 梦梦', ready: true });
  });

  it('实体已绑定但人设空/服务缺失/抛错 → ready=false（调用方可 warn 一次）', () => {
    const empty = ctxWith('astrboxBridge', { entities: () => ['梦梦'], persona: () => '' });
    expect(readBridgePersonaStatus(empty, '梦梦')).toEqual({ text: '', ready: false });
    expect(readBridgePersonaStatus(emptyCtx(), '梦梦')).toEqual({ text: '', ready: false });
    const throwing = ctxWith('astrboxBridge', { entities: () => [], persona: () => { throw new Error('down'); } });
    expect(readBridgePersonaStatus(throwing, '梦梦')).toEqual({ text: '', ready: false });
  });

  it('未绑定实体（undefined/空串）→ ready=false 且不访问服务', () => {
    const persona = vi.fn(() => '# x');
    const ctx = ctxWith('astrboxBridge', { entities: () => [], persona });
    expect(readBridgePersonaStatus(ctx, undefined)).toEqual({ text: '', ready: false });
    expect(readBridgePersonaStatus(ctx, '')).toEqual({ text: '', ready: false });
    expect(persona).not.toHaveBeenCalled();
  });
});

describe('readBridgeMainSessionPrompt（K28 主会话提示词）', () => {
  it('命中返回 trim 后的文本；未配置/空实体 → 空串', () => {
    const bridge = { entities: () => [], persona: () => '', mainSessionPrompt: (id: string) => (id === '猫猫' ? '  你是猫猫  ' : '') };
    const ctx = ctxWith('astrboxBridge', bridge);
    expect(readBridgeMainSessionPrompt(ctx, ' 猫猫 ')).toBe('你是猫猫');
    expect(readBridgeMainSessionPrompt(ctx, '梦梦')).toBe('');
    expect(readBridgeMainSessionPrompt(ctx, undefined)).toBe('');
    expect(readBridgeMainSessionPrompt(ctx, '   ')).toBe('');
  });

  it('旧版桥接（无该方法）/服务缺失/抛错/非字符串 → 空串，不抛错', () => {
    expect(readBridgeMainSessionPrompt(emptyCtx(), '猫猫')).toBe('');
    const legacy = ctxWith('astrboxBridge', { entities: () => [], persona: () => '' });
    expect(readBridgeMainSessionPrompt(legacy, '猫猫')).toBe('');
    const throws = ctxWith('astrboxBridge', {
      entities: () => [],
      persona: () => '',
      mainSessionPrompt: () => { throw new Error('boom'); },
    });
    expect(() => readBridgeMainSessionPrompt(throws, '猫猫')).not.toThrow();
    expect(readBridgeMainSessionPrompt(throws, '猫猫')).toBe('');
    const weird = ctxWith('astrboxBridge', { entities: () => [], persona: () => '', mainSessionPrompt: () => 42 });
    expect(readBridgeMainSessionPrompt(weird, '猫猫')).toBe('');
  });
});

describe('refreshBridge（K3 预热）', () => {
  it('有 refresh 时 fire-and-forget 调用一次（不等待）', async () => {
    const refresh = vi.fn(async () => {});
    const ctx = ctxWith('astrboxBridge', { entities: () => [], persona: () => '', refresh });
    refreshBridge(ctx);
    expect(refresh).toHaveBeenCalledTimes(1);
    await Promise.resolve();
  });

  it('无服务/无 refresh/refresh 抛错或 reject 时全部静默', async () => {
    expect(() => refreshBridge(emptyCtx())).not.toThrow();
    expect(() => refreshBridge(ctxWith('astrboxBridge', { entities: () => [], persona: () => '' }))).not.toThrow();
    const syncThrow = ctxWith('astrboxBridge', { entities: () => [], persona: () => '', refresh: () => { throw new Error('sync'); } });
    expect(() => refreshBridge(syncThrow)).not.toThrow();
    const rejected = ctxWith('astrboxBridge', {
      entities: () => [],
      persona: () => '',
      refresh: () => Promise.reject(new Error('async')),
    });
    expect(() => refreshBridge(rejected)).not.toThrow();
    await Promise.resolve();
  });
});
