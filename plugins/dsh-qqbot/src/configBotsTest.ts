/**
 * configBots 归一化测试：旧配置迁移、bots[] 覆盖合并、校验与降级告警。
 */
import { describe, expect, it } from 'vitest';
import { ConfigSchema, type ImQQBotConfig } from './config.ts';
import { DEFAULT_BOT_ID, normalizeBots } from './configBots.ts';

/** ConfigSchema 的测试入口：运行时接受部分输入，类型层面收窄为完整配置。 */
function configOf(input: Record<string, unknown>): ImQQBotConfig {
  return ConfigSchema(input as unknown as ImQQBotConfig);
}

describe('normalizeBots：旧版单 bot 配置迁移', () => {
  it('没有 bots 时合成 default 机器人并沿用顶层凭据', () => {
    const config = configOf({ appId: 'app-1', appSecret: 'sec-1' });
    const result = normalizeBots(config);
    expect(result.legacy).toBe(true);
    expect(result.warnings).toEqual([]);
    expect(result.bots).toHaveLength(1);
    expect(result.bots[0]?.id).toBe(DEFAULT_BOT_ID);
    expect(result.bots[0]?.enabled).toBe(true);
    expect(result.bots[0]?.config.appId).toBe('app-1');
    expect(result.bots[0]?.config.appSecret).toBe('sec-1');
  });

  it('旧配置凭据为空时回退环境变量', () => {
    const config = configOf({ appId: '', appSecret: '' });
    const result = normalizeBots(config, { envAppId: 'env-app', envAppSecret: 'env-secret' });
    expect(result.bots[0]?.config.appId).toBe('env-app');
    expect(result.bots[0]?.config.appSecret).toBe('env-secret');
  });

  it('旧配置凭据是 __FROM_ENV__ 占位符时回退环境变量', () => {
    const config = configOf({ appId: '__FROM_ENV__', appSecret: '__FROM_ENV__' });
    const result = normalizeBots(config, { envAppId: 'env-app', envAppSecret: 'env-secret' });
    expect(result.bots[0]?.config.appId).toBe('env-app');
    expect(result.bots[0]?.config.appSecret).toBe('env-secret');
  });

  it('旧配置缺凭据时保持空串（交给扫码绑定流程）', () => {
    const config = configOf({});
    const result = normalizeBots(config);
    expect(result.legacy).toBe(true);
    expect(result.bots[0]?.config.appId).toBe('');
    expect(result.bots[0]?.enabled).toBe(true);
  });
});

