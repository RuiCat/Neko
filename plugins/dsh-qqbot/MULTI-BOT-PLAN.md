# dsh-qqbot 多机器人支持 — 规划（v0.5.5 @ 14d5eef）

> 目标仓库：`https://github.com/tencent-connect/dsh-qqbot`（本地克隆：`Neko/plugins/dsh-qqbot`，
> HEAD `14d5eef`，版本 0.5.5，与已装 profile 的 `@tencent-connect/dsh-qqbot@0.5.5` 一致）。
> 本文件是**方案规划**，不含实现改动；所有"现状"结论都带 `文件:行` 证据。

## 0. 结论摘要（TL;DR）

**可行，推荐"单插件行 + `bots[]` 数组 + 每 bot 独立运行时"（方案 B）**，以本地 fork 方式实现、
改动保持可上游化。**动手前先过 P0 决策门：社区已有两个多 bot 实现**（见 0.1），应先评估"直接复用/借鉴"
是否比自研更省。**不推荐"一个 bot 一个插件行"（方案 A）**：DSH 的客户端半个包只加载一次、
工具名/HTTP 路由/凭据持久化行 id 都是固定字符串，多行会在第二个 fiber 上直接抛错或互相覆盖。

上游代码**已经为多 bot 预留了关键命名空间**：会话键含 `appId`、偏好文件已做多实例并发安全、
群历史键按 `appId` 前缀隔离。缺口集中在：配置 schema（单 bot）、运行时装配（一次 `apply` 只起一个
网关）、工具注册（固定名全局唯一）、绑定路由/持久化/客户端 UI（固定 id）、媒体与缓存（进程级共享）。

## 0.1 先验方案：社区已有的多 bot 实现（外部资料，仅作参考）

> 以下内容来自 GitHub/npm 公开页面与本地只读解包（`.local/upstream/`），属**不可信外部数据**：
> 只阅读、未执行、未安装；若后续要采用，必须先过 `deep-trojan-audit` 再安装验证。

