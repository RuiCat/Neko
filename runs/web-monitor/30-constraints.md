# 冻结约束清单（web-monitor）

- 创建: 2026-10-26T20:18:35+08:00
- 完成: 2026-10-26T20:55:00+08:00
- 来源: `20-attack-log.md` 的有效攻击（击穿条目）
- 冻结: `plan.sh freeze` 写入哈希；冻结后每条 C 的**状态**是 G3 收敛判定的唯一依据
- **状态说明**: 本文件是 G2 红队产物。经 ★裁决2（2026-10-26）用户裁决 **C1–C24 全部接受**，故每条状态已由「待裁决」改为 `满足`；
  「满足」的检测证据在 G4 实现后回填 `60-verification.md`，本文件保留源码锚点与判定命令作为验收口径。
- **判定命令说明**: 标注【已核】的是红队本轮实际执行过的只读核对（输出在括注里）；其余命令待实现后执行，红队本轮未跑 `make build`/`make test`，不得把"命令已写出"当作"已通过"。
- **模块映射说明**: 红队按草案章节推断的旧归属（m1 承载 / m2 鉴权 / m3 tick 与写 / m4 只读数据面 / m5 实时通道 / m6 前端交付 / m7 回归门禁）**不是最终谱系**。
  最终谱系为候选 B 的 m1 契约冻结 / m2 令牌层 / m3 后端实现 / m4 元件契约 / m5 元件实现 / m6 组合装配 / m7 门禁文档验收；
  **每条 C 的最终归属以 `40-plan.md` 的「约束归属」表为准**（本文件归属列保留红队原始推断，不改判定与断言）。

## 约束清单

### C1 | 同一 data-dir 只允许一个可写引擎：监视器必须与被监视引擎同进程，或持有同一 `.engine.lock`

- 断言: 启动监视器/引擎的进程必须持有 `<data-dir>/.engine.lock`；锁被其他进程持有时必须拒绝启动。不得出现两个可写引擎进程同时服务同一 data-dir。
- 判定: `./bin/dsh-mcp -data-dir /tmp/abx &` 后 `./bin/astrbox-mcp -data-dir /tmp/abx -transport http -addr 127.0.0.1:18765`，后者必须非 0 退出且 stderr 含 `.engine.lock`；反方向 `./bin/astrbox-mcp` 持有锁时 `./bin/dsh-mcp` 退出码必须为 3。若改为把监视器挂进 `dsh-mcp`，则判定改为 `go test ./cmd/dsh-mcp -run TestMonitorSameRuntimeAndLock`。
- 严重级: 阻断
- 归属模块: global
- 攻击来源: A1-1（另有越界发现 #2）
- 状态: 满足
- 证据: `cmd/dsh-mcp/main.go:68-79`（取锁）、`cmd/dsh-mcp/lock.go:15-16,36-56`（flock 实现）、`cmd/astrbox-mcp/main.go:23-29`（无锁、独立 runtime）、`dsh/cordis.patch.yml:17`（实际 MCP 命令是 `bin/dsh-mcp`）、`internal/entity/store.go:116-140`（整体重写 Save）
- 降级理由: -
- 残留影响: 若降级：监视器与 MCP 两个引擎并发写同一 data-dir，last-writer-wins 丢更新、读到陈旧状态、WS 收不到另一进程的写入，V3/V4 均不成立。
- 复核触发: 承载形态（U1）改动；MCP 入口进程变化；新增第二个写 data-dir 的入口。

### C2 | `store.OnCreate` 回调不得调用 Store 任何加锁方法

- 断言: 监视器的 `OnCreate` 回调只允许读传入的 `*entity.Entity`（ID/Type/Snapshot/AddChangeListener）；不得调用 `Store.Get/GetOrRaise/ListAll/Count/ListTemplates/GetTemplate/Templates/Save/Create/Delete/Query*/SetPath`。
- 判定: `go test ./internal/monitor -run TestOnCreateCallbackNoStoreCall -timeout 15s` 通过；把回调改为调用 `store.ListAll()` 时该测试必须超时失败（证明测试有牙）。辅助静态检查：`grep -rn "OnCreate =" internal/ cmd/ | grep -v _test` 的监视器包装处不出现 `store\.` 调用。
- 严重级: 阻断
- 归属模块: m5
- 攻击来源: A2-1
- 状态: 满足
- 证据: `internal/entity/store.go:261,271-273`（Create 持写锁调用 OnCreate）、`internal/events/system.go:27`（单赋值）、`internal/entity/store.go:22-23`（无链式 API）
- 降级理由: -
- 残留影响: 若降级：任一 `entity_create` 即可能永久阻塞进程（store 写锁不可重入）。
- 复核触发: `Store` 增加链式注册 API 或把 OnCreate 移出写锁后，可重新评估。

