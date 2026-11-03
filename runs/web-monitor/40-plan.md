# 任务规划（web-monitor）

- 创建: 2026-10-26
- 谱系: 见 `10-generalization.md`（候选 B：7 模块）
- 约束: 见 `30-constraints.md`（C1–C24 全部接受，状态=满足）
- 冻结状态: 未冻结（D3 裁决后执行 `plan.sh freeze`）
- 本轮范围: 规划与契约冻结后，用户已裁决**进入 G4**；按批 1（m1+m2）→ 批 2（m3+m4）→ 批 3（m5）→ 批 4（m6）→ 批 5（m7）派发。

## 依赖图

子任务 DAG（单向、无环；同层可并行）：

```text
m1 契约冻结 ───────────────→ m3 后端实现 ───────────────┐
                                                         ├─→ m7 门禁/文档/验收
m2 令牌层 ──→ m4 元件契约 ──→ m5 元件实现 ──→ m6 组合装配 ┘
```

| 子任务 | 名称 | depends-on | 写域（独占） | 验收命令 |
|---|---|---|---|---|
| m1 | 契约冻结与规划 | - | `runs/web-monitor/40-plan.md`、`runs/web-monitor/50-subtasks/` | `bash ~/.dsh/skills/task-planning/scripts/plan.sh validate --slug web-monitor` |
| m2 | 设计令牌层 | - | `design/tokens.json`、`design/dist` | `bash ~/.dsh/skills/ui-composition/scripts/lint-ui-plan.sh --plan-dir design --tokens design/tokens.json` |
| m3 | 后端监视服务 | m1 | `internal/monitor/server.go`、`internal/monitor/api`、`internal/monitor/ws`、`internal/monitor/auth`、`cmd/dsh-mcp/main.go`、`internal/control/state.go`、`internal/control/state_test.go`、`internal/mcpserver/runTick.go`、`internal/mcpserver/server.go`、`internal/diagnostics/store.go`、`internal/bootstrap/bootstrap.go`、`go.mod`、`go.sum` | `go test ./internal/monitor/... ./internal/control/... ./internal/mcpserver/... && go test -race ./internal/monitor/...` |
| m4 | 元件契约 | m2 | `design/components.json`、`design/composition.json` | `python3 -c "import json;a=json.load(open('design/components.json'));b=json.load(open('design/composition.json'));assert a and b"` |
| m5 | 元件实现 | m4 | `internal/monitor/web/components` | `bash ~/.dsh/skills/ui-composition/scripts/lint-ui-plan.sh --plan-dir design --tokens design/tokens.json --src-root internal/monitor/web` |
| m6 | 组合装配 | m4, m5 | `internal/monitor/web/index.html`、`internal/monitor/web/app.js`、`internal/monitor/web/app.css`、`internal/monitor/web/wsClient.js` | `curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8765/monitor/` = 200 + `lint-ui-plan` 退出 0 |
| m7 | 门禁/文档/验收 | m3, m6 | `design/fixtures`、`Makefile`、`docs/web-monitor.md`、`runs/web-monitor/60-verification.md` | `make test && make lint && make doctor` + 反例门禁 |

批次：**批 1 = m1 + m2**（写域互斥可并行）；**批 2 = m3 + m4**（m4 依赖 m2；m3 依赖 m1）；**批 3 = m5**；**批 4 = m6**；**批 5 = m7**。
并发上限 4：批 1/批 2 各 2 个执行体，批 3–5 各 1 个。

## 接口契约

### K1：监视 REST 数据面（m1 → m3 实现 / m5 消费）

- 方向: 浏览器 → `bin/dsh-mcp` 进程内监视服务（同进程，共享 `bootstrap.Runtime` 与 `mcpserver` 的 tick/写 gate）；不跨进程。
- 载体: HTTP/1.1，`-addr`（默认 `127.0.0.1:8765`），前缀 `/monitor/api/*`；JSON；静态页面在 `/monitor/`（见 K6）。
- 输入: `GET /monitor/api/health`；`GET /monitor/api/entities?maxBytes=`；`GET /monitor/api/entities/{id}?path=&maxBytes=`（`path` 可重复）；
  `GET /monitor/api/diagnostics?severity=&source=&entity=&script=&code=&limit=`；`GET /monitor/api/scripts`；
  `GET /monitor/api/scripts/{name}?source=`；`GET /monitor/api/logs?kind=state|control&date=YYYY-MM-DD&limit=`；
  `GET /monitor/api/tick/latest`；`POST /monitor/api/tick {"scope"}`；`POST /monitor/api/entities/{id}/set {"path","value"}`。
