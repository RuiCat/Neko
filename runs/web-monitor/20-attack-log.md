# 红队攻击日志（web-monitor）

- 创建: 2026-10-26T20:18:35+08:00
- 完成: 2026-10-26T20:52:00+08:00
- 红队执行体: `red-team-monitor`（独立上下文，未参与泛化与接口起草）
- 输入: `00-scope.md`、`15-interfaces.draft.md`、项目源码（`internal/`、`cmd/`、`Makefile`、`go.mod`、`go.sum`）、`github.com/modelcontextprotocol/go-sdk@v1.6.0` 依赖源码、ui-composition 门禁脚本
- 独立性: 按任务书**未读** `10-generalization.md`（教师自辩与候选理由）；结论只依赖草案文字与源码事实
- 处置: 有效攻击固化到 `30-constraints.md`；接受/降级由 Lead+用户裁决
- 本轮只读核对，未执行 `make build` / `make test`；凡判定命令均标注"待实现后执行"

## 六攻击面结论（缺一不可）

> 结论列只填 `击穿` 或 `未击穿` 二者之一。依据见下文明细（A<n>-<m>）。

| 面 | 结论 | 依据（反例 / 命令 / 证据摘要） |
|---|---|---|
| A1 假假设 | 击穿 | A1-1 同进程假设与 DSH 实际入口 `bin/dsh-mcp` 矛盾（两个引擎进程写同一 data-dir）；A1-3 tokenMiddleware 只读 header，浏览器页面/WS 无法携带；A1-4 K1.9 的「path 不存在 400」与 `entity.Set` 语义相反；A1-5 resumeSeq 无历史缓冲定义 |
| A2 隐藏耦合 | 击穿 | A2-1 `store.OnCreate` 在 store 写锁内回调（`store.go:260-274`），回调调 Store 方法即死锁；A2-2 tick 广播落在 `tickMu` 内（`runTick.go:36-54,56-65`）；A2-3 引擎每 tick 注册监听器且移除只置 nil，长期运行槽位线性增长（`engine.go:543-558`+`model.go:45-51`）；A2-4 诊断无通知；A2-6 path 数据源与 MCP 工具不一致；A2-7 tick 载荷无法单一来源 |
| A3 循环依赖 | 击穿 | A3-1 草案 K2.4 让 mcpserver 调用点回调监视器，而监视器经 K1.8/K1.9 依赖 mcpserver → 类型引用即 import cycle；A3-2 诊断通知若由 monitor 定义/引用则 diagnostics→monitor 与 monitor→diagnostics 成环 |
| A4 边界 / 失败 / 回滚 | 击穿 | A4-1 `ent.Set` 先改内存再 `store.Save`，保存失败不回滚且两文件非原子（`model.go:126-157`、`store.go:136-139`）；A4-2 `WithStateWrite` 用 `Lock` 且监视器无 deadline，gate 被占时 HTTP 无界挂起；A4-3 maxBytes、A4-4 日志、A4-5 WS 快照/增量边界均未定义 |
| A5 资源与权限 | 击穿 | A5-1 Origin==Host 自比自，挡不住 DNS rebinding，且 monitor 路由绕过 SDK 自带 Host 校验（`streamable.go:249-259`）；A5-2 WS token 子协议字符集/回显未约束、`http_token` 无校验（`config.go:24-25`）；A5-3 source 读取未指定带护栏的 `registry.ReadScript`；A5-4 审计日志无轮转 |
| A6 交付可验证性 | 击穿 | A6-1 lint 门禁默认令牌路径是 `<plan-dir>/tokens/tokens.json`（`lint-ui-plan.sh:24`），与 K4 的 `design/tokens.json` 不符且未传 `--src-root`，门禁验错对象；A6-2 `go:embed` 缺生成物即编译失败；A6-3 Makefile 不把 design/web 资产列为前置；A6-4 V6 命令含占位符；A6-5 WebSocket 依赖不在 `go.mod`/`go.sum`/模块缓存；A6-6 V8 无命令且 lint 依赖 `$HOME` 绝对路径 |

## 攻击明细

### A1-1 同进程假设与真实 MCP 入口不一致（同一 data-dir 出现两个可写引擎）

- 攻击点: K0:9「沿用 `cmd/astrbox-mcp`，仅 `-transport http` 分支挂监视器」+ 00-scope V3「手动 tick 与 MCP tick 共享 `TryRunTick`/`WithStateWrite` 串行语义」。该假设只在"MCP 客户端也连 `astrbox-mcp -transport http` 这一进程"时成立。
- 源码事实: 本项目 DSH 插件把 `${REPO_ROOT}/bin/dsh-mcp` 配置为 MCP 命令（`dsh/cordis.patch.yml:17`）；`dsh-mcp` 对 `<data-dir>/.engine.lock` 取 `flock` 单实例锁（`cmd/dsh-mcp/lock.go:15-16,36-56`、`cmd/dsh-mcp/main.go:68-79`）并装配自己的 runtime（`cmd/dsh-mcp/main.go:81-89`）；而 `cmd/astrbox-mcp` **从不获取该锁**，也各自 `bootstrap.New` 一份内存引擎（`cmd/astrbox-mcp/main.go:23-29`）；`Store.Save` 每次整体重写两份文件（`internal/entity/store.go:116-140`）。
- 反例（待实现后执行）:
  ```
  ./bin/dsh-mcp -data-dir /tmp/abx &            # 持有 .engine.lock、装载引擎 A
  ./bin/astrbox-mcp -data-dir /tmp/abx -transport http -addr 127.0.0.1:18765   # 引擎 B，不报锁占用
  # 通过 dsh-mcp 的 MCP 工具写 猫猫.状态.情绪=开心（落盘）
  curl -s http://127.0.0.1:18765/monitor/api/entities/猫猫   # B 的内存没有该写入 → 旧值
  curl -s -X POST .../monitor/api/entities/猫猫/set -d '{"path":"备注","value":"x"}'  # B 用旧快照整体 Save → 覆盖 A 的写入
  ```
