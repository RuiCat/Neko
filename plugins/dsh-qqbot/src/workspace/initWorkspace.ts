/**
 * 工作区初始化 — 新增机器人时按模板建目录，幂等（已存在即跳过、不覆盖）。
 */
import { existsSync, mkdirSync, writeFileSync } from 'node:fs';
import { dirname } from 'node:path';
import {
  AGENTS_FILE,
  AGENTS_TEMPLATE,
  MEMORY_INDEX_FILE,
  MEMORY_INDEX_TEMPLATE,
  WORKSPACE_DIRS,
  workspacePath,
} from './workspaceLayout.ts';

/** 初始化结果：本次新建与已存在的条目分别列出，便于日志与测试断言。 */
export interface InitWorkspaceResult {
  /** 本次新建的目录/文件（相对 workspace 根） */
  created: string[];
  /** 已存在、本次跳过（不覆盖）的条目 */
  skipped: string[];
}

/** 只在文件缺失时写入，返回是否实际写入。 */
function writeIfMissing(path: string, content: string): boolean {
  if (existsSync(path)) return false;
  mkdirSync(dirname(path), { recursive: true });
  writeFileSync(path, content, 'utf8');
  return true;
}

/**
 * 按模板初始化一个 bot 工作区。
 *
 * 幂等：重复调用只补齐缺失项，绝不覆盖已有 `AGENTS.md` / `memory/INDEX.md`。
 * @param workspaceDir - 工作区绝对路径（由 `resolveAgentCwd` 解析）
 * @returns 本次新建与跳过的条目
 */
export function initWorkspace(workspaceDir: string): InitWorkspaceResult {
  const created: string[] = [];
  const skipped: string[] = [];

  for (const dir of WORKSPACE_DIRS) {
    const abs = workspacePath(workspaceDir, dir);
    if (existsSync(abs)) {
      skipped.push(dir);
      continue;
    }
    mkdirSync(abs, { recursive: true });
    created.push(dir);
  }

  if (writeIfMissing(workspacePath(workspaceDir, AGENTS_FILE), AGENTS_TEMPLATE)) created.push(AGENTS_FILE);
  else skipped.push(AGENTS_FILE);

  if (writeIfMissing(workspacePath(workspaceDir, MEMORY_INDEX_FILE), MEMORY_INDEX_TEMPLATE)) {
    created.push(MEMORY_INDEX_FILE);
  } else {
    skipped.push(MEMORY_INDEX_FILE);
  }

  return { created, skipped };
}
