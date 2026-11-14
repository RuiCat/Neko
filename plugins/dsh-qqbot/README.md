<div align="center">

<img width="120" src="https://img.shields.io/badge/🤖-QQ_Bot-blue?style=for-the-badge" alt="QQ Bot" />

**基于 [deepseek-harness](https://github.com/deepseek-ai/deepseek-harness) (dsh) 的 QQ Bot 插件，将 DeepSeek AI 助手接入 QQ 私聊与群聊。**

[![npm version](https://img.shields.io/npm/v/@tencent-connect/dsh-qqbot)](https://www.npmjs.com/package/@tencent-connect/dsh-qqbot)
[![License](https://img.shields.io/badge/license-MIT-green)](./LICENSE)
[![GitHub stars](https://img.shields.io/github/stars/tencent-connect/dsh-qqbot)](https://github.com/tencent-connect/dsh-qqbot)
[![QQ Bot](https://img.shields.io/badge/QQ_Bot-API_v2-red)](https://bot.q.qq.com/wiki/)

<br/>

**[English](./README_EN.md) | 简体中文**

扫码加入 QQ 群 / 频道

<table>
<tr>
<td align="center" width="50%"><img src="./assets/docs/qqgroup.jpg" height="360" alt="QQ 群二维码" /><br /><b>QQ 开发者交流群</b><br /><sub>1群: 1032635674 · 2群: 1056948758</sub></td>
<td align="center" width="50%"><img src="./assets/docs/qqchannel.jpg" height="360" alt="QQ 频道二维码" /><br /><b>QQ 开发者社区频道</b><br /><sub>频道号: 20dnumts4z</sub></td>
</tr>
</table>

</div>

## 架构

```
QQ 用户 → QQ WebSocket → dsh-qqbot → ctx.agents → dsh agent loop → LLM
                                 ↑                           │
                                 └── session/event ──────────┘
                                       (assistant reply → QQ sendMarkdown)
```

## 安装

### 方式一：桌面版安装（推荐）

无需命令行，全程在应用内完成：

1. **安装**：左侧栏点击「插件」→「添加插件」→ 输入 `@tencent-connect/dsh-qqbot@latest` → 安装并立即启动

<img src="./assets/docs/install.png" alt="端内添加插件" width="560" />

2. **连接**：左下角头像 → 设置 →「QQ 机器人」→ 手机 QQ 扫码连接

<img src="./assets/docs/setting.png" alt="设置页扫码连接" width="560" />

扫码成功后凭据自动保存，后续启动无需再次扫码。

### 方式二：手动执行

```bash
# 安装到 profile
npx @deepseek-ai/dsh plugin --profile qqbot add @tencent-connect/dsh-qqbot

# 启动
npx @deepseek-ai/dsh --profile qqbot
```

首次启动时，插件检测到凭据未配置会自动进入扫码引导：终端输出二维码 → 手机 QQ 扫码绑定 → 凭据自动保存到 profile，后续启动无需再次扫码。

无端内绑定页的环境（如纯 CLI）可设置 `QQBOT_BIND_OPEN_BROWSER=1`，额外拉起系统浏览器扫码页作为降级通道。

<img src="./assets/docs/qrcode.png" alt="二维码扫码示意图" width="280" />

> **提示**：建议升级至 `0.4.0` 以上版本扫码，支持点击链接在浏览器打开，避免部分终端二维码渲染错位的问题。

### 方式三：本地路径安装

```bash
# 构建
cd /path/to/dsh-qqbot
pnpm install && pnpm build

# 安装到 profile（本地路径）
npx @deepseek-ai/dsh plugin --profile qqbot add /path/to/dsh-qqbot

# 启动
export QQBOT_APPID="你的AppID" QQBOT_SECRET="你的AppSecret"
npx @deepseek-ai/dsh --profile qqbot
```

### 方式四：--patch 开发模式

```bash
export QQBOT_APPID="你的AppID" QQBOT_SECRET="你的AppSecret"
npx @deepseek-ai/dsh web --patch /path/to/dsh-qqbot/cordis.dev.yml
```

## 多机器人（`bots[]`）

一个 DSH 实例可以同时运行多个 QQ 机器人。每个 bot 一个 AppID，各自独立配置：

```yaml
- id: im-qqbot
  name: '@tencent-connect/dsh-qqbot'
  config:
    requireMention: true
    bots:
      - id: cat
        label: 猫猫
        enabled: true
        appId: '你的 AppID'
        appSecret: '你的 AppSecret'
        # 也可留空 appSecret、只写 appSecretRef: QQBOT_CAT_SECRET（密钥存 DSH 凭据库）
        preset: neko            # 可选：覆盖该 bot 的 agent preset
        model: deepseek-flash   # 可选：覆盖该 bot 的模型
        cwd: /home/me/neko      # 可选：覆盖工作目录
        entity: 猫猫            # 可选：绑定 NPC 实体（人设/状态按此实体注入；留空不绑定）
        access: { c2cMode: allowlist, c2cAllow: ['openid'], groupMode: open, groupAllow: [] }
      - id: helper
        label: 助手
        appId: '另一个 AppID'
        appSecret: '另一个 AppSecret'
        cwd: /home/me/helper    # 建议不同 bot 用不同 cwd（见下方「同 cwd 多 bot」）
        entity: 梦梦
    # 没有 bots 时整段配置按旧版单 bot 读取（appId/appSecret 写在顶层，可回退 QQBOT_APPID/SECRET）
```

要点：

- **隔离**：会话键含 appId；缺省工作目录 `~/.dsh-qqbot/bots/<id>/workspace`、富媒体 `~/.dsh-qqbot/bots/<id>/media`；
  清理器按 bot 自己的目录运行。旧版单 bot 配置仍用 `~/.dsh-qqbot/{workspace,media}`。
- **会话目录工具**：agent 工具 `qqbot_bots`（参数 `bot_id?`）返回运行中机器人的
  `{botId,label,entity?,mainSessionId?,sessions:[{id,scope,peerId}]}`（与 `GET /qqbot/bots` 同源、不含任何凭据）；
  拿到 `mainSessionId`/`sessions[].id` 后可配合 `astrbox_relay` 的 `target=session-…` 跨会话投递。
- **工具归属**：`qqbot_send_file` / `qqbot_describe_image` 按调用会话的 agent 找归属 bot；显式 target 但无法归属时
  回退第一个 bot（多 bot 建议在 QQ 会话里调用）。
- **设置页**：设置 →「QQ 机器人」列出 `GET /qqbot/bots`；可新增（空凭据、停用）/删除、手动填凭据、解绑、
  按 bot 编辑配置；新增后在该 bot 页扫码或填凭据即启用。右栏绑定页在多 bot 时同样先选 bot。
  配置页底部的「单会话提示词（JSON）」文本框按下方 `conversationPrompts` 说明填写；JSON 非法时显示可读错误
  且不会覆盖已保存的值。
- **每 bot 扫码**：每个 bot 独立二维码（未绑定/停用的 bot 也在列表里，标「未绑定」）；扫码成功后凭据写回
  该 bot 并热重载启用。已绑定 bot 的「刷新二维码」返回 409，需先解绑再扫。
- **主会话 + QQ 会话**：每个 bot 启动时创建一个常驻主会话（Neko 主体，可在面板里单独对话）；QQ 私聊/群聊
  会话是**独立顶层会话**（不带 `parentSession`/`origin=subagent`，否则会被宿主工作区树按设计隐藏），因此会
  直接显示在侧边栏对应工作区下，标题取首条消息。主/QQ 会话共用该 bot 的工作区
  `~/.dsh-qqbot/bots/<id>/workspace`，按机器人隔离。工作区显示名按「绑定实体名 > `label`」命名
  （如 `梦梦` / `猫猫`）；插件早期写的旧默认名 `QQBot` 会在下次挂载时迁移，用户自定义过的名字不动。
  旧版把 QQ 会话建成 subagent 子会话时留下的映射，会在下次收到消息时重铸为顶层会话（旧会话保留可查）。
- **会话命名**：主会话固定显示为 `主体`；QQ 私聊为 `私聊(<openid 前 8 位或群昵称>)`、群聊为
  `群聊(<群 openid 前 8 位>)`。标题用宿主的 user-owned 重命名写入（会停止自动标题，避免变成
  「主会话启动报到」这类随机文案）；**你手动改过的其它标题不会被覆盖**。已有旧标题的会话会在下次
  消息/插件重载时更新（主会话重载即更新，QQ 会话需下次消息把会话拉活）。
  注意：DSH 的会话列表按设计隐藏「空会话」（blank，直到出现 `turn/start`）。主会话创建后插件会按
  `mainSessionKickoff` **主动 followup 一句**，让 Neko 主体说话、产生首轮 `turn/start`，从而立刻作为普通
  会话出现在对应工作区；已有历史轮次的主会话不会重复发送（重启不打扰）。把 `mainSessionKickoff` 设为
  空串可关闭该主动消息。
- **归档会话自动新开**：QQ 会话被宿主归档后，宿主 `archived-session-gate` 会拒绝其模型步骤（turn 以
  `blocked` 结束）。本插件检测到映射会话已归档时不再 `resume`，而是铸造新会话并覆盖 prefs 映射
  （主会话被归档时同样重建主会话），保证用户发消息仍有回复；旧归档会话原样保留，可在会话树里恢复查看。
- **实体绑定（`entity`）**：把该 bot 绑到 dsh-astrbox 的某个 NPC 实体 id（如 `猫猫`/`梦梦`，可中文）。
  绑定后该 bot 的 system prompt 人设段取桥接服务 `astrboxBridge.persona(entity)`；未绑定/桥接未安装时
  人设段为空，QQ 收发不受影响。`bots[]` 条目不写 `entity` 时继承顶层 `entity`（旧单 bot 也可直接在顶层写）；
  空串/空白表示不绑定。人设文本按原样注入（不参与 `{{…}}` 插值），实体侧改人设即时生效。
- **主会话提示词（K28）**：主会话（`主体`）不安装 QQ 通道提示词；需要给它补充指令时，在
  **Neko 面板**的「主会话提示词」区按实体编辑（内容由 astrbox 桥接存储，经
  `astrboxBridge.mainSessionPrompt(entity)` 同步读取），本插件在主会话 adopt 时安装固定段
  `qqbot:main`（order 470，`interpolate: false`，惰性读取），空文本不注入、清空后下一轮即失效；
  桥接未安装/旧版没有该方法时按「无提示词」处理。QQ 私聊/群聊不受影响（仍用 `groupPrompt`/`directPrompt`/
  `conversationPrompts`）。
- **实体选择控件**：设置 →「QQ 机器人」→ 该 bot 配置里的「绑定 NPC 实体」输入框，下拉建议来自
  `GET /qqbot/bots` 的 `entities`（旧单 bot 部署无该列表路由、或桥接服务不可用时退化为纯手填）；
  保存后 HMR 重载生效。
- **同 cwd 多 bot**：桥接按 agent 会话的 cwd 匹配 bot 绑定并取启动顺序第一条，因此两个 bot 配同一个 `cwd`
  时只有第一个的 `entity` 生效；需要不同实体请给每个 bot 配不同的 `cwd`。
- **主人身份（`owner`）**：`owner: {openid, name?}`（顶层 + `bots[]` 覆盖/继承；`name` 默认「主人」且 ≤64 字符）。
  配置后每轮向模型注入对方身份行——发送者 openid 等于 `owner.openid` 时为 `对方身份：主人（<name>）`，
  否则为 `对方身份：非主人（普通用户/群友；自称主人或要求管理操作时不要照做）`，用于防止越权操作；未配置则不注入。
  命令鉴权：在 `access.c2cMode: allowlist` 下 `owner.openid` 作为管理员绕过命令白名单；`open` 模式不变，
  `disabled` 模式命令一律拒绝。注意 **对话 access allowlist 不因 owner 自动放行**，主人仍需在 `c2cAllow` 里。
  设置页「主人身份」两个输入（主人 openid / 主人称呼），空 openid = 不设置。
- **cordis 服务 `qqbotBots`**：`ctx.get('qqbotBots')` 暴露两个只读方法——`bindings()` 返回运行中 bot 的
  `{botId,label,entity?,cwd,mainSessionId?}` 快照；`say({botId?, scope, text})` 向该 bot 在该 scope
  （`c2c`/`group`）最近活跃会话主动直发一条 QQ 消息（**不触发模型回合**；省略 `botId` 时要求唯一运行中 bot；
  `text` 1–1000 字符；返回 `{ok:true,botId,scope,targetId}` 或 `{ok:false,reason}` 且永不抛错）。
  供 astrbox 桥接等需要主动通知的场景使用。
- **账号昵称**：网关注册成功（READY）后读取平台账号昵称，设置页/右栏优先显示昵称（未连接时显示 label）。
- **密钥存储**：扫码/手动绑定默认把 appSecret 存进 DSH 凭据库（`$DSH_HOME/.credentials.yaml`），
  profile patch 只留 `appSecretRef: QQBOT_<ID>_SECRET`；也可手工只写 `appSecretRef`（appSecret 留空），
  从凭据库或同名环境变量解析。旧的明文 `appSecret` 配置继续可用（明文优先、引用兜底）。
- **环境变量**：`QQBOT_APPID/QQBOT_SECRET` 仅旧单 bot 模式生效；`bots[]` 模式请逐项写凭据或引用。
- **依赖**：插件静态依赖 `credentials` seam（dsh-base 必备），用它在启动时解析 `appSecretRef`。

### 单会话独立提示词（`conversationPrompts`）

除了全局的 `groupPrompt`/`directPrompt`，还可以给**单个私聊/群聊**追加独立提示词（不替换全局提示词）：

```yaml
conversationPrompts:
  - scope: c2c
    peerId: '用户 openid'      # 私聊对端
    prompt: '这个人喜欢简短回答，不要用列表。'
  - scope: group
    peerId: '群 openid'        # 群聊对端
    prompt: '本群是技术群，回答先给结论，代码必须可运行。'
```

- 命中当前会话 `scope + peerId` 时，`prompt` 追加在 `groupPrompt`/`directPrompt` 之后（空行分隔），未命中不注入。
- `bots[]` 条目写 `conversationPrompts` 时整体覆盖顶层列表（写 `[]` 表示该 bot 不追加任何会话级提示词）。
- 校验 fail-fast：`scope` 必须是 `c2c`/`group`、`peerId` 非空、`prompt` 非空且 ≤80000 字符、同一列表
  `(scope, peerId)` 不得重复；非法配置会让插件加载失败并指出具体条目。
- 也可在设置 →「QQ 机器人」的配置页用「单会话提示词（JSON）」文本框编辑（同一套校验，保存后重载生效）。

## 配置项

| 配置 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `appId` | string | **必填** | QQ Bot AppID（或通过 `QQBOT_APPID` 环境变量） |
| `appSecret` | string | **必填** | QQ Bot AppSecret（或通过 `QQBOT_SECRET` 环境变量） |
| `provider` | string | `deepseek-official` | LLM 提供商名称 |
| `model` | string | `deepseek-chat` | 模型名称 |
| `preset` | string | - | Agent preset id |
| `cwd` | string | `process.cwd()` | Agent 工作目录 |
| `entity` | string | - | 绑定的 NPC 实体 id（dsh-astrbox 人设/状态；空 = 不绑定；中文合法） |
| `requireMention` | boolean | `true` | 群聊是否需要 @bot 才触发 |
| `groupPrompt` | string | - | 群聊额外 system prompt |
| `directPrompt` | string | - | 私聊额外 system prompt |
| `conversationPrompts` | array | `[]` | 单会话独立提示词：`[{scope: 'c2c'\|'group', peerId, prompt}]`；命中当前会话 `scope+peerId` 时把 `prompt` **追加**在 `groupPrompt`/`directPrompt` 之后（空行分隔），不替换原提示词；`peerId` 私聊=用户 openid、群聊=群 openid；`prompt` 1–80000 字符；同一列表 `(scope,peerId)` 不得重复；`bots[]` 条目可整体覆盖 |
| `owner` | object | - | 主人/管理员身份：`{openid, name?}`（`openid` 必填；`name` trim 后默认「主人」且 ≤64 字符；`bots[]` 条目可覆盖，顶层继承）。配置后逐轮注入对方身份行，并在命令 allowlist 模式下放行该 openid；`null` = 不设置 |
| `textChunkLimit` | number | `4500` | 单条消息最大字符数 |
| `streaming` | boolean | `true` | 是否启用流式输出（群聊始终不启用） |
| `sessionIdleTimeout` | number | `1800000` | 会话闲置超时(ms)，默认 30 分钟；`<=0` 表示不回收 |
| `processingTimeoutMs` | number | `1800000` | 处理超时(ms)，超时中断当前 LLM 调用 |
| `maxQueue` | number | `20` | 并发队列最大长度 |
| `historyLimit` | number | `10` | 群历史缓冲条数 |
| `mainSessionKickoff` | string | 系统提示「主会话已启动…」 | 主会话**首次**启动时主动 followup 的提示词，让 Neko 主体产生第一轮对话（否则空会话被工作区隐藏）；空串 = 不主动发送；`bots[]` 条目可覆盖 |
| `askTimeoutMs` | number | `300000` | 待答问题超时(ms)，默认 5 分钟（ask_user_question） |
| `showToolResults` | boolean | `false` | 是否展示工具调用成功结果（错误始终展示） |
| `debug` | boolean | `false` | 调试模式 |

### 访问控制（access）

| 配置 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `access.c2cMode` | `open`/`allowlist`/`disabled` | `open` | 私聊访问模式 |
| `access.c2cAllow` | string[] | `[]` | 私聊白名单（user openid） |
| `access.groupMode` | `open`/`allowlist`/`disabled` | `open` | 群聊访问模式 |
| `access.groupAllow` | string[] | `[]` | 群聊白名单（group openid） |

### 主动推送（proactiveDelivery）

QQ 入站触发的回合始终被动回复；定时任务等服务回合始终主动推送到 QQ。下表只控制「非 QQ 输入（如桌面续聊 QQ 会话）」的回合：

| 配置 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `proactiveDelivery.externalTurns` | `off`/`c2c`/`all` | `c2c` | `off` 不推送；`c2c` 仅推送私聊；`all` 私聊和群聊都推送 |

### 富媒体理解（media）

| 配置 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `media.enabled` | boolean | `true` | 是否启用富媒体理解（图片/视频下载 + 工具分析） |
| `media.maxMB` | number | `200` | 富媒体下载大小上限(MB) |
| `media.ttlHours` | number | `24` | 富媒体存活时长(小时)，0=永不过期 |

### 视觉理解（vision）

| 配置 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `vision.enabled` | boolean | `false` | 是否启用视觉理解（qqbot_describe_image 工具） |
| `vision.provider` | string | - | 视觉模型 provider（如 pi-ai） |
| `vision.model` | string | - | 视觉模型 id（如 qwen-vl-max） |
| `vision.maxBytes` | number | `10MB` | 图片字节上限 |
| `vision.maxTokens` | number | `1024` | 输出 token 上限 |
| `vision.timeoutMs` | number | `120000` | 视觉调用超时(ms) |

### 附件发送（sendFile）

| 配置 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `sendFile.restrictPaths` | boolean | `true` | 是否启用路径白名单（仅 media + cwd + extraRoots） |
| `sendFile.extraRoots` | string[] | `[]` | 额外允许访问的根目录 |

## 内置命令

| 命令 | 说明 |
|------|------|
| `/new`（别名 `/reset` `/clear`） | 开始新会话（清空上下文） |
| `/compact` | 压缩会话历史（摘要替换旧记录，保留上下文） |
| `/model` | 查看或切换模型 |
| `/preset` | 查看或切换 agent preset（新会话生效） |
| `/stop` | 中止当前生成 |
| `/bot-ping` | 网络延迟检测（返回传输与处理耗时） |
| `/bot-version` | 查看版本信息 |
| `/bot-status` | 查看当前会话状态 |
| `/bot-help` | 查看所有指令 |

## 核心模块

```
src/
├── index.ts                    # Cordis 插件入口（async apply）
├── config.ts                   # 配置 Schema
├── types.ts                    # 全局类型定义
├── setup/                      # 凭据绑定（扫码）
│   ├── index.ts                # 绑定编排（ensureCredentials）
│   ├── binding-service.ts      # 扫码状态机（宿主内唯一事实源）
│   ├── bind-routes.ts          # 端内绑定页路由（/qqbot/im-qqbot/bind）
│   ├── presenters.ts           # 呈现策略（终端/浏览器降级）
│   ├── qr-svg.ts               # 二维码 SVG 渲染（宿主侧）
│   └── persist.ts              # 凭据写入 cordis.patch.yml
├── client/                     # 浏览器半边（dsh.client Web Bundle）
│   ├── index.ts                # 侧栏 Tab 注册 + 待绑定自动引导
│   └── bind-page.ts             # 端内绑定页（二维码 + 状态轮询）
├── gateway/                    # 网关装配
│   ├── bootstrap.ts            # 启动装配（监听 session/event 等）
│   └── middleware-setup.ts     # 中间件链配置
├── transport/                  # 传输层
│   ├── inbound.ts              # QQ 入站消息 → agent.followup()
│   ├── outbound.ts             # session/event → QQ sendMarkdown
│   ├── outbound-buffer.ts      # 流式缓冲
│   ├── streaming-writer.ts     # 流式写入
│   ├── reply-target.ts         # 回复目标解析
│   ├── msgid-cache.ts          # 被动回复 msgid 缓存
│   ├── reply-limiter.ts        # 被动回复限流
│   ├── tool-presenter.ts       # 工具调用展示
│   └── chunker.ts              # Markdown 文本切分
├── session/                    # 会话管理层
│   ├── session-manager.ts      # QQ peer → Agent 映射
│   └── idle-evictor.ts         # 闲置回收
├── model/                      # 模型路由层
│   ├── model-resolver.ts       # 路由解析
│   ├── prefs-store.ts          # per-peer 偏好持久化
│   └── settings-reader.ts      # settings.yaml 只读
├── features/                   # 交互特性
│   ├── question-channel.ts     # ask_user_question 问答通道
│   ├── approval-channel.ts     # approval 审批确认通道
│   └── answer-parser.ts        # 答案解析
├── media/                      # 多媒体
│   ├── vision-tool.ts          # 图片视觉理解
│   ├── send-file-tool.ts       # 发送本地文件
│   └── media-cleaner.ts        # 媒体 TTL 清理
├── middleware/                 # 中间件
│   ├── question-answer.ts      # 问答答案处理
│   └── attachment.ts           # 附件处理
├── shared/                     # 共享工具
│   ├── utils.ts                # 通用函数
│   ├── scope.ts                # scope/peer 提取
│   └── send-helper.ts          # 分块发送
└── commands/                   # 斜杠命令
```

## 会话路由

sessionKey: `qqbot:${appId}:${scope}:${peerId}`，新会话铸造 UUID 会话 id 并持久化到 `model-prefs.json`，重启后按持久化 id 恢复；0.5.x 老会话按原派生 id 沿用。

解析策略：进程内复用 → 持久化恢复 → 全新创建。

QQ 群聊和私聊的补充提示词按当前 Agent 作用域加载，恢复会话时继续使用对应场景的配置。

## 设计原则

- **纯 Cordis 插件** — 遵循 dsh "Plugins, not loop changes" 原则
- **声明式依赖** — `inject = ['agents']`，不直接耦合其他插件
- **会话隔离** — 每个 QQ 私聊用户/群聊各一个独立 Agent
- **Preset 支持** — 可通过 `agent-presets` 服务挂载预设（工具集、prompt 等）
- **闲置回收** — 超时自动 dispose Agent，防止内存泄漏（回收后仍可按会话映射主动推送）
- **Markdown 输出** — 回复以 Markdown 格式发送，支持代码块/表格感知切分
- **图片理解** — 支持 `qqbot_describe_image` 视觉理解，可分析用户发送的图片
- **附件发送** — 支持 `qqbot_send_file` 将本地文件发送给用户（默认启用路径白名单）
- **问答互动** — 支持 `ask_user_question`，单选生成内联按钮（点一个其余变灰）、多选回复编号，逐题推进 + 问题级超时
- **操作确认** — 支持 `approval/request`，关键操作通过「允许/拒绝」按钮请求用户确认

## 本地开发

```bash
# 安装依赖
pnpm install

# 构建
pnpm build

# 开发模式（watch）
pnpm dev

# 用 --patch 方式调试
export QQBOT_APPID="xxx" QQBOT_SECRET="xxx"
npx @deepseek-ai/dsh web --patch /path/to/dsh-qqbot/cordis.dev.yml
```

## License

[MIT](./LICENSE)