- 红队结论: **击穿**。两个独立引擎共享同一 data-dir 时：① 互不串行（V3 不成立）；② 监视器读到的永远是启动时快照（丢失外部写入）；③ `Save` 整体覆盖导致 last-writer-wins 丢更新；④ WS 收不到 dsh-mcp 进程里的 tick/写入（V4 不成立）；⑤ 诊断环与 tick 环也各一份。这直接推翻 K0 的承载选择。
- 建议处置: C1。二选一：把监视器挂进 `dsh-mcp`（同一 runtime + 新增 TCP listener），或让 `astrbox-mcp -transport http` 获取同一 `.engine.lock`（锁被占即拒绝启动）并在契约中声明"监视器只覆盖本进程引擎"。
- 建议严重级: 阻断

### A1-2 与现有 handler 组装冲突（未击穿）

- 攻击点: 草稿 8 点之 1「streamable MCP handler 是否吃 `/monitor` 路径」。
- 源码事实: `main.go:35-37` 的 `mcp.NewStreamableHTTPHandler` 是 catch-all，无路径判断（`streamable.go:249` 起 `ServeHTTP` 直接进入会话逻辑）；`main.go:44` 直接 `http.ListenAndServe(addr, handler)`，当前无 mux。外层 `http.ServeMux` 只要把 `/monitor/` 与 `/monitor` 注册在 `/` 之前，其余路径回退给 MCP handler，现有 MCP 路径不受影响。
- 红队结论: **未击穿**。挂载本身可行，代价只是把"裸 handler"改为 mux 组合；但注意 SDK 的 DNS-rebinding Host 校验只覆盖回退给它的请求，monitor 路由**不继承**该保护（见 A5-1）。
- 建议处置: 无需冻结约束；实现时在 main 中显式组合 mux，并为 monitor 路由补 Host 校验（并入 C8）。
- 建议严重级: 中

### A1-3 「同一 token 中间件」复用于页面与 WS 不成立

- 攻击点: K0:16-17「`/monitor/api/*` 与 `/monitor/ws` 走与 MCP 相同的 `tokenMiddleware`」；K3:140-142「读回环允许 / 写必须 token」；K2.1:95-98 WS token 走子协议或 query。
- 源码事实: `tokenMiddleware` 只读 `X-Auth-Token` 或 `Authorization: Bearer`（`cmd/astrbox-mcp/httpAuth.go:10-27`，读取在 13-19，精确比较在 20）。浏览器的顶层导航、`<script>`/`<link>` 加载与 `WebSocket` 握手都**不能**自定义请求头。
- 反例（待实现后执行）:
  ```
  # 配置 http_token=t 后
  curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8765/monitor/       # 中间件 → 401，页面根本打不开
  curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8765/monitor/index.html
  # 浏览器 new WebSocket('ws://127.0.0.1:8765/monitor/ws', ['astrbox.token.t'])
  #   → 握手请求无 Authorization/X-Auth-Token，先被 tokenMiddleware 401，子协议永远到不了 WS accept
  ```
- 红队结论: **击穿**。K0:16 与 K2.1:96 自相矛盾：要么页面 401（静态资源被中间件挡），要么 WS 401（子协议在中间件之后才可见）。且草案没有定义浏览器如何获得 token（不设 cookie、页面又不带 token），"有 token 形态"在浏览器里不可用。
- 建议处置: C7。路径分层鉴权：静态资源免 token；API 走 header；WS 在中间件**之前**分流（子协议或 query），页面提供 token 输入并存 sessionStorage。
- 建议严重级: 阻断

### A1-4 K1.9「path 不存在 400」与 `entity.Set` 语义相反

- 攻击点: K1.9:79-82 承诺「path 非法/不存在 400」并「走 `ent.Set(path,value)`」。
- 源码事实: `Entity.Set` 只在空路径时报 `PathNotFoundError`（`internal/entity/model.go:126-130`）；对缺失路径会逐层创建 map 后写入（`model.go:135-145`），不存在的 path 不会报"不存在"。`oldValue` 也是用 `Get` 尽力取（`model.go:131-134`），模板回退可能给出旧值。
- 反例（待实现后执行）:
  ```
  curl -s -X POST .../monitor/api/entities/猫猫/set -d '{"path":"新字段.子字段","value":1}'
  # 现状语义：200 成功并创建 新字段.子字段；而 K1.9 写的是 400（path 不存在）
  ```
- 红队结论: **击穿**。契约与实现语义二选一未定，验收会按 400 判失败或按 200 判契约错；`?path=` 空段、超长路径、`a..b` 也未定义。
- 建议处置: C1。明确"不存在=创建（与 MCP `entity_state_set` 一致）"，删除 400 承诺；另定 path 语法（非空、无空段、长度/深度上限）并只对语法非法返回 400。
- 建议严重级: 高

### A1-5 `resumeSeq` 假设服务端可续传增量

- 攻击点: K2.3:125-126「每连接一个单调递增 seq」「重连可带 `?resumeSeq=<n>`；只保证 `seq > resumeSeq` 的增量，缺口 >1024 改发 snapshot」。
- 源码事实: 草案没有定义任何进程级/连接外的增量历史缓冲；seq 是"每连接"的，新连接从 1 起，`seq > resumeSeq` 在重连后要么永不成立、要么与旧连接序号重叠。`hello` 也不带 seq（K2.3:125），客户端无法判断服务端是否重启。
- 红队结论: **击穿**。resumeSeq 在当前定义下不可实现：没有可回放的数据结构，也没有代际（epoch）标识；重连后要么静默丢更新，要么重复推送。
- 建议处置: C9。二选一：进程级单调 seq + ≥1024 帧环形重放缓冲 + `epoch`；或删除 resumeSeq，重连首帧必发 snapshot。
- 建议严重级: 高

### A1-6 非回环无 token 拒绝启动（未击穿）

