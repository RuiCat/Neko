/**
 * 插件数据目录 — 单一事实源。
 *
 * workspace / media / 模型偏好等子路径统一从 `PLUGIN_HOME` 派生，
 * 避免各模块各自 `resolve(homedir(), '.dsh-qqbot', ...)` 造成漂移。
 *
 * 目录名规则：机器人目录取**绑定实体名**（`bots/<entity>`，实体允许中文），
 * 未绑定实体时回退 bot id（`bots/<botId>`）。磁盘名与实体对齐，路由与配置键仍用 bot id。
 */
import { homedir } from 'node:os';
import { resolve } from 'node:path';

/** 插件数据根目录（区别于宿主的 ~/.dsh）：机器人数据与共享 Neko 脚本系统都放这里。 */
export const PLUGIN_HOME = resolve(homedir(), '.dsh-neko');

/** 目录名里不允许出现的字符：路径分隔符、Windows 保留字符、控制字符。 */
const UNSAFE_DIR_CHARS = /[\\/:*?"<>|\u0000-\u001f]/g;

/**
 * 把实体名收敛成安全目录名。
 * @param value - 实体 id（允许中文）
 * @returns 清理后的目录名；空、纯点或清理后为空时 undefined
 */
export function sanitizeDirName(value: string | undefined): string | undefined {
  const trimmed = value?.trim();
  if (trimmed === undefined || trimmed === '') return undefined;
  const cleaned = trimmed.replace(UNSAFE_DIR_CHARS, '');
  if (cleaned === '' || /^\.+$/.test(cleaned)) return undefined;
  return cleaned;
}

/**
 * 机器人目录名：实体名优先，回退 bot id。
 * @param botId - 配置里的 bot id（ASCII 受限）
 * @param entity - 绑定的实体 id（可空；允许中文）
 * @returns `bots/` 下的目录名
 */
export function botDirName(botId: string, entity?: string): string {
  return sanitizeDirName(entity) ?? botId;
}

/**
 * 单个机器人的数据根目录：`PLUGIN_HOME/bots/<entity|botId>`（workspace/media 按 bot 隔离）。
 * @param botId - bot id
 * @param entity - 绑定的实体 id（可选）
 * @returns 绝对路径
 */
export function botHome(botId: string, entity?: string): string {
  return resolve(PLUGIN_HOME, 'bots', botDirName(botId, entity));
}

/**
 * 富媒体下载根目录：旧单 bot 保持 `PLUGIN_HOME/media`；多 bot 用 `bots/<entity|botId>/media`，
 * 避免清理器按 TTL 误删其它 bot 的文件。
 * @param botId - 机器人 id
 * @param legacy - 是否为旧单 bot 配置（保持历史目录）
 * @param entity - 绑定的实体 id（可选）
 * @returns 下载根目录
 */
// 媒体目录入口：legacy 或无 botId → 旧全局目录；多 bot → per-bot 目录。
export function resolveMediaRoot(botId?: string, legacy = false, entity?: string): string {
  if (botId === undefined || legacy) return resolve(PLUGIN_HOME, 'media');
  return resolve(botHome(botId, entity), 'media');
}

/**
 * agent 工作目录：显式配置优先（空串视为未配置）。
 * 缺省值：多机器人按实体隔离为 `PLUGIN_HOME/bots/<entity|botId>/workspace`；
 * 未提供 botId（旧单 bot 调用/单测）保持 `PLUGIN_HOME/workspace` 不变。
 *
 * session 创建、路径白名单（send-file 工具与交付物投递）共用此解析，
 * 保证「agent 在哪写文件」与「白名单允许哪个目录」不漂移。
 * @param cwdConfig - 显式配置的工作目录
 * @param botId - 机器人 id（多 bot 时传入）
 * @param entity - 绑定的实体 id（可选；决定磁盘目录名）
 * @returns 解析后的工作目录
 */
// 目录解析入口：显式 cwd > 实体/bot 隔离缺省 > 旧单 bot 缺省。
export function resolveAgentCwd(cwdConfig: string | undefined, botId?: string, entity?: string): string {
  if (cwdConfig) return cwdConfig;
  return botId === undefined ? resolve(PLUGIN_HOME, 'workspace') : resolve(botHome(botId, entity), 'workspace');
}
