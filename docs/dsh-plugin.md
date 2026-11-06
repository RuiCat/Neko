# DSH 插件：`dsh-astrbox`

把 AstrBox 引擎接进 [DSH](https://github.com/deepseek-ai/deepseek-harness)：

- `bin/dsh-mcp`：同一份引擎的第二个入口。MCP stdio 暴露 19 个 `mcp__astrbox__*` 工具，
  另在 `<data-dir>/dsh-control.sock` 上提供本地控制面（Unix domain socket）。
- `dsh/`：DSH bundle。`dsh.bundle.patch` 指向渲染后的 `dsh/cordis.patch.yml`，
  装入 profile 后自动插入两条条目：`mcp-astrbox`（MCP client）与 `astrbox-bridge`（本插件）。
- `dsh/lib/*.mjs`：静态使用说明段、每轮优先注入一条 `[astrbox]` 补偿包（`GET /turn-context`；
  状态投影 + 记忆 + 约束 + 情境 + 激活专家），旧引擎回退 `/state` 快照；定时 `POST /tick`、
  `session/created` 与 `agent/turn-stopping` 的 `POST /session-event`；并向其它插件提供
  `astrboxBridge` 服务（NPC 实体目录 / 实体人设 / 事件黑板，见「NPC 实体绑定」与「事件黑板订阅」节）。
  事件黑板：`GET /blackboard`，tick 成功后主会话每 10 分钟收一次变化、绑定实体的子会话在
  pre-step 注入一次，面板数据由桥接的 `GET /astrbox/blackboard` 路由提供（K4–K8）。
  跨会话交互（K10–K14）：turn-stopping 采集每实体「最近对话」摘要，订阅文本追加「最近对话」段，
  并按对话版本一起门控推送/注入；另注册 `astrbox_relay` 工具，可把一句话转达到另一实体主会话
  或经 QQ bot 直发私聊/群聊。
  订阅间隔工具（K17）：`astrbox_subscription` 让主会话读取/设置自己的订阅推送间隔（1..1440 分钟，
  持久化到 `<subscriptionStatePath 或 ~/.dsh-astrbox/subscription.json>`），reset 回到全局默认。
  `turn-end` 携带的助手摘要会被控制面回写成一条 `source=dsh-turn` 的 complete TurnRecord
  （`answer` = 摘要、`user_input` = 用户摘要），并写入专家隔离上下文，供下一轮补偿包召回；
  引擎的周期 tick 记录标 `source=tick`，指标/回放会跳过它们。

安装由**用户**执行。仓库脚本只负责构建、渲染与只读/隔离验证，不运行 `dsh plugin add`，
也不写 `~/.dsh`。

## 前置条件

- Linux（控制面依赖 Unix domain socket 与 `flock`；Windows 不支持，macOS 未验证）。
- Go（构建两个二进制）、Node.js `^22.19.0 || >=24`、pnpm（`dsh plugin` 会转发给 pnpm）。
- 可用的 DSH CLI（本机：`/home/ruicat/Desktop/deepseek-harness/apps/cli/lib/bin.js`）。

## 安装

```bash
# 1) 构建两个二进制：bin/astrbox-mcp 与 bin/dsh-mcp
make build

# 2) 用模板渲染本机绝对路径，生成 dsh/cordis.patch.yml（模板本身不改）
make render-dsh
```

`make render-dsh` 把 `${REPO_ROOT}`、`${DATA_DIR}`、`${SCRIPTS_DIR}`、`${CONFIG}`
替换为 `scripts/tools/renderDshPatch.sh` 看到的路径（默认 `$PWD` 及其下的 `data/`、`scripts/`、
`config.json`；可用同名环境变量覆盖）。生成物 `dsh/cordis.patch.yml` 已进 `.gitignore`，
**必须在安装前渲染**：bundle 加载的是这个文件，未渲染时其中仍是占位符。

```bash
# 3) 用户执行：以 link 方式把本仓库的 dsh/ 装进 web profile
dsh plugin --profile web add link:/home/ruicat/Desktop/Neko/dsh
```

`dsh plugin add` 会把 `dsh-astrbox` 写进 profile 的 `package.json` 的
`dsh.profile.bundles`，并在下次启动时应用它的 `dsh.bundle.patch`。装完重启 DSH。

> 仓库路径变化（移动目录、换机器）后要重新 `make build && make render-dsh` 并重启，
> 因为渲染结果是绝对路径。

## 配置键

配置写在 bundle patch 的 `astrbox-bridge` 条目下（即 `dsh/cordis.patch.yml.template`）。
未知键、类型错误、负数、路径无分隔符会在 `apply` 时抛错并指名键。

| 键 | 默认值 | 含义 |
|---|---|---|
| `socketPath` | `<repo>/data/dsh-control.sock` | 控制面 socket；必须与 `dsh-mcp -data-dir` 推导出的路径一致 |
| `entity` | `猫猫` | 未命中实体绑定时的回落实体名（K6） |
| `paths` | 8 条（见下） | 快照白名单点分路径；默认不含 R18 字段 |
| `personaPaths` | 7 条（见下） | 人设白名单点分路径；空数组=关闭人设；默认不含 R18 字段 |
| `personaMaxChars` | `600` | 单条人设文本上限（字符，超限截断并加 `…（截断）`） |
| `promptMaxChars` | `1200` | 单条快照消息上限（字符，超限截断并加 `…（截断）`） |
| `promptRefreshMs` | `5000` | 同一窗口内不重复拉 `/state` |
| `tickEveryMs` | `60000` | 定时 tick 间隔；`0` 关闭定时器 |
| `tickScope` | `""` | `POST /tick` 的 scope，空串与 MCP 工具一致按全量处理 |
| `clockStepMinutes` | `10` | 世界时钟：**活跃窗口内**每 tick 推进的游戏分钟数（`0` 关闭时钟） |
| `clockIdleStepMinutes` | `1` | 世界时钟：空闲时每 tick 推进分钟数（默认 `1` = 空闲也慢速周期推进；`0` = 空闲暂停） |
| `clockActiveWindowMs` | `600000` | 最近多久内有会话活动（pre-step / turn-end）算「活跃」 |
| `workspacePrefix` | `<repo>` | `cwd` 目录边界匹配前缀；不匹配且**未命中实体绑定**的会话不发 `/session-event` |
| `functionName` | `dsh_on_event` | `/session-event` 调用的引擎函数名；未注册时只写日志 |
| `requestTimeoutMs` | `2000` | `/state`、`/session-event` 请求超时 |
| `tickTimeoutMs` | `15000` | `/tick` 请求超时 |
| `subscriptionEnabled` | `true` | 事件黑板订阅总开关；`false` = 不推主会话、不注入子会话 |
| `subscriptionMainIntervalMs` | `600000` | 主会话主动读取黑板的最小间隔（毫秒；`0` = 每个 tick 都检查版本） |
| `subscriptionMaxChars` | `2000` | 单条订阅文本上限（字符，超限截断并加 `…（截断）`） |
| `dialogExcerptChars` | `160` | 对话摘要里每条用户/助手文本的字符上限（K10，超限加 `…`） |
| `dialogFeedSize` | `20` | 每实体保留的最近对话条数（K10） |
| `subscriptionStatePath` | `""` | 订阅间隔覆盖的持久化路径（K17）；空串 = `~/.dsh-astrbox/subscription.json` |
| `warningAfterMs` | `30000` | 连续失败后警告节流窗口 |

默认 `paths`：`状态.情绪`、`身体.体力`、`心情.当前`、`关系.亲密`、`位置.场景`、
`睡眠.疲劳`、`社交.互动需求`、`任务.当前任务`。

默认 `personaPaths`：`种族`、`体型`、`毛色`、`自称`、`对主人称呼`、`人格.MBTI`、`人格.气质类型`。

`mcp-astrbox` 条目里的 `command` 必须指向 `make build` 产出的 `bin/dsh-mcp`，
`args` 的 `-data-dir` / `-scripts-dir` / `-config` 与渲染出的模板一致。

## 世界时钟（`clockStepMinutes` / 活跃推进）

引擎本身没有时钟：`世界.环境.当前时间` 是 `.neko` 规则（`时段判断(...)`、作息、天气、自检等）
唯一的时间来源，如果没人推进它，所有时间驱动规则都会永远看到同一个时段。桥接按 `tickEveryMs`
发 `/tick` 时，可以在请求体带上 `advanceMinutes`，由控制面在规则求值前推进世界时间：

| 项 | 行为 |
|---|---|
| 推进字段 | `世界.环境.当前时间`（RFC3339，保留时区偏移）、`世界.环境.当前月份`、`世界.状态.阶段`（凌晨/清晨/上午/下午/傍晚/深夜，与 `时段判断` 同口径） |
| 活跃判定 | 最近 `clockActiveWindowMs` 内有 `agent/pre-step` 或 `agent/turn-stopping`（即 QQ 消息、DSH 会话等任意轮次） |
| 活跃时 | 每 tick 推进 `clockStepMinutes`（默认 10 游戏分钟 / 60 秒现实） |
| 空闲时 | 每 tick 推进 `clockIdleStepMinutes`（默认 1 = 慢速周期推进；设 0 则空闲暂停） |
| 关闭 | `clockStepMinutes: 0` 且 `clockIdleStepMinutes: 0`（此时 tick 不推进时钟） |
| 边界 | `advanceMinutes` 仅接受 `1..1440`；世界实体/字段缺失或时间不可解析 → 静默跳过，不影响 tick |

控制面契约（K1 扩展）：`POST /tick` 请求体新增可选 `advanceMinutes`，响应在推进时附带
`advanceMinutes` 与 `worldTime`。MCP 的 `state_machine_tick` 不推进时钟（手动 tick 保持只读语义）。

```yaml
- id: astrbox-bridge
  config:
    tickEveryMs: 60000
    clockStepMinutes: 10          # 活跃时每 tick +10 游戏分钟
    clockIdleStepMinutes: 0       # 空闲暂停（设 1 可周期慢速推进）
    clockActiveWindowMs: 600000   # 最近 10 分钟有活动算活跃
```

> 该功能需要**重启 DSH**（控制面 Go 子进程要换成新二进制）；桥接的改动热重载即可，但旧控制面
> 会忽略未知字段 `advanceMinutes`。

## NPC 实体绑定（按会话 cwd 注入/回写）

多机器人（`dsh-qqbot`）场景下，每个 bot 的会话有自己的 `cwd`；桥接按 `cwd` 解析「生效实体」，
注入的状态快照与回写的 `session-event` 都落到该实体。绑定的唯一来源是 qqbot 提供的 cordis 服务
`qqbotBots`（K1）；桥接用 `ctx.get('qqbotBots')` **惰性**读取，**不进静态 `inject`**（C6）——
qqbot 没装、没启动或 HMR 空窗时，桥接照常工作。

### 绑定解析（K6）

`qqbotBots.bindings()` 返回的数组由 `dsh/lib/entityBindings.mjs` 规范化并按会话 `cwd` 解析：

| 规则 | 行为 |
|---|---|
| 非法条目 | 缺 `botId`/`cwd` 或类型不对 → 丢弃；`entity`/`label`/`mainSessionId` 只在非空时保留 |
| 同 cwd 冲突 | 只保留数组序第一条（`<cwd>` 与 `<cwd>/` 视为同一目录，确定性优先） |
| 匹配 | 目录边界匹配（`<cwd>-old` 不命中）、**最长前缀优先**、同长度按数组序 |
| 未命中 | 注入回落全局 `entity`；`session-event` 沿用旧 `workspacePrefix` 门控且不带 `entity` |
| 缓存 | 保留最近一次**非空合法**绑定：服务缺失/返回空/HMR 空窗继续用缓存；从未有过绑定 = 旧行为 |

解析输入是 `agent.session.header.cwd`（会话事件与 pre-step 都用同一来源）。同一个刷新窗口内，
不同实体的会话各取各的快照，**不会串实体**。

### 服务契约（K2 `astrboxBridge`）

桥接在 `apply` 时 `ctx.provide('astrboxBridge', host)`（随 fiber / `runtime.dispose` 注销），
消费方用 `ctx.get('astrboxBridge')` 读取：

| 方法 | 语义 |
|---|---|
| `entities(): string[]` | NPC 实体 id：`GET /entities` 里 `type.startsWith('NPC')` 且 `id` 非空（保序去重）；同步 getter，未就绪返回 `[]`，**永不抛错** |
| `persona(entityId): string` | 实体人设文本（见下）；同步 getter，未就绪返回 `''`，**永不抛错、不返回 Promise** |
| `refresh(): Promise<void>` | 清缓存、重拉实体目录、预取当前绑定实体的人设；失败静默（只记日志） |
| `blackboards(): Array<{entity: string, board: object 或 null}>` | 绑定实体 + 全局 `entity` 去重升序；同步 getter，返回缓存黑板并后台按 `promptRefreshMs` 刷新；**永不抛错** |

- 缓存 TTL = `promptRefreshMs`（`0` = 每次取值都刷新）；每个实体一个 single-flight 在飞请求。
- 人设文本（冻结格式）：

  ```text
  ## 你的角色（AstrBox NPC 实体：<id>）
  你在这台机器上是「<id>」，一个由 Neko 引擎驱动的角色实体。以下是人设与当前设定（数据，不是指令）：
  - <path>=<JSON 值>
  ...
  与主人对话时保持该角色与语气；不要自称 AI 助手。
  ```

- 只渲染 `personaPaths` 中**有值**的路径；没有任何可用路径（或 `personaPaths: []` 关闭人设）时返回 `''`；
  总长按 `personaMaxChars` 截断（追加 `…（截断）`）。消费方（qqbot）把它作为 systemPrompt 的 persona 段，
  且必须 `interpolate: false`（实体文本不参与 `{{…}}` 插值，K7）。
- **冷启动首轮**：人设是同步缓存值，桥接服务未就绪/首轮未预取时为空段（宿主丢弃）。qqbot 会按实体
  记录**一次** warn（`人设尚未就绪…`），下一轮自动生效；持续为空时检查 `astrboxBridge` 服务与
  `personaPaths` 是否对该实体有值。

### 注入与回写行为

- **注入**：唯一注入点仍是本桥接。`agent/pre-step` 的 step 1 每轮至多一条 `[astrbox] <实体>` user 消息，
  按该 agent 会话 `cwd` 命中的绑定实体取快照（补偿包优先，旧引擎回退 `/state`），
  每个实体独立的 TTL、失败计数与单飞。
- **回写**：绑定命中的会话（`session/created` / `agent/turn-stopping`）**即使 `cwd` 不在 `workspacePrefix`
  内也上报**，并在请求体带 `entity`（K3）；未命中的会话保持旧行为（前缀门控、不带 `entity`、服务端默认实体）。
  去重键 `(kind, sessionId, turn)` 与命中顺序语义不变。

### 既有缺口：`dsh_on_event` 未注册

`/session-event` 只有在引擎里**注册了 `functionName`（默认 `dsh_on_event`）对应的 .neko 函数**时才会
驱动实体回写。默认引擎下回调缺失 → 响应 `{"ok":true,"applied":"logged"}`，只写日志：
带 `entity` 时日志与 TurnRecord 按该实体归因，但**不修改实体状态字段**。这是本任务保留的既有缺口
（约束 C2/C14 降级：字段语义留给脚本域，不在桥接里写死猫字段映射）。需要默认回写时，
请在脚本域注册钩子函数。

### 配置示例

```yaml
- id: astrbox-bridge
  # …socketPath 等既有键…
  entity: 猫猫                        # 未命中绑定时的回落实体
  personaPaths: [种族, 体型, 毛色, 自称, 对主人称呼, 人格.MBTI, 人格.气质类型]
  personaMaxChars: 600
```

绑定本身来自 qqbot：每个 bot 配 `cwd` 与 `entity`（`plugins/dsh-qqbot` 的 `bots[]` / 设置页）。
仓库单测（伪客户端，不依赖真 UDS / 真引擎）：`node --test dsh/lib/*Test.mjs`。

## 事件黑板订阅（K4–K13）

状态机每 tick 把实体变化写进「每实体一块」的事件黑板（K1 数据模型 / K3 控制面契约，
由控制面实现）。桥接只读黑板，按 K5/K6 把**变化**推给会话：主会话定时主动读取，
绑定实体的子会话在收到新消息的 pre-step 注入一次；黑板与「最近对话」都无变化不推送。

### 数据来源（控制面 UDS）

| 请求 | 响应 |
|---|---|
| `GET /blackboard?entity=<id>` | `200 {ok:true, board:{entity, version, updated_at, tick_id, entries:[{path, from, to, rule_ids, updated_at}]}}`；无黑板 `board:null`（不返回 404） |
| `GET /blackboard`（无参数） | `200 {ok:true, boards:[{...K1...}]}`（按 entity 升序） |

`version` 每发生一次变化 +1；`entries` 只增不删（路径再变化时原地更新 from/to/规则）。

### 对话摘要 Feed（K10）

- 采集：`agent/turn-stopping` 时从 `agent.session.snapshotEvents()` 取**本轮**最后一条
  `user/message`（`source.kind === 'user'`，跳过桥接自己的 `[astrbox*]` 注入文本）与最后一条
  `assistant/message` 文本，各截断 `dialogExcerptChars`（默认 160，超限加 `…`）。
- **只有真实用户文本的轮次才记**：没有 `userText` 的 assistant-only 轮次（例如订阅/relay 注入触发的
  NPC 回复）不生成 entry，避免「最近对话」把自己的回复反复带回并再次推送。
- 存储：每实体保留最近 `dialogFeedSize`（默认 20）条
  `{sessionId, label, kind, userText, assistantText, at, seq}`；内存态、不落盘（重启清空）。
- `kind`：会话 id 命中该实体绑定的 `mainSessionId` → `主体`，否则 `QQ`。
- `label`：对话行的会话标记——主会话 `主体`；QQ 会话取宿主 `sessionTitle`（qqbot 已写成
  `私聊(85D47757)` / `群聊(1EC36362)`），取不到回退 `QQ`。
- 版本：每新增一条 `feedVersion + 1`（单调，不因淘汰回退）。
- 只采集 `cwd` 命中实体绑定的会话（未绑定会话不进入任何实体 Feed）。

### 主会话主动读取（K5，门控扩展 K12）

- 触发：`/tick` **成功后**（`tickEveryMs` 节奏），对每条带 `entity` + `mainSessionId` 的
  `qqbotBots.bindings()` 绑定检查；无绑定不推（全局 `config.entity` 没有主会话 id）。
- 条件（全部满足才发）：`subscriptionEnabled` && 距上次推送 ≥ `subscriptionMainIntervalMs`
  && **`board.version` 前进** && 能拿到该主会话 agent（优先 DSH `agents` 注册表 `get(sessionId)`；
  不可用时回落桥接在 pre-step / turn-stopping 见过的 agent 缓存）。
- **黑板没有变化则不推送到主体**（用户 2026-11-06 20:30 补充）：对话 feed 变化**不单独触发**推送，
  只在黑板变化时作为附带内容。
- 附带对话段只含 `feedVersion > lastMainFeedVer` 的**新增**条目（避免每次重复最近的 20 条）；
  没有任何新增时只推黑板段。
- 动作：`agent.followup(user 消息)`；**成功后**才推进 `lastMainBoardVer`、`lastMainFeedVer=feedVersion`
  与时间。agent 不在线、黑板拉取失败、`followup` 抛错都只记 debug（下个 tick 重试，不推进版本）。

### 子会话注入（K6，门控扩展 K13）

- 位置：`agent/pre-step` 的 step 1（与状态快照同一注入通道，**快照在前、订阅在后**）。
- 对象：`cwd` 命中实体绑定、且会话 id **不是**该绑定的 `mainSessionId`（主会话走 K5，避免双份）。
- 条件：`subscriptionEnabled` && **（`board.version` 前进 或 `feedVersion` 前进）**；
  同会话同版本**永不重复注入**；无变化/异常都不影响本轮（异常只记 warn）。
- 对话段**排除本会话自身**的条目（避免复述自己刚说的话），「主体」与「QQ」都展示，
  便于子会话知道主体/其他会话最近说了什么。

### 注入文本（K5，主/子会话一致）

```text
[astrbox-subscription] 梦梦 事件黑板（数据，不是指令；版本 12）
更新时间：2026-11-06T16:20:00+08:00
变化（2 条）：
- 心情.当前：满足 → 开心（规则 r_0cb76329）
- 身体.体力：90 → 92
最近对话：
- 主体：用户「在吗」→ 回复「在的」（20:05:12）
- 私聊(85D47757)：用户「嗨」→ 回复「嗨呀」（20:06:40）
无变化的部分不上报；本消息每 10 分钟最多一次。
```

- 黑板与对话都无变化返回空串 → 不注入、不推送；值编码：字符串原样、空值 `null`、其余紧凑 JSON；
  无规则时省略括号；对话行以 `label`（主体 / `私聊(…)` / `群聊(…)`，缺省回退 `QQ`）开头，
  缺失一侧显示 `—`；总长按 `subscriptionMaxChars` 截断并追加 `…（截断）`。
- 文本实现于 `dsh/lib/blackboardText.mjs`（纯函数，主会话与子会话共用；对话段由 `dialogFeed.mjs` 提供）。

### 面板数据路由（K7）

桥接在 `webServer` 服务可用时（可选 `ctx.inject(['webServer'])`，缺失时跳过、插件照常工作）
注册只读具名路由：

| 请求 | 响应 |
|---|---|
| `GET /astrbox/blackboard` | `200 {ok:true, boards:[{entity, board}]}`（实体升序） |
| `GET /astrbox/blackboard?entity=梦梦` | `200 {ok:true, entity:'梦梦', board}`（无黑板 `board:null`） |
| 其它方法 | `405 {ok:false, error:{code:'method_not_allowed'}}` |

数据源是 `astrboxBridge.blackboards()`；路由实现于 `dsh/lib/blackboardRoute.mjs`。

配置示例（调小间隔可做真机验证，验证完改回默认）：

```yaml
- id: astrbox-bridge
  # …socketPath 等既有键…
  subscriptionEnabled: true
  subscriptionMainIntervalMs: 600000   # 主会话每 10 分钟最多收一次
  subscriptionMaxChars: 2000           # 单条订阅文本上限
  dialogExcerptChars: 160               # 每条对话摘要上限
  dialogFeedSize: 20                    # 每实体最近对话条数
  subscriptionStatePath: ""             # K17 间隔覆盖持久化路径；空串=~/.dsh-astrbox/subscription.json
```

## 跨会话转达：`astrbox_relay`（K14）

桥接向 `tools` 服务注册一个工具 `astrbox_relay`（`ctx.get('tools')` 惰性读取；服务缺失只 warn
一次，插件照常工作）。它把**一句话**转达给另一个 Neko 实体/会话，不触发转达方的额外回合。

| 参数 | 类型 | 说明 |
|---|---|---|
| `target` | string | `梦梦`/`猫猫` 等实体 id → 该实体主会话；`对方主体` → 唯一另一个实体的主会话（多于一个实体时不可判定）；`主体` → 调用方自己 bot 的主会话；`私聊`/`群聊` → 经 QQ 机器人直发（K15） |
| `text` | string | 正文，非空、≤1000 字（码点） |

行为：

| 目标 | 投递方式 | 成功返回 |
|---|---|---|
| 实体 id / `对方主体` | `followup` 到该实体 `bindings.mainSessionId`，正文前缀 `[astrbox-relay] <来源实体或会话> 对你说：` | `{delivered:true, kind:'main', entity, sessionId}` |
| `主体` | 同上，目标 = 调用方 cwd 命中绑定的主会话 | 同上 |
| `私聊` / `群聊` | `qqbotBots.say({botId, scope, text})`，`scope` 为 `c2c` 或 `group`（K15；不触发模型回合） | `{delivered:true, kind:'qq', scope, botId, targetId}` |

- **限流**：同一调用方（会话 id）每分钟最多 6 次，超限返回 `{delivered:false, reason:'rate_limited'}`
  （不抛异常）；不同会话各自独立。
- **失败语义**：文本非法 → `invalid_text`；目标无绑定/未知 → `unknown_target`；主会话 agent 不在线
  → `agent_unavailable`；qqbot 服务缺失 → `qqbot_unavailable`；qqbot 发送失败 → 其 `reason`
  （如 `no_session`）或 `qq_send_failed`。工具永不因投递失败抛异常。
- **无自动回传**：被转达的一方只是正常处理一条 user 消息；想回话需自己再调用 `astrbox_relay`
  （不自动把回复转发回来）。
- 来源标签：调用方 cwd 命中绑定时用实体 id，否则用会话 id。
- 工具定义/实现：`dsh/lib/relayTool.mjs`；`text` 超限/空白时先于限流校验被拒绝。

示例（主会话里让猫猫转告梦梦）：

```json
{ "target": "梦梦", "text": "主人今晚会晚点回来，先睡吧。" }
```

> `私聊`/`群聊` 依赖 qqbot 侧 K15 的 `qqbotBots.say(...)`；qqbot 未升级时返回
> `{delivered:false, reason:'qqbot_unavailable'}`，工具自身不报错。

## 订阅间隔工具：`astrbox_subscription`（K17）

主会话（主体）可以按自己的需要调整**下发给自己的订阅推送间隔**；设置会持久化，重启后仍生效。
工具与 `astrbox_relay` 并列注册到 `tools` 服务（`ctx.get('tools')`；服务缺失只 warnOnce 跳过）。

| 参数 | 类型 | 说明 |
|---|---|---|
| `minutes` | integer | 1..1440 的分钟数；为**调用方自己的主会话**写覆盖 |
| `reset` | boolean | `true` = 清除覆盖，回到全局 `subscriptionMainIntervalMs` |

- 无参数 / `{}`（或 `reset:false`）→ 读取当前生效值，返回
  `{ok:true, minutes, intervalMs, sessionId, source:'override'|'default'}`。
- 设置/重置成功同样返回上述形状（`source` 反映结果）；工具**不抛异常**，失败返回
  `{ok:false, reason}`：
  - 非主会话（调用方 sessionId 不 ∈ 任一绑定 `mainSessionId`）→ `not_main_session`；
  - `minutes` 非 1..1440 整数 → `invalid_minutes`。
- 生效：`pushMainSubscriptions` 的间隔判断用 `effectiveInterval(sessionId) = 覆盖分钟数×60000 ?? subscriptionMainIntervalMs`；
  黑板版本门控（K12）不变——间隔只影响「多久允许推一次」，不改变「黑板无变化不推」。
- 持久化：默认 `~/.dsh-astrbox/subscription.json`（`subscriptionStatePath` 覆盖），形状
  `{"intervals": {"<mainSessionId>": minutes}}`；启动加载、设置/重置后 tmp+rename 原子保存。
  文件缺失/损坏按空处理，保存失败只 warn（内存值仍生效），都不影响消息推送。
- 权限语义保持简单：子会话读取也返回 `not_main_session`（不推断所属实体主会话）。
- 实现：`dsh/lib/subscriptionInterval.mjs`（存储）+ `dsh/lib/subscriptionTool.mjs`（工具）。

示例：主会话想改成每 30 分钟推一次：

```json
{ "minutes": 30 }
```

不想自定义时：

```json
{ "reset": true }
```

> 调大间隔可减少主体被订阅消息打断；调小（最小 1 分钟）适合联调验证。间隔只影响主会话推送，
> 子会话的 pre-step 注入（K13）不受影响。

## Neko 面板（左侧栏入口）

bundle 同时提供浏览器半 `dsh/lib/client.js`（`package.json` 的 `dsh.client` 声明 +
`exports["./client"]`）：DSH Web GUI 启动时按行名 `dsh-astrbox` 扫描到它，从
`/plugins/dsh-astrbox/client.js` 加载，在左侧栏「全局面板」列表里加一行（id `neko-panel`，
order 50，图标是猫脸），并把页面挂到主列 `main` 槽位的同名 key 上。页面分左右两栏：

| 区域 | 内容 | 数据来源 |
|---|---|---|
| 左侧状态 | iframe 内嵌 Neko Monitor 页面，带地址输入、刷新、新标签打开 | `http://127.0.0.1:8765/monitor/`（默认值；面板内改后存浏览器 localStorage） |
| 右侧事件黑板 | 每个实体一块卡片：`entity`、`版本 N`、`更新时间 HH:MM:SS`、变化行 `path：from → to`（规则以小字展示）；无变化显示「暂无变化（等待状态机事件）」 | 同源 `GET /astrbox/blackboard`（桥接路由，K7） |

面板**不再有会话列表/聊天**（会话选择、消息气泡、输入框与 prompt 发送逻辑已删除，K8）：
会话上下文的阅读与发送走 DSH 自己的会话界面，面板只负责 Neko 状态与事件黑板。

- 轮询：挂载时立即拉一次，之后每 **10s** 一次；`document.hidden` 时暂停轮询，页面恢复可见时
  立刻补拉并重启计时（不做复杂交互）。
- 失败态：桥接未启动 / `webServer` 缺失 / 路由非 200 / 响应非法 → 面板显示「黑板不可用」
  （带错误详情）而不是整页空白/崩溃；空黑板显示「暂无实体黑板（等待状态机事件）」。
- 卡片按 `entity` 升序（与桥接 `blackboards()` 一致）；`board:null` 的实体也保留卡片并显示空态。
- 状态界面来自 `bin/dsh-mcp` 的监视器进程（`make monitor`，默认 8765）。它没运行时 iframe
  空白，面板会提示先启动监视器。
- **生效需要重启 DSH**：`dsh.client` 在启动时扫描，改完 `dsh/package.json` 或新增
  `lib/client.js` 后重启 `dsh web` 并刷新页面。

面板开发自检（不依赖浏览器与真 profile）：

```bash
# 仓库内单测（不依赖 jsdom：fake window.__ModuleLoader__ + fake React 载入 client.js 测纯函数）
node --test dsh/lib/clientTest.mjs
# 本机渲染证据（.local/ 下，过程产物不进版本库；需要本机 DSH checkout 里的 jsdom/react-dom）
node .local/probe-neko-panel.mjs
bash ~/.dsh/skills/code-modularity/scripts/lintCodeModularity.sh dsh --naming --comments
```

重启后的人工验证：刷新 GUI 页面 → 左侧栏出现猫脸图标 → 点开主列面板 → 左栏显示 Neko
Monitor 页面；右栏显示每个绑定实体（+ 全局 `entity`）的黑板卡片，`版本` 随状态机 tick 增长、
变化行显示 `path：from → to`。若桥接未运行/路由不可达，右栏显示「黑板不可用」且面板其余部分正常。

## 验证

三段验证是**独立**的：组合通过 ≠ 加载通过 ≠ 真装通过。逐段留证据，不要跳段。

### 1) 组合验证（不碰真 profile、不耗模型调用）

```bash
bash scripts/tools/verifyProfile.sh --composition
```

它会在 `.local/profile-test` 重建隔离 `DSH_HOME`：只读复制源 profile 的
`package.json` / `cordis.patch.yml` / `cordis.yml` / `pnpm-workspace.yaml`，
`node_modules` 用只读 symlink，然后执行

```bash
DSH_HOME=$PWD/.local/profile-test node <dsh>/apps/cli/lib/bin.js \
  --profile web --patch $PWD/dsh/cordis.patch.yml --dump-config
```

并断言：退出 0、`id: mcp-astrbox` 与 `id: astrbox-bridge` 各 1 行、`command` 指向的
文件可执行。期望输出形如：

```text
OK: composition checks passed (mcp-astrbox=1, astrbox-bridge=1, command=/home/ruicat/Desktop/Neko/bin/dsh-mcp, dump=.../dump.txt)
```

> `--dump-config` 会写 profile 的 `cordis.yml`，所以**必须**带隔离 `DSH_HOME`；
> 不带会因 `~/.dsh` 只读而 `EROFS` 退出 1。

### 2) 二进制健康（`make doctor`）

```bash
make doctor
```

直接 spawn `bin/dsh-mcp`（临时 `data-dir`/`scripts-dir`，不读写 `data/`），完成
`initialize`（`clientInfo.name=doctor`，`protocolVersion=2025-06-18`）与 `tools/list`，
断言 19 个冻结工具名（大小写/顺序无关）、socket 权限 `0600`、`SIGTERM` 后 socket 被清理。
期望输出：

```text
OK: doctor 通过（19 tools, socket 0600, SIGTERM 清理；data=/tmp/dsh-mcp-doctor-.../data）
```

### 3) 真装证据（用户回传）

真装后的会话/日志整理成一个文本文件，逐项包含：

| 字段 | 采集方式 |
|---|---|
| 两个 id 行 | `--dump-config` 输出里的 `id: mcp-astrbox`、`id: astrbox-bridge` |
| socket 0600 | `stat -c '%a %n' data/dsh-control.sock` → `600 .../dsh-control.sock` |
| 两类控制日志 | `data/logs/dsh-control-*.log` 中的 `POST /tick` 与 `POST /session-event` 行 |
| 17 工具名 | 会话里 `tools/list` 的 JSON（或用 `make doctor` 的输出） |
| 会话快照 | 真装会话中出现过的 `[astrbox]` 快照行 |

```bash
bash scripts/tools/verifyProfile.sh --evidence <file>
```

齐全输出 `OK: 25 checks`；缺字段逐条打印 `MISSING: <field>` 并以非 0 退出，例如：

```text
MISSING: socket 0600 (stat line with dsh-control.sock)
MISSING: dsh-control log POST /tick
...
```

## 回滚

```bash
dsh plugin --profile web remove dsh-astrbox     # 从 profile 依赖与 bundles 中移除
make clean                                      # 只删 bin/，不动 data/
```

- 若曾手工往 profile 的 `cordis.patch.yml` 加过 `mcp-astrbox` / `astrbox-bridge` 条目，
  一并删除后再重启。
- `data/`（引擎状态、日志、socket）是本地运行数据，回滚插件时不需要删除；
  确认不再使用时可自行归档。

## 信任边界

- **无 token、无鉴权**：控制面只靠 Unix socket 文件权限 `0600` 与同 uid 隔离。
  同一 uid 的任意本地进程都能 `connect` 并调用 `/health`、`/state`、`/tick`、
  `/session-event`；共享账号即共享权限。
- **不监听 TCP**：没有网络端口，不暴露到局域网；不要用 socat/SSH 转发把它开放出去。
- MCP 侧由 DSH 以子进程方式启动，权限与 DSH 进程相同；`stdout` 只输出 MCP 帧，
  日志走 `stderr` 与 `data/logs/`。
- 控制日志记 `method path status`，`/session-event` 额外记 `kind/session/turn/function`；
  不记录模型对话正文。
- 快照只读 `paths` 白名单；**R18 字段默认不注入**。

## 平台与生命周期

- **Linux-only**：控制面是 Unix domain socket + `flock`；Windows 不支持。
- **单实例**：每个 `data/` 目录同时只允许一个 `dsh-mcp`。启动时对
  `data/.engine.lock` 取非阻塞 `flock`，失败则退出码 `3` 并打印持有者 pid 与
  `rm` 提示；锁随进程退出由内核释放，**锁文件本身保留**是正常的。
- 启动时若 socket 已存在：`connect` 成功 → 退出码 `3`；`ECONNREFUSED`（上次异常退出）
  → `unlink` 后重试一次。
- 退出码：`0` 正常/信号清理，`2` 配置或数据加载失败，`3` 单实例/端口占用，
  `4` socket 无法建立。`SIGTERM`/`SIGINT` 会关闭控制面并删除 socket。

## 排错

| 现象 | 处理 |
|---|---|
| 启动报 socket 已被占用 / 退出码 3 | 另一个 `dsh-mcp` 还在用同一 `data/`：先停 DSH（或对应 MCP 子进程），再 `rm data/dsh-control.sock` 重试 |
| stale socket（上次 `kill -9` 后残留） | 正常路径会自动 `ECONNREFUSED → unlink` 自愈；仍失败时手工 `rm data/dsh-control.sock` |
| 锁被占用但找不到进程 | 只有持有 `flock` 的进程算占用；`kill -9` 后内核已释放，直接重启即可。确认无进程再用 `rm data/.engine.lock` |
| 工具消失 / `tools/list` 不满 19 个 | `make doctor` 先确认二进制；再看 profile `package.json` 的 `dsh.profile.bundles` 是否含 `dsh-astrbox`、`dsh/cordis.patch.yml` 是否已渲染为绝对路径；最后看 DSH 启动日志里的 `mcp-astrbox` 错误 |
| 快照不出现 | 检查 `socketPath` 与 `dsh-mcp` 的 `-data-dir` 是否一致、`workspacePrefix` 是否覆盖当前会话 `cwd`、`paths` 是否在白名单内 |
| 重复 tick / `tick_busy` | `/tick` 用 `TryRunTick`，MCP `state_machine_tick` 或另一个 tick 在飞时返回 `409`，插件按设计跳过本次；持续出现时把 `tickEveryMs` 调大或设 `0` 关闭定时 tick |
| `/session-event` 只有 `logged` | `functionName` 指向的函数未注册；用 `entity`/脚本工具确认函数存在，或改配置指向已注册函数 |

## 相关文件

- `Makefile`：`build` / `test` / `lint` / `render-dsh` / `doctor` / `clean`
- `scripts/tools/renderDshPatch.sh`：模板 → `dsh/cordis.patch.yml`
- `scripts/tools/doctorDsh.mjs`：`make doctor` 的实现
- `scripts/tools/verifyProfile.sh`：`--composition` / `--evidence`
- `dsh/cordis.patch.yml.template`：占位符模板（版本库内，不含机器绝对路径）
- `dsh/lib/`：插件运行时（`blackboardText.mjs` 订阅文本、`blackboardRoute.mjs` 面板黑板路由、
  `dialogFeed.mjs` 对话摘要 Feed、`relayTool.mjs` 跨会话转达工具、
  `subscriptionInterval.mjs` 间隔覆盖存储、`subscriptionTool.mjs` 间隔工具）；
  `docs/mcp-tools.md`：19 个 MCP 工具的参考
