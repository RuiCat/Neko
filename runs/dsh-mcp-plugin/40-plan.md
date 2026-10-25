# 任务规划（dsh-mcp-plugin）

- 创建: 2026-10-25
- 谱系: 见 `10-generalization.md`（候选 A：4 模块）
- 约束: 见 `30-constraints.md`（C1–C12 满足，C13–C16 裁决降级）
- 冻结状态: 已冻结（D3 后执行 `plan.sh freeze`）
- 本轮范围: 只到规划与契约冻结；**不进入 G4**（用户裁决）。实现按批 1（m1+m3）→ 批 2（m2）→ 批 3（m4）另起执行。

## 依赖图

子任务 DAG（单向、无环；同层可并行）：

```text
m1 共享装配与并发基座 ──→ m2 Go DSH 入口与控制面 ──┐
                                                   ├─→ m4 安装、验证与文档
m3 DSH bundle 与联动运行时 ────────────────────────┘
```

| 子任务 | 名称 | depends-on | 写域（独占） | 验收命令 |
|---|---|---|---|---|
| m1 | 共享装配与并发安全基座 | - | `internal/bootstrap`、`internal/mcpserver`、`internal/entity`、`cmd/astrbox-mcp` | `make test` + `go test -race ./internal/entity ./internal/mcpserver` |
| m2 | Go DSH 入口与控制面 | m1 | `cmd/dsh-mcp`、`internal/control` | `go test ./internal/control/... ./cmd/dsh-mcp/...` |
| m3 | DSH bundle 与联动运行时 | - | `dsh/lib`、`dsh/package.json`、`dsh/cordis.patch.yml.template` | `node --test dsh/lib/*Test.mjs` |
| m4 | 安装、验证与文档 | m2, m3 | `Makefile`、`scripts`、`docs`、`.gitignore`、`README.md`、`dsh/cordis.patch.yml` | `bash scripts/verifyProfile.sh --composition` + `make doctor` |

批次：**批 1 = m1 + m3**（写域互斥，可并行）；**批 2 = m2**（依赖 m1）；**批 3 = m4**（依赖 m2、m3）。

## 接口契约

### K1：控制面（m2 Go 服务 ↔ m3 DSH 插件）

- 方向: m3（DSH 插件）→ m2（Go 服务）；Go 服务不主动连插件。
- 载体: HTTP/1.1 over Unix domain socket，默认 `<data-dir>/dsh-control.sock`（默认 `/home/ruicat/Desktop/Neko/data/dsh-control.sock`），文件模式 `0600`；无 token（C13：同 uid 信任域）；DSH 侧必须用 `node:http` 的 `socketPath`（`fetch` 不支持 UDS）。
- 输入: `GET /health`；`GET /state?entity=<id>&path=<点分路径>（可重复 0..N）&maxBytes=<1024..1048576>`；`POST /tick {"scope":"<string，可空，空即 "*">"}`；`POST /session-event {"kind":"session-created"|"turn-end","sessionId":"<string>","cwd":"<abs>","turn":<int，turn-end 必填>,"function":"<string，可选，默认 dsh_on_event>"}`；请求体 ≤64 KiB；`Content-Type: application/json`。
- 输出: 成功 `200 {"ok":true,...}`：`/health` → `{version,pid,dataDir,scriptsDir,entityCount,uptimeMs}`；`/state`（指定 path）→ `{entity,values:{path:value},missing:[path],bytes}`，未指定 path → `{entity,data,bytes,truncated}`；`/tick` → `{tickId,status,transitions,conflicts,errors:[...]}`；`/session-event` → `{applied:"function"|"logged",error?:string}`。
- 错误语义: `400 bad_request`（JSON/参数/kind/function 非法）、`404 not_found`（实体不存在）、`409 tick_busy`（已有 tick 在飞，含 MCP tick；不排队）、`405 method_not_allowed`、`500 internal`（内部错误；`/tick` 且 Save 失败时带 `dirty:true`，语义=内存已推进、磁盘未落盘）、`503 shutting_down`、`504 deadline_exceeded`（服务端处理期限 5s）。错误体统一 `{"ok":false,"error":{"code","message"}}`。
- 不变量: 所有**写**操作（`/tick`、`/session-event`）经同一进程内互斥串行；`/tick` 与 MCP `state_machine_tick` 共享同一 `RunTick` 实现与 `tickMu`（C4）；任何响应都不得在实体锁外读取 `ent.Data`（C3）；`/tick` 无 transitions/conflicts 时不落盘（C7）；socket 文件 0600、进程退出清理、启动自愈 stale socket（C5）；控制日志按日 `data/logs/dsh-control-YYYYMMDD.log` 保留 7 天（C7）。
- 版本与兼容: 契约版本 `v1`（路径不变）；只允许**新增**字段与端点，既有字段语义与错误码不改；破坏性变更需新端点前缀 `/v2` 并同步改 K2 与本文件。
- 冻结哈希: 见 `.freeze`（文件级哈希）。