- 输出: 成功 `200 {"ok":true,...}`；`health` 含 `version/pid/dataDir/scriptsDir/uptimeMs/engine{entities,templates,rules,scripts}/diagnostics{...}/monitor{readOnly}`；
  `entities` 含 `items[{id,type,keys}]/bytes/truncated`；`entities/{id}` 无 path 含 `data/bytes/truncated`、有 path 含 `values/missing/bytes`；
  `diagnostics` 含 `generation/summary/items`；`scripts` 含 `items[{name,path,hash,rules,templates,funcs,events}]`；
  `scripts/{name}` 含 `hash/actualHash/rules/templates/funcs/events/source?`；`logs` 含 `kind/date/lines[{ts,raw,parsed?}]/truncated`；
  `tick` 与 `tick/latest` 用同一构造函数（与 control `/tick` 字段集合相等，含 `tick_id` 与 `tickId`）；`set` 含 `entity/path/value/saved`。
- 错误语义: `400 bad_request`（JSON/参数/`maxBytes` 越界/date 非法/空 path）；`401 unauthorized`（缺/错 token）；`403 forbidden`（Host/Origin/写禁用）；
  `404 not_found`（实体/脚本不存在）；`409 tick_busy`（tick 在飞）或 `409 write_busy`（set 超过 2s gate 等待）；`413 too_large`（脚本源/日志响应超限）；
  `500 internal`（Save 失败：**回滚内存并返回 500**，或返回 `ok:false` + `dirty:true`，二选一在实现中固定并在 `60-verification` 声明）；
  `504 deadline_exceeded`（仅当实现引入请求级 deadline）。错误体统一 `{"ok":false,"error":{"code","message"}}`。
- 不变量: 所有读路径经 `Entity.Snapshot()`/`Entity.Get()`（`path` 语义与 unified `/state` 一致，见 K5）；写路径与 MCP tick 共享 `mcpserver` 的 `tickMu`；
  不得在实体锁外读 `ent.Data`；`maxBytes` 截断按完整单元进行且响应 JSON 恒可解析（`bytes` 口径写死为 payload 序列化字节数，不含 envelope）；
  日志只读 `data/logs/` 白名单命名（`Lstat` 拒 symlink、尾部有界、只返回完整行、响应 ≤1 MiB）；脚本源只走 `registry.ReadScript`；
  `/monitor/api/*` 全部继承 K3 的鉴权与 Host/Origin 校验。
- 版本与兼容: 契约版本 `v1`；只允许新增字段与端点，既有字段语义不改；破坏性变更走 `/monitor/api/v2` 前缀并在本文档记新裁决。
- 冻结哈希: 见 `.freeze`（文件级哈希）。

### K2：WebSocket 实时通道（m1 → m3 实现 / m5 消费）

- 方向: `bin/dsh-mcp` → 浏览器（推送）；浏览器 → 服务（订阅/心跳）。
- 载体: `GET /monitor/ws`，RFC6455，依赖 `github.com/coder/websocket`（版本锁进 `go.mod`/`go.sum`，进 `GOPROXY=off` 构建）；
  握手 token 走 `Sec-WebSocket-Protocol: astrbox.token.<token>`（不把 token 放 query）。
- 输入: 连接参数 `topics=state,diagnostics,logs,tick`（可空=全部只读）、`entity`、`paths`；客户端帧 `subscribe{topics,entity,paths}`、`unsubscribe{topics}`、`ping{}`。
- 输出: 服务端帧 `hello{version,readOnly,serverTime}` → 每个订阅 topic 一个 `snapshot`；之后 `delta{entity,path,oldValue,newValue}`、
  `tick{result}`（K1 同一载荷）、`diagnostics{op:"added"|"updated"|"removed",record|id}`、`log{kind,line}`、`pong`、`error{code,message}`。
