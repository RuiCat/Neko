/**
 * dsh-im-qqbot 插件配置 Schema
 */
import Schema from '@deepseek-ai/schemastery';
import type { ConversationPromptEntry } from './shared/conversationPrompts.ts';
import type { OwnerConfig } from './shared/owner.ts';

export interface AccessControlConfig {
  /** C2C 访问模式 */
  c2cMode: 'open' | 'allowlist' | 'disabled';
  /** C2C 白名单（user openid） */
  c2cAllow: string[];
  /** 群聊访问模式 */
  groupMode: 'open' | 'allowlist' | 'disabled';
  /** 群聊白名单（group openid） */
  groupAllow: string[];
}

export interface MediaConfig {
  /** 是否启用富媒体理解（图片/视频下载 + 工具分析） */
  enabled: boolean;
  /** 富媒体下载大小上限（MB），默认 200 */
  maxMB: number;
  /** 富媒体存活时长（小时，TTL），默认 24，0 = 永不过期 */
  ttlHours: number;
}

export interface VisionConfig {
  /** 是否启用视觉理解（qqbot_describe_image 工具） */
  enabled: boolean;
  /** 视觉模型 provider（dsh 注册的 llm adapter，如 pi-ai） */
  provider: string;
  /** 视觉模型 id（如 qwen-vl-max） */
  model: string;
  /** 默认描述 prompt（调用未显式指定时使用） */
  defaultPrompt: string;
  /** 图片字节上限，默认 10MB */
  maxBytes: number;
  /** 输出 token 上限 */
  maxTokens: number;
  /** 视觉调用超时(ms) */
  timeoutMs: number;
}

/** 附件发送配置（qqbot_send_file 工具与 present 交付物投递共用） */
export interface SendFileConfig {
  /** 是否启用路径白名单（默认 true，仅允许 media + cwd + extraRoots；关闭则任意路径） */
  restrictPaths: boolean;
  /** 额外允许访问的根目录（media 和 agent cwd 始终默认允许） */
  extraRoots: string[];
  /** 是否把 present 声明的交付文件在轮末发送到当前会话 */
  sendPresentedFiles: boolean;
  /** 单轮最多投递的交付文件数 */
  maxFiles: number;
}

/**
 * 非 QQ 读者输入（如桌面端续聊 QQ 会话）的回合是否主动推送到 QQ。
 * - off：不推送，回复只留在发起的界面；
 * - c2c：只推送私聊会话（默认，避免桌面上的私人操作出现在群里）；
 * - all：私聊和群聊都推送。
 */
export type ExternalTurnDelivery = 'off' | 'c2c' | 'all';

/** 主动推送策略（定时任务等服务回合始终主动推送） */
export interface ProactiveDeliveryConfig {
  externalTurns: ExternalTurnDelivery;
}

