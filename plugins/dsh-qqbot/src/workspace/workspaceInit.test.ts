/**
 * 工作区初始化与 Neko 部署：结构、幂等、不覆盖已有内容。
 */
import { mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { afterEach, describe, expect, it } from 'vitest';
import { initWorkspace } from './initWorkspace.ts';
import { deployNekoSystem, NEKO_SUBPATHS } from './deployNeko.ts';
import { MEMORY_SUBDIRS, WORKSPACE_DIRS } from './workspaceLayout.ts';

const roots: string[] = [];

/** 建临时目录并在用例结束后清理。 */
function tempDir(prefix: string): string {
  const dir = mkdtempSync(join(tmpdir(), prefix));
  roots.push(dir);
  return dir;
}

afterEach(() => {
  for (const dir of roots.splice(0)) rmSync(dir, { recursive: true, force: true });
});

describe('initWorkspace（工作区目录模板）', () => {
  it('空目录下建出完整结构', () => {
    const dir = tempDir('neko-ws-');
    const result = initWorkspace(dir);
    expect(result.created).toEqual([...WORKSPACE_DIRS, 'AGENTS.md', 'memory/INDEX.md']);
    expect(result.skipped).toEqual([]);
    for (const sub of MEMORY_SUBDIRS) expect(result.created).toContain(`memory/${sub}`);
  });

  it('重复调用幂等：全部跳过，不重写', () => {
    const dir = tempDir('neko-ws-');
    initWorkspace(dir);
    const second = initWorkspace(dir);
    expect(second.created).toEqual([]);
    expect(second.skipped).toContain('AGENTS.md');
    expect(second.skipped).toContain('memory/INDEX.md');
  });

  it('已有 AGENTS.md / INDEX.md 不被覆盖', () => {
    const dir = tempDir('neko-ws-');
    mkdirSync(join(dir, 'memory'), { recursive: true });
    writeFileSync(join(dir, 'AGENTS.md'), '# 手写人设\n', 'utf8');
    writeFileSync(join(dir, 'memory', 'INDEX.md'), '# 手写索引\n', 'utf8');
    initWorkspace(dir);
    expect(readFileSync(join(dir, 'AGENTS.md'), 'utf8')).toBe('# 手写人设\n');
    expect(readFileSync(join(dir, 'memory', 'INDEX.md'), 'utf8')).toBe('# 手写索引\n');
  });
});

describe('deployNekoSystem（共享脚本系统部署）', () => {
  it('源未配置 → 记录 missing，不抛错', () => {
    const result = deployNekoSystem({ targetDir: tempDir('neko-t-') });
    expect(result.deployed).toEqual([]);
    expect(result.missing).toContain('<source-unset>');
  });

  it('按子路径拷贝，已存在则跳过', () => {
    const source = tempDir('neko-src-');
    const target = tempDir('neko-t-');
    for (const sub of NEKO_SUBPATHS) {
      mkdirSync(join(source, sub), { recursive: true });
      writeFileSync(join(source, sub, 'marker.txt'), sub, 'utf8');
    }
    const first = deployNekoSystem({ sourceDir: source, targetDir: target });
    expect(first.deployed).toEqual([...NEKO_SUBPATHS]);
    expect(readFileSync(join(target, 'scripts', 'marker.txt'), 'utf8')).toBe('scripts');
    const second = deployNekoSystem({ sourceDir: source, targetDir: target });
    expect(second.deployed).toEqual([]);
    expect(second.skipped).toEqual([...NEKO_SUBPATHS]);
  });
});
