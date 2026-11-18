/**
 * 端内绑定页路由 — /qqbot/im-qqbot/bind
 *
 * 注册在 dsh-host-webserver（ctx.webServer）的具名路由：
 * - Web 模式：页面同源直接访问
 * - Desktop：渲染进程的 fetch 经 IPC 桥自动附加 Host 认证转发
 *
 * 响应只含 task 级短时效数据（二维码 URL/SVG）与脱敏的 appId；
 * appSecret 绝不出现在任何响应中。manual 提交走与扫码一致的
 * persist 路径（写入 profile patch → HMR 重载生效）。
 */
import type { IncomingMessage, ServerResponse } from 'node:http';
import type { Logger } from '../types.ts';
import type { BindingService } from './binding-service.ts';
import {
  persistCredentialsToProfile,
  removeCredentialsFromProfile,
  updateProfileConfig,
  type SetupCredentials,
} from './persist.ts';
import { getProfileDir } from '../shared/index.ts';
import { AccessControlSchema } from '../config.ts';
import { BOT_ID_PATTERN } from '../configBots.ts';
import { normalizeConversationPrompts } from '../shared/conversationPrompts.ts';
import { normalizeOwner } from '../shared/owner.ts';
import {
  BIND_CONFIG_PATH,
  BIND_MANUAL_PATH,
  BIND_REFRESH_PATH,
  BIND_STATE_PATH,
  BIND_UNBIND_PATH,
  BOTS_DELETE_PATH,
  BOTS_LIST_PATH,
  type BindRoutePaths,
  type BindStateWire,
  type BotsListWire,
} from '../shared/bind-protocol.ts';

// 协议单一来源：shared/bind-protocol.ts（类型 + 路径常量，客户端共用）
export { BIND_CONFIG_PATH, BIND_MANUAL_PATH, BIND_REFRESH_PATH, BIND_STATE_PATH, BIND_UNBIND_PATH };
export type { BindStateWire };

/** dsh-host-webserver 的具名路由注册形状（局部投影，避免硬依赖） */
export interface WebRoute {
  kind: 'exact' | 'prefix';
  path: string;
  handler: (req: IncomingMessage, res: ServerResponse) => void | Promise<void>;
}

/** ctx.webServer 的局部投影（服务可用性由调用方探测） */
export interface WebServerLike {
  register(route: WebRoute): () => void;
}

/** 手动凭据提交的处理结果 */
export interface ManualSubmitResult {
  /** 是否已持久化（false = 落盘失败，需展示手动配置指引） */
  persisted: boolean;
  /** 失败原因（persisted=false 时回传给表单展示） */
  error?: string;
}

/** 解绑处理结果 */
export interface UnbindResult {
  persisted: boolean;
  error?: string;
}

/** 路由行为注入 */
export interface BindRoutesOptions {
  /**
   * 已绑定态：返回脱敏的 appId（可回显，非机密），null 表示未绑定。
   * 已绑定 fiber 注册路由时提供；待绑定流程不提供。
   */
  boundAppId?: () => string | null;
  /**
   * 手动凭据提交处理（默认：persist 到 profile patch）。
   * 待绑定流程会注入「先终止扫码轮询」的包装；写凭据库等异步路径可返回 Promise。
   */
  onManualCredentials?: (credentials: SetupCredentials) => ManualSubmitResult | Promise<ManualSubmitResult>;
  /**
   * 解绑处理（默认：移除 profile patch 中的凭据字段）。
   * 未绑定态调用返回 409，不触达本回调；清理凭据库可返回 Promise。
   */
  onUnbind?: () => UnbindResult | Promise<UnbindResult>;
  /**
   * 当前生效配置（apply 时 loader 合并后的值，含默认值填充），
   * 用于 GET 回显（appSecret 剔除后返回）。
   */
  currentConfig?: () => Record<string, unknown>;
  /**
   * 配置写入处理（默认：最小合并写入 profile patch）。
   */
  onUpdateConfig?: (config: Record<string, unknown>) => boolean | Promise<boolean>;
  /** 该组路由的路径（默认旧单 bot 固定路径；多 bot 传 botBindPaths(botId)） */
  paths?: BindRoutePaths;
  /** 平台账号昵称（网关 READY 后可用；未知时 undefined） */
  accountName?: () => string | undefined;
  /** 不可用原因（如 appId 在但凭据未加载/已停用）：返回时状态直接报 failed */
  unavailableReason?: () => string | undefined;
}