export interface ImQQBotConfig {
  /** QQ Bot AppID */
  appId: string;
  /** QQ Bot AppSecret */
  appSecret: string;
  /** dsh LLM 提供商名称 */
  provider?: string;
  /** 模型名称 */
  model?: string;
  /** Agent preset id */
  preset?: string;
  /** Agent 工作目录（缺省 ~/.dsh-qqbot/workspace，不随宿主进程 cwd） */
  cwd?: string;
  /**
   * 绑定的 NPC 实体 id（旧单 bot 顶层字段；bots[] 条目可各自覆盖）。
   * 空串 = 未绑定；可含中文（实体 id 不走 bot id 的 ASCII 规则）。
   */
  entity?: string;
  /**
   * 实体分类路径（从 `分类:名称` 绑定值里拆出来，仅用于「实体不存在时按该类型创建」；
   * 不设缺省值：没有分类就不自动创建）。
   */
  entityType?: string;
  /** 是否启用群消息 @mention 门控 */
  requireMention: boolean;
  /** 群聊额外 system prompt */
  groupPrompt?: string;
  /** 私聊额外 system prompt */
  directPrompt?: string;
  /**
   * 单会话独立提示词（K26）：命中会话 scope+peerId 时追加在 groupPrompt/directPrompt 之后。
   * bots[] 条目可整体覆盖；空数组 = 不追加任何会话级提示词。
   */
  conversationPrompts: ConversationPromptEntry[];
  /**
   * 主人/管理员身份（K27）：配置后逐轮向模型注入对方是否主人的身份行，
   * 并在 allowlist 模式下允许主人绕过命令白名单。
   */
  owner?: OwnerConfig;
  /** 单条消息最大长度（QQ 限制约 5000 字符） */
  textChunkLimit: number;
  /** 是否启用流式输出（群聊始终不启用） */
  streaming: boolean;
  /** 每会话最大闲置时长(ms)，超时自动回收；<=0 表示不回收 */
  sessionIdleTimeout: number;
  /** 并发队列最大长度 */
  maxQueue: number;
  /** 处理超时(ms)，超时中断当前 LLM 调用 */
  processingTimeoutMs: number;
  /** 待答问题超时(ms)，超时自动拒绝并提示（ask_user_question） */
  askTimeoutMs: number;
  /** 群历史缓冲条数 */
  historyLimit: number;
  /** 主会话启动时主动发送的 kickoff 提示词（空串/空白 = 不主动发送） */
  mainSessionKickoff: string;
  /** 访问控制 */
  access: AccessControlConfig;
  /** 多机器人：非空时每个元素是一个独立机器人（appId/appSecret 分别配置）；空数组 = 沿用顶层单 bot 字段 */
  bots: BotEntryConfig[];
  /** 是否展示工具调用成功结果（工具错误始终展示） */
  showToolResults: boolean;
  /** 调试模式 */
  debug: boolean;
  /** 富媒体理解（图片/视频） */
  media: MediaConfig;
  /** 视觉理解（qqbot_describe_image 工具，走 dsh llm + attachments） */
  vision: VisionConfig;
  /** 附件发送（qqbot_send_file 工具与交付物投递共用） */
  sendFile: SendFileConfig;
  /** 主动推送策略 */
  proactiveDelivery: ProactiveDeliveryConfig;
}

/** 访问控制子 Schema（独立导出：config 路由的服务端校验复用） */
export const AccessControlSchema: Schema<AccessControlConfig> = Schema.object({
  c2cMode: Schema.union(['open', 'allowlist', 'disabled']).default('open').description('C2C访问模式（默认开放）'),
  c2cAllow: Schema.array(Schema.string()).default([]).description('C2C白名单（QQ OpenID）'),
  groupMode: Schema.union(['open', 'allowlist', 'disabled']).default('open').description('群聊访问模式（默认开放：群自带进群门槛 + @bot 触发）'),
  groupAllow: Schema.array(Schema.string()).default([]).description('群聊白名单（群 OpenID）'),
});

/** 单个机器人的配置项（多机器人模式下 bots[] 的元素；未写的字段继承顶层）。 */
export interface BotEntryConfig {
  /** 稳定标识：存储/路由/工具归属都用它（[a-z0-9-]{2,32}，同一配置内唯一） */
  id: string;
  /** 展示名（缺省取 id） */
  label?: string;
  /** 是否启用（默认 true；缺少 appId 时会被归一化层停用） */
  enabled: boolean;
  /** QQ 机器人 AppID */
  appId: string;
  /** QQ 机器人 AppSecret（明文；为空时按 appSecretRef 从凭据库解析） */
  appSecret: string;
  /** 凭据库引用（环境变量名语法；新绑定默认写 QQBOT_<ID>_SECRET，避免密钥进 profile patch） */
  appSecretRef?: string;
  /** 覆盖顶层 LLM provider */
  provider?: string;
  /** 覆盖顶层模型 */
  model?: string;
  /** 覆盖顶层 agent preset */
  preset?: string;
  /** 覆盖顶层工作目录 */
  cwd?: string;
  /** 覆盖顶层的实体绑定（`分类:名称` 或纯名称；空串 = 不绑定；缺省继承顶层 entity） */
  entity?: string;
  /** 实体分类路径（通常由 `分类:名称` 自动拆出，无需单独配置） */
  entityType?: string;
  /** 覆盖顶层群聊额外 system prompt */
  groupPrompt?: string;
  /** 覆盖顶层私聊额外 system prompt */
  directPrompt?: string;
  /** 覆盖顶层单会话独立提示词（提供时整体替换顶层列表） */
  conversationPrompts?: ConversationPromptEntry[];
  /** 覆盖顶层主人身份（提供时整体覆盖；null = 显式不设置） */
  owner?: OwnerConfig | null;
  /** 覆盖顶层群 @ 门控 */
  requireMention?: boolean;
  /** 覆盖顶层主会话 kickoff 提示词（空串 = 不发送） */
  mainSessionKickoff?: string;
  /** 访问控制（提供时整体覆盖顶层 access，需写全） */
  access?: AccessControlConfig;
}

