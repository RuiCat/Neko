/**
 * K26 单会话提示词纯函数单测：归一化校验/重复检测/命中选择/设置页 JSON 解析（config-panel 复用同一函数）。
 */
import { describe, expect, it } from 'vitest';
import {
  CONVERSATION_PROMPT_MAX_CHARS,
  formatConversationPromptsJson,
  normalizeConversationPrompts,
  parseConversationPromptsJson,
  selectConversationPrompt,
  type ConversationPromptEntry,
} from './conversationPrompts.ts';
import { EXTRA_MAX_CHARS } from '../prompt/extra.ts';

const valid: ConversationPromptEntry[] = [
  { scope: 'c2c', peerId: 'u1', prompt: '仅私聊' },
  { scope: 'group', peerId: 'g1', prompt: '仅群聊' },
];

describe('normalizeConversationPrompts（K26 fail-fast 校验）', () => {
  it('缺省/undefined → 空数组；合法列表原样保留并 trim peerId', () => {
    expect(normalizeConversationPrompts(undefined)).toEqual([]);
    expect(normalizeConversationPrompts(null)).toEqual([]);
    expect(normalizeConversationPrompts([])).toEqual([]);
    expect(normalizeConversationPrompts([
      { scope: 'c2c', peerId: '  u1  ', prompt: ' 提示词 ' },
    ])).toEqual([{ scope: 'c2c', peerId: 'u1', prompt: ' 提示词 ' }]); // prompt 保留原文本
  });

  it('非法 scope 报错（含项序号与收到的值）', () => {
    expect(() => normalizeConversationPrompts([{ scope: 'dm', peerId: 'x', prompt: 'p' }]))
      .toThrow(/conversationPrompts\[0\]\.scope/);
  });

  it('空 peerId / 空白 peerId / 非字符串 peerId 报错', () => {
    for (const peerId of ['', '   ', 42]) {
      expect(() => normalizeConversationPrompts([{ scope: 'c2c', peerId, prompt: 'p' }]))
        .toThrow(/peerId 不能为空/);
    }
  });

  it('空 prompt / 空白 prompt / 非字符串 prompt 报错', () => {
    for (const prompt of ['', ' \n\t ', 42]) {
      expect(() => normalizeConversationPrompts([{ scope: 'c2c', peerId: 'u1', prompt }]))
        .toThrow(/prompt 不能为空/);
    }
  });

  it('prompt 超 80000 字符报错；恰好 80000 合法', () => {
    expect(CONVERSATION_PROMPT_MAX_CHARS).toBe(EXTRA_MAX_CHARS); // 与 extra 注入上限一致
    expect(() => normalizeConversationPrompts([
      { scope: 'c2c', peerId: 'u1', prompt: 'x'.repeat(CONVERSATION_PROMPT_MAX_CHARS + 1) },
    ])).toThrow(/prompt 超长/);
    expect(() => normalizeConversationPrompts([
      { scope: 'c2c', peerId: 'u1', prompt: 'x'.repeat(CONVERSATION_PROMPT_MAX_CHARS) },
    ])).not.toThrow();
  });

  it('同列表 (scope,peerId) 重复报错，错误信息带重复键与项序号（trim 后比较）', () => {
    expect(() => normalizeConversationPrompts([
      { scope: 'c2c', peerId: 'u1', prompt: 'a' },
      { scope: 'c2c', peerId: ' u1 ', prompt: 'b' },
    ])).toThrow(/第 1 项重复.*c2c:u1/);
    // 不同 scope 同 peerId 不算重复
    expect(normalizeConversationPrompts([
      { scope: 'c2c', peerId: 'u1', prompt: 'a' },
      { scope: 'group', peerId: 'u1', prompt: 'b' },
    ])).toHaveLength(2);
  });

  it('非数组 / 非对象元素报错', () => {
    expect(() => normalizeConversationPrompts('nope')).toThrow(/必须是数组/);
    expect(() => normalizeConversationPrompts([42])).toThrow(/必须是对象/);
    expect(() => normalizeConversationPrompts([[]])).toThrow(/必须是对象/);
  });

  it('label 注入错误路径（bots[] 覆盖时用 bots[i].conversationPrompts）', () => {
    expect(() => normalizeConversationPrompts([{ scope: 'c2c', peerId: '', prompt: 'p' }], 'bots[1].conversationPrompts'))
      .toThrow(/bots\[1\]\.conversationPrompts\[0\]\.peerId/);
  });
});

describe('selectConversationPrompt（运行时命中）', () => {
  it('按 scope+peerId 命中；trim 后匹配', () => {
    expect(selectConversationPrompt(valid, 'c2c', 'u1')).toBe('仅私聊');
    expect(selectConversationPrompt(valid, 'group', 'g1')).toBe('仅群聊');
    expect(selectConversationPrompt(valid, 'c2c', ' u1 ')).toBe('仅私聊');
  });

  it('scope 不符 / peerId 不符 / 空列表 / 非法值 → undefined', () => {
    expect(selectConversationPrompt(valid, 'group', 'u1')).toBeUndefined();
    expect(selectConversationPrompt(valid, 'c2c', 'u9')).toBeUndefined();
    expect(selectConversationPrompt([], 'c2c', 'u1')).toBeUndefined();
    expect(selectConversationPrompt(undefined, 'c2c', 'u1')).toBeUndefined();
    expect(selectConversationPrompt('nope' as unknown, 'c2c', 'u1')).toBeUndefined();
    expect(selectConversationPrompt(valid, 'c2c', '')).toBeUndefined();
  });

  it('命中但 prompt 空白 → undefined（不注入空段）', () => {
    expect(selectConversationPrompt([{ scope: 'c2c', peerId: 'u1', prompt: '   ' }], 'c2c', 'u1')).toBeUndefined();
  });
});

describe('设置页 JSON 序列化/解析（config-panel 共用）', () => {
  it('format：空列表为 []，非空为可回读的 pretty JSON', () => {
    expect(formatConversationPromptsJson(undefined)).toBe('[]');
    expect(formatConversationPromptsJson([])).toBe('[]');
    expect(JSON.parse(formatConversationPromptsJson(valid))).toEqual(valid);
  });

  it('parse：空文本 → []; 合法 JSON → 归一化列表；可往返', () => {
    expect(parseConversationPromptsJson('')).toEqual({ ok: true, value: [] });
    expect(parseConversationPromptsJson('  \n ')).toEqual({ ok: true, value: [] });
    expect(parseConversationPromptsJson(formatConversationPromptsJson(valid))).toEqual({ ok: true, value: valid });
  });

  it('parse：JSON 语法错误 → 可读错误（含「JSON 解析失败」）', () => {
    const result = parseConversationPromptsJson('[{"scope":"c2c",');
    expect(result.ok).toBe(false);
    if (!result.ok) expect(result.error).toContain('JSON 解析失败');
  });

  it('parse：语义错误（非数组/非法 scope/重复）→ 归一化错误原文', () => {
    const notArray = parseConversationPromptsJson('{}');
    expect(notArray).toEqual({ ok: false, error: 'conversationPrompts 必须是数组' });
    const badScope = parseConversationPromptsJson('[{"scope":"dm","peerId":"u1","prompt":"p"}]');
    expect(badScope.ok).toBe(false);
    if (!badScope.ok) expect(badScope.error).toContain('.scope');
    const duplicated = parseConversationPromptsJson(JSON.stringify([valid[0], valid[0]]));
    expect(duplicated.ok).toBe(false);
    if (!duplicated.ok) expect(duplicated.error).toContain('重复');
  });
});