/**
 * 扫码状态机来源：旧单 bot 传实例；多 bot / 未绑定 bot 传惰性工厂
 * （首个状态轮询才创建服务，避免已绑定 bot 也起二维码轮询）。
 */
export type BindingServiceSource = BindingService | null | (() => BindingService | null);

/** 旧单 bot 固定路径（默认）。 */
const LEGACY_PATHS: BindRoutePaths = {
  state: BIND_STATE_PATH,
  refresh: BIND_REFRESH_PATH,
  manual: BIND_MANUAL_PATH,
  unbind: BIND_UNBIND_PATH,
  config: BIND_CONFIG_PATH,
};

/** 可经 UI 编辑的配置键（白名单：只收用户必要的字段，其余走 patch/env） */
const EDITABLE_CONFIG_KEYS = new Set([
  'requireMention', 'streaming', 'showToolResults', 'debug',
  'provider', 'model', 'preset',
  // entity 空串 = 解绑（不绑定任何 NPC 实体）；非空即绑定
  'entity',
  'groupPrompt', 'directPrompt',
  // K26：单会话独立提示词（数组整体替换；空数组 = 清空会话级提示词）
  'conversationPrompts',
  // K27：主人身份（对象整体替换；null = 清空）
  'owner',
  'access',
]);

/** 白名单键的子 Schema（服务端校验：客户端校验仅为体验，安全边界在宿主） */
const EDITABLE_CONFIG_SCHEMAS: Record<string, (value: unknown) => unknown> = {
  requireMention: (v) => booleanSchema(v, 'requireMention'),
  streaming: (v) => booleanSchema(v, 'streaming'),
  showToolResults: (v) => booleanSchema(v, 'showToolResults'),
  debug: (v) => booleanSchema(v, 'debug'),
  provider: stringSchema('provider'),
  model: stringSchema('model'),
  preset: stringSchema('preset'),
  entity: stringSchema('entity'),
  groupPrompt: stringSchema('groupPrompt'),
  directPrompt: stringSchema('directPrompt'),
  // K26：逐条校验（scope 枚举/非空/≤80000/同列表 (scope,peerId) 不重复），错误信息直接回 400。
  conversationPrompts: (v) => normalizeConversationPrompts(v, 'conversationPrompts'),
  // K27：null = 清空；对象走统一归一化（openid 非空/name 默认「主人」≤64），非法直接 400。
  owner: (v) => normalizeOwner(v, 'owner') ?? null,
  access: (v) => AccessControlSchema(v as never),
};

function booleanSchema(value: unknown, name: string): boolean {
  if (typeof value !== 'boolean') throw new TypeError(`${name} 必须是布尔值`);
  return value;
}

function stringSchema(name: string): (value: unknown) => unknown {
  return (value) => {
    if (typeof value !== 'string') throw new TypeError(`${name} 必须是字符串`);
    if (value.length > 20000) throw new TypeError(`${name} 过长`);
    return value;
  };
}

/** manual 请求体大小上限（appId + appSecret 远小于此值） */
const MAX_MANUAL_BODY_BYTES = 4096;

/**
 * config 请求体大小上限：对齐 Schema 的业务边界——白名单字符串字段单个
 * 允许 20000 字符（全中文 UTF-8 约 60KB）；K26 的 conversationPrompts 每条
 * 允许 80000 字符（全中文 UTF-8 约 240KB），故放宽到 2MB（≈8 条满长提示词）。
 * allowlist 不设条目上限（openid 短标识）。上限只是反滥用闸门，语义校验在 Schema。
 */
const MAX_CONFIG_BODY_BYTES = 2 * 1024 * 1024;