- 错误语义: 握手阶段 token 缺失/错误 → HTTP 401；Origin/Host 不合法 → 403；协议错误 → `error` 帧 + close code 1008；慢消费者 → 丢 `delta` 并随后发一次 `snapshot`（替代丢失增量）；服务端内部错误 → close 1011。
- 不变量: **每次连接（含重连）在 `hello` 后必发 `snapshot`**（不实现 `resumeSeq`，方案②）；每连接 `seq` 单调，`snapshot` 覆盖到 `seq=S` 后不再发 `seq≤S` 的帧；
  `delta` 只在 `snapshot` 之后发；订阅/重连不得新增实体监听器（监听器全进程只注册一次，见 K5）；tick 观察者非阻塞，慢观察者不得拉长 `tickMu`；
  诊断帧必须能区分新增/更新/清除；WS 不复用 `tokenMiddleware`（在它之前分流）。
- 版本与兼容: 帧 `type` 只增不改；新增帧类型需同步 K2 与前端 `wsClient.js`；`resumeSeq` 在 v1 明确不存在（不留半实现）。
- 冻结哈希: 见 `.freeze`（文件级哈希）。

### K3：鉴权、跨源与写安全（m1 → m3）

- 方向: 浏览器 → 服务；所有 `/monitor/*` 请求先过 Host 校验，再过鉴权，写操作再过 Origin 与审计。
- 载体: 复用并扩展 `cmd/astrbox-mcp/httpAuth.go` 的 token 取法（`X-Auth-Token` / `Authorization: Bearer`）；新增 `internal/monitor/auth`；
  WS 用子协议 token；`internal/config` 的 `http_token` 启动时校验字符集 `^[A-Za-z0-9_-]{16,128}$`（不合法拒绝启动）。
- 输入: token（header / WS 子协议）、`Host`、`Origin`、写请求体。
- 输出: 通过 → 进入处理器；拒绝 → 401/403；`health.monitor.readOnly` 反映当前形态；每次写追加审计行（时间/动作/实体/path/结果/来源 IP，不含 token）。
- 错误语义: 缺/错 token → `401 unauthorized`；Host 不在 `127.0.0.1[:port]`/`localhost[:port]`/`[::1][:port]` 白名单 → `403 forbidden`；
  WS `Origin` 非精确同源（或 `null`）→ 403；无 token 配置形态下写请求 → `403 write_disabled`；审计写失败只记 stderr，不影响业务响应。
- 不变量: `/monitor/` 与静态资源免 token（否则页面打不开）；`/monitor/api/*` 在配置了 token 时必须带 token；写操作**任何形态**都必须 token（回环无 token 形态只读）；
  `Origin == Host` 自比自不算校验，必须 Host 白名单 + Origin 精确匹配；`/monitor/*` 响应不得带 `Access-Control-Allow-Origin`；
  审计按日 `data/logs/monitor-audit-YYYYMMDD.log` 保留 7 天、单行有上限、不泄 token、写失败不挡请求。
- 版本与兼容: token 载体与错误码为对外契约；新增鉴权方式不得削弱既有拒绝语义；`http_token` 字符集约束是启动期硬校验。
- 冻结哈希: 见 `.freeze`（文件级哈希）。

### K4：前端设计真源、元件契约与门禁（m2/m4 → m5/m6/m7）

- 方向: `design/` 三件套（令牌 → 元件 → 组合）→ 生成 CSS 与元件实现 → `internal/monitor/web/`；门禁只读校验。
- 载体: `design/tokens.json`（唯一真源）+ `design/dist/tokens.css`（生成物）；`design/components.json` + `design/composition.json`；
  `internal/monitor/web/components/**`（元件实现）、`internal/monitor/web/index.html` + `app.js` + `app.css` + `wsClient.js`（组合层）；
  `design/fixtures/fail/**`（4 类反例）。
- 输入: 需求节点（状态树/诊断/脚本/日志/tick/写表单/连接状态）、令牌集、元件契约。
- 输出: 自描述令牌源；分解表（界面节点 → 元件唯一归属）；DAG 组合树（支持 repeat/conditional/slot）；每元件 `verify`；可重建的页面资源。
- 错误语义: 缺令牌/孤儿令牌/ref 环 → lint 退出 1；元件 `style` 填非语义令牌或原始色阶 → 退出 1；组合树引用未登记元件/有环 → 退出 1；
  组合层写原生标签或覆盖样式 → 退出 1；schema/IO 错 → 退出 2；反例未变红 → 退出 2。
