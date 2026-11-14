/**
 * QQ 网关 READY 事件里的账号身份提取。
 *
 * 扫码/手动绑定只拿到 AppID/AppSecret，**账号昵称**要等网关连接成功后的
 * READY 负载（`user.username`）才有：本模块把该负载解析成稳定的账号信息，
 * 供运行时句柄（列表/绑定面板展示）使用。
 */

/** 机器人账号身份（网关 READY 前为空）。 */
export interface BotAccountInfo {
  /** 平台账号 id（READY.user.id） */
  id?: string;
  /** 账号昵称（READY.user.username） */
  name?: string;
}

/**
 * 从 READY 事件负载读取账号身份；字段缺失或类型不符时返回空对象。
 * @param data - QQ 网关 READY 事件数据
 * @returns 账号身份（不抛错：负载是外部数据）
 */
export function readAccountInfo(data: unknown): BotAccountInfo {
  const user = (data as { user?: { id?: unknown; username?: unknown } } | null | undefined)?.user;
  const info: BotAccountInfo = {};
  if (typeof user?.id === 'string' && user.id !== '') info.id = user.id;
  if (typeof user?.username === 'string' && user.username.trim() !== '') info.name = user.username.trim();
  return info;
}