/**
 * 注册多机器人列表路由（GET /qqbot/bots）。
 * 仅返回 id/label/appId/enabled，不返回凭据；与 per-bot 路由分开注册，避免每 bot 重复。
 * @param webServer - ctx.webServer
 * @param logger - 日志
 * @param getBots - 返回当前机器人列表（由调用方按启动结果提供）
 * @returns 注销函数
 */
// 列表端点：只读、脱敏（appId 非机密，可回显；appSecret 永不出现）。
/** 列表端点的写操作（新增/删除机器人条目；不触发凭据回显）。 */
export interface BotsAdminHandlers {
  onCreate?: (id: string, label?: string) => { ok: boolean; error?: string } | Promise<{ ok: boolean; error?: string }>;
  onDelete?: (id: string) => { ok: boolean; error?: string };
}

/** 解析并校验「新增机器人」请求体。 */
function parseCreateBody(body: unknown): { id: string; label?: string } {
  if (typeof body !== 'object' || body === null) throw new Error('请求体必须是对象');
  const raw = body as { id?: unknown; label?: unknown };
  if (typeof raw.id !== 'string' || !BOT_ID_PATTERN.test(raw.id.trim())) {
    throw new Error('id 是内部标识（路由/目录/工具归属），需匹配 [a-z0-9][a-z0-9-]{1,31}；展示名可用中文');
  }
  if (raw.id.trim() === 'default') {
    throw new Error('id "default" 保留给旧单 bot 迁移，请换一个');
  }
  if (raw.label !== undefined && typeof raw.label !== 'string') throw new Error('label 必须是字符串');
  if (typeof raw.label === 'string' && raw.label.length > 64) throw new Error('label 过长');
  const label = typeof raw.label === 'string' ? raw.label.trim() : undefined;
  return { id: raw.id.trim(), ...(label === undefined || label === '' ? {} : { label }) };
}

/** 解析删除请求体。 */
function parseDeleteBody(body: unknown): string {
  if (typeof body !== 'object' || body === null) throw new Error('请求体必须是对象');
  const raw = (body as { id?: unknown }).id;
  if (typeof raw !== 'string' || raw.trim() === '') throw new Error('id 不能为空');
  return raw.trim();
}

export function registerBotsListRoute(
  webServer: WebServerLike,
  logger: Logger,
  getBots: () => BotsListWire,
  handlers: BotsAdminHandlers = {},
): () => void {
  const disposers: Array<() => void> = [];
  disposers.push(webServer.register({
    kind: 'exact',
    path: BOTS_LIST_PATH,
    handler: async (req, res) => {
      if (!trusted(req)) {
        sendJson(res, 403, { error: 'forbidden' });
        return;
      }
      if (req.method === 'GET' || req.method === 'HEAD') {
        sendJson(res, 200, getBots());
        return;
      }
      if (req.method !== 'POST' || handlers.onCreate === undefined) {
        sendJson(res, 405, { error: 'method not allowed' });
        return;
      }
      let body: unknown;
      try {
        body = await readJsonBody(req, MAX_MANUAL_BODY_BYTES);
      } catch (err) {
        sendJson(res, 400, { ok: false, error: `invalid request body: ${err instanceof Error ? err.message : String(err)}` });
        return;
      }
      let parsed: { id: string; label?: string };
      try {
        parsed = parseCreateBody(body);
      } catch (err) {
        sendJson(res, 400, { ok: false, error: err instanceof Error ? err.message : String(err) });
        return;
      }
      const result = await handlers.onCreate(parsed.id, parsed.label);
      sendJson(res, result.ok ? 200 : 400, result);
    },
  }));
  const onDelete = handlers.onDelete;
  if (onDelete !== undefined) {
    disposers.push(webServer.register({
      kind: 'exact',
      path: BOTS_DELETE_PATH,
      handler: async (req, res) => {
        if (!trusted(req)) {
          sendJson(res, 403, { error: 'forbidden' });
          return;
        }
        if (req.method !== 'POST') {
          sendJson(res, 405, { error: 'method not allowed' });
          return;
        }
        let body: unknown;
        try {
          body = await readJsonBody(req, MAX_MANUAL_BODY_BYTES);
        } catch (err) {
          sendJson(res, 400, { ok: false, error: `invalid request body: ${err instanceof Error ? err.message : String(err)}` });
          return;
        }
        let id: string;
        try {
          id = parseDeleteBody(body);
        } catch (err) {
          sendJson(res, 400, { ok: false, error: err instanceof Error ? err.message : String(err) });
          return;
        }
        const result = onDelete(id);
        sendJson(res, result.ok ? 200 : 400, result);
      },
    }));
  }
  logger.info(`im-qqbot: 多机器人列表端点已就绪: ${BOTS_LIST_PATH}`);
  return () => { for (const dispose of disposers) dispose(); };
}

