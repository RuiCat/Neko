import { installQQPrompt, type QQPromptHandle, type QQTurnContext } from '../prompt/install.ts';
import { turnMetaText } from '../prompt/turn-meta.ts';
import { readBridgeMainSessionPrompt, readBridgePersonaStatus } from '../shared/astrboxBridge.ts';
import type { QqSessionWire } from '../shared/bind-protocol.ts';
import { collectQqSessions, parseQqSessionKey } from './qqSessions.ts';
import { selectConversationPrompt } from '../shared/conversationPrompts.ts';
/**
 * SessionManager — 管理 QQ peer → dsh Agent 的映射和生命周期
 *
 * sessionKey 格式: `qqbot:${appId}:${kind}:${peerId}`
 *   - c2c:   peerId = senderId (用户 openid)
 *   - group: peerId = groupOpenid
 *
 * sessionKey → sessionId 的映射存于 prefs（`model-prefs.json`），新会话 id 由
 * `mintSessionId()` 铸造（`session-` + UUID，与宿主一致）。0.5.x 的 SHA-256 确定性
 * 派生 id 仅用于探测存量会话并迁移，不再用于新建——确定性 id 跨进程必然相同，
 * 两个进程各自新建会把同一 id 写入不同 project 目录。
 *
 * 支持 agent-presets 系统：通过 setup hook 在 create/resume 时
 * 挂载 preset（工具集、prompt sections 等），实现场景化配置。
 */
import { legacyDerivedSessionId, mintSessionId } from './session-id.ts';
import { attachWorkspaceWhenPersisted } from './attachWorkspace.ts';
import { peerSessionTitle } from '../shared/sessionTitles.ts';
import { SessionId } from '@deepseek-ai/dsh-session';
import type { Context } from '@deepseek-ai/cordis';
import { createHostAdapter, type HostAdapter } from '../host/adapter.ts';
import { mkdirSync } from 'node:fs';
import { resolveAgentCwd } from '../shared/dirs.ts';
import type { ChatScope, Logger, ReplyTarget } from '../types.ts';
import type { ImQQBotConfig } from '../config.ts';
import { ModelResolver } from '../model/model-resolver.ts';
import type { ModelRoute, ModelEntry } from '../model/types.ts';
import { IdleEvictor } from './idle-evictor.ts';
import { MainSession } from './mainSession.ts';
import { latestCompletedPrefixBoundary } from './sessionBoundary.ts';
import { errorText, extractMessageText } from './sessionText.ts';
import type { QuestionChannel } from '../features/question-channel.ts';
import type { ApprovalChannel } from '../features/approval-channel.ts';
import type {
  SessionEventLike,
  DshAgent,
  DshAgentHandle,
  DshAgentRegistry,
  AgentPresetsLike,
  PresetComposition,
  PresetEntry,
  PresetSwitchOutcome,
  SessionRecord,
  SessionStatus,
  TokenUsageStats,
  CompactionServiceLike,
  CompactOutcome,
} from './types.ts';

/** ManualCompactionError 各 code 的友好提示 */
const COMPACTION_ERROR_HINTS: Record<string, string> = {
  busy: '压缩服务忙，请稍后重试',
  cancelled: '压缩已取消',
  changed: '历史在压缩过程中发生变化，请重试',
  commit: '压缩提交失败',
  persistence: '压缩完成但保存失败',
};

export class SessionManager {
  private sessions = new Map<string, SessionRecord>();
  /**
   * 同 key 正在进行的 getOrCreate（singleflight）。
   *
   * 斜杠命令与待答问题截获位于 concurrencyGuard 之前（见 gateway/middleware-setup），
   * 与普通消息不串行：同 key 并发进入会各自 resume/create，产出两个 agent，
   * 后写入 sessions 的覆盖前者且前者无人 dispose（泄漏）。
   */
  private inflight = new Map<string, Promise<SessionRecord>>();
  private readonly evictor: IdleEvictor;
  private readonly modelResolver: ModelResolver;
  private readonly ctx: Context;
  private readonly host: HostAdapter;
  private readonly agents: DshAgentRegistry;
  private readonly config: ImQQBotConfig;
  private readonly logger: Logger;
  /** 多机器人时用于按 bot 隔离默认工作目录（缺省 = 旧单 bot 路径）。 */
  private readonly botId?: string;
  /** 工作区显示名提示（机器人名/绑定实体名；缺省回落到 config.entity 再回落 QQBot）。 */
  private readonly workspaceTitleHint?: string;
  /** 由 bootstrap 注入的问答通道（ask_user_question → QQ），会话回收时清理其待答问题 */
  public questionChannel?: QuestionChannel;
  /** 由 bootstrap 注入的审批通道（approval/request → QQ），会话回收时清理其待批审批 */
  public approvalChannel?: ApprovalChannel;
  /** 主会话（Neko 主体）：每个 bot 一个常驻父会话（QQ 私聊/群聊子会话挂在其下） */
  private readonly main: MainSession;