- 不变量: 任何视觉值有且只有一个令牌名；元件只引用语义令牌，原始色阶只属令牌层；组合层只装配、不造原子、不覆盖元件内部样式；
  `design/dist/tokens.css` 与 `internal/monitor/web/**` 生成物**入库**且可复现（`make design` 后 `git diff --exit-code` 为空）；
  门禁命令必须含 `--tokens design/tokens.json --src-root internal/monitor/web`（默认布局不会扫真实源码）。
- 版本与兼容: 令牌/元件/组合 schema 由 `ui-composition` 冻结；新增令牌或元件只增不改，删除需迁移说明；`lint-ui-plan` 升级改变默认布局时重审本契约。
- 冻结哈希: 见 `.freeze`（文件级哈希）。

### K5：引擎观察钩子、锁与 `/state` 统一（m1 → m3；被观察包定义类型）

- 方向: 被观察包（`internal/mcpserver`、`internal/diagnostics`、`internal/entity`）**定义**钩子类型并提供 setter；`internal/monitor` 赋值；`cmd/dsh-mcp` 接线；被观察包不得 import monitor。
- 载体: `mcpserver.Server.SetTickObserver(func(engine.TickResult))`（在 `runTickLocked` 产生结果后、`tickMu` 外投递）；
  `diagnostics.Store.SetNotify(func(diagnostics.Record, string /*op*/))`（锁外回调）；`entity.Store.OnCreate` 的链式包装（保留 `events.System` 的既有赋值）；
  `mcpserver.TryRunTick` / `mcpserver.WithStateWrite`；`internal/control/state.go` 的 path 查询改走 `Entity.Get`（与 MCP `entity_state_get` 一致）。
- 输入: `engine.TickResult`、`diagnostics.Record` + 操作类型、新 `*entity.Entity`、`/state` 的 path 列表。
- 输出: 非阻塞投递给 monitor 的事件队列（有界，满则丢并标记）；`/state` 与 `/monitor/api/entities/{id}` 对同一 path 返回同值/同 missing。
- 错误语义: 观察者 panic 被 recover 并记 stderr，不影响 tick/写；观察者阻塞超过阈值（建议 100ms）只记警告，不改变 tick 结果；
  `OnCreate` 回调若调用 Store 加锁方法 → 测试必须超时失败（证明约束有牙）；诊断 notify 锁内回调 → 测试超时失败。
- 不变量: 被观察包对 monitor 零 import（`go list` 可判）；`OnCreate` 回调只读传入实体；tick 观察者不在 `tickMu` 内做重活/重入；
  诊断通知在释放 `Store.mu` 之后；`/state` 的 path 与 MCP 同源（`Entity.Get`，含模板回退）；监视器不改变 MCP 工具集合与控制面既有端点语义（`/health` `/tick` `/session-event` 不变；
  `/state` 的 path 回退变化由 D3 显式记录）。
- 版本与兼容: 内部包接口，不承诺外部兼容；setter 只增不改；`/state` 行为变化必须在 `docs/` 与 `60-verification` 记录，客户端影响面 = 模板回退字段由 missing 变有值。
- 冻结哈希: 见 `.freeze`（文件级哈希）。

### K6：构建、内嵌与交付（m3 → m7）

- 方向: Go 源码 + `design/` + `internal/monitor/web/` → `bin/dsh-mcp` 单二进制 + Makefile 目标 + 文档。
- 载体: `Makefile`（`build`/`test`/`lint`/`doctor`/`design`/`monitor`）；`internal/monitor/server.go` 的 `//go:embed web`；`-ldflags "-X main.version=<git short HEAD>"` 注入；
  `docs/web-monitor.md`；`scripts/doctorDsh.mjs` 保持 17 工具断言。
- 输入: 仓库源、`design/` 三件套、web 资源、`git rev-parse --short HEAD`。
- 输出: `bin/dsh-mcp` 内含页面与静态资源；`make design` 重建 `design/dist/tokens.css` 与 web 产物；`make monitor` 以 `-transport http -addr 127.0.0.1:8765` 起服务；
  `make test && make lint && make doctor` 全绿；`/monitor/api/health` 的 `version` = git short HEAD。
- 错误语义: 缺 web 生成物 → 编译失败（由「生成物入库」规避，不允许依赖本地临时生成）；`make design` 产生 diff → 非 0 并提示生成物未入库；
  `make lint` 的 modularity 脚本路径不可硬依赖 `$HOME`（用 `MODULARITY_LINT ?= $(HOME)/.dsh/...` 可覆盖）；`make doctor` 工具数不符 → 非 0。