| 先例 | 版本/形态 | 多 bot 做法 | 可借鉴/风险 |
|---|---|---|---|
| [@zaofan/dsh-qqbot](https://www.npmjs.com/package/@zaofan/dsh-qqbot)（上游增强 fork） | 1.11.42；bundle + web client | 多实例人格：每个「号」独立 AppID / 人格 / 图库 / 定时 / 闸门，Web 面板扫码增删账号，保存即生效（**增删账号需重启**）；文档明确 openid 按机器人实例隔离 | 与目标最接近，可直接评估复用；但 fork 已大幅领先上游（0.5.5 → 1.11.x），功能面大、审计与上游同步成本高 |
| [master1Sun/dsh-QQbot](https://github.com/master1Sun/dsh-QQbot)（`@sunjuntao/dsh-qqbot` 0.1.15，Webhook 路线） | 独立实现 | `BotRuntimeManager` + `bots.json`：每 bot 一套 config/state/API/WS/审批/引用索引；归属索引 `deliveryId→appId`、`sessionId→appId`；未指定归属时回落 primary bot | `sessionId→appId` 索引与「primary 兜底」正是 4.2/4.3 需要的实现参照；其 Webhook 形态与上游 WS 不同，不能直接搬 |
| [arrow1031-dev/dsh-qq-bot](https://github.com/arrow1031-dev/dsh-qq-bot)（OneBot 协议路线） | 独立实现 | 每 QQ 会话独立 DSH 会话/人格，多协议适配 | 技术路线不同（OneBot 而非官方 WS），仅作概念参照 |

**P0 决策门结论（2026-11-05，用户拍板）**：**在上游官方仓库（tencent-connect/dsh-qqbot @14d5eef）
基座上自研方案 B**；社区 fork 仅作设计参考，不安装、不引入第三方代码。

## 1. 范围与假设（可推翻）

| # | 假设 | 推翻条件 |
|---|---|---|
| A1 | 一个 DSH 实例内同时运行 **N 个 QQ 机器人**（N 个 AppID/AppSecret），共享同一 Host/模型路由 | 若要求 N 个独立 DSH 进程，则方案 C 足够 |
| A2 | 每个 bot 可独立配置 **preset / provider / model / cwd / prompt / 访问白名单 / 媒体策略** | 若全部共享同一角色，配置层可简化 |
| A3 | 管理面 = GUI 列表（增删/扫码绑定/解绑/启停/状态）+ 配置文件可手工编辑，二者一致 | 若只要 headless 配置，可砍掉客户端改动 |
| A4 | 每个 bot 在 QQ 开放平台是独立 AppID（平台侧"一个机器人 = 一个 AppID"，需在 q.qq.com 侧复核） | 若平台支持一个 AppID 多实例，凭据模型另议 |
| A5 | 交付形态：本地 fork 先跑通，改动按上游风格写成可 PR 的形态 | 若只做内部魔改，可放宽兼容层 |

**不做什么**：不改 QQ 平台协议；不引入多进程/多 profile 编排；不改变单 bot 现有配置的语义与工具名
（`qqbot_send_file` / `qqbot_describe_image` 保持可用）。

## 2. 现状架构与证据

单 bot 的装配链（`apply` → `bootstrapGateway`）：

- 配置只有单 bot 字段：`src/config.ts:70-72`（`appId`/`appSecret`）、`src/config.ts` 其余为单 bot 运行参数；
- 凭据支持环境变量回退（单值）：`src/index.ts:28-29`（`QQBOT_APPID` / `QQBOT_SECRET`），`src/index.ts:81` 起一个网关；
- 一次 `bootstrapGateway` 创建一套：`SessionManager`、`QQBot` SDK、中间件链、出站、问答/审批通道
  （`src/gateway/bootstrap.ts:46` 起）；全局监听各注册一次：`session/event`（`:128`，`{ global: true }`）、
  `agent/assistant-stream`（`:135`）；
- 工具注册：`qqbot_send_file`（`src/media/send-file-tool.ts:20`，注册于 `:172`）、
  `qqbot_describe_image`（`src/media/vision-tool.ts:339`）——**固定名字、全局唯一**；
- 富媒体清理每个网关启动一份（`src/gateway/bootstrap.ts:188`），根目录进程级固定（`src/media/media-cleaner.ts:15`）；
- 凭据持久化硬编码插件行 id：`src/setup/persist.ts:37`（`ENTRY_ID = 'im-qqbot'`）；
- 绑定页 HTTP 路由固定前缀：`src/shared/bind-protocol.ts:11-23`（`/qqbot/im-qqbot/...`）；
- 客户端席位固定 id：设置页 `id: 'qqbot'`（`src/client/index.ts:133`）、右栏 Tab `im-qqbot/bind`（`:27`）、
  主面板 key `im-qqbot`（`:31`）。

**已具备多 bot 基础**（上游预留，值得直接复用）：

- 会话键按 `appId` 隔离：`src/session/session-manager.ts:436-438`
  （`qqbot:${appId}:${scope}:${peerId}`），注释在 `:5-13`；
- 偏好文件（sessionKey→sessionId、模型、preset）已做**同进程多 bot / 跨进程多 profile** 并发安全：
  `src/model/prefs-store.ts:5-13`（原子写 + 写前合并 + 删除墓碑；CHANGELOG 0.5.4 已记录）；
- 群历史缓冲键带 `appId` 前缀，注释明确"保留多账号扩展"：`src/features/history-store.ts:18`；
- 出站路由按 `SessionManager` 过滤（`src/transport/outbound.ts:127` `manager.findBySessionId`，
  且 `turn-tracker` 只认本实例登记过的回合），因此多实例全局监听不会把别的 bot 的回合发错。

## 3. 方案对比

| 方案 | 做法 | 硬阻塞（证据） | 结论 |
|---|---|---|---|
| **A. 一个 bot 一行（多插件行）** | 复制 `im-qqbot` 行为 `im-qqbot-2`…，各带独立 config | 工具重名：DSH 工具表同层重名直接抛 `tool "…" is already registered`（`deepseek-harness/packages/core/tools/src/index.ts:69`）；绑定路由重复：宿主 `duplicate route` 抛错（`packages/host/webserver/src/index.ts:448`）；持久化写同一个 `im-qqbot` 条目（`persist.ts:37`）→ 互相覆盖；客户端半按**包名**只加载一次（`packages/client/modules/src/index.ts:1083-1093`）→ 无法按行出 UI；`QQBOT_APPID` 环境回退单值（`index.ts:28`） | ❌ 不推荐（要修的点不比 B 少，还得引入"组长/从属行"之类的丑结构） |
| **B. 单行 + `bots[]` 数组 + 每 bot 运行时**（推荐） | 一行配置里声明 N 个 bot；插件内部 `BotCoordinator` 为每个 bot 建一套运行时；工具/全局监听/路由各注册一次，按 bot 路由 | 需要重构装配与 UI；无结构性阻塞 | ✅ 推荐 |
| C. N 个 DSH 进程/ profile | 每个 bot 一个 profile，零插件改动 | 运维重（N 份 Host、端口、内存）、无统一管理、跨 bot 无法共享同一会话/模型配置 | ⚠️ 仅作短期兜底 |

## 4. 推荐设计（方案 B）

### 4.1 配置模型（向后兼容）

```yaml
- id: im-qqbot
  name: '@tencent-connect/dsh-qqbot'
  config:
    defaults:                 # 可选：所有 bot 的公共默认（缺省沿用现 schema 默认值）
      requireMention: true
      sessionIdleTimeout: 1800000
    bots:
      - id: cat               # 稳定标识：存储/路由/工具归属都用它（[a-z0-9-]{2,32}，唯一）
        label: 猫猫
        enabled: true
        appId: '...'
        appSecret: '...'
        preset: neko          # 每 bot 独立角色（agent preset）
        provider: deepseek-official
        model: deepseek-flash
        cwd: /path/to/Neko
        groupPrompt: ...
        directPrompt: ...
        access: { c2cMode: allowlist, c2cAllow: [...], groupMode: open, groupAllow: [] }
        media: { enabled: true, maxMB: 200, ttlHours: 24 }
        vision: { ... }
      - id: helper
        label: 助手
        ...
```

兼容规则（`src/config.ts` + 新 `normalizeConfig`）：

1. 顶层旧字段（`appId`…）保持可读：**没有 `bots` 时**合成为 `bots: [{ id: 'default', ...顶层字段 }]`，
   日志提示"建议迁移到 bots[]"；`defaults` 与单 bot 顶层字段同 key 时以顶层为准（老配置零改动可用）。
2. 校验并在启动时 fail-fast（或单 bot 降级 + 明确报错）：`id` 唯一且合法、`appId` 非空、`appId` 不重复；
   合法 bot 才启动，非法项进 status/日志（不能静默丢弃）。
3. 凭据来源优先级：bot 字段 > 环境变量（仅当**只有一个 bot** 时才允许 env 回退，多 bot 下禁用并 warn）。
4. 密钥存储：短期沿用 profile `cordis.patch.yml`（与现状一致）；中期建议改存 DSH credential store
   （`ctx.credentials`），配置里只留 `credentialRef`，避免 N 份明文密钥扩散（列为 P4 可选项）。

### 4.2 运行时结构

```text
apply()
  └─ normalizeConfig(config) → { defaults, bots[], warnings }
  └─ BotCoordinator
       ├─ 注册一次：qqbot_send_file / qqbot_describe_image（带 bot 归属解析）
       ├─ 注册一次：session/event、agent/assistant-stream（{global:true}）→ 按 sessionId→botId 分派
       ├─ 注册一次：HTTP 路由（/qqbot/bots…，见 4.4）
       └─ bots.forEach(bot => BotRuntime)
            ├─ QQBot（各自 AppID 的 WebSocket）
            ├─ SessionManager（config = defaults ⊕ bot；sessionKey 已含 appId）
            ├─ outbound / TurnTracker / QqInputRegistry / ReplyLimiter
            ├─ QuestionChannel / ApprovalChannel
            └─ 媒体清理（per-bot media 根）
```

- `bootstrapGateway` 从"插件级一次"重构为"每个 bot 一次"：把 `botId`/`BotRuntimeHandle` 传下去；
  全局监听与工具注册上移到 coordinator，避免 N 份重复监听（当前 `bootstrap.ts:128,135` 是每实例注册）。
- 出站分派：`session/event` 先经 coordinator 用 `sessionId → botId`（各 runtime 的 manager 注册）选 runtime；
  未命中直接忽略（桌面会话/其它插件会话）。保留各 runtime 内部的 `turn-tracker` 过滤，行为与现状等价。
- 生命周期：`ctx.effect` 里 `coordinator.stopAll()`；HMR 重复 apply 由 coordinator 的"先停旧再起新"幂等；
  QQ 连接、定时器、路由、工具 disposer 全部挂在 fiber 上。
- 单点状态：`activeService`（绑定轮询单例）、`msgid-cache`（`src/transport/msgid-cache.ts:31` 进程级 Map）
  按"仅 coordinator/共享一份"使用或按 `botId` 加键前缀，避免跨 bot 串号。

### 4.3 工具归属（单注册、按会话定位 bot）

- `qqbot_send_file` / `qqbot_describe_image` 仍各注册一次（保持模型可见名不变，兼容旧提示词）。
- 新增可选参数 `bot?: string`：
  - 显式给 `bot` → 用该 bot 的发送器；
  - 未给 → 用**调用方 agent 的 sessionId** 反查 owning bot（coordinator 维护 `sessionId → botId`）；
  - 反查不到且只有 1 个 bot → 回退该 bot；多于 1 个 bot → 明确报错"无法确定发送机器人，请传 bot 参数"。
- 归属表在 `BotRuntime` 创建/回收 session 时维护（或复用各 manager 的 `findBySessionId` 反向扫描，
  N 很小，先扫后建索引均可）。

### 4.4 绑定 / 配置 / 持久化 / 客户端

- 路由参数化：`/qqbot/bots`（列表+状态）、`/qqbot/bots/:id/bind`、`/qqbot/bots/:id/bind/{refresh,manual,unbind}`、
  `/qqbot/bots/:id/config`（`src/shared/bind-protocol.ts` 改为函数生成；旧固定路径保留为
  "指向 default/首个 bot"的 alias，迁移期两版客户端都能用）。
- 持久化：`persist.ts` 的编辑目标从"条目 config 根"改为"条目 config.bots[id]"（upsert/删除），
  文档保真、原子写、读回校验四条约束全部保留；旧格式（无 bots）仍按原路径写回，保持兼容。
- 客户端：设置页「QQ 机器人」一个 section，内部渲染 bot 列表（label / 状态 / 已绑定 AppID 尾号 /
  启停 / 重新扫码 / 解绑 / 新增 / 删除）；右栏绑定页加 bot 选择器；二维码轮询按 bot 发请求。
  席位 id 保持单套（`settings.section` id `qqbot`、主面板 `im-qqbot`），不新增席位，避免升宿主时踩契约。
- 状态命令：`/status` 等命令回答"当前 bot + 全部 bot 概览"，避免多 bot 下语义不清。

### 4.5 存储与隔离

| 数据 | 现状 | 多 bot 方案 |
|---|---|---|
| 会话键 | 已含 `appId`（`session-manager.ts:436`） | 不变；`bots[].id` 与 appId 一一对应校验 |
| `model-prefs.json` | 全局、多写者安全（`prefs-store.ts:5-13`） | 不变（key 已含 appId）；可留 `botId` 索引便于排障 |
| 群历史 | key 含 appId（`history-store.ts:18`） | 不变 |
| 媒体目录 | 全局 `~/.dsh-qqbot/media`（`media-cleaner.ts:15`） | 改为 `~/.dsh-qqbot/bots/<botId>/media`；清理器按 bot 根各起一份，避免 A 的清理器删 B 的文件 |
| agent 工作目录 | 全局 `PLUGIN_HOME/workspace`（`dirs.ts`） | 默认 `PLUGIN_HOME/bots/<botId>/workspace`；显式 `cwd` 优先 |

## 4.6 实施状态（本仓库）

- **P1 配置层：已完成**（2026-11-05）
  - `src/config.ts`：新增 `BotEntryConfig` / `BotEntrySchema` 与 `bots[]`（空数组 = 旧单 bot 语义不变）；
    `ImQQBotConfig` 增加 `bots` 字段。
  - `src/configBots.ts`：`normalizeBots()` 统一「旧顶层单 bot」与「bots[]」两种输入；
    校验 id 格式/唯一、appId 唯一（trim 后），缺 appId 的 bot 停用并告警，多机器人模式下忽略
    `QQBOT_APPID/QQBOT_SECRET` 并告警。
  - `src/index.ts`：运行时接入前 **fail-fast**（检测到 `bots[]` 即报错，避免静默退回单 bot）；
    P2 会用 BotCoordinator 替换该守卫。
  - 测试：`src/configBotsTest.ts`（10 例）；`vitestConfig.ts` + `package.json#test` 让 camelCase
    命名的 `*Test.ts` 参与测试发现（新文件遵守命名规范）。
  - 验证：`npx tsc --noEmit -p tsconfig.json` 通过；`npm test` **32 文件 / 381 用例全绿**；
    `lintCodeModularity` 对新文件与改动文件 **0 违规 0 告警**。
- **P2 运行时：已完成**（2026-11-05）
  - `src/gateway/bootstrap.ts`：`bootstrapGateway` 重构为 `createBotRuntime()`（每个 bot 一套
    QQBot/SessionManager/出站/问答/审批/生命周期），不再自行注册全局工具。
  - `src/gateway/botCoordinator.ts`（新增）：`startBotCoordinator()` 逐 bot 装配（单个失败隔离）、
    `BotCoordinator` 按 agent/session 解析归属（`sessionId→bot`）；全局工具只注册一次：
    `qqbot_send_file` 按归属 bot 的 sender/manager/白名单发送，`qqbot_describe_image` 按归属 bot 取
    `vision` 配置；视觉 settings 补写按 (provider,model) 去重；媒体清理只启一份（P4 再按 bot 分根）。
  - `src/media/send-file-tool.ts` / `src/media/vision-tool.ts`：注册接口改为 `lookup`（byAgent/fallback），
    执行期解析归属机器人，工具名保持全局唯一（兼容旧提示词）。
  - `src/features/question-channel.ts`：`userQuestions.ask` 补丁改为进程内**一次补丁 + 多通道分派**
    （`__qqChannels` 挂在 service 上），多 bot 按 `request.agent.id` 归属路由；全部卸载后恢复原 ask。
  - `src/index.ts`：旧单 bot 配置走同一 coordinator（行为与上游一致）；`bots[]` 多机器人直接启动，
    缺凭据/占位符的 bot 在归一化层停用并告警；扫码绑定仍仅旧单 bot 路径（每 bot 绑定属 P3）。
  - 测试：`src/gateway/botCoordinatorTest.ts`（5 例：归属解析/单 bot 失败隔离/工具只注册一次/空列表）、
    `src/features/questionChannelMultiBotTest.ts`（2 例：按归属分派、卸载恢复）。
  - 验证：`npx tsc --noEmit -p tsconfig.json` 通过；`npm test` **34 文件 / 390 用例全绿**；
    新增文件 lintCodeModularity **0 违规 0 告警**（存量文件 `send-file-tool.ts`/`vision-tool.ts`
    的 kebab-case 与 export-doc 告警为上游既有，非本次引入）。
- **P3a 宿主侧：已完成**（2026-11-05）
  - `src/shared/bind-protocol.ts`：新增 `botBindPaths(botId)`（`/qqbot/bots/<id>/bind|bind/refresh|bind/manual|bind/unbind|config`），旧固定路径常量保留。
  - `src/setup/bind-routes.ts`：`registerBindRoutes` 增加可选 `paths`（默认旧路径），同一套 handler 服务单/多 bot。
  - `src/setup/persist.ts`：新增 `persistBotCredentialsToProfile` / `removeBotCredentialsFromProfile` /
    `updateBotProfileConfig`——按 botId 对 `config.bots[]` upsert/删除/合并，保留注释、其它条目与文档保真；
    重复 id fail-closed；解绑删凭据并置 `enabled:false`。
  - `src/setup/index.ts`：`registerBotCredentialRoutes()` 为每个 bot 注册独立路由组（状态/手动凭据/解绑/配置）；
    `src/index.ts` 多机器人启动后调用。
  - 测试：`src/setup/persistBotsTest.ts`（5 例：upsert/新增/重复 id fail-closed/解绑/配置合并）、
    `src/setup/botRoutesTest.ts`（2 例：路径按 bot 隔离、状态与配置只回显自己、缺 webServer 静默跳过）。
  - 验证：`npx tsc --noEmit` 通过；`npm test` **36 文件 / 397 用例全绿**。
- **P3b 客户端基础版：已完成**（2026-11-05）
  - 宿主新增 `GET /qqbot/bots`（`registerBotsListRoute`）：只返回 `id/label/appId/enabled`，appSecret 永不出现。
  - `src/client/bind-page.ts`：绑定路径改为 `BindPathsContext`（默认旧固定路径，多 bot 传
    `botBindPaths(id)`），状态轮询/刷新/手动提交/解绑全部按作用域路径走。
  - `src/client/config-panel.ts`：新增 `configPath` 属性（默认旧路径），多 bot 指向
    `/qqbot/bots/<id>/config`。
  - `src/client/index.ts`：设置页拉 `/qqbot/bots`；有列表时渲染 bot 选择器 + 该 bot 的绑定面板与配置面板
    （每个 bot 独立轮询/保存），无列表时回退旧单 bot 视图；右栏绑定页保持旧路径（多 bot 下不显示）。
  - 构建：`npm run build` 通过，`lib/client.js`（tracked 产物）已重建（55KB，含 `/qqbot/bots` 路径）。
  - 验证：host `tsc --noEmit` rc=0；`tsc -p tsconfig.client.json --noEmit` rc=0；`npm test` **36 文件 / 397 用例全绿**。
- **P4 部分完成**（2026-11-05）
  - 设置页「新增/删除机器人」：宿主 `POST /qqbot/bots`（新增空凭据、enabled=false 条目）与
    `POST /qqbot/bots/delete`（整项删除，HMR 让运行时下线）；客户端 bot 工具栏按钮 + prompt/confirm；
    新增后通过该 bot 页手动绑定凭据即启用。
  - `src/setup/persist.ts`：`addBotToProfile` / `removeBotFromProfile`（文档保真、幂等、重复 id 返回 false）。
  - 验证：host/client tsc rc=0；`npm test` **401 用例全绿**；`npm run build` 重建 tracked `lib/`；
    新增文件 lint 0/0。
- **P4b 工作目录隔离：已完成**（2026-11-05）
  - `src/shared/dirs.ts`：新增 `botHome(botId)`；`resolveAgentCwd(cwd, botId?)` 在多 bot 下缺省为
    `~/.dsh-qqbot/bots/<botId>/workspace`，无 botId 保持旧 `PLUGIN_HOME/workspace`（向后兼容）。
  - `createBotRuntime(..., botId)` → `SessionManager` / `DeliverablesDeliverer` / `qqbot_send_file`
    路径白名单全部按 bot 解析；`BotCoordinator` 按 botId 装配。
  - 测试：`src/shared/dirsTest.ts`（4 例）；`botCoordinatorTest` 断言 createRuntime 收到正确 botId；
    `npm test` **405 用例全绿**。
- **P4c 富媒体目录隔离：已完成**（2026-11-05）
  - `src/shared/dirs.ts`：`resolveMediaRoot(botId?, legacy)`——多 bot 用 `bots/<id>/media`，legacy/无 botId 保持旧 `PLUGIN_HOME/media`。
  - `src/transport/attachment.ts` / `src/middleware/attachment.ts` / `src/gateway/middleware-setup.ts`：
    下载目录参数化；`crecreateBotRuntime` 计算 `mediaRoot` 并贯穿中间件。
  - `src/media/media-cleaner.ts`：清理函数与定时器接收 `mediaRoot`；`BotCoordinator` 为每个启用 media 的 bot
    各起一份清理（legacy 单 bot 仍用旧全局目录）。
  - `src/media/send-media.ts` / `send-file-tool.ts` / `deliverables.ts`：路径白名单按 bot 的 media 根 + workspace。
  - 测试：`dirsTest` 增补媒体根用例；`npm test` **406 用例全绿**；`npm run build` / client tsc 通过。
- **P4d 右栏切换 + 文档：已完成**（2026-11-05）
  - `src/client/index.ts`：右栏绑定页改为 `RightPaneBody`——有 `/qqbot/bots` 列表时先选 bot 再显示该 bot 的绑定面板，
    旧单 bot 保持原样；客户端 tsc 通过，`npm run build` 重建 `lib/client.js`。
  - `README.md` / `README_EN.md` 新增「多机器人（bots[]）」章节；`CHANGELOG.md` / `CHANGELOG_EN.md` 未发布段补条目。
- **P4e 每 bot 扫码绑定：已完成**（2026-11-05）
  - `src/setup/botBindingRegistry.ts`（新增）：按 bot id 懒创建 `BindingService`（首次状态轮询触发）、
    HMR 双 fiber 接管（旧服务先终止）、扫码成功后经注入的 persist 回调写回该 bot；写盘失败转 `failed` 相位，
    不再永久停在 `saving`。
  - `src/setup/bind-routes.ts`：`registerBindRoutes` 的服务参数改为 `BindingServiceSource`
    （实例 / null / 惰性工厂）；已绑定 bot 的 `bind/refresh` 返回 409，不会误建扫码任务；
    manual / unbind 回调允许返回 Promise（为凭据库写入留异步路径）。
  - `src/setup/index.ts`：`registerBotCredentialRoutes` 为全部 configured bot 注册路由（含未绑定/停用项），
    `GET /qqbot/bots` 回报真实 `enabled`；扫码服务经 registry 注入，凭据库适配层可透传。
  - `src/index.ts`：多 bot 分支改为「全部 configured bots 注册路由、仅 enabled 启动运行时」——修复此前
    新增/解绑后该 bot 从列表消失、无法扫码的缺陷。
  - `src/client/index.ts`：`useBotList` 15s（页面可见时）刷新，标签区分「（未绑定）/（停用）」。
  - 账号昵称：`src/gateway/accountInfo.ts` 从网关 READY 负载提取 `user.username`/`user.id`，运行时句柄 `account` 暴露；
  `GET /qqbot/bots` 与 per-bot `bind` 状态携带 `name`，客户端优先显示昵称（未 READY 回退 label）。
- 测试：`botBindingRegistryTest.ts`（7 例：懒创建/写回/失败/异常/refresh/dispose/接管）、
    `botRoutesTest.ts` 扩充（未绑定可达扫码路由、已绑定刷新 409、列表真实 enabled）。
- **P4f 密钥改存凭据库：已完成**（2026-11-05）
  - `src/setup/credentialStore.ts`（新增）：`botSecretRef`（`QQBOT_<ID>_SECRET`）、`resolveConfigSecrets`
    （明文优先、`appSecretRef` 经凭据库解析、无 seam 回退同名环境变量）、`storeBotSecret`/`removeBotSecret`、
    `awaitCredentials`（可选注入 `ctx.credentials`，窗口内未提供按无凭据库继续）。
  - `src/config.ts` / `src/configBots.ts`：`BotEntryConfig.appSecretRef` 与 `EffectiveBot.appSecretRef`。
  - `src/setup/persistBots.ts`（新增，从 persist.ts 拆出 bots[] 持久化）：`persistBotCredentialRefToProfile`
    只写 `appId` + `appSecretRef` 并删除同条目明文 `appSecret`；解绑同时删 `appSecretRef`；配置合并忽略该键。
  - `src/setup/index.ts`：绑定优先写凭据库（成功则 patch 只留引用，写 patch 失败会回滚引用并回退明文）；
    解绑清理默认引用与已声明引用。
  - `src/index.ts`：静态 `inject = ['agents','credentials']`（cordis 保证 apply 在凭据库 init 完成后运行），
  apply 内 `resolveConfigSecrets` 再归一化启动；解析失败时 per-bot 绑定状态报 failed 并说明原因（不误报 bound）。
  - 测试：`credentialStoreTest.ts`（17 例：命名/解析优先级/env 兜底/写库/清理/可选注入）、
    `persistBotsTest.ts` 新增引用写入与解绑删除用例。
- **P4g 主会话 / 子会话：已完成**（2026-11-05）
  - `src/session/session-manager.ts`：新增主会话（每 bot 一个，key `qqbot:<appId>:main`，读盘 resume/缺失 create，
    cwd 与子会话共用 bot 工作区，无 QQ 回复目标因此不装 QQ 通道提示词）；`ensureMainSession()` 失败只告警不阻塞；
    `disposeAll()` 释放主会话；`mainSessionId` 供排障。
  - 新建 QQ 会话（私聊/群聊）时带 `meta.parentSession = 主会话` + `origin: 'subagent'`（DSH 原生会话树，
    子会话出现在主会话下）；resume 的存量会话保留原 header（历史会话不追溯改父指针）。
  - `src/gateway/bootstrap.ts`：运行时启动即 `void manager.ensureMainSession()`（常驻）。
  - 测试：`session-manager.test.ts` 新增 3 例（主会话创建/落盘/幂等、子会话父指针、主会话失败不阻塞）；
    `npm test` 40 文件 / 440 用例绿。
- **P4 剩余**：安装到测试 profile 真机验证（两个测试 AppID）；真机双 bot 验收（第 6 节 P5 清单）。

## 5. 分阶段实施计划

| 阶段 | 内容 | 产物 | 验收（可执行） | 预估 |
|---|---|---|---|---|
| P0 | 本规划 + 上游版本基线 + **社区 fork 复用评估**（见 0.1） | 本文件、base commit 记录、复用/自研结论 | 文档评审通过 + 决策门有明确结论 | 已做（决策待用户拍板） |
| P1 | 配置层：`bots[]` schema + normalize/迁移 + 校验 + 单测 | `src/config.ts`、新 `src/config-normalize.ts`、`config.test.ts` 增例 | `pnpm test -- config` 全绿；旧配置加载等价 | 1–2 d |
| P2 | 运行时：`BotCoordinator` + `BotRuntime`；bootstrap 重构为 per-bot；全局监听/工具注册上移；工具 bot 归属 | `src/bot/coordinator.ts`、改 `gateway/bootstrap.ts`、`media/*-tool.ts` | 伪 SDK 双 bot 集成测试：互不串、工具归属正确、停用 bot 不联网 | 2–3 d |
| P3 | 绑定/持久化/客户端：路由参数化 + `bots[]` 持久化 + 设置页列表 + 右栏选择器 | `setup/*`、`shared/bind-protocol.ts`、`client/*` | 单测 + 页面冒烟（双 bot 列表、扫一个、另一个不受影响） | 2–3 d |
| P4 | 存储隔离（media/workspace）、密钥存储评估、文档/CHANGELOG/README_EN、`cordis.patch.yml` 示例 | 相应文件 + 文档 | 文档示例可直接抄；`pnpm build` 通过 | 1 d |
| P5 | 真机双 bot 验收（两个 AppID） | 验收记录 | 见第 6 节手工清单 | 0.5 d |

## 6. 测试与验收

**自动化（P1–P4 期间）**

- 单测：`normalizeConfig`（legacy→bots、重复 appId/id 拒绝、defaults 合并、env 回退仅单 bot）；
  `sessionKey` 仍含 appId；工具归属解析（本会话/显式 bot/歧义报错）；`persist` 对 `bots[]` 的 upsert 与
  其他条目/注释保真；路由路径生成与旧 alias。
- 集成（fake `QQBot` SDK + fake agents registry）：两个 bot 各自收消息 → 只回自己会话；查问/审批按钮
  只在本 bot 会话生效；`qqbot_send_file` 从 A 会话发只走 A 发送器；HMR 重复 apply 不产生重复工具/路由；
  停用 bot 无 WS、无轮询。
- 静态：`pnpm typecheck`、`pnpm lint`、`pnpm build`、`pnpm test` 全绿（作为交付门禁）。

**真机手工（P5，两个真实 QQ 机器人）**

1. 两个 bot 同时在线；各自私聊/群聊发消息，回复只出现在对应 bot 的会话；
2. 两 bot 配不同 preset/persona（如猫猫 vs 助手），回复风格/工具集隔离；
3. A 会话里让模型发文件，文件经 A 发送；B 会话不受影响；
4. 重启 DSH：两 bot 凭据与会话映射恢复；解绑 A 不影响 B；重新扫码 A 只改 A；
5. 桌面端续聊 QQ 会话（非 QQ 输入）主动推送仍只回到所属 bot；
6. 群历史/上下文不串（同群不同 bot 各自独立）。

## 7. 风险与开放问题

**风险**

1. **上游分叉维护成本**：改动面跨 config/运行时/客户端；建议每个阶段保持"可单独上游"的小 PR 粒度，
   并在文档记录 base commit（当前 `14d5eef`）。
2. **QQ 平台限制**：每 bot 一条 WS 与独立限流；N 个 AppID 需要 N 个开发者应用；平台侧是否有
   "同主体机器人数量/频率"限制需在 q.qq.com 复核（假设 A4）。
3. **密钥扩散**：N 份 appSecret 明文存 `cordis.patch.yml`；建议 P4 改存 credential store，
   配置只留引用（同时保留现有明文方式做兼容）。
4. **工具归属歧义**：非 QQ 会话（桌面）调用 `qqbot_send_file` 时无归属；多 bot 下必须显式 `bot`，
   否则报错——这是有意的安全行为，需要在工具描述与文档写清。
5. **客户端席位契约**：多 bot 列表复用现有 `settings.section`/`sidebarRightTabs` 席位，避免新增席位；
   宿主大版本升级后按上游既有兼容性声明重新验收。
6. **HMR/单例**：`activeService`、`msgid-cache`、媒体清理定时器在多 bot/重载窗口下需按 bot 隔离或
   单例收敛，否则会出现重复轮询/串号。

**开放问题（需用户拍板）**

1. 每个 bot 是否要独立角色与模型（preset/persona/provider/model/cwd/白名单）？本方案按"要"设计。
2. 管理面：只需 GUI（本方案）还是也要 headless/CLI 管理（例如脚本批量增删）？
3. 交付路径：本地 fork 先跑通再考虑上游 PR（本方案），还是直接按上游贡献流程提 PR？
4. 是否考虑直接采用/借鉴社区 fork（@zaofan 已有多 bot；master1Sun 的 `sessionId→appId` 索引）？
   若采用第三方 fork，需要先做安全审计与版本锁定。

## 8. 附：证据索引

| 断言 | 证据 |
|---|---|
| 单 bot 配置/一次装配 | `src/config.ts:70-72`、`src/index.ts:28-29,81`、`src/gateway/bootstrap.ts:46` |
| 每实例全局监听 | `src/gateway/bootstrap.ts:128,135` |
| 工具固定名 | `src/media/send-file-tool.ts:20,172`、`src/media/vision-tool.ts:339` |
| 工具重名抛错 | `deepseek-harness/packages/core/tools/src/index.ts:69` |
| 路由重复抛错 | `deepseek-harness/packages/host/webserver/src/index.ts:448` |
| 持久化固定行 id | `src/setup/persist.ts:37` |
| 绑定路径固定 | `src/shared/bind-protocol.ts:11-23` |
| 客户端固定 id | `src/client/index.ts:27,31,133` |
| 客户端每包只加载一次 | `deepseek-harness/packages/client/modules/src/index.ts:1083-1093` |
| 会话键含 appId | `src/session/session-manager.ts:5-13,436-438` |
| 偏好多实例安全 | `src/model/prefs-store.ts:5-13`、`CHANGELOG.md`（0.5.4 条目） |
| 群历史 appId 前缀 | `src/features/history-store.ts:18` |
| 媒体根进程级 | `src/media/media-cleaner.ts:15` |
| msgid 缓存进程级 | `src/transport/msgid-cache.ts:31` |
| 出站按会话过滤 | `src/transport/outbound.ts:127` |