- 攻击点: K0:14 形态 3 与 00-scope V5 引用 `main.go:32-34`。
- 源码事实: `main.go:32-34` 在 `!isLoopbackAddr(*addr) && HttpToken == ""` 时 `log.Fatalf`；`isLoopbackAddr` 对空 host 与非回环 IP 都返回 false（`httpAuth.go:29-43`），`:8765`、`0.0.0.0:8765` 均会强制要求 token。
- 红队结论: **未击穿**。事实核对通过（但"拒绝启动"只保护 HTTP 入口，不解决 A1-1 的跨进程问题）。
- 建议处置: 无需新约束；与 C1 叠加检查（锁 + token 两道启动门）。
- 建议严重级: 中

### A2-1 `store.OnCreate` 在 store 写锁内回调，监视器链式包装易死锁

- 攻击点: K2.4:132-133「启动时对 `store.ListAll()` 每个实体挂 `Entity.AddChangeListener`；新实体通过 `store.OnCreate` 补挂」。草案自己也标了此点为攻击目标。
- 源码事实: `store.Create` 先 `s.mu.Lock()`（`internal/entity/store.go:261`），放入实体后**持锁**调用 `s.OnCreate(ent)`（`store.go:271-273`）；`events.NewSystem` 直接赋值该字段（`internal/events/system.go:27`），`Store` 不提供链式注册 API（`store.go:22-23`）。`sync.RWMutex` 不可重入。
- 反例（待实现后执行）:
  ```
  // 监视器包装后，若回调里做任何 Store 加锁读：
  store.OnCreate = func(e *entity.Entity) { _ = store.ListAll(); attach(e) }
  // MCP 调 entity_create → store.Create 持写锁 → OnCreate → ListAll 求读锁 → 永久阻塞
  go test ./internal/monitor -run TestOnCreateCallbackNoStoreCall -timeout 15s   # 期望通过；把回调改成 ListAll 必须超时失败
  ```
- 红队结论: **击穿**。回调内只能读传入的 `*Entity`；调用 `Get/ListAll/Count/ListTemplates/GetTemplate/Templates/Save/Query*` 任一方法都会挂住整个进程的创建路径（并连带 tick）。
- 建议处置: C2（行为测试 + 代码检查双判定）。
- 建议严重级: 阻断

### A2-2 tick 广播挂 `runTickLocked` 时处于 `tickMu` 内

- 攻击点: K2.4:135「包一层 `runTickLocked` 的调用点（mcpserver）」+ K2.2:109 tick 帧 + K1.10 tick ring。
- 源码事实: `runTickLocked` 明确要求调用方持有 `tickMu`（`internal/mcpserver/runTick.go:36-54`）；`WithStateWrite` 注释写明 gate 不可重入（`runTick.go:56-65`，注释在 59-60）；现有唯一"广播类"钩子 `recordTick` 就在锁内被调用（`runTick.go:43` → `internal/mcpserver/diagnosticsTools.go:81-110`，其中还会取 `s.machine.PendingConflicts()`）。
- 红队结论: **击穿**。观察者若调 `RunTick` 会死锁；若同步阻塞（例如向满队列阻塞写）则直接拉长 tick、拖住所有写请求；K2.3:127 只承诺"WS 队列不阻塞引擎写路径"，没约束观察者本身。
- 建议处置: C4。观察者必须非阻塞入队 + 禁止 gate 内调用 `RunTick/TryRunTick/WithStateWrite/Save`；用"观察者 sleep 500ms 时 tick 墙钟仍 <100ms"的测试钉死。
- 建议严重级: 高

### A2-3 每 tick 监听器槽位线性增长，长期运行下写路径劣化（既有缺陷，叠加放大）

- 攻击点: K2.4:132 依赖 `AddChangeListener`；监视器的卖点是"页面长期开着"。
- 源码事实: 引擎每 tick 给**每个实体**注册监听器并在 tick 结束移除（`internal/engine/engine.go:543-558`，挂载/移除在 `engine.go:216-223`）；而 `AddChangeListener` 的移除只是把槽位置 nil，不压缩切片（`internal/entity/model.go:40-52`，置 nil 在 45-51）；`Set` 每次写都**复制整个 listeners 切片**（`model.go:146`）。每 tick 每个实体新增 1 个槽位，永不回收。
- 反例（待实现后执行）:
  ```
  // 100 实体、tick 1000 次后，每个实体的 listeners 长度 ≈ 1000（绝大多数是 nil）
  go test -race ./internal/entity -run TestListenerSlotsDoNotGrow   # 现状必然失败
  ```
- 红队结论: **击穿**（对"长期运行"的资源假设）。这是引擎既有缺陷，不是监视器引入，但监视器的"常开页面"把 O(tick 数) 的内存与每写 O(槽位) 的复制成本暴露成用户可见的退化；监视器若再按连接注册监听器会成倍放大。
- 建议处置: 越界上报（见"越界发现"#1，不自行修）；监视器侧由 C10 约束"全进程只注册一次、连接不注册"。
- 建议严重级: 高

### A2-4 诊断无变更通知，generation 轮询无法区分新增/更新/清除

- 攻击点: K2.4:134「诊断 Store 当前只有 Add，没有变更通知；草稿方案是新增 SetNotify 或轮询 generation」。
- 源码事实: `internal/diagnostics/store.go` 只有 `Add`（31-57）、`Snapshot`（113-127）、`Summary`（130-149）、`Generation`（152-156）、`Len`（159-163），无任何回调字段；去重更新会 `Count++` 并抬高 generation（`store.go:38-45`）；`ClearByScript` 等也 `generation++`（`store.go:95-110`）。K2.2:110 只有一种 `diagnostics` 帧，payload 只有 `record`。
- 红队结论: **击穿**。轮询只能看到"generation 变了"，无法区分：新记录 / 同一条 count 增加 / 记录被 Clear / 环形淘汰。按 K2.2 的帧语义，客户端会把一次去重更新画成两条诊断，或者记录被清除后页面永远残留。
- 建议处置: C11。帧必须带 `added/updated/removed` 语义（或 generation 变化发全量 snapshot + generation），并保留 `count/first_seen/last_seen`。
- 建议严重级: 高

### A2-5 诊断通知若在 `Add` 的锁内回调 → 死锁

