/**
 * 多机器人共享 ask_user_question 补丁的归属分派测试（P2）。
 */
import { describe, expect, it, vi } from 'vitest';
import type { ChatScope, Logger, ReplyTarget } from '../types.ts';
import {
  QuestionChannel,
  type QuestionSessionRecordLike,
  type UserQuestionRequest,
  type UserQuestionResult,
  type UserQuestionsServiceLike,
} from './question-channel.ts';

function createLogger(): Logger {
  return { debug: vi.fn(), info: vi.fn(), warn: vi.fn(), error: vi.fn() };
}

function recordFor(sessionId: string): QuestionSessionRecordLike {
  const scope: ChatScope = 'c2c';
  const replyTarget: ReplyTarget = { scope, targetId: 'peer', msgId: 'msg' };
  return { sessionKey: `qqbot:app:${scope}:${sessionId}`, scope, replyTarget };
}

function createChannel(sessionIds: string[]): QuestionChannel {
  const manager = {
    findBySessionId: (sessionId: string) => (sessionIds.includes(sessionId) ? recordFor(sessionId) : undefined),
    getSessionRecord: () => undefined,
  };
  const sender = { sendMarkdown: vi.fn(async () => undefined) };
  return new QuestionChannel(manager, sender, { requireMention: true, askTimeoutMs: 1000 }, createLogger());
}

function spyAsk(channel: QuestionChannel): ReturnType<typeof vi.spyOn> {
  return vi.spyOn(channel as unknown as { askViaQQ: (...args: never[]) => Promise<UserQuestionResult> }, 'askViaQQ')
    .mockResolvedValue({ answers: [] });
}

function requestFor(sessionId: string): UserQuestionRequest {
  return { agent: { id: sessionId }, questions: [{ id: 'q', question: '选哪个？', options: [{ label: 'A' }] }] };
}

describe('多机器人 question channel', () => {
  it('一次补丁按会话归属分派；非归属通道不处理，原 ask 不被调用', async () => {
    const channelA = createChannel(['session-a']);
    const channelB = createChannel(['session-b']);
    const spyA = spyAsk(channelA);
    const spyB = spyAsk(channelB);
    const original = vi.fn(async (_request: UserQuestionRequest) => ({ answers: [] }));
    const uq = { ask: original } as UserQuestionsServiceLike;
    const ctx = { get: (name: string) => (name === 'userQuestions' ? uq : undefined) };

    channelA.install(ctx);
    channelB.install(ctx);
    expect(uq.ask).not.toBe(original);

    await uq.ask(requestFor('session-b'));
    expect(spyB).toHaveBeenCalledTimes(1);
    expect(spyA).not.toHaveBeenCalled();
    expect(original).not.toHaveBeenCalled();

    await uq.ask(requestFor('unknown-session'));
    expect(original).toHaveBeenCalledTimes(1);
  });

  it('卸载一个通道不影响其它；全部卸载后恢复原 ask', async () => {
    const channelA = createChannel(['session-a']);
    const channelB = createChannel(['session-b']);
    spyAsk(channelA);
    const spyB = spyAsk(channelB);
    const original = vi.fn(async (_request: UserQuestionRequest) => ({ answers: [] }));
    const uq = { ask: original } as UserQuestionsServiceLike;
    const ctx = { get: (name: string) => (name === 'userQuestions' ? uq : undefined) };

    channelA.install(ctx);
    channelB.install(ctx);
    const patched = uq.ask;
    channelA.uninstall();

    await uq.ask(requestFor('session-b'));
    expect(spyB).toHaveBeenCalledTimes(1);

    channelB.uninstall();
    expect(uq.ask).not.toBe(patched);
    expect((uq as UserQuestionsServiceLike & { __qqQuestionPatched?: boolean }).__qqQuestionPatched).toBeUndefined();
    await uq.ask(requestFor('session-b'));
    expect(original).toHaveBeenCalledTimes(1);
  });
});
