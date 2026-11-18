/**
 * 多机器人凭据/配置持久化测试：bots[] upsert、解绑、配置合并、fail-closed。
 */
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { parse } from 'yaml';
import { describe, expect, it, vi } from 'vitest';
import {
  addBotToProfile,
  addBotToProfileMigratingLegacy,
  addLegacyBotToProfile,
  persistBotCredentialRefToProfile,
  persistBotCredentialsToProfile,
  removeBotCredentialsFromProfile,
  removeBotFromProfile,
  updateBotProfileConfig,
} from './persistBots.ts';

function createLogger() {
  return { info: vi.fn(), warn: vi.fn() };
}

function makeProfile(patch: string): { dir: string; cleanup: () => void } {
  const dir = mkdtempSync(join(tmpdir(), 'im-qqbot-bots-'));
  writeFileSync(join(dir, 'cordis.patch.yml'), patch, 'utf8');
  return { dir, cleanup: () => rmSync(dir, { recursive: true, force: true }) };
}

interface Row { id?: string; config?: Record<string, unknown> }
function rows(dir: string): Row[] {
  const parsed: unknown = parse(readFileSync(join(dir, 'cordis.patch.yml'), 'utf8'));
  return Array.isArray(parsed) ? parsed as Row[] : [];
}
function botEntry(dir: string, botId: string): Record<string, unknown> | undefined {
  const row = rows(dir).find(item => item.id === 'im-qqbot');
  const bots = row?.config?.bots;
  if (!Array.isArray(bots)) return undefined;
  return (bots as Array<Record<string, unknown>>).find(item => item.id === botId);
}

const BASE = `# keep this comment
- insert:
    - id: other-plugin
      name: other
      config:
        foo: bar
- id: im-qqbot
  config:
    requireMention: true
    bots:
      - id: cat
        appId: old-cat
        appSecret: old-secret
`;

describe('persistBotCredentialsToProfile', () => {
  it('upsert 已有 bot：写凭据 + 扫码者白名单，其它条目与注释保留', () => {
    const { dir, cleanup } = makeProfile(BASE);
    try {
      const ok = persistBotCredentialsToProfile('cat', { appId: 'new-cat', appSecret: 'new-secret', userOpenid: 'u1' }, dir, createLogger());
      expect(ok).toBe(true);
      const entry = botEntry(dir, 'cat');
      expect(entry?.appId).toBe('new-cat');
      expect(entry?.appSecret).toBe('new-secret');
      const access = entry?.access as { c2cMode?: string; c2cAllow?: string[] };
      expect(access.c2cMode).toBe('allowlist');
      expect(access.c2cAllow).toEqual(['u1']);
      const text = readFileSync(join(dir, 'cordis.patch.yml'), 'utf8');
      expect(text).toContain('keep this comment');
      expect(text).toContain('id: other-plugin');
    } finally {
      cleanup();
    }
  });

  it('新增 bot：在 bots[] 追加并启用', () => {
    const { dir, cleanup } = makeProfile(BASE);
    try {
      expect(persistBotCredentialsToProfile('helper', { appId: 'helper-app', appSecret: 'helper-secret' }, dir, createLogger())).toBe(true);
      const entry = botEntry(dir, 'helper');
      expect(entry?.appId).toBe('helper-app');
      expect(entry?.enabled).toBe(true);
    } finally {
      cleanup();
    }
  });

  it('bots[] 中 id 重复时 fail-closed，原文件不变', () => {
    const dup = `${BASE}      - id: cat
        appId: second
        appSecret: second-secret
`;
    const { dir, cleanup } = makeProfile(dup);
    try {
      const before = readFileSync(join(dir, 'cordis.patch.yml'), 'utf8');
      const ok = persistBotCredentialsToProfile('cat', { appId: 'x', appSecret: 'y' }, dir, createLogger());
      expect(ok).toBe(false);
      expect(readFileSync(join(dir, 'cordis.patch.yml'), 'utf8')).toBe(before);
    } finally {
      cleanup();
    }
  });
});