  constructor(
    ctx: Context,
    agents: DshAgentRegistry,
    config: ImQQBotConfig,
    logger: Logger,
    botId?: string,
    workspaceTitle?: string,
  ) {
    this.ctx = ctx;
    this.agents = agents;
    this.config = config;
    this.logger = logger;
    this.botId = botId;
    this.workspaceTitleHint = workspaceTitle;
    this.host = createHostAdapter(ctx, logger);
    this.logger.info(`host adapter capabilities: ${this.host.capabilities.join(', ')}`);
    this.modelResolver = new ModelResolver(ctx, config, logger);
    this.main = new MainSession({
      appId: config.appId,
      agents,
      host: this.host,
      logger,
      modelResolver: this.modelResolver,
      workspaceDir: () => this.workspaceDir(),
      workspaceTitle: () => this.workspaceTitle(),
      isSessionArchived: sessionId => this.host.isSessionArchived(sessionId),
      mainSessionKickoff: () => this.config.mainSessionKickoff ?? '',
      // K28：主会话提示词按本 bot 绑定实体从桥接读取（面板编辑，下一轮生效）。
      mainSessionPrompt: () => readBridgeMainSessionPrompt(this.ctx, this.config.entity?.trim() || undefined),
      effectivePreset: key => this.getEffectivePresetByKey(key),
      effectiveRoute: key => this.modelResolver.getEffectiveRoute(key),
      resolvePersistedPreset: sessionId => this.resolvePersistedPreset(sessionId),
      composePreset: presetId => this.composePreset(presetId),
    });

    this.evictor = new IdleEvictor(
      this.sessions,
      config.sessionIdleTimeout,
      (key, record) => {
        this.logger.info(`evicting idle session: key=${key}`);
        this.sessions.delete(key);
        this.releasePrompt(record.agent);
        record.agent.cancel({ kind: 'user' });
        void record.handle.dispose().catch(() => {});
        // 回收会话时统一清理其待答问题/待批审批，避免 pending 悬挂泄漏
        this.questionChannel?.cancelPending(key);
        this.approvalChannel?.cancelPending(key);
      },
    );

  }

  /** 已装 prompt 的 agent → 句柄。释放时须同步 delete：live 复用的 agent 要能重装，否则 prompt 永久丢失。 */
  private readonly promptHandles = new WeakMap<DshAgent, QQPromptHandle>();
  /** 已就「人设未就绪」告警过的实体（每实体只 warn 一次，避免每轮刷屏）。 */
  private readonly personaNotReadyWarned = new Set<string>();

  /**
   * 在 agent.ctx 上安装 QQ prompt（幂等、fail-soft）：失败仅 warn，不打断会话建立。
   * 人设卡（K7）来自桥接服务 `astrboxBridge.persona(本 bot 的 entity)`：
   * 同步读取缓存字符串，无实体/无服务/异常时为空段（宿主丢弃空段）。
   */
  private attachPrompt(agent: DshAgent, scope: ChatScope, peerId: string): void {
    if (this.promptHandles.has(agent)) return;
    if (!agent.ctx.get?.('systemPrompt')) return;
    const entityId = this.config.entity?.trim() || undefined;
    try {
      this.promptHandles.set(agent, installQQPrompt(agent.ctx, {
        scope,
        personaCard: () => this.personaCardFor(entityId),
        // K26：场景词（group/direct）+ 命中 (scope,peerId) 的会话级提示词（追加，空行分隔）。
        extraPrompts: () => ({
          group: this.config.groupPrompt,
          direct: this.config.directPrompt,
          conversation: selectConversationPrompt(this.config.conversationPrompts, scope, peerId),
        }),
        // K27：配置 owner 时逐轮注入对方身份行（未配置返回 undefined，旧行为不变）。
        turnContext: () => this.turnContextFor(scope, peerId),
      }));
    } catch (err) {
      this.logger.warn(`qq prompt install failed (session keeps working without it): ${errorText(err)}`);
    }
  }

  /**
   * personaCard 实现：同步读桥接人设；实体已绑定但人设未就绪时每实体 warn 一次
   * （重启/冷启动首轮竞态：下一轮自动生效，不静默）。
   * @param entityId - 本 bot 绑定的实体 id（未绑定 undefined）
   * @returns 人设 markdown；未就绪为空串（宿主丢弃空段）
   */
  private personaCardFor(entityId: string | undefined): string {
    const status = readBridgePersonaStatus(this.ctx, entityId);
    if (entityId !== undefined && !status.ready && !this.personaNotReadyWarned.has(entityId)) {
      this.personaNotReadyWarned.add(entityId);
      this.logger.warn(`qqbot: 实体 ${entityId} 的人设尚未就绪（本轮到下一轮之间自动生效；若持续为空请检查 astrboxBridge 服务与 personaPaths）`);
    }
    return status.text;
  }

  /**
   * 本轮 turn-meta 快照（K27）：配置 owner 时注入「对方身份」行；未配置返回 undefined。
   * 发送者来自当前 SessionRecord（每条消息在 getOrCreate 时更新 senderId/senderName）。
   * @param scope - 会话场景
   * @param peerId - 对端 openid
   * @returns 注入用的 turn-meta 文本；无 owner 时 undefined（不注册空内容）
   */
  private turnContextFor(scope: ChatScope, peerId: string): QQTurnContext | undefined {
    const owner = this.config.owner;
    if (owner === undefined) return undefined;
    const record = this.sessions.get(this.sessionKey(scope, peerId));
    return {
      turnMetaText: turnMetaText({ scope, senderId: record?.senderId, senderName: record?.senderName, owner }),
      backgroundText: '',
    };
  }

