/**
 * 宿主 API 适配层 — 收口所有版本漂移点，一次探测按需复用。
 *
 * 宿主新旧版本 API 形态不同（本插件踩过的漂移：events → snapshotEvents、
 * presets.serviceFor 实例方法 → serviceForAgent 模块函数、./fork 子路径导出）。
 * 调用方只依赖本模块的稳定签名，不再直接假设宿主形态。
 */
import type { Context } from '@deepseek-ai/cordis';
import { shouldApplySessionTitle, type SessionTitleSnapshotLike } from '../shared/sessionTitles.ts';

/** 日志最小接口 */
export interface AdapterLogger {
  info(msg: string): void;
  warn(msg: string): void;
  debug?(msg: string): void;
}

export interface HostAdapter {
  /** 会话事件读取：新宿主 snapshotEvents()，旧宿主 events getter */
  sessionEvents(session: { snapshotEvents?: () => readonly unknown[]; events?: readonly unknown[] }): readonly unknown[];
  /** preset 作用域服务解析：旧宿主实例方法 / 新宿主模块函数 */
  serviceFor(agent: { ctx: unknown }, name: string): Promise<unknown | undefined>;
  /** 全局服务读取（未注册返回 undefined，不抛） */
  globalService(name: string): unknown | undefined;
  /** 官方 fork seed 构造；旧宿主无导出时返回 undefined */
  buildForkSeed(events: readonly unknown[], boundary: number): Promise<readonly unknown[] | undefined>;
  /** 会话 header 读取（取 agentPreset 等）；会话不存在、服务缺失或读取失败返回 undefined */
  sessionHeader(sessionId: string): Promise<SessionHeaderLike | undefined>;
  /**
   * 会话是否存在于持久化层。
   *
   * `undefined` = 没有求证手段（宿主未注册 sessionPersistence，或其形态不含
   * `stat`）。调用方不得把它当成「不存在」——那正是重建已存在会话、撞出重复
   * id 的根因。后端真故障时照常抛出。
   */
  sessionExists(sessionId: string): Promise<boolean | undefined>;
  /**
   * 会话是否处于「已归档」状态（workspaceRegistry.archivedSessionIds）。
   *
   * 归档会话的 agent/pre-step 会被宿主 archived-session-gate 拒绝（turn 以
   * `blocked` 结束），因此调用方**不得**再 resume 它，而应铸造新会话。
   * 服务缺失/形态不符一律返回 false（fail-soft，等同未知），绝不抛出。
   */
  isSessionArchived(sessionId: string): boolean;
  /**
   * 将会话挂载到其 cwd 对应的工作区（侧边栏可见性），并确保工作区显示名。
   *
   * 全部 fail-soft：workspaceRegistry 未挂载（如 headless profile）→ debug；
   * create/attach/setTitle 失败或宿主接口演进（无 attachSession）→ warn，绝不抛出。
   * `attachSession` 幂等，创建与恢复路径都可安全调用；升级后老会话自动补挂。
   *
   * @param title - 期望的工作区显示名；缺省沿用插件旧默认名 `QQBot`。
   *   仅当现有工作区名仍是旧默认名时才改名（避免覆盖用户自定义名）。
   */
  attachSessionToWorkspace(sessionId: string, cwd: string, title?: string): Promise<void>;
  /**
   * 确保会话显示标题（工作区用）：无标题/自动标题/本插件命名的标题会被写成
   * 指定的 user-owned 标题（pins，停止自动标题）；用户手动改过的其它标题保持不动。
   *
   * 全部 fail-soft：宿主未挂载 sessionTitle 服务、会话未 live、rename 抛错都只 debug/warn。
   * @param agent - 会话所属的 agent（取其 session 对象）
   * @param title - 目标标题（如 `主体` / `私聊(85D47757)`）
   */
  ensureSessionTitle(agent: unknown, title: string): void;
  /** 探测结果摘要（启动契约检查用） */
  readonly capabilities: string[];
}

/** 会话 header 的最小结构（只读插件关心的字段；`agentPreset` 在 header 顶层） */
export interface SessionHeaderLike {
  agentPreset?: string;
  /** 会话来源：qqbot 旧版把 QQ 会话建成 subagent 子会话（首页隐藏），需迁移 */
  origin?: string;
  /** 子会话的父指针（仅诊断/迁移用） */
  parentSession?: string;
}

/** 工作区实体最小契约（对齐 dsh-workspace Workspace） */
export interface WorkspaceLike {
  readonly id?: string;
  readonly path?: string;
  /** 显示名（dsh-workspace 的 Workspace.title） */
  readonly title?: string;
  /** 改名（dsh-workspace 的 Workspace.setTitle）；旧宿主可能缺失 */
  setTitle?(title: string): Promise<void>;
  attachSession(sessionId: string): Promise<void>;
}