### C3 | 新增观察/通知钩子只能"被观察包定义类型、监视器赋值"，被观察包不得 import monitor

- 断言: `internal/mcpserver`、`internal/diagnostics`、`internal/bootstrap` 不得 import `internal/monitor`；tick 观察者与诊断通知的类型定义在被观察包内（`func(engine.TickResult)` / `func(diagnostics.Record)`）；接线只在 `cmd/` 与 monitor 构造处。
- 判定: `go build ./...` 无 import cycle（编译失败即违反）；`grep -rn 'astrbox/internal/monitor' internal/mcpserver internal/diagnostics internal/bootstrap` 无输出；`go list -f '{{join .Imports "\n"}}' ./internal/mcpserver ./internal/diagnostics ./internal/bootstrap | grep -c 'internal/monitor'` = 0。
- 严重级: 阻断
- 归属模块: global
- 攻击来源: A3-1、A3-2
- 状态: 满足
- 证据: `internal/mcpserver/runTick.go:36-54`（唯一 tick 汇点）、`internal/diagnostics/store.go:31-57,113-163`（无钩子）、`internal/bootstrap/bootstrap.go:125-133`（共享装配层）
- 降级理由: -
- 残留影响: 若降级：Go 包级循环直接编译失败，方案不可实现。
- 复核触发: 钩子挪到独立共享包（如 `internal/observe`）时重评。

### C4 | tick 观察者必须非阻塞且不得在 `tickMu` 内做重活/重入

- 断言: tick 观察者在 `runTickLocked` 路径上只做非阻塞入队（或移到解锁后）；观察者内不得调用 `RunTick/TryRunTick/WithStateWrite/Store.Save`；观察者阻塞不得把 tick 墙钟拖长到阈值以上（建议 100ms）。
- 判定: `go test ./internal/monitor -run TestTickObserverNonBlocking -race`：观察者 `sleep 500ms` 时 `RunTick` 墙钟 < 100ms 且本轮 tick 仍进入监视记录；`grep -rn "OnTick" -A 20 internal/mcpserver` 中无 `RunTick|WithStateWrite|Save` 调用。
- 严重级: 高
- 归属模块: m3
- 攻击来源: A2-2
- 状态: 满足
- 证据: `internal/mcpserver/runTick.go:36-54,56-65`（gate 与"不可重入"注释）、`internal/mcpserver/diagnosticsTools.go:81-110`（现有锁内钩子的样子）
- 降级理由: -
- 残留影响: 若降级：慢观察者拉长所有写请求；重入直接死锁。
- 复核触发: 观察者改为异步 worker 后可放宽。

### C5 | 保存失败必须回滚或显式 dirty，不得静默分叉

- 断言: `/monitor/api/entities/{id}/set` 在 `store.Save` 失败时，要么回滚内存值到写前值并返回 500，要么返回 `ok:false` 且带 `dirty:true` 并声明"内存已生效、磁盘未生效"；不得返回普通成功或 `saved:true`。成功响应的 `saved:true` 只能来自真实 `Save` 成功。
- 判定: `go test ./internal/monitor -run TestSetSaveFailureRollback`：把 `<dataDir>/entities.json` 置为目录制造 rename 失败；断言 POST 返回 500、之后 GET 值等于写前值（回滚）**或**响应含 `dirty:true`；把 store 从磁盘重载后值一致（无静默分叉）。
- 严重级: 高
- 归属模块: m3
- 攻击来源: A4-1
- 状态: 满足
- 证据: `internal/entity/model.go:126-157`（先改内存）、`internal/entity/store.go:116-140,171-180`（两文件非原子、单文件 rename）
- 降级理由: -
- 残留影响: 若降级：500 后内存/磁盘/重启三方不一致；NPC 写入可能只落一半文件。
- 复核触发: `Store.Save` 改为跨文件事务或临时文件+两次 rename 后可重评。

### C6 | gate 争用必须有界：tick 409、set 有超时