  /** 释放 prompt 注册项；须在 handle.dispose() 之前（agent 销毁后 ctx 可能失效）。 */
  private releasePrompt(agent: DshAgent): void {
    const handle = this.promptHandles.get(agent);
    if (!handle) return;
    this.promptHandles.delete(agent);
    try {
      handle.dispose();
    } catch (err) {
      this.logger.warn(`qq prompt dispose failed: ${errorText(err)}`);
    }
  }

  /**
   * 动态获取 agentPresets 服务（可选，未注入时返回 undefined）
   */
  private getPresetsService(): AgentPresetsLike | undefined {
    try {
      return this.ctx.get('agentPresets') as AgentPresetsLike | undefined;
    } catch {
      return undefined;
    }
  }

  /**
   * 从 session 持久化 header 读 session 当初的 preset（started 锁定语义）。
   * 读不到（服务缺失 / session 不存在 / 未记录 agentPreset）返回 undefined。
   */
  private async resolvePersistedPreset(sessionId: string): Promise<string | undefined> {
    return (await this.host.sessionHeader(sessionId))?.agentPreset;
  }

  // ── 模型相关（委托给 ModelResolver） ──

  getEffectiveModel(scope: ChatScope, peerId: string): ModelRoute | undefined {
    return this.modelResolver.getEffectiveRoute(this.sessionKey(scope, peerId));
  }

  /**
   * 获取指定 peer 的生效 preset（per-peer 覆盖 > config.preset）
   */
  getEffectivePreset(scope: ChatScope, peerId: string): string | undefined {
    return this.getEffectivePresetByKey(this.sessionKey(scope, peerId));
  }

  /** 按 sessionKey 解析生效 preset */
  private getEffectivePresetByKey(key: string): string | undefined {
    return this.modelResolver.getPreset(key) ?? this.config.preset;
  }

  /**
   * 列出所有可用 preset（/preset 命令用）
   */
  async listPresets(): Promise<PresetEntry[]> {
    const presets = this.getPresetsService();
    if (!presets) return [];
    try {
      const list = await presets.list();
      return list.map((p) => ({ id: p.id, name: p.name, description: p.description }));
    } catch (err) {
      this.logger.warn(`list presets failed: ${err instanceof Error ? err.message : String(err)}`);
      return [];
    }
  }

  /**
   * 切换模型（fork + 重建会话，保留历史）
   */
  async setModelOverride(scope: ChatScope, peerId: string, route: ModelRoute): Promise<void> {
    const key = this.sessionKey(scope, peerId);

    this.modelResolver.setOverride(key, route);

    const record = this.sessions.get(key);
    if (!record) {
      this.logger.info(`model pref saved (no active session): key=${key} → ${route.provider}/${route.model}`);
      return;
    }

    await this.rebuildSession(record, { route });
    this.logger.info(`model switched via fork: key=${key} → ${route.provider}/${route.model}`);
  }

  /**
   * 切换指定 peer 的 agent preset（per-peer 覆盖）
   *
   * 已开始的会话保持锁定（blank-session contract）：只持久化为新会话默认，
   * 不 fork/rebuild 当前会话，避免历史工具调用与新 preset 工具集冲突。
   */
  async setPresetOverride(scope: ChatScope, peerId: string, presetId: string): Promise<PresetSwitchOutcome> {
    const key = this.sessionKey(scope, peerId);

    // 1. 校验 preset 存在 + 可加载（broken 拒绝）
    const presets = this.getPresetsService();
    if (!presets) return { ok: false, reason: 'unavailable' };
    let resolved: { id: string; broken?: string };
    try {
      resolved = await presets.resolve(presetId);
    } catch (err) {
      return { ok: false, reason: 'unknown-preset', message: err instanceof Error ? err.message : String(err) };
    }
    if (resolved.broken !== undefined) {
      return { ok: false, reason: 'broken', message: resolved.broken };
    }

    // 2. 持久化 per-peer 覆盖（新会话生效）
    this.modelResolver.setPreset(key, presetId);

    const hasActive = this.sessions.has(key);
    this.logger.info(
      `preset pref saved: key=${key} → ${presetId}${hasActive ? ' (active session locked, next session applies)' : ''}`,
    );
    return { ok: true, presetId };
  }

  /**
   * 清除 per-peer preset 覆盖，回到 config.preset / 默认（新会话生效）
   */
  async clearPresetOverride(scope: ChatScope, peerId: string): Promise<PresetSwitchOutcome> {
    const key = this.sessionKey(scope, peerId);
    this.modelResolver.clearPreset(key);
    this.logger.info(`preset pref cleared: key=${key} (next session applies)`);
    return { ok: true };
  }

