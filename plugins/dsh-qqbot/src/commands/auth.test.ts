import { describe, it, expect, vi } from 'vitest';
import { commandAllowlistAuth } from './auth.ts';
import type { SlashCommandHandlerContext } from '@tencent-connect/qqbot-nodejs';

function makeCtx(senderId: string | undefined): SlashCommandHandlerContext {
  return {
    message: { senderId, kind: 'c2c' },
  } as unknown as SlashCommandHandlerContext;
}

const allowlist = { access: { c2cMode: 'allowlist' as const, c2cAllow: ['u1', 'u2'] } };

describe('commandAllowlistAuth（#5：命令白名单鉴权，复用审批名单语义）', () => {
  it('allowlist 模式：名单内用户放行', () => {
    expect(commandAllowlistAuth(allowlist)(makeCtx('u1'))).toBe(true);
  });

  it('allowlist 模式：名单外用户拒绝并返回提示', () => {
    const auth = commandAllowlistAuth(allowlist);
    const result = auth(makeCtx('stranger'));
    expect(result).not.toBe(true);
    expect(typeof result).toBe('string');
  });

  it('allowlist 模式：身份缺失 fail-closed', () => {
    const auth = commandAllowlistAuth(allowlist);
    const result = auth(makeCtx(undefined));
    expect(result).not.toBe(true);
  });

  it('open 模式：直接放行（名单不参与）', () => {
    const open = { access: { c2cMode: 'open' as const, c2cAllow: [] } };
    expect(commandAllowlistAuth(open)(makeCtx('anyone'))).toBe(true);
  });

  it('拒绝时记录 warn 日志', () => {
    const logger = { warn: vi.fn() };
    commandAllowlistAuth(allowlist, logger)(makeCtx('stranger'));
    expect(logger.warn).toHaveBeenCalledOnce();
    expect(logger.warn.mock.calls[0]?.[0]).toContain('stranger');
  });
});

describe('commandAllowlistAuth：K27 owner 管理员绕过', () => {
  const withOwner = { ...allowlist, owner: { openid: 'boss' } };

  it('allowlist 模式：owner.openid 不在名单也放行', () => {
    expect(commandAllowlistAuth(withOwner)(makeCtx('boss'))).toBe(true);
  });

  it('allowlist 模式：非 owner 且不在名单仍拒绝；owner 空值不参与', () => {
    expect(commandAllowlistAuth(withOwner)(makeCtx('stranger'))).not.toBe(true);
    const emptyOwner = { ...allowlist, owner: { openid: '   ' } };
    expect(commandAllowlistAuth(emptyOwner)(makeCtx('boss'))).not.toBe(true);
  });

  it('open 模式不变：任何人放行', () => {
    const open = { access: { c2cMode: 'open' as const, c2cAllow: [] }, owner: { openid: 'boss' } };
    expect(commandAllowlistAuth(open)(makeCtx('anyone'))).toBe(true);
  });

  it('disabled 模式：即使 owner 也拒绝', () => {
    const disabled = { access: { c2cMode: 'disabled' as const, c2cAllow: [] }, owner: { openid: 'boss' } };
    const result = commandAllowlistAuth(disabled)(makeCtx('boss'));
    expect(result).not.toBe(true);
    expect(typeof result).toBe('string');
  });
});
