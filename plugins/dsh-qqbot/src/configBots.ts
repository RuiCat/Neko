/**
 * 多机器人配置归一化（P1 配置层）。
 *
 * 把两种输入统一成同一份「生效机器人」列表：
 * - 旧配置：顶层单 bot 字段（appId/appSecret/...），可回退环境变量；
 * - 新配置：`bots[]` 数组，每项独立凭据并可覆盖 provider/model/preset/cwd/prompt/access。
 *
 * 纯数据变换 + 业务校验（id/appId 唯一、格式、缺凭据降级），不做运行时装配，
 * 也不读写文件，便于单测与后续 BotRuntime 复用。
 */
import type { BotEntryConfig, ImQQBotConfig } from './config.ts';
import { normalizeConversationPrompts } from './shared/conversationPrompts.ts';
import { normalizeOwner } from './shared/owner.ts';

/** bot id 规范：小写字母/数字/连字符，2-32 位，首字符为字母或数字。 */
export const BOT_ID_PATTERN = /^[a-z0-9][a-z0-9-]{1,31}$/;

/** 顶层单 bot 配置迁移后使用的 id。 */
export const DEFAULT_BOT_ID = 'default';

/**
 * 归一化 NPC 实体 id：trim 后为空视为「未绑定」。
 * 实体 id 允许中文及其它非 ASCII 字符，因此不套用 `BOT_ID_PATTERN`。
 * @param value - 配置里的原始实体 id
 * @returns trim 后的实体 id；未配置/空白时 undefined
 */
export function normalizeEntityId(value: string | undefined): string | undefined {
  const trimmed = value?.trim();
  return trimmed === undefined || trimmed === '' ? undefined : trimmed;
}

/**
 * 识别 bundle patch 的凭据占位符（与 shared/resolveEnv 的语义对齐）：
 * 空串 / `__FROM_ENV__` / `process.env…` 都视为「未配置，需回退环境变量」。
 * @param value - 配置里的原始凭据值
 * @returns 是否应回退环境变量
 */
export function isEnvPlaceholder(value: string): boolean {
  return value === '' || value === '__FROM_ENV__' || value.startsWith('process.env');
}


/** 一个可启动机器人的生效配置。 */
export interface EffectiveBot {
  /** 稳定标识（bots[] 的 id；旧配置为 DEFAULT_BOT_ID） */
  id: string;
  /** 展示名（缺省取 id） */
  label: string;
  /** 是否应启动（false = 配置禁用或缺凭据） */
  enabled: boolean;
  /** true = 由旧版顶层单 bot 配置迁移而来（媒体目录等保持历史路径） */
  legacy: boolean;
  /** 凭据库引用（bots[] 条目声明了 appSecretRef 时存在；用于解绑清理） */
  appSecretRef?: string;
  /** 合并后的完整单 bot 配置 */
  config: ImQQBotConfig;
}

/** 归一化选项：只用于旧配置的环境变量回退。 */
export interface NormalizeBotsOptions {
  /** QQBOT_APPID 解析结果 */
  envAppId?: string;
  /** QQBOT_SECRET 解析结果 */
  envAppSecret?: string;
}

/** 归一化结果。 */
export interface NormalizedBots {
  /** 全部机器人（含被停用的项，便于状态展示） */
  bots: EffectiveBot[];
  /** 非致命问题（缺凭据被跳过、禁用、忽略环境变量等） */
  warnings: string[];
  /** true = 输入是旧版顶层单 bot 配置（已迁移为 DEFAULT_BOT_ID） */
  legacy: boolean;
}


/**
 * 把 `分类:名称` 形式的绑定值拆成实体 id 与分类路径。
 * 实体名称全局唯一，分类只在「实体不存在时按该类型创建」时使用；
 * 纯名称（无冒号）只表示 id，不产生类型。
 * @param raw - 配置里的 entity 原始值
 * @returns entity=末段名称；entityType=前面的分类路径（无冒号时 undefined）
 */
export function splitEntityRef(raw: string | undefined): { entity?: string; entityType?: string } {
  const text = raw?.trim();
  if (text === undefined || text === '') return {};
  const cut = text.lastIndexOf(':');
  if (cut <= 0 || cut === text.length - 1) return { entity: text };
  const entity = text.slice(cut + 1).trim();
  const entityType = text.slice(0, cut).trim();
  if (entity === '' || entityType === '') return {};
  return { entity, entityType };
}

/**
 * 把 bots[] 的一项合并到顶层基础配置上。
 * 标量字段按需覆盖；access 是整体替换（提供时需写全，避免半份白名单）。
 * @param base - 顶层配置（已含 schema 默认值）
 * @param bot - bots[] 的一项
 * @returns 合并后的单 bot 配置
 */
function mergeBotConfig(base: ImQQBotConfig, bot: BotEntryConfig): ImQQBotConfig {
  const merged: ImQQBotConfig = { ...base, appId: bot.appId, appSecret: bot.appSecret };
  if (bot.provider !== undefined) merged.provider = bot.provider;
  if (bot.model !== undefined) merged.model = bot.model;
  if (bot.preset !== undefined) merged.preset = bot.preset;
  if (bot.cwd !== undefined) merged.cwd = bot.cwd;
  // BotEntrySchema 会把缺省 entity 填成空串，因此空白条目视为「未设置」并继承顶层；
  // 绑定值可写成 `分类:名称`：id 取末段，分类留给「不存在时按该类型创建」。
  const botRef = splitEntityRef(bot.entity);
  const baseRef = splitEntityRef(base.entity);
  const ref = botRef.entity !== undefined ? botRef : baseRef;
  merged.entity = normalizeEntityId(ref.entity);
  merged.entityType = normalizeEntityId(ref.entityType);
  if (bot.groupPrompt !== undefined) merged.groupPrompt = bot.groupPrompt;
  if (bot.directPrompt !== undefined) merged.directPrompt = bot.directPrompt;
  // K26：单会话提示词整体覆盖（条目提供时），缺省继承顶层已归一化列表。
  if (bot.conversationPrompts !== undefined) merged.conversationPrompts = bot.conversationPrompts;
  // K27：owner 整体覆盖（null = 显式清空），缺省继承顶层已归一化值。
  if (bot.owner !== undefined) merged.owner = bot.owner === null ? undefined : bot.owner;
  if (bot.requireMention !== undefined) merged.requireMention = bot.requireMention;
  if (bot.mainSessionKickoff !== undefined) merged.mainSessionKickoff = bot.mainSessionKickoff;
  if (bot.access !== undefined) merged.access = bot.access;
  return merged;
}