- 不变量: 单二进制、零 node 构建链；`make build` 把 `design/` 与 `internal/monitor/web/**` 列为前置依赖（改动触发重建）；
  `GOPROXY=off make build` 通过（`coder/websocket` 已锁版本并进模块缓存或 vendor）；`go:embed` 路径在包目录内且构建时存在；
  文档命令字面可执行、无 `<占位符>`；不新增 npm 包、不改 DSH 插件安装流程。
- 版本与兼容: Makefile 目标只增不改；二进制名 `bin/dsh-mcp` 与参数为对外契约；新增 `-monitor-addr` 等参数必须带默认值。
- 冻结哈希: 见 `.freeze`（文件级哈希）。

## 写域互斥表

| 子任务 | 独占写的路径 | 只读的路径 |
|---|---|---|
| m1 | `runs/web-monitor/40-plan.md`、`runs/web-monitor/50-subtasks` | `runs/web-monitor/00-scope.md`、`10-generalization.md`、`20-attack-log.md`、`30-constraints.md`、`15-interfaces.draft.md` |
| m2 | `design/tokens.json`、`design/dist` | `docs/`、`internal/monitor/web`（后续消费方，只读） |
| m3 | `internal/monitor/server.go`、`internal/monitor/api`、`internal/monitor/ws`、`internal/monitor/auth`、`cmd/dsh-mcp/main.go`、`internal/control/state.go`、`internal/control/state_test.go`、`internal/mcpserver/runTick.go`、`internal/mcpserver/server.go`、`internal/diagnostics/store.go`、`internal/bootstrap/bootstrap.go`、`go.mod`、`go.sum` | `internal/entity`、`internal/engine`、`internal/registry`、`internal/events`、`internal/function`、`internal/config`、`internal/monitor/web` |
| m4 | `design/components.json`、`design/composition.json` | `design/tokens.json`、`design/dist`、`15-interfaces.draft.md` |
| m5 | `internal/monitor/web/components` | `design/tokens.json`、`design/components.json`、`design/composition.json`、`design/dist` |
| m6 | `internal/monitor/web/index.html`、`internal/monitor/web/app.js`、`internal/monitor/web/app.css`、`internal/monitor/web/wsClient.js` | `internal/monitor/web/components`、`design/`、`internal/monitor/api`、`internal/monitor/ws` |
| m7 | `design/fixtures`、`Makefile`、`docs/web-monitor.md`、`runs/web-monitor/60-verification.md` | 全部实现与契约产物（只读） |

## 共享资源裁决

| 路径 | 处置（独占 / 复制 / 契约化 / 串行化） | 说明 |
|---|---|---|
| `Makefile` | 独占（m7） | m3 不改 Makefile；`make build` 的 design/web 前置与 `design`/`monitor` 目标全部由 m7 在批 5 一次性加。 |
| `go.mod` / `go.sum` | 独占（m3） | 新增 `github.com/coder/websocket` 由 m3 单方修改并锁版本；其它子任务不得改依赖。 |
| `data/` | 运行期独占 | 不属于任何子任务写域；运行时由 `bin/dsh-mcp` 以 `.engine.lock` 独占；测试一律 `t.TempDir()`。 |
| `internal/monitor/web` | 契约化（路径与文件名由 K4 冻结） | m5 只写 `web/components/**`，m6 只写 `web/index.html`/`app.js`/`app.css`/`wsClient.js`；路径不重叠，无串行依赖（m6 仍 depends-on m5 以拿到真实实现验收）。 |

## 模块映射

| 子任务 | 实现载体（代码模块 / 子智能体 / 工作流） | 并行批次 | 预估成本 |
|---|---|---|---|
| m1 | Lead 本人（本文件 + 子任务包；不派子体） | 1 | 低（契约文本） |
| m2 | 子智能体（设计令牌 + 生成 CSS） | 1 | 中（tokens.json + dist + 验收） |
| m3 | 子智能体（Go 后端：API/WS/鉴权/钩子/embed 接线） | 2 | 高（新包 + 跨包钩子 + 并发测试） |
| m4 | 子智能体（分解表 + components/composition） | 2 | 中（schema + 门禁绿） |
| m5 | 子智能体（元件实现，可按元件再并行 2–3 个） | 3 | 中高（6–8 个元件） |
| m6 | 子智能体（组合层 + WS 客户端接线） | 4 | 中（页面入口 + 装配） |
| m7 | 子智能体（Makefile/门禁反例/文档/证据表） | 5 | 中（脚本 + 文档 + 回归） |