/**
 * 注册绑定路由（状态查询 / 刷新二维码 / 手动凭据）。
 *
 * @param webServer - ctx.webServer
 * @param service - 绑定状态机来源；已绑定 fiber 传 null，多 bot 传惰性工厂
 * @param logger - 日志
 * @param options - 行为注入（bound 态回显、manual 处理）
 * @returns 注销函数（与插件 fiber 生命周期绑定）
 */
export function registerBindRoutes(
  webServer: WebServerLike,
  service: BindingServiceSource,
  logger: Logger,
  options: BindRoutesOptions = {},
): () => void {
  const defaultManual = (credentials: SetupCredentials): ManualSubmitResult => {
    const persisted = persistCredentialsToProfile(credentials, getProfileDir() ?? undefined, logger);
    return persisted ? { persisted: true } : { persisted: false, error: '无法定位 profile 目录或写入失败，请改用环境变量配置' };
  };
  const handleManualSubmit = options.onManualCredentials ?? defaultManual;
  const resolveService = (): BindingService | null => (typeof service === 'function' ? service() : service);

  const paths = options.paths ?? LEGACY_PATHS;
  const disposers = [
    webServer.register({
      kind: 'exact',
      path: paths.state,
      handler: (req, res) => { handleState(req, res, resolveService, options); },
    }),
    webServer.register({
      kind: 'exact',
      path: paths.refresh,
      handler: (req, res) => { handleRefresh(req, res, resolveService, options); },
    }),
    webServer.register({
      kind: 'exact',
      path: paths.manual,
      handler: (req, res) => handleManual(req, res, handleManualSubmit),
    }),
    webServer.register({
      kind: 'exact',
      path: paths.unbind,
      handler: (req, res) => handleUnbind(req, res, options),
    }),
    webServer.register({
      kind: 'exact',
      path: paths.config,
      handler: (req, res) => handleConfig(req, res, options),
    }),
  ];
  logger.info(`im-qqbot: 端内绑定页已就绪: ${paths.state}`);
  return () => { for (const dispose of disposers) dispose(); };
}

/**
 * 请求来源校验（最小 CSRF 防护）。
 *
 * 带 Origin 的请求必须与 Host 同源；同源 fetch 与 Desktop IPC 桥
 * （后者会删除 origin 头）均放行。本路由数据不敏感且动作无害，
 * 该级别的防护足够。
 */
function trusted(req: IncomingMessage): boolean {
  const origin = req.headers.origin;
  if (origin === undefined) return true;
  try {
    return new URL(origin).host === req.headers.host;
  } catch {
    return false;
  }
}

function sendJson(res: ServerResponse, status: number, body: unknown): void {
  const payload = JSON.stringify(body);
  res.writeHead(status, {
    'content-type': 'application/json; charset=utf-8',
    'cache-control': 'no-store',
  });
  res.end(payload);
}

function handleState(
  req: IncomingMessage,
  res: ServerResponse,
  resolveService: () => BindingService | null,
  options: BindRoutesOptions,
): void {
  if (!trusted(req)) {
    sendJson(res, 403, { error: 'forbidden' });
    return;
  }
  if (req.method !== 'GET' && req.method !== 'HEAD') {
    sendJson(res, 405, { error: 'method not allowed' });
    return;
  }
  // 已绑定优先（appSecret 不回传，appId 非机密可回显）
  const boundAppId = options.boundAppId?.() ?? null;
  const unavailable = options.unavailableReason?.();
  if (boundAppId !== null && unavailable !== undefined) {
    sendJson(res, 200, { phase: 'failed', reason: unavailable });
    return;
  }
  if (boundAppId !== null) {
    const name = options.accountName?.();
    sendJson(res, 200, name === undefined ? { phase: 'bound', appId: boundAppId } : { phase: 'bound', appId: boundAppId, name });
    return;
  }
  const service = resolveService();
  sendJson(res, 200, service === null ? { phase: 'starting' } : service.wireState());
}