  /**
   * fork + 重建会话（保留历史，换 model）
   *
   * options.route 缺省时沿用当前生效值。preset 沿用当前生效 preset（getEffectivePresetByKey）。
   * fork 不可用或失败时降级为 dispose（下次消息重建）。
   */
  private async rebuildSession(
    record: SessionRecord,
    options: { route?: ModelRoute },
  ): Promise<void> {
    const key = record.sessionKey;

    // 对齐宿主官方 fork（commands.fork）：buildForkSeed 产物原样作 seed，
    // create 传 inheritedEventCount + isSeeded + parentSession
    const events = this.host.sessionEvents(record.agent.session);
    const boundary = latestCompletedPrefixBoundary(events);
    if (boundary === undefined) {
      this.logger.warn(`fork unavailable (no completed turn), fallback to dispose: key=${key}`);
      this.sessions.delete(key);
      this.releasePrompt(record.agent);
      record.agent.cancel({ kind: 'user' });
      await record.handle.dispose().catch(() => {});
      return;
    }

    const seed = await this.host.buildForkSeed(events, boundary);
    if (seed === undefined) {
      this.logger.warn(`fork seed unavailable (old host), fallback to dispose: key=${key}`);
      this.sessions.delete(key);
      this.releasePrompt(record.agent);
      record.agent.cancel({ kind: 'user' });
      await record.handle.dispose().catch(() => {});
      return;
    }

    const childId = SessionId(mintSessionId());
    const effectiveRoute = options.route ?? this.modelResolver.getEffectiveRoute(key);
    const composed = await this.composePreset(this.getEffectivePresetByKey(key));
    const created = await this.agents.create({
      sessionId: childId,
      seed,
      inheritedEventCount: boundary + 1,
      meta: {
        cwd: this.workspaceDir(),
        isSeeded: true,
        // 旧版宿主的 delivery 豁免要求 parentSession，新版忽略——都传
        parentSession: record.sessionId,
        ...(composed.agentPreset ? { agentPreset: composed.agentPreset } : {}),
      },
      ...(effectiveRoute ? { agentOptions: effectiveRoute } : {}),
      ...(composed.setup ? { setup: composed.setup } : {}),
    });

    this.modelResolver.setSessionId(key, childId);

    const oldHandle = record.handle;
    const oldAgent = record.agent;
    record.sessionId = childId;
    this.attachPrompt(created.agent, record.scope, record.peerId);
    record.agent = created.agent;
    record.handle = created;
    record.agentPreset = composed.agentPreset;
    record.lastActivity = Date.now();

    this.releasePrompt(oldAgent);
    void oldHandle.dispose().catch(() => {});
    this.logger.info(`session rebuilt via fork: key=${key} preset=${composed.agentPreset ?? 'none'} sessionId=${childId}`);
  }

  clearModelOverride(scope: ChatScope, peerId: string): void {
    const key = this.sessionKey(scope, peerId);
    this.modelResolver.clearOverride(key);
    this.modelResolver.clearSessionId(key);
  }

  async listAvailableModels(): Promise<ModelEntry[]> {
    return this.modelResolver.listModels();
  }

  listProviders(): string[] {
    return this.modelResolver.listProviders();
  }

  // ── 会话状态 / 统计 ──

  getSessionRecord(scope: ChatScope, peerId: string): SessionRecord | undefined {
    return this.sessions.get(this.sessionKey(scope, peerId));
  }

  /**
   * 该 scope 最近活跃会话的回复目标（K15 主动直发用；只读，不唤醒会话）。
   *
   * 只遍历内存中的 QQ 会话记录（不含主会话），按 `lastActivity` 取最大者；
   * 调用方负责把返回目标改成主动推送（`forceProactive`），不复用被动窗口。
   * @param scope - 私聊 / 群聊
   * @returns 最近活跃记录里的 replyTarget；该 scope 无活跃会话时为 undefined
   */
  latestTarget(scope: ChatScope): ReplyTarget | undefined {
    let latest: SessionRecord | undefined;
    for (const record of this.sessions.values()) {
      if (record.scope !== scope) continue;
      if (latest === undefined || record.lastActivity > latest.lastActivity) latest = record;
    }
    return latest?.replyTarget;
  }

  getStatus(scope: ChatScope, peerId: string): SessionStatus {
    const record = this.getSessionRecord(scope, peerId);
    const route = this.getEffectiveModel(scope, peerId);

    return {
      active: !!record,
      sessionId: record?.sessionId,
      provider: route?.provider,
      model: route?.model,
      preset: record?.agentPreset,
      lastActivity: record?.lastActivity,
      messageCount: this.countMessages(record),
    };
  }

  getTokenUsage(scope: ChatScope, peerId: string): TokenUsageStats {
    const record = this.getSessionRecord(scope, peerId);
    const stats: TokenUsageStats = { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 };
    if (!record) return stats;

    const events = this.host.sessionEvents(record.agent.session) as readonly SessionEventLike[];
    if (!events) return stats;

    for (const event of events) {
      if (event.type !== 'assistant/message' || !event.usage) continue;
      stats.input += event.usage.input ?? 0;
      stats.output += event.usage.output ?? 0;
      stats.cacheRead += event.usage.cacheRead ?? 0;
      stats.cacheWrite += event.usage.cacheWrite ?? 0;
    }

    return stats;
  }