## 约束归属

| 约束 | 状态 | 说明 / 证据 |
|---|---|---|
| C1 | 满足 | K5 + K6：监视器挂 `bin/dsh-mcp`（持 `.engine.lock`），不新增可写 data-dir 的入口。实测替代：双实例第二实例退出码 3（`TestAcquireEngineLockIsExclusive` / `TestE2EDoubleInstanceExitsThree`，见 60-verification §C1）；原拟名 `TestMonitorSameRuntimeAndLock` 不存在，按 D5 修正 |
| C2 | 满足 | K5：`OnCreate` 链式包装，回调只读传入实体，不调 Store 方法；`TestOnCreateCallbackNoStoreCall` |
| C3 | 满足 | K5：钩子类型定义在 `mcpserver`/`diagnostics`，两者零 import monitor；`go list` + `grep` 门禁 |
| C4 | 满足 | K5：tick observer 只入队，`tickMu` 外投递；`TestTickObserverNonBlocking -race` |
| C5 | 满足 | K1.9：`set` 的 Save 失败回滚内存或返回 `dirty:true`；`TestSetSaveFailureRollback` |
| C6 | 满足 | K1.8/K1.9：`tick` 走 `TryRunTick` → 409；`set` gate 等待 2s → 409/503；`TestGateBusyBounded` |
| C7 | 满足 | K3：静态免 token、API header token、WS 子协议 token、无 token 只读、写必须 token；`TestAuthMatrix` |
| C8 | 满足 | K3：Host 白名单 + Origin 精确 + 无 CORS 头；`TestHostAllowlist`/`TestWSOriginRejected` |
| C9 | 满足 | K2：每次连接必发 snapshot（方案②），无 `resumeSeq`；`TestWSSeqContract` |
| C10 | 满足 | K5：`grep -rn AddChangeListener internal/monitor/*.go` 恰 2 处（构造 + OnCreate），连接处理文件无注册；`go test -race ./internal/monitor/...` 无泄漏。原拟名 `TestReconnectNoLeak` 不存在，按 D5 修正 |
| C11 | 满足 | K2：诊断帧 `added/updated/removed`；`TestDiagnosticsFrameSemantics` |
| C12 | 满足 | K5：`diagnostics.SetNotify` 锁外回调；`TestNotifyNotUnderLock` |
| C13 | 满足 | K1.7：日志白名单/拒 symlink/尾部有界/完整行/≤1 MiB；`TestLogsGuards` |
| C14 | 满足 | K5：`/state` 与 monitor 都走 `Entity.Get`（含模板回退）；`TestPathParityWithControl` |
| C15 | 满足 | K1：`maxBytes` 确定性截断、`bytes` 口径写死；`TestMaxBytesDeterministic` |
| C16 | 满足 | K1.6：脚本源只走 `registry.ReadScript`；`TestScriptSourceGuards` |
| C17 | 满足 | K4/K6：门禁命令含 `--tokens design/tokens.json --src-root internal/monitor/web`；正例 0 + 4 反例非 0 |
| C18 | 满足 | K4/K6：`internal/monitor/web/**` 与 `design/dist` 入库且可复现；`git archive` 干净树编译 |
| C19 | 满足 | K6：`make build` 前置含 `design/` 与 `internal/monitor/web/**`；`touch … && make -q build` = 1 |
| C20 | 满足 | K6：`-ldflags` 注入 git short HEAD；文档/验收命令无 `<占位符>` |
| C21 | 满足 | K6：`coder/websocket` 锁版本；`GOPROXY=off make build` 通过 |
| C22 | 满足 | K6：`make test && make lint && make doctor`；modularity 路径可覆盖 |
| C23 | 满足 | K3：审计按日轮转 7 天、不泄 token、写失败不挡请求；`TestAuditLog` |
| C24 | 满足 | K1.8：control `/tick`、monitor `/tick`、WS `tick` 同一载荷构造函数；`TestTickPayloadParity` |

## 裁决记录

