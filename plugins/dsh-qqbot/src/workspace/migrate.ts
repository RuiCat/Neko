/**
 * 机器人工作区迁移 — 实体/工作目录变化时**重命名**旧目录与旧会话存储，而不是新建。
 *
 * 规则：工作区 cwd = `bots/<实体名|botId>/workspace`。机器人第一次启动时若还没绑实体，
 * 会先落 `bots/<botId>/workspace`；之后设置实体，cwd 改变。这里把：
 *   1. 工作区目录整体 rename 到新 cwd（目标已存在则合并缺失项）；
 *   2. `~/.dsh/sessions/<编码 cwd>` 整体 rename 到新编码目录，并把每个会话头里的 cwd 改写为新路径；
 *   3. 上次 cwd 记录在 `<PLUGIN_HOME>/state/qqbot-workspaces.json`，下次启动据此判断。
 *
 * 纯同步 fs（启动路径上调用，必须在机器人挂载前完成），失败只记日志。
 */
import { existsSync, mkdirSync, readdirSync, readFileSync, renameSync, rmSync, statSync, writeFileSync } from 'node:fs';
import { homedir } from 'node:os';
import { dirname, resolve } from 'node:path';
import { zstdCompressSync, zstdDecompressSync } from 'node:zlib';
import type { Logger } from '../types.ts';
import { PLUGIN_HOME, resolveAgentCwd } from '../shared/dirs.ts';

/**
 * DSH 会话目录编码（与 `session-persistence-jsonl` 的 `projectKey` 一致）：
 * 分隔符收敛成单个 `-`（开头剥离），非安全字符按 code unit 转 `~XXXX`，整体包 `--…--`。
 */
export function encodeSessionDir(cwd: string): string {
  let readable = '';
  let separatorRun = false;
  for (let i = 0; i < cwd.length; i++) {
    const code = cwd.charCodeAt(i);
    const ch = String.fromCharCode(code);
    if (ch === '/' || ch === '\\' || ch === ':') {
      if (!separatorRun) readable += '-';
      separatorRun = true;
    } else if (ch !== '~' && /^[A-Za-z0-9._-]$/.test(ch)) {
      readable += ch;
      separatorRun = false;
    } else {
      readable += `~${code.toString(16).toUpperCase().padStart(4, '0')}`;
      separatorRun = false;
    }
  }
  const slug = readable.replace(/^-+/, '') || 'root';
  return `--${slug.slice(0, 251)}--`;
}

/** 会话状态文件默认落点。 */
export function defaultWorkspaceStatePath(): string {
  return resolve(PLUGIN_HOME, 'state', 'qqbot-workspaces.json');
}

/** 读上次 cwd 记录（缺失/损坏按空表处理）。 */
function readState(path: string): Record<string, string> {
  try {
    const parsed: unknown = JSON.parse(readFileSync(path, 'utf8'));
    return typeof parsed === 'object' && parsed !== null ? parsed as Record<string, string> : {};
  } catch {
    return {};
  }
}

/** 写回上次 cwd 记录。 */
function writeState(path: string, state: Record<string, string>): void {
  try {
    mkdirSync(dirname(path), { recursive: true });
    writeFileSync(path, `${JSON.stringify(state, null, 2)}\n`, 'utf8');
  } catch {
    // 状态只是下次迁移的依据，写不进去不影响机器人运行
  }
}

/** 把目录整体移到目标；目标已存在时合并缺失项后删除源目录。 */
function moveDir(from: string, to: string): void {
  if (!existsSync(from)) return;
  mkdirSync(dirname(to), { recursive: true });
  if (!existsSync(to)) {
    renameSync(from, to);
    return;
  }
  for (const entry of readdirSync(from)) {
    const src = resolve(from, entry);
    const dst = resolve(to, entry);
    if (!existsSync(dst)) renameSync(src, dst);
    else if (statSync(src).isDirectory() && statSync(dst).isDirectory()) moveDir(src, dst);
  }
  rmSync(from, { recursive: true, force: true });
}