/** workspaceRegistry 服务最小契约（对齐 dsh-workspace WorkspaceRegistry） */
export interface WorkspaceRegistryLike {
  /** 获取目录对应的现有工作区；不存在时创建（幂等；路径须已存在且为目录；title 仅新建时生效） */
  create(path: string, title?: string): Promise<WorkspaceLike>;
}

/** 插件创建的工作区显示名；仅在该 cwd 尚无工作区时生效（幂等 create 不改既有 title） */
const WORKSPACE_TITLE = 'QQBot';

/**
 * 持久化服务的最小只读契约。
 *
 * 只用 `stat`：它不读事件日志、不取写所有权，且**会话不存在时返回 `undefined`
 * 而非抛错**，正好是存在性判定需要的语义。不用 `open`（会争写所有权）、不用
 * `list`（全量扫描）。
 */
interface SessionPersistenceLike {
  stat(id: string): Promise<{ header?: SessionHeaderLike } | undefined>;
}

/**
 * 取出可用的持久化服务；形态不匹配返回 undefined。
 *
 * 宿主服务是版本漂移点，必须探测方法是否存在再调用：直接调用缺失的方法会抛
 * `TypeError`，与「后端故障」混在一起，让调用方误以为求证过了。
 */
function statCapablePersistence(ctx: Context): SessionPersistenceLike | undefined {
  const persistence = safeGet(ctx, 'sessionPersistence') as SessionPersistenceLike | undefined;
  return typeof persistence?.stat === 'function' ? persistence : undefined;
}

/** 取出可用的 workspaceRegistry；形态不匹配返回 undefined（dsh-workspace 未挂载时无侧边栏） */
function workspaceRegistry(ctx: Context): WorkspaceRegistryLike | undefined {
  const registry = safeGet(ctx, 'workspaceRegistry') as WorkspaceRegistryLike | undefined;
  return typeof registry?.create === 'function' ? registry : undefined;
}

/** 取归档会话集合（读 workspaceRegistry.archivedSessionIds；缺失/异常返回空集） */
function archivedSessionSet(ctx: Context): ReadonlySet<string> {
  try {
    const registry = safeGet(ctx, 'workspaceRegistry') as { archivedSessionIds?: readonly string[] } | undefined;
    const ids = registry?.archivedSessionIds;
    return new Set(Array.isArray(ids) ? ids.filter((id): id is string => typeof id === 'string') : []);
  } catch {
    return new Set();
  }
}

/** 模块级函数的动态 import 结果缓存（spec 变量路径绕开本地依赖类型检查） */
const moduleCache = new Map<string, Record<string, unknown> | undefined>();

async function importModule(spec: string, logger?: AdapterLogger): Promise<Record<string, unknown> | undefined> {
  if (moduleCache.has(spec)) return moduleCache.get(spec);
  let mod: Record<string, unknown> | undefined;
  try {
    mod = (await import(spec)) as Record<string, unknown>;
  } catch {
    mod = undefined;
    logger?.warn(`host adapter: module not resolvable: ${spec}`);
  }
  moduleCache.set(spec, mod);
  return mod;
}