- 攻击点: K2.4:134 建议的 `SetNotify(func(Record))`。
- 源码事实: `Add` 全程持有 `s.mu`（`internal/diagnostics/store.go:36-45`），`Snapshot`/`Summary`/`Generation`/`Len` 都要拿同一把锁（`store.go:113-163`）。
- 反例（待实现后执行）:
  ```
  store.SetNotify(func(r diagnostics.Record) { _ = store.Generation() })
  store.Add(record)   // 若 notify 在 s.mu 内调用 → 自锁
  go test ./internal/diagnostics -run TestNotifyNotUnderLock -timeout 10s
  ```
- 红队结论: **击穿**（若按最自然写法实现）。与 A2-1 同一模式：被观察者持锁回调，观察者回读即死锁。
- 建议处置: C12。回调必须在解锁后调用（收集后统一触发），并有"回调内调 Generation 不超时"的测试。
- 建议严重级: 高

### A2-6 K1.3 的 path 数据源与 MCP `entity_state_get` 不一致

- 攻击点: K1.3:36-41（`values`/`missing`）没有声明数据源；00-scope V1 要求与 MCP 工具语义一致。
- 源码事实: 若照抄 control `/state`：`handleState` 用 `entity.Snapshot()`（`internal/control/state.go:45-52`），`pathsStateResponse` 只在快照副本里查（`state.go:74-98`），`lookupSnapshotPath` 无模板回退（`state.go:114-125`）；而 MCP `entity_state_get` 走 `ent.Get`，会回退模板链（`internal/mcpserver/server.go:128-132`、`internal/entity/model.go:99-114`）。
- 反例（待实现后执行）:
  ```
  // 先 Create 实体，再 AddTemplate 增加模板字段（实例 Data 不含该字段）
  // MCP entity_state_get → 返回模板值；/monitor/api/entities/{id}?path=... → missing
  go test ./internal/monitor -run TestPathParityWithControl   # 现状两边不一致
  ```
- 红队结论: **击穿**。同一 path 在两个入口一个有值一个 missing；页面按 missing 画空，MCP 却能读到值，验收（V1/V3）会互相打脸。
- 建议处置: C14。K1.3 明确数据源，并与 `/state` 保持同一语义；加 parity 测试。
- 建议严重级: 中

### A2-7 tick 载荷无法单一来源，复制字段必然漂移（K1.8 已漏 `tick_id`）

- 攻击点: K1.8:73-74「字段与 `internal/control/tick.go:55-66` 一致」；K2.2:109 tick 帧也要同一载荷。
- 源码事实: `tickBody` 是 `internal/control` 的未导出函数（`internal/control/tick.go:55-66`），其 body 同时含 `tickId` 与 `tick_id`（`tick.go:57-58`）；错误封装类型 `response` 也未导出（`internal/control/response.go:29-34`）。监视器无法复用，只能复制字段；草案 K1.8:73 的字段清单已经**漏掉 `tick_id`**。
- 红队结论: **击穿**。三处载荷（control `/tick`、monitor `/tick`、WS tick 帧）会各自漂移；V1 的"逐字段一致"没有编译期或测试期保证。
- 建议处置: C24。把 tick 载荷构造函数提取到共享包（或 `internal/control` 导出），三处调用同一函数 + parity 测试。
- 建议严重级: 中

### A2-8 用 AddChangeListener 不会与 events.Bridge 的 OnChange 冲突（未击穿）

- 攻击点: K2.4:132 用 `Entity.AddChangeListener` 而非 `OnChange` 是否与 `internal/events` 打架。
- 源码事实: `Entity` 同时有单值 `OnChange` 与 `listeners` 切片（`internal/entity/model.go:33-34`）；`Bridge.Attach` 独占 `OnChange`，已非空则跳过（`internal/events/bridge.go:57-63`）；`Set` 先调 `OnChange` 再遍历 listeners（`model.go:148-155`）。监视器只读 `listeners` 不写 `OnChange`，二者互不影响。
- 红队结论: **未击穿**。前提是监视器**不得**改 `OnChange`（否则 Bridge 的校验/@事件会静默失效）。
- 建议处置: 无需新约束；在实现说明里写死"monitor 只用 AddChangeListener"。
- 建议严重级: 中

### A3-1 mcpserver ↔ monitor 的 import cycle

- 攻击点: K2.4:135「包一层 `runTickLocked` 的调用点（mcpserver）」；同时 K1.8:72 与 K1.9:80 让 monitor 调 `mcpserver.TryRunTick` / `WithStateWrite`。
- 源码事实: `internal/mcpserver` 是共享包（`bootstrap.NewMCPServer` 使用，`internal/bootstrap/bootstrap.go:125-133`），没有观察者接口；若在 `runTickLocked` 里直接调用 monitor 的函数或引用 monitor 类型，则 `mcpserver → monitor`；而 monitor 为 tick/写又要 `monitor → mcpserver`。Go 禁止包级循环。
- 红队结论: **击穿**（方案级）。K2.4 的"包一层调用点"在没有中性接口时无法落地，编译器会直接拒绝。
- 建议处置: C3。在 `mcpserver` 定义 `TickObserver`/`SetTickObserver(func(engine.TickResult))`（类型属于 mcpserver，不引用 monitor），monitor 单向 import mcpserver，接线只在 `cmd/`。
- 建议严重级: 阻断

### A3-2 diagnostics ↔ monitor 的 import cycle

- 攻击点: K2.4:134「新增 `SetNotify(func(Record))`」没有规定钩子在哪个包定义、由谁引用。
- 源码事实: monitor 必须 import `internal/diagnostics` 才能调 `Snapshot/Summary/Generation`（`store.go:113-156`）；若为了"发帧"把回调实现放进 diagnostics 并让它 import monitor，或把钩子类型定义成 `func(*monitor.Event)`，就形成 `diagnostics → monitor` 与 `monitor → diagnostics` 双向依赖。
- 红队结论: **击穿**（草案未定方向）。唯一合法方向：钩子类型 `func(diagnostics.Record)` 由 diagnostics 自己定义，monitor 负责赋值；diagnostics 不 import monitor。
- 建议处置: C3（与 A3-1 合并为同一方向约束）。
- 建议严重级: 阻断

