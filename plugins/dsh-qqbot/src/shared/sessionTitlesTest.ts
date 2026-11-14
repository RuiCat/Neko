/**
 * 会话标题命名单测：主体 / 私聊(xxx) / 群聊(xxx)，以及「用户手动标题不覆盖」的判定。
 */
import { describe, expect, it } from 'vitest';
import {
  isOwnSessionTitle,
  mainSessionTitle,
  peerSessionTitle,
  shortPeerId,
  shouldApplySessionTitle,
} from './sessionTitles.ts';

describe('shortPeerId', () => {
  it('长 openid 取前 8 位；短值原样；空值兜底', () => {
    expect(shortPeerId('85D47757A89F71E06CD008AAC76C57F8')).toBe('85D47757');
    expect(shortPeerId('abc')).toBe('abc');
    expect(shortPeerId('   ')).toBe('未知');
  });
});

describe('peerSessionTitle', () => {
  it('私聊：有昵称用昵称，否则用 openid 短标识', () => {
    expect(peerSessionTitle('c2c', '85D47757A89F71E06CD008AAC76C57F8')).toBe('私聊(85D47757)');
    expect(peerSessionTitle('c2c', '85D47757A89F71E06CD008AAC76C57F8', ' 小明 ')).toBe('私聊(小明)');
  });

  it('群聊：用群 openid 短标识（QQ 群消息事件不带群名）', () => {
    expect(peerSessionTitle('group', '1EC36362906D509645E352607FA92685')).toBe('群聊(1EC36362)');
    // 群聊忽略 senderName（消息发送者昵称不是群名）
    expect(peerSessionTitle('group', '1EC36362906D509645E352607FA92685', '小明')).toBe('群聊(1EC36362)');
  });

  it('主会话标题固定为「主体」', () => {
    expect(mainSessionTitle()).toBe('主体');
  });
});

describe('shouldApplySessionTitle（用户标题保护）', () => {
  it('无标题/空标题/自动标题（fallback/provider）→ 可以写入', () => {
    expect(shouldApplySessionTitle(undefined)).toBe(true);
    expect(shouldApplySessionTitle({ title: '  ' })).toBe(true);
    expect(shouldApplySessionTitle({ title: '主会话启动报到', source: { kind: 'provider' } })).toBe(true);
    expect(shouldApplySessionTitle({ title: '主会话启动报到', source: { kind: 'fallback' } })).toBe(true);
  });

  it('本插件命名（主体/私聊()/群聊()）→ 允许更新', () => {
    for (const title of ['主体', '私聊(85D47757)', '群聊(1EC36362)']) {
      expect(isOwnSessionTitle(title)).toBe(true);
      expect(shouldApplySessionTitle({ title, source: { kind: 'user' } })).toBe(true);
    }
  });

  it('用户手动改过的其它标题 → 不覆盖', () => {
    expect(shouldApplySessionTitle({ title: '我的私聊记录', source: { kind: 'user' } })).toBe(false);
    expect(isOwnSessionTitle('我的私聊记录')).toBe(false);
  });
});
