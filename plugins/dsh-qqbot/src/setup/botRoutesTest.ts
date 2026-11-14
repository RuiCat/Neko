/**
 * 多机器人路由注册测试：每 bot 一组 /qqbot/bots/<id>/... 端点，状态/配置按 bot 回显；
 * 未绑定 bot 注册扫码路由，已绑定 bot 的刷新不创建扫码服务。
 */
import { describe, expect, it, vi } from 'vitest';
import type { Context } from '@deepseek-ai/cordis';
import { registerBotCredentialRoutes } from './index.ts';
import { registerBotsListRoute } from './bind-routes.ts';
import type { BindingService } from './binding-service.ts';
import type { SetupCredentials } from './persist.ts';

vi.mock('../shared/index.ts', async (importOriginal) => {
  const original = await importOriginal<Record<string, unknown>>();
  return { ...original, getProfileDir: () => undefined };
});

interface CapturedRoute {
  path: string;
  handler: (req: unknown, res: unknown) => unknown;
}

function createCtx(): { ctx: Context; routes: Map<string, CapturedRoute>; disposeAll: () => void } {
  const routes = new Map<string, CapturedRoute>();
  const disposers: Array<() => void> = [];
  const webServer = {
    register: (route: CapturedRoute) => {
      routes.set(route.path, route);
      return () => routes.delete(route.path);
    },
  };
  const ctx = {
    inject: (_deps: readonly string[], callback: (scoped: unknown) => void) => callback({
      webServer,
      effect: (fn: () => () => void) => { disposers.push(fn()); },
    }),
  } as unknown as Context;
  return { ctx, routes, disposeAll: () => { for (const dispose of disposers.reverse()) dispose(); } };
}

function makeRes(): { res: unknown; body: () => unknown; status: () => number } {
  let status = 0;
  let body = '';
  const res = {
    writeHead: (code: number) => { status = code; },
    end: (payload: string) => { body = payload; },
  };
  return { res, body: () => JSON.parse(body), status: () => status };
}

/** 可控扫码服务替身（未绑定 bot 的路由测试用；wireState 固定 waiting）。 */
function fakeService(): { service: BindingService; start: ReturnType<typeof vi.fn>; refresh: ReturnType<typeof vi.fn> } {
  const start = vi.fn(() => new Promise<SetupCredentials | null>(() => {}));
  const refresh = vi.fn();
  const service = {
    start,
    refresh,
    dispose: vi.fn(),
    wireState: vi.fn(() => ({ phase: 'waiting', url: 'https://q.qq.com/x', issuedAt: 1 })),
  } as unknown as BindingService;
  return { service, start, refresh };
}

const logger = { debug: vi.fn(), info: vi.fn(), warn: vi.fn(), error: vi.fn() };

describe('GET /qqbot/bots（K4/K8 扩展字段）', () => {
  /** 调用 /qqbot/bots 并返回 (status, body) */
  async function listOf(handler: { handler?: (req: unknown, res: unknown) => unknown } | undefined) {
    const res = makeRes();
    await handler?.handler({ method: 'GET', headers: {} }, res.res);
    return { status: res.status(), body: res.body() };
  }

  it('运行中 bot：entity/cwd/mainSessionId/sessions；顶层 entities；旧字段不变且不含密钥', async () => {
    const { ctx, routes, disposeAll } = createCtx();
    registerBotCredentialRoutes(ctx, logger, [{
      id: 'cat',
      label: '猫猫',
      appId: 'cat-app',
      enabled: true,
      config: { preset: 'neko', appSecret: 'super-secret-value' },
      entity: '梦梦',
      cwd: '/srv/qqbot/workspaces/cat',
      mainSessionId: () => 'session-main',
      qqSessions: () => [
        { id: 'session-c2c', scope: 'c2c', peerId: 'u1' },
        { id: 'session-group', scope: 'group', peerId: 'g1' },
      ],
    }], { entities: () => ['猫猫', '梦梦'] });

    const { status, body } = await listOf(routes.get('/qqbot/bots'));
    expect(status).toBe(200);
    expect(body).toEqual({
      bots: [{
        id: 'cat', label: '猫猫', appId: 'cat-app', enabled: true,
        entity: '梦梦',
        cwd: '/srv/qqbot/workspaces/cat',
        mainSessionId: 'session-main',
        sessions: [
          { id: 'session-c2c', scope: 'c2c', peerId: 'u1' },
          { id: 'session-group', scope: 'group', peerId: 'g1' },
        ],
      }],
      entities: ['猫猫', '梦梦'],
    });
    expect(JSON.stringify(body)).not.toContain('super-secret-value');
    disposeAll();
  });

  it('停用/未运行 bot：省略 sessions 与 mainSessionId，entity/cwd 仍从配置层出现', async () => {
    const { ctx, routes, disposeAll } = createCtx();
    registerBotCredentialRoutes(ctx, logger, [{
      id: 'off',
      label: '离线',
      appId: '',
      enabled: false,
      config: {},
      entity: '猫猫',
      cwd: '/srv/qqbot/workspaces/off',
      // 未运行：协调查询返回 undefined（wire 省略字段）
      mainSessionId: () => undefined,
      qqSessions: () => undefined,
    }]);

    const { status, body } = await listOf(routes.get('/qqbot/bots'));
    expect(status).toBe(200);
    expect(body).toEqual({
      bots: [{
        id: 'off', label: '离线', appId: '', enabled: false,
        entity: '猫猫', cwd: '/srv/qqbot/workspaces/off',
      }],
    });
    disposeAll();
  });

  it('桥接服务不可用（未提供 entities）→ 顶层省略 entities，路由仍 200', async () => {
    const { ctx, routes, disposeAll } = createCtx();
    registerBotCredentialRoutes(ctx, logger, [{ id: 'cat', label: '猫猫', appId: 'a', enabled: true, config: {} }]);

    const { status, body } = await listOf(routes.get('/qqbot/bots'));
    expect(status).toBe(200);
    expect(body).toEqual({ bots: [{ id: 'cat', label: '猫猫', appId: 'a', enabled: true }] });
    expect((body as { entities?: unknown }).entities).toBeUndefined();
    disposeAll();
  });

  it('entities 来源抛错 → 按不可用处理（省略字段，仍 200）', async () => {
    const { ctx, routes, disposeAll } = createCtx();
    registerBotCredentialRoutes(ctx, logger, [{ id: 'cat', label: '猫猫', appId: 'a', enabled: true, config: {} }], {
      entities: () => { throw new Error('bridge died'); },
    });

    const { status, body } = await listOf(routes.get('/qqbot/bots'));
    expect(status).toBe(200);
    expect((body as { entities?: unknown }).entities).toBeUndefined();
    disposeAll();
  });
});

