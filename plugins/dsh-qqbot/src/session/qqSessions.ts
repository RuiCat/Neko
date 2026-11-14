/**
 * QQ 子会话快照（K4：`GET /qqbot/bots` 的 `sessions` 字段）。
 *
 * 从 PrefsStore 的 sessionKey→sessionId 快照解析：只认本 bot 的
 * `qqbot:<appId>:(c2c|group):<peer>` 键；只读、不唤醒会话、不写盘。
 */
import type { QqSessionWire } from '../shared/bind-protocol.ts';

/** sessionKey 的解析结果。 */
export interface ParsedQqSessionKey {
  /** 私聊 / 群聊 */
  scope: 'c2c' | 'group';
  /** 对端 openid */
  peerId: string;
}

/**
 * 解析 `qqbot:<appId>:(c2c|group):<peer>`；只认指定 appId。
 * @param key - prefs 里的 sessionKey
 * @param appId - 本 bot 的 AppID
 * @returns scope + peerId；键不属于本 bot 或格式不符时 undefined
 */
export function parseQqSessionKey(key: string, appId: string): ParsedQqSessionKey | undefined {
  const match = /^qqbot:([^:]+):(c2c|group):(.+)$/.exec(key);
  if (match === null || match[1] !== appId) return undefined;
  const scope = match[2];
  const peerId = match[3];
  if ((scope !== 'c2c' && scope !== 'group') || peerId === undefined || peerId === '') return undefined;
  return { scope, peerId };
}

/**
 * 列出本 bot 的 QQ 子会话（主会话 key 天然不匹配，自动排除）。
 * @param entries - prefs 的 sessionKey→sessionId 快照
 * @param appId - 本 bot 的 AppID
 * @returns 子会话描述数组（保持快照顺序）
 */
export function collectQqSessions(
  entries: ReadonlyArray<{ sessionKey: string; sessionId: string }>,
  appId: string,
): QqSessionWire[] {
  const sessions: QqSessionWire[] = [];
  for (const { sessionKey, sessionId } of entries) {
    const parsed = parseQqSessionKey(sessionKey, appId);
    if (parsed === undefined) continue;
    sessions.push({ id: sessionId, scope: parsed.scope, peerId: parsed.peerId });
  }
  return sessions;
}
