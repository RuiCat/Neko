import { describe, it, expect } from 'vitest';
import { ConfigSchema } from './config.ts';

describe('ConfigSchema 默认值（内层外层默认必须对齐：#94-1 不变量）', () => {
  it('完全省略 access → c2cMode open（子 schema 默认生效）', () => {
    const cfg = ConfigSchema({});
    expect(cfg.access.c2cMode).toBe('open');
    expect(cfg.access.groupMode).toBe('open');
  });

  it('access: {} → 子字段默认填充 open', () => {
    const cfg = ConfigSchema({ access: {} });
    expect(cfg.access.c2cMode).toBe('open');
  });

  it('access 空值（YAML access: 后无内容）→ open', () => {
    const cfg = ConfigSchema({ access: null });
    expect(cfg.access.c2cMode).toBe('open');
  });

  it('显式配置不受默认影响', () => {
    const cfg = ConfigSchema({ access: { c2cMode: 'allowlist' } });
    expect(cfg.access.c2cMode).toBe('allowlist');
  });

  it('闲置会话默认 30 分钟回收', () => {
    const cfg = ConfigSchema({});
    expect(cfg.sessionIdleTimeout).toBe(30 * 60 * 1000);
  });

  it.each([
    [{}, 'c2c'],
    [{ proactiveDelivery: {} }, 'c2c'],
    [{ proactiveDelivery: null }, 'c2c'],
    [{ proactiveDelivery: { externalTurns: 'off' } }, 'off'],
    [{ proactiveDelivery: { externalTurns: 'all' } }, 'all'],
  ])('proactiveDelivery.externalTurns：%j → %s', (input, expected) => {
    expect(ConfigSchema(input).proactiveDelivery.externalTurns).toBe(expected);
  });
});

describe('conversationPrompts Schema（K26）', () => {
  it('缺省 → 空数组；合法条目保留', () => {
    expect(ConfigSchema({}).conversationPrompts).toEqual([]);
    const cfg = ConfigSchema({
      conversationPrompts: [{ scope: 'c2c', peerId: 'u1', prompt: 'P' }],
    });
    expect(cfg.conversationPrompts).toEqual([{ scope: 'c2c', peerId: 'u1', prompt: 'P' }]);
  });

  it('非法 scope / 缺字段 → ConfigSchema 直接抛错（fail-fast）', () => {
    expect(() => ConfigSchema({ conversationPrompts: [{ scope: 'dm', peerId: 'u1', prompt: 'P' }] }))
      .toThrow(/scope/);
    expect(() => ConfigSchema({ conversationPrompts: [{ scope: 'c2c', prompt: 'P' }] }))
      .toThrow(/peerId/);
    expect(() => ConfigSchema({ conversationPrompts: [{ scope: 'c2c', peerId: 'u1' }] }))
      .toThrow(/prompt/);
  });

  it('bots[] 条目不填 conversationPrompts 时保持 undefined（继承顶层由 normalizeBots 处理）', () => {
    const cfg = ConfigSchema({
      conversationPrompts: [{ scope: 'c2c', peerId: 'u1', prompt: 'P' }],
      bots: [{ id: 'a', appId: 'a1', appSecret: 's1' }],
    });
    expect(cfg.bots[0]?.conversationPrompts).toBeUndefined();
  });
});

describe('owner Schema（K27）', () => {
  it('缺省 → undefined；合法对象保留（name 由归一化层补默认）', () => {
    expect(ConfigSchema({}).owner).toBeUndefined();
    expect(ConfigSchema({ owner: { openid: 'u1' } }).owner).toEqual({ openid: 'u1' });
    expect(ConfigSchema({ owner: { openid: 'u1', name: '喵喵' } }).owner).toEqual({ openid: 'u1', name: '喵喵' });
  });

  it('null 合法（设置页清空用），归一化层转 undefined', () => {
    expect(ConfigSchema({ owner: null }).owner).toBeNull();
  });

  it('缺 openid / 非对象 → Schema fail-fast', () => {
    expect(() => ConfigSchema({ owner: {} })).toThrow(/openid/);
    expect(() => ConfigSchema({ owner: 'u1' })).toThrow(/owner/);
  });

  it('bots[] 条目未写 owner → undefined（继承顶层由 normalizeBots 处理）', () => {
    const cfg = ConfigSchema({ bots: [{ id: 'a', appId: 'a1', appSecret: 's1' }] });
    expect(cfg.bots[0]?.owner).toBeUndefined();
  });
});
