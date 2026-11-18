/**
 * 多机器人 bots[] 持久化 — profile patch 中 bots[] 的增删改与凭据写入。
 *
 * 复用 persist.ts 的 YAML 事务引擎（文档保真 / fail-closed / 原子写 / 乐观校验），
 * 本文件只承载「bots[] 条目」这组操作：凭据 upsert（明文或凭据库引用）、解绑、
 * 配置合并、新增与删除。
 */
import { resolve } from 'node:path';
import { isMap, isSeq, isScalar, YAMLMap, YAMLSeq } from 'yaml';
import {
  PatchFormatError,
  c2cAllowlistContains,
  editProfilePatch,
  overwriteC2cAllowlist,
  type PersistLogger,
} from './persist.ts';
import type { SetupCredentials } from './persist.ts';
import { DEFAULT_BOT_ID } from '../configBots.ts';

/** 读取 im-qqbot 条目 config 里 bots[] 中 botId 的映射（不存在返回 undefined，重复 id fail-closed）。 */
function findBotItem(config: YAMLMap, botId: string): YAMLMap | undefined {
  const node = config.get('bots', true);
  if (!isSeq(node)) return undefined;
  let found: YAMLMap | undefined;
  for (const item of node.items) {
    if (!isMap(item)) continue;
    const idNode = item.get('id', true);
    if (!isScalar(idNode) || String(idNode.value) !== botId) continue;
    if (found !== undefined) throw new PatchFormatError(`im-qqbot.bots[] 中 id 重复: ${botId}`);
    found = item;
  }
  return found;
}

/** 取/建 bots 序列；结构非法时 fail-closed。 */
function ensureBotsSeq(config: YAMLMap): YAMLSeq {
  const node = config.get('bots', true);
  if (node === undefined) {
    const seq = new YAMLSeq();
    seq.flow = false;
    config.set('bots', seq);
    return seq;
  }
  if (!isSeq(node)) throw new PatchFormatError('im-qqbot 的 bots 不是序列');
  return node;
}

/** 取/建 bots[] 中 botId 的映射（id 重复 fail-closed）。 */
function ensureBotItem(config: YAMLMap, botId: string): YAMLMap {
  const existing = findBotItem(config, botId);
  if (existing !== undefined) return existing;
  const seq = ensureBotsSeq(config);
  const item = new YAMLMap();
  item.set('id', botId);
  seq.add(item);
  return item;
}

/**
 * 把某个 bot 的凭据写入 profile patch 的 `bots[]`（按 botId upsert）。
 * @param botId - 配置里的稳定 bot id
 * @param credentials - 扫码/手动提交的凭据
 * @param profileDir - profile 目录
 * @param logger - 日志
 * @returns 是否写入成功
 */
// multi-bot 凭据持久化：只改 bots[] 中该 id 的 appId/appSecret（+ 扫码者白名单），其它内容原样保留。
export function persistBotCredentialsToProfile(
  botId: string,
  credentials: SetupCredentials,
  profileDir?: string,
  logger?: PersistLogger,
): boolean {
  const log = logger ?? console;
  if (!profileDir) {
    log.warn('未定位到 profile 目录（源码直载或非常规安装），机器人凭据未保存');
    return false;
  }
  try {
    const ok = editProfilePatch(
      profileDir,
      ({ config, attached, appendEntry }) => {
        const bot = ensureBotItem(config, botId);
        bot.set('appId', credentials.appId);
        bot.set('appSecret', credentials.appSecret);
        bot.set('enabled', true);
        if (credentials.userOpenid) overwriteC2cAllowlist(bot, credentials.userOpenid);
        if (!attached) appendEntry();
        return true;
      },
      {},
      (config) => config !== undefined && (() => {
        const bot = findBotItem(config, botId);
        return bot !== undefined
          && bot.get('appId') === credentials.appId
          && bot.get('appSecret') === credentials.appSecret
          && (!credentials.userOpenid || c2cAllowlistContains(bot, credentials.userOpenid));
      })(),
      log,
    );
    if (ok) log.info(`✔ 机器人 ${botId} 凭据已写入: ${resolve(profileDir, 'cordis.patch.yml')}`);
    return ok;
  } catch (err) {
    if (err instanceof PatchFormatError) {
      log.warn(`拒绝写入 cordis.patch.yml: ${err.message}（原文件保留）`);
      return false;
    }
    log.warn(`写入机器人凭据失败: ${err instanceof Error ? err.message : String(err)}`);
    return false;
  }
}

/**
 * 该 bot 凭据的引用形态：密钥已存凭据库，patch 里只留引用。
 */