describe('registerBotCredentialRoutes', () => {
  it('每个 bot 注册独立路径；状态与配置只回显自己的数据', async () => {
    const { ctx, routes, disposeAll } = createCtx();
    registerBotCredentialRoutes(ctx, logger, [
      { id: 'cat', label: '猫猫', appId: 'cat-app', enabled: true, config: { preset: 'neko', appSecret: 'secret-cat' } },
      { id: 'helper', label: '助手', appId: 'helper-app', enabled: true, config: { preset: 'helper' } },
    ]);

    for (const botId of ['cat', 'helper']) {
      const base = `/qqbot/bots/${botId}`;
      expect(routes.has(`${base}/bind`)).toBe(true);
      expect(routes.has(`${base}/bind/refresh`)).toBe(true);
      expect(routes.has(`${base}/bind/manual`)).toBe(true);
      expect(routes.has(`${base}/bind/unbind`)).toBe(true);
      expect(routes.has(`${base}/config`)).toBe(true);
    }

    const state = routes.get('/qqbot/bots/cat/bind');
    const stateRes = makeRes();
    await state?.handler({ method: 'GET', headers: {} }, stateRes.res);
    expect(stateRes.body()).toEqual({ phase: 'bound', appId: 'cat-app' });

    const list = routes.get('/qqbot/bots');
    const listRes = makeRes();
    await list?.handler({ method: 'GET', headers: {} }, listRes.res);
    expect(listRes.body()).toEqual({ bots: [
      { id: 'cat', label: '猫猫', appId: 'cat-app', enabled: true },
      { id: 'helper', label: '助手', appId: 'helper-app', enabled: true },
    ] });

    const configRoute = routes.get('/qqbot/bots/cat/config');
    const configRes = makeRes();
    await configRoute?.handler({ method: 'GET', headers: {} }, configRes.res);
    const payload = configRes.body() as { config: Record<string, unknown> };
    expect(payload.config.preset).toBe('neko');
    expect(payload.config.appSecret).toBeUndefined();
    disposeAll();
  });

  it('未绑定 bot（appId 空）也注册扫码路由：状态来自惰性服务，刷新走该服务', async () => {
    const { ctx, routes, disposeAll } = createCtx();
    const fake = fakeService();
    const createService = vi.fn(() => fake.service);
    registerBotCredentialRoutes(ctx, logger, [
      { id: 'newbie', label: '新机器人', appId: '', enabled: false, config: {} },
    ], { createService });

    const state = routes.get('/qqbot/bots/newbie/bind');
    const stateRes = makeRes();
    await state?.handler({ method: 'GET', headers: {} }, stateRes.res);
    expect(stateRes.body()).toEqual({ phase: 'waiting', url: 'https://q.qq.com/x', issuedAt: 1 });
    expect(createService).toHaveBeenCalledWith('newbie');

    const refresh = routes.get('/qqbot/bots/newbie/bind/refresh');
    const refreshRes = makeRes();
    await refresh?.handler({ method: 'POST', headers: {}, resume: () => {} }, refreshRes.res);
    expect(refreshRes.body()).toEqual({ phase: 'starting' });
    expect(fake.refresh).toHaveBeenCalledTimes(1);

    const list = routes.get('/qqbot/bots');
    const listRes = makeRes();
    await list?.handler({ method: 'GET', headers: {} }, listRes.res);
    expect(listRes.body()).toEqual({ bots: [
      { id: 'newbie', label: '新机器人', appId: '', enabled: false },
    ] });
    disposeAll();
  });

  it('已绑定 bot 的刷新请求返回 409，不创建扫码服务', async () => {
    const { ctx, routes, disposeAll } = createCtx();
    const createService = vi.fn();
    registerBotCredentialRoutes(ctx, logger, [
      { id: 'cat', label: '猫猫', appId: 'cat-app', enabled: true, config: {} },
    ], { createService });

    const refresh = routes.get('/qqbot/bots/cat/bind/refresh');
    const res = makeRes();
    await refresh?.handler({ method: 'POST', headers: {} }, res.res);
    expect(res.status()).toBe(409);
    expect(createService).not.toHaveBeenCalled();
    disposeAll();
  });

  it('列表与绑定状态携带平台账号昵称（网关 READY 后）', async () => {
    const { ctx, routes, disposeAll } = createCtx();
    registerBotCredentialRoutes(ctx, logger, [
      {
        id: 'cat', label: '猫猫', appId: 'cat-app', enabled: true, config: {},
        accountName: () => '猫猫号',
      },
    ]);

    const list = routes.get('/qqbot/bots');
    const listRes = makeRes();
    await list?.handler({ method: 'GET', headers: {} }, listRes.res);
    expect(listRes.body()).toEqual({ bots: [
      { id: 'cat', label: '猫猫', appId: 'cat-app', enabled: true, name: '猫猫号' },
    ] });

    const state = routes.get('/qqbot/bots/cat/bind');
    const stateRes = makeRes();
    await state?.handler({ method: 'GET', headers: {} }, stateRes.res);
    expect(stateRes.body()).toEqual({ phase: 'bound', appId: 'cat-app', name: '猫猫号' });
    disposeAll();
  });

  it('已停用/凭据未加载时绑定状态报 failed 并给原因', async () => {
    const { ctx, routes, disposeAll } = createCtx();
    registerBotCredentialRoutes(ctx, logger, [
      { id: 'cat', label: '猫猫', appId: 'cat-app', enabled: false, config: {} },
    ]);
    const state = routes.get('/qqbot/bots/cat/bind');
    const res = makeRes();
    await state?.handler({ method: 'GET', headers: {} }, res.res);
    expect(res.body()).toMatchObject({ phase: 'failed' });
    expect(String((res.body() as { reason?: string }).reason)).toContain('凭据');
    disposeAll();
  });

  it('列表路由支持新增/删除回调（POST 请求体解析）', async () => {
    const routes = new Map<string, { path: string; handler: (req: unknown, res: unknown) => unknown }>();
    const webServer = {
      register: (route: { path: string; handler: (req: unknown, res: unknown) => unknown }) => {
        routes.set(route.path, route);
        return () => routes.delete(route.path);
      },
    };
    const created = vi.fn((id: string, label?: string) => ({ ok: true, id, label }));
    const deleted = vi.fn((id: string) => ({ ok: true, id }));
    registerBotsListRoute(webServer as never, logger, () => ({ bots: [] }), { onCreate: created, onDelete: deleted });

    const reqWithBody = (method: string, body: unknown) => ({
      method,
      headers: {},
      async *[Symbol.asyncIterator]() { yield Buffer.from(JSON.stringify(body)); },
    });

    const create = routes.get('/qqbot/bots');
    const createRes = makeRes();
    await create?.handler(reqWithBody('POST', { id: 'cat', label: '猫猫' }), createRes.res);
    expect(created).toHaveBeenCalledWith('cat', '猫猫');
    expect(createRes.body()).toMatchObject({ ok: true });

    const del = routes.get('/qqbot/bots/delete');
    const delRes = makeRes();
    await del?.handler(reqWithBody('POST', { id: 'cat' }), delRes.res);
    expect(deleted).toHaveBeenCalledWith('cat');
    expect(delRes.body()).toMatchObject({ ok: true });
  });

  it('无 webServer 时安静跳过', () => {
    const ctx = { inject: (_deps: readonly string[], callback: (scoped: unknown) => void) => callback({}) } as unknown as Context;
    expect(() => registerBotCredentialRoutes(ctx, logger, [{ id: 'cat', appId: 'a', enabled: true, config: {} }])).not.toThrow();
  });
});
