/**
 * 多机器人目录隔离：botHome 与 resolveAgentCwd 的实体/bot 维度解析。
 */
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import { botDirName, botHome, PLUGIN_HOME, resolveAgentCwd, resolveMediaRoot, sanitizeDirName } from './dirs.ts';

describe('多机器人目录隔离', () => {
  it('botHome 按 botId 分目录（未绑定实体时）', () => {
    expect(botHome('cat')).toBe(join(PLUGIN_HOME, 'bots', 'cat'));
  });

  it('缺省工作目录按 bot 隔离', () => {
    expect(resolveAgentCwd(undefined, 'cat')).toBe(join(PLUGIN_HOME, 'bots', 'cat', 'workspace'));
    expect(resolveAgentCwd('', 'helper')).toBe(join(PLUGIN_HOME, 'bots', 'helper', 'workspace'));
  });

  it('绑定实体后目录名用实体名（允许中文）', () => {
    expect(botDirName('cat', '梦梦')).toBe('梦梦');
    expect(botHome('cat', '梦梦')).toBe(join(PLUGIN_HOME, 'bots', '梦梦'));
    expect(resolveAgentCwd(undefined, 'helper', '猫猫')).toBe(join(PLUGIN_HOME, 'bots', '猫猫', 'workspace'));
  });

  it('实体名做目录安全清理：空白/纯点/路径分隔符不进目录名', () => {
    expect(botDirName('cat', '  ')).toBe('cat');
    expect(botDirName('cat', '..')).toBe('cat');
    expect(sanitizeDirName('梦/梦')).toBe('梦梦');
    expect(sanitizeDirName('../escape')).toBe('..escape');
  });

  it('无 botId 时保持旧单 bot 路径（向后兼容）', () => {
    expect(resolveAgentCwd(undefined)).toBe(join(PLUGIN_HOME, 'workspace'));
  });

  it('显式 cwd 优先于 bot 隔离默认值', () => {
    expect(resolveAgentCwd('/opt/my-workspace', 'cat')).toBe('/opt/my-workspace');
    expect(resolveAgentCwd('/opt/my-workspace', 'cat', '梦梦')).toBe('/opt/my-workspace');
  });

  it('媒体根：多 bot 按目录隔离，legacy/无 botId 保持旧全局目录', () => {
    expect(resolveMediaRoot('cat')).toBe(join(PLUGIN_HOME, 'bots', 'cat', 'media'));
    expect(resolveMediaRoot('cat', false, '梦梦')).toBe(join(PLUGIN_HOME, 'bots', '梦梦', 'media'));
    expect(resolveMediaRoot('cat', true)).toBe(join(PLUGIN_HOME, 'media'));
    expect(resolveMediaRoot()).toBe(join(PLUGIN_HOME, 'media'));
  });
});