  exportMarkdown(scope: ChatScope, peerId: string): string {
    const record = this.getSessionRecord(scope, peerId);
    if (!record) return '';

    const events = this.host.sessionEvents(record.agent.session) as readonly SessionEventLike[];
    if (!events || events.length === 0) return '';

    const lines: string[] = [`# QQ 会话导出\n`, `> session: ${record.sessionId}\n`];

    for (const event of events) {
      if (event.type === 'user/message') {
        const text = extractMessageText(event.message);
        if (text) lines.push(`## 用户\n\n${text}\n`);
      } else if (event.type === 'assistant/message') {
        const text = extractMessageText(event.message);
        if (text) lines.push(`## 助手\n\n${text}\n`);
      }
    }

    return lines.join('\n');
  }

  private countMessages(record: SessionRecord | undefined): number {
    if (!record) return 0;
    const events = this.host.sessionEvents(record.agent.session) as readonly SessionEventLike[];
    if (!events) return 0;

    let count = 0;
    for (const event of events) {
      if (event.type === 'user/message' || event.type === 'assistant/message') {
        count += 1;
      }
    }
    return count;
  }

  // ── Session 生命周期管理 ──

  private sessionKey(scope: ChatScope, peerId: string): string {
    return `qqbot:${this.config.appId}:${scope}:${peerId}`;
  }

  /** prefs 命中沿用；miss 返回 0.5.x 派生候选，由 getOrCreate 按 resume 结果迁移或铸造 */
  private currentSessionId(sessionKey: string): string {
    return this.modelResolver.getSessionId(sessionKey) ?? legacyDerivedSessionId(sessionKey);
  }

  /**
   * 是否为 qqbot 旧版创建的 subagent 子会话（origin=subagent）。
   * 宿主工作区树不展示 subagent 会话，残留映射必须重铸才能在侧边栏看到对话。
   */
  private async isLegacySubagentSession(sessionId: string): Promise<boolean> {
    try {
      const header = await this.host.sessionHeader(sessionId);
      return header?.origin === 'subagent';
    } catch {
      return false;
    }
  }

  /** 候选命中后落盘，后续永远走 prefs */
  private persistSessionId(sessionKey: string, sessionId: string): void {
    if (this.modelResolver.getSessionId(sessionKey) !== sessionId) {
      this.modelResolver.setSessionId(sessionKey, sessionId);
    }
  }

  private async composePreset(presetId?: string): Promise<PresetComposition> {
    const presets = this.getPresetsService();
    if (!presets) return {};

    try {
      const resolved = await presets.resolve(presetId);
      const resolvedId = resolved.id;
      return {
        agentPreset: resolvedId,
        setup: async (agentCtx: Context) => {
          await presets.mount(agentCtx, resolvedId);
        },
      };
    } catch (err) {
      this.logger.warn(
        `im-qqbot: preset ${presetId ?? '(default)'} unavailable: ${err instanceof Error ? err.message : String(err)} — using host composition`,
      );
      return {};
    }
  }

  /**
   * 确保主会话（Neko 主体）存在；QQ 私聊/群聊子会话创建时挂到它下面。
   * @returns 主会话 id；创建失败时为 null
   */
  async ensureMainSession(): Promise<SessionId | null> {
    return this.main.ensure();
  }

  /** 主会话 id（未创建/创建失败时 undefined，供 /status 与排障使用）。 */
  get mainSessionId(): string | undefined {
    return this.main.id;
  }

  /** 本 bot 的工作区绝对路径（遗弃会话清理用）。 */
  get workspaceDirectory(): string {
    return this.workspaceDir();
  }

  /**
   * 本 bot 当前仍被引用的会话 id：主会话 + prefs 里该 appId 的记录 + 内存中活动会话。
   * 供遗弃会话清理做保留集合（不唤醒会话、不写盘）。
   * @returns 会话 id 数组
   */
  knownSessionIds(): string[] {
    const ids = new Set<string>();
    if (this.mainSessionId !== undefined) ids.add(this.mainSessionId);
    const prefix = `qqbot:${this.config.appId}:`;
    for (const entry of this.modelResolver.listSessionEntries()) {
      if (entry.sessionKey.startsWith(prefix)) ids.add(entry.sessionId);
    }
    for (const record of this.sessions.values()) ids.add(record.sessionId);
    return [...ids];
  }