- 断言: `/monitor/api/tick` 在 `TryRunTick` 返回 `acquired=false` 时必须 409 `tick_busy`，不得返回 200 空结果；`/monitor/api/entities/{id}/set` 在 gate 被占用超过契约阈值（建议 2s）时必须返回 409 或 503，不得无界阻塞。
- 判定: 复用 `internal/control/harness_test.go:107-130` 的 `holdStateGate` 模式：占住 gate 后 `timeout 3 curl -s -o /dev/null -w '%{http_code}' -X POST .../monitor/api/entities/猫猫/set` 必须在 3s 内返回 409/503；同一状态下 POST `/monitor/api/tick` 返回 409 且 body `error.code == "tick_busy"`。
- 严重级: 高
- 归属模块: m3
- 攻击来源: A4-2
- 状态: 满足
- 证据: `internal/mcpserver/runTick.go:27-34,61-64`（TryLock vs Lock）、`internal/control/server.go:106-120`（control 有 deadline，monitor 没有）、`internal/control/harness_test.go:107-130`
- 降级理由: -
- 残留影响: 若降级：长 tick/卡死写会让页面写请求永久挂起，超时重试叠加排队。
- 复核触发: 引入请求级 deadline 或异步写队列时重评。

### C7 | 浏览器可用鉴权矩阵：静态免 token、API 头 token、WS 子协议 token、无 token 只读

- 断言: ①`/monitor/` 与静态资源不要求 token（否则页面打不开）；②`/monitor/api/*` 要求 `X-Auth-Token`/`Authorization: Bearer`（无 token 配置的只读形态除外）；③`/monitor/ws` 在 `tokenMiddleware` **之前**分流，token 从 `Sec-WebSocket-Protocol: astrbox.token.<base64url>` 或 query 取，服务端必须回显选中的子协议；④`http_token` 启动时校验字符集（建议 `^[A-Za-z0-9_-]{16,128}$`），不合法拒绝启动或 base64url 编码；⑤无 token 形态写接口返回 403 `write_disabled`。
- 判定: `go test ./internal/monitor -run TestAuthMatrix` 覆盖 5 项；补充命令（token=t 起服务）：`curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8765/monitor/` = 200；同 URL `/monitor/api/entities` 无头 = 401、带 `X-Auth-Token: t` = 200；WS 子协议握手 = 101（Go client）；无 token 配置时 `curl -X POST .../monitor/api/tick` = 403 且 `error.code == "write_disabled"`。
- 严重级: 阻断
- 归属模块: m2
- 攻击来源: A1-3、A5-2
- 状态: 满足
- 证据: `cmd/astrbox-mcp/httpAuth.go:10-27`（只读 header）、`internal/config/config.go:24-25,67-89`（token 无校验）、`15-interfaces.draft.md:16-17,95-98,140-142`
- 降级理由: -
- 残留影响: 若降级：配置 token 后页面 401 或 WS 401，写操作在浏览器里不可用；token 含非法字符时 WS 抛 SyntaxError。
- 复核触发: 引入 cookie/会话或页面 token 输入方案变化时。

### C8 | 反 DNS rebinding 与跨源：Host 白名单 + WS Origin 精确匹配 + 无 CORS 头

- 断言: 回环监听时，所有 `/monitor/*` 必须拒绝 Host 不属于 {`127.0.0.1[:port]`, `localhost[:port]`, `[::1][:port]`} 的请求（403）；WS 若带 Origin，必须精确等于 `http://`+Host（不得用前缀/包含匹配），`Origin: null` 或异源 403；任何 `/monitor/*` 响应不得包含 `Access-Control-Allow-Origin`。
- 判定: 回环服务起后：`curl -s -o /dev/null -w '%{http_code}' -H 'Host: evil.example:8765' http://127.0.0.1:8765/monitor/api/entities` = 403；`curl -si -H 'Origin: http://evil.example' http://127.0.0.1:8765/monitor/api/entities | grep -c 'Access-Control-Allow-Origin'` = 0；`go test ./internal/monitor -run TestWSOriginRejected`（Origin 异源/null → 403）。
- 严重级: 阻断
- 归属模块: m2
- 攻击来源: A5-1
- 状态: 满足
- 证据: `15-interfaces.draft.md:98,142`（Origin==Host 判据）、`streamable.go:249-259`（SDK 自带 Host 校验，monitor 路由不继承）、`cmd/astrbox-mcp/httpAuth.go:29-43`（回环判据可复用）
- 降级理由: -
- 残留影响: 若降级：任意网页经 DNS rebinding 可在无 token 形态下读取全部实体状态并订阅 WS。
- 复核触发: 支持非回环部署或反向代理时（Host 白名单需按绑定地址配置）。

### C9 | WS 序号契约：进程级单调 + 有界重放，或重连必发 snapshot；快照边界写死

