/**
 * 机器人密钥的凭据库存储 — ctx.credentials 适配。
 *
 * 多机器人默认把 appSecret 写在 profile patch（明文）。credentials 是
 * dsh-base 的必备 seam（插件静态 inject 声明），新绑定的密钥改存凭据库
 * （`$DSH_HOME/.credentials.yaml`，仅属主可读），配置里只留 `appSecretRef`
 * 引用；读取按「明文优先、引用兜底」兼容旧配置，形状不符（单测）时回退明文路径。
 */
import type { BotEntryConfig } from '../config.ts';
import { isEnvPlaceholder } from '../configBots.ts';

/** ctx.credentials 的局部投影（不引入 @deepseek-ai/dsh-credentials 依赖）。 */
export interface CredentialsLike {
  /** 解析一个引用；未配置时返回 undefined */
  resolve(ref: string): Promise<{ value: string } | undefined>;
  /** 把值写入提供方管理的可写层 */
  set(ref: string, value: string): Promise<void>;
  /** 删除引用（不存在时为空操作） */
  unset(ref: string): Promise<void>;
}

/** 环境变量名语法：凭据库引用与进程环境变量共用。 */
const SECRET_REF_PATTERN = /^[A-Za-z_][A-Za-z0-9_]*$/;

/** bots[] 密钥解析结果。 */
export interface ResolvedConfigSecrets {
  /** 已按引用补全 appSecret 的 bots[]（无需解析的条目原样返回） */
  bots: BotEntryConfig[];
  /** 引用非法 / 未解析到密钥等非致命问题 */
  warnings: string[];
}

/**
 * 该 bot 默认的凭据库引用名：`QQBOT_<BOT_ID>_SECRET`（bot id 连字符转下划线）。
 * @param botId - 配置里的稳定 bot id
 * @returns 环境变量名语法的引用名
 */
export function botSecretRef(botId: string): string {
  const normalized = botId.toUpperCase().replace(/[^A-Z0-9]+/g, '_');
  return `QQBOT_${normalized}_SECRET`;
}

/**
 * 是否是可用的凭据库引用名。
 * @param value - 待校验的引用
 * @returns true = 匹配环境变量名语法
 */
export function isSecretRef(value: string): boolean {
  return SECRET_REF_PATTERN.test(value);
}

/**
 * 从插件上下文取 credentials 服务（未安装或形状不符时返回 undefined）。
 * @param ctx - 插件上下文
 * @returns 可用的凭据库适配层
 */
export function credentialsFrom(ctx: unknown): CredentialsLike | undefined {
  const candidate = (ctx as { credentials?: unknown }).credentials;
  if (candidate === undefined || candidate === null) return undefined;
  const service = candidate as Partial<CredentialsLike>;
  if (typeof service.resolve !== 'function' || typeof service.set !== 'function' || typeof service.unset !== 'function') {
    return undefined;
  }
  return service as CredentialsLike;
}

/**
 * 把 bots[] 里只写了 `appSecretRef` 的条目补全为明文（供归一化层判断可用性）。
 * @param credentials - 凭据库适配层；undefined 时仅回退进程环境变量
 * @param entries - 原始 bots[] 配置
 * @returns 补全后的 bots[] 与告警
 */
export async function resolveConfigSecrets(
  credentials: CredentialsLike | undefined,
  entries: readonly BotEntryConfig[],
): Promise<ResolvedConfigSecrets> {
  const warnings: string[] = [];
  const bots: BotEntryConfig[] = [];
  for (const entry of entries) {
    if (!needsReference(entry)) {
      bots.push(entry);
      continue;
    }
    const ref = entry.appSecretRef ?? '';
    if (!isSecretRef(ref)) {
      warnings.push(`机器人 ${entry.id} 的 appSecretRef 非法（${JSON.stringify(ref)}），需匹配环境变量名`);
      bots.push(entry);
      continue;
    }
    const secret = await readReference(credentials, ref);
    if (secret === undefined) {
      warnings.push(`机器人 ${entry.id} 的凭据引用 ${ref} 未解析到密钥，本次跳过`);
      bots.push(entry);
      continue;
    }
    bots.push({ ...entry, appSecret: secret });
  }
  return { bots, warnings };
}

/** 把 bot 密钥写进凭据库；无 seam / 引用非法 / 写入失败返回 false。 */
export async function storeBotSecret(
  credentials: CredentialsLike | undefined,
  ref: string,
  secret: string,
): Promise<boolean> {
  if (credentials === undefined || !isSecretRef(ref) || secret === '') return false;
  try {
    await credentials.set(ref, secret);
    return true;
  } catch {
    return false;
  }
}

/**
 * 删除凭据库里的引用。
 * @param credentials - 凭据库适配层
 * @param ref - 引用名
 * @returns true = 已删除（或本来就不存在）；false = 无 seam / 引用非法 / 删除失败
 */
export async function removeBotSecret(
  credentials: CredentialsLike | undefined,
  ref: string,
): Promise<boolean> {
  if (credentials === undefined || !isSecretRef(ref)) return false;
  try {
    await credentials.unset(ref);
    return true;
  } catch {
    return false;
  }
}

/** 该条目是否需要从凭据库解析 appSecret（明文为空/占位符且有引用）。 */
function needsReference(entry: BotEntryConfig): boolean {
  const ref = entry.appSecretRef;
  if (ref === undefined || ref === '') return false;
  return isEnvPlaceholder(entry.appSecret);
}

/** 解析一个引用：凭据库优先，进程环境变量兜底（无 seam 时）。 */
async function readReference(
  credentials: CredentialsLike | undefined,
  ref: string,
): Promise<string | undefined> {
  if (credentials !== undefined) {
    try {
      const resolved = await credentials.resolve(ref);
      if (resolved !== undefined && resolved.value !== '') return resolved.value;
    } catch {
      // 凭据库异常：退回进程环境变量，保持可启动
    }
  }
  const fallback = process.env[ref];
  return fallback !== undefined && fallback !== '' ? fallback : undefined;
}