### A3-3 当前包图无环，monitor 不依赖 control/bootstrap（未击穿）

- 攻击点: 监控实现是否会与现有包形成环。
- 源码事实: `internal/control` 未导出 response/tickBody（`response.go:29-34`、`tick.go:55-66`），monitor 无法直接 import 复用，只能走新建共享包或复制；`bootstrap` 是装配层（`bootstrap.go:109-133`），不会被 monitor 反向依赖；当前仓库没有 `internal/monitor` 包（`ls internal` 无该项），现有包图无环。
- 红队结论: **未击穿**。但"无法复用 control 的未导出实现"会把实现推向复制（见 A2-7），环的风险集中在新钩子上（A3-1/A3-2）。
- 建议处置: 无需独立约束；C3 已覆盖钩子方向。
- 建议严重级: 中

### A4-1 保存失败不回滚：内存与磁盘静默分叉

- 攻击点: K1.9:80-82「`WithStateWrite` 包裹 `ent.Set` + `store.Save`」「写失败/保存失败 500」「成功 `saved:true`」；00-scope V3 同义。
- 源码事实: `Set` 在 `Save` 之前已改内存（`internal/entity/model.go:126-157`）；`Save` 先写 public 再写 user，两文件不是一次原子操作（`internal/entity/store.go:116-140`，写入分别在 136 与 139）；`atomicWrite` 只保证单文件 rename（`store.go:171-180`）。
- 反例（待实现后执行）:
  ```
  mkdir -p /tmp/abx && mkdir /tmp/abx/entities.json      # 让 rename 必败
  # 起服务后 POST set 猫猫.状态.情绪=开心 → 500
  curl -s .../monitor/api/entities/猫猫 | jq .entity     # 仍是"开心"（内存已改）
  # 重启进程 → 回到写前值（磁盘没改）；若写的是 NPC，还可能 public 已更新、user 未更新
  ```
- 红队结论: **击穿**。草案没有回滚或 dirty 语义：客户端看到 500 却无法知道内存已生效；WS 若仍发 delta，页面与磁盘/重启后状态不一致；`saved:true` 在成功响应里无条件出现也不可靠。
- 建议处置: C5。失败时用 oldValue 回滚（或返回 `dirty:true` 并在契约写明），成功后 `saved` 只能来自真实 `Save` 返回。
- 建议严重级: 高

### A4-2 写路径无界阻塞：gate 被占时 HTTP 永久挂起

- 攻击点: K1.9:80「`WithStateWrite` 包裹」+ K1.8:72「`TryRunTick`」。两者对争用的处理不对称：tick 用 `TryLock` 立即 409，set 用 `Lock` 阻塞。
- 源码事实: `WithStateWrite` 是 `s.tickMu.Lock()` 阻塞获取（`internal/mcpserver/runTick.go:61-64`）；`TryRunTick` 用 `TryLock`（`runTick.go:27-34`）；control 有 5s 服务端 deadline（`internal/control/server.go:106-120`），但那是 control 自己的 `dispatchWithinDeadline`，monitor 路由不走它。
- 反例（待实现后执行）:
  ```
  # 复用 control/harness_test.go:107-130 的 holdStateGate 占住 tickMu
  timeout 3 curl -s -o /dev/null -w '%{http_code}\n' -X POST .../monitor/api/entities/猫猫/set -d '{"path":"x","value":1}'
  # 期望：2s 内 409/503；现状：timeout 3 杀掉 curl，服务端 handler 仍挂着
  ```
- 红队结论: **击穿**。长 tick 或卡死的写会让页面写请求无限挂起；客户端超时重试会叠加排队写，gate 恢复后连续落盘多次。
- 建议处置: C6。set 用带超时的获取（如 `TryLock` + 重试预算）或 ctx 取消，超时返回 409/503；tick 必须把 `acquired=false` 映射为 409 `tick_busy`。
- 建议严重级: 高

### A4-3 maxBytes 语义未定义，截断不可判定

- 攻击点: K1.2:32-34、K1.3:38-41。
- 源码事实: control 的同类实现给出了参考语义：`bytes` = `json.Marshal` 后长度，超限时 `data:null` 且 `truncated:true`（`internal/control/state.go:54-72`）；path 模式下 `bytes` 只计 `values`（`state.go:74-98`）。草案 K1.2 说截断时 items 只含 id/type/keys，但没说：`bytes` 计什么、按实体还是按字段截断、`keys` 本身超限怎么办、`truncated` 后 `data` 是 null 还是前缀。
- 红队结论: **击穿**。同一请求不同实现都能自称合规，V2 无法判定；页面无法区分"空实体"与"被截断"。
- 建议处置: C15。写死计量单位、截断粒度、超限字段的降级表示与 `data` 取值，并用 json.Valid + 长度上限测试钉死。
- 建议严重级: 中

### A4-4 日志读取边界未定义：symlink / 半行 / 整文件读 / 无字节上限 / 当日无日志

- 攻击点: K1.7:61-67。
- 源码事实: `StateLogger.Log` 在 `Transitions` 与 `Conflicts` 都为空时**直接 return，不写任何行**（`internal/engine/stateLogger.go:30-33`）；文件名 `state_2006-01-02.log`（`stateLogger.go:51`），保留 7 天后删除（`store.go` 同目录的 `pruneLocked`，`stateLogger.go:64-89`）；control 日志是"RFC3339 METHOD path status 耗时"纯文本（`internal/control/logger.go:84-90`），不是 JSON；两份日志共用 `<dataDir>/logs`（`bootstrap.go:117`、`control/server.go:75`）。草案没有规定符号链接、单行上限、读取方式（尾部 seek 还是整文件）、半截行、响应字节上限。
- 反例（待实现后执行）:
  ```
  ln -s /etc/passwd data/logs/state_2026-10-26.log
  curl -s '.../monitor/api/logs?kind=state&date=2026-10-26'   # 若直接 os.ReadFile → 泄露目标文件
  # 10MiB 日志 + ?limit=2000：若整文件读入并全量返回，内存/响应无上限
  # 写入方正在 append 时读取：最后一行可能无 \n，返回半行 JSON
  ```