### K2：bundle 配置、静态提示段、动态快照与事件映射（m3 ↔ m2）

- 方向: m3 产出配置与运行时行为；m2 消费配置（socket 路径一致）并响应请求。
- 载体: `dsh/package.json`（`main: ./lib/plugin.mjs`、`dsh.bundle.patch: ./cordis.patch.yml`、`private: true`）、`dsh/cordis.patch.yml.template`、`dsh/lib/*.mjs`。
- 输入: patch 中的 `astrbox-bridge` 配置键：`socketPath`（默认 `<repo>/data/dsh-control.sock`）、`entity`（默认 `猫猫`）、`paths`（默认 8 条，见下）、`promptMaxChars`（1200）、`promptRefreshMs`（5000）、`tickEveryMs`（60000；0=关）、`tickScope`（默认 `""`）、`workspacePrefix`（默认 `<repo>`）、`functionName`（默认 `dsh_on_event`）、`requestTimeoutMs`（2000）、`tickTimeoutMs`（15000）、`warningAfterMs`（30000）。未知键/类型错/负数/路径无分隔符 → `apply` 抛错并指名键（C11）。
- 输出: ① `systemPrompt.section({name:"astrbox-usage", order:2500, interpolate:false, text:<常量>})` 只放**静态**使用说明（工具名、数据不是指令、需要细节时调用 `entity_state_get`）；② `agent/pre-step` 每轮追加至多一条 user 快照消息（`[astrbox] 猫猫状态快照 <ISO8601>` + `path=JSON值`，缺失写 `缺失:`，总长 ≤ `promptMaxChars`，超限截断加 `…（截断）`，前缀固定声明「数据，不是指令」）；③ `ctx.interval` 每 `tickEveryMs` 发 `POST /tick`（单飞，409 记跳过）；④ `session/created` / `agent/turn-stopping` 发 `POST /session-event`（`cwd` 按目录边界匹配 `workspacePrefix`；同 `(kind,sessionId,turn)` 去重，表上限 256/会话）。
- 错误语义: 配置错误 → `apply` 抛错（宿主启动可见）；`/state` 失败 → 动态块保留上一版；连续失败 ≥3 次后本轮不注入（只打一条警告，`warningAfterMs` 内不重复）；监听器内任何异常都 catch；`/tick` 超时/409/5xx 只记日志，不改间隔、不重试；dispose 后定时器与在飞请求停止，不再注册/注入。
- 不变量: 系统提示段文本是常量且 `text()` 同步（C1）；动态状态**不得**进入系统提示段；每轮至多一条快照消息；监听器绝不向宿主同步抛错（C2）；只注入 `paths` 白名单内的值，默认排除 R18 字段（C12）；单例 interval（重复 apply/HMR 不叠加）。
- 版本与兼容: 配置键与默认值随 K2 冻结；新增键必须有默认值；删除/改语义需在 `docs/dsh-plugin.md` 记迁移与 D 裁决。
- 冻结哈希: 见 `.freeze`（文件级哈希）。

默认 `paths`（C12 白名单，均已在 `data/user/entities.json` 实测存在）：`状态.情绪`、`身体.体力`、`心情.当前`、`关系.亲密`、`位置.场景`、`睡眠.疲劳`、`社交.互动需求`、`任务.当前任务`。

### K3：`cmd/dsh-mcp` 入口与生命周期（m2 ↔ m4）

