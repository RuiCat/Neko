/**
 * K28 主会话提示词段：section 注册形状、惰性读取、空值不注入、释放幂等与失败降级。
 */
import { describe, expect, it, vi } from 'vitest';
import type { Context } from '@deepseek-ai/cordis';
import { MAIN_PROMPT_ORDER, MAIN_PROMPT_SECTION_NAME, installMainSessionPrompt } from './mainPrompt.ts';

/** 假 systemPrompt 服务：记录 section 规格，返回注销函数。 */
function fakeAgentCtx() {
  const sections = new Map<string, { name: string; order: number; interpolate?: boolean; text: () => string }>();
  const service = {
    section(spec: { name: string; order: number; interpolate?: boolean; text: () => string }) {
      sections.set(spec.name, spec);
      return () => { sections.delete(spec.name); };
    },
  };
  const ctx = { get: (name: string) => (name === 'systemPrompt' ? service : undefined) } as unknown as Context;
  return { ctx, sections };
}

describe('installMainSessionPrompt（K28）', () => {
  it('注册固定段名/顺序，插值关闭', () => {
    const { ctx, sections } = fakeAgentCtx();
    const handle = installMainSessionPrompt(ctx, () => '提示');
    expect(handle).toBeDefined();
    const spec = sections.get(MAIN_PROMPT_SECTION_NAME);
    expect(MAIN_PROMPT_SECTION_NAME).toBe('qqbot:main');
    expect(spec?.order).toBe(MAIN_PROMPT_ORDER);
    expect(spec?.interpolate).toBe(false);
    expect(spec?.text()).toBe('提示');
    handle?.dispose();
    expect(sections.size).toBe(0);
  });

  it('每步惰性读取：配置改动即时生效（无需重装）', () => {
    const { ctx, sections } = fakeAgentCtx();
    let text = 'OLD';
    installMainSessionPrompt(ctx, () => text);
    const spec = sections.get(MAIN_PROMPT_SECTION_NAME);
    expect(spec?.text()).toBe('OLD');
    text = 'NEW';
    expect(spec?.text()).toBe('NEW');
  });

  it('空文本/读取抛错/非字符串 → 空串（宿主丢弃空段）', () => {
    const { ctx, sections } = fakeAgentCtx();
    let mode: 'empty' | 'throw' | 'weird' = 'empty';
    installMainSessionPrompt(ctx, () => {
      if (mode === 'throw') throw new Error('boom');
      if (mode === 'weird') return 42 as unknown as string;
      return '';
    });
    const spec = sections.get(MAIN_PROMPT_SECTION_NAME);
    expect(spec?.text()).toBe('');
    mode = 'throw';
    expect(spec?.text()).toBe('');
    mode = 'weird';
    expect(spec?.text()).toBe('');
  });

  it('宿主缺 systemPrompt / 注册抛错 → undefined，只 warn 不抛', () => {
    const warn = vi.fn();
    const logger = { warn, info: vi.fn(), error: vi.fn(), debug: vi.fn() };
    expect(installMainSessionPrompt({ get: () => undefined } as unknown as Context, () => 'x', logger)).toBeUndefined();
    expect(warn).not.toHaveBeenCalled();
    const throwing = {
      get: () => ({ section: () => { throw new Error('register failed'); } }),
    } as unknown as Context;
    expect(installMainSessionPrompt(throwing, () => 'x', logger)).toBeUndefined();
    expect(warn).toHaveBeenCalledTimes(1);
  });

  it('dispose 幂等：重复释放不抛错', () => {
    const { ctx } = fakeAgentCtx();
    const handle = installMainSessionPrompt(ctx, () => 'x');
    handle?.dispose();
    expect(() => handle?.dispose()).not.toThrow();
  });
});
