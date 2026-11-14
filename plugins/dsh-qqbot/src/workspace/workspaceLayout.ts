/**
 * workspace 目录模板 — 单个 bot 工作区结构的单一事实源。
 *
 * 与 `PLUGIN_HOME/Neko`（共享脚本系统）配套，新机器人初始化时按本表建目录：
 *   <workspace>/
 *     AGENTS.md              人设（缺省骨架，缺失才创建）
 *     memory/                记忆根
 *       INDEX.md             索引（地图，不是事实来源）
 *       diary/               日记、周记
 *       facts/               记忆摘要
 *       sessions/            会话状态
 *       history/             对话历史归档
 *       persona/             人设原文存档
 *     knowledge/             知识库
 *     materials/             资料落点
 *     .dsh/skills/           工作区技能
 *     neko/                  专用 Neko 脚本（本 bot 独有）
 */
import { resolve } from 'node:path';

/** 记忆根下的固定子目录名（英文目录 + 中文内容）。 */
export const MEMORY_SUBDIRS = ['diary', 'facts', 'sessions', 'history', 'persona'] as const;

/** 工作区必须存在的目录（相对 workspace 根；顺序即创建顺序）。 */
export const WORKSPACE_DIRS: readonly string[] = [
  'memory',
  ...MEMORY_SUBDIRS.map((name) => `memory/${name}`),
  'knowledge',
  'materials',
  '.dsh/skills',
  'neko',
];

/** 人设文件（权威落点，只在缺失时写骨架）。 */
export const AGENTS_FILE = 'AGENTS.md';

/** 记忆索引文件。 */
export const MEMORY_INDEX_FILE = 'memory/INDEX.md';

/** 记忆索引初始骨架。 */
export const MEMORY_INDEX_TEMPLATE = [
  '# 记忆索引',
  '',
  '> 本表是地图不是事实来源；原文优先于本表。',
  '',
  '## 日记（memory/diary/）',
  '',
  '| 文件 | 日期 | 首句 |',
  '|---|---|---|',
  '',
  '## 记忆摘要（memory/facts/）',
  '',
  '## 会话状态（memory/sessions/）',
  '',
  '## 对话历史（memory/history/）',
  '',
  '## 人设原文（memory/persona/）',
  '',
].join('\n');

/** 人设文件初始骨架。 */
export const AGENTS_TEMPLATE = [
  '# 人设',
  '',
  '> 本文件是人设与工作约定的权威落点（状态数据在实体里，不是人设）。',
  '',
].join('\n');

/**
 * 解析工作区内的相对路径。
 * @param workspaceDir - 工作区绝对路径
 * @param relative - 相对路径（用 `/` 分隔）
 * @returns 绝对路径
 */
export function workspacePath(workspaceDir: string, relative: string): string {
  return resolve(workspaceDir, relative);
}
