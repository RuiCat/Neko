/**
 * 端内绑定协议 —— 宿主路由与客户端面板的共享契约
 *
 * 类型与路径常量的唯一事实源：宿主（setup/bind-routes.ts）与浏览器
 * （client/bind-page.ts）各自 import，杜绝两端协议漂移。
 * 本模块必须保持纯类型 + 纯常量（零 node 依赖、零副作用）：
 * 客户端 tsdown 构建会将其内联进 bundle。
 */

/** 状态查询（绑定页轮询） */
export const BIND_STATE_PATH = '/qqbot/im-qqbot/bind';

/** 刷新二维码（绑定页按钮） */
export const BIND_REFRESH_PATH = '/qqbot/im-qqbot/bind/refresh';

/** 手动填写/编辑凭据（绑定页表单） */
export const BIND_MANUAL_PATH = '/qqbot/im-qqbot/bind/manual';

/** 解绑：移除已保存凭据，回到绑定流程 */
export const BIND_UNBIND_PATH = '/qqbot/im-qqbot/bind/unbind';

/** 配置读写（设置面板表单） */
export const BIND_CONFIG_PATH = '/qqbot/im-qqbot/config';

/** 多机器人路由前缀：/qqbot/bots/<botId>/... */
export const BOTS_PREFIX = '/qqbot/bots';

/** 一组绑定/配置路由路径（旧单 bot 固定路径与多 bot 参数化路径共用）。 */
export interface BindRoutePaths {
  state: string;
  refresh: string;
  manual: string;
  unbind: string;
  config: string;
}

/**
 * 按 bot id 生成一组路由路径。
 * @param botId - 配置里的稳定 bot id
 * @returns 该 bot 的绑定/配置路径
 */
export function botBindPaths(botId: string): BindRoutePaths {
  const base = `${BOTS_PREFIX}/${encodeURIComponent(botId)}`;
  return {
    state: `${base}/bind`,
    refresh: `${base}/bind/refresh`,
    manual: `${base}/bind/manual`,
    unbind: `${base}/bind/unbind`,
    config: `${base}/config`,
  };
}

/** 多机器人列表端点（GET，id/label/appId；不含凭据）。 */
export const BOTS_LIST_PATH = '/qqbot/bots';

/** 删除机器人端点（POST，body {id}）。 */
export const BOTS_DELETE_PATH = '/qqbot/bots/delete';

/** 一个 QQ 子会话的浏览器可读描述（不含消息内容；地址由 scope+peerId 决定）。 */
export type QqSessionWire = {
  /** dsh 会话 id */
  id: string;
  /** 私聊 / 群聊 */
  scope: 'c2c' | 'group';
  /** 对端 openid：c2c 为用户、group 为群 */
  peerId: string;
};

/**
 * 列表端点的序列化形态（name = 网关 READY 后的平台账号昵称）。
 *
 * K4：只增字段——`entity/cwd/mainSessionId/sessions` 为多机器人新增，
 * 旧字段语义不变；`sessions` 仅运行中 bot 出现（未启动的 bot 省略）；
 * 顶层 `entities` 仅在桥接服务（astrboxBridge）可用时出现（K8）。
 */
export type BotsListWire = {
  bots: Array<{
    id: string;
    label: string;
    appId: string;
    enabled: boolean;
    name?: string;
    /** 绑定的 NPC 实体 id（未绑定省略） */
    entity?: string;
    /** agent 工作目录（与 SessionManager 同源） */
    cwd?: string;
    /** 主会话 id（运行中且已就绪时出现） */
    mainSessionId?: string;
    /** 该 bot 的 QQ 子会话（仅运行中 bot 出现） */
    sessions?: QqSessionWire[];
  }>;
  /** NPC 实体目录（桥接服务可用时出现；不可用时省略字段） */
  entities?: string[];
};

/** 线上协议（HTTP 路由序列化形态） */
export type BindStateWire =
  | { phase: 'starting' }
  | { phase: 'waiting'; url: string; qrSvg?: string; issuedAt: number }
  | { phase: 'saving' }
  | { phase: 'failed'; reason: string }
  | { phase: 'bound'; appId: string; name?: string };

/** manual / unbind 操作的统一响应 */
export interface BindActionResponse {
  ok: boolean;
  message?: string;
  error?: string;
}