  /** 获取或恢复或创建会话（get → resume → create）；同 key 并发合并为一次 */
  async getOrCreate(
    scope: ChatScope,
    peerId: string,
    senderId: string,
    replyTarget: ReplyTarget,
    senderName?: string,
  ): Promise<SessionRecord> {
    const key = this.sessionKey(scope, peerId);
    const existing = this.sessions.get(key);

    if (existing) {
      // 会话被归档后宿主 gate 会拒绝其 turn：丢掉本地记录并重新开会话，
      // 否则用户发消息只会得到 blocked（无回复、无新会话）。
      if (!this.host.isSessionArchived(existing.sessionId)) {
        existing.replyTarget = replyTarget;
        existing.lastActivity = Date.now();
        // K27：身份行按「本轮发送者」判定，多人群聊里每条消息都要更新。
        existing.senderId = senderId;
        existing.senderName = senderName;
        return existing;
      }
      this.logger.info(`session archived; reopening: key=${key} sessionId=${existing.sessionId}`);
      this.sessions.delete(key);
      this.releasePrompt(existing.agent);
      existing.agent.cancel({ kind: 'user' });
      void existing.handle.dispose().catch(() => {});
      this.questionChannel?.cancelPending(key);
      this.approvalChannel?.cancelPending(key);
    }

    // singleflight：同 key 并发复用同一次 resume/create，后到者只更新回复目标
    const pending = this.inflight.get(key);
    if (pending) {
      const shared = await pending;
      shared.replyTarget = replyTarget;
      shared.lastActivity = Date.now();
      shared.senderId = senderId;
      shared.senderName = senderName;
      return shared;
    }

    // K9：主会话缺失（启动时创建失败/被回收/被归档）→ 按需重试一次。
    // ensureMainSession 自身 singleflight + 失败只 warn，不阻塞消息；子会话创建时
    // 会经 parentIfReady 等待在飞的主会话，从而恢复 parentSession 父指针。
    if (this.mainSessionId === undefined
      || (this.mainSessionId !== undefined && this.host.isSessionArchived(this.mainSessionId))) {
      void this.ensureMainSession();
    }

    const task = this.openSession(key, scope, peerId, senderId, replyTarget, senderName);
    this.inflight.set(key, task);
    try {
      return await task;
    } finally {
      this.inflight.delete(key);
    }
  }

  /** getOrCreate 的实际打开流程（已通过 singleflight 保证同 key 串行） */
  private async openSession(
    key: string,
    scope: ChatScope,
    peerId: string,
    senderId: string,
    replyTarget: ReplyTarget,
    senderName?: string,
  ): Promise<SessionRecord> {
    const route = this.modelResolver.getEffectiveRoute(key);
    // 归档会话不得 resume（宿主 archived-session-gate 会让 turn 直接 blocked）：
    // 先把映射改写为新铸造的 id，后续流程自然走 create 分支。
    const persistedId = this.modelResolver.getSessionId(key);
    if (persistedId !== undefined) {
      const archived = this.host.isSessionArchived(persistedId);
      // 旧版把 QQ 会话建成 origin=subagent 的子会话 → 工作区树按设计隐藏 subagent，
      // 用户看不到对话。映射到这种会话时同样重铸成顶层会话（历史仍保留在旧会话里）。
      const legacyChild = !archived && await this.isLegacySubagentSession(persistedId);
      if (archived || legacyChild) {
        const fresh = mintSessionId();
        this.modelResolver.setSessionId(key, fresh);
        this.logger.info(`${archived ? 'archived' : 'legacy-subagent'} session; minting fresh one: key=${key} old=${persistedId} new=${fresh}`);
      }
    }
    // prefs 命中 = 已知存量会话；miss 时取 0.5.x 派生 id 仅作存量探测候选，不用于新建
    const candidateIsLegacy = this.modelResolver.getSessionId(key) === undefined;
    let sessionId = SessionId(this.currentSessionId(key));
    this.logger.info(`getOrCreate: key=${key} route=${route ? `${route.provider}/${route.model}` : 'host-default'} sessionId=${sessionId}`);

    let agent: DshAgent;
    let handle: DshAgentHandle | undefined;
    let agentPreset: string | undefined;

    const live = this.agents.get(sessionId);
    if (live) {
      agent = live;
      this.persistSessionId(key, sessionId);
      this.logger.info(`reusing live agent: key=${key}`);
    } else {
      // preset 只解析一次：resume/create 共用同一组合，避免重复 resolve/mount 目录。
      // 优先 session 当初的 preset（started 锁定语义），无则用当前 effective preset。
      const presetId = (await this.resolvePersistedPreset(sessionId)) ?? this.getEffectivePresetByKey(key);
      const composed = await this.composePreset(presetId);
      agentPreset = composed.agentPreset;
      try {
        const resumeRoute = this.modelResolver.getResumeRoute(key);
        const resumed = await this.agents.resume({
          resumeSessionId: sessionId,
          ...(resumeRoute ? { agentOptions: resumeRoute } : {}),
          ...(composed.setup ? { setup: composed.setup } : {}),
        });
        agent = resumed.agent;
        handle = resumed;
        // 0.5.x 迁移：候选命中老会话，沿用并落盘
        this.persistSessionId(key, sessionId);
        this.logger.info(`resumed session: key=${key} preset=${agentPreset ?? 'none'} route=${resumeRoute ? `${resumeRoute.provider}/${resumeRoute.model}` : 'session-own'}`);
      } catch (err) {
        if (!(await this.mayCreateAfterResumeFailure(key, sessionId, err))) throw err;
        sessionId = this.resolveCreateId(key, sessionId, candidateIsLegacy);
        // QQ 会话是独立顶层会话（不带 parentSession/origin=subagent）：
        // 宿主工作区树按设计隐藏 subagent 会话，只有顶层会话会在侧边栏可见。
        const created = await this.agents.create({
          sessionId,
          meta: {
            cwd: this.workspaceDir(),
            ...(agentPreset ? { agentPreset } : {}),
          },
          ...(route ? { agentOptions: route } : {}),
          ...(composed.setup ? { setup: composed.setup } : {}),
        });
        agent = created.agent;
        handle = created;
        this.persistSessionId(key, sessionId);
        this.logger.info(`created new session: key=${key} sessionId=${sessionId} preset=${agentPreset ?? 'none'}`);
      }
    }

    const record: SessionRecord = {
      sessionKey: key,
      sessionId,
      agent,
      handle: handle ?? { agent, dispose: async () => {} },
      replyTarget,
      scope,
      peerId,
      senderId,
      senderName,
      lastActivity: Date.now(),
      agentPreset,
    };

    this.attachPrompt(agent, scope, peerId);
    this.sessions.set(key, record);
    // 工作区会话名：主体 / 私聊(xxx) / 群聊(xxx)；用户手动改过的标题保持不动。
    this.host.ensureSessionTitle(agent, peerSessionTitle(scope, peerId, senderName));

    // 挂载到对应工作区（侧边栏可见性）；fail-soft、异步执行，不阻塞消息处理。
    // 宿主只在客户端 session.create RPC 时挂载会话，插件会话不走那条路径，
    // 闲置回收后会从侧边栏消失（详见 host/adapter.ts 的 attachSessionToWorkspace）。
    // cwd 必须与 meta.cwd 同源（workspaceDir()），否则 attachSession 的
    // header.cwd 校验（realpath 后须等于工作区 path）会失败；新建会话还要等头文件落盘。
    void attachWorkspaceWhenPersisted(
      this.host,
      sessionId,
      () => this.workspaceDir(),
      () => this.workspaceTitle(),
      this.logger,
    );

    return record;
  }