- 断言: 二选一并在契约写死：①seq 进程级单调（跨连接不复位）+ ≥1024 帧环形重放 + `epoch`，重连带 `resumeSeq` 时只发 `seq>resumeSeq` 的增量，缺口 >1024 发 snapshot；②删除 `resumeSeq`，每次重连首帧（hello 之后）必发 snapshot。无论哪种：snapshot 覆盖到 `seq=S`，之后该连接不得再发 `seq≤S` 的帧。
- 判定: `go test ./internal/monitor -run TestWSSeqContract`：连接 A 收 snapshot(S) 与若干 delta；断开；`?resumeSeq=S` 重连；断言首帧为 snapshot，或所有 delta 的 seq>S 且无缺口；`go test -run TestWSSeqMonotonic`：两次连接的 seq 在方案①下严格递增、方案②下无跨连接语义依赖；并发写在订阅窗口内制造竞态，客户端不得先收旧 snapshot。
- 严重级: 高
- 归属模块: m5
- 攻击来源: A1-5、A4-5
- 状态: 满足
- 证据: `15-interfaces.draft.md:107-108,125-127`
- 降级理由: -
- 残留影响: 若降级：重连后丢更新或重复推送，页面状态与引擎不一致。
- 复核触发: 改为每次重连全量 snapshot（方案②）时只需验 snapshot 边界。

### C10 | 监听器生命周期：全进程只注册一次，连接/重连不得注册

- 断言: `AddChangeListener` 调用点只允许两处：监视器构造时对现有实体、`OnCreate` 对新实体；WS 连接/订阅/重连不得新增监听器；断开后不得残留无法回收的监听器；100 次连接/断开后 goroutine 数与监听器槽位数回到基线。
- 判定: `grep -rn "AddChangeListener" internal/monitor/*.go` 的调用点恰为 2 处（构造 + OnCreate），且不在连接处理文件里；`go test ./internal/monitor -run TestReconnectNoLeak -race`：100 次连接/断开后 `runtime.NumGoroutine()` 差值 ≤1，实体监听器计数不增长（测试需提供 `entity` 包内的只读计数辅助）。
- 严重级: 高
- 归属模块: m5
- 攻击来源: A2-3（另见越界发现 #1）
- 状态: 满足
- 证据: `internal/entity/model.go:40-52`（按索引置 nil）、`internal/entity/model.go:146`（每次写复制切片）、`internal/engine/engine.go:543-558`（引擎每 tick 注册）
- 降级理由: -
- 残留影响: 若降级：每个连接一份监听器，重连风暴下写路径 O(监听器数) 退化；页面长时间开着内存持续涨。
- 复核触发: 引擎侧修复监听器压缩后，可放宽到"每连接一次注册 + 明确移除"。

### C11 | 诊断帧必须能区分新增/更新/清除

- 断言: `diagnostics` 帧必须带可判定的语义（`added|updated|removed`），或 generation 变化时发全量 snapshot + generation；同一条去重记录（同 fingerprint）不得以两个不同 id 出现；`ClearByScript/ClearByEntity/ClearBySource` 或环形淘汰后，客户端不得永久残留已删记录。
- 判定: `go test ./internal/monitor -run TestDiagnosticsFrameSemantics`：同一 `diagnostics.Record` Add 两次 → 只出现 1 个 id 且 `count==2`；`ClearByScript` 后收到 removed 帧或 snapshot 不含该记录；`Summary` 的 overflow 变化有对应帧。
- 严重级: 高
- 归属模块: m5
- 攻击来源: A2-4
- 状态: 满足
- 证据: `internal/diagnostics/store.go:31-57`（Add/count++）、`95-110`（clear）、`152-156`（generation）；`15-interfaces.draft.md:110`（单一 record 帧）
- 降级理由: -
- 残留影响: 若降级：页面把更新画成重复条目，清除后残留幽灵诊断。
- 复核触发: diagnostics 增加 SetNotify 后。

### C12 | 诊断通知必须在锁外回调

- 断言: 若新增 `diagnostics.Store.SetNotify`，回调必须在释放 `Store.mu` 之后调用；回调内调用 `Generation()/Snapshot()/Summary()/Len()` 不得超时。
- 判定: `go test ./internal/diagnostics -run TestNotifyNotUnderLock -timeout 10s`：notify 回调内调用 `store.Generation()` 必须在 1s 内返回；把实现改为锁内回调时该测试必须超时失败（证明测试有牙）。
- 严重级: 高
- 归属模块: global
- 攻击来源: A2-5
- 状态: 满足
- 证据: `internal/diagnostics/store.go:36-45`（Add 持锁）、`113-163`（读方法同锁）
- 降级理由: -
- 残留影响: 若降级：第一条诊断入库即自锁，整条诊断/事件链停摆。
- 复核触发: 改为无锁 snapshot+原子指针实现时重评。