- 红队结论: **击穿**。安全（symlink/穿越）、稳定性（大文件）、正确性（半行/空日）三处都没有可执行判据；"当日无 tick 就没有文件"与"文件不存在 → 200 空数组"的语义也需写死。
- 建议处置: C13。解析后重组路径、Lstat 拒绝 symlink、尾部有界读取、只认完整行、响应字节上限；空日返回 200 空数组并在契约写明。
- 建议严重级: 高

### A4-5 WS snapshot 与 delta 的 seq 边界未定义（重复/漏更新）

- 攻击点: K2.2:107-108 的 `snapshot.seq` 与 `delta.seq`、K2.3:125-127 的重连语义。
- 源码事实: 草案没有规定"先订阅注册、后拍快照"还是反之；没有规定 snapshot.seq 与并发 delta 的先后；也没有规定客户端收到 seq ≤ snapshot.seq 的 delta 时应丢弃。服务端若在拍快照期间把 delta 入队，客户端可能先应用 delta 再收到旧快照，画面回退。
- 红队结论: **击穿**。同一连接上"快照 + 增量"没有可判定的拼接规则；断线重连场景会更明显（A1-5）。
- 建议处置: C9。规定 snapshot 覆盖到 seq=S，之后只允许 seq>S 的帧；客户端丢弃 seq≤S；测试用并发写在订阅窗口内制造竞态。
- 建议严重级: 高

### A5-1 DNS rebinding：`Origin == Host` 自比自，挡住不攻击者

- 攻击点: K2.1:98「若请求带 Origin，必须与 Host 同源（scheme+host+port）」；K3:142「写请求校验 Origin 与 Host 同源」。该判据假定攻击者无法同时控制 Origin 与 Host。
- 源码事实: SDK 的 MCP handler 自带回环保护：监听地址是回环、`req.Host` 不是回环时 403（`streamable.go:249-259`，依赖 `http.LocalAddrContextKey`）；monitor 路由不经过该 handler，**不继承**此保护（A1-2）。DNS rebinding 下，恶意页面把自己解析到 `127.0.0.1:8765`，浏览器发出的 Host 与 Origin **都是**攻击者域名，同源比较恒真；REST 读又因同源而可读，WS 完全不受 SOP 限制。
- 反例（待实现后执行）:
  ```
  # 攻击者页面 http://evil.example:8765 重绑定到 127.0.0.1
  curl -s -o /dev/null -w '%{http_code}\n' -H 'Host: evil.example:8765' http://127.0.0.1:8765/monitor/api/entities
  # 回环无 token 形态下若返回 200 → 任意网页可读全部状态；WS 同理可订阅
  ```
- 红队结论: **击穿**。回环无 token 是 K0 形态 1 的主推场景（"用户本机没配 token 时页面只能看"），恰好是该攻击的靶子。Origin 校验只能防跨站，防不住 rebinding。
- 建议处置: C8。所有 `/monitor/*` 在回环监听时必须校验 Host ∈ 回环名/IP（与绑定地址一致），不匹配 403；WS 的 Origin 精确匹配且拒绝 `null`；不输出 CORS 头。
- 建议严重级: 阻断

### A5-2 WS token 载体与浏览器/协议不兼容，且 query token 会泄漏

- 攻击点: K2.1:95-97「首选 `Sec-WebSocket-Protocol: astrbox.token.<token>`，客户端同时在 `token` query 兼容」；K3:142「token 只从 header/WS 子协议/query 取」。
- 源码事实: `http_token` 来自配置且**无任何字符集校验**（`internal/config/config.go:24-25`；`Config.Validate` 只校验领域字段，`config.go:67-89`）。RFC6455 子协议值必须是 token 字符；token 含空格/`=`/中文时浏览器 `new WebSocket(url, [protocol])` 直接抛 `SyntaxError`，连接根本不会发出。服务端还必须回显选中的子协议，否则握手失败。query token 会进入浏览器历史、Referer 与任何中间日志（草案只担心"访问日志"，没覆盖 Referer）。
- 反例（待实现后执行）:
  ```
  # config: {"http_token":"a b=c"}；页面 JS: new WebSocket(url, ["astrbox.token.a b=c"])
  # → 浏览器抛 SyntaxError: The string contains invalid characters
  ```
- 红队结论: **击穿**。要么限制 token 字符集并在启动时校验，要么对 token 做 base64url 编码；同时必须规定"服务端回显子协议"和"query 只在无 Referer 泄漏风险时使用"。
- 建议处置: C7。token 字符集校验（建议 `^[A-Za-z0-9_-]{16,128}$`）+ 子协议用 base64url + 服务端 `Subprotocols` 回显 + 测试三件套。
- 建议严重级: 高

### A5-3 K1.6 未指定带护栏的脚本读取，symlink/超大文件可绕过

- 攻击点: K1.6:55-59（`source=true` 返回脚本文本，>256KiB → 413），未声明数据源。
- 源码事实: `registry.ReadScript` 已经做了：拒绝符号链接（`internal/registry/scriptFiles.go:52-58`）、`MAX_SCRIPT_BYTES=256KiB` 上限（`scriptFiles.go:11-12,59-61`）、`safeSourcePath` 路径越界校验 + `validateName`（`scriptFiles.go:200-221`、`registry.go:96,492-500`）。若 handler 图省事直接 `os.ReadFile(filepath.Join(scriptsDir, name))`，symlink 与超大文件都能读；`?source` 的 413 也就成了摆设。
- 红队结论: **击穿**（契约没把安全实现钉住）。同名脚本的 `ReadScript` 错误/size 语义必须映射为 404/413，否则脚本名含中文/大写时路由与文件读取各走一套规则，容易出现 404 与读取成功并存。
- 建议处置: C16。K1.6 必须写"数据源 = `registry.ReadScript`"，并加 symlink/超限测试。
- 建议严重级: 高