/** 单个 owner 对象 Schema（K27；openid 必填，其余业务校验在 shared/owner 归一化层）。 */
const OwnerObjectSchema: Schema<OwnerConfig> = Schema.object({
  openid: Schema.string().required().description('主人的 QQ openid（私聊发送者 openid）'),
  name: Schema.string().description('主人的称呼（默认「主人」；≤64 字符）'),
});

/** owner 配置 Schema：缺失 = 未设置；null = 显式不设置（设置页清空走 null）。 */
export const OwnerSchema = Schema.union([OwnerObjectSchema, Schema.const(null)])
  .description('主人/管理员身份；空 openid 不允许，清空请用 null');

/** 单会话独立提示词条目 Schema（K26；业务校验在 shared/conversationPrompts 归一化层）。 */
export const ConversationPromptSchema: Schema<ConversationPromptEntry> = Schema.object({
  scope: Schema.union(['c2c', 'group']).required().description('会话类型：私聊 c2c / 群聊 group'),
  peerId: Schema.string().required().description('对端 openid（私聊=用户、群聊=群）；匹配时忽略首尾空白'),
  prompt: Schema.string().required().description('追加在该会话场景提示词之后的文本（1–80000 字符）'),
});

/** bots[] 元素 Schema：业务校验（id/appId 唯一、非空）在 configBots.ts 归一化层完成。 */
export const BotEntrySchema: Schema<BotEntryConfig> = Schema.object({
  id: Schema.string().default('').description('机器人稳定标识（[a-z0-9-]{2,32}，同一配置内唯一）'),
  label: Schema.string().description('展示名（缺省取 id）'),
  enabled: Schema.boolean().default(true).description('是否启用'),
  appId: Schema.string().default('').description('QQ 机器人 AppID'),
  appSecret: Schema.string().default('').description('QQ 机器人 AppSecret'),
  appSecretRef: Schema.string().description('凭据库引用（appSecret 为空时从这里解析，env 名语法）'),
  provider: Schema.string().description('覆盖顶层 LLM provider'),
  model: Schema.string().description('覆盖顶层模型'),
  preset: Schema.string().description('覆盖顶层 agent preset'),
  cwd: Schema.string().description('覆盖顶层工作目录'),
  entity: Schema.string().default('').description('绑定的 NPC 实体 id（空串 = 不绑定；缺省继承顶层 entity）'),
  groupPrompt: Schema.string().description('覆盖顶层群聊额外 system prompt'),
  directPrompt: Schema.string().description('覆盖顶层私聊额外 system prompt'),
  // 用 any 保留「未提供 = undefined」，避免数组默认 [] 覆盖顶层；元素校验在 configBots 归一化层 fail-fast。
  conversationPrompts: Schema.any().description('覆盖顶层单会话提示词（提供时整体替换；由 configBots 归一化校验）'),
  owner: OwnerSchema,
  requireMention: Schema.boolean().description('覆盖顶层群 @ 门控'),
  access: AccessControlSchema.description('访问控制覆盖（整体覆盖顶层 access，需写全）'),
});

