/**
 * READY 账号身份解析测试：昵称/账号 id 提取与坏负载容错。
 */
import { describe, expect, it } from 'vitest';
import { readAccountInfo } from './accountInfo.ts';

describe('readAccountInfo', () => {
  it('读取 READY 负载里的 user.id / user.username', () => {
    expect(readAccountInfo({ user: { id: 'u1', username: '猫猫号' } })).toEqual({ id: 'u1', name: '猫猫号' });
  });

  it('缺字段时返回空对象', () => {
    expect(readAccountInfo(undefined)).toEqual({});
    expect(readAccountInfo(null)).toEqual({});
    expect(readAccountInfo({})).toEqual({});
    expect(readAccountInfo({ user: {} })).toEqual({});
  });

  it('类型不符或空昵称时忽略对应字段', () => {
    expect(readAccountInfo({ user: { id: 1, username: '   ' } })).toEqual({});
    expect(readAccountInfo({ user: { id: 'x', username: 5 } })).toEqual({ id: 'x' });
    expect(readAccountInfo({ user: { id: '', username: '名' } })).toEqual({ name: '名' });
  });
});