/**
 * 旧版顶层单 bot 配置 → 一个生效机器人（保留环境变量回退语义）。
 * @param base - 顶层配置
 * @param options - 环境变量值
 * @returns 生效机器人
 */
function legacyBot(base: ImQQBotConfig, options: NormalizeBotsOptions): EffectiveBot {
  return {
    id: DEFAULT_BOT_ID,
    label: '默认机器人',
    enabled: true,
    legacy: true,
    config: {
      ...base,
      appId: isEnvPlaceholder(base.appId) ? (options.envAppId ?? '') : base.appId,
      appSecret: isEnvPlaceholder(base.appSecret) ? (options.envAppSecret ?? '') : base.appSecret,
      // Schema 默认值把未配置的顶层 entity 填成空串：旧单 bot 同样归一为「未绑定」。
      entity: normalizeEntityId(splitEntityRef(base.entity).entity),
      entityType: normalizeEntityId(splitEntityRef(base.entity).entityType),
    },
  };
}

/**
 * 归一化配置。重复 id/appId 与非法 id 抛错（配置错误应尽早失败）；
 * 缺凭据只停用该 bot 并记 warning（不拖垮其它机器人）。
 * @param config - 插件配置（ConfigSchema 已填默认值）
 * @param options - 旧配置的环境变量回退值
 * @returns 生效机器人列表与告警
 * @throws 当 bots[] 中出现重复 id、重复 appId 或非法 id
 */
// 归一化入口（纯函数）：配置错误抛错，可恢复问题记 warning。
export function normalizeBots(config: ImQQBotConfig, options: NormalizeBotsOptions = {}): NormalizedBots {
  // K26：会话级提示词在这里 fail-fast（Schema 只校验形状与 scope 枚举；空值/超长/重复在此报错）。
  const base: ImQQBotConfig = {
    ...config,
    conversationPrompts: normalizeConversationPrompts(config.conversationPrompts, 'conversationPrompts'),
    // K27：owner 顶层归一化（openid 非空、name trim/默认「主人」/≤64；null = 未设置）。
    owner: normalizeOwner(config.owner, 'owner'),
  };
  const entries = Array.isArray(config.bots) ? config.bots : [];
  if (entries.length === 0) {
    return { bots: [legacyBot(base, options)], warnings: [], legacy: true };
  }

  const warnings: string[] = [];
  if (options.envAppId || options.envAppSecret) {
    warnings.push('多机器人模式下忽略 QQBOT_APPID/QQBOT_SECRET 环境变量，请在 bots[] 内逐项配置凭据');
  }
  const seenIds = new Set<string>();
  const seenAppIds = new Set<string>();
  const bots: EffectiveBot[] = [];

  entries.forEach((rawEntry, index) => {
    // entry 提供的覆盖字段先归一化（fail-fast），缺省则继承顶层。
    let entry: BotEntryConfig = rawEntry;
    if (rawEntry.conversationPrompts !== undefined) {
      entry = {
        ...entry,
        conversationPrompts: normalizeConversationPrompts(rawEntry.conversationPrompts, `bots[${index}].conversationPrompts`),
      };
    }
    if (rawEntry.owner !== undefined) {
      // null = 显式不设置（覆盖后不让顶层 owner 继承下来）。
      entry = { ...entry, owner: normalizeOwner(rawEntry.owner, `bots[${index}].owner`) ?? null };
    }
    const id = entry.id.trim();
    if (!BOT_ID_PATTERN.test(id)) {
      throw new Error(`bots[${index}].id 非法（${JSON.stringify(entry.id)}），需匹配 ${String(BOT_ID_PATTERN)}`);
    }
    if (seenIds.has(id)) {
      throw new Error(`bots[] 中 id 重复: ${id}`);
    }
    seenIds.add(id);

    const appId = entry.appId.trim();
    const appSecret = entry.appSecret.trim();
    if (isEnvPlaceholder(appId) || isEnvPlaceholder(appSecret)) {
      warnings.push(`机器人 ${id} 缺少 appId/appSecret（占位符不算配置），本次跳过`);
      bots.push({ id, label: entry.label?.trim() || id, enabled: false, legacy: false, appSecretRef: entry.appSecretRef, config: mergeBotConfig(base, entry) });
      return;
    }
    if (seenAppIds.has(appId)) {
      throw new Error(`bots[] 中 appId 重复: ${id}`);
    }
    seenAppIds.add(appId);

    const enabled = entry.enabled !== false;
    if (!enabled) warnings.push(`机器人 ${id} 已禁用`);
    bots.push({ id, label: entry.label?.trim() || id, enabled, legacy: false, appSecretRef: entry.appSecretRef, config: mergeBotConfig(base, entry) });
  });

  if (!bots.some(bot => bot.enabled)) {
    warnings.push('没有可启动的机器人：检查 bots[].enabled 与 appId');
  }
  return { bots, warnings, legacy: false };
}
