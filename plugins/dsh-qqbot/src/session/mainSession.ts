/**
 * 主会话（Neko 主体）管理 — 每个 bot 一个常驻父会话。
 *
 * QQ 私聊/群聊会话创建时挂到主会话下（`parentSession` + `origin=subagent`），
 * 形成「主会话 -控制-> 子会话」的原生会话树；主/子会话共用该 bot 的工作区。
 * 主会话没有 QQ 回复目标，因此不安装 QQ 通道提示词（rules/scope/persona/extra），
 * 只安装 K28 主会话提示词段（内容按实体来自 astrbox 桥接）；创建失败只告警、不阻塞 QQ 收发。
 */
import { SessionId } from '@deepseek-ai/dsh-session';
import { createUserMessage } from '@deepseek-ai/dsh-llm';
import { mainSessionTitle } from '../shared/sessionTitles.ts';
import { mintSessionId } from './session-id.ts';
import { attachWorkspaceWhenPersisted } from './attachWorkspace.ts';
import { installMainSessionPrompt, type MainPromptHandle } from './mainPrompt.ts';
import type { ModelResolver } from '../model/model-resolver.ts';
import type { ModelRoute } from '../model/types.ts';
import type { HostAdapter } from '../host/adapter.ts';
import type { Logger } from '../types.ts';
import type { DshAgent, DshAgentHandle, DshAgentRegistry, PresetComposition } from './types.ts';

/** 主会话记录（常驻 agent + 会话 id）。 */
export interface MainSessionHandle {
  /** 会话 id（prefs 持久化，跨进程 resume） */
  sessionId: SessionId;
  /** 常驻 agent */
  agent: DshAgent;
  /** agent 句柄（卸载时 dispose） */
  handle: DshAgentHandle;
  /** 生效 preset（缺省 = 宿主组合） */
  agentPreset?: string;
}

/** MainSession 依赖（由 SessionManager 注入）。 */
export interface MainSessionDeps {
  /** QQ 机器人 AppID（拼主会话 key） */
  appId: string;
  agents: DshAgentRegistry;
  host: HostAdapter;
  logger: Logger;
  modelResolver: ModelResolver;
  /** bot 工作区（与子会话同源） */
  workspaceDir: () => string;
  /** bot 的工作区显示名（机器人名/绑定实体名，K10） */
  workspaceTitle: () => string;
  /** 判定会话是否已被宿主归档（归档会话不得 resume，须重建） */
  isSessionArchived: (sessionId: string) => boolean;
  /** 主会话启动时主动发送的 kickoff 提示词（空白 = 不发送） */
  mainSessionKickoff: () => string;
  /** 主会话提示词（K28）：按本 bot 绑定实体同步读取；空白 = 不注入 */
  mainSessionPrompt: () => string;
  /** 按 key 解析生效 preset（per-key 覆盖 > 配置默认） */
  effectivePreset: (key: string) => string | undefined;
  /** 按 key 解析生效模型路由 */
  effectiveRoute: (key: string) => ModelRoute | undefined;
  /** 会话持久化的 preset（resume 时优先沿用） */
  resolvePersistedPreset: (sessionId: SessionId) => Promise<string | undefined>;
  /** preset 组合（setup hook） */
  composePreset: (presetId?: string) => Promise<PresetComposition>;
}