  /**
   * resume 失败后是否允许首次创建。
   *
   * 只有「会话确实不在盘上」才可创建：损坏/占用/后端故障沿用同一 id 重建，会把
   * 同一 sessionId 写进多个 project 目录，宿主 `list()` 随后报 duplicate，会话
   * 列表整体打不开（对齐宿主 `restoreOrCreateConfigured` 的 fail-loud 语义）。
   *
   * @returns 允许创建返回 true；拒绝时已记录 warn，调用方应抛出原 resume 错误。
   */
  private async mayCreateAfterResumeFailure(key: string, sessionId: string, resumeError: unknown): Promise<boolean> {
    const context = `key=${key} sessionId=${sessionId} resumeErr=${errorText(resumeError)}`;
    let exists: boolean | undefined;
    try {
      exists = await this.host.sessionExists(sessionId);
    } catch (probeError) {
      // 求证本身失败（持久化后端故障）：无法断言不在盘上，按存在处理
      this.logger.warn(`refusing to recreate session, existence unknown: ${context} probeErr=${errorText(probeError)}`);
      return false;
    }
    if (exists === true) {
      this.logger.warn(`refusing to recreate existing session: ${context}`);
      return false;
    }
    if (exists === undefined) {
      // 宿主无 sessionPersistence.stat：没有求证手段，只能按 resume 结果创建（= 修复前行为）
      this.logger.warn(`creating session without existence check (host lacks sessionPersistence.stat): ${context}`);
    }
    return true;
  }

  /**
   * 确认候选会话缺失后决定用哪个 id 新建。
   *
   * - resume 等待期间 prefs 被并发改写（`/new` 换新会话）：采用 prefs 的最新 id，
   *   否则内存与 prefs 指向不同会话，重启后用户被切走；
   * - 候选来自 0.5.x 确定性派生：一律铸造新 id，确定性 id 不进入新建路径；
   * - 候选来自 prefs 且确认缺失：沿用该 id 新建，prefs 无需改写。
   */
  private resolveCreateId(key: string, candidate: SessionId, candidateIsLegacy: boolean): SessionId {
    const latest = this.modelResolver.getSessionId(key);
    if (latest !== undefined && latest !== candidate) {
      this.logger.info(`adopting concurrently written session id: key=${key} sessionId=${latest}`);
      return SessionId(latest);
    }
    if (candidateIsLegacy) {
      const minted = SessionId(mintSessionId());
      this.logger.info(`migrated to minted session id: key=${key} sessionId=${minted}`);
      return minted;
    }
    return candidate;
  }

  findBySessionId(sessionId: string): SessionRecord | undefined {
    for (const record of this.sessions.values()) {
      if (record.sessionId === sessionId) return record;
    }
    return undefined;
  }

  /**
   * 这条 session 对应的 QQ 地址。只读：不把宿主里的 agent 收进会话表，也不拥有它的生命周期。
   * 内存里有记录时沿用它的 replyTarget（含 msgId）；否则按 prefs 的 sessionId 反查对端。
   */
  deliveryAddress(sessionId: string): ReplyTarget | undefined {
    const existing = this.findBySessionId(sessionId);
    if (existing) return existing.replyTarget;

    const key = this.modelResolver.findSessionKey(sessionId);
    if (key === undefined || this.sessions.has(key)) return undefined;
    const parsed = parseQqSessionKey(key, this.config.appId);
    if (parsed === undefined) return undefined;
    return { scope: parsed.scope, targetId: parsed.peerId };
  }