export function createHostAdapter(ctx: Context, logger?: AdapterLogger): HostAdapter {
  const capabilities: string[] = [];

  // events 读取路径只有运行时才能确定，sessionEvents 内自适应
  capabilities.push('sessionEvents(s自适应)');

  // 可能在插件启动后才注册，sessionExists/sessionHeader 内每次重新探测；此处仅记录启动时形态
  capabilities.push(statCapablePersistence(ctx)
    ? 'sessionPersistence.stat(可用)'
    : 'sessionPersistence.stat(不可用，会话存在性判定降级)');
  capabilities.push(workspaceRegistry(ctx)
    ? 'workspaceRegistry.create(可用)'
    : 'workspaceRegistry.create(不可用，侧边栏工作区归属降级)');
  capabilities.push('workspaceRegistry.archivedSessionIds(归档检测)');
  capabilities.push('sessionTitle.rename(会话标题命名)');

  let resolveFn: ((agent: { ctx: unknown }, name: string) => unknown | undefined) | undefined;
  const presets = safeGet(ctx, 'agentPresets') as
    | { serviceFor?: (agent: unknown, name: string) => unknown }
    | undefined;
  if (presets && typeof presets.serviceFor === 'function') {
    resolveFn = (agent, name) => presets.serviceFor!(agent, name);
    capabilities.push('serviceFor(presets实例方法)');
  } else {
    capabilities.push('serviceFor(serviceForAgent动态)');
  }

  return {
    capabilities,

    sessionEvents(session) {
      if (typeof session.snapshotEvents === 'function') return session.snapshotEvents();
      const legacy = session.events;
      return Array.isArray(legacy) ? legacy : [];
    },

    async serviceFor(agent, name) {
      if (resolveFn) {
        try {
          const found = resolveFn(agent, name);
          if (found) return found;
        } catch { /* 宿主实现差异，走模块函数 */ }
      }
      const mod = await importModule('@deepseek-ai/dsh-agent-presets', logger);
      const fn = mod?.serviceForAgent as
        | ((ctx: unknown, agent: { ctx: unknown }, name: string) => unknown)
        | undefined;
      if (typeof fn !== 'function') return undefined;
      try {
        return fn(ctx, agent, name);
      } catch {
        return undefined;
      }
    },

    globalService(name) {
      return safeGet(ctx, name);
    },

    async sessionHeader(sessionId) {
      const persistence = statCapablePersistence(ctx);
      if (!persistence) return undefined;
      try {
        return (await persistence.stat(sessionId))?.header;
      } catch {
        return undefined; // 读不到 preset 不该阻断会话打开，回落到当前 effective preset
      }
    },

    ensureSessionTitle(agent, title) {
      const session = (agent as { session?: unknown } | undefined)?.session;
      if (session === undefined || session === null || typeof title !== 'string' || title.trim() === '') return;
      const service = safeGet(ctx, 'sessionTitle') as
        | {
          get?: (target: unknown) => SessionTitleSnapshotLike | undefined;
          rename?: (target: unknown, next: string) => unknown;
        }
        | undefined;
      if (typeof service?.rename !== 'function') return; // 宿主未挂载标题服务：保持默认标题
      let current: SessionTitleSnapshotLike | undefined;
      try {
        current = typeof service.get === 'function' ? service.get(session) : undefined;
      } catch {
        current = undefined;
      }
      if (!shouldApplySessionTitle(current)) return;
      try {
        void service.rename(session, title.trim());
      } catch (err) {
        logger?.warn?.(`host adapter: 会话标题写入失败（保持原样）: ${err instanceof Error ? err.message : String(err)}`);
      }
    },

    isSessionArchived(sessionId) {
      try {
        return archivedSessionSet(ctx).has(sessionId);
      } catch {
        return false; // 判定失败按未归档处理：宁可 resume 被宿主 gate 拒绝，也不误删用户会话
      }
    },

    async sessionExists(sessionId) {
      const persistence = statCapablePersistence(ctx);
      if (!persistence) return undefined;
      return (await persistence.stat(sessionId)) !== undefined;
    },

    async attachSessionToWorkspace(sessionId, cwd, title) {
      const registry = workspaceRegistry(ctx);
      if (!registry) {
        // 未挂载 dsh-workspace（如 headless profile）→ 无侧边栏，属预期降级
        logger?.debug?.(`host adapter: workspaceRegistry 未挂载，跳过会话 ${sessionId} 的工作区归属`);
        return;
      }
      const wanted = typeof title === 'string' && title.trim() !== '' ? title.trim() : WORKSPACE_TITLE;
      try {
        const workspace = await registry.create(cwd, wanted);
        if (!workspace || typeof workspace.attachSession !== 'function') {
          // 宿主接口演进：返回对象不含 attachSession → warn 避免静默失效
          logger?.warn?.(`host adapter: workspaceRegistry.create 返回了无 attachSession 的对象，跳过会话 ${sessionId}`);
          return;
        }
        // 已存在工作区的 create 幂等且不改名：把插件旧默认名迁移到机器人名，
        // 但用户自定义过的名字保持不动（只认 WORKSPACE_TITLE 是「我们建的」）。
        if (workspace.title === WORKSPACE_TITLE && wanted !== WORKSPACE_TITLE) {
          if (typeof workspace.setTitle === 'function') {
            try {
              await workspace.setTitle(wanted);
              logger?.info?.(`host adapter: 工作区已改名 ${workspace.path ?? cwd} → ${wanted}`);
            } catch (err) {
              logger?.warn?.(`host adapter: 工作区改名失败（保持原名）: ${err instanceof Error ? err.message : String(err)}`);
            }
          }
        }
        await workspace.attachSession(sessionId);
        logger?.debug?.(`host adapter: attached session ${sessionId} to workspace ${workspace.path ?? cwd}`);
      } catch (err) {
        // 仅影响侧边栏展示，绝不打断消息处理；但属异常（cwd 不匹配/目录缺失等），warn 留痕
        logger?.warn?.(`host adapter: workspace attach failed for ${sessionId}: ${err instanceof Error ? err.message : String(err)}`);
      }
    },

    async buildForkSeed(events, boundary) {
      const mod = await importModule('@deepseek-ai/dsh-session/fork', logger);
      const fn = mod?.buildForkSeed as
        | ((events: readonly unknown[], boundary: number) => readonly unknown[])
        | undefined;
      if (typeof fn !== 'function') return undefined;
      try {
        return fn(events, boundary);
      } catch {
        return undefined;
      }
    },
  };
}

/** ctx.get 的非抛出版本 */
function safeGet(ctx: Context, name: string): unknown {
  try {
    return ctx.get(name) as unknown;
  } catch {
    return undefined;
  }
}

/** 测试辅助：清空模块缓存 */
export function _resetAdapterCache(): void {
  moduleCache.clear();
}