### C13 | 日志读取护栏：路径白名单、拒 symlink、尾部有界、完整行、响应上限

- 断言: 只读 `<dataDir>/logs/` 下 `state_YYYY-MM-DD.log` 与 `dsh-control-YYYYMMDD.log`；`date` 先 `time.Parse` 再重新 format（不得字符串拼接）；`Lstat` 拒绝符号链接（并用 `O_NOFOLLOW` 防 TOCTOU）；从文件尾有界读取（不得整文件读入），只返回以 `\n` 结尾的完整行；响应有 maxBytes（建议 ≤1 MiB）且超限 `truncated:true`；当日无 tick 导致无文件时返回 200 + 空数组（不是 404）。
- 判定: `go test ./internal/monitor -run TestLogsGuards`：`?date=../../etc/passwd` → 400；`data/logs/state_2026-10-26.log` 是指向敏感文件的 symlink → 不返回目标内容；10MiB 日志 `?limit=2000` → 响应 ≤1MiB 且 `json.Valid`；文件末行无 `\n` 时该行不出现。
- 严重级: 高
- 归属模块: m4
- 攻击来源: A4-4
- 状态: 满足
- 证据: `internal/engine/stateLogger.go:30-33,51`（空 tick 不写）、`internal/engine/stateLogger.go:64-89`（7 天 prune）、`internal/control/logger.go:84-90`（control 文本行）
- 降级理由: -
- 残留影响: 若降级：符号链接可读任意文件；大日志拖垮内存；半行导致页面解析失败。
- 复核触发: 日志源改为既有 API（如 ring）而非文件时重评。

### C14 | K1.3 path 语义与 `/state` 一致，数据源必须声明

- 断言: K1.3 必须写明数据源（建议 `Entity.Snapshot()`，与 control `/state` 一致，模板回退表现为 missing；若选 `Entity.Get` 则必须显式声明模板回退），并保证同一 path 在 `/state` 与 `/monitor/api/entities/{id}` 上同值/同 missing；重复 path 去重、空 path 400。
- 判定: `go test ./internal/monitor -run TestPathParityWithControl`：先 `store.Create` 再 `store.AddTemplate` 新增字段，同一 path 两端必须同 missing 或同值；`?path=`（空）→ 400。
- 严重级: 中
- 归属模块: m4
- 攻击来源: A2-6（另见越界发现 #3）
- 状态: 满足
- 证据: `internal/control/state.go:45-52,74-98,114-125`（Snapshot 无回退）、`internal/mcpserver/server.go:128-132`（Get 有回退）
- 降级理由: -
- 残留影响: 若降级：页面与 MCP 对同一字段判断相反，V1/V3 互斥。
- 复核触发: 统一 `/state` 与 MCP 语义时同步更新。

### C15 | maxBytes 截断确定性

- 断言: `bytes` = 序列化后 payload 字节数（不含 envelope，或明确含 envelope 的口径）；截断按完整单元（实体/行）进行，响应 JSON 必须可解析；实体 `keys` 自身超限时省略该实体并置 `truncated:true`；全量超限时 `data` 的取值（`null` 或前缀）在契约写死；请求 `maxBytes` 越界 400。
- 判定: `go test ./internal/monitor -run TestMaxBytesDeterministic`：100 实体、每实体 keys 1KiB、`maxBytes=4096` → `json.Valid`、payload ≤4096、items 是 id 字典序前缀、`truncated:true`；`maxBytes=1023` → 400；`data` 在超限时为契约写死的取值。
- 严重级: 中
- 归属模块: m4
- 攻击来源: A4-3
- 状态: 满足
- 证据: `internal/control/state.go:54-72`（同类语义参考：bytes/data:null/truncated）、`15-interfaces.draft.md:32-40`
- 降级理由: -
- 残留影响: 若降级：本机大实体列表可能撑爆页面；截断语义不可复算，V2 无法判定。
- 复核触发: 增加 gzip 或分页后重评。

### C16 | 脚本源码读取只走 `registry.ReadScript`

