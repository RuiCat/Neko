# 需求范围（web-monitor）

- 创建: 2026-10-26T20:18:35+08:00
- 目标（一句话）: 为 AstrBox 引擎添加 Go 内嵌前端网页监视器（状态/诊断/脚本/日志 + 受控写操作，WebSocket 实时）
- 状态: 草稿（待 G1 裁决后随谱系冻结）

## 目标

给 AstrBox 引擎加一个**浏览器可打开的监视器**，覆盖四类数据与两类受控写操作：

1. **只读数据**：实体状态树（全部实体 / 单实体全量或白名单点分路径）、诊断（error/warning/info，按实体/脚本/来源过滤）、脚本注册表（已应用脚本、hash、规则/模板/函数/事件计数）、最新 tick 结果与 `data/logs/state_YYYY-MM-DD.log` 日志流。
2. **受控写操作**：手动触发一次 tick；写单个实体字段（点分路径 + JSON 值）。
3. **实时**：页面通过 WebSocket 接收快照/增量/日志/tick 结果，断线重连后自动重订阅。
4. **交付形态**：前端资源 `go:embed` 进现有 Go 二进制，单 `make build` 产出，不引入 node 构建链。

非目标（本轮明确不做）：多用户账号体系、远程/公网部署、TLS 终止、SQLite/时序存储、移动端专门适配。

## 验收

| # | 验收项 | 判定方式（命令 / 标准） | 由谁判 |
|---|---|---|---|
| V1 | 监视数据面契约完整 | `runs/web-monitor/40-plan.md` 的 K1（REST JSON）/K2（WS 帧）与实现逐字段一致；`go test ./internal/monitor/...` 绿 | 主智能体 |
| V2 | 只读端点可查询 | 起服务后 `curl` 四个端点（state/diagnostics/scripts/logs）返回契约 JSON 且字段非空 | 主智能体 |
| V3 | 写操作受控且串行 | 手动 tick 与 MCP tick 共享 `mcpserver.TryRunTick`/`WithStateWrite` 串行语义（并发时 409/排队按契约）；写实体字段先鉴权、再走 `entity.Set` + `store.Save` | 主智能体 |
| V4 | WebSocket 实时可用 | 订阅后收到初始快照；写实体/tick 后收到增量帧；断开重连自动重订阅；`go test -race` 无泄漏 | 主智能体 |
| V5 | 鉴权边界 | 非回环监听无 token 拒绝启动（沿用 `cmd/astrbox-mcp/main.go:32-34` 判据）；WS 无 token/错 Origin 拒绝 | 主智能体 |
| V6 | 前端分解可机检 | `bash ~/.dsh/skills/ui-composition/scripts/lint-ui-plan.sh --plan-dir design --tokens design/tokens.json --src-root internal/monitor/web` 退出 0；至少 4 条反例退出非 0 | 主智能体 |
| V7 | 单二进制内嵌资源 | `make build` 后 `bin/astrbox-mcp` 内含页面与静态资源；无 node 依赖 | 主智能体 |
| V8 | 回归不破 | `make test`、`make lint` 绿；现有 MCP 工具清单/控制面契约不变 | 主智能体 |
| V9 | 文档可执行 | `docs/web-monitor.md` 命令可复制执行；`make monitor`（或等价目标）能起服务 | 主智能体 |

## 约束

| 类别 | 内容 | 来源 |
|---|---|---|
| 时间 / 预算 | L3 单轮规划：子智能体 ≤4/批、红队 1 轮、复核 1 轮；实现阶段预算另计 | 全局预算表 |
| 环境 | Linux；Go 1.25；构建缓存 `.local/`；无 node 构建链（用户裁决：Go 内嵌静态页） | 工作区事实 + 用户 |
| 既有接口不可动 | MCP 工具集合与 `internal/control` 的 `/health` `/state` `/tick` `/session-event` 语义不变；`bootstrap.Runtime` 字段可增不改 | `internal/mcpserver/server.go:57-69`、`internal/control/router.go:7-31` |
| 并发/锁 | 引擎写操作必须与 MCP 共享 gate：`mcpserver.RunTick`/`TryRunTick`（`internal/mcpserver/runTick.go`）与 `WithStateWrite`；监视器不得直接并发调 `engine.Tick` | `internal/control/server.go:106-120`、`internal/control/harness_test.go:107-130` |
| 代码规范 | 文件 <500 有效行（单功能）/ <1000（多功能）；camelCase；每文件一句职责；构建统一 `make` | `AGENTS.md` + `docs/ARCHITECTURE.md:140-145` |
| UI 规范 | 令牌层先冻结；元件只引用语义令牌；组合层不造原子、不覆盖样式；`lint-ui-plan` 全绿 | `ui-composition` 技能 |
| 安全 | 默认只绑回环；写操作必须鉴权（token）+ Origin 校验；日志只读不写；不得读凭据/环境变量 | 用户 + 现有 `httpAuth.go` |
| 数据落点 | `design/` 放令牌与元件契约真源；前端实现放 `internal/monitor/web/`；过程产物进 `.local/` | 本规划（待 D 裁决） |

## 未知

| # | 未知项 | 为何会改变方案 | 如何消除 | 何时必须消除 |
|---|---|---|---|---|
| U1 | 监视器承载形态：`astrbox-mcp -transport http` 同进程挂 `/monitor`，还是新增 `cmd/astrbox-monitor` 独立二进制 | 改变 m1/m2 写域与启动/鉴权复用方式；独立进程需复制 bootstrap 与锁语义 | 用户裁决（Q2） | G1 前 |
| U2 | WebSocket 实现：零依赖手写 RFC6455 最小帧，还是引入 `github.com/coder/websocket` 等库 | 影响依赖面、测试与代码量；手写有分片/掩码/关闭握手风险 | 用户裁决（Q4）+ 红队攻击 | G3 前 |
| U3 | 写操作范围：仅 tick / tick + 实体字段写 | 决定鉴权强度、CSRF/Origin、审计日志需求与测试面 | 用户裁决（Q3） | G3 前 |
| U4 | 日志流：轮询 tail 文件 vs fsnotify | 影响依赖（fsnotify 第三方）与 CPU；日志文件按日切换 | 红队 + 实现期小实验 | G3 前 |
| U5 | 前端设计真源落点：`design/`（项目根）vs `internal/monitor/design/` | 影响写域与 `go:embed` 路径；ui-composition 要求真源可重建 | 用户裁决（并入 Q1 推荐） | G3 前 |
| U6 | UI 验收工具：无 node/Playwright 时，浏览器交互怎么判 | 决定 V6 是否只有 lint 门禁、交互靠手工清单 | 红队 + 主智能体裁决 | G3 前 |

## 变更记录（需求中途变更时追加，并把受影响产物标 stale）

| 日期 | 变更 | 受影响产物 | 需重做的门 |
|---|---|---|---|
| 2026-10-26 | 用户裁决：范围 = 四类数据 + 可操作；形态 = Go 内嵌静态页单二进制；实时 = WebSocket；数据面 = 新增独立 HTTP 监视端点 | — | G0 已记录 |
