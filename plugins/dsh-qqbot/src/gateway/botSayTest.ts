/**
 * K15 `qqbotBots.say` 单测：主动直发（不触发模型回合）、bot 定位、失败原因与 fail-soft。
 */
import { describe, expect, it, vi } from 'vitest';
import type { Context } from '@deepseek-ai/cordis';
import type { ChatScope, ReplyTarget } from '../types.ts';
import { provideQQBotBots } from './qqbotBotsService.ts';
import { sayToBot, SAY_TEXT_MAX_CHARS, type SayableBot } from './botSay.ts';

/** 会话回复目标替身（带 msgId，验证直发不复用被动窗口）。 */
function target(scope: ChatScope, targetId: string): ReplyTarget {
  return { scope, targetId, msgId: `msg-${targetId}` };
}

/** 直发 bot 替身：记录发送参数，并暴露 manager 上的 agent 入口用于断言未被调用。 */
function fakeBot(
  id: string,
  latest: (scope: ChatScope) => ReplyTarget | undefined,
  sendImpl: (resolved: ReplyTarget, text: string) => Promise<unknown> = async () => ({}),
) {
  const sendMarkdown = vi.fn(sendImpl);
  const getOrCreate = vi.fn();
  const bot: SayableBot = {
    id,
    manager: { latestTarget: latest },
    sender: { sendMarkdown },
  } as unknown as SayableBot;
  (bot.manager as unknown as { getOrCreate: typeof getOrCreate }).getOrCreate = getOrCreate;
  return { bot, sendMarkdown, getOrCreate };
}

describe('sayToBot：成功路径', () => {
  it('显式 botId + c2c：取最近活跃目标、强制主动推送、返回 targetId；不触发 agent 回合', async () => {
    const cat = fakeBot('cat', (scope) => (scope === 'c2c' ? target('c2c', 'u1') : undefined));
    const helper = fakeBot('helper', () => undefined);

    const result = await sayToBot([cat.bot, helper.bot], { botId: 'cat', scope: 'c2c', text: 'hello' });

    expect(result).toEqual({ ok: true, botId: 'cat', scope: 'c2c', targetId: 'u1' });
    expect(cat.sendMarkdown).toHaveBeenCalledWith(
      { scope: 'c2c', targetId: 'u1', forceProactive: true },
      'hello',
    );
    expect(helper.sendMarkdown).not.toHaveBeenCalled();
    expect(cat.getOrCreate).not.toHaveBeenCalled(); // 直发不进入会话/模型回合
  });

  it('省略 botId 且唯一 bot → 成功；group scope 同样可用', async () => {
    const only = fakeBot('cat', (scope) => (scope === 'group' ? target('group', 'g1') : undefined));

    const result = await sayToBot([only.bot], { scope: 'group', text: '群消息' });

    expect(result).toEqual({ ok: true, botId: 'cat', scope: 'group', targetId: 'g1' });
    expect(only.sendMarkdown).toHaveBeenCalledWith(
      { scope: 'group', targetId: 'g1', forceProactive: true },
      '群消息',
    );
  });

  it('text 恰好 1000 字符合法', async () => {
    const only = fakeBot('cat', () => target('c2c', 'u1'));
    const result = await sayToBot([only.bot], { botId: 'cat', scope: 'c2c', text: 'x'.repeat(SAY_TEXT_MAX_CHARS) });
    expect(result.ok).toBe(true);
  });
});

describe('sayToBot：bot 定位失败', () => {
  it('省略 botId 且有多个运行中 bot → ambiguous_bot，不发送', async () => {
    const a = fakeBot('a', () => target('c2c', 'u1'));
    const b = fakeBot('b', () => target('c2c', 'u2'));
    expect(await sayToBot([a.bot, b.bot], { scope: 'c2c', text: 'hi' })).toEqual({ ok: false, reason: 'ambiguous_bot' });
    expect(a.sendMarkdown).not.toHaveBeenCalled();
    expect(b.sendMarkdown).not.toHaveBeenCalled();
  });

  it('空 botId 视同省略（唯一 bot 可发）', async () => {
    const only = fakeBot('cat', () => target('c2c', 'u1'));
    expect(await sayToBot([only.bot], { botId: '', scope: 'c2c', text: 'hi' })).toEqual({
      ok: true, botId: 'cat', scope: 'c2c', targetId: 'u1',
    });
  });

  it('显式 botId 不存在 / 没有任何 bot → bot_not_found', async () => {
    const cat = fakeBot('cat', () => target('c2c', 'u1'));
    expect(await sayToBot([cat.bot], { botId: 'nope', scope: 'c2c', text: 'hi' })).toEqual({ ok: false, reason: 'bot_not_found' });
    expect(await sayToBot([], { scope: 'c2c', text: 'hi' })).toEqual({ ok: false, reason: 'bot_not_found' });
    expect(cat.sendMarkdown).not.toHaveBeenCalled();
  });
});

