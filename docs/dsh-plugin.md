# DSH 插件：`dsh-astrbox`

把 AstrBox 引擎接进 [DSH](https://github.com/deepseek-ai/deepseek-harness)：

- `bin/dsh-mcp`：同一份引擎的第二个入口。MCP stdio 暴露 31 个 `mcp__astrbox__*` 工具，
  另在 `<data-dir>/dsh-control.sock` 上提供本地控制面（Unix domain socket）。
- `plugins/dsh-astrbox/`：DSH bundle。`dsh.bundle.patch` 指向 `plugins/dsh-astrbox/cordis.patch.yml`，
  装入 profile 后自动插入五行：`mcp-astrbox`（MCP client）、`astrbox-bridge`（本插件）、
  `im-qqbot`、`diary`、`bilibili`。
- `plugins/dsh-astrbox/lib/*.mjs`：静态使用说明段；`agent/pre-step` 每轮至多注入一条 `[astrbox-state]`
  现状卡（`GET /v2/observations/state`）与一条 `[astrbox-compensation]` 补偿包（`GET /turn-context`：
  记忆/约束/情境/专家）；定时 `POST /tick`、`session/created` 与 `agent/turn-stopping`
  的 `POST /session-event`；并向其它插件提供 `astrboxBridge` 服务（NPC 实体目录 / 实体人设 / 面板黑板，
  见「NPC 实体绑定」与「状态变化推送」节）。
  状态变化推送（Observation v2）：tick 成功后主会话按**实体游标**拉 `GET /v2/observations` 的 delta，
  用 `renderDelta` 渲染后推送，对话摘要随同一条消息附带；面板数据由桥接的 `GET /astrbox/blackboard`
  路由提供（数据源为 `/v2/observations/state`）。
  跨会话交互（K10–K14）：turn-stopping 采集每实体「最近对话」摘要，变化推送附带「最近对话」段，
  并按对话版本一起门控；另注册 `astrbox_relay` 工具，可把一句话转达到另一实体主会话
  或经 QQ bot 直发私聊/群聊。
  订阅间隔工具（K17）：`astrbox_subscription` 让主会话读取/设置自己的订阅推送间隔（1..1440 分钟，
  持久化到 `<subscriptionStatePath 或 ~/.dsh-astrbox/subscription.json>`），reset 回到全局默认。
  主提示词工具（K29）：`astrbox_main_prompt` 让主会话读/写/清除按实体的主会话提示词（与面板同一
  份存储，写入后下一轮生效），子会话只读。
  控制工具与面板控制（K21/K22）：桥接把 `/astrbox/control/*` 代理到控制面（手动 tick、世界时钟设定、
  实体目录/状态读写、实体删除），面板新增「控制」区（时钟 +15/+60/设定、手动 tick、实体状态编辑）。
  `turn-end` 携带的助手摘要会被控制面回写成一条 `source=dsh-turn` 的 complete TurnRecord
  （`answer` = 摘要、`user_input` = 用户摘要），并写入专家隔离上下文，供下一轮补偿包召回；
  引擎的周期 tick 记录标 `source=tick`，指标/回放会跳过它们。

安装由**用户**执行。仓库脚本只负责构建、渲染与只读/隔离验证，不运行 `dsh plugin add`，
也不写 `~/.dsh`。

## 前置条件

- Linux（控制面依赖 Unix domain socket 与 `flock`；Windows 不支持，macOS 未验证）。
- Go（构建两个二进制）、Node.js `^22.19.0 || >=24`、pnpm（`dsh plugin` 会转发给 pnpm）。
- 可用的 DSH CLI（如 `<dsh-checkout>/apps/cli/lib/bin.js`）。

## 安装

```bash
# 1) 构建两个二进制：bin/astrbox-mcp 与 bin/dsh-mcp
make build

# 2) 把运行时（bin/scripts/config）拷贝到生产运行根 ~/.dsh-neko/Neko
make install-runtime
```

`make install-runtime` 在 Makefile 内直接完成：把 `bin/dsh-mcp`、`bin/astrbox-mcp`、`scripts/**`、
`config.json` **拷贝**到 `~/.dsh-neko/Neko`。可用 `NEKO_HOME=` 改目标、`PRUNE=1` 先清空旧脚本，
`make -n install-runtime` 为 dry-run。

`plugins/dsh-astrbox` 是**聚合 bundle**：`package.json` 的 dependencies 带入 4 个同级组件
（`dsh-mcp-client` / `@tencent-connect/dsh-qqbot` / `dsh-plugin-diary` / `dsh-plugin-bilibili`），
`cordis.patch.yml` 声明 5 行、**不含 config 与机器路径**。运行配置由各插件从
`~/.dsh-neko/Neko/data/<组件>.config.json` 读取；MCP 启动参数由 `dsh-mcp-client` 包装插件默认指向
`~/.dsh-neko/Neko/bin/dsh-mcp`（可用 `mcp-astrbox.json` 覆盖）。

### `link:` 安装与依赖解析（必读）

DSH 的 loader 在 **profile 目录**上解析 patch 里的**裸包名**；而 pnpm 的 `link:` 只建符号链接，
**不会安装被链 bundle 自己的 dependencies**。所以「dependencies 带入组件」只对**真实安装**（拷贝/打包）成立，
`link:` 安装下裸名能否解析取决于 profile 的 `node_modules`，缺一个就是启动告警
`<id> (<name>): failed to import: Cannot find package '<name>'`，该行静默不激活。

本仓库对两种安装都成立的写法：

| 行 | 入口 | `link:` 安装要求 |
|---|---|---|
| `mcp-astrbox` | `./lib/mcpClientEntry.mjs`（bundle 内相对入口，loader 会锚定到 bundle 目录） | 仓库内两处 `node_modules` 链接（见下） |
| `astrbox-bridge` | `dsh-astrbox`（自己，profile 一定可解析） | — |
| `im-qqbot` / `diary` / `bilibili` | 裸包名 | profile 的 `node_modules` 里必须有同名包 |

仓库内链接依赖（幂等，且已被 `.gitignore` 的 `/plugins/*/node_modules/` 覆盖）：

```bash
cd /path/to/Neko
mkdir -p plugins/dsh-astrbox/node_modules plugins/dsh-mcp-client/node_modules/@deepseek-ai
ln -sfn ../../dsh-mcp-client plugins/dsh-astrbox/node_modules/dsh-mcp-client
ln -sfn "$DSH_HOME/profiles/node_modules/@deepseek-ai/dsh-mcp-client" \
        plugins/dsh-mcp-client/node_modules/@deepseek-ai/dsh-mcp-client
```

第一条让 `mcpClientEntry.mjs` 找得到 `dsh-mcp-client`；第二条让包装插件找得到它自己的上游
`@deepseek-ai/dsh-mcp-client`（`link:` 下 Node 按真实路径解析，不经过 `~/.dsh/profiles/node_modules`
这一层目录——必须显式链到宿主那份，才能与宿主共用同一个模块实例与版本）。

其余 4 行的包在 profile 里也得有；本机 profile 是用 `dsh plugin --profile web add link:<组件路径>`
分别装过的产物。换机器或重装 profile 后，缺哪个就在 profile 里补 link 哪个（或改用真实安装，
让 pnpm 一次装齐）。

```bash
# 3) 用户执行：以 link 方式把本仓库的 dsh/ 装进 web profile
dsh plugin --profile web add link:/path/to/Neko/plugins/dsh-astrbox
```

`dsh plugin add` 会把 `dsh-astrbox` 写进 profile 的 `package.json` 的
`dsh.profile.bundles`，并在下次启动时应用它的 `dsh.bundle.patch`。装完重启 DSH。

> 仓库路径变化（移动目录、换机器）后要重新 `make build && make install-runtime`、
> 重做上面的仓库内 `node_modules` 链接，并重启 DSH：`install-runtime` 的落点是绝对路径，
> 链接是相对路径（`plugins/*/node_modules` 不入库）。

## 配置键

配置写在运行数据目录的 `bridge.config.json`（默认 `~/.dsh-neko/Neko/data/bridge.config.json`），行配置为空时由插件读取。
未知键、类型错误、负数、路径无分隔符会在 `apply` 时抛错并指名键。

| 键 | 默认值 | 含义 |
|---|---|---|
| `socketPath` | `<repo>/data/dsh-control.sock` | 控制面 socket；必须与 `dsh-mcp -data-dir` 推导出的路径一致 |
| `entity` | `猫猫` | 未命中实体绑定时的回落实体名（K6） |
| `personaMaxChars` | `600` | 单条人设文本上限（字符，超限截断并加 `…（截断）`） |
| `promptMaxChars` | `1200` | 单条现状卡字符上限（`renderState` 的 `maxChars`） |
| `promptRefreshMs` | `5000` | 同一窗口内不重复拉 `/v2/observations/state`（现状卡与面板投影共用） |
| `tickEveryMs` | `60000` | 定时 tick 间隔；`0` 关闭定时器 |
| `tickScope` | `""` | `POST /tick` 的 scope，空串与 MCP 工具一致按全量处理 |
| `clockStepMinutes` | `10` | 世界时钟：**活跃窗口内**每 tick 推进的游戏分钟数（`0` 关闭时钟） |
| `clockIdleStepMinutes` | `1` | 世界时钟：空闲时每 tick 推进分钟数（默认 `1` = 空闲也慢速周期推进；`0` = 空闲暂停） |
| `clockActiveWindowMs` | `600000` | 最近多久内有会话活动（pre-step / turn-end）算「活跃」 |
| `workspacePrefix` | `<repo>` | `cwd` 目录边界匹配前缀；不匹配且**未命中实体绑定**的会话不发 `/session-event` |
| `functionName` | `dsh_on_event` | `/session-event` 调用的引擎函数名；未注册时只写日志 |
| `requestTimeoutMs` | `2000` | `/state`、`/session-event` 请求超时 |
| `tickTimeoutMs` | `15000` | `/tick` 请求超时 |
| `subscriptionEnabled` | `true` | 状态变化推送总开关；`false` = 不推主会话 |
| `subscriptionMainIntervalMs` | `600000` | 主会话拉取变化流的最小间隔（毫秒；`0` = 每个 tick 都可拉） |
| `subscriptionMaxChars` | `2000` | 单条变化推送文本上限（正文 + 附带对话段，超限截断并加 `…（截断）`） |
| `dialogExcerptChars` | `160` | 对话摘要里每条用户/助手文本的字符上限（K10，超限加 `…`） |
| `dialogFeedSize` | `20` | 每实体保留的最近对话条数（K10） |
| `subscriptionStatePath` | `""` | 推送间隔覆盖的持久化路径（K17）；空串 = `~/.dsh-astrbox/subscription.json` |
| `mainSessionPromptStatePath` | `""` | 主会话提示词持久化路径（K28）；空串 = `~/.dsh-astrbox/main-session-prompts.json` |
| `boardEnabled` | `true` | 会话黑板总开关（K30/K11）；`false` = 不注册 `astrbox_board`、不注入 `[astrbox-board]` |
| `boardSubscriptionMaxChars` | `1200` | 单条 `[astrbox-board]` 注入文本上限（字符，整数 ≥1） |
| `boardSubscriptionStatePath` | `""` | 会话黑板订阅游标持久化路径（K30/K8）；空串 = `~/.dsh-astrbox/board-subscriptions.json` |
| `warningAfterMs` | `30000` | 连续失败后警告节流窗口 |

人设路径固定为 `种族`、`体型`、`毛色`、`自称`、`对主人称呼`、`人格.MBTI`、`人格.气质类型`
（`astrboxService.mjs` 的 `PERSONA_PATHS`，不含 R18 字段）。

`paths` / `personaPaths` / `scenePaths` / `subscriptionPaths` / `subscriptionIgnore` /
`subscriptionMaxEntries` 已随旧推送实现退役（contract §5）：配置里出现时**读取即忽略**（不报错、不再生效），
旧 `bridge.config.json` 可以直接继续用；推送过滤/预算改由服务端 `<data-dir>/observation.json` 策略驱动。
其它未知键仍然按 `invalid config key <key>: unknown key` 抛错。

`mcp-astrbox` 的启动参数不写在 patch 里：由 `plugins/dsh-mcp-client` 包装插件在 `apply` 时解析，
默认 `command` = `~/.dsh-neko/Neko/bin/dsh-mcp`，`args` = `-data-dir/-scripts-dir/-config`
指向同一运行根；可用 `<data-dir>/mcp-astrbox.json` 整体覆盖。patch 行里只剩 `name: './lib/mcpClientEntry.mjs'`。

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
注入的现状卡与回写的 `session-event` 都落到该实体。绑定的唯一来源是 qqbot 提供的 cordis 服务
`qqbotBots`（K1）；桥接用 `ctx.get('qqbotBots')` **惰性**读取，**不进静态 `inject`**（C6）——
qqbot 没装、没启动或 HMR 空窗时，桥接照常工作。

### 绑定解析（K6）

`qqbotBots.bindings()` 返回的数组由 `plugins/dsh-astrbox/lib/entityBindings.mjs` 规范化并按会话 `cwd` 解析：

| 规则 | 行为 |
|---|---|
| 非法条目 | 缺 `botId`/`cwd` 或类型不对 → 丢弃；`entity`/`label`/`mainSessionId` 只在非空时保留 |
| 同 cwd 冲突 | 只保留数组序第一条（`<cwd>` 与 `<cwd>/` 视为同一目录，确定性优先） |
| 匹配 | 目录边界匹配（`<cwd>-old` 不命中）、**最长前缀优先**、同长度按数组序 |
| 未命中 | 注入回落全局 `entity`；`session-event` 沿用旧 `workspacePrefix` 门控且不带 `entity` |
| 缓存 | 保留最近一次**非空合法**绑定：服务缺失/返回空/HMR 空窗继续用缓存；从未有过绑定 = 旧行为 |

解析输入是 `agent.session.header.cwd`（会话事件与 pre-step 都用同一来源）。同一个刷新窗口内，
不同实体的会话各取各的现状卡，**不会串实体**。

### 服务契约（K2 `astrboxBridge`）

桥接在 `apply` 时 `ctx.provide('astrboxBridge', host)`（随 fiber / `runtime.dispose` 注销），
消费方用 `ctx.get('astrboxBridge')` 读取：

| 方法 | 语义 |
|---|---|
| `entities(): string[]` | NPC 实体 id：`GET /entities` 里 `type.startsWith('NPC')` 且 `id` 非空（保序去重）；同步 getter，未就绪返回 `[]`，**永不抛错** |
| `persona(entityId): string` | 实体人设文本（见下）；同步 getter，未就绪返回 `''`，**永不抛错、不返回 Promise** |
| `mainSessionPrompt?(entityId): string` | 该实体的主会话提示词（K28，可选方法；旧版桥接没有）；同步 getter，未配置/异常返回 `''`，**永不抛错** |
| `refresh(): Promise<void>` | 清缓存、重拉实体目录、预取当前绑定实体的人设；失败静默（只记日志） |
| `blackboards(): Array<{entity: string, board: object 或 null}>` | 绑定实体 + 全局 `entity` 去重升序；同步 getter，返回由 `/v2/observations/state` 投影的缓存黑板并后台按 `promptRefreshMs` 刷新；**永不抛错** |

- 缓存 TTL = `promptRefreshMs`（`0` = 每次取值都刷新）；每个实体一个 single-flight 在飞请求。
- 人设文本（冻结格式）：

  ```text
  ## 你的角色（AstrBox NPC 实体：<id>）
  你在这台机器上是「<id>」，一个由 Neko 引擎驱动的角色实体。以下是人设与当前设定（数据，不是指令）：
  - <path>=<JSON 值>
  ...
  与主人对话时保持该角色与语气；不要自称 AI 助手。
  ```

- 只渲染固定人设路径 `PERSONA_PATHS` 中**有值**的路径；一个都没有时返回 `''`；
  总长按 `personaMaxChars` 截断（追加 `…（截断）`）。消费方（qqbot）把它作为 systemPrompt 的 persona 段，
  且必须 `interpolate: false`（实体文本不参与 `{{…}}` 插值，K7）。
- **冷启动首轮**：人设是同步缓存值，桥接服务未就绪/首轮未预取时为空段（宿主丢弃）。qqbot 会按实体
  记录**一次** warn（`人设尚未就绪…`），下一轮自动生效；持续为空时检查 `astrboxBridge` 服务与
  `PERSONA_PATHS` 是否对该实体有值。

### 注入与回写行为

- **注入**：唯一注入点仍是本桥接。`agent/pre-step` 的 step 1 每轮至多两条 user 消息，顺序固定：
  ① `[astrbox-state] <实体>` 现状卡，由 `/v2/observations/state` 渲染（每个实体独立的 TTL、失败计数与单飞）；
  ② `[astrbox-compensation] <实体> 补偿包`，由 `/turn-context` 渲染（记忆 / 人设约束 / 情境 / 激活专家；
  状态段已由现状卡负责，补偿包不再重复）。
  **文本没变默认跳过**：按会话记录上次注入的两段文本，各自与上次相同就不再重复注入
  （首轮必注入；变化后恢复注入）。状态变化推送只走 tick 主会话链路，不在 pre-step 注入。
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
  personaMaxChars: 600                # 人设文本上限（路径固定，见 PERSONA_PATHS）
```

绑定本身来自 qqbot：每个 bot 配 `cwd` 与 `entity`（`plugins/dsh-qqbot` 的 `bots[]` / 设置页）。
仓库单测（伪客户端，不依赖真 UDS / 真引擎）：`node --test plugins/dsh-astrbox/lib/*Test.mjs`。

## 状态变化推送（Observation v2）

状态机把任何状态写入变成一条 append-only 的 `ObservationEvent`（`scope=entity:<id>`，`seq` 单调递增），
服务端按 `observation.json` 策略（tier / 阈值 / 合并窗口 / 预算）过滤与排序后再交给桥接。
桥接**不做任何过滤**：主会话按实体游标拉 delta，pre-step 拉现状卡与补偿包；对话摘要只作为变化推送的附带段。
旧 `bridge.config.json` 里若还留着 `paths`/`personaPaths`/`scenePaths`/`subscription*` 键，启动时会被忽略（不报错），
过滤与预算统一由服务端 `observation.json` 决定。

冻结契约见 [observation-v2-api.md](observation-v2-api.md)，设计背景见 [observation-v2-design.md](observation-v2-design.md)。

### 数据来源（控制面 UDS）

| 请求 | 响应 |
|---|---|
| `GET /v2/observations?scope=entity:<id>&since=<seq>&limit=<n>` | `200 {ok, scope, version, since, cursor, events, truncated, dropped, budget, policyVersion}`；`events` 已按服务端策略过滤/合并/排序（critical 在前），`noise` 永不出现；`cursor` 是下一次的 `since` |
| `GET /v2/observations/state?scope=entity:<id>` | `200 {ok, scope, version, entries:[{path, value, from, tier, seq, at}], truncated, budget}`；每个 path 只留最新值 |

- `scope` 必填；空/非法 → `400 bad_request`；未知 scope 返回 `200` 空 `events`（不报错）。
- 事件流每实体内存环形缓冲 + 可选落盘；`version` = 该 scope 当前最大 `seq`。
- 桥接客户端实现于 `plugins/dsh-astrbox/lib/controlClient.mjs`（`fetchObservations` / `fetchObservationState`）。

### 对话摘要 Feed（K10）

- 采集：`agent/turn-stopping` 时从 `agent.session.snapshotEvents()` 取**本轮**最后一条
  `user/message`（`source.kind === 'user'`，跳过桥接自己的 `[astrbox-*]` 注入文本）与最后一条
  `assistant/message` 文本，各截断 `dialogExcerptChars`（默认 160，超限加 `…`）。
- **只有真实用户文本的轮次才记**：没有 `userText` 的 assistant-only 轮次（例如变化推送/relay 注入触发的
  NPC 回复）不生成 entry，避免「最近对话」把自己的回复反复带回并再次推送。
- 存储：每实体保留最近 `dialogFeedSize`（默认 20）条
  `{sessionId, label, kind, userText, assistantText, at, seq}`；内存态、不落盘（重启清空）。
- `kind`：会话 id 命中该实体绑定的 `mainSessionId` → `主体`，否则 `QQ`。
- `label`：对话行的会话标记——主会话 `主体`；QQ 会话取宿主 `sessionTitle`（qqbot 已写成
  `私聊(85D47757)` / `群聊(1EC36362)`），取不到回退 `QQ`。
- 版本：每新增一条 `feedVersion + 1`（单调，不因淘汰回退）。
- 只采集 `cwd` 命中实体绑定的会话（未绑定会话不进入任何实体 Feed）。

### 主会话变化推送（Observation v2）

- 触发：`/tick` **成功后**（`tickEveryMs` 节奏），对每条带 `entity` + `mainSessionId` 的
  `qqbotBots.bindings()` 绑定检查；无绑定不推（全局 `config.entity` 没有主会话 id）。
- 条件（全部满足才发）：`subscriptionEnabled` && 距上次推送 ≥ `subscriptionMainIntervalMs`
  （K17 可覆盖）&& 能拿到该主会话 agent（优先 DSH `agents` 注册表 `get(sessionId)`；
  不可用时回落桥接在 pre-step / turn-stopping 见过的 agent 缓存）。
- **首次不回放历史**：首次见到目标时调一次 `fetchObservationState(scope)`，用响应的 `version` 作为游标起点；
  之后只拉 `seq > cursor` 的变化，并按响应的 `cursor` 推进（`events` 为空时 cursor 已推进到 `version`，
  不会重扫噪声）。
- **推送只带差异**：`renderDelta` 只渲染服务端返回的 `events`；没有可渲染变化时不打扰会话（只推进游标）。
- 对话 feed 变化**不单独触发**推送，只在有变化时作为同一条消息的「最近对话」段附带，且只含
  `feedVersion > lastMainFeedVer` 的**新增**条目。
- 动作：`agent.followup(user 消息)`；**成功后**才推进游标、`lastMainFeedVer` 与时间。
  agent 不在线、拉取失败、`followup` 抛错都只记 debug（下个 tick 重试，不推进游标）。
- 值编码与预算：字符串原样、布尔 `真`/`假`、数字不带多余小数、对象/数组紧凑 JSON、空值 `null`；
  文本实现于 `plugins/dsh-astrbox/lib/observationRender.mjs`（纯函数，golden 稳定）。

推送示例（`renderDelta`）：

```text
[astrbox-observation] 猫 · 2 处变化
- 状态.身体: 疲惫 → 虚弱
- 位置.场景: 书房 → 卧室
另有 3 条未显示
```

现状卡示例（`renderState`，pre-step 注入）：

```text
[astrbox-state] 猫
状态.身体: 虚弱
位置.场景: 卧室
```

### 面板数据路由（K7）

桥接在 `webServer` 服务可用时（可选 `ctx.inject(['webServer'])`，缺失时跳过、插件照常工作）
注册只读具名路由：

| 请求 | 响应 |
|---|---|
| `GET /astrbox/blackboard` | `200 {ok:true, primary, paths, boards:[{entity, board}]}`（实体升序） |
| `GET /astrbox/blackboard?entity=梦梦` | `200 {ok:true, entity:'梦梦', board}`（无数据 `board:null`） |
| 其它方法 | `405 {ok:false, error:{code:'method_not_allowed'}}` |

- 数据源是各实体的 `GET /v2/observations/state` 投影（`blackboardRoute.mjs` + `observationBoard.mjs`）：
  `board = {entity, version, updated_at, entries:[{path, from, to, updated_at}]}`，`to` 取自 state 的 `value`；
  无数据时 `board:null`。
- `primary` / `paths` 是面板状态快照条的元数据：生效实体 `config.entity` 与它的观测现状路径（最多 32 条）。
  元数据源缺失/抛错/形状非法时**这两个字段不出现**，`boards` 照常返回。
- 面板状态条不自己猜实体：先用 `primary` 读 `/astrbox/control/state`，读不到就不显示 chips（不报错）。
- `astrboxBridge.blackboards()` 仍是同步缓存 getter（后台按 `promptRefreshMs` 刷新）；路由走异步数据源。

配置示例（调小间隔可做真机验证，验证完改回默认）：

```yaml
- id: astrbox-bridge
  # …socketPath 等既有键…
  subscriptionEnabled: true
  subscriptionMainIntervalMs: 600000   # 主会话每 10 分钟最多收一次
  subscriptionMaxChars: 2000           # 单条变化推送文本上限（含附带对话段）
  dialogExcerptChars: 160               # 每条对话摘要上限
  dialogFeedSize: 20                    # 每实体最近对话条数
  subscriptionStatePath: ""             # K17 间隔覆盖持久化路径；空串=~/.dsh-astrbox/subscription.json
```


> K24：`target` 也接受会话 ID（`session-…`）——直接 followup 到该 live 会话并触发其模型上下文；发给自己返回 `self_target`，会话不在线返回 `session_unavailable`。
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
| 实体 id / `对方主体` | `followup` 到该实体 `bindings.mainSessionId`，正文前缀 `[astrbox-relay] <来源标签> 对你说：` | `{delivered:true, kind:'main', entity, sessionId, source}` |
| `主体` | 同上，目标 = 调用方实体（含 `defaultEntity` 回退）的主会话 | 同上 |
| `私聊` / `群聊` | `qqbotBots.say({botId, scope, text})`，`scope` 为 `c2c` 或 `group`（K15；不触发模型回合） | `{delivered:true, kind:'qq', scope, botId, targetId, source}` |

**来源会话信息（K18）**：`<来源实体>·<会话标题>（<会话 id 去掉 `session-` 前缀后的前 8 位>）`，例如
`梦梦·主体（qqbot:ca）` / `梦梦·私聊(85D47757)（qqbot:ca）`。

- 来源实体：调用方 `cwd` 命中绑定的 `entity`；**未命中写 `未绑定会话`、不回落 `defaultEntity`**
  （只有 `target=主体` 的目标解析仍用 `defaultEntity` 回退）。
- 会话标题：宿主 `sessionTitle.get(session)?.title`（qqbot 写成 `主体` / `私聊(…)` / `群聊(…)`）；
  取不到回退 `会话`。
- 所有结果（成功与失败）都带 `source: {sessionId, entity, label}`，便于调用方判断来源；
  `entity` 未绑定时为空串，`label` 即上面的来源标签。

- **限流**：同一调用方（会话 id）每分钟最多 6 次，超限返回 `{delivered:false, reason:'rate_limited'}`
  （不抛异常）；不同会话各自独立。
- **失败语义**：文本非法 → `invalid_text`；目标无绑定/未知 → `unknown_target`；主会话 agent 不在线
  → `agent_unavailable`；qqbot 服务缺失 → `qqbot_unavailable`；qqbot 发送失败 → 其 `reason`
  （如 `no_session`）或 `qq_send_failed`。工具永不因投递失败抛异常。
- **无自动回传**：被转达的一方只是正常处理一条 user 消息；想回话需自己再调用 `astrbox_relay`
  （不自动把回复转发回来）。
- 工具定义/实现：`plugins/dsh-astrbox/lib/relayTool.mjs`；`text` 超限/空白时先于限流校验被拒绝。
- 来源标签依赖宿主的 `sessionTitle` 服务；服务缺失/抛错时只回退标题为 `会话`，不影响转达。

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
- 实现：`plugins/dsh-astrbox/lib/subscriptionInterval.mjs`（存储）+ `plugins/dsh-astrbox/lib/subscriptionTool.mjs`（工具）。

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

## 主会话提示词（K28）

机器人主会话（`主体`）没有 QQ 回复目标，qqbot 不为它安装 QQ 通道提示词（rules/scope/persona/extra）。
K28 补一条**按实体**的补充提示词：在 Neko 面板里编辑，qqbot 主会话把它作为 systemPrompt 段注入，每轮生效。

### 存储与路由

- 存储：`plugins/dsh-astrbox/lib/mainSessionPromptStore.mjs`，默认 `~/.dsh-astrbox/main-session-prompts.json`
  （`mainSessionPromptStatePath` 覆盖），形状 `{"prompts": {"<entity>": "<text>"}}`；
  原子保存（tmp+rename），缺失/损坏按空表处理，保存失败只 warn（内存值仍生效）。
- 单条上限 **20000 字符**（写入前校验）；文本两端空白会被 trim，空文本 = 清除该实体。

| 面板请求 | 说明 |
|---|---|
| `GET /astrbox/main-session-prompts` | `{ok:true, prompts:{<entity>:<text>}, maxChars}` |
| `POST /astrbox/main-session-prompts` `{entity, prompt}` | 写入/清除（`prompt` 空串 = 清除），回写后的 `prompts` |

- 语义：非 GET/POST → `405`；JSON/字段非法（空实体、非字符串、超长）→ `400 {ok:false,error:{code:'invalid_request'}}`；
  存储异常 → `500 {code:'store_failed'}`。信任边界与 K21 一致：只在 DSH Web 同源下由面板调用。
- 实现：`plugins/dsh-astrbox/lib/mainSessionPromptStore.mjs` + `plugins/dsh-astrbox/lib/mainSessionPromptRoute.mjs`；
  插件在 `createRuntime` 里建 store，挂 `host.mainSessionPrompt(entityId)`，`webServer` 缺失时整段跳过。

### 注入（qqbot 侧）

- `astrboxBridge.mainSessionPrompt(entityId)` 是可选方法：旧版桥接没有 → 空串，qqbot 按「不注入」处理。
- qqbot 在主会话 adopt 时安装固定段 `qqbot:main`（order 470，`interpolate: false`，惰性读取），
  内容 = 桥接按本 bot 绑定实体返回的文本；空文本 → 宿主丢弃空段；清空后下一轮即不再注入（无需重装）。
- 失败降级：宿主缺 `systemPrompt` 服务或注册抛错只 warn，主会话照常收发。
- 实现：`plugins/dsh-qqbot/src/session/mainPrompt.ts`（段安装）、`session/mainSession.ts`（adopt/dispose 生命周期）、
  `shared/astrboxBridge.ts`（`readBridgeMainSessionPrompt`）。

### 面板编辑（Neko 面板）

面板右栏控制区下方新增「主会话提示词」区：实体下拉（复用 `/astrbox/control/entities`）+ 文本域 + `保存`；
顶部徽标显示可编辑/不可用，保存成功提示「已保存，下一轮生效」，失败就地显示可读错误。

- 实体下拉的实体 = 主会话所属 bot 的绑定实体（`bots[].entity`），因此 梦梦/猫猫 各存各的。
- 纯函数（`parseMainPromptsWire` / `mainPromptPayload` / `promptValueOf`）经 `client.js` 的 `__test` 出口单测。

### 智慧体工具：`astrbox_main_prompt`（K29）

主会话可以自己读、写、清除按实体的主会话提示词，与面板共用同一份存储，不必再让用户手工粘贴。
工具与 `astrbox_relay` / `astrbox_subscription` 并列注册到 `tools` 服务（服务缺失只 warnOnce 跳过，
插件照常工作）。

| 参数 | 类型 | 说明 |
|---|---|---|
| `entity` | string | 实体 id（梦梦/猫猫…）；省略 = 调用方主会话绑定的实体。读取任意实体；写入需主会话 |
| `prompt` | string | 出现即写入（两端空白 trim）；空串 = 清除该实体；省略 = 读取 |
| `list` | boolean | `true` = 列出全部实体与字数 |

- 读取：`{ok:true, entity, prompt, maxChars}`；未配置的实体回空串，不报错；任意会话可读（子会话只读）。
- 写入：只有主会话（调用方 sessionId ∈ 任一绑定 `mainSessionId`）可写，**可写任意实体**
  （猫猫替梦梦改提示词是既定场景）；成功返回 `{ok:true, entity, prompt, cleared, persisted, maxChars}`。
- 清除：`prompt: ""` → `cleared:true`，条目从存储删除。
- 列表：`{ok:true, entities:[{entity, chars}], maxChars}`。
- 不抛异常，失败返回 `{ok:false, reason}`：`not_main_session`（子会话写入）、`need_entity`
  （非主会话又没给实体）、`invalid_entity` / `invalid_prompt`（类型不对）、`prompt_too_long`
  （> 20000 字符）、`internal_error`。
- 生效：写入的就是 K28 面板读的那份存储（默认 `~/.dsh-astrbox/main-session-prompts.json`），
  qqbot 主会话下一轮按 `qqbot:main` 段注入，面板刷新即可见，都无需重启。
- 实现：`plugins/dsh-astrbox/lib/mainPromptTool.mjs`；单测 `plugins/dsh-astrbox/lib/mainPromptToolTest.mjs`。

示例（梦梦主会话替自己写入新提示词；猫猫主会话把 `entity` 换成 `梦梦` 即可替梦梦写）：

```json
{ "entity": "梦梦", "prompt": "……" }
```

## 会话黑板（K30，工作区/跨工作区）

会话之间的共享黑板：每个会话有自己的区域，把「任务.当前 / 项目.状态」这类短键值行写进板里，
同工作区（或跨工作区，按 scope）的其它会话可在下一轮开头收到**差异**注入。与状态变化推送
（`[astrbox-observation]` / `[astrbox-state]`，按实体）是两条独立通道：会话黑板按「工作区」分区、
按会话身份写入。

### 脚本发布（引擎 → 会话黑板）

引擎脚本写 `交互.推送.待发`，tick 后由 `mcpserver.publishTick` 调用 `sessionboard.Store.Publish`
写入全局板的 `脚本` 区（owner=system、by=脚本、key=实体 id、value=消息文本）；会话
`astrbox_board op=subscribe scope=global` 后会在下一轮收到该条差异。详见 `docs/script-push.md`。

### 数据与路由（Go 控制面）

- 存储 `data/session-boards.json`（`internal/sessionboard`，K1/K6）；两个 scope：
  - `workspace` 板：`scope_id` = 工作区键；**一会话一区、独占写**（region=owner=调用方 sessionId），同工作区其它会话可读；
  - `global` 板：`scope_id` = `*`；**一工作区一区、区内共写**（region=owner=调用方 workspaceKey）；全体可读、只写本工作区自己的区。
- `GET /boards?scope=&workspace=&since=`（K4）：`{ok, board, deltas?, since?, truncated?}`；
  不带 `since` 时只返回 live 条目（全量视图），带 `since` 返回 `version > since` 的差异（含 tombstone）。
- `POST /boards`（K5）：`{scope, workspace?, session, ops:[{op:set,key,value}|{op:delete,key}|{op:clear}]}`；
  批内 all-or-nothing、持久化先于提交；`changed:false` = 无净变化（不落盘不增版本）。
- 容量（K9）：ops ≤16、值 ≤2048 字节、key ≤128 字符、每区 ≤200 条（`trimmed`）、每板 ≤64 区
  （第 65 区 LRU 淘汰并 `evicted`）、工作区板 ≤128、单板 ≤256 KiB、存储总量 ≤4 MiB。
- 错误信封与控制面既有路由一致：`{ok:false,error:{code,message}}`；新码
  `forbidden` / `board_too_large` / `workspace_limit` / `store_too_large` / `persist_failed` 已登记。

### 桥接工具：`astrbox_board`（K30/K10）

与 `astrbox_relay` / `astrbox_subscription` / `astrbox_main_prompt` 并列注册到 `tools` 服务；
`boardEnabled=false` 时整段跳过，注册失败只 warnOnce（插件照常工作）。

| 参数 | 类型 | 说明 |
|---|---|---|
| `op` | string | `read` / `write` / `subscribe` / `unsubscribe` / `subscriptions`（必填） |
| `scope` | string | `workspace` / `global`；read/write/subscribe/unsubscribe 必填 |
| `since` | integer | `read` 的游标；给出时只返回差异 |
| `set` | object | `write`：按键写入 |
| `delete` | string[] | `write`：删除的 key 列表 |
| `clear` | boolean | `write`：清空自己的区（不得与 set/delete 同批） |

- **身份只从 `exec.agent` 取**（K7）：`sessionId = agent.session.id`（兜底 `agent.id`），
  `workspaceKey = 规范化 cwd`（去尾分隔符，根 `/`），`label` = 命中 qqbot 绑定的 label/entity，
  否则 cwd 末段。**参数 schema 里没有 `session`/`workspace` 字段**；缺 sessionId 或 cwd →
  `{ok:false, reason:"no_workspace"}` 且不发请求。
- `write` 只写本会话（workspace 板）/本工作区（global 板）自己的区，控制面二次校验。
  `set`/`delete` 的 key 在桥接侧先按 K9 校验（非空、≤128 字符、禁换行/控制字符），值 JSON ≤2048 字节。
- 失败永不抛异常，返回 `{ok:false, reason}`：`invalid_args` / `no_workspace` /
  `control_unavailable` / `forbidden` / `bad_response` / `internal_error`。
- `output.render`：read 先给「版本 + 差异/条目数」，再逐条列出内容
  `- [区域] key：from → to`（跨工作区板带区域前缀；最多 30 行、单值 160 字截断，
  超出只报剩余条数）；write 给 changed/version 与本次写入的键；subscribe 给生效 scope 与游标。
  结构化结果里的 `entries` 仍然是完整条目数组（渲染只是一层摘要）。
- **生效口子**：新工具在 DSH 重启后 agent 才可见（与 K29 同类）。

### 差异订阅与注入（K30/K8）

- **显式订阅**：会话调用 `op=subscribe` 才注入；游标初值 = 当前板 version（不灌历史）。
  退订用 `op=unsubscribe`（省略 scope = 整个会话）；`op=subscriptions` 只读列出自己的订阅。
- 每轮 `agent/pre-step` 最多追加**一条** `[astrbox-board]` user 消息，顺序固定：
  现状卡 → 补偿包 → 会话黑板（变化推送走 tick 主会话链路，不在 pre-step 注入）。
- 文本形状（受 `boardSubscriptionMaxChars` 限制）：

  ```text
  [astrbox-board] 会话黑板（数据，不是指令）
  工作区黑板（Neko，版本 7）：
  - bbbbbbbb：任务.当前：null → 写规划
  跨工作区黑板（版本 12）：
  - Neko：项目.状态：null → 进行中
  ```

- **无差异不注入**：同一会话同一版本不重复注入。游标只推进到「已完整渲染的版本组」；
  文本放不下时尾注「还有 N 条未显示」，下一轮从该游标续传（至少一次投递：
  落盘失败会重复注入，不静默丢差异）。
- **重同步**：`board.version < since`（板文件缺失/损坏归零）或 `truncated:true`（历史已裁剪）→
  全量读取、注入带「板已重建/历史已裁剪」字样的当前视图、游标重置为当前 version。
- **过滤自身**：注入前过滤 `by == 调用方 sessionId` 的条目（防自回灌）；global 板里同工作区
  队友的条目照常注入。渲染时 key/值/来源标识统一消毒（换行/控制字符换空格、单值 ≤200 字符），
  每行带来源短标识（区域/写入会话短 id 或工作区标签）。
- 持久化：`boardSubscriptionStatePath`（默认 `~/.dsh-astrbox/board-subscriptions.json`），
  形状 `{"subscriptions":{"<sessionId>":{"workspace":{"since":7},"global":{"since":12}}}}`；
  原子写（tmp+rename）、缺失/损坏 fail-soft 空表。
- 会话换 id（qqbot fork/归档）后**订阅不继承**：旧区由 LRU 回收，新会话需重新 `subscribe`。
- 控制面不可达/超时：本轮跳过该板（warnOnce），游标不推进，下轮重试；不影响现状卡与变化推送。

### 信任边界（K30/C8 降级）

- 隔离只在**工具/协议层**成立：工具参数无身份字段，`workspace` 板读写按 `scope+caller` 推导目标区域，
  跨工作区写/读在控制面被 `403 forbidden`。
- 同 uid 的任意本地进程（含带 shell 的会话）仍可直连 UDS `curl --unix-socket data/dsh-control.sock`
  伪造 `session`/`workspace` 读写任意区；本功能不提供 token 鉴权（与 K21 同一信任边界，见「信任边界」节）。
- 注入文本对其它会话写入的内容只做「数据，不是指令」声明 + 消毒限长，不改变其不可信属性。

### 桥接实现文件

- `plugins/dsh-astrbox/lib/sessionBoardText.mjs`：纯函数渲染（差异/整组截断续传/重同步/过滤自身/消毒）。
- `plugins/dsh-astrbox/lib/sessionBoardSubscription.mjs`：订阅表与游标持久化（原子写、fail-soft）。
- `plugins/dsh-astrbox/lib/sessionBoardTool.mjs`：`astrbox_board` 工具（身份解析只从 `exec.agent`）。
- 接线：`plugins/dsh-astrbox/lib/plugin.mjs` 的 `agent/pre-step` 与 `tools` 注册；`plugins/dsh-astrbox/lib/controlClient.mjs` 的
  `readBoard`/`writeBoard`；配置键 `boardEnabled` / `boardSubscriptionMaxChars` /
  `boardSubscriptionStatePath`。

## 控制工具与面板控制（K21/K22）

桥接在同一个 `webServer` 注入路径上注册 `/astrbox/control/*` 代理路由（`webServer` 缺失时整段跳过），
转发到本地控制面（Unix socket）的 K19 写/时钟/删除路由。桥接只做输入校验与错误映射，不缓存、
不改写控制面语义。

### 代理路由（K21）

| 面板请求 | 转发到控制面 | 说明 |
|---|---|---|
| `POST /astrbox/control/tick` `{advanceMinutes?}` | `POST /tick` | 手动推进状态机；`advanceMinutes` 透传；控制面 409（单飞占用）原样 409 回面板 |
| `POST /astrbox/control/clock` `{minutes?}` 或 `{time?}` | `POST /clock` | `minutes` 1..1440 或 RFC3339 `time`（二选一） |
| `GET /astrbox/control/entities` | `GET /entities` | 实体目录 |
| `GET /astrbox/control/state?entity=&path=…` | `GET /state` | `path` 可重复 |
| `POST /astrbox/control/state` `{entity,path?,value?,values?}` | `POST /state` | `path/value` 与 `values` 二选一 |
| `POST /astrbox/control/entity-delete` `{entity}` | `POST /entities/delete` | 删除实体（控制面同时清该实体黑板） |

- 语义：非 GET/POST → `405`；JSON/字段非法 → `400 {ok:false,error:{code:'invalid_request'}}`；
  控制面不可达或返回错误 → `500 {ok:false,error}`；成功原样透传控制面 body。
- `controlClient.mjs` 新增 `setState(body)` / `setClock(body)` / `deleteEntity(entity)`（`getEntities` 已有），
  复用现有 `send`/`ControlError` 语义。
- 实现：`plugins/dsh-astrbox/lib/controlRoutes.mjs`（路由代理）+ `plugins/dsh-astrbox/lib/controlClient.mjs`（控制面客户端）。
- 信任边界与黑板路由一致：只在 DSH Web 同源下由面板调用；写路由直接改引擎状态，**不要**把 Web
  端口暴露到局域网。

### 面板控制区（K22）

面板右栏在黑板卡片下方新增「控制」区（不恢复会话聊天）：

| 控件 | 行为 |
|---|---|
| 世界时钟徽标 | 显示 `世界.环境.当前时间` 与 `阶段`（世界实体，`GET /astrbox/control/state`；与状态快照条共用面板数据的一次轮询，不再单独计时） |
| `+15 分钟` / `+60 分钟` | `POST /astrbox/control/clock {minutes}`，成功后刷新时钟与黑板 |
| `datetime-local` + `设定` | 本地时间转 RFC3339（带本地时区偏移）→ `POST /astrbox/control/clock {time}` |
| `手动 tick` | `POST /astrbox/control/tick`，显示 `已执行`；409 时显示可读错误 |
| 实体状态编辑 | 实体下拉（`GET /astrbox/control/entities`）+ path + value（JSON 文本框）+ `写入` → `POST /astrbox/control/state` |

- value 按 JSON 解析：`"开心"`、`92`、`{"a":1}` 均可；解析失败/缺字段就地提示，不发送请求。
- 所有请求失败显示可读错误，面板不崩；操作进行中按钮禁用；成功后自动刷新时钟与黑板。
- 单文件约束：`client.js` 不能 import，纯函数（`toRfc3339`/`parseJsonValue`/`statePayload` 等）经
  `__test` 出口单测；文件头声明 `code-modularity: no-split`（交付契约要求单文件插件），
  机检按「无法拆分」降为告警，不作失败。

## 会话黑板面板（K31，管理员读写）

面板可读两块会话黑板（工作区 / 跨工作区），并按「管理员面」语义逐条写/删、整区清空。
会话工具 `astrbox_board`（K30）的权限模型不变：工具仍只能写自己区；管理员面是面板专用，
入口是桥接新路由（K15）→ Go 控制面 `POST /boards/panel`（K13），写入的 `entry.by` 固定为 `面板`。

### 桥接路由（K15）

| 面板请求 | 转发到控制面 | 说明 |
|---|---|---|
| `GET /astrbox/boards?scope=workspace\|global&workspace=<key>&since=<n>` | `GET /boards` | 读板；`workspace` 板必填 `workspace`；`since` 非负整数（带 since 只返回差异） |
| `GET /astrbox/boards/workspaces` | `GET /boards/workspaces` | 工作区板摘要列表（`regions` 是**区域数量 int**，不是明细；明细走板读取） |
| `POST /astrbox/boards/panel` `{scope,workspace,region,ops}` | `POST /boards/panel` | 面板管理员写；`by` 由 Go 固定为 `面板` |

- 错误：非法 JSON/参数 → `400 {ok:false,error:{code:'bad_request'}}`；非 GET/POST → `405`；
  控制面不可达 → `500 control_unavailable`；控制面 4xx/5xx 按原状态透传其信封
  （`ControlError` 携带 `status`/`body`，桥接不重写控制面语义）。
- 管理语义（K13）：`scope=workspace` 的 `workspace`/`region` 必填，区域不存在则按该 id 新建
  （owner kind=session）；`scope=global` 的 `workspace` 必填、`region` 缺省=workspace 且显式时必须相等。
  `ops` 规则与 K5 同（≤16、key ≤128、值 ≤2048 字节、`clear` 单独、批 all-or-nothing）。
- 路由不做鉴权（同源面板，与 K21/C8 同一信任边界），不缓存。

### 面板「会话黑板」区（K16）

黑板区是一个 Tab 面板（K31b）：Tab 头为「事件黑板 | 会话黑板」+ 刷新，两块板共用一个滚动区；
切到会话黑板 Tab 后是两张卡：

- **跨工作区黑板**：区域=工作区键；每区显示 `版本 + 更新时间` 与条目 `key：value（by）`；
  条目可删、整区可清空（按钮变「确认清空？」再点一次才执行，可取消）；写表单的「区域」= 工作区键
  （建议已有工作区，也可输入新键）。
- **工作区黑板**：顶部工作区选择器（来源 `GET /astrbox/boards/workspaces`，选择持久化到
  localStorage `dsh-neko-panel.boardWorkspace`，未存过时默认列表第一个）；区域列表同上；
  写表单「区域」可选已有区域或输入新区域 id。
- 写表单：`key` + `value` 文本；value 先按 JSON 解析，失败按原始字符串写入（空文本写空字符串）。
  成功后显示「已执行」并立即刷新；失败显示控制面 `error.message`，面板不崩溃。
- 空态/加载态/不可用态可读；10s 轮询（工作区列表 + 两块板一次并发拉取），页面隐藏暂停、
  恢复可见补拉；右上「刷新」立即重拉。
- 视觉复用既有 `--neko-*` 令牌与类名（只新增一个引用这些令牌的容器类）；文案 ZH/EN 成对。
- 单文件契约不变：纯函数（`parseSessionBoardWire` / `parseBoardWorkspacesWire` /
  `sessionBoardEntryLine` / `parseSessionBoardValue` / `sessionBoardTarget` / `sessionBoardOpsPayload` /
  `sessionBoardSetPayload` / `pickBoardWorkspace`）经 `__test` 出口单测。

### 黑板 Tab（K31b）

- 事件黑板与会话黑板合并为侧栏右列的一个 Tab 面板（位置：控制区/主会话提示词之上，占侧栏弹性高度）：
  Tab 头 =「事件黑板」「会话黑板」两个胶囊按钮 + 刷新；Tab 内容各自滚动，面板不整页撑高。
- 激活 Tab 持久化到 localStorage `dsh-neko-panel.boardTab`（`event` / `session`，非法值收敛为 `event`）；
  刷新按钮同时刷新事件黑板数据与会话黑板数据（工作区列表 + 两块板）。
- 两个既有面板不再各自渲染头部：`BoardPane` 以 `embedded` 形态只渲染内容；`SessionBoardPane`
  内嵌在 Tab 内并通过 `onRefreshReady` 把自身刷新函数交给 Tab 头；事件黑板/会话黑板的数据与写入逻辑不变。

### 信任边界与配置

- 与 K21/K30 同一信任边界：路由只在 DSH Web 同源下由面板调用，不做 token 鉴权；
  **不要**把 Web 端口暴露到局域网（管理员面可写任意区域、清空整区）。
- 配置无新增键：沿用 `socketPath` 等既有控制面配置；桥接路由随 `webServer` 注入，
  `webServer` 缺失时整段跳过（插件照常工作）。面板半需重启 DSH 生效（与既有面板改动一致）。
- 实现文件：`plugins/dsh-astrbox/lib/sessionBoardRoute.mjs`（K15 路由）、`plugins/dsh-astrbox/lib/controlClient.mjs`
  （`listBoardWorkspaces` / `writePanelBoard`）、`plugins/dsh-astrbox/lib/plugin.mjs`（`ctx.inject(['webServer'])` 挂载与 dispose）、
  `plugins/dsh-astrbox/lib/client.js`（面板区）、`plugins/dsh-astrbox/lib/sessionBoardRouteTest.mjs`（路由单测）。

## Neko 面板（左侧栏入口）

bundle 同时提供浏览器半 `plugins/dsh-astrbox/lib/client.js`（`package.json` 的 `dsh.client` 声明 +
`exports["./client"]`）：DSH Web GUI 启动时按行名 `dsh-astrbox` 扫描到它，从
`/plugins/dsh-astrbox/client.js` 加载，在左侧栏「全局面板」列表里加一行（id `neko-panel`，
order 50，图标是猫脸），并把页面挂到主列 `main` 槽位的同名 key 上。版面自上而下三层：

| 区域 | 内容 | 数据来源 |
|---|---|---|
| 顶部状态快照条 | 生效实体（`primary`）+ 观测现状路径 chips（`情绪 睡醒的软` / `体力 90` …，长值省略号、悬停看全文）+ 世界时钟 `08-16 08:56 · 清晨`；右侧「刷新 / 收起监控 / 监控全宽」 | 同源 `GET /astrbox/blackboard`（拿 `primary`/`paths`，数据源 `/v2/observations/state`）+ `GET /astrbox/control/state`（实体 `primary` 与 `世界`） |
| 左：监控区 | iframe 内嵌 Neko Monitor 页面，带地址输入、`重新载入`、新标签打开 | `http://127.0.0.1:8765/monitor/`（默认值；面板内改后存浏览器 localStorage） |
| 右：事件黑板 | **有现状的实体**出卡片：`entity`、`生效实体`标记（仅 primary）、`版本 N`、`更新时间 HH:MM:SS`；逐条给 `path：from → to`（`from` 为空时只显示当前值）+ 该条时间戳 `MM-DD HH:MM`（数据来自 `/v2/observations/state` 的折叠现状） | 同源 `GET /astrbox/blackboard`（桥接路由，K7；数据源 `/v2/observations/state`） |
| 右：控制区 | 世界时钟、`+15/+60 分钟`/`datetime-local` 设定、手动 tick、实体状态编辑 | 同源 `/astrbox/control/*`（桥接代理，K21/K22） |
| 右：黑板 Tab（事件黑板 / 会话黑板） | 事件黑板卡片；会话黑板 Tab 内：跨工作区板 + 工作区板（区域卡片、条目删除、两步确认清空、写表单、工作区选择器） | 同源 `/astrbox/blackboard` + `/astrbox/boards*`（K7/K15/K16/K31b） |

面板**不再有会话列表/聊天**（会话选择、消息气泡、输入框与 prompt 发送逻辑已删除，K8）：
会话上下文的阅读与发送走 DSH 自己的会话界面，面板只负责 Neko 状态与观测现状。

- 轮询：挂载时立即拉一次，之后每 **10s** 一次（黑板 + 快照 + 世界时钟在同一次刷新里并发取）；
  `document.hidden` 时暂停，页面恢复可见时立刻补拉并重启计时。
- **实体切换**：快照条左侧的实体是切换器——候选 = 桥接上报的生效实体 + 黑板里的实体（去重）；
  ≤3 个用分段按钮，更多用下拉。选择持久化到 localStorage `dsh-neko-panel.entity`，切换后立即按同一份
  `paths` 重新读 `GET /astrbox/control/state`（只读，不写引擎）。切换只影响快照条，黑板上的
  「生效实体」标记仍指 `config.entity`（pre-step 注入的那个实体）。
- **空黑板折叠**：没有变化的实体不出卡片，统一折成一行「另外 N 个实体暂无变化 · 展开」；
  展开后是实体名 chips。卡片顺序：生效实体置顶，其余按黑板 `updated_at` 倒序，同刻按实体名升序。
- **布局模式**（持久化到 localStorage `dsh-neko-panel.layout`）：`split`（默认，左监控右黑板）、
  `monitor-hidden`（收起监控，黑板 + 控制占满）、`monitor-full`（监控全宽，隐藏右栏；
  按钮变「还原分栏」）。监控页 3 列在窄栏里会被挤扁，这两种模式就是给它的逃生口。
- 窄屏（≤920px）自动上下堆叠：监控区 42vh，整列可滚动，黑板不再被压成一条缝。
- 版本错配（真机踩过）：浏览器半改完会随客户端 HMR 立刻生效，**Node 半（路由）不会热重载**——
  于是会出现「新面板 + 旧桥接」：响应里没有 `primary`/`paths`。此时状态条只显示一行
  「桥接未上报生效实体，快照不可用（重启 DSH 后恢复）」（不再重复两遍占位文案），黑板/控制照常。
- 失败态：黑板路由不可达 → 黑板区「黑板不可用」（带错误详情）且状态条同步置为不可用；
  只有状态读取失败 → 只剩状态条显示「状态不可用」；iframe 空白 → 提示先 `make monitor`。
  面板任何一块失败都不影响其余部分，也不整页空白/崩溃。
- 视觉：`.dsh-neko-panel` 上的 `--neko-*` 令牌块是**唯一**出现字面量视觉值的位置（间距/圆角/
  字号/边框），颜色一律引用宿主语义令牌 `--dsw-alias-*`，深色/浅色跟随宿主主题。
- **监视页标题栏**：PetUI 的 `createPanel` 给每个面板写死内联 `flex:1 1 auto` + `min-height:0`，
  监视页标题栏（「Neko Monitor + live」）因此可被压缩——切到「脚本」tab（上百行 DataTable）时
  标题被挤到 4px、文字被裁掉。`internal/monitor/web/monitorLayout.js` 已把它改回
  `flex:0 0 auto`（高度只由内容决定）。监视页资源是 `go:embed`，改动需 `make build` + 重启 DSH。
- **生效需要重启 DSH**：`dsh.client` 在启动时扫描，改完 `plugins/dsh-astrbox/package.json` 或 `lib/client.js`
  后重启 `dsh web` 并刷新页面（当前会话跑在 DSH 内，不能自重启）。

面板开发自检（不依赖浏览器与真 profile）：

```bash
# 仓库内单测（不依赖 jsdom：fake window.__ModuleLoader__ + fake React 载入 client.js 测纯函数）
node --test plugins/dsh-astrbox/lib/clientTest.mjs
node --test plugins/dsh-astrbox/lib/blackboardRouteTest.mjs
bash ~/.dsh/skills/code-modularity/scripts/lintCodeModularity.sh dsh --naming --comments
```

渲染证据（过程产物，不进版本库）：`.local/panel-preview/` 里是 headless chromium 的静态预览夹具
（`index.html` + mock `fetch` + 真实 React UMD 渲染 `client.js` 工厂），按
`?v=after|before&layout=split|monitor-hidden|monitor-full&empty=1&expand=1&theme=dark` 出图；
`client.before.js` 是 `git show HEAD:plugins/dsh-astrbox/lib/client.js` 的快照，用于前后对比。

重启后的人工验证：刷新 GUI 页面 → 左侧栏出现猫脸图标 → 点开主列面板 → 顶部状态条显示
`猫猫 · 情绪 … · 体力 … · 世界 08-16 …`；右栏只出现有变化的实体卡片（生效实体带「生效实体」标记），
底部是「另外 N 个实体暂无变化」；点「收起监控」/「监控全宽」版面切换并在刷新页面后保持。
若桥接未运行/路由不可达，右栏显示「黑板不可用」且面板其余部分正常。

## 验证

三段验证是**独立**的：组合通过 ≠ 加载通过 ≠ 真装通过。逐段留证据，不要跳段。

### 1) 组合验证（不碰真 profile、不耗模型调用）

```bash
bash tools/verifyProfile.sh --composition
```

它会在 `.local/profile-test` 重建隔离 `DSH_HOME`：只读复制源 profile 的
`package.json` / `cordis.patch.yml` / `cordis.yml` / `pnpm-workspace.yaml`，
`node_modules` 用只读 symlink，然后执行

```bash
DSH_HOME=$PWD/.local/profile-test node <dsh>/apps/cli/lib/bin.js \
  --profile web --patch $PWD/plugins/dsh-astrbox/cordis.patch.yml --dump-config
```

并断言：退出 0、`id: mcp-astrbox` 与 `id: astrbox-bridge` 各 1 行、`command` 指向的
文件可执行。期望输出形如：

```text
OK: composition checks passed (mcp-astrbox=1, astrbox-bridge=1, command=/path/to/Neko/bin/dsh-mcp, dump=.../dump.txt)
```

> `--dump-config` 会写 profile 的 `cordis.yml`，所以**必须**带隔离 `DSH_HOME`；
> 不带会因 `~/.dsh` 只读而 `EROFS` 退出 1。

### 2) 二进制健康（`make doctor`）

```bash
make doctor
```

直接 spawn `bin/dsh-mcp`（临时 `data-dir`/`scripts-dir`，不读写 `data/`），完成
`initialize`（`clientInfo.name=doctor`，`protocolVersion=2025-06-18`）与 `tools/list`，
断言 31 个冻结工具名（大小写/顺序无关）、socket 权限 `0600`、`SIGTERM` 后 socket 被清理。
期望输出：

```text
OK: doctor 通过（31 tools, socket 0600, SIGTERM 清理；data=/tmp/dsh-mcp-doctor-.../data）
```

### 3) 真装证据（用户回传）

真装后的会话/日志整理成一个文本文件，逐项包含：

| 字段 | 采集方式 |
|---|---|
| 两个 id 行 | `--dump-config` 输出里的 `id: mcp-astrbox`、`id: astrbox-bridge` |
| socket 0600 | `stat -c '%a %n' data/dsh-control.sock` → `600 .../dsh-control.sock` |
| 两类控制日志 | `data/logs/dsh-control-*.log` 中的 `POST /tick` 与 `POST /session-event` 行 |
| 17 工具名 | 会话里 `tools/list` 的 JSON（或用 `make doctor` 的输出） |
| 会话现状卡 | 真装会话中出现过的 `[astrbox-state]` 现状卡行 |

```bash
bash tools/verifyProfile.sh --evidence <file>
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
- 现状卡/推送由服务端 `observation.json` 策略决定（桥接不做过滤）；**R18 字段默认不登记为可推路径**。

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
| 启动告警 `mcp-astrbox (dsh-mcp-client): failed to import: Cannot find package 'dsh-mcp-client'` | `link:` 安装下裸包名解析不到（loader 在 profile 目录上解析）。现行 patch 用 bundle 内相对入口，出现这条说明是旧版 patch 或仓库内 `node_modules` 链接缺失：按「`link:` 安装与依赖解析」重做两条链接 |
| 工具消失 / `tools/list` 不满 31 个 | `make doctor` 先确认二进制；再确认 profile `package.json` 的 `dsh.profile.bundles` **仍然含** `dsh-astrbox`（GUI 插件管理页的移除/停用会改写这里）；最后看 DSH 启动日志里的 `mcp-astrbox` 错误 |
| 现状卡不出现 | 检查 `socketPath` 与 `dsh-mcp` 的 `-data-dir` 是否一致、`workspacePrefix` 是否覆盖当前会话 `cwd`、`observation.json` 是否把该路径登记为 critical/normal |
| 重复 tick / `tick_busy` | `/tick` 用 `TryRunTick`，MCP `state_machine_tick` 或另一个 tick 在飞时返回 `409`，插件按设计跳过本次；持续出现时把 `tickEveryMs` 调大或设 `0` 关闭定时 tick |
| `/session-event` 只有 `logged` | `functionName` 指向的函数未注册；用 `entity`/脚本工具确认函数存在，或改配置指向已注册函数 |

## 相关文件

- `Makefile`：`build` / `test` / `lint` / `install-runtime` / `doctor` / `clean`
- `tools/doctorDsh.mjs`：`make doctor` 的实现
- `tools/verifyProfile.sh`：`--composition` / `--evidence`
- `plugins/dsh-astrbox/cordis.patch.yml`：本 bundle 的插件行（不含 config / 机器路径）
- `plugins/dsh-astrbox/lib/mcpClientEntry.mjs`：`mcp-astrbox` 行的 bundle 内相对入口（转发 `dsh-mcp-client`）
- `plugins/dsh-mcp-client/`：`mcp-astrbox` 的包装组件（解析 `mcp-astrbox.json` / 默认启动参数）
- `plugins/dsh-astrbox/lib/`：插件运行时（`observationRender.mjs` v2 渲染（formatValue/renderDelta/renderState）、
  `observationBoard.mjs` 面板投影、`blackboardRoute.mjs` 面板黑板路由、
  `dialogFeed.mjs` 对话摘要 Feed、`relayTool.mjs` 跨会话转达工具、
  `sessionBoardText.mjs` / `sessionBoardSubscription.mjs` / `sessionBoardTool.mjs` 会话黑板（K30）、
  `sessionBoardRoute.mjs` 面板会话黑板路由（K15/K31）、
  `subscriptionInterval.mjs` 间隔覆盖存储、`subscriptionTool.mjs` 间隔工具、
  `controlRoutes.mjs` 控制面代理路由）；
  `docs/mcp-tools.md`：31 个 MCP 工具的参考