function handleRefresh(
  req: IncomingMessage,
  res: ServerResponse,
  resolveService: () => BindingService | null,
  options: BindRoutesOptions,
): void {
  if (!trusted(req)) {
    sendJson(res, 403, { error: 'forbidden' });
    return;
  }
  if (req.method !== 'POST') {
    sendJson(res, 405, { error: 'method not allowed' });
    return;
  }
  // 已绑定 bot 不重建扫码流程（客户端只在未绑定面板提供刷新）
  if ((options.boundAppId?.() ?? null) !== null) {
    sendJson(res, 409, { ok: false, error: '已绑定，无需刷新二维码' });
    return;
  }
  // 丢弃可能的请求体（refresh 无参数）
  req.resume();
  resolveService()?.refresh();
  // 新二维码异步生成，统一由状态轮询收敛
  sendJson(res, 200, { phase: 'starting' });
}

/** 读取并解析 JSON 请求体（带大小上限） */
async function readJsonBody(req: IncomingMessage, maxBytes: number = MAX_MANUAL_BODY_BYTES): Promise<unknown> {
  const chunks: Buffer[] = [];
  let size = 0;
  for await (const chunk of req) {
    const buffer = chunk as Buffer;
    size += buffer.length;
    if (size > maxBytes) throw new Error('request body too large');
    chunks.push(buffer);
  }
  if (size === 0) throw new Error('empty request body');
  return JSON.parse(Buffer.concat(chunks).toString('utf8'));
}

/** 校验手动提交的凭据形态（不打印内容） */
function parseCredentials(body: unknown): SetupCredentials | null {
  if (typeof body !== 'object' || body === null) return null;
  const record = body as Record<string, unknown>;
  const appId = typeof record.appId === 'string' ? record.appId.trim() : '';
  const appSecret = typeof record.appSecret === 'string' ? record.appSecret.trim() : '';
  if (appId.length === 0 || appSecret.length === 0) return null;
  if (appId.length > 128 || appSecret.length > 256) return null;
  return { appId, appSecret };
}

async function handleManual(
  req: IncomingMessage,
  res: ServerResponse,
  submit: (credentials: SetupCredentials) => ManualSubmitResult | Promise<ManualSubmitResult>,
): Promise<void> {
  if (!trusted(req)) {
    sendJson(res, 403, { error: 'forbidden' });
    return;
  }
  if (req.method !== 'POST') {
    sendJson(res, 405, { error: 'method not allowed' });
    return;
  }
  let body: unknown;
  try {
    body = await readJsonBody(req);
  } catch (err) {
    sendJson(res, 400, { ok: false, error: `invalid request body: ${err instanceof Error ? err.message : String(err)}` });
    return;
  }
  const credentials = parseCredentials(body);
  if (credentials === null) {
    sendJson(res, 400, { ok: false, error: 'appId 与 appSecret 不能为空' });
    return;
  }
  const result = await submit(credentials);
  if (!result.persisted) {
    sendJson(res, 500, { ok: false, error: result.error ?? '保存失败' });
    return;
  }
  // 写入 profile patch 触发 HMR 重载，新 fiber 以新凭据启动网关
  sendJson(res, 200, { ok: true, message: '凭据已保存，插件将自动重载' });
}

