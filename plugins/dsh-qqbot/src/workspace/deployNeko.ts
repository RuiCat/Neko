/**
 * Neko 脚本系统部署 — 把共享 Neko 资源拷贝到 `PLUGIN_HOME/Neko`，幂等。
 *
 * 部署项：`scripts/`（.neko 脚本）、`data/templates/`（实体模板）、`bin/`（astrbox-mcp 等）。
 * 目标已存在则跳过，保留本地改动；源缺失只记录、不抛错（未安装 Neko 的项目可跳过）。
 */
import { cpSync, existsSync, mkdirSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { PLUGIN_HOME } from '../shared/dirs.ts';

/** 共享 Neko 脚本系统根目录：`PLUGIN_HOME/Neko`。 */
export const NEKO_HOME = resolve(PLUGIN_HOME, 'Neko');

/** 需要部署的 Neko 子路径（相对 Neko 源根）。 */
export const NEKO_SUBPATHS = ['scripts', 'data/templates', 'bin'] as const;

/** 部署选项。 */
export interface DeployNekoOptions {
  /** Neko 源根（含 scripts/、data/、bin/）；空或缺省表示不部署。 */
  sourceDir?: string;
  /** 目标根；缺省 `PLUGIN_HOME/Neko`。 */
  targetDir?: string;
}

/** 部署结果。 */
export interface DeployNekoResult {
  /** 本次拷入的子路径 */
  deployed: string[];
  /** 目标已存在、跳过（不覆盖）的子路径 */
  skipped: string[];
  /** 源或目标缺失的子路径 */
  missing: string[];
}

/**
 * 幂等部署 Neko 脚本系统。
 * @param options - 源根与目标根
 * @returns 每个子路径的处理结果
 */
export function deployNekoSystem(options: DeployNekoOptions = {}): DeployNekoResult {
  const result: DeployNekoResult = { deployed: [], skipped: [], missing: [] };
  const sourceDir = options.sourceDir?.trim();
  if (sourceDir === undefined || sourceDir === '') {
    result.missing.push('<source-unset>');
    return result;
  }
  if (!existsSync(sourceDir)) {
    result.missing.push(sourceDir);
    return result;
  }
  const targetDir = options.targetDir ?? NEKO_HOME;
  for (const sub of NEKO_SUBPATHS) {
    const from = resolve(sourceDir, sub);
    const to = resolve(targetDir, sub);
    if (!existsSync(from)) {
      result.missing.push(sub);
      continue;
    }
    if (existsSync(to)) {
      result.skipped.push(sub);
      continue;
    }
    mkdirSync(dirname(to), { recursive: true });
    cpSync(from, to, { recursive: true });
    result.deployed.push(sub);
  }
  return result;
}

/**
 * 解析 Neko 脚本系统源根。
 *
 * 优先级：显式参数 > 环境变量 `DSH_NEKO_SOURCE` > 仓库探测（向上找含 `scripts/` 与 `data/templates/` 的根）。
 * @param explicit - 调用方显式传入的源根
 * @returns 源根绝对路径；全部失败时 undefined（跳过部署，只记录 missing）
 */
export function resolveNekoSourceDir(explicit?: string): string | undefined {
  const fromOption = explicit?.trim();
  if (fromOption !== undefined && fromOption !== '') return fromOption;
  const fromEnv = process.env.DSH_NEKO_SOURCE?.trim();
  if (fromEnv !== undefined && fromEnv !== '') return fromEnv;
  return probeRepoRoot();
}

/** 从插件文件位置向上探测仓库根（含 scripts/ 与 data/templates/）。 */
function probeRepoRoot(): string | undefined {
  let here: string;
  try {
    here = dirname(fileURLToPath(import.meta.url));
  } catch {
    return undefined;
  }
  for (const up of ['../..', '../../..', '../../../..']) {
    const candidate = resolve(here, up);
    if (existsSync(resolve(candidate, 'scripts')) && existsSync(resolve(candidate, 'data/templates'))) {
      return candidate;
    }
  }
  return undefined;
}
