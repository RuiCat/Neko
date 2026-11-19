/**
 * astrbox 桥接服务（`astrboxBridge`）的可选只读访问。
 *
 * 契约 K2：该 cordis 服务由 dsh-astrbox 桥接插件提供，本插件只用 `ctx.get()`
 * 惰性读取（**不得**进入静态 `inject`）：服务缺失时插件照常工作，
 * 实体目录为空、人设段为空。
 */
import type { Context } from '@deepseek-ai/cordis';

/** 桥接服务的最小投影（K2 v1；未声明字段不读取）。 */
export interface AstrboxBridgeLike {
  /** 当前 NPC 实体 id 列表（未就绪返回空数组，永不抛错） */
  entities(): ReadonlyArray<string>;
  /** 指定实体的 role-card 人设文本（同步；未就绪返回空串，永不抛错） */
  persona(entityId: string): string;
  /**
   * K28：指定实体的主会话提示词（同步；未配置返回空串，永不抛错）。
   * 旧版桥接没有该字段，调用方必须按可选处理。
   */
  mainSessionPrompt?(entityId: string): string;
  /** 重拉实体列表并预取当前绑定实体人设（失败静默） */
  refresh?(): Promise<void>;
  /**
   * K33：确保实体存在（不存在则由控制面按模板创建）；旧版桥接没有该字段，
   * 调用方必须按可选处理。失败会 reject，由调用方决定是否降级。
   */
  ensureEntity?(entityId: string, type?: string): Promise<{ created?: boolean } | undefined>;
}

/** 取插件上下文上的可选服务读取函数（cordis Context.get）。 */
function readService(ctx: Context, name: string): unknown {
  try {
    const accessor = (ctx as unknown as { get?: (key: string) => unknown }).get;
    return typeof accessor === 'function' ? accessor.call(ctx, name) : undefined;
  } catch {
    return undefined;
  }
}

/**
 * 惰性读取桥接服务；服务缺失或形状不符时返回 undefined。
 * @param ctx - 插件（或任意可访问根服务的）cordis 上下文
 * @returns 桥接服务投影，或 undefined
 */
export function getAstrboxBridge(ctx: Context): AstrboxBridgeLike | undefined {
  const bridge = readService(ctx, 'astrboxBridge');
  if (typeof bridge !== 'object' || bridge === null) return undefined;
  const candidate = bridge as Partial<AstrboxBridgeLike>;
  if (typeof candidate.entities !== 'function' || typeof candidate.persona !== 'function') return undefined;
  return candidate as AstrboxBridgeLike;
}

/**
 * 读取 NPC 实体目录（`/qqbot/bots` 的 `entities` 字段）。
 * 服务不可用或返回非字符串项时返回 undefined / 过滤后的数组（K8：不可用即省略字段）。
 * @param ctx - 插件上下文
 * @returns 实体 id 数组；服务不可用时 undefined
 */
export function readBridgeEntities(ctx: Context): string[] | undefined {
  const bridge = getAstrboxBridge(ctx);
  if (bridge === undefined) return undefined;
  try {
    const list = bridge.entities();
    if (!Array.isArray(list)) return undefined;
    return list.filter((entity): entity is string => typeof entity === 'string' && entity !== '');
  } catch {
    return undefined;
  }
}

/**
 * 人设读取结果：`text` 恒为字符串；`ready=false` 表示实体已绑定但桥接还没备好人设
 * （重启/冷启动首轮竞态、服务未就绪或实体无可用人设字段），调用方可据此记录一次性 warn。
 */
export interface BridgePersonaStatus {
  /** 人设 markdown；未就绪时为空串 */
  text: string
  /** 实体已绑定且当前有非空人设 */
  ready: boolean
}

/**
 * 读取指定实体的 role-card 人设文本并附带就绪状态（K7：同步、永不抛错）。
 * @param ctx - 插件上下文
 * @param entityId - 配置里绑定的实体 id（未绑定传 undefined）
 * @returns 人设文本与就绪标记；未绑定/服务缺失/异常时 `{text:'', ready:false}`
 */