- 断言: `/monitor/api/scripts/{name}` 的 `source=true` 必须调用 `registry.ReadScript(name)`（自带 symlink 拒绝、256KiB 上限、路径越界校验），不得直接 `os.ReadFile`，不得自拼路径；将 `ReadScript` 的 size/symlink 错误映射为 413 `too_large`/4xx。
- 判定: `grep -rn "os.ReadFile" internal/monitor/*.go` 对脚本源路径无命中；`go test ./internal/monitor -run TestScriptSourceGuards`：脚本是指向敏感文件的 symlink → 4xx 且响应不含目标内容；>256KiB → 413 `too_large`。
- 严重级: 高
- 归属模块: m4
- 攻击来源: A5-3
- 状态: 满足
- 证据: `internal/registry/scriptFiles.go:47-78`（ReadScript 护栏）、`11-12`（MAX_SCRIPT_BYTES）、`200-221`（safeSourcePath）、`registry.go:96,492-500`（脚本名规则）
- 降级理由: -
- 残留影响: 若降级：符号链接可读任意文件；大文件可拖垮进程。
- 复核触发: 脚本存储改为内存注册表时重评。

### C17 | 前端门禁命令可执行且扫描真实源码

- 断言: 门禁命令必须字面可执行、无占位符，且扫描真实前端源码：
  `bash ~/.dsh/skills/ui-composition/scripts/lint-ui-plan.sh --plan-dir design --tokens design/tokens.json --src-root internal/monitor/web` 退出 0；4 个反例（硬编码样式/原始色阶/未登记元件/组合层造原子）各自退出非 0，且反例命中真实源码文件。
- 判定: 上述命令 `echo $?` = 0；`for d in design/fixtures/fail/*/; do bash ... --plan-dir "$d" ...; test $? -ne 0 || exit 1; done`；在真实 `internal/monitor/web/*.css|*.js` 注入一个硬编码色值后命令必须 exit 1（证明扫描面覆盖实现文件）。
- 严重级: 阻断
- 归属模块: m6
- 攻击来源: A6-1
- 状态: 满足
- 证据: `~/.dsh/skills/ui-composition/scripts/lint-ui-plan.sh:22-27`（默认 `<plan-dir>/tokens/tokens.json`、`--src-root` 默认=plan-dir）【已核：脚本存在且帮助行如引】；`15-interfaces.draft.md:148-151`；`00-scope.md:27`（占位符命令）
- 降级理由: -
- 残留影响: 若降级：门禁只校验方案 JSON，页面实现可以完全绕过分解约束，V6 变成假绿。
- 复核触发: ui-composition 技能升级改变默认布局时。

### C18 | `go:embed` 生成物入版本库且可复现

- 断言: `internal/monitor/web/` 必须存在真实文件（至少 `index.html` + 引用的 css/js）并提交进版本库；`make design` 之后生成物可复现（`git diff --exit-code` 为空）；干净工作树（`git archive HEAD` 解包）能编译/测试该包，不依赖本地临时生成。
- 判定: `test -n "$(git ls-files internal/monitor/web)"`；`make design && git diff --exit-code -- internal/monitor/web design/dist`；`tmp=$(mktemp -d) && git archive HEAD | tar -x -C "$tmp" && (cd "$tmp" && GOCACHE="$PWD/.local/go-cache" go test ./internal/monitor/ -run TestNoop)` 编译通过；`strings bin/astrbox-mcp | grep -q '<title>'`（或等价页面特征）。
- 严重级: 阻断
- 归属模块: m6
- 攻击来源: A6-2
- 状态: 满足
- 证据: 仓库当前无 `internal/monitor/`、无 `go:embed`（`grep -rn "go:embed" internal cmd` 无命中）【已核】；`15-interfaces.draft.md:149,152-153`
- 降级理由: -
- 残留影响: 若降级：`go test ./...` 或 `make build` 在干净树失败；V7 不可复算。
- 复核触发: 改用 `//go:generate` + CI 生成缓存时重评。

### C19 | `make build` 把设计真源与 web 资产列为重建前置

- 断言: `design/tokens.json`、`design/components.json`、`design/composition.json` 与 `internal/monitor/web/**` 必须是引擎二进制目标的前置依赖；修改任一文件后 `make -q build` 必须返回 1（需要重建）。
- 判定: `touch design/tokens.json && make -q build; echo $?` = 1；`touch internal/monitor/web/index.html && make -q build; echo $?` = 1；`make -n build | grep -q 'design'` 命中生成步骤。
- 严重级: 阻断
- 归属模块: m6
- 攻击来源: A6-3
- 状态: 满足
- 证据: `Makefile:16-24`（`$(GO_SOURCES)` 仅 cmd/internal 的 *.go）、`15-interfaces.draft.md:152`
- 降级理由: -
- 残留影响: 若降级：页面/令牌改动不触发重建，二进制内嵌旧页面；V7 假绿。
- 复核触发: 改用独立前端构建产物分发时重评。

