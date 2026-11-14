/**
 * 主人/管理员身份（K27 owner）— 配置类型与归一化校验。
 *
 * 纯函数 + 无 node 依赖：宿主（ConfigSchema 归一化层、bind-routes 白名单校验）
 * 与浏览器设置页共用同一套归一化规则，避免两端口径漂移。
 */

/** 主人称呼上限（字符数）。 */
export const OWNER_NAME_MAX_CHARS = 64;

/** 未填写称呼时的默认值。 */
export const DEFAULT_OWNER_NAME = '主人';

/** 归一化后的 owner 配置（openid 非空、name 已 trim 且非空）。 */
export interface OwnerConfig {
  /** 主人的 QQ openid（私聊发送者 openid；群聊 @ 发送者 openid 同值可比对） */
  openid: string;
  /** 对主人的称呼（默认「主人」） */
  name: string;
}

/** 归一化并校验 owner 配置（`undefined`/`null` = 未设置）。 */
export function normalizeOwner(input: unknown, label = 'owner'): OwnerConfig | undefined {
  if (input === undefined || input === null) return undefined;
  if (typeof input !== 'object' || Array.isArray(input)) {
    throw new TypeError(`${label} 必须是对象`);
  }
  const record = input as Record<string, unknown>;
  const openid = typeof record.openid === 'string' ? record.openid.trim() : '';
  if (openid === '') {
    throw new TypeError(`${label}.openid 不能为空（不设置主人请删除 owner 配置或保存 null）`);
  }
  const rawName = record.name;
  if (rawName !== undefined && rawName !== null && typeof rawName !== 'string') {
    throw new TypeError(`${label}.name 必须是字符串`);
  }
  const name = (typeof rawName === 'string' ? rawName : '').trim() || DEFAULT_OWNER_NAME;
  if (name.length > OWNER_NAME_MAX_CHARS) {
    throw new TypeError(`${label}.name 超长（${name.length} > ${OWNER_NAME_MAX_CHARS} 字符）`);
  }
  return { openid, name };
}