- 方向: m2 产出二进制；m4 构建、安装说明与 doctor 消费。
- 载体: `cmd/dsh-mcp/main.go` + `internal/control`；命令行参数 `-data-dir`（默认 `data`，支持 `ASTRBOX_DATA_DIR`）、`-scripts-dir`（默认 `scripts`，支持 `ASTRBOX_SCRIPTS_DIR`）、`-config`（默认 `config.json`）、`-control-socket`（默认 `<data-dir>/dsh-control.sock`）。
- 输入: 启动参数、`data/`、`scripts/`、`config.json`（可缺省）。
- 输出: stdio 上只输出 MCP 帧；日志与诊断只走 stderr 与 `data/logs/dsh-control-*.log`；退出码：`0` 正常，`2` 配置/数据加载失败，`3` 单实例锁被占用（stderr 含持有者 pid 与 `rm` 提示），`4` socket 无法建立/自愈失败。
- 错误语义: 数据目录不可写、脚本重放失败、MCP stdio 结束 → 记录原因并以非 0 退出（0 仅在正常 EOF/信号清理后）；任何路径都不得把日志写到 stdout。
- 不变量: 一个 `data/` 同时只允许一个进程（flock `data/.engine.lock`，C5）；control listener 在 MCP stdio 开始前就绪（插件可轮询）；SIGTERM/SIGINT/defer 清理 socket；启动时若 socket 已存在：connect 成功 → 退出码 3 并提示；`ECONNREFUSED` → unlink 后重试一次。
- 版本与兼容: 二进制名与参数为对外契约；新增参数必须带默认值；Unix socket 仅 Linux/macOS，Windows 不支持并在文档写明。
- 冻结哈希: 见 `.freeze`（文件级哈希）。

### K4：验证台账与证据格式（m4 → 父体/用户）

- 方向: m4 产出脚本；父体执行隔离验证，用户执行真装并回传证据文件。
- 载体: `scripts/verifyProfile.sh`（`--composition` 与 `--evidence <file>` 两种模式）、`make doctor`、`docs/dsh-plugin.md`。
- 输入: `--composition`：仓库路径、隔离 `DSH_HOME`（`.local/profile-test`）、生成的 `dsh/cordis.patch.yml`；`--evidence`：用户回传文本文件（最小字段集见下）。
- 输出: 退出码 0/非 0；失败时逐条打印缺失字段；成功时打印 `OK: <n> checks`。证据文件最小字段集：① `--dump-config` 中 `mcp-astrbox` 与 `astrbox-bridge` 各 1 行；② `stat -c %a` socket = 600；③ `data/logs/dsh-control-*.log` 中 `POST /tick` 与 `POST /session-event` 各 ≥1 行；④ `tools/list` 的 17 名 JSON；⑤ 真装会话请求里 `[astrbox]` 快照 ≥1 条。
- 错误语义: 缺任一字段 → 非 0 并打印 `MISSING: <field>`；证据文件不存在/不可读 → 非 0；`--composition` 不访问真 profile、不消耗模型调用。
- 不变量: 隔离副本只写 `.local/`；真 profile 的改动只由用户执行；「组合通过」不得被当作「加载通过」或「真装通过」（三段分开判定）。
- 版本与兼容: 证据字段只增不减；字段含义变更需同步改本文档与 `verifyProfile.sh`。
- 冻结哈希: 见 `.freeze`（文件级哈希）。

### K5：共享引擎接口（m1 → m2）