describe('normalizeBots：bots[] 多机器人', () => {
  const twoBots = {
    provider: 'deepseek-official',
    model: 'deepseek-flash',
    preset: 'standard',
    bots: [
      { id: 'cat', label: '猫猫', appId: 'a1', appSecret: 's1', preset: 'neko', access: { c2cMode: 'allowlist', c2cAllow: ['u1'], groupMode: 'open', groupAllow: [] } },
      { id: 'helper', appId: 'a2', appSecret: 's2', requireMention: false },
    ],
  } as const;

  it('每项独立凭据；覆盖项生效，未覆盖项继承顶层', () => {
    const config = configOf(twoBots);
    const result = normalizeBots(config);
    expect(result.legacy).toBe(false);
    expect(result.bots.map(bot => bot.id)).toEqual(['cat', 'helper']);
    expect(result.bots[0]?.label).toBe('猫猫');
    expect(result.bots[0]?.config.appId).toBe('a1');
    expect(result.bots[0]?.config.appSecret).toBe('s1');
    expect(result.bots[0]?.config.preset).toBe('neko');
    expect(result.bots[0]?.config.access.c2cMode).toBe('allowlist');
    expect(result.bots[1]?.label).toBe('helper');
    expect(result.bots[1]?.config.appId).toBe('a2');
    expect(result.bots[1]?.config.model).toBe('deepseek-flash');
    expect(result.bots[1]?.config.requireMention).toBe(false);
    expect(result.bots[1]?.config.access.c2cMode).toBe(config.access.c2cMode);
  });

  it('多机器人模式下忽略环境变量并给 warning', () => {
    const config = configOf(twoBots);
    const result = normalizeBots(config, { envAppId: 'env-app', envAppSecret: 'env-secret' });
    expect(result.bots[0]?.config.appId).toBe('a1');
    expect(result.warnings.some(text => text.includes('QQBOT_APPID'))).toBe(true);
  });

  it('缺 appId 的机器人被停用并给 warning', () => {
    const config = configOf({ bots: [{ id: 'broken', enabled: true, appId: '', appSecret: '' }] });
    const result = normalizeBots(config);
    expect(result.bots[0]?.enabled).toBe(false);
    expect(result.warnings.some(text => text.includes('缺少 appId'))).toBe(true);
    expect(result.warnings.some(text => text.includes('没有可启动的机器人'))).toBe(true);
  });

  it('bots[] 里的 __FROM_ENV__ 占位符按缺凭据处理', () => {
    const config = configOf({ bots: [{ id: 'placeholder', enabled: true, appId: '__FROM_ENV__', appSecret: '' }] });
    const result = normalizeBots(config);
    expect(result.bots[0]?.enabled).toBe(false);
    expect(result.warnings.some(text => text.includes('缺少 appId/appSecret'))).toBe(true);
  });

  it('enabled:false 的机器人保留但停用', () => {
    const config = configOf({ bots: [{ id: 'off', appId: 'a1', appSecret: 's1', enabled: false }] });
    const result = normalizeBots(config);
    expect(result.bots[0]?.enabled).toBe(false);
    expect(result.warnings.some(text => text.includes('已禁用'))).toBe(true);
  });

  it('id 重复抛错', () => {
    const config = configOf({ bots: [{ id: 'dup', enabled: true, appId: 'a1', appSecret: 's1' }, { id: 'dup', enabled: true, appId: 'a2', appSecret: 's2' }] });
    expect(() => normalizeBots(config)).toThrow('id 重复');
  });

  it('appId 重复抛错（trim 后相同也算重复）', () => {
    const config = configOf({ bots: [{ id: 'aa', enabled: true, appId: 'same', appSecret: 's1' }, { id: 'bb', enabled: true, appId: ' same ', appSecret: 's2' }] });
    expect(() => normalizeBots(config)).toThrow('appId 重复');
  });

  it('非法 id 抛错', () => {
    const config = configOf({ bots: [{ id: 'BadId', enabled: true, appId: 'a1', appSecret: 's1' }] });
    expect(() => normalizeBots(config)).toThrow('id 非法');
  });
});

describe('normalizeBots：NPC 实体绑定（entity）', () => {
  it('bots[] 条目缺省继承顶层 entity；条目显式值优先', () => {
    const config = configOf({
      entity: '猫猫',
      bots: [
        { id: 'cat', appId: 'a1', appSecret: 's1' },
        { id: 'helper', appId: 'a2', appSecret: 's2', entity: '梦梦' },
      ],
    });
    const result = normalizeBots(config);
    expect(result.bots[0]?.config.entity).toBe('猫猫');
    expect(result.bots[1]?.config.entity).toBe('梦梦');
  });

  it('实体 id 允许中文，不套用 bot id 的 ASCII 规则；首尾空白被去掉', () => {
    const config = configOf({
      bots: [{ id: 'cat', appId: 'a1', appSecret: 's1', entity: '  梦梦  ' }],
    });
    const result = normalizeBots(config);
    expect(result.bots[0]?.config.entity).toBe('梦梦');
  });

  it('顶层与条目都是空白 → undefined（Schema 默认空串不算绑定）', () => {
    const config = configOf({
      entity: '   ',
      bots: [{ id: 'cat', appId: 'a1', appSecret: 's1', entity: '  ' }],
    });
    const result = normalizeBots(config);
    expect(result.bots[0]?.config.entity).toBeUndefined();
  });

  it('旧单 bot：顶层 entity 中文保留、空白归一为 undefined', () => {
    const bound = normalizeBots(configOf({ appId: 'a', appSecret: 's', entity: ' 猫猫 ' }));
    expect(bound.legacy).toBe(true);
    expect(bound.bots[0]?.config.entity).toBe('猫猫');

    const unbound = normalizeBots(configOf({ appId: 'a', appSecret: 's', entity: ' ' }));
    expect(unbound.bots[0]?.config.entity).toBeUndefined();
  });

  it('缺凭据被停用的条目同样带归一化后的 entity（配置层可见）', () => {
    const config = configOf({ entity: '猫猫', bots: [{ id: 'broken', appId: '', appSecret: '', entity: '梦梦' }] });
    const result = normalizeBots(config);
    expect(result.bots[0]?.enabled).toBe(false);
    expect(result.bots[0]?.config.entity).toBe('梦梦');
  });
});