export interface BotCredentialRef {
  /** QQ 机器人 AppID */
  appId: string;
  /** 凭据库引用名（环境变量名语法） */
  appSecretRef: string;
  /** 扫码用户 openid（存在时自动加入私聊白名单） */
  userOpenid?: string;
}

/**
 * 把某个 bot 的凭据引用写入 profile patch 的 `bots[]`（按 botId upsert）。
 *
 * 与 persistBotCredentialsToProfile 的区别：密钥不落 patch，只写
 * `appSecretRef` 并删除同条目的明文 `appSecret`，由下次 apply 从凭据库解析。
 * @param botId - 配置里的稳定 bot id
 * @param credentials - 扫码/手动提交的 AppID 与凭据库引用
 * @param profileDir - profile 目录
 * @param logger - 日志
 * @returns 是否写入成功
 */
// 凭据引用持久化：不写明文密钥，其它内容与注释原样保留。
export function persistBotCredentialRefToProfile(
  botId: string,
  credentials: BotCredentialRef,
  profileDir?: string,
  logger?: PersistLogger,
): boolean {
  const log = logger ?? console;
  if (!profileDir) {
    log.warn('未定位到 profile 目录（源码直载或非常规安装），机器人凭据引用未保存');
    return false;
  }
  try {
    const ok = editProfilePatch(
      profileDir,
      ({ config, attached, appendEntry }) => {
        const bot = ensureBotItem(config, botId);
        bot.set('appId', credentials.appId);
        bot.delete('appSecret');
        bot.set('appSecretRef', credentials.appSecretRef);
        bot.set('enabled', true);
        if (credentials.userOpenid) overwriteC2cAllowlist(bot, credentials.userOpenid);
        if (!attached) appendEntry();
        return true;
      },
      {},
      (config) => config !== undefined && (() => {
        const bot = findBotItem(config, botId);
        return bot !== undefined
          && bot.get('appId') === credentials.appId
          && bot.get('appSecretRef') === credentials.appSecretRef
          && bot.get('appSecret') === undefined
          && (!credentials.userOpenid || c2cAllowlistContains(bot, credentials.userOpenid));
      })(),
      log,
    );
    if (ok) log.info(`✔ 机器人 ${botId} 凭据引用已写入: ${resolve(profileDir, 'cordis.patch.yml')}`);
    return ok;
  } catch (err) {
    if (err instanceof PatchFormatError) {
      log.warn(`拒绝写入 cordis.patch.yml: ${err.message}（原文件保留）`);
      return false;
    }
    log.warn(`写入机器人凭据引用失败: ${err instanceof Error ? err.message : String(err)}`);
    return false;
  }
}

/**
 * 从 profile patch 的 `bots[]` 移除某 bot 的凭据并停用（解绑）。
 * @param botId - 配置里的稳定 bot id
 * @param profileDir - profile 目录
 * @param logger - 日志
 * @returns 是否写入成功
 */
// 解绑：删 appId/appSecret/appSecretRef、置 enabled=false；条目不存在也算成功（幂等），force 写回触发 HMR。
export function removeBotCredentialsFromProfile(
  botId: string,
  profileDir?: string,
  logger?: PersistLogger,
): boolean {
  const log = logger ?? console;
  if (!profileDir) return false;
  try {
    const ok = editProfilePatch(
      profileDir,
      ({ config }) => {
        const bot = findBotItem(config, botId);
        if (bot === undefined) return true;
        bot.delete('appId');
        bot.delete('appSecret');
        bot.delete('appSecretRef');
        bot.set('enabled', false);
        return true;
      },
      { force: true },
      (config) => config === undefined || (() => {
        const bot = findBotItem(config, botId);
        return bot === undefined
          || (bot.get('appId') === undefined
            && bot.get('appSecret') === undefined
            && bot.get('appSecretRef') === undefined);
      })(),
      log,
    );
    if (ok) {
      log.info(`✔ 机器人 ${botId} 凭据已移除: ${resolve(profileDir, 'cordis.patch.yml')}`);
    }
    return ok;
  } catch (err) {
    if (err instanceof PatchFormatError) {
      log.warn(`拒绝写入 cordis.patch.yml: ${err.message}（原文件保留）`);
      return false;
    }
    log.warn(`移除机器人凭据失败: ${err instanceof Error ? err.message : String(err)}`);
    return false;
  }
}

/**
 * 把配置更新合并进 profile patch 的 `bots[]` 中某 bot 的条目。
 * @param botId - 配置里的稳定 bot id
 * @param configPatch - 待合并的配置键值（appId/appSecret/id 会被忽略）
 * @param profileDir - profile 目录
 * @param logger - 日志
 * @returns 是否写入成功
 */