/** 改写一个会话文件的 header cwd（zstd JSONL 的第一行）。 */
function rewriteSessionCwd(file: string, fromCwd: string, toCwd: string): void {
  try {
    const text = zstdDecompressSync(readFileSync(file)).toString('utf8');
    const newline = text.indexOf('\n');
    const first = newline < 0 ? text : text.slice(0, newline);
    const rest = newline < 0 ? '' : text.slice(newline);
    const header = JSON.parse(first) as { cwd?: string };
    if (header.cwd !== fromCwd) return;
    header.cwd = toCwd;
    writeFileSync(file, zstdCompressSync(Buffer.from(`${JSON.stringify(header)}${rest}`, 'utf8')), { mode: 0o600 });
  } catch {
    // 单个会话文件损坏不应该阻塞启动
  }
}

/** 会话存储目录改名并改写其中会话头的 cwd；旧锁文件直接丢弃。 */
function moveSessionStore(fromCwd: string, toCwd: string, sessionsRoot: string): void {
  const from = resolve(sessionsRoot, encodeSessionDir(fromCwd));
  const to = resolve(sessionsRoot, encodeSessionDir(toCwd));
  if (!existsSync(from) || from === to) return;
  moveDir(from, to);
  if (!existsSync(to)) return;
  for (const entry of readdirSync(to)) {
    const dir = resolve(to, entry);
    if (!statSync(dir).isDirectory()) continue;
    for (const file of readdirSync(dir)) {
      const path = resolve(dir, file);
      if (file.endsWith('.jsonl.zstd')) rewriteSessionCwd(path, fromCwd, toCwd);
      else if (file.endsWith('.lock')) rmSync(path, { force: true });
    }
  }
}

/** 迁移结果。 */
export interface WorkspaceMigrationResult {
  /** 迁移前的 cwd（无记录时为 undefined） */
  from?: string;
  /** 迁移后的 cwd */
  to: string;
  /** 是否真的发生了重命名 */
  moved: boolean;
}

/** migrateBotWorkspace 的可选注入点（测试用）。 */
export interface WorkspaceMigrationOptions {
  botId: string;
  cwdConfig?: string;
  entity?: string;
  logger?: Logger;
  /** 会话根（缺省 ~/.dsh/sessions） */
  sessionsRoot?: string;
  /** 状态文件（缺省 PLUGIN_HOME/state/qqbot-workspaces.json） */
  statePath?: string;
  /** 工作区路径解析（缺省 resolveAgentCwd；测试注入临时目录） */
  resolveWorkspace?: (botId: string, entity?: string) => string;
}

/**
 * 把机器人工作区与会话存储从上次 cwd 迁移到当前 cwd（同步、fail-soft）。
 * @param options - botId / 配置 cwd / 实体 + 测试注入点
 * @returns 迁移前后的 cwd 与是否发生重命名
 */
export function migrateBotWorkspace(options: WorkspaceMigrationOptions): WorkspaceMigrationResult {
  const sessionsRoot = options.sessionsRoot ?? resolve(homedir(), '.dsh', 'sessions');
  const statePath = options.statePath ?? defaultWorkspaceStatePath();
  const resolveWorkspace = options.resolveWorkspace
    ?? ((botId: string, entity?: string) => resolveAgentCwd(options.cwdConfig, botId, entity));
  const to = resolveWorkspace(options.botId, options.entity);
  const state = readState(statePath);
  const entity = options.entity?.trim() ?? '';
  // 首次迁移：还没记录过 cwd、但已绑定实体时，旧目录是未绑定时的 bots/<botId>/workspace。
  const from = state[options.botId] ?? (entity === '' ? undefined : resolveWorkspace(options.botId, undefined));
  let moved = false;
  if (from !== undefined && from !== to && existsSync(from)) {
    try {
      moveDir(from, to);
      moveSessionStore(from, to, sessionsRoot);
      moved = true;
      options.logger?.info(`im-qqbot: 机器人 ${options.botId} 工作区已迁移 ${from} → ${to}`);
    } catch (error) {
      options.logger?.warn(`im-qqbot: 机器人 ${options.botId} 工作区迁移失败（继续启动）: ${error instanceof Error ? error.message : String(error)}`);
    }
  }
  state[options.botId] = to;
  writeState(statePath, state);
  return { ...(from === undefined ? {} : { from }), to, moved };
}