### C20 | 构建版本可追溯；验收命令无占位符

- 断言: `health.version` 非空且等于 `git rev-parse --short HEAD`（或等价的构建期注入版本）；`00-scope.md`/`15-interfaces.draft.md`/`40-plan.md` 中所有判定命令不得含 `<...>` 占位符（如 `<ui-composition>`），必须字面可执行。
- 判定: `curl -s http://127.0.0.1:8765/monitor/api/health | jq -r .version` == `git rev-parse --short HEAD`；`grep -nE '<[a-z][a-z-]*>' runs/web-monitor/00-scope.md runs/web-monitor/15-interfaces.draft.md | grep -v '<string>\|<int>\|<bool>\|<abs>\|<JSON>\|<token>\|<id>\|<path>\|<name>\|<rel>\|<sha1>\|<text'` 无输出（占位符清零）。
- 严重级: 中
- 归属模块: m6
- 攻击来源: A6-3、A6-4
- 状态: 满足
- 证据: `Makefile:16-24`（无 `-ldflags`）、`cmd/astrbox-mcp/main.go`（无 version 变量）、`00-scope.md:27`（`<ui-composition>`）、`internal/mcpserver/server.go:58`（硬编码 "1.0.0"）
- 降级理由: -
- 残留影响: 若降级：无法从页面判断跑的是哪个提交；验收步骤拷不动，V6/V7 不可复算。
- 复核触发: 引入正式发布版本号方案时。

### C21 | WebSocket 依赖可离线复现

- 断言: 二选一并写进契约：①引入 `github.com/coder/websocket` 时，`go.mod`/`go.sum` 必须已锁版本且模块已进入 `.local/gomodcache`（或仓库内 vendor），`GOPROXY=off make build` 通过；②零依赖手写时，必须覆盖分片/掩码/关闭握手并有对应测试。
- 判定: 库方案：`grep -q 'github.com/coder/websocket' go.mod go.sum` 且 `GOPROXY=off make build` exit 0；手写方案：`go test ./internal/monitor -run 'TestWSFragment|TestWSMask|TestWSCloseHandshake'` 通过。当前反例：`ls .local/gomodcache/github.com` 无 `coder`【已核】。
- 严重级: 阻断
- 归属模块: m7
- 攻击来源: A6-5
- 状态: 满足
- 证据: `go.mod:1-14`、`go.sum`（无 coder）【已核】；`.local/gomodcache/github.com/` 仅 golang-jwt/google/modelcontextprotocol/segmentio/yosida95【已核】；`15-interfaces.draft.md:94`
- 降级理由: -
- 残留影响: 若降级：离线/受限网络下 `make build` 直接失败，V7 不成立；手写帧实现则引入协议兼容风险。
- 复核触发: 更换依赖或改为 stdlib 实验特性时。

### C22 | 回归门禁：`make test && make lint && make doctor` 全绿且路径可移植

- 断言: V8 必须写成 `make test && make lint && make doctor` 三条命令；`make doctor` 断言 17 tools；`make lint` 的 lintCodeModularity 路径必须可配置/仓库内解析，不得硬依赖 `$HOME`。
- 判定: `make test && make lint && make doctor` exit 0；`make doctor` 输出含 "17 tools"；`make lint` 在 `HOME=/tmp/nohome` 下仍可运行（脚本路径通过环境变量或仓库内路径提供）。
- 严重级: 阻断
- 归属模块: m7
- 攻击来源: A6-6
- 状态: 满足
- 证据: `Makefile:22-23,26-38`（`$(HOME)/.dsh/...` 硬路径、test/lint/doctor 目标）、`scripts/doctorDsh.mjs:3,161-171,237`（17 tools 断言）【已核：脚本存在】
- 降级理由: -
- 残留影响: 若降级：换环境/CI 时 lint 失败；"MCP 工具清单不变"无人判定，回归可静默破坏。
- 复核触发: 工具数量变化时同步更新 doctor 与 V8。

### C23 | 审计日志有轮转、不泄 token、写失败不影响请求