- 方向: m1 在既有包内提供稳定方法；m2 调用，不得另写一份 tick 体或直接读 `ent.Data`。
- 载体: `internal/entity` 的 `func (e *Entity) Snapshot() map[string]any`（`RLock` + 深拷贝）；`internal/mcpserver` 的 `func (s *Server) RunTick(scope string, options TickOptions) (engine.TickResult, error)`、`func (s *Server) TryRunTick(scope string, options TickOptions) (engine.TickResult, bool, error)`（`TryLock`，拿不到锁立即返回 `false`，不排队）、`TickOptions{SkipSaveWhenUnchanged bool}`；`func (s *Server) WithStateWrite(run func() error) error`（导出写闸，供 m2 的 `/session-event` 与 `RunTick`/`entity_state_set` 共用同一把锁）；MCP 工具 `state_machine_tick` 与控制面共用它们。
- 输入: `scope` 字符串（空按 `"*"` 处理，与现有工具一致）；`SkipSaveWhenUnchanged`。
- 输出: `engine.TickResult`（既有 JSON 字段不变）+ error；副作用：`recordTick`、状态日志、按 options 决定 `Store.Save()`。
- 错误语义: 参数/scope 非法 → 返回 `bad_request` 语义错误但 `TickResult.Status` 仍可用；Tick 内部错误 → `error`，控制面映射 500；Save 失败 → `error` + `dirty=true`。
- 不变量: 同一时刻只有一个 `RunTick`/`TryRunTick`/`WithStateWrite` 临界区在飞（`tickMu`）；控制面 `/tick` 用 `TryRunTick`，拿不到锁立即 409，不排队；`Store.saveLocked` 与所有实体读取都必须经 `Snapshot()`，不得在实体锁外暴露 `ent.Data`（C3）；`state_machine_tick` 的既有行为（总是 Save）保持不变；`WithStateWrite` 不得重入 `RunTick`/`TryRunTick`/`WithStateWrite`（回调内只做函数调用与保存）。
- 版本与兼容: 内部包接口，不承诺外部兼容；改名需同步 m2/m1 测试；`TickResult` JSON 字段是 MCP 契约，不改。D4/D6 扩展只新增方法，不改变既有方法语义。
- 冻结哈希: 见 `.freeze`（文件级哈希）。

### K6：17 个 MCP 工具名（m1/m2 ↔ 所有）

- 方向: m1 的 `mcpserver` 注册、m2 的 dsh-mcp 暴露、m3 的静态说明文本三方一致。
- 载体: MCP `tools/list`；冻结清单（按字母排序）：`diagnostics_query, entity_create, entity_query, entity_snapshot, entity_state_get, entity_state_set, event_log, inventory, neko_script_apply, neko_script_list, neko_script_read, neko_script_remove, neko_script_validate, neko_script_write, state_machine_resolve, state_machine_tick, stats`（17 项）。
- 输入: 无（服务端注册表）。
- 输出: `tools/list` 的名称集合；每个名称的 `inputSchema` 由现有 `mcp.AddTool` 决定，本次不改。
- 错误语义: 任一缺失/多出 → e2e 失败（非 0）；新增工具必须同时更新本清单与 `docs/mcp-tools.md`，否则 V2/V7 判失败。
- 不变量: dsh-mcp 与 astrbox-mcp 的工具集合完全相等；工具名前缀由 mcp-client 加 `mcp__astrbox__`，注册名不含前缀。
- 版本与兼容: 工具名是模型可见契约；只增不改；改名视为破坏性变更。
- 冻结哈希: 见 `.freeze`（文件级哈希）。

## 写域互斥表

| 子任务 | 独占写的路径 | 只读的路径 |
|---|---|---|
| m1 | `internal/bootstrap`、`internal/mcpserver`、`internal/entity`、`cmd/astrbox-mcp` | `internal/engine`、`internal/events`、`internal/function`、`internal/registry`、`internal/diagnostics`、`internal/config` |
| m2 | `cmd/dsh-mcp`、`internal/control` | `internal/bootstrap`、`internal/mcpserver`、`internal/entity`、`internal/engine` |
| m3 | `dsh/lib`、`dsh/package.json`、`dsh/cordis.patch.yml.template` | `dsh/cordis.patch.yml`（生成物，只读）、DSH checkout（只读） |
| m4 | `Makefile`、`scripts`、`docs`、`.gitignore`、`README.md`、`dsh/cordis.patch.yml` | `dsh/lib`、`dsh/package.json`、`dsh/cordis.patch.yml.template`、`cmd`、`internal` |

## 共享资源裁决

| 路径 | 处置（独占 / 复制 / 契约化 / 串行化） | 说明 |
|---|---|---|
| `dsh/cordis.patch.yml` | 串行化 | 由 m4 的 `make render-dsh` 从 m3 的模板生成；m3 只写模板，不写生成物；m4 在 m3 完成后执行。 |
| `data/` | 运行期独占 | 不属于任何子任务写域；由运行中的 dsh-mcp 以 flock 独占；测试一律用 `t.TempDir()`。 |
| `go.mod` / `go.sum` | 独占（m1） | 本次不新增 Go 依赖；若确需新增，由 m1 单方修改并记录。 |

## 模块映射

