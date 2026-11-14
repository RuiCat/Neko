/**
 * 命令鉴权 — 挂到 SlashCommand.authorized 的白名单校验
 *
 * 名单语义与审批按钮校验（approval-channel）一致：c2cMode=allowlist 时发送者
 * 必须在 c2cAllow；身份缺失同样拒绝（fail-closed）。open 模式放行。
 * K27：allowlist 模式下 owner.openid 是管理员，绕过白名单；disabled 模式拒绝。
 */
import type { SlashCommandHandlerContext } from '@tencent-connect/qqbot-nodejs';
import type { Logger } from '../types.ts';

/** 鉴权配置投影（结构化，便于单测） */
export interface CommandAuthConfigLike {
  access: {
    c2cMode: 'open' | 'allowlist' | 'disabled';
    c2cAllow: readonly string[];
  };
  /** K27：主人 openid（allowlist 模式下管理员绕过白名单）；null/缺省 = 未设置 */
  owner?: { openid?: string } | null;
}

const DENIED = '⚠️ 无权限执行此命令';

/** 构造命令白名单鉴权回调 */
export function commandAllowlistAuth(
  config: CommandAuthConfigLike,
  logger?: Logger,
): (ctx: SlashCommandHandlerContext) => boolean | string {
  return (ctx) => {
    const mode = config.access.c2cMode;
    // K27：disabled 模式命令一律拒绝（不再依赖上游中间件兜底）。
    if (mode === 'disabled') return DENIED;
    if (mode !== 'allowlist') return true; // open 模式不变
    const senderId = ctx.message.senderId;
    // K27：owner 是管理员，allowlist 模式下绕过名单（仍要求发送者身份存在）。
    const ownerOpenid = config.owner?.openid?.trim();
    if (ownerOpenid !== undefined && ownerOpenid !== '' && senderId === ownerOpenid) {
      logger?.info?.(`im-qqbot: command auth allows owner: sender=${senderId}`);
      return true;
    }
    if (!senderId || !config.access.c2cAllow.includes(senderId)) {
      logger?.warn(`im-qqbot: command auth denied: sender=${senderId ?? '(missing)'}`);
      return DENIED;
    }
    return true;
  };
}