describe('addBotToProfile / removeBotFromProfile', () => {
  it('新增空凭据、停用状态的机器人', () => {
    const { dir, cleanup } = makeProfile(BASE);
    try {
      expect(addBotToProfile('helper', '助手', dir, createLogger())).toBe(true);
      const entry = botEntry(dir, 'helper');
      expect(entry?.label).toBe('助手');
      expect(entry?.appId).toBe('');
      expect(entry?.enabled).toBe(false);
    } finally {
      cleanup();
    }
  });

  it('重复新增同一 id 返回 false 且不写文件', () => {
    const { dir, cleanup } = makeProfile(BASE);
    try {
      const logger = createLogger();
      expect(addBotToProfile('cat', 'again', dir, logger)).toBe(false);
      expect(readFileSync(join(dir, 'cordis.patch.yml'), 'utf8')).toBe(BASE);
    } finally {
      cleanup();
    }
  });

  it('删除机器人条目，保留其它 bot 与插件条目', () => {
    const { dir, cleanup } = makeProfile(`${BASE}      - id: helper
        appId: helper-app
        appSecret: helper-secret
`);
    try {
      expect(removeBotFromProfile('cat', dir, createLogger())).toBe(true);
      expect(botEntry(dir, 'cat')).toBeUndefined();
      expect(botEntry(dir, 'helper')?.appId).toBe('helper-app');
      expect(readFileSync(join(dir, 'cordis.patch.yml'), 'utf8')).toContain('id: other-plugin');
    } finally {
      cleanup();
    }
  });
});

describe('removeBotCredentialsFromProfile / updateBotProfileConfig', () => {
  it('解绑删除凭据并停用，不删其它字段', () => {
    const { dir, cleanup } = makeProfile(BASE);
    try {
      expect(removeBotCredentialsFromProfile('cat', dir, createLogger())).toBe(true);
      const entry = botEntry(dir, 'cat');
      expect(entry?.appId).toBeUndefined();
      expect(entry?.appSecret).toBeUndefined();
      expect(entry?.enabled).toBe(false);
    } finally {
      cleanup();
    }
  });

  it('配置合并只写目标 bot，忽略 appId/appSecret/appSecretRef/id', () => {
    const { dir, cleanup } = makeProfile(BASE);
    try {
      expect(updateBotProfileConfig('cat', {
        preset: 'neko', appId: 'ignored', appSecret: 'ignored', appSecretRef: 'IGNORED', id: 'ignored',
        access: { c2cMode: 'open', c2cAllow: [], groupMode: 'open', groupAllow: [] },
      }, dir, createLogger())).toBe(true);
      const entry = botEntry(dir, 'cat');
      expect(entry?.preset).toBe('neko');
      expect(entry?.id).toBe('cat');
      expect(entry?.appId).toBe('old-cat');
      expect(entry?.appSecretRef).toBeUndefined();
      expect((entry?.access as { c2cMode?: string }).c2cMode).toBe('open');
    } finally {
      cleanup();
    }
  });
});

describe('persistBotCredentialRefToProfile', () => {
  it('只写 appId + appSecretRef，删除同条目的明文密钥', () => {
    const { dir, cleanup } = makeProfile(BASE);
    try {
      const ok = persistBotCredentialRefToProfile('cat', {
        appId: 'new-cat', appSecretRef: 'QQBOT_CAT_SECRET',
      }, dir, createLogger());
      expect(ok).toBe(true);
      const entry = botEntry(dir, 'cat');
      expect(entry?.appId).toBe('new-cat');
      expect(entry?.appSecretRef).toBe('QQBOT_CAT_SECRET');
      expect(entry?.appSecret).toBeUndefined();
      expect(entry?.enabled).toBe(true);
      expect(readFileSync(join(dir, 'cordis.patch.yml'), 'utf8')).toContain('keep this comment');
    } finally {
      cleanup();
    }
  });

  it('解绑同时删除凭据引用并停用', () => {
    const { dir, cleanup } = makeProfile(BASE);
    try {
      expect(persistBotCredentialRefToProfile('cat', { appId: 'x', appSecretRef: 'QQBOT_X_SECRET' }, dir, createLogger())).toBe(true);
      expect(removeBotCredentialsFromProfile('cat', dir, createLogger())).toBe(true);
      const entry = botEntry(dir, 'cat');
      expect(entry?.appId).toBeUndefined();
      expect(entry?.appSecret).toBeUndefined();
      expect(entry?.appSecretRef).toBeUndefined();
      expect(entry?.enabled).toBe(false);
    } finally {
      cleanup();
    }
  });
});

