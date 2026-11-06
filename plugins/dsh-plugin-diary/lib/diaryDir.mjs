/**
 * 日记目录定位：优先 `DSH_DIARY_DIR`，其次向上找项目的 `.agent-memory/` 并在其下用 `diary/`，
 * 都没有时回落到 `<起始目录>/.dsh-diary`。
 *
 * 为什么向上找：会话 cwd 常常是项目子目录（子智能体、写域子路径），日记必须落到项目级
 * 而不是散在子目录里；为什么有回落：插件要在「没有记忆目录」的项目里也能用，不能直接报错。
 */
import { existsSync, mkdirSync } from "node:fs";
import { dirname, join, resolve } from "node:path";

const MEMORY_DIR_NAME = ".agent-memory";
const FALLBACK_DIR_NAME = ".dsh-diary";
const OVERRIDE_ENV = "DSH_DIARY_DIR";

/** 目录是否像一个项目记忆根（INDEX.md / topics / diary 任一存在）。 */
export function looksLikeMemoryDir(dir) {
  return (
    existsSync(join(dir, "INDEX.md")) ||
    existsSync(join(dir, "topics")) ||
    existsSync(join(dir, "diary"))
  );
}

/**
 * 解析日记目录（不创建）。
 * @param {string} [start] 起始目录，默认 process.cwd()
 * @returns {string} 绝对路径
 */
export function resolveDiaryDir(start = process.cwd()) {
  const override = process.env[OVERRIDE_ENV];
  if (override) return resolve(override);
  let current = resolve(start);
  while (true) {
    const candidate = join(current, MEMORY_DIR_NAME);
    if (looksLikeMemoryDir(candidate)) return join(candidate, "diary");
    const parent = dirname(current);
    if (parent === current) break;
    current = parent;
  }
  return join(resolve(start), FALLBACK_DIR_NAME);
}

/**
 * 解析并创建日记目录。
 * @param {string} [start] 起始目录
 * @returns {string} 已存在的绝对路径
 */
export function ensureDiaryDir(start = process.cwd()) {
  const dir = resolveDiaryDir(start);
  mkdirSync(dir, { recursive: true });
  return dir;
}

/** 从工具执行上下文里取会话 cwd（拿不到就用进程 cwd）。 */
export function cwdOf(exec) {
  const cwd = exec?.agent?.cwd;
  return typeof cwd === "string" && cwd.length > 0 ? cwd : process.cwd();
}
