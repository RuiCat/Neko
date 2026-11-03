# 接口契约草稿（web-monitor）

> 本文件是 **G2 红队输入**：只描述接口与承载方式，不写「为什么这么设计」的自辩。
> 红队按 `20-attack-log.md` 模板逐面攻击；有效攻击在 G3 收敛为 `40-plan.md` 的冻结契约（K1/K2/K3/K4）。
> 任何字段可在 ★裁决2 后被改。

## K0 承载与路由

- **进程**：沿用 `cmd/astrbox-mcp`，仅 `-transport http` 分支挂监视器。启动：
  `./bin/astrbox-mcp -data-dir data -scripts-dir scripts -transport http -addr 127.0.0.1:8765`
- **监听的三种形态**（草稿，待红队攻击）：
  1. 回环地址 + 无 `http_token`：页面与只读 API 可用，写操作拒绝（403）。
  2. 回环地址 + 有 `http_token`：页面/读写 API 均可，token 必填。
  3. 非回环地址 + 无 token：沿用 `main.go:32-34` 直接拒绝启动。
- **路由前缀**：监视器全部在 `/monitor/*`；现有 MCP 端点路径不变（`cmd/astrbox-mcp/main.go:35-42` 的 handler 保留）。
- **同一 listener、同一 token 中间件**：`/monitor/api/*` 与 `/monitor/ws` 走与 MCP 相同的 `tokenMiddleware`，
  但 WS 的 token 取法不同（见 K3）。

## K1 REST 数据面（只读 + 写）

通用响应：成功 `200 {"ok":true, ...payload}`；失败 `4xx/5xx {"ok":false,"error":{"code":"<稳定码>","message":"<中文>"}}`。
错误码在 `internal/control/response.go` 的 `CODE_*` 基础上增加：`unauthorized`(401)、`forbidden`(403)、`too_large`(413)、`write_disabled`(403)。

### K1.1 `GET /monitor/api/health`

- 响应：`{"ok":true,"version":"<构建版本>","pid":<int>,"dataDir":"<abs>","scriptsDir":"<abs>","uptimeMs":<int>,
  "engine":{"entities":<int>,"templates":<int>,"rules":<int>,"scripts":<int>},"diagnostics":{"total":<int>,"error":<int>,"warning":<int>,"info":<int>},"monitor":{"readOnly":<bool>}}`
- 语义：进程存活即 200；引擎未就绪（bootstrap 失败）不会走到这里（进程已退出）。

### K1.2 `GET /monitor/api/entities`

- 请求：`?maxBytes`（默认 262144，范围 4096..4194304）；响应截断时 `truncated:true` 且 `items` 只含 id/type/keys。
- 响应：`{"ok":true,"items":[{"id":"<string>","type":"<string>","keys":{...}}],"bytes":<int>,"truncated":<bool>}`
- 排序：id 字典序稳定。

### K1.3 `GET /monitor/api/entities/{id}`

- 请求：`?path=<点分路径>`（可重复 0..N）、`?maxBytes`（默认 65536，范围 1024..1048576）。
- 响应（无 path）：`{"ok":true,"entity":"<id>","type":"<type>","data":{...},"bytes":<int>,"truncated":<bool>}`
- 响应（有 path）：`{"ok":true,"entity":"<id>","values":{"<path>":<任何 JSON>},"missing":["<path>"],"bytes":<int>}`
- 错误：实体不存在 404；`maxBytes` 越界 400。

### K1.4 `GET /monitor/api/diagnostics`

- 请求：`?severity=error|warning|info&source=&entity=&script=&code=&limit`（默认 100，范围 1..512）。
- 响应：`{"ok":true,"generation":<int>,"summary":{"total":<int>,"error":<int>,"warning":<int>,"info":<int>},
  "items":[{"id","code","severity","source","script","entity","type","line","message","hint","count","first_seen","last_seen"}]}`
- 数据源：`internal/diagnostics.Store.Snapshot(filter, limit)`（内存环形，重启清空）。

### K1.5 `GET /monitor/api/scripts`

- 响应：`{"ok":true,"items":[{"name":"<id>","path":"<rel>","hash":"<sha1>","rules":<int>,"templates":<int>,"funcs":<int>,"events":<int>}]}`
- 数据源：`registry.ListScripts()`。

### K1.6 `GET /monitor/api/scripts/{name}`

