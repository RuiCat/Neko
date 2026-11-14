/**
 * 未绑定机器人的扫码会话注册表 — 每 bot 懒创建 BindingService。
 *
 * 多机器人模式下，没有凭据的 bot 不启动网关运行时，但设置页/右栏仍需
 * 扫码绑定能力：本注册表按 bot id 懒创建扫码状态机（首次状态轮询触发），
 * 扫码成功后把凭据写回该 bot 的 bots[] 条目（经注入的 persist 回调），
 * 由宿主 HMR 以新凭据重启运行时。
 *
 * HMR 双 fiber 窗口下同一 bot 可能被两个注册表创建服务：新服务创建时
 * 终止进程内登记的旧服务（接管），避免两份二维码轮询并行。
 */
import type { Logger } from '../types.ts';
import { BindingService } from './binding-service.ts';
import type { SetupCredentials } from './persist.ts';
import type { BindStateWire } from '../shared/bind-protocol.ts';

/** 扫码成功后的持久化回调；false = 未写出（面板展示失败原因）。 */
export type PersistBotCredentialsFn = (
  botId: string,
  credentials: SetupCredentials,
) => boolean | Promise<boolean>;

/** 扫码服务工厂（测试注入点；默认按 source/logger 构造 BindingService）。 */
export type BotBindingServiceFactory = (botId: string) => BindingService;

/** 注册表构造参数。 */
export interface BotBindingRegistryOptions {
  /** 插件日志器 */
  logger: Logger;
  /** 该 bot 凭据的持久化回调（写 profile patch，触发 HMR） */
  persist: PersistBotCredentialsFn;
  /** 二维码 URL 的 source 标识（默认 dsh-qqbot） */
  source?: string;
  /** 扫码服务工厂（缺省使用真实 BindingService） */
  createService?: BotBindingServiceFactory;
}

/** 一个 bot 的扫码会话：状态机 + 持久化失败原因。 */
interface BotBindingSession {
  service: BindingService;
  /** 扫码成功但写盘失败时的原因（覆盖状态机相位，避免永久停在 saving） */
  failure?: string;
}

/** 进程内活动服务（HMR 双 fiber 接管用；按 bot id 单例）。 */
const ACTIVE_BOT_SERVICES = new Map<string, BindingService>();

/**
 * 每 bot 扫码会话注册表。
 *
 * 只服务于「当前没有凭据」的 bot：已绑定 bot 的绑定路由由 boundAppId
 * 直接回报 bound 相位，不会创建扫码服务。
 */
export class BotBindingRegistry {
  private readonly sessions = new Map<string, BotBindingSession>();
  private readonly logger: Logger;
  private readonly persist: PersistBotCredentialsFn;
  private readonly source: string;
  private readonly createService: BotBindingServiceFactory;
  private disposed = false;

  /**
   * @param options - 日志器、持久化回调与测试注入点
   */
  public constructor(options: BotBindingRegistryOptions) {
    this.logger = options.logger;
    this.persist = options.persist;
    this.source = options.source ?? 'dsh-qqbot';
    this.createService = options.createService ?? (() => new BindingService({
      source: this.source,
      logger: options.logger,
    }));
  }

  /**
   * 取该 bot 的扫码服务；首次调用创建并启动二维码轮询。
   * @param botId - 配置里的稳定 bot id
   * @returns 该 bot 的扫码状态机
   */
  public serviceFor(botId: string): BindingService {
    const existing = this.sessions.get(botId);
    if (existing !== undefined) return existing.service;

    if (this.disposed) {
      // fiber 已卸载（请求在途）：创建一个已终止的服务，避免留下轮询任务
      const orphan = this.createService(botId);
      orphan.dispose();
      return orphan;
    }

    // HMR 接管：上一 fiber 遗留的同 bot 服务先终止
    const stale = ACTIVE_BOT_SERVICES.get(botId);
    if (stale !== undefined) stale.dispose();

    const service = this.createService(botId);
    const session: BotBindingSession = { service };
    this.sessions.set(botId, session);
    ACTIVE_BOT_SERVICES.set(botId, service);

    void service.start()
      .then((credentials) => {
        if (credentials === null) return undefined;
        return this.handleBound(botId, session, credentials);
      })
      .catch((err: unknown) => {
        session.failure = err instanceof Error ? err.message : String(err);
        this.logger.warn(`im-qqbot: 机器人 ${botId} 扫码绑定失败: ${session.failure}`);
      });
    return service;
  }

  /**
   * 面板轮询用的线上相位：优先回报持久化失败，其余透传状态机。
   * @param botId - 配置里的稳定 bot id
   * @returns 绑定面板可消费的相位
   */
  public wireState(botId: string): BindStateWire {
    const session = this.sessions.get(botId);
    if (session === undefined) return { phase: 'starting' };
    if (session.failure !== undefined) return { phase: 'failed', reason: session.failure };
    return session.service.wireState();
  }

  /**
   * 用户点击「刷新二维码」：废弃当前二维码任务并重建。
   * @param botId - 配置里的稳定 bot id
   */
  public refresh(botId: string): void {
    this.serviceFor(botId).refresh();
  }

  /** 终止全部扫码会话（插件 fiber 卸载 / HMR 接管）。 */
  public dispose(): void {
    if (this.disposed) return;
    this.disposed = true;
    for (const [botId, session] of this.sessions) {
      session.service.dispose();
      if (ACTIVE_BOT_SERVICES.get(botId) === session.service) ACTIVE_BOT_SERVICES.delete(botId);
    }
    this.sessions.clear();
  }

  /**
   * 扫码成功：把凭据写回该 bot 条目；失败时让面板显示原因而不是停在 saving。
   * @param botId - 配置里的稳定 bot id
   * @param session - 该 bot 的会话
   * @param credentials - 扫码获取的凭据
   * @returns void
   */
  private async handleBound(
    botId: string,
    session: BotBindingSession,
    credentials: SetupCredentials,
  ): Promise<void> {
    try {
      const persisted = await this.persist(botId, credentials);
      if (!persisted) {
        session.failure = '凭据未能写入 profile，请改用手动填写或在 cordis.patch.yml 中配置';
        this.logger.warn(`im-qqbot: 机器人 ${botId} 扫码成功但凭据写入失败`);
        return;
      }
      this.logger.info(`✔ 机器人 ${botId} 扫码绑定成功（AppID ${credentials.appId}），等待热更新重载`);
    } catch (err) {
      session.failure = err instanceof Error ? err.message : String(err);
      this.logger.warn(`im-qqbot: 机器人 ${botId} 凭据保存异常: ${session.failure}`);
    }
  }
}
