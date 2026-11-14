/**
 * 工作区初始化入口 — 新增机器人时调用：建目录模板 + 部署共享 Neko 脚本系统。
 *
 * 单一入口 `ensureBotWorkspace` 供网关装配使用；两个子步骤各自幂等。
 */
import { resolveAgentCwd } from '../shared/dirs.ts';
import { initWorkspace, type InitWorkspaceResult } from './initWorkspace.ts';
import { deployNekoSystem, resolveNekoSourceDir, type DeployNekoResult } from './deployNeko.ts';

export { initWorkspace, type InitWorkspaceResult } from './initWorkspace.ts';
export { deployNekoSystem, NEKO_HOME, resolveNekoSourceDir, type DeployNekoOptions, type DeployNekoResult } from './deployNeko.ts';
export * from './workspaceLayout.ts';

/** 初始化选项。 */
export interface EnsureBotWorkspaceOptions {
  /** 显式配置的工作目录（优先于缺省 `bots/<entity|botId>/workspace`） */
  cwdConfig?: string;
  /** 绑定的实体 id（决定磁盘目录名） */
  entity?: string;
  /** Neko 脚本系统源根（缺省按环境变量/仓库探测） */
  nekoSourceDir?: string;
}

/** 初始化结果。 */
export interface EnsureBotWorkspaceResult {
  /** 解析出的工作区绝对路径 */
  workspaceDir: string;
  /** 目录模板结果 */
  workspace: InitWorkspaceResult;
  /** Neko 部署结果 */
  neko: DeployNekoResult;
}

/**
 * 初始化一个机器人的工作区（幂等）。
 * @param botId - 机器人 id
 * @param options - cwd/entity/Neko 源
 * @returns 解析路径与两步结果
 */
export function ensureBotWorkspace(botId: string, options: EnsureBotWorkspaceOptions = {}): EnsureBotWorkspaceResult {
  const workspaceDir = resolveAgentCwd(options.cwdConfig, botId, options.entity);
  const workspace = initWorkspace(workspaceDir);
  const neko = deployNekoSystem({ sourceDir: resolveNekoSourceDir(options.nekoSourceDir) });
  return { workspaceDir, workspace, neko };
}