- 请求：`?source=true|false`（默认 false）。
- 响应：`{"ok":true,"name":"<id>","hash":"<sha1>","actualHash":"<sha1>","rules":[...],"templates":[...],"funcs":[...],"events":[...],"source":"<text，source=true 时>"}`
- 错误：未知脚本 404；source 超 256 KiB → `too_large`（413）。

### K1.7 `GET /monitor/api/logs`

- 请求：`?kind=state|control&date=YYYY-MM-DD&limit`（默认 200，范围 1..2000）。
- 响应：`{"ok":true,"kind":"state","date":"YYYY-MM-DD","lines":[{"ts":"<RFC3339>","raw":"<string>","parsed":<object|nil>}],"truncated":<bool>}`
- 语义：`state` 读 `data/logs/state_YYYY-MM-DD.log`（JSONL）；`control` 读 `data/logs/dsh-control-YYYYMMDD.log`（纯文本）。
  `date` 缺省 = 今天；文件不存在 → 200 + 空数组（不是 404）；非法 date → 400。
- 限制：只允许读 `data/logs/` 下符合两种命名的文件（防目录穿越）。

### K1.8 `POST /monitor/api/tick`

- 请求：`{"scope":"<string，可选，默认空=全量>"}`
- 语义：复用 `mcpserver.TryRunTick(scope, TickOptions{SkipSaveWhenUnchanged:true})`；已有 tick/写在飞 → 409 `tick_busy`。
- 响应：`{"ok":true,"tickId":"<string>","status":"<string>","transitions":[...],"conflicts":[...],"errors":[...],"hint":"<string>","rules_evaluated":<int>}`
  （字段与 `internal/control/tick.go:55-66` 一致）
- 错误：`500 internal`（Tick/Save 失败，message 带原因）；`403 write_disabled`（回环无 token 形态）。

### K1.9 `POST /monitor/api/entities/{id}/set`

- 请求：`{"path":"<点分路径>","value":<任意 JSON>}`
- 语义：`mcpserver.WithStateWrite` 包裹 `ent.Set(path, value)` + `store.Save()`；与 tick/其它写串行。
- 响应：`{"ok":true,"entity":"<id>","path":"<path>","value":<JSON>,"saved":true}`
- 错误：实体 404；path 非法/不存在 400；写失败/保存失败 500；`403 write_disabled`。

### K1.10 `GET /monitor/api/tick/latest`

- 响应：`{"ok":true,"items":[{"tickId","status","transitions","conflicts","errors","hint","rules_evaluated","at":"<RFC3339>"}]}`
- 数据源：新增内存 ring（最近 `N=20` 次 tick，进程内，重启清空）。**此数据当前不存在**，需在 m3 新增。

## K2 WebSocket 实时通道

### K2.1 连接

- URL：`GET /monitor/ws?token=<token>&topics=state,diagnostics,logs,tick&entity=<id>&paths=<a,b>`
- 握手：标准 RFC6455；引入 `github.com/coder/websocket`。
- 鉴权：token 载体（草稿，待攻击）：
  - 首选 `Sec-WebSocket-Protocol: astrbox.token.<token>`（不进 URL/访问日志）；客户端同时在 `token` query 兼容。
  - 无 token 形态（回环只读）：允许连接，但只推 `state/diagnostics/tick/logs` 只读帧。
- Origin：若请求带 `Origin`，必须与 `Host` 同源（scheme+host+port），否则 403 并关闭。

### K2.2 帧格式（JSON text）

服务端 → 客户端：

| type | 何时 | payload |
|---|---|---|
| `hello` | 连接建立 | `{"version":"<string>","readOnly":<bool>,"serverTime":"<RFC3339>"}` |
| `snapshot` | 订阅后首个帧 | `{"topic":"state","seq":<int>,"entities":[...]}`（按 topic 分帧） |
| `delta` | 实体字段变化 | `{"topic":"state","seq":<int>,"entity":"<id>","path":"<path>","oldValue":<JSON>,"newValue":<JSON>}` |
| `tick` | tick 完成 | `{"topic":"tick","seq":<int>,"result":{...K1.8 字段...}}` |
| `diagnostics` | 诊断新增 | `{"topic":"diagnostics","seq":<int>,"record":{...}}` |
| `log` | 日志追加 | `{"topic":"logs","seq":<int>,"kind":"state\|control","line":{...}}` |
| `pong` | 心跳应答 | `{"seq":<int>,"serverTime":"<RFC3339>"}` |
| `error` | 协议/鉴权错误 | `{"code":"<string>","message":"<string>"}` |

客户端 → 服务端：

