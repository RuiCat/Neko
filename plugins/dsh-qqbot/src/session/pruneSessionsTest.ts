/**
 * 遗弃会话清理：只删「本工作区 + 不在保留集合 + 超过宽限期」的会话目录。
 */
import { existsSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { zstdCompressSync } from 'node:zlib';
import { describe, expect, it } from 'vitest';
import { pruneOrphanSessions } from './pruneSessions.ts';
import { encodeSessionDir } from '../workspace/migrate.ts';

/** 写一个会话目录（session.v4.jsonl.zstd 头）。 */
function writeSession(root: string, cwd: string, id: string, createdAt: number): void {
  const dir = join(root, encodeSessionDir(cwd), id);
  mkdirSync(dir, { recursive: true });
  const header = { type: 'session', id, cwd, createdAt };
  writeFileSync(join(dir, 'session.v4.jsonl.zstd'), zstdCompressSync(Buffer.from(`${JSON.stringify(header)}\n`, 'utf8')));
}

describe('pruneOrphanSessions', () => {
  it('只删除本工作区的旧孤儿会话', () => {
    const base = mkdtempSync(join(tmpdir(), 'qqbot-prune-'));
    const sessions = join(base, 'sessions');
    const workspace = join(base, 'bots', '猫猫', 'workspace');
    const now = 1_000_000_000_000;
    try {
      writeSession(sessions, workspace, 'session-orphan', now - 60 * 60 * 1000);
      writeSession(sessions, workspace, 'session-kept', now - 60 * 60 * 1000);
      writeSession(sessions, workspace, 'session-young', now - 60 * 1000);
      writeSession(sessions, join(base, 'bots', '梦梦', 'workspace'), 'session-other', now - 60 * 60 * 1000);
      const deleted = pruneOrphanSessions({
        workspaceDir: workspace,
        keep: new Set(['session-kept']),
        sessionsRoot: sessions,
        now: () => now,
      });
      expect(deleted).toEqual(['session-orphan']);
      const project = join(sessions, encodeSessionDir(workspace));
      expect(existsSync(join(project, 'session-orphan'))).toBe(false);
      expect(existsSync(join(project, 'session-kept'))).toBe(true);
      expect(existsSync(join(project, 'session-young'))).toBe(true);
      expect(existsSync(join(sessions, encodeSessionDir(join(base, 'bots', '梦梦', 'workspace')), 'session-other'))).toBe(true);
    } finally {
      rmSync(base, { recursive: true, force: true });
    }
  });
});
