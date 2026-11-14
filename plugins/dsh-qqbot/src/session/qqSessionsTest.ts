/**
 * qqSessions 纯函数单测：K4 sessions 字段的 key 解析与按 appId 过滤。
 */
import { describe, expect, it } from 'vitest';
import { collectQqSessions, parseQqSessionKey } from './qqSessions.ts';

describe('parseQqSessionKey', () => {
  it('解析本 bot 的 c2c/group 键', () => {
    expect(parseQqSessionKey('qqbot:app1:c2c:u1', 'app1')).toEqual({ scope: 'c2c', peerId: 'u1' });
    expect(parseQqSessionKey('qqbot:app1:group:g1', 'app1')).toEqual({ scope: 'group', peerId: 'g1' });
  });

  it('其它 appId / 非 QQ 键 / 非法 scope / 空 peer → undefined', () => {
    expect(parseQqSessionKey('qqbot:other:c2c:u1', 'app1')).toBeUndefined();
    expect(parseQqSessionKey('qqbot:app1:main', 'app1')).toBeUndefined();
    expect(parseQqSessionKey('qqbot:app1:dm:u1', 'app1')).toBeUndefined();
    expect(parseQqSessionKey('qqbot:app1:c2c:', 'app1')).toBeUndefined();
    expect(parseQqSessionKey('not-a-qq-key', 'app1')).toBeUndefined();
    expect(parseQqSessionKey('', 'app1')).toBeUndefined();
  });
});

describe('collectQqSessions', () => {
  it('按 appId 过滤并保持快照顺序；主会话键被排除', () => {
    const entries = [
      { sessionKey: 'qqbot:app1:c2c:u1', sessionId: 'session-a' },
      { sessionKey: 'qqbot:app1:main', sessionId: 'session-main' },
      { sessionKey: 'qqbot:other:c2c:u9', sessionId: 'session-other' },
      { sessionKey: 'qqbot:app1:group:g1', sessionId: 'session-b' },
    ];
    expect(collectQqSessions(entries, 'app1')).toEqual([
      { id: 'session-a', scope: 'c2c', peerId: 'u1' },
      { id: 'session-b', scope: 'group', peerId: 'g1' },
    ]);
  });

  it('空快照 / 全不匹配 → 空数组', () => {
    expect(collectQqSessions([], 'app1')).toEqual([]);
    expect(collectQqSessions([{ sessionKey: 'qqbot:other:c2c:u9', sessionId: 's' }], 'app1')).toEqual([]);
  });
});