- 断言: 写操作审计写入 `data/logs/monitor-audit-YYYYMMDD.log`；按日轮转并保留 7 天（现有两个 prune 均不覆盖该前缀，必须自带 prune）；写失败不得令请求失败；审计行不得包含 token 值；单行有长度上限。
- 判定: `go test ./internal/monitor -run TestAuditLog`：制造 `data/logs` 不可写 → POST set 的业务结果不受影响；用 8 天前的 audit 文件 fixture 触发 prune → 最旧文件被删；grep 审计行不含 token 值；`go test ./internal/monitor -run TestAuditLineBound`。
- 严重级: 中
- 归属模块: m2
- 攻击来源: A5-4
- 状态: 满足
- 证据: `internal/engine/stateLogger.go:64-89`（只删 state_*）、`internal/control/logger.go:63-81`（只删 dsh-control-*）、`15-interfaces.draft.md:143`
- 降级理由: -
- 残留影响: 若降级：审计无限增长写满磁盘，最终让 `store.Save`（tick）失败；审计行可能泄漏 token。
- 复核触发: 审计改由外部日志系统承接时。

### C24 | tick 载荷单一来源，三处字段集合相同且含 `tick_id`

- 断言: control `/tick`、monitor `/monitor/api/tick`、WS `tick` 帧必须由同一构造函数生成载荷，字段集合相同且包含 `tick_id`（或 `tickId` 且契约声明二选一）；不得复制字段定义。
- 判定: `go test ./internal/monitor -run TestTickPayloadParity`：分别请求/接收三处，解 JSON 后断言 key 集合相等且含 `tick_id`/`tickId`；`grep -rn "rules_evaluated" internal/control internal/monitor` 只在一处载荷构造实现中出现（加测试断言），其余为调用/测试。
- 严重级: 中
- 归属模块: m3
- 攻击来源: A2-7
- 状态: 满足
- 证据: `internal/control/tick.go:55-66`（未导出 tickBody，已含 tick_id）、`internal/control/response.go:29-34`（未导出 response）、`15-interfaces.draft.md:73-74`（字段清单漏 tick_id）
- 降级理由: -
- 残留影响: 若降级：三处载荷漂移，页面按 monitor 字段开发、MCP 按 control 字段消费，V1 逐字段一致不可满足。
- 复核触发: 载荷增加字段或改为 protobuf 时。

## 状态汇总

| 约束 | 严重级 | 归属 | 状态 | 一句话 |
|---|---|---|---|---|
| C1 | 阻断 | global | 满足 | 同一 data-dir 单写者（锁或同进程） |
| C2 | 阻断 | m5 | 满足 | OnCreate 回调不得取 Store 锁 |
| C3 | 阻断 | global | 满足 | 钩子单向依赖，被观察包不 import monitor |
| C4 | 高 | m3 | 满足 | tick 观察者非阻塞、不重入 |
| C5 | 高 | m3 | 满足 | 保存失败回滚或 dirty |
| C6 | 高 | m3 | 满足 | gate 争用有界（tick 409 / set 超时） |
| C7 | 阻断 | m2 | 满足 | 浏览器可用鉴权矩阵 |
| C8 | 阻断 | m2 | 满足 | 反 rebinding：Host 白名单 + Origin 精确 |
| C9 | 高 | m5 | 满足 | WS seq/重连/快照边界 |
| C10 | 高 | m5 | 满足 | 监听器只注册一次 |
| C11 | 高 | m5 | 满足 | 诊断帧区分增/改/删 |
| C12 | 高 | global | 满足 | 诊断通知锁外回调 |
| C13 | 高 | m4 | 满足 | 日志读取护栏 |
| C14 | 中 | m4 | 满足 | path 语义与 /state 一致 |
| C15 | 中 | m4 | 满足 | maxBytes 截断确定性 |
| C16 | 高 | m4 | 满足 | 脚本源只走 ReadScript |
| C17 | 阻断 | m6 | 满足 | lint 门禁可执行且扫真实源码 |
| C18 | 阻断 | m6 | 满足 | embed 生成物入库且可复现 |
| C19 | 阻断 | m6 | 满足 | make 依赖设计真源/web 资产 |
| C20 | 中 | m6 | 满足 | 版本可追溯 + 命令无占位符 |
| C21 | 阻断 | m7 | 满足 | WS 依赖离线可复现 |
| C22 | 阻断 | m7 | 满足 | 回归三命令全绿、路径可移植 |
| C23 | 中 | m2 | 满足 | 审计轮转/不泄 token/失败不挡请求 |
| C24 | 中 | m3 | 满足 | tick 载荷单一来源 |

> 收敛判定（G3 完成）：本表 24 行已全部落为 `满足`（★裁决2：全部接受）；实现后由 `60-verification.md` 回填每条的实际命令输出。
> 任何一条在 G4 无法给出检测证据 ⇒ 回 G3 重裁（转降级需附理由+残留影响+复核触发），不得静默放行。