describe('sayToBot：会话与发送失败', () => {
  it('该 scope 无最近活跃会话 → no_session，不发送', async () => {
    const cat = fakeBot('cat', (scope) => (scope === 'c2c' ? target('c2c', 'u1') : undefined));
    expect(await sayToBot([cat.bot], { botId: 'cat', scope: 'group', text: 'hi' })).toEqual({ ok: false, reason: 'no_session' });
    expect(cat.sendMarkdown).not.toHaveBeenCalled();
  });

  it('非法 scope → no_session（不抛）', async () => {
    const cat = fakeBot('cat', () => target('c2c', 'u1'));
    const bad = { botId: 'cat', scope: 'dm' as unknown as ChatScope, text: 'hi' };
    expect(await sayToBot([cat.bot], bad)).toEqual({ ok: false, reason: 'no_session' });
  });

  it('发送 reject 或同步抛错 → send_failed（不抛）', async () => {
    const rejected = fakeBot('cat', () => target('c2c', 'u1'), () => Promise.reject(new Error('platform down')));
    expect(await sayToBot([rejected.bot], { botId: 'cat', scope: 'c2c', text: 'hi' })).toEqual({ ok: false, reason: 'send_failed' });

    const throwing = fakeBot('cat', () => target('c2c', 'u1'), () => { throw new Error('sync boom'); });
    expect(await sayToBot([throwing.bot], { botId: 'cat', scope: 'c2c', text: 'hi' })).toEqual({ ok: false, reason: 'send_failed' });
  });

  it('manager.latestTarget 抛错 → send_failed（桥接不被炸）', async () => {
    const cat = fakeBot('cat', () => { throw new Error('manager exploded'); });
    expect(await sayToBot([cat.bot], { botId: 'cat', scope: 'c2c', text: 'hi' })).toEqual({ ok: false, reason: 'send_failed' });
  });
});

describe('sayToBot：text 校验（K15 invalid_text）', () => {
  it.each([
    ['空串', ''],
    ['纯空白', '   \n\t '],
    ['超 1000 字符', 'x'.repeat(SAY_TEXT_MAX_CHARS + 1)],
    ['非字符串', 42 as unknown as string],
  ])('%s → invalid_text，不发送', async (_label, text) => {
    const cat = fakeBot('cat', () => target('c2c', 'u1'));
    expect(await sayToBot([cat.bot], { botId: 'cat', scope: 'c2c', text })).toEqual({ ok: false, reason: 'invalid_text' });
    expect(cat.sendMarkdown).not.toHaveBeenCalled();
  });

  it('request 为 null → invalid_text（不抛）', async () => {
    expect(await sayToBot([], null as unknown as Parameters<typeof sayToBot>[1])).toEqual({ ok: false, reason: 'invalid_text' });
  });
});

describe('qqbotBots 服务（K15 接线）', () => {
  /** 带 provide 记录的 ctx 替身。 */
  function provideCtx(): { ctx: Context; provided: Map<string, unknown> } {
    const provided = new Map<string, unknown>();
    const ctx = {
      provide: (name: string, value: unknown) => {
        provided.set(name, value);
        return () => { provided.delete(name); };
      },
    } as unknown as Context;
    return { ctx, provided };
  }

  it('服务暴露 say：转发到运行中 bot；bindings 形状不变', async () => {
    const cat = fakeBot('cat', () => target('c2c', 'u1'));
    const runtime = {
      id: 'cat',
      label: '猫猫',
      legacy: false,
      config: { appId: 'app-cat', cwd: '/work/cat' },
      manager: { mainSessionId: 'session-main', latestTarget: cat.bot.manager.latestTarget },
      sender: cat.bot.sender,
    };
    const { ctx, provided } = provideCtx();
    provideQQBotBots(ctx, () => [runtime], undefined as never);

    const service = provided.get('qqbotBots') as {
      bindings(): Array<Record<string, unknown>>;
      say(request: { botId?: string; scope: ChatScope; text: string }): Promise<unknown>;
    };
    expect(service.bindings()).toHaveLength(1);
    expect(service.bindings()[0]).toMatchObject({ botId: 'cat', cwd: '/work/cat', mainSessionId: 'session-main' });

    expect(await service.say({ scope: 'c2c', text: 'from service' })).toEqual({
      ok: true, botId: 'cat', scope: 'c2c', targetId: 'u1',
    });
    expect(cat.sendMarkdown).toHaveBeenCalledWith({ scope: 'c2c', targetId: 'u1', forceProactive: true }, 'from service');
  });

  it('listBots 同步抛错 → say 返回 send_failed，不向桥接抛错', async () => {
    const { ctx, provided } = provideCtx();
    provideQQBotBots(ctx, () => { throw new Error('coordinator gone'); });
    const service = provided.get('qqbotBots') as { say(request: unknown): Promise<unknown> };
    await expect(service.say({ scope: 'c2c', text: 'hi' })).resolves.toEqual({ ok: false, reason: 'send_failed' });
  });
});