  /**
   * 本 bot 的 QQ 子会话快照（K4：`/qqbot/bots` 的 `sessions` 字段）。
   * 只读：枚举 prefs 的 sessionKey→sessionId，按本 bot appId 过滤并解析 scope/peer；
   * 不唤醒 agent、不写盘，未启动的会话同样会出现在列表里（历史会话可见）。
   * @returns 子会话描述数组（prefs 内存表顺序）
   */
  listQqSessions(): ReadonlyArray<QqSessionWire> {
    return collectQqSessions(this.modelResolver.listSessionEntries(), this.config.appId);
  }

  findByAgent(agent: DshAgent): SessionRecord | undefined {
    for (const record of this.sessions.values()) {
      if (record.agent === agent) return record;
    }
    return undefined;
  }

  async remove(scope: ChatScope, peerId: string): Promise<void> {
    const key = this.sessionKey(scope, peerId);
    const record = this.sessions.get(key);
    this.modelResolver.setSessionId(key, mintSessionId());

    if (!record) return;
    this.sessions.delete(key);
    this.releasePrompt(record.agent);
    record.agent.cancel({ kind: 'user' });
    await record.handle.dispose().catch(() => {});
    this.logger.info(`session removed: key=${key}`);
  }

  /** Agent 工作目录：显式配置优先，缺省独立目录 ~/.dsh-qqbot/workspace（不随宿主进程 cwd） */
  private workspaceCache: string | undefined;
  /** 工作区显示名：机器人名（绑定实体名优先）> 配置实体 > 插件旧默认名。 */
  private workspaceTitle(): string {
    const hinted = this.workspaceTitleHint?.trim();
    if (hinted !== undefined && hinted !== '') return hinted;
    const entity = this.config.entity?.trim();
    return entity !== undefined && entity !== '' ? entity : 'QQBot';
  }

  private workspaceDir(): string {
    if (this.workspaceCache) return this.workspaceCache;
    const dir = resolveAgentCwd(this.config.cwd, this.botId, this.config.entity);
    mkdirSync(dir, { recursive: true });
    this.workspaceCache = dir;
    return dir;
  }

  /**
   * 原地压缩当前会话历史（保留 sessionId，用摘要替换旧历史）。
   *
   * compaction 是 preset 作用域服务，经 host adapter 从会话 preset 解析，
   * 全局注册的部署再兜底。这是「压缩上下文」的语义，区别于 remove() 的「换新会话」。
   */
  async compact(
    scope: ChatScope,
    peerId: string,
    revive?: { senderId: string; replyTarget: ReplyTarget },
  ): Promise<CompactOutcome> {
    const key = this.sessionKey(scope, peerId);
    let record = this.sessions.get(key);
    if (!record) {
      // 无活跃会话（重启/闲置回收）时从磁盘拉起，历史在磁盘上仍可压缩
      if (!revive) return { ok: false, reason: 'no-session' };
      record = await this.getOrCreate(scope, peerId, revive.senderId, revive.replyTarget);
    }
    if (record.agent.status !== 'idle') return { ok: false, reason: 'busy' };

    const compaction = (await this.host.serviceFor(record.agent, 'compaction')
      ?? this.host.globalService('compaction')) as CompactionServiceLike | undefined;
    if (!compaction) return { ok: false, reason: 'unavailable' };

    const route = this.modelResolver.getEffectiveRoute(key);
    const agentCtx = {
      session: record.agent.session,
      options: { provider: route?.provider, model: route?.model },
      runMaintenance: record.agent.runMaintenance.bind(record.agent),
    };

    try {
      const result = await compaction.compactNow(agentCtx, new AbortController().signal);
      if (result === null) return { ok: true, shadowed: 0, tokens: 0 };
      return { ok: true, shadowed: result.shadowedSeqs.length, tokens: result.shadowedTokenCount };
    } catch (err) {
      const code = (err as { code?: string } | null)?.code;
      // summary = 摘要没有更小 = 历史不足以压缩，属正常结果而非失败
      if (code === 'summary') {
        return { ok: true, shadowed: 0, tokens: 0 };
      }
      // 其他 ManualCompactionError 预期失败，给友好文案（debug 记录，不 warn 刷屏）
      if (code !== undefined && code in COMPACTION_ERROR_HINTS) {
        this.logger.debug(`compact declined: key=${key} code=${code}`);
        return { ok: false, reason: 'failed', message: COMPACTION_ERROR_HINTS[code] };
      }
      const message = err instanceof Error ? err.message : String(err);
      this.logger.warn(`compact failed: key=${key} err=${message}`);
      return { ok: false, reason: 'failed', message };
    }
  }

  async disposeAll(): Promise<void> {
    this.evictor.dispose();
    await this.main.dispose();
    const records = [...this.sessions.values()];
    this.sessions.clear();
    for (const record of records) {
      this.releasePrompt(record.agent);
      record.agent.cancel({ kind: 'user' });
    }
    await Promise.allSettled(records.map((r) => r.handle.dispose()));
    this.logger.info(`all sessions disposed (count=${records.length})`);
  }

  get size(): number {
    return this.sessions.size;
  }
}