/** 解绑：仅已绑定态可执行；移除 profile patch 中的凭据并触发 HMR 重载 */
async function handleUnbind(req: IncomingMessage, res: ServerResponse, options: BindRoutesOptions): Promise<void> {
  if (!trusted(req)) {
    sendJson(res, 403, { error: 'forbidden' });
    return;
  }
  if (req.method !== 'POST') {
    sendJson(res, 405, { error: 'method not allowed' });
    return;
  }
  req.resume();
  if ((options.boundAppId?.() ?? null) === null) {
    sendJson(res, 409, { ok: false, error: '当前未绑定' });
    return;
  }
  const result: UnbindResult = options.onUnbind === undefined
    ? (() => {
        const removed = removeCredentialsFromProfile(getProfileDir() ?? undefined, undefined);
        if (removed) {
          // 清除进程内凭据：扫码绑定成功时会写入 env（供热更新后的下次
          // apply 兜底读取）；不清则 HMR 重载后新 fiber 仍从 env 读到
          // 凭据，解绑不生效（状态一直停留在 bound）
          delete process.env.QQBOT_APPID;
          delete process.env.QQBOT_SECRET;
        }
        return removed
          ? { persisted: true }
          : { persisted: false, error: '无法定位 profile 目录或写入失败，请手动编辑 cordis.patch.yml' };
      })()
    : await options.onUnbind();
  if (!result.persisted) {
    sendJson(res, 500, { ok: false, error: result.error ?? '解绑失败' });
    return;
  }
  // 移除后写入触发 HMR 重载，插件回到未绑定态（绑定流程重新可用）
  sendJson(res, 200, { ok: true, message: '已解绑，插件将重载并重新进入绑定流程' });
}

/**
 * 配置读写：
 * - GET：回显当前生效配置（appSecret 剔除，cwd 等只读字段正常返回）
 * - POST：白名单 + 子 Schema 校验 → 最小写入 profile patch → HMR 重载
 */
async function handleConfig(req: IncomingMessage, res: ServerResponse, options: BindRoutesOptions): Promise<void> {
  if (!trusted(req)) {
    sendJson(res, 403, { error: 'forbidden' });
    return;
  }
  if (req.method !== 'GET' && req.method !== 'POST') {
    sendJson(res, 405, { error: 'method not allowed' });
    return;
  }

  if (req.method === 'GET') {
    const current = options.currentConfig?.() ?? {};
    // appSecret 永不回传；其余（含只读 cwd）原样回显
    const { appSecret: _s, ...safe } = current;
    sendJson(res, 200, { config: safe });
    return;
  }

  let body: unknown;
  try {
    body = await readJsonBody(req, MAX_CONFIG_BODY_BYTES);
  } catch (err) {
    sendJson(res, 400, { ok: false, error: `invalid request body: ${err instanceof Error ? err.message : String(err)}` });
    return;
  }
  if (typeof body !== 'object' || body === null) {
    sendJson(res, 400, { ok: false, error: '请求体必须是 JSON 对象' });
    return;
  }
  const submitted = body as Record<string, unknown>;

  // 白名单与凭据/只读字段拦截
  for (const key of Object.keys(submitted)) {
    if (!EDITABLE_CONFIG_KEYS.has(key)) {
      const reason = key === 'appId' || key === 'appSecret'
        ? '凭据请使用绑定/更换/解绑功能'
        : key === 'cwd'
          ? 'cwd 为只读字段'
          : '不支持的字段';
      sendJson(res, 400, { ok: false, error: `${reason}: ${key}` });
      return;
    }
  }

  // 逐键 Schema 校验（非法值拒绝，返回首个错误）
  let validated: Record<string, unknown> = {};
  try {
    validated = Object.fromEntries(Object.entries(submitted).map(([key, value]) => {
      const check = EDITABLE_CONFIG_SCHEMAS[key];
      if (check === undefined) throw new TypeError(`不支持的字段: ${key}`);
      return [key, check(value)];
    }));
  } catch (err) {
    sendJson(res, 400, { ok: false, error: err instanceof TypeError ? err.message : String(err) });
    return;
  }

  const persisted = options.onUpdateConfig === undefined
    ? updateProfileConfig(validated, getProfileDir() ?? undefined, undefined)
    : await options.onUpdateConfig(validated);
  if (!persisted) {
    sendJson(res, 500, { ok: false, error: '无法定位 profile 目录或写入失败' });
    return;
  }
  sendJson(res, 200, { ok: true, message: '配置已保存，插件将自动重载（进行中的机器人会话将重置）' });
}
