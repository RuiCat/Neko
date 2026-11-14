/**
 * 凭据库适配测试：引用命名、明文优先、引用解析（含 env 兜底）、写库与清理。
 */
import { describe, expect, it, vi } from 'vitest';
import {
  botSecretRef,
  credentialsFrom,
  isSecretRef,
  removeBotSecret,
  resolveConfigSecrets,
  storeBotSecret,
  type CredentialsLike,
} from './credentialStore.ts';
import type { BotEntryConfig } from '../config.ts';

/** 可观测的凭据库替身 */
function fakeStore(values: Record<string, string> = {}): {
  store: CredentialsLike;
  values: Map<string, string>;
  resolve: ReturnType<typeof vi.fn>;
  set: ReturnType<typeof vi.fn>;
  unset: ReturnType<typeof vi.fn>;
} {
  const map = new Map(Object.entries(values));
  const resolve = vi.fn(async (ref: string) => (map.has(ref) ? { value: map.get(ref) as string } : undefined));
  const set = vi.fn(async (ref: string, value: string) => { map.set(ref, value); });
  const unset = vi.fn(async (ref: string) => { map.delete(ref); });
  return { store: { resolve, set, unset }, values: map, resolve, set, unset };
}

/** 一条 bots[] 配置（默认值可按需覆盖） */
function botEntry(overrides: Partial<BotEntryConfig> = {}): BotEntryConfig {
  return { id: 'cat', enabled: true, appId: 'app-1', appSecret: '', ...overrides };
}

describe('botSecretRef / isSecretRef', () => {
  it('默认引用名为 QQBOT_<BOT_ID>_SECRET，连字符转下划线', () => {
    expect(botSecretRef('cat')).toBe('QQBOT_CAT_SECRET');
    expect(botSecretRef('my-bot')).toBe('QQBOT_MY_BOT_SECRET');
    expect(isSecretRef(botSecretRef('cat'))).toBe(true);
  });

  it('只接受环境变量名语法', () => {
    expect(isSecretRef('QQBOT_X_SECRET')).toBe(true);
    expect(isSecretRef('_X')).toBe(true);
    expect(isSecretRef('1BAD')).toBe(false);
    expect(isSecretRef('has-dash')).toBe(false);
    expect(isSecretRef('')).toBe(false);
  });
});

describe('resolveConfigSecrets', () => {
  it('明文 appSecret 优先，不访问凭据库', async () => {
    const fake = fakeStore();
    const result = await resolveConfigSecrets(fake.store, [
      botEntry({ appSecret: 'plain-secret', appSecretRef: 'QQBOT_CAT_SECRET' }),
    ]);
    expect(result.bots[0]?.appSecret).toBe('plain-secret');
    expect(result.warnings).toEqual([]);
    expect(fake.resolve).not.toHaveBeenCalled();
  });

  it('appSecret 为空时按 appSecretRef 从凭据库解析', async () => {
    const fake = fakeStore({ QQBOT_CAT_SECRET: 'from-store' });
    const result = await resolveConfigSecrets(fake.store, [
      botEntry({ appSecret: '', appSecretRef: 'QQBOT_CAT_SECRET' }),
    ]);
    expect(result.bots[0]?.appSecret).toBe('from-store');
    expect(fake.resolve).toHaveBeenCalledWith('QQBOT_CAT_SECRET');
  });

  it('引用未解析到密钥时告警并保留原条目', async () => {
    const fake = fakeStore();
    const result = await resolveConfigSecrets(fake.store, [
      botEntry({ appSecret: '', appSecretRef: 'QQBOT_MISSING_SECRET' }),
    ]);
    expect(result.bots[0]?.appSecret).toBe('');
    expect(result.warnings.join('\n')).toContain('QQBOT_MISSING_SECRET');
  });

  it('非法引用名告警且不访问凭据库', async () => {
    const fake = fakeStore();
    const result = await resolveConfigSecrets(fake.store, [
      botEntry({ appSecret: '', appSecretRef: 'bad-ref' }),
    ]);
    expect(result.warnings.join('\n')).toContain('非法');
    expect(fake.resolve).not.toHaveBeenCalled();
  });

  it('无凭据库时回退进程环境变量', async () => {
    process.env.QQBOT_ENV_FALLBACK_SECRET = 'env-secret';
    try {
      const result = await resolveConfigSecrets(undefined, [
        botEntry({ appSecret: '', appSecretRef: 'QQBOT_ENV_FALLBACK_SECRET' }),
      ]);
      expect(result.bots[0]?.appSecret).toBe('env-secret');
    } finally {
      delete process.env.QQBOT_ENV_FALLBACK_SECRET;
    }
  });

  it('没有 appSecretRef 的条目原样返回', async () => {
    const entry = botEntry({ appSecret: 'only-plain' });
    const result = await resolveConfigSecrets(fakeStore().store, [entry]);
    expect(result.bots[0]).toBe(entry);
  });
});

describe('storeBotSecret / removeBotSecret', () => {
  it('写库成功返回 true 并调用 set', async () => {
    const fake = fakeStore();
    expect(await storeBotSecret(fake.store, 'QQBOT_CAT_SECRET', 's3cret')).toBe(true);
    expect(fake.set).toHaveBeenCalledWith('QQBOT_CAT_SECRET', 's3cret');
    expect(fake.values.get('QQBOT_CAT_SECRET')).toBe('s3cret');
  });

  it('无凭据库 / 空密钥 / 非法引用时返回 false', async () => {
    expect(await storeBotSecret(undefined, 'QQBOT_CAT_SECRET', 's3cret')).toBe(false);
    expect(await storeBotSecret(fakeStore().store, 'QQBOT_CAT_SECRET', '')).toBe(false);
    expect(await storeBotSecret(fakeStore().store, 'bad-ref', 's3cret')).toBe(false);
  });

  it('清理引用调用 unset 并从库里移除', async () => {
    const fake = fakeStore({ QQBOT_CAT_SECRET: 's3cret' });
    expect(await removeBotSecret(fake.store, 'QQBOT_CAT_SECRET')).toBe(true);
    expect(fake.unset).toHaveBeenCalledWith('QQBOT_CAT_SECRET');
    expect(fake.values.has('QQBOT_CAT_SECRET')).toBe(false);
  });

  it('无凭据库时清理返回 false', async () => {
    expect(await removeBotSecret(undefined, 'QQBOT_CAT_SECRET')).toBe(false);
  });
});

describe('credentialsFrom', () => {
  it('无 credentials 服务时返回 undefined', () => {
    expect(credentialsFrom({})).toBeUndefined();
    expect(credentialsFrom({ credentials: {} })).toBeUndefined();
  });

  it('形状完整时返回服务', () => {
    const fake = fakeStore();
    expect(credentialsFrom({ credentials: fake.store })).toBe(fake.store);
  });
});