export function readBridgePersonaStatus(ctx: Context, entityId: string | undefined): BridgePersonaStatus {
  if (entityId === undefined || entityId === '') return { text: '', ready: false };
  const bridge = getAstrboxBridge(ctx);
  if (bridge === undefined) return { text: '', ready: false };
  try {
    const text = bridge.persona(entityId);
    const safe = typeof text === 'string' ? text : '';
    return { text: safe, ready: safe !== '' };
  } catch {
    return { text: '', ready: false };
  }
}

/**
 * 读取指定实体的人设卡文本（K7：同步、永不抛错、空实体/无服务返回空串）。
 * @param ctx - 插件上下文
 * @param entityId - 配置里绑定的实体 id（未绑定传 undefined）
 * @returns 人设 markdown；未绑定/服务缺失/异常时为空串
 */
export function readBridgePersona(ctx: Context, entityId: string | undefined): string {
  return readBridgePersonaStatus(ctx, entityId).text;
}

/**
 * 读取指定实体的主会话提示词（K28：同步、永不抛错、空实体/无服务/旧桥接返回空串）。
 * @param ctx - 插件上下文
 * @param entityId - 配置里绑定的实体 id（未绑定传 undefined）
 * @returns 提示词文本；未配置/服务缺失/异常时为空串
 */
export function readBridgeMainSessionPrompt(ctx: Context, entityId: string | undefined): string {
  const id = typeof entityId === 'string' ? entityId.trim() : '';
  if (id === '') return '';
  const bridge = getAstrboxBridge(ctx);
  if (bridge === undefined) return '';
  const read = bridge.mainSessionPrompt;
  if (typeof read !== 'function') return '';
  try {
    const text = read.call(bridge, id);
    return typeof text === 'string' ? text.trim() : '';
  } catch {
    return '';
  }
}

/**
 * 触发桥接预热（fire-and-forget，K3：运行时启动后调用；失败静默）。
 * @param ctx - 插件上下文
 * @returns void（不等待、不抛错）
 */
export function refreshBridge(ctx: Context): void {
  const bridge = getAstrboxBridge(ctx);
  if (bridge === undefined || typeof bridge.refresh !== 'function') return;
  try {
    void Promise.resolve(bridge.refresh()).catch(() => {});
  } catch {
    // 同步抛错同样静默：预热失败不影响 QQ 收发
  }
}

/**
 * 确保绑定实体存在（K33）：不存在时由控制面按模板创建；服务缺失/旧版桥接/失败
 * 一律静默降级，返回是否本次创建。
 * @param ctx - 插件上下文
 * @param entityId - 绑定的实体 id（空串/未绑定直接跳过）
 * @param type - 实体类型（必填；实体名称全局唯一，但类型由内容层显式给出）
 * @param logger - 可选的告警出口（失败时记一条）
 * @returns 本次是否创建了新实体
 */
export async function ensureBridgeEntity(
  ctx: Context,
  entityId: string | undefined,
  type: string,
  logger?: { warn?: (message: string) => void },
): Promise<boolean> {
  const id = typeof entityId === 'string' ? entityId.trim() : '';
  const typeName = typeof type === 'string' ? type.trim() : '';
  // 实体名称唯一，但类型不设缺省：没有显式类型就不创建，交给内容层。
  if (id === '' || typeName === '') return false;
  const bridge = getAstrboxBridge(ctx);
  if (bridge === undefined || typeof bridge.ensureEntity !== 'function') return false;
  try {
    const result = await bridge.ensureEntity(id, typeName);
    return result?.created === true;
  } catch (error) {
    logger?.warn?.(`im-qqbot: 实体 ${id} 自动创建失败（继续启动）: ${error instanceof Error ? error.message : String(error)}`);
    return false;
  }
}