const LEGACY = `- id: im-qqbot
  config:
    appId: legacy-app
    appSecret: legacy-secret
    entity: 猫猫
`;

describe('addLegacyBotToProfile', () => {
  it('旧单 bot 迁移成 bots[] 的 default 项（幂等）', () => {
    const { dir, cleanup } = makeProfile(LEGACY);
    try {
      expect(addLegacyBotToProfile('legacy-app', 'legacy-secret', dir, createLogger())).toBe(true);
      const entry = botEntry(dir, 'default');
      expect(entry?.appId).toBe('legacy-app');
      expect(entry?.appSecret).toBe('legacy-secret');
      expect(entry?.enabled).toBe(true);
      expect(addLegacyBotToProfile('legacy-app', 'legacy-secret', dir, createLogger())).toBe(false);
      const row = rows(dir).find(item => item.id === 'im-qqbot');
      const bots = (row?.config as { bots?: unknown[] } | undefined)?.bots;
      expect(bots).toHaveLength(1);
    } finally {
      cleanup();
    }
  });

  it('已有 bots[] 时不迁移', () => {
    const { dir, cleanup } = makeProfile(BASE);
    try {
      expect(addLegacyBotToProfile('legacy-app', 'legacy-secret', dir, createLogger())).toBe(false);
      expect(botEntry(dir, 'default')).toBeUndefined();
    } finally {
      cleanup();
    }
  });
});

describe('addBotToProfileMigratingLegacy', () => {
  it('旧单 bot + 新机器人：一次写入内完成迁移与追加', () => {
    const { dir, cleanup } = makeProfile(LEGACY);
    try {
      expect(addBotToProfileMigratingLegacy('helper', '助手', { appId: 'legacy-app', appSecret: 'legacy-secret' }, dir, createLogger())).toBe(true);
      expect(botEntry(dir, 'default')?.appId).toBe('legacy-app');
      expect(botEntry(dir, 'helper')?.enabled).toBe(false);
      const row = rows(dir).find(item => item.id === 'im-qqbot');
      const bots = (row?.config as { bots?: unknown[] } | undefined)?.bots;
      expect(bots).toHaveLength(2);
    } finally {
      cleanup();
    }
  });
});

describe('删除最后一个机器人', () => {
  it('同时清掉顶层 appId/appSecret，避免 legacy 回落复活', () => {
    const { dir, cleanup } = makeProfile(LEGACY);
    try {
      addBotToProfileMigratingLegacy('helper', '助手', { appId: 'legacy-app', appSecret: 'legacy-secret' }, dir, createLogger());
      expect(removeBotFromProfile('helper', dir, createLogger())).toBe(true);
      expect(removeBotFromProfile('default', dir, createLogger())).toBe(true);
      const row = rows(dir).find(item => item.id === 'im-qqbot');
      const cfg = row?.config as { bots?: unknown[]; appId?: string; appSecret?: string } | undefined;
      expect(cfg?.appId).toBeUndefined();
      expect(cfg?.appSecret).toBeUndefined();
      expect(cfg?.bots).toEqual([]);
    } finally {
      cleanup();
    }
  });
});