### A5-4 审计日志无轮转，长期运行写满磁盘

- 攻击点: K3:143「每次写操作追加 `data/logs/monitor-audit-YYYYMMDD.log`」。
- 源码事实: 现有两个 prune 只清理自己的前缀：`StateLogger.pruneLocked` 只删 `state_*.log`（`internal/engine/stateLogger.go:64-89`）；control 的 `dailyLogger.pruneLocked` 只删 `dsh-control-*.log`（`internal/control/logger.go:63-81`）。`monitor-audit-*` 不在任何清理范围；草案也没写保留天数/单行上限。
- 红队结论: **击穿**。监视器是"常开"组件，审计随每次写增长且永不删除；磁盘写满会影响 `store.Save`（tick 也会失败），从审计问题升级为数据可用性问题。
- 建议处置: C23。审计日轮转 + 保留 7 天 + 单行上限 + 不记录 token；写失败不影响响应（草案已要求，保留）。
- 建议严重级: 中

### A5-5 health 暴露 dataDir/scriptsDir/pid（未击穿）

- 攻击点: K1.1:26-28 返回绝对路径与 pid。
- 源码事实: 非回环监听无 token 已拒绝启动（`main.go:32-34`）；回环无 token 属于"本机同用户信任边界"，本机进程本来就能读 data-dir（MCP 工具即可）；且 C8 修复后浏览器侧也被 Host 校验挡住。
- 红队结论: **未击穿**。信息面可接受；但若后续允许非回环只读或加 CORS，必须重新评估。
- 建议处置: 无需新约束；在 C8 中保留 Host/Origin 判据作为前置。
- 建议严重级: 中

### A6-1 K4 的 lint 门禁与门禁自身默认布局不符，且不扫描真实前端源码

- 攻击点: K4:148-151「真源 `design/tokens.json`……门禁 `lint-ui-plan.sh --plan-dir design`」；00-scope V6 还写成 `bash <ui-composition>/scripts/...`。
- 源码事实: `lint-ui-plan.sh` 的默认令牌路径是 `<plan-dir>/tokens/tokens.json`（`~/.dsh/skills/ui-composition/scripts/lint-ui-plan.sh:24`），元件/组合默认 `<plan-dir>/components.json`、`<plan-dir>/composition.json`（24-26）；`--src-root` 缺省 = 方案目录（27），即源码扫描面也落在 `design/`。按 K4 的命令，`design/tokens/tokens.json` 不存在 → 退出 2（IO 错误）；即使补上 `--tokens`，前端源码 `internal/monitor/web/` 也不在扫描面，"页面分解可机检"实际只校验三份 JSON。
- 反例（已核脚本行为，待实现后执行）:
  ```
  bash ~/.dsh/skills/ui-composition/scripts/lint-ui-plan.sh --plan-dir design
  echo $?   # 现状：exit 2（design/tokens/tokens.json 缺失）
  ```
- 红队结论: **击穿**。验收命令要么跑不起来，要么验的不是页面源码；4 条反例也没法证明"组合层造原子/硬编码样式"会被拦住。
- 建议处置: C17。固定完整命令（显式 `--tokens`、`--src-root internal/monitor/web`），并用"在真实前端文件里塞一个硬编码色值必须 exit 1"的反例证明扫描面有效。
- 建议严重级: 阻断

### A6-2 `go:embed` 缺生成物即编译失败，K4 二选一未裁决

- 攻击点: K4:149「前端静态资源放 `internal/monitor/web/`（`go:embed`）」+ 152-153「`make test` 不依赖生成物已存在（或生成物进版本库，二选一，待红队攻击）」。
- 源码事实: 仓库当前没有 `internal/monitor/`（`ls internal`），也没有任何 `go:embed`（`grep -rn "go:embed" internal cmd` 无命中）；Go 的 embed 指令在匹配不到文件时编译失败（`pattern all: no matching files found`）。`go test ./...` 会编译该包，因此"测试不依赖生成物"不成立，除非把生成物提交进仓库。
- 红队结论: **击穿**。未裁决的二选一让"干净工作树能构建/测试"不可判定；选择"生成物进版本库"又必须证明生成可复现，否则设计真源与 embed 物会漂移。
- 建议处置: C18。生成物入版本库 + `make design` 可复现（diff 为空）+ 干净树可编译。
- 建议严重级: 阻断

### A6-3 Makefile 不把 design/web 资产列为前置，修改后不重建；也没有版本注入

- 攻击点: K4:152「`make build` 内含 design → web 的生成步骤」；K1.1:26 返回 `version`。
- 源码事实: `Makefile:16-24` 的 `$(ENGINE_BIN)` 只依赖 `$(GO_SOURCES)`（`find cmd internal -name '*.go'`）与 `go.mod/go.sum`；`design/` 与 `internal/monitor/web/**` 都不是前置，改 tokens/CSS/JS 后 `make build` 认为已最新；`Makefile` 也没有 `-ldflags`，仓库内不存在构建版本变量（`mcpserver/server.go:58` 的 Version 是硬编码 "1.0.0"，`cmd/astrbox-mcp/main.go` 无 version）。
- 反例（待实现后执行）:
  ```
  touch design/tokens.json && make -q build; echo $?      # 期望 1（需要重建）；现状 0
  touch internal/monitor/web/index.html && make -q build; echo $?   # 同上
  ```
- 红队结论: **击穿**。交付物会陈旧；`health.version` 没有真实来源，V7 的"单二进制内含页面"也可能拿到旧页面。
- 建议处置: C19 + C20。把设计真源与 web 资产加入 build 前置；`-ldflags -X main.buildVersion=$(git rev-parse --short HEAD)`。
- 建议严重级: 阻断

### A6-4 V6 判定命令含占位符，V7 无判定命令