| 子任务 | 实现载体（代码模块 / 子智能体 / 工作流） | 并行批次 | 预估成本 |
|---|---|---|---|
| m1 | 子智能体（Go 共享基座） | 1 | 中（2–3 文件 + 测试） |
| m2 | 子智能体（Go 控制面/入口） | 2 | 中高（新包 + e2e） |
| m3 | 子智能体（零构建 .mjs） | 1 | 中高（插件 + 单测） |
| m4 | 子智能体（脚本/文档/Makefile） | 3 | 中（脚本 + 文档） |

## 约束归属

| 约束 | 状态 | 说明 / 证据 |
|---|---|---|
| C1 | 满足 | K2：静态系统提示段常量 + `agent/pre-step` 动态快照；单测门见 `60-verification.md`（G4 回填） |
| C2 | 满足 | K2：监听器 try/catch、`cwd` 类型与目录边界、去重表 256；G4 单测 |
| C3 | 满足 | K5 + m1：`Entity.Snapshot()`、`saveLocked` 与 `entity_state_get` 改用快照、`-race` 用例 |
| C4 | 满足 | K5：`RunTick` + `tickMu` 覆盖两个入口；m1 写域包含 `internal/mcpserver` |
| C5 | 满足 | K3：flock + stale socket 自愈 + 退出清理；G4 双实例/`kill -9` 测试 |
| C6 | 满足 | K1/K2：服务端 5s、tick 15s、state 2s、`500 dirty:true` |
| C7 | 满足 | K1：无变化 tick 跳过 Save；控制日志按日 + 7 天 |
| C8 | 满足 | K2/K3：`cordis.patch.yml.template` 渲染、id 固定、生成物忽略 |
| C9 | 满足 | K4：隔离 `DSH_HOME` + `verifyProfile.sh` + 三段验证分离；本会话已实测隔离 dump 退出 0 |
| C10 | 满足 | K6：17 名冻结清单；V2/V7 集合比较 |
| C11 | 满足 | K2：手写配置校验，错误指名键 |
| C12 | 满足 | K2：白名单 8 路径、默认非 R18、JSON 编码、1200 字符上限、固定前缀 |
| C13 | 降级 | 控制面无 token（同 uid 信任域 + socket 0600）；理由/残留/复核见 `30-constraints.md` |
| C14 | 降级 | `data/` 不强制 0700；socket 0600 足够 |
| C15 | 降级 | 不引入 schemastery；用 import+apply 单测做加载面 |
| C16 | 降级 | `failOnStartupError=false`；`make doctor` 可观测 |

## 裁决记录

| 编号 | 门 | 决定 | 用户裁决 |
|---|---|---|---|
| D1 | G1 谱系 | 采用候选 A（4 模块：m1 装配复用 / m2 Go 入口+控制面 / m3 DSH bundle+联动 / m4 安装验证文档）；bundle 落点 Neko `dsh/`；允许提取共享装配；插件用零构建 `.mjs` | 接受 |
| D2 | G2 约束 | C1–C16 全部接受：C1–C12 满足，C13–C16 降级（无 token / 目录 0700 放开 / 不用 schemastery / `failOnStartupError=false` + doctor）；注入接缝=静态系统提示段 + 动态 pre-step 消息；Tick 默认 60s | 接受 |
| D3 | G3 任务图 | 4 模块、3 批次（m1+m3 → m2 → m4）、写域与 K1–K6 契约如本文件；本轮**停在规划**，不进入 G4 | 接受 |
| D4 | G3 契约扩展 | 实现期发现 K5 缺导出写闸：新增 `Server.WithStateWrite(func() error) error`，让 m2 的 `/session-event` 与 `RunTick`/`entity_state_set` 共用 `tickMu`；其余契约不变。走推荐（用户已授权实现，Lead 代行） | 接受（事后追认） |
| D5 | G3 口径修正 | C6 的 `/state` 客户端 2s 是快速失败（读不取写锁），只有 `/tick` 要求 15s ≥ 服务端 5s；C16 告警按 `warningAfterMs` 限频即可；测试文件改 camelCase `*Test.mjs`（`node --test` 显式参数），同步所有验收命令。走推荐（Lead 代行） | 接受（事后追认） |
| D6 | G3 契约扩展 | 实现期两处缺口：① `/tick` 在 MCP tick 占锁时阻塞到 504，违反 K1「已有 tick → 409」；新增 `Server.TryRunTick`（TryLock，不排队）并让 m2 改走它。② K2 的 `functionName` 配置未生效；K1 的 `/session-event` 新增可选 `function` 字段，服务端缺省 `dsh_on_event`。走推荐（Lead 代行） | 接受（事后追认） |
| D7 | G3 复核勘误 | 独立复核 F1–F6：修正 C4/C11/C16 的判定文本、60 证据计数与 40 冻结哈希文本；补强 `TestTickBusyImmediate409`（1s 安全释放，排队回归时失败而非挂死）。不新增/修改运行接口。走推荐（Lead 代行） | 接受（事后追认） |