describe('normalizeBots：单会话提示词（K26 conversationPrompts）', () => {
  it('旧单 bot：顶层列表归一（peerId trim，prompt 保留）', () => {
    const config = configOf({
      appId: 'a',
      appSecret: 's',
      conversationPrompts: [{ scope: 'c2c', peerId: '  u1  ', prompt: ' 仅私聊 ' }],
    });
    const result = normalizeBots(config);
    expect(result.bots[0]?.config.conversationPrompts).toEqual([{ scope: 'c2c', peerId: 'u1', prompt: ' 仅私聊 ' }]);
  });

  it('bots[] 条目缺省继承顶层；提供时整体替换（含空数组 = 清空）', () => {
    const config = configOf({
      conversationPrompts: [{ scope: 'c2c', peerId: 'u1', prompt: 'TOP' }],
      bots: [
        { id: 'aa', appId: 'a1', appSecret: 's1' },
        { id: 'bb', appId: 'a2', appSecret: 's2', conversationPrompts: [{ scope: 'group', peerId: 'g1', prompt: 'BOT' }] },
        { id: 'cc', appId: 'a3', appSecret: 's3', conversationPrompts: [] },
      ],
    });
    const result = normalizeBots(config);
    expect(result.bots[0]?.config.conversationPrompts).toEqual([{ scope: 'c2c', peerId: 'u1', prompt: 'TOP' }]);
    expect(result.bots[1]?.config.conversationPrompts).toEqual([{ scope: 'group', peerId: 'g1', prompt: 'BOT' }]);
    expect(result.bots[2]?.config.conversationPrompts).toEqual([]);
  });

  it('顶层重复 → fail-fast（错误带字段路径与重复键）', () => {
    const config = configOf({
      conversationPrompts: [
        { scope: 'c2c', peerId: 'u1', prompt: 'a' },
        { scope: 'c2c', peerId: 'u1', prompt: 'b' },
      ],
    });
    expect(() => normalizeBots(config)).toThrow(/conversationPrompts.*重复.*c2c:u1/);
  });

  it('bots[] 内超长 → fail-fast（错误带 bots[i] 路径）', () => {
    const config = configOf({
      bots: [{ id: 'aa', appId: 'a1', appSecret: 's1', conversationPrompts: [{ scope: 'c2c', peerId: 'u1', prompt: 'x'.repeat(80001) }] }],
    });
    expect(() => normalizeBots(config)).toThrow(/bots\[0\]\.conversationPrompts\[0\]\.prompt 超长/);
  });
});

describe('normalizeBots：主人身份（K27 owner）', () => {
  it('旧单 bot：顶层 owner 归一化（trim openid / name 默认「主人」）', () => {
    const bound = normalizeBots(configOf({ appId: 'a', appSecret: 's', owner: { openid: '  u1  ' } }));
    expect(bound.bots[0]?.config.owner).toEqual({ openid: 'u1', name: '主人' });

    const unset = normalizeBots(configOf({ appId: 'a', appSecret: 's', owner: null }));
    expect(unset.bots[0]?.config.owner).toBeUndefined();
  });

  it('bots[] 条目缺省继承顶层；提供时整体覆盖；null 显式清空', () => {
    const config = configOf({
      owner: { openid: 'top-owner', name: '顶顶' },
      bots: [
        { id: 'aa', appId: 'a1', appSecret: 's1' },
        { id: 'bb', appId: 'a2', appSecret: 's2', owner: { openid: ' bot-owner ', name: '' } },
        { id: 'cc', appId: 'a3', appSecret: 's3', owner: null },
      ],
    });
    const result = normalizeBots(config);
    expect(result.bots[0]?.config.owner).toEqual({ openid: 'top-owner', name: '顶顶' });
    expect(result.bots[1]?.config.owner).toEqual({ openid: 'bot-owner', name: '主人' });
    expect(result.bots[2]?.config.owner).toBeUndefined(); // null 覆盖后不继承顶层
  });

  it('非法 openid/超长 name → fail-fast 且带路径', () => {
    expect(() => normalizeBots(configOf({ appId: 'a', appSecret: 's', owner: { openid: '' } })))
      .toThrow(/owner\.openid 不能为空/);
    expect(() => normalizeBots(configOf({ bots: [{ id: 'aa', appId: 'a1', appSecret: 's1', owner: { openid: '' } }] })))
      .toThrow(/bots\[0\]\.owner\.openid 不能为空/);
    expect(() => normalizeBots(configOf({ appId: 'a', appSecret: 's', owner: { openid: 'u1', name: 'x'.repeat(65) } })))
      .toThrow(/owner\.name 超长/);
  });
});