export const ConfigSchema: Schema<ImQQBotConfig> = Schema.object({
  appId: Schema.string().default('').description('QQ 机器人 AppID'),
  appSecret: Schema.string().default('').description('QQ 机器人 AppSecret'),
  provider: Schema.string().description('LLM provider name'),
  model: Schema.string().description('Model name'),
  preset: Schema.string().description('Agent preset id'),
  cwd: Schema.string().description('Agent working directory (default ~/.dsh-qqbot/workspace)'),
  entity: Schema.string().default('').description('绑定的 NPC 实体 id（空串 = 不绑定；bots[] 条目可覆盖）'),
  requireMention: Schema.boolean().default(true).description('群聊是否需要@bot触发'),
  groupPrompt: Schema.string().description('群聊额外system prompt'),
  directPrompt: Schema.string().description('私聊额外system prompt'),
  conversationPrompts: Schema.array(ConversationPromptSchema).default([])
    .description('单会话独立提示词：命中 scope+peerId 时追加在 groupPrompt/directPrompt 之后（空数组 = 不追加）'),
  // 运行时允许 null（设置页清空写入），归一化层（configBots）再转成 undefined；
  // 类型层对外仍是 normalized OwnerConfig，故此处断言。
  owner: OwnerSchema as unknown as Schema<OwnerConfig | undefined>,
  textChunkLimit: Schema.number().default(4500).description('单条消息最大字符数'),
  streaming: Schema.boolean().default(true).description('是否启用流式输出（群聊始终不启用）'),
  sessionIdleTimeout: Schema.number().default(30 * 60 * 1000).description('会话闲置超时(ms)，<=0 不回收'),
  maxQueue: Schema.number().default(20).description('并发队列最大长度'),
  processingTimeoutMs: Schema.number().default(30 * 60 * 1000).description('处理超时(ms)，超时中断当前 LLM 调用'),
  askTimeoutMs: Schema.number().default(5 * 60 * 1000).description('待答问题超时(ms)，超时自动拒绝并提示'),
  historyLimit: Schema.number().default(10).description('群历史缓冲条数'),
  mainSessionKickoff: Schema.string().default('（系统提示）主会话已启动：请以你的角色向主人自然地报到一句，然后等待主人消息。')
    .description('主会话启动时主动发送的 kickoff 提示词（空串 = 不主动发送）'),
  access: AccessControlSchema.default({
    c2cMode: 'open',
    c2cAllow: [],
    groupMode: 'open',
    groupAllow: [],
  }).description('访问控制'),
  showToolResults: Schema.boolean().default(false).description('是否展示工具调用成功结果（错误始终展示）'),
  debug: Schema.boolean().default(false),
  media: Schema.object({
    enabled: Schema.boolean().default(true).description('是否启用富媒体理解（图片/视频下载 + 工具分析）'),
    maxMB: Schema.number().default(200).description('富媒体下载大小上限(MB)'),
    ttlHours: Schema.number().default(24).description('富媒体存活时长(小时)，0=永不过期'),
  }).default({
    enabled: true,
    maxMB: 200,
    ttlHours: 24,
  }).description('富媒体理解配置'),
  vision: Schema.object({
    enabled: Schema.boolean().default(false).description('是否启用视觉理解（qqbot_describe_image 工具）'),
    provider: Schema.string().default('').description('视觉模型 provider（dsh 注册的 llm adapter，如 pi-ai）'),
    model: Schema.string().default('').description('视觉模型 id（如 qwen-vl-max）'),
    defaultPrompt: Schema.string().default('Describe this image in detail.').description('默认描述 prompt'),
    maxBytes: Schema.number().default(10 * 1024 * 1024).description('图片字节上限'),
    maxTokens: Schema.number().default(1024).description('输出 token 上限'),
    timeoutMs: Schema.number().default(120000).description('视觉调用超时(ms)'),
  }).default({
    enabled: false,
    provider: '',
    model: '',
    defaultPrompt: 'Describe this image in detail.',
    maxBytes: 10 * 1024 * 1024,
    maxTokens: 1024,
    timeoutMs: 120000,
  }).description('视觉理解配置'),
  sendFile: Schema.object({
    restrictPaths: Schema.boolean().default(true).description('是否启用路径白名单（默认 true，仅允许 media + cwd + extraRoots）'),
    extraRoots: Schema.array(Schema.string()).default([]).description('额外允许访问的根目录'),
    sendPresentedFiles: Schema.boolean().default(true).description('是否将 present 声明的交付文件发送到当前会话'),
    maxFiles: Schema.number().default(4).description('单轮最多投递的交付文件数'),
  }).default({
    restrictPaths: true,
    extraRoots: [],
    sendPresentedFiles: true,
    maxFiles: 4,
  }).description('附件发送配置'),
  proactiveDelivery: Schema.object({
    externalTurns: Schema.union(['off', 'c2c', 'all']).default('c2c')
      .description('非 QQ 输入（如桌面续聊）的回合是否推送到 QQ：off=不推送，c2c=仅私聊，all=私聊和群聊'),
  }).default({
    externalTurns: 'c2c',
  }).description('主动推送策略（定时任务等服务回合始终推送）'),
  bots: Schema.array(BotEntrySchema).default([])
    .description('多机器人列表：每项独立 appId/appSecret 与可选的 provider/model/preset/cwd/prompt/access 覆盖；空 = 沿用顶层单 bot 配置'),
});
