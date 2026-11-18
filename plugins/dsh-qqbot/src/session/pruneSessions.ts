/**
 * 遗弃会话自动清理 — 启动后删除该 bot 工作区里不再被引用的旧会话目录。
 *
 * 判定（只删同时满足）：
 * - 会话头 `cwd` 正好等于本 bot 的工作区（不会误删其它 bot/工作区）；
 * - 不在保留集合里（主会话、prefs 里该 appId 的 sessionKey→sessionId、内存中活动会话）；
 * - 创建时间早于宽限期（缺省 10 分钟），避免删掉刚创建、prefs 还没写入的会话。
 *
 * 删除粒度是 `~/.dsh/sessions/<编码 cwd>/<session-id>/`（含 zstd 日志与锁文件）；
 * 宿主没有公开的会话删除 API，这里直接删目录，失败只记日志。
 */
import { existsSync, readdirSync, readFileSync, rmSync } from 'node:fs';
import { homedir } from 'node:os';
import { resolve } from 'node:path';
import { zstdDecompressSync } from 'node:zlib';
import type { Logger } from '../types.ts';
import { encodeSessionDir } from '../workspace/migrate.ts';

/** 默认宽限期：新建会话在 prefs 落盘前的保护窗口。 */
const DEFAULT_MIN_AGE_MS = 10 * 60 * 1000;

/** pruneOrphanSessions 参数。 */
export interface PruneSessionsOptions {
  /** 本 bot 的工作区绝对路径 */
  workspaceDir: string;
  /** 保留的会话 id（主会话 + prefs 引用 + 活动会话） */
  keep: ReadonlySet<string>;
  /** 宽限期（毫秒），缺省 10 分钟 */
  minAgeMs?: number;
  /** 当前时间（测试注入） */
  now?: () => number;
  /** 会话根（缺省 ~/.dsh/sessions） */
  sessionsRoot?: string;
  /** 日志器 */
  logger?: Logger;
}

/** 读取会话头（zstd JSONL 第一行）；失败返回 undefined。 */
function readSessionHeader(file: string): { id?: string; cwd?: string; createdAt?: number } | undefined {
  try {
    const text = zstdDecompressSync(readFileSync(file)).toString('utf8');
    const newline = text.indexOf('\n');
    const first = newline < 0 ? text : text.slice(0, newline);
    const parsed = JSON.parse(first) as { id?: unknown; cwd?: unknown; createdAt?: unknown };
    return {
      ...(typeof parsed.id === 'string' ? { id: parsed.id } : {}),
      ...(typeof parsed.cwd === 'string' ? { cwd: parsed.cwd } : {}),
      ...(typeof parsed.createdAt === 'number' ? { createdAt: parsed.createdAt } : {}),
    };
  } catch {
    return undefined;
  }
}

/**
 * 删除本 bot 工作区下不再被引用的旧会话目录。
 * @param options - 工作区、保留集合与测试注入点
 * @returns 实际删除的会话 id（新数组）
 */
export function pruneOrphanSessions(options: PruneSessionsOptions): string[] {
  const root = options.sessionsRoot ?? resolve(homedir(), '.dsh', 'sessions');
  const projectDir = resolve(root, encodeSessionDir(options.workspaceDir));
  if (!existsSync(projectDir)) return [];
  const now = options.now?.() ?? Date.now();
  const minAgeMs = options.minAgeMs ?? DEFAULT_MIN_AGE_MS;
  const deleted: string[] = [];
  let entries: string[];
  try {
    entries = readdirSync(projectDir);
  } catch {
    return [];
  }
  for (const entry of entries) {
    if (!entry.startsWith('session-')) continue;
    const sessionDir = resolve(projectDir, entry);
    const header = readSessionHeader(resolve(sessionDir, 'session.v4.jsonl.zstd'));
    if (header === undefined) continue;
    const id = header.id ?? entry;
    if (options.keep.has(id) || options.keep.has(entry)) continue;
    if (header.cwd !== options.workspaceDir) continue;
    if (header.createdAt !== undefined && now - header.createdAt < minAgeMs) continue;
    try {
      rmSync(sessionDir, { recursive: true, force: true });
      deleted.push(id);
    } catch (error) {
      options.logger?.warn(`im-qqbot: 清理会话 ${id} 失败（跳过）: ${error instanceof Error ? error.message : String(error)}`);
    }
  }
  if (deleted.length > 0) {
    options.logger?.info(`im-qqbot: 已清理 ${deleted.length} 个遗弃会话: ${deleted.join(', ')}`);
  }
  return deleted;
}