> 规则：三道门各至少一条；`用户裁决` 只能是 接受 / 修改 / 暂缓；默认走推荐的行必须在「决定」里写明「走推荐，事后追认」；出现「暂缓」则「未决项」不得为空。

## 未决项

| # | 未决 / 暂缓项 | 为何暂缓 | 复核触发条件 |
|---|---|---|---|
| P2 | K2 的 `/state` 未指定 path 时截断语义（H2） | 当前实体 47 KB，未触及 64 KiB 上限，构造不出真实反例 | 实体超过 64 KiB，或插件改用整体快照 |
| P3 | 两条 insert 的挂载顺序（H4） | `cordis-plugin-include` 不在本 checkout，无法读源码 | m4 的隔离 dump / 真启动观察 bridge 首批 `/state` 是否 ENOENT |
| P4 | `dsh plugin add link:` 在半离线环境可成功（H5） | 需要用户宿主环境与 pnpm store，本会话禁止联网/写 profile | 用户真装时回传 pnpm 退出码与 stderr |
| P5 | 隔离副本能否承载 bundle 的 link 解析（H6） | 当前 `.local/profile-test` 的 node_modules 是只读 symlink；K4 用 `--patch` 直接注入生成 patch，绕开 link 解析 | m4 交付 `verifyProfile.sh` 后 `grep -c 'astrbox-bridge' .local/profile-test/dump.txt` ≥1 |
| P6 | `ctx.interval` 回调 reject 时宿主行为（H7） | 只有同步签名，异步 reject 处置需实测 | m3 交付后注入一次 socket 失败观察宿主日志 |
| P7 | `internal/bootstrap` 是否会膨胀为装配桶（H3） | 取决于实现 | m1 交付后跑 `lintCodeModularity.sh internal cmd --naming --comments` |

## 变更记录（冻结后任何契约改动都要走新裁决并重新 freeze）

| 日期 | 改动 | 裁决编号 | 重新冻结 |
|---|---|---|---|
| 2026-10-25 | 独立复核 F1–F6 勘误：C4/C11/C16 判据文本、60 计数、40 冻结哈希文本；409 测试加安全释放 | D7 | 已 freeze（见 `.freeze`） |
| 2026-10-25 | D6 扩展：K5 加 `TryRunTick`（/tick 立即 409）；K1 的 `/session-event` 加可选 `function` 字段让 `functionName` 生效 | D6 | 已 freeze（见 `.freeze`） |
| 2026-10-25 | D5 口径修正：C6 `/state` 2s 快速失败、C16 告警限频、测试文件 `*Test.mjs` + 验收命令同步 | D5 | 已 freeze（见 `.freeze`） |
| 2026-10-25 | 实现期 K5 扩展：新增 `Server.WithStateWrite(func() error) error`，m2 的 `/session-event` 与 `RunTick`/`entity_state_set` 共用 `tickMu` | D4 | 已 freeze（见 `.freeze`） |
| 2026-10-25 | 红队 A1-1/A5-2 后，动态状态从系统提示段改为 `agent/pre-step` 快照消息；系统提示段只留常量 | D2 | 已 freeze（见 `.freeze`） |
| 2026-10-25 | 新增单实例 flock、stale socket 自愈、无变化 tick 跳过落盘、控制日志轮转 | D2 | 已 freeze（见 `.freeze`） |
| 2026-10-25 | 验证改为隔离 `DSH_HOME` + 用户真装两段；真装证据字段化 | D2 | 已 freeze（见 `.freeze`） |