- 攻击点: 00-scope:27「`bash <ui-composition>/scripts/lint-ui-plan.sh --plan-dir design`」；V7「`make build` 后 `bin/astrbox-mcp` 内含页面与静态资源；无 node 依赖」没有给命令。
- 源码事实: `<ui-composition>` 是字面占位符，不能复制执行；V7 的"内含页面"需要用 `strings`/`go tool nm` 或运行页面请求判定，草案没写。
- 红队结论: **击穿**。验收命令不可执行，V6/V7 无法复算。
- 建议处置: C20。所有 V*/K* 判定必须是无占位符的字面命令；V7 建议用 "strings 命中 index.html 特征 + 无 node 进程/依赖检查" 或 "起服务后 curl 页面 200"。
- 建议严重级: 中

### A6-5 WebSocket 依赖不在 go.mod/go.sum/模块缓存，离线构建必失败

- 攻击点: K2.1:94「引入 `github.com/coder/websocket`」。
- 源码事实: `go.mod:1-14` 与 `go.sum` 没有该依赖；模块缓存 `.local/gomodcache/github.com/` 下只有 golang-jwt/google/modelcontextprotocol/segmentio/yosida95（**已核** `ls` 输出），没有 coder。`Makefile:6-8` 把 `GOPROXY` 之外的缓存指向 `.local/`，默认缓存可能只读；一旦代码 import，离线/受限网络下 `make build` 直接失败。
- 红队结论: **击穿**。V7 的"单 `make build` 产出"在当前环境下不成立，除非实现前完成 `go get` 并提交 go.mod/go.sum（或改为零依赖手写，但那会引入 U2 的分片/掩码/关闭握手风险）。
- 建议处置: C21。二选一裁决（库 vs 手写），库方案必须提交 go.mod/go.sum 并验证 `GOPROXY=off make build`。
- 建议严重级: 阻断

### A6-6 V8 回归门禁无命令，且 lint 依赖 `$HOME` 绝对路径

- 攻击点: 00-scope:29 V8「`make test`、`make lint` 绿；现有 MCP 工具清单/控制面契约不变」。
- 源码事实: `scripts/doctorDsh.mjs` 已存在并断言 17 个工具（`scripts/doctorDsh.mjs:3,161-171,237`），`Makefile:34-36` 有 `doctor` 目标，但 V8 没把它写成判定；`Makefile:22-23,31-32` 中 lint 依赖 `$(HOME)/.dsh/skills/code-modularity/scripts/lintCodeModularity.sh`，换用户/CI 环境会直接失败。
- 红队结论: **击穿**。回归判据不可复算；`make doctor` 未被纳入验收，"工具清单不变"没有机器判定。
- 建议处置: C22。V8 明确写 `make test && make lint && make doctor`；lint 脚本路径改为可配置（环境变量/仓库内）。
- 建议严重级: 阻断

## 越界发现（只提交，不处理）

| # | 位置 | 最小复现 | 影响面 | 建议 |
|---|---|---|---|---|
| 1 | `internal/engine/engine.go:543-558` + `internal/entity/model.go:40-52,146` | 100 实体连续 tick 1000 次后检查各实体 `listeners` 长度 ≈1000（引擎每 tick 注册、移除只置 nil、不压缩；`Set` 每次复制整个切片） | 长驻进程内存与写延迟随 tick 数线性增长；监视器"常开页面"把它暴露成用户可见退化 | 由引擎侧修：移除时压缩（swap-remove 或计数式），或改 `OnChange` 复数化；**不属于 web-monitor 写域，红队不处理** |
| 2 | `cmd/astrbox-mcp/main.go:23-29`（对比 `cmd/dsh-mcp/main.go:68-79`、`cmd/dsh-mcp/lock.go:36-56`） | 先起 `dsh-mcp` 持有 `.engine.lock`，再起 `astrbox-mcp -transport http`：后者正常启动，两个引擎写同一 data-dir | 已有 HTTP 模式即可与 DSH 插件并发写同一 data-dir，last-writer-wins 丢更新；监视器会让用户更常用该模式 | 把锁逻辑提取为共享包，`astrbox-mcp` 启动时取同一 flock；或明确只允许一个入口 |
| 3 | `internal/control/state.go:45-52,114-125` vs `internal/mcpserver/server.go:128-132` | 模板新增字段后，`/state?path=模板字段` 返回 missing，MCP `entity_state_get` 返回模板值 | 两个既有入口语义已不一致；监视器若"照抄 control"会把不一致固化 | 二选一统一（推荐 `/state` 与 MCP 都走 `ent.Get` 或都走 Snapshot），由 Lead 裁决；红队不处理 |

## 已撤回（反挑战通道）

| 攻击 | 质疑方 | 撤回原因（无法给出可复现反例） |
|---|---|---|
| 无 | — | — |

## 待观察假设（写不出判定方法的条目落这里，不计入约束）

| # | 假设 | 为何无法判定 | 何时复核 |
|---|---|---|---|
| H1 | 页面实际可渲染、可交互（点选实体、看日志、写字段成功） | 无 node/浏览器自动化；`lint-ui-plan` 只校验方案 JSON 与源码静态面，不能证明渲染与交互。V6 只能判"分解合规"，不能判"页面可用" | 接入浏览器自动化（Playwright/手工清单）或用户人工验收时；复核前不得宣称"页面可用" |
| H2 | 慢客户端下"丢 delta 后补 snapshot"的用户可接受性 | 丢帧后的体感与"snapshot 跳变"是否可接受属主观体验，无机器判据 | 真实浏览器 + 弱网/后台标签页压测后 |
| H3 | 审计日志字段的敏感边界（`value`/`path` 是否要脱敏、是否算凭据） | 需用户按数据性质裁决；红队无权定义"敏感" | 用户裁决数据分级时 |
| H4 | tick ring 容量 N=20 是否满足"最新 tick 结果"的产品期望 | 容量是产品取舍，非对错；无客观判据 | 用户试用后 |
| H5 | 30s ping / 90s pong 在浏览器后台标签页（定时器被节流）下不会误断 | 需要真实浏览器后台节流环境；纯 Go 测试无法模拟标签页降频 | 浏览器实测（后台挂 30 分钟）后 |

> 撤回/降级规则：能给出可复现反例的攻击不得降级；H1-H5 若后续拿到可执行判定，应升级为 C 条目并重跑对应攻击面。
