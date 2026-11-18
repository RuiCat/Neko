/**
 * 工作区迁移：会话目录编码、目录/会话存储重命名、会话头 cwd 改写。
 */
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { zstdCompressSync, zstdDecompressSync } from 'node:zlib';
import { describe, expect, it } from 'vitest';
import { encodeSessionDir, migrateBotWorkspace } from './migrate.ts';

describe('encodeSessionDir', () => {
  it('与 DSH 会话目录编码一致', () => {
    expect(encodeSessionDir('/home/ruicat/.dsh-neko/workspace')).toBe('--home-ruicat-.dsh-neko-workspace--');
    expect(encodeSessionDir('/home/ruicat/.dsh-neko/bots/猫猫/workspace'))
      .toBe('--home-ruicat-.dsh-neko-bots-~732B~732B-workspace--');
    expect(encodeSessionDir('/a b/c')).toBe('--a~0020b-c--');
  });
});

describe('migrateBotWorkspace', () => {
  it('实体设置后重命名工作区与会话存储，并改写会话头 cwd', () => {
    const base = mkdtempSync(join(tmpdir(), 'qqbot-migrate-'));
    const oldWs = join(base, 'bots', 'bot-1', 'workspace');
    const newWs = join(base, 'bots', '猫猫', 'workspace');
    const sessions = join(base, 'sessions');
    const statePath = join(base, 'state.json');
    const oldCwd = oldWs;
    const newCwd = newWs;
    try {
      mkdirSync(oldWs, { recursive: true });
      writeFileSync(join(oldWs, 'AGENTS.md'), 'hello', 'utf8');
      const sessionDir = join(sessions, encodeSessionDir(oldCwd), 'session-x');
      mkdirSync(sessionDir, { recursive: true });
      const header = { type: 'session', id: 'session-x', cwd: oldCwd };
      writeFileSync(join(sessionDir, 'session.v4.jsonl.zstd'),
        zstdCompressSync(Buffer.from(`${JSON.stringify(header)}\n{"type":"other"}\n`, 'utf8')));
      writeFileSync(join(sessionDir, 'session.lock'), '', 'utf8');

      const result = migrateBotWorkspace({
        botId: 'bot-1',
        entity: '猫猫',
        sessionsRoot: sessions,
        statePath,
        resolveWorkspace: (_botId, entity) => (entity === undefined ? oldWs : newWs),
      });
      expect(result.moved).toBe(true);
      expect(existsSync(oldWs)).toBe(false);
      expect(readFileSync(join(newWs, 'AGENTS.md'), 'utf8')).toBe('hello');
      const movedSession = join(sessions, encodeSessionDir(newCwd), 'session-x', 'session.v4.jsonl.zstd');
      expect(existsSync(movedSession)).toBe(true);
      const first = zstdDecompressSync(readFileSync(movedSession)).toString('utf8').split('\n')[0]!;
      expect(JSON.parse(first).cwd).toBe(newCwd);
      expect(existsSync(join(sessions, encodeSessionDir(oldCwd)))).toBe(false);
      expect(JSON.parse(readFileSync(statePath, 'utf8'))['bot-1']).toBe(newWs);
    } finally {
      rmSync(base, { recursive: true, force: true });
    }
  });

  it('cwd 未变化时不移动', () => {
    const base = mkdtempSync(join(tmpdir(), 'qqbot-migrate-'));
    try {
      const result = migrateBotWorkspace({
        botId: 'bot-1',
        entity: '猫猫',
        sessionsRoot: join(base, 'sessions'),
        statePath: join(base, 'state.json'),
        resolveWorkspace: () => join(base, 'same'),
      });
      expect(result.moved).toBe(false);
    } finally {
      rmSync(base, { recursive: true, force: true });
    }
  });
});