| 编号 | 门 | 决定 | 用户裁决 |
|---|---|---|---|
| D1 | G1 谱系 | 采用候选 B（7 模块：m1 契约 / m2 令牌 / m3 后端 / m4 元件契约 / m5 元件实现 / m6 组合 / m7 门禁文档）；Q2 监视器同进程挂载；Q3 写操作=tick+实体字段写；Q4 WS=引入 `github.com/coder/websocket` | 接受 |
| D2 | G2 约束 | C1–C24 全部接受（不降级）；红队推翻「astrbox-mcp 自带锁」假设，承载改为独立包 `internal/monitor` 由 `bin/dsh-mcp` 挂载；越界发现 #3 采纳「改 `/state` 跟随 MCP（走 Entity.Get）」；页面可用性只交 lint 门禁 + 人工验收（H1 保留待观察） | 接受 |
| D3 | G3 任务图 | 7 模块、5 批次（m1+m2 → m3+m4 → m5 → m6 → m7）、写域与 K1–K6 契约如本文件；用户裁决接受任务图并**进入 G4 实现** | 接受 |
| D4 | G3 验收口径修正 | m4 执行中发现 `lint-ui-plan` 的 G3/G4 会检查元件/场景 `source` 文件存在性，而 m4 阶段 m5/m6 源码尚未创建 → m4 的 lint 验收必然为红。走推荐：m4 只按真实未来路径登记 `source`，验收改为 JSON 可解析 + 字段/分解/组合树自查；lint 全绿由 m7 在 m5/m6 交付后统一执行。不改任何实现契约 | 接受（事后追认） |
| D5 | G4 证据口径修正 | 集成核对发现三处：①C1/C10 冻结判定引用的 `TestMonitorSameRuntimeAndLock` / `TestReconnectNoLeak` 不存在（`go test -run` 对不存在测试返回 0，假绿），改用双实例 flock 实测与 `grep`+`-race` 取证；②C18 的干净树/入库要求由 Lead 执行 `git add design internal/monitor/web` 后成立；③C20 的 2 处文档占位符由 Lead 修正。不改实现与运行接口 | 接受（事后追认） |

> 规则：三道门各至少一条；`用户裁决` 只能是 接受 / 修改 / 暂缓；默认走推荐的行必须在「决定」里写明「走推荐，事后追认」；出现「暂缓」则「未决项」不得为空。

## 未决项

| # | 未决 / 暂缓项 | 为何暂缓 | 复核触发条件 |
|---|---|---|---|
| H1 | 页面实际可渲染与可交互（点选实体、看日志、写字段成功） | 无 node/浏览器自动化；`lint-ui-plan` 只能判分解与样式来源，不能证明渲染与交互 | 用户人工验收（D2：只交门禁 + 人工验收）；接入 Playwright 时升级为 C 条目 |
| H3 | 审计字段 `value`/`path` 是否需脱敏 | 数据敏感边界需用户按数据性质裁决，红队无权定义 | 用户数据分级明确时；默认只记 `path` 与结果，不记 `value` |
| H4 | tick ring 容量 N=20 是否满足产品期望 | 产品取舍，无客观判据 | 用户试用后 |
| H5 | 30s ping / 90s pong 在浏览器后台标签页节流下是否误断 | 需真实浏览器后台节流环境，纯 Go 测试无法模拟 | 浏览器实测（后台挂 30 分钟）后 |
| P1 | `events.System` 对 `store.OnCreate` 的单赋值与监视器链式包装的最终形态 | 依赖 m3 实现；已由 C2 约束判定，但包装 API 未定 | m3 交付后跑 `TestOnCreateCallbackNoStoreCall` |

## 变更记录（冻结后任何契约改动都要走新裁决并重新 freeze）

| 日期 | 改动 | 裁决编号 | 重新冻结 |
|---|---|---|---|
| 2026-10-26 | 承载形态由「astrbox-mcp -transport http」改为「独立包 `internal/monitor` 由 `bin/dsh-mcp` 挂载」；`/state` path 语义统一走 `Entity.Get` | D2 | 待 freeze |
| 2026-10-26 | m4 验收口径：lint-ui-plan 的 source 存在性检查后移到 m7；m4 改为 JSON + 字段自查 | D4 | 已重新 freeze |
| 2026-10-27 | C1/C10 证据改实测替代（不存在的测试名移除）；C18 由 git add 成立；C20 占位符修正 | D5 | 已重新 freeze |
