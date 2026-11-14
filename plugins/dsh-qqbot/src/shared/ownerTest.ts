/**
 * K27 owner 归一化校验单测：openid 非空、name 默认「主人」/trim/≤64、路径化错误。
 */
import { describe, expect, it } from 'vitest';
import {
  DEFAULT_OWNER_NAME,
  OWNER_NAME_MAX_CHARS,
  normalizeOwner,
} from './owner.ts';

describe('normalizeOwner（K27）', () => {
  it('缺省/null → undefined（未设置）', () => {
    expect(normalizeOwner(undefined)).toBeUndefined();
    expect(normalizeOwner(null)).toBeUndefined();
  });

  it('trim openid/name，name 缺省或空白 → 「主人」', () => {
    expect(normalizeOwner({ openid: '  u1  ' })).toEqual({ openid: 'u1', name: DEFAULT_OWNER_NAME });
    expect(normalizeOwner({ openid: 'u1', name: '   ' })).toEqual({ openid: 'u1', name: DEFAULT_OWNER_NAME });
    expect(normalizeOwner({ openid: 'u1', name: ' 喵喵 ' })).toEqual({ openid: 'u1', name: '喵喵' });
  });

  it('空 openid → 报错（含字段路径）', () => {
    for (const openid of ['', '   ', 42, undefined]) {
      expect(() => normalizeOwner({ openid })).toThrow(/owner\.openid 不能为空/);
    }
    expect(() => normalizeOwner({ openid: '' }, 'bots[1].owner')).toThrow(/bots\[1\]\.owner\.openid/);
  });

  it('name 非字符串 → 报错；≤64 合法、>64 报错', () => {
    expect(() => normalizeOwner({ openid: 'u1', name: 42 })).toThrow(/owner\.name 必须是字符串/);
    const atLimit = '名'.repeat(OWNER_NAME_MAX_CHARS);
    expect(normalizeOwner({ openid: 'u1', name: atLimit })?.name).toBe(atLimit);
    expect(() => normalizeOwner({ openid: 'u1', name: atLimit + '名' })).toThrow(/owner\.name 超长/);
  });

  it('非对象（数组/字符串）→ 报错', () => {
    expect(() => normalizeOwner([], 'owner')).toThrow(/owner 必须是对象/);
    expect(() => normalizeOwner('u1', 'owner')).toThrow(/owner 必须是对象/);
  });
});