// 最小写入：只合并白名单顶层键，不固化 bundle 默认值；嵌套对象整体替换。
export function updateBotProfileConfig(
  botId: string,
  configPatch: Record<string, unknown>,
  profileDir?: string,
  logger?: PersistLogger,
): boolean {
  const log = logger ?? console;
  if (!profileDir) {
    log.warn('未定位到 profile 目录，机器人配置未保存');
    return false;
  }
  try {
    const ok = editProfilePatch(
      profileDir,
      ({ config, attached, appendEntry }) => {
        const bot = ensureBotItem(config, botId);
        for (const [key, value] of Object.entries(configPatch)) {
          if (key === 'appId' || key === 'appSecret' || key === 'appSecretRef' || key === 'id') continue;
          bot.set(key, value);
        }
        if (!attached) appendEntry();
        return true;
      },
      {},
      (config) => config !== undefined && findBotItem(config, botId) !== undefined,
      log,
    );
    if (ok) log.info(`✔ 机器人 ${botId} 配置已保存: ${resolve(profileDir, 'cordis.patch.yml')}`);
    return ok;
  } catch (err) {
    if (err instanceof PatchFormatError) {
      log.warn(`拒绝写入 cordis.patch.yml: ${err.message}（原文件保留）`);
      return false;
    }
    log.warn(`保存机器人配置失败: ${err instanceof Error ? err.message : String(err)}`);
    return false;
  }
}

/**
 * 在 bots[] 中新增一个空凭据机器人（enabled=false，等待手动绑定/配置）。
 * @param botId - 新 bot id（需匹配 configBots 的 id 规范）
 * @param label - 展示名（空 = 用 id）
 * @param profileDir - profile 目录
 * @param logger - 日志
 * @returns 是否写入（已存在时返回 false）
 */
// 新增机器人：只写 id/label/空凭据/enabled=false，凭据由后续手动绑定或配置补全。
export function addBotToProfile(
  botId: string,
  label: string | undefined,
  profileDir?: string,
  logger?: PersistLogger,
): boolean {
  const log = logger ?? console;
  if (!profileDir) {
    log.warn('未定位到 profile 目录，机器人未新增');
    return false;
  }
  try {
    // editProfilePatch 对「无需变更」也返回 true，因此单独记录本次是否真的新增。
    let created = false;
    const ok = editProfilePatch(
      profileDir,
      ({ config, attached, appendEntry }) => {
        created = false;
        if (findBotItem(config, botId) !== undefined) return false;
        const seq = ensureBotsSeq(config);
        const item = new YAMLMap();
        item.set('id', botId);
        if (label !== undefined && label !== '') item.set('label', label);
        item.set('appId', '');
        item.set('appSecret', '');
        item.set('enabled', false);
        seq.add(item);
        if (!attached) appendEntry();
        created = true;
        return true;
      },
      {},
      (config) => config !== undefined && findBotItem(config, botId) !== undefined,
      log,
    );
    if (ok && created) log.info(`✔ 机器人 ${botId} 已新增，请在该 bot 页填写凭据`);
    if (!created) log.warn(`机器人 ${botId} 已存在，未新增`);
    return ok && created;
  } catch (err) {
    if (err instanceof PatchFormatError) {
      log.warn(`拒绝写入 cordis.patch.yml: ${err.message}（原文件保留）`);
      return false;
    }
    log.warn(`新增机器人失败: ${err instanceof Error ? err.message : String(err)}`);
    return false;
  }
}

/**
 * 新增机器人（单次写入）：bots[] 为空时先把旧单 bot 迁移成 `default` 项，
 * 再追加新机器人。避免两次写 patch 触发两次 HMR 重载、请求中途断流。
 */
export function addBotToProfileMigratingLegacy(
  botId: string,
  label: string | undefined,
  legacy: { appId: string; appSecret: string } | undefined,
  profileDir?: string,
  logger?: PersistLogger,
): boolean {
  const log = logger ?? console;
  if (!profileDir) {
    log.warn('未定位到 profile 目录，机器人未新增');
    return false;
  }
  try {
    let created = false;
    const ok = editProfilePatch(
      profileDir,
      ({ config, attached, appendEntry }) => {
        created = false;
        const node = config.get('bots', true);
        const seq = isSeq(node) ? node : ensureBotsSeq(config);
        if (legacy !== undefined && seq.items.length === 0) {
          const legacyItem = new YAMLMap();
          legacyItem.set('id', DEFAULT_BOT_ID);
          legacyItem.set('label', '默认机器人');
          legacyItem.set('enabled', true);
          legacyItem.set('appId', legacy.appId);
          legacyItem.set('appSecret', legacy.appSecret);
          seq.add(legacyItem);
        }
        if (findBotItem(config, botId) !== undefined) return seq.items.length > 0;
        const item = new YAMLMap();
        item.set('id', botId);
        if (label !== undefined && label !== '') item.set('label', label);
        item.set('appId', '');
        item.set('appSecret', '');
        item.set('enabled', false);
        seq.add(item);
        if (!attached) appendEntry();
        created = true;
        return true;
      },
      {},
      (config) => config !== undefined && findBotItem(config, botId) !== undefined,
      log,
    );
    if (ok && created) log.info(`✔ 机器人 ${botId} 已新增，请在该 bot 页填写凭据`);
    if (!created) log.warn(`机器人 ${botId} 已存在，未新增`);
    return ok && created;
  } catch (err) {
    if (err instanceof PatchFormatError) {
      log.warn(`拒绝写入 cordis.patch.yml: ${err.message}（原文件保留）`);
      return false;
    }
    log.warn(`新增机器人失败: ${err instanceof Error ? err.message : String(err)}`);
    return false;
  }
}