/** 错误文案（日志用）。 */
function errorText(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

/**
 * 主会话管理器：懒创建 + singleflight + 失败降级。
 *
 * 一个实例只对应一个 bot；`dispose()` 后不再创建。
 */
export class MainSession {
  private record: MainSessionHandle | null = null;
  private inflight: Promise<SessionId | null> | null = null;
  /** K28 主会话提示词段的注册句柄（随 record 生命周期，重装前先释放）。 */
  private promptHandle: MainPromptHandle | undefined;

  /**
   * @param deps - 代理仓库、宿主适配、工作区与 preset 依赖
   */
  public constructor(private readonly deps: MainSessionDeps) {}

  /** 主会话 key：`qqbot:<appId>:main`（与 QQ peer key 不冲突）。 */
  private key(): string {
    return `qqbot:${this.deps.appId}:main`;
  }

  /** 主会话 id（未创建/创建失败/已归档时 undefined，供 /status 与排障使用）。 */
  public get id(): string | undefined {
    if (this.record === null) return undefined;
    // 归档的主会话不能再作为子会话父指针（其后代 turn 会被宿主 gate 拒绝）。
    return this.deps.isSessionArchived(this.record.sessionId) ? undefined : this.record.sessionId;
  }

  /**
   * 确保主会话存在（读盘 resume → 缺失则 create）；幂等。
   * @returns 主会话 id；创建/恢复失败时为 null
   */
  public async ensure(): Promise<SessionId | null> {
    if (this.record !== null) {
      if (!this.deps.isSessionArchived(this.record.sessionId)) return this.record.sessionId;
      // 已归档：释放旧 agent（它已无法推进 turn），重新开一个主会话。
      this.deps.logger.info(`main session archived; recreating: ${this.record.sessionId}`);
      await this.dispose();
    }
    if (this.inflight !== null) return this.inflight;
    const task = this.open()
      .catch((err: unknown): SessionId | null => {
        this.deps.logger.warn(`main session init failed: ${errorText(err)}`);
        return null;
      })
      .finally(() => { this.inflight = null; });
    this.inflight = task;
    return task;
  }

  /**
   * 子会话创建时的父指针：主会话已就绪则返回 id；创建在飞则等它落定；否则 null。
   * @returns 父会话 id 或 null（不建主会话，避免历史/测试路径副作用）
   */
  public async parentIfReady(): Promise<SessionId | null> {
    if (this.record !== null && this.deps.isSessionArchived(this.record.sessionId)) {
      // 归档主会话不能再当父指针：重建后再给子会话用（失败则子会话无父，仍可收发）。
      await this.ensure().catch(() => null);
    }
    if (this.inflight !== null) await this.inflight.catch(() => null);
    return this.record === null || this.deps.isSessionArchived(this.record.sessionId) ? null : this.record.sessionId;
  }

  /** 已发送过 kickoff 的会话 id（同一主会话只发一次；有历史轮次的会话永不再发）。 */
  private readonly kickoffSent = new Set<string>();

  /**
   * 主会话 kickoff：会话还从未产生过 turn/start 时，主动 followup 一条提示词，
   * 让「Neko 主体」拥有自己的第一轮对话——否则主会话是 DSH 的空会话（blank），
   * 会被侧边栏工作区隐藏，用户看不到该对话。
   * @param agent 已登记的主会话 agent
   * @param sessionId 主会话 id
   */
  private maybeKickoff(agent: DshAgent, sessionId: SessionId): void {
    let prompt = '';
    try {
      prompt = String(this.deps.mainSessionKickoff() ?? '').trim();
    } catch {
      return;
    }
    if (prompt === '' || this.kickoffSent.has(sessionId)) return;
    this.kickoffSent.add(sessionId);
    try {
      const events = this.deps.host.sessionEvents(agent.session);
      if (events.some((event) => (event as { type?: string } | null)?.type === 'turn/start')) return;
      agent.followup(createUserMessage({ content: [{ type: 'text', text: prompt }], source: { kind: 'user' } }));
      this.deps.logger.info(`main session kickoff sent: sessionId=${sessionId}`);
    } catch (err) {
      this.deps.logger.warn(`main session kickoff failed (ignore): ${errorText(err)}`);
    }
  }

  /** 释放主会话（插件卸载 / HMR 接管）。 */
  public async dispose(): Promise<void> {
    if (this.record === null) return;
    const record = this.record;
    this.record = null;
    this.disposeMainPrompt();
    record.agent.cancel({ kind: 'user' });
    await record.handle.dispose().catch(() => {});
  }

  /**
   * 安装主会话提示词段（K28）：内容每步现读桥接值（空串 = 宿主丢弃空段），失败只 warn。
   * @param agent 主会话 agent
   */
  private attachMainPrompt(agent: DshAgent): void {
    this.disposeMainPrompt();
    try {
      this.promptHandle = installMainSessionPrompt(agent.ctx, () => this.deps.mainSessionPrompt(), this.deps.logger);
    } catch (err) {
      this.deps.logger.warn(`main session prompt install failed (ignore): ${errorText(err)}`);
    }
  }

  /** 释放主会话提示词段（幂等）。 */
  private disposeMainPrompt(): void {
    if (this.promptHandle === undefined) return;
    const handle = this.promptHandle;
    this.promptHandle = undefined;
    handle.dispose();
  }

  /** 创建/恢复实现（singleflight 内串行）。 */
  private async open(): Promise<SessionId | null> {
    const key = this.key();
    const { modelResolver, logger } = this.deps;
    const persistedRaw = modelResolver.getSessionId(key);
    // 归档会话不得 resume：宿主 archived-session-gate 会拒绝其 pre-step，
    // 表现为「消息进来但 turn blocked」。直接铸造新主会话。
    const persisted = persistedRaw !== undefined && this.deps.isSessionArchived(persistedRaw) ? undefined : persistedRaw;
    if (persistedRaw !== undefined && persisted === undefined) {
      logger.info(`main session archived; minting a fresh one: old=${persistedRaw}`);
    }
    const presetId = (persisted === undefined ? undefined : await this.deps.resolvePersistedPreset(SessionId(persisted)))
      ?? this.deps.effectivePreset(key);
    const composed = await this.deps.composePreset(presetId);
    const route = this.deps.effectiveRoute(key);
    const meta = {
      cwd: this.deps.workspaceDir(),
      ...(composed.agentPreset ? { agentPreset: composed.agentPreset } : {}),
    };

    if (persisted !== undefined) {
      const sessionId = SessionId(persisted);
      const live = this.deps.agents.get(sessionId);
      if (live) {
        const adopted = this.adopt(key, sessionId, live, undefined, composed.agentPreset);
        this.maybeKickoff(live, sessionId);
        return adopted;
      }
      try {
        const resumed = await this.deps.agents.resume({
          resumeSessionId: sessionId,
          ...(route ? { agentOptions: route } : {}),
          ...(composed.setup ? { setup: composed.setup } : {}),
        });
        const adopted = this.adopt(key, sessionId, resumed.agent, resumed, composed.agentPreset);
        this.maybeKickoff(resumed.agent, sessionId);
        return adopted;
      } catch (err) {
        const exists = await this.deps.host.sessionExists(sessionId).catch(() => undefined);
        if (exists === true) throw err;
        logger.warn(`main session resume failed (recreating): ${errorText(err)}`);
      }
    }

    const sessionId = SessionId(mintSessionId());
    const created = await this.deps.agents.create({
      sessionId,
      meta,
      ...(route ? { agentOptions: route } : {}),
      ...(composed.setup ? { setup: composed.setup } : {}),
    });
    const createdAdopted = this.adopt(key, sessionId, created.agent, created, composed.agentPreset);
    this.maybeKickoff(created.agent, sessionId);
    return createdAdopted;
  }

  /** 登记主会话：落盘 sessionId、挂到 bot 工作区（fail-soft）、记日志。 */
  private adopt(
    key: string,
    sessionId: SessionId,
    agent: DshAgent,
    handle: DshAgentHandle | undefined,
    agentPreset?: string,
  ): SessionId {
    this.record = {
      sessionId,
      agent,
      handle: handle ?? { agent, dispose: async () => {} },
      agentPreset,
    };
    if (this.deps.modelResolver.getSessionId(key) !== sessionId) {
      this.deps.modelResolver.setSessionId(key, sessionId);
    }
    // 工作区会话名固定为「主体」（先把标题 pin 住，后续 kickoff 的自动标题不会再覆盖它）。
    this.deps.host.ensureSessionTitle(agent, mainSessionTitle());
    // K28：主会话提示词段（按实体，来自 astrbox 桥接；空值不注入）。
    this.attachMainPrompt(agent);
    // 与 QQ 子会话同一工作区：侧边栏按 bot 工作区挂载，主/子会话都在该工作区下可见；
    // 工作区显示名用机器人名（绑定实体名），旧默认名 QQBot 会被迁移。
    // 新建会话需等头文件落盘再 attach，否则会被宿主拒绝并掉进「未分组」。
    void attachWorkspaceWhenPersisted(
      this.deps.host,
      sessionId,
      () => this.deps.workspaceDir(),
      () => this.deps.workspaceTitle(),
      this.deps.logger,
    );
    this.deps.logger.info(`main session ready: key=${key} sessionId=${sessionId} preset=${agentPreset ?? 'none'}`);
    return sessionId;
  }
}