| type | payload |
|---|---|
| `subscribe` | `{"topics":["state","diagnostics","logs","tick"],"entity":"<可选>","paths":["<可选>"]}` |
| `unsubscribe` | `{"topics":[...]}` |
| `ping` | `{}` |

### K2.3 序号、重连、背压（草稿，待攻击）

- 每连接一个单调递增 `seq`（uint64），`hello` 不带 seq。
- 客户端重连可带 `?resumeSeq=`（草案设想；**最终 K2 未采用**，见 `40-plan.md`：每次连接必发 snapshot）；服务端只保证 `seq > resumeSeq` 的增量，缺口过大（> 1024）时改发 `snapshot`。
- 每连接有界发送队列（256 帧）。队列满：丢弃 `delta`，置 `dropped:true` 并在队列恢复后发一次 `snapshot` 替代丢失增量；不得阻塞引擎写路径。
- 服务端每 30s 发 WebSocket ping 控制帧；90s 无 pong 关闭连接。

### K2.4 数据来源（草稿，待攻击）

- `state`：启动时对 `store.ListAll()` 每个实体挂 `Entity.AddChangeListener`；新实体通过 `store.OnCreate` 补挂。
  **注意**：`internal/events/system.go:27` 已直接赋值 `store.OnCreate = func(...)`，监视器不得覆盖，需链式包装（此点列为红队攻击目标）。
- `diagnostics`：诊断 Store 当前只有 `Add`，**没有变更通知**；草稿方案是 m3 新增 `SetNotify(func(Record))` 或在监视器里轮询 generation。
- `tick`：包一层 `runTickLocked` 的调用点（`mcpserver`）或由监视器自己发起 tick 时广播；MCP 工具触发的 tick 也需要广播。
- `logs`：轮询文件尾（默认 1s）或 `fsnotify`（未引入依赖）；按日切换时重置偏移。

## K3 鉴权与写安全（草稿，待攻击）

- **读**：回环无 token 允许；非回环必须 token。
- **写**（tick / set）：必须 token，即使回环；回环无 token 形态下 `readOnly:true`，写返回 403 `write_disabled`。
- **CSRF**：token 只从 header / WS 子协议 / query 取，不设 cookie；写请求校验 `Origin`（若存在）与 Host 同源。
- **审计**：每次写操作追加 `data/logs/monitor-audit-YYYYMMDD.log`（时间/动作/实体/path/结果/来源 IP），写失败不影响响应。
- **不做**：账号、会话、RBAC、TLS。

## K4 前端交付与门禁（草稿）

- **真源**：`design/tokens.json`、`design/components.json`、`design/composition.json`（ui-composition 三件套）。
- **生成物**：`design/dist/tokens.css`（由令牌生成）；前端静态资源放 `internal/monitor/web/`（`go:embed`）。
- **门禁**：`bash ~/.dsh/skills/ui-composition/scripts/lint-ui-plan.sh --plan-dir design` 退出 0；
  反例（硬编码样式/原始色阶/未登记元件/组合层造原子）各一份进 `design/fixtures/` 并断言非 0。
- **构建**：`make build` 内含 `design` → `web` 的生成步骤（脚本零依赖，node 不参与）；`make test` 不依赖生成物已存在
  （或生成物进版本库，二选一，待红队攻击）。
- **页面**：`GET /monitor/` 返回 `index.html`；SPA 回退到 `index.html`；`/monitor/api/*` 与 `/monitor/ws` 不参与回退。

## 待红队重点攻击的 8 个点

1. 同进程挂载与现有 `main.go` 的 handler 组装方式是否冲突（streamable MCP handler 是否吃 `/monitor` 路径）。
2. `store.OnCreate` 单赋值被 `events.System` 占用，监视器补挂新实体的方案是否成立。
3. 回环无 token 允许读、写必须 token 的「两种形态」是否可用（用户本机没配 token 时页面只能看不能动）。
4. WS token 走 query 会进访问日志；子协议方案在浏览器与 `coder/websocket` 客户端的兼容性。
5. 诊断无变更通知、日志需轮询，是否会引入忙轮询/漏事件。
6. 写操作与 tick 共享 `tickMu` 的语义：`WithStateWrite` 不可重入，set 内部若触发 `@事件` 再调 tick 会死锁。
7. `go:embed` 与 `design/` 真源分离：未生成时 `go test ./...` 是否失败；生成物是否进版本库。
8. 无 node/Playwright 时，UI 交互验收只剩 lint 门禁 + 手工清单，是否足以判「页面可用」。