/**
 * 把旧版顶层单 bot 迁移成 bots[] 的第一项（id=`default`）。
 *
 * 只在 bots[] 缺失/为空时写入；顶层字段（provider/model/cwd/entity/access 等）
 * 继续作为 base 被各 bot 继承，因此这里只写凭据与身份字段即可保持行为不变。
 */
export function addLegacyBotToProfile(
  appId: string,
  appSecret: string,
  profileDir?: string,
  logger?: PersistLogger,
): boolean {
  const log = logger ?? console;
  if (!profileDir) {
    log.warn('未定位到 profile 目录，旧单 bot 未迁移');
    return false;
  }
  try {
    let migrated = false;
    const ok = editProfilePatch(
      profileDir,
      ({ config, attached, appendEntry }) => {
        const node = config.get('bots', true);
        // 已有 bots[]：说明已经迁移过（或用户手写过），不重复写。
        if (isSeq(node) && node.items.length > 0) return false;
        const seq = ensureBotsSeq(config);
        const item = new YAMLMap();
        item.set('id', DEFAULT_BOT_ID);
        item.set('label', '默认机器人');
        item.set('enabled', true);
        item.set('appId', appId);
        item.set('appSecret', appSecret);
        seq.add(item);
        if (!attached) appendEntry();
        migrated = true;
        return true;
      },
      {},
      (config) => {
        if (config === undefined) return false;
        const node = config.get('bots', true);
        return isSeq(node) && node.items.length > 0;
      },
      log,
    );
    if (ok && migrated) log.info(`✔ 旧单 bot 已迁移进 bots[]（id=${DEFAULT_BOT_ID}），可继续新增机器人`);
    return ok && migrated;
  } catch (err) {
    if (err instanceof PatchFormatError) {
      log.warn(`拒绝写入 cordis.patch.yml: ${err.message}（原文件保留）`);
      return false;
    }
    log.warn(`迁移旧单 bot 失败: ${err instanceof Error ? err.message : String(err)}`);
    return false;
  }
}

/**
 * 从 bots[] 删除一个机器人条目（含凭据）。
 * @param botId - 要删除的 bot id
 * @param profileDir - profile 目录
 * @param logger - 日志
 * @returns 是否写入（不存在时返回 false）
 */
// 删除机器人：整项移除；force 写回触发 HMR 让运行时下线该 bot。
export function removeBotFromProfile(
  botId: string,
  profileDir?: string,
  logger?: PersistLogger,
): boolean {
  const log = logger ?? console;
  if (!profileDir) return false;
  try {
    const ok = editProfilePatch(
      profileDir,
      ({ config }) => {
        const node = config.get('bots', true);
        if (!isSeq(node)) return false;
        const index = node.items.findIndex((item) => {
          if (!isMap(item)) return false;
          const idNode = item.get('id', true);
          return isScalar(idNode) && String(idNode.value) === botId;
        });
        if (index < 0) return false;
        node.items.splice(index, 1);
        // 删掉最后一个 bot 时同步清掉顶层凭据：否则 normalizeBots 会回落成 legacy 单 bot，
        // 让刚删除的机器人（以及顶层 appId）在下次加载/迁移时复活。
        if (node.items.length === 0) {
          config.delete('appId');
          config.delete('appSecret');
        }
        return true;
      },
      { force: true },
      (config) => config === undefined || !findBotItem(config, botId),
      log,
    );
    if (ok) log.info(`✔ 机器人 ${botId} 已删除: ${resolve(profileDir, 'cordis.patch.yml')}`);
    return ok;
  } catch (err) {
    if (err instanceof PatchFormatError) {
      log.warn(`拒绝写入 cordis.patch.yml: ${err.message}（原文件保留）`);
      return false;
    }
    log.warn(`删除机器人失败: ${err instanceof Error ? err.message : String(err)}`);
    return false;
  }
}
