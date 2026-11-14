/**
 * 每 bot 扫码注册表测试：懒创建、扫码成功写回、失败相位、刷新、HMR 接管与释放。
 */
import { describe, expect, it, vi } from 'vitest';
import { BotBindingRegistry } from './botBindingRegistry.ts';
import type { BindingService } from './binding-service.ts';
import type { SetupCredentials } from './persist.ts';
import type { Logger } from '../types.ts';

/** 可控扫码服务替身：start 的 Promise 由测试手动收敛，wireState 固定为 waiting。 */
function fakeService(): {
  service: BindingService;
  start: ReturnType<typeof vi.fn>;
  refresh: ReturnType<typeof vi.fn>;
  dispose: ReturnType<typeof vi.fn>;
  succeed: (credentials: SetupCredentials | null) => void;
  fail: (error: Error) => void;
} {
  let succeed!: (credentials: SetupCredentials | null) => void;
  let fail!: (error: Error) => void;
  const started = new Promise<SetupCredentials | null>((resolve, reject) => {
    succeed = resolve;
    fail = reject;
  });
  const start = vi.fn(() => started);
  const refresh = vi.fn();
  const dispose = vi.fn();
  const service = {
    start,
    refresh,
    dispose,
    wireState: vi.fn(() => ({ phase: 'waiting', url: 'https://q.qq.com/x', issuedAt: 1 })),
  } as unknown as BindingService;
  return { service, start, refresh, dispose, succeed, fail };
}

/** 等待 registry 的 start().then 链跑完（两跳微任务足够）。 */
async function flush(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
}

/** 测试日志器 */
function testLogger(): Logger {
  return { debug: vi.fn(), info: vi.fn(), warn: vi.fn(), error: vi.fn() } as unknown as Logger;
}

const CREDENTIALS: SetupCredentials = { appId: 'app-1', appSecret: 'secret-1' };

describe('BotBindingRegistry', () => {
  it('首次轮询才创建服务；未创建时回报 starting', () => {
    const fake = fakeService();
    const createService = vi.fn(() => fake.service);
    const registry = new BotBindingRegistry({ logger: testLogger(), persist: () => true, createService });

    expect(registry.wireState('lazy-bot')).toEqual({ phase: 'starting' });
    expect(createService).not.toHaveBeenCalled();

    const service = registry.serviceFor('lazy-bot');
    expect(service).toBe(fake.service);
    expect(createService).toHaveBeenCalledTimes(1);
    expect(fake.start).toHaveBeenCalledTimes(1);
    expect(registry.wireState('lazy-bot')).toMatchObject({ phase: 'waiting' });
    registry.dispose();
  });

  it('扫码成功把凭据写回该 bot', async () => {
    const fake = fakeService();
    const persist = vi.fn(() => true);
    const registry = new BotBindingRegistry({ logger: testLogger(), persist, createService: () => fake.service });

    registry.serviceFor('cat');
    fake.succeed(CREDENTIALS);
    await flush();

    expect(persist).toHaveBeenCalledWith('cat', CREDENTIALS);
    expect(registry.wireState('cat')).toMatchObject({ phase: 'waiting' });
    registry.dispose();
  });

  it('写回失败时相位变 failed，不永久停在 saving', async () => {
    const fake = fakeService();
    const registry = new BotBindingRegistry({
      logger: testLogger(),
      persist: () => false,
      createService: () => fake.service,
    });

    registry.serviceFor('cat');
    fake.succeed(CREDENTIALS);
    await flush();

    expect(registry.wireState('cat')).toMatchObject({ phase: 'failed' });
    registry.dispose();
  });

  it('扫码异常时相位变 failed', async () => {
    const fake = fakeService();
    const registry = new BotBindingRegistry({
      logger: testLogger(),
      persist: () => true,
      createService: () => fake.service,
    });

    registry.serviceFor('cat');
    fake.fail(new Error('二维码过期'));
    await flush();

    expect(registry.wireState('cat')).toEqual({ phase: 'failed', reason: '二维码过期' });
    registry.dispose();
  });

  it('refresh 交给该 bot 的服务；未创建时先创建再刷新', () => {
    const fake = fakeService();
    const registry = new BotBindingRegistry({
      logger: testLogger(),
      persist: () => true,
      createService: () => fake.service,
    });

    registry.refresh('cat');
    expect(fake.start).toHaveBeenCalledTimes(1);
    expect(fake.refresh).toHaveBeenCalledTimes(1);
    registry.dispose();
  });

  it('dispose 终止全部服务，重复调用幂等', () => {
    const cat = fakeService();
    const dog = fakeService();
    const registry = new BotBindingRegistry({
      logger: testLogger(),
      persist: () => true,
      createService: botId => (botId === 'cat' ? cat.service : dog.service),
    });
    registry.serviceFor('cat');
    registry.serviceFor('dog');
    registry.dispose();
    registry.dispose();
    expect(cat.dispose).toHaveBeenCalledTimes(1);
    expect(dog.dispose).toHaveBeenCalledTimes(1);
  });

  it('HMR 接管：新注册表为同 bot 创建服务时终止旧服务', () => {
    const oldFake = fakeService();
    const newFake = fakeService();
    const oldRegistry = new BotBindingRegistry({
      logger: testLogger(),
      persist: () => true,
      createService: () => oldFake.service,
    });
    oldRegistry.serviceFor('takeover-bot');

    const newRegistry = new BotBindingRegistry({
      logger: testLogger(),
      persist: () => true,
      createService: () => newFake.service,
    });
    newRegistry.serviceFor('takeover-bot');

    expect(oldFake.dispose).toHaveBeenCalledTimes(1);
    expect(newFake.dispose).not.toHaveBeenCalled();
    newRegistry.dispose();
    oldRegistry.dispose();
  });
});
