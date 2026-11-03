# Neko/AstrBox 代码缺陷扫描报告

- 日期：2026-11-03（Asia/Shanghai）
- 范围：预防性缺陷扫描 · 聚焦高风险核心包（`internal/` 全部 30+ 包，非测试源码约 9 万行；深审重点在 engine / function / control / events / registry / monitor / driver / mcpserver / neko / compiler / entity 等高危核心）
- 方法：机械基线（`go build` / `go vet` / `gofmt` / `staticcheck 0.8.2` / `go test -race`）+ 4 路并行人工深审（按包分片、写域互斥）+ Lead 独立复核
- 结论锚点：每条均带 `文件:行` 与代码片段；Lead 已逐条核对 high 与主要 medium，low/info 为 reviewer 锚定（均已核对行号存在性）

## 总览

| 严重度 | 数量 | 说明 |
|---|---|---|
| critical | 0 | 无默认配置下可直接远程利用/大规模数据丢失的缺陷 |
| high | 5 | 可触发崩溃、数据竞争/静默损坏、数据丢失、引用泄漏、并发串扰 |
| medium | 16 | 逻辑/并发/错误处理缺陷，需特定触发条件 |
| low | 22 | 边界、健壮性、纵深防御、死代码 |
| info | 6 | 性能/可维护性/疑似 |

机械基线结论：`go build`、`go vet`、`gofmt -l`（0 未格式化）全干净；`staticcheck` 仅 2 处非测试死代码 + 3 处测试文件未用符号；`go test -race` 无真实数据竞态（`internal/control`/`cmd/dsh-mcp` 的 FAIL 均为控制面 Unix socket 路径 >100 字节的沙箱伪缺陷，非代码缺陷，见 L3）。

---

## HIGH（5）

### H1. 实体 `Get` 实例命中路径返回活引用 → 数据竞争 + 静默损坏（绕过锁与 OnChange）
- 位置：`internal/entity/model.go:104-108`（对照模板回退路径 `:223-224` 的 `deepCopy`）；实锤利用点 `internal/engine/ops.go:95-105`
- 类型：并发竞态 / 逻辑错误
- 代码：
  ```go
  // model.go Get 实例命中直接返回 e.Data 内嵌套对象
  e.mu.RLock(); current, found := lookup(e.Data, parts); e.mu.RUnlock()
  if found { return current, nil }   // 未 deepCopy
  // 模板回退路径：lookupTemplates → return deepCopy(value), true
  ```
  ```go
  // ops.go "append" 分支原地 append 返回的活切片
  current := target.GetOrNone(path)
  items, _ := current.([]any)
  items = append(items, value)       // 容量富余时在原地改 e.Data 共享底层数组，未持锁
  target.Set(path, items)            // 随后才深拷贝写回
  ```
- 推理：`Get`/`GetOrNone` 对「实例数据命中」返回内部嵌套 `map[string]any`/`[]any` 的活引用，只有模板回退才深拷贝，行为不一致。调用方原地 `append` 会绕过 `e.mu` 锁与 `OnChange`，并发下与 `Snapshot()`/其它 `Get()` 构成数据竞争（控制面 `/turn-context` 走 `Store.Get().Snapshot()` 不经过 `WithStateWrite`，可与 tick 的 `append` 交错）。
- 建议：`Get` 实例命中路径也 `deepCopy(value)` 后返回（与模板路径统一）；或显式提供 `GetRef` 语义并让调用方自担责。

### H2. 实体 JSON 解析失败被静默 rename+return nil → 下次 Save 以空状态覆盖，数据丢失
- 位置：`internal/entity/store.go:102-105`（同类 `:60-67` loadTemplatesLocked 用 `continue` 静默跳过）
- 类型：错误处理 / 数据损坏
- 代码：
  ```go
  if err := json.Unmarshal(raw, &payload); err != nil {
      _ = os.Rename(path, path+".broken.bak")
      return nil          // 不向上传播错误
  }
  ```
- 推理：`entities.json` 因非原子写/掉电出现半截内容时，`Load()` 不返回错误，仅把文件改名 `.broken.bak`，调用方认为加载成功；随后任意一次 `Save()` 用内存中的空实体集覆盖 `entities.json`，原数据只剩无人知晓的 `.broken.bak`（不自动恢复），等效永久丢失。
- 建议：向上传播错误（或至少记 diagnostics/warning），不要静默 rename+return nil；rename 失败的错误也不要 `_ =` 丢弃；`.broken.bak` 已存在时做覆盖保护。

### H3. registry 更新脚本不注销被删除的 @函数/@事件 → 引用泄漏，旧定义继续生效
- 位置：`internal/registry/applyCore.go:64-67`、`:105-108`（对照 `registry.go:259-262` Remove 路径的对称处理）
- 类型：资源泄漏 / 逻辑错误
- 代码：
  ```go
  // commitLocked 对旧 entry 只释放模板/规则，从不注销 funcs/events
  if old := r.entries[name]; old != nil { delete(r.entries, name); r.releaseLocked(old, ...) }
  ...
  r.sink.RegisterFuncs(prepared.Compiled.Funcs)   // 只注册新的，未注销旧的
  r.sink.RegisterEvents(prepared.Compiled.Events)
  // Remove 路径则显式 UnregisterFuncs/UnregisterEvents
  ```
- 推理：脚本 A v1 定义 @函数 `foo`/@事件 `bar`，更新到 v2 删除它们。`commitLocked` 只 `releaseLocked`（`registry.go:269-287` 仅处理 Templates/Rules），从不 `UnregisterFuncs/UnregisterEvents`，随后只注册 v2 新定义。`foo`/`bar` 残留在 FunctionSink（事件总线/函数注册表）中继续生效，与 Remove 路径明显不对称。
- 建议：在 `releaseLocked` 后、`Register*` 前，对 `old.Funcs`/`old.Events` 执行 `Unregister*`。

### H4. 函数切片负步长 `high` 未钳制下界 → 负索引 panic（进程崩溃）
- 位置：`internal/function/eval.go:305-321`（sliceIndices）、`:259-272`（evalSlice 取元素）
- 类型：边界条件 / 越界 panic
- 代码：
  ```go
  // step<0 分支只 clamp 了 start，high 仅 normalize 未 clamp 到 >= -1
  if high != nil { end = normalizeIndex(*high, length) }
  for index := start; index > end; index += step { indices = append(indices, index) }
  // evalSlice：typed[index] / runes[index]  —— 负 index 直接 panic
  ```
- 推理：`normalizeIndex(-8, 5) = -3`，`end=-3`，循环把 `index` 递减到 -1、-2，产出负索引；`typed[-1]` 触发 `index out of range` panic。最小触发：`.neko` 函数里写 `列表[:-8:-1]` 且列表长度 ≤ 6。正步长分支对 low/high 都做了 `[0,length]` 钳制，唯独负步长漏了 high 下界。
- 建议：负步长分支对 `end` 同样钳制 `if end < -1 { end = -1 }`；取元素前对索引做 `0<=index<length` 断言。

### H5. 控制面 buildTurnContext 并发写共享 Board（SetSnapshot+Assemble 非原子，快照串扰）
- 位置：`internal/control/turnContext.go:64-67`、`internal/control/turnRecord.go:32`、`internal/control/server.go:118`
- 类型：并发竞态（逻辑 TOCTOU）
- 推理：`dispatchWithinDeadline` 为每个请求起 `go func(){ s.dispatch(r) }()`，`GET /turn-context` 与 `POST /session-event` 可并发调用 `buildTurnContext`。该函数先 `Board.SetSnapshot(snapshot)`（本请求快照）再 `Assembler.AssembleWithActive(Board: s.runtime.Board)`（内部 `Board.Snapshot()`）。`Board` 自身有 `RWMutex`（`blackboard/board.go:44`），单次 Set/Get 原子，但「Set→Assemble 读」复合操作非原子：并发请求可在两次操作之间覆盖 snapshot，使 assembler 用另一实体/请求的快照组装补偿包，而响应携带的 `snapshot` 又是本地正确值 → 响应内 `snapshot` 与 `compensation` 不一致。`.neko` 回调 + Save 被刻意包进 `WithStateWrite` 串行化，此处却不串行，与代码自身并发约定矛盾。
- 建议：把 `buildTurnContext` 的 Board 写入纳入与 `WithStateWrite` 等价的串行化；或在 Board/Assembler 层加锁并保证 SetSnapshot+Assemble 原子。

---

## MEDIUM（16）

### M1. dispatchWithinDeadline 超时后 handler 仍继续运行（期限未真正生效 + 潜在 goroutine 累积）
- 位置：`internal/control/server.go:115-124`
- 代码：`ctx, cancel := context.WithTimeout(r.Context(), s.deadline)`；`go func(){ result <- s.dispatch(r) }()`；select 超时返回 504，但 `s.dispatch(r)` 用 `r.Context()`（非派生 ctx）未被取消，继续后台执行。
- 推理：超时只约束「响应返回」，不取消 handler 副作用；handler 无限阻塞时 goroutine 常驻并可能长期持有状态锁，且「客户端已收到 504 后」状态仍被改写。
- 建议：把派生 `ctx` 传入 dispatch 链（或让 handler 感知 ctx）。

### M2. events 总线递归深度计数为全局共享，并发事件误判「触发过深」
- 位置：`internal/events/bus.go:150-166`
- 推理：`depth` 是 Bus 单一 int，本意是「同一调用栈嵌套深度」，但 `Call` 在锁外执行，多个 goroutine 的 handler 重叠运行各自把全局 `depth`+1；10 个互不嵌套的并发事件累加后 `depth>=10`，第 11 个被误拒为「循环」跳过。
- 建议：depth 按调用栈传递（goroutine-local/上下文携带），或用带上限的并发信号量区分「嵌套深度」与「并发数」。

### M3. handleSessionEvent 未做实体 nil 检查（疑似空指针）
- 位置：`internal/control/sessionEvent.go:120`、`:130`
- 推理：`entity := s.runtime.Store.Get(s.entityID)` 直接传入 `Funcs.Call(functionName, args, entity)`，未判 nil；同包其它 handler（`state.go`、`turnContext.go:25-26`）均判 nil。若实体不存在，`runner.current == nil`，函数体解析相对路径时会解引用 nil → panic。成立前提是回调函数体确实解引用 current（标疑似）。
- 建议：与其它 handler 一致，`Store.Get` 后判 nil 返回 notFound。

### M4. setdefault 原地改 map、不写回也不触发 Set → 写入丢失/绕过监听
- 位置：`internal/function/methods.go:311`、`:44-61`
- 推理：`methodSetDefault` 对 `r.eval(target)` 返回的 map 直接 `object[name] = fallback`。实例字段是活引用（见 H1），原地写绕过 `Set`/`OnChange`（branch_race 追踪与持久化都看不到）；模板字段是副本，写入静默丢弃。与 append/insert/pop/update 等走 `assignPath→Set` 的突变方法不一致。
- 建议：`setdefault` 检测 target 为路径时走 `r.assignPath` 写回；仅局部 map 变量才允许原地写。

### M5. 变更监听器无锁访问 StateMachine 共享字段（疑似并发竞态）
- 位置：`internal/engine/conflict.go:82-114`、`internal/engine/engine.go:548-557`
- 推理：`noteStateWrite` 在实体变更监听器回调里无锁读写 `m.tickResult/m.tickTracker/m.currentRule/m.pendingConflicts`。Tick 期间由 `ent.Set` 同步触发是安全的，但 `Entity.Set` 释放 `e.mu` 后才调监听器（`model.go:148-155`），若宿主在 Tick 进行时从另一 goroutine 写实体（`Store.SetPath`/`Executor.Call`），监听器在不持 `m.mu` 时并发读写上述 map/切片 → 数据竞态。标疑似（假设宿主允许并发写成立）。
- 建议：`noteStateWrite` 改为依赖 Tick 已持有的锁语义，或用局部变量/原子替代共享字段。

### M6. set 操作用未 clamp 的 value 判等 → 转移虚报/遗漏
- 位置：`internal/engine/ops.go:67-77`
- 推理：真正写入的是 `newValue := m.clampValue(...)`，但 `equalValues(oldValue, value)` 用原始 `value` 比较。clamp 改变数值时：`old==value` 但实际状态已变 → 转移被遗漏；`old==newValue!=value` → 虚报转移（且 `applyTextEffects` 副作用已执行）。
- 建议：改为 `equalValues(oldValue, newValue)`。

### M7. replacePlaceholders 循环引用 → 无限循环（DoS/挂死）
- 位置：`internal/engine/ops.go:178-203`
- 推理：`for { ...; text = strings.Replace(text, placeholder, pyString(fieldValue), 1); continue }` 无条件 continue 且替换位置不前进；若字段值含 `&{...}`（自引用 `A="&{A}"` 或互引用），替换后仍含 `&{`，无限循环，CPU 打满、Tick 永不返回。
- 建议：设最大替换轮数上限，或记录已访问占位符检测循环引用。

### M8. 列表索引赋值越界/负索引吞错 + 原地改活引用
- 位置：`internal/function/exec.go:269-288`
- 推理：`position<0 || position>=len` 直接 `return nil`（Python 应 IndexError、`-1` 应为末元素）；且 `typed[position]=value` 原地写 `Get` 返回的活引用，未走 `Set`（不触发监听、模板字段写入丢失）。
- 建议：越界/负索引报脚本错误（负索引先换算 `len+position`）；写入走「读→复制→Set 写回」。

### M9. 回填规则硬编码 "状态" 前缀，与可配置 StateGroup 脱节
- 位置：`internal/engine/decisionLog.go:60`
- 推理：其它写状态处走 `config.Current().StatePath(group)`（默认 `"状态"`），此处写死 `".状态."`。用户改 `state_group` 配置后，仲裁回填的 learned 规则写到错误路径，后续 Tick 读不到该状态。
- 建议：改用 `config.Current().StatePath(group)`。

### M10. 回环白名单校验可伪造的 Host 头（疑似，需非回环绑定）
- 位置：`internal/monitor/server.go:116-120`、`internal/monitor/auth/auth.go:63-75`
- 推理：`auth.HostAllowed(r.Host)` 校验的是请求方可任意填写的 `Host` 头，非连接实际本地地址。若运维把 `-monitor-addr` 绑到非回环（flag 允许 `0.0.0.0`），攻击者带 `Host: 127.0.0.1` 即通过校验；在未配置 token 的只读形态下，`/monitor/api/*` 全部状态/诊断/日志与 `/monitor/ws` 可被远程无鉴权读取。
- 建议：用 `r.Context().Value(http.LocalAddrContextKey)` 校验连接实际落在回环，或在绑定层强制只允许回环监听。

### M11. entity_create / state_machine_resolve 绕过 tickMu 写门（疑似）
- 位置：`internal/mcpserver/server.go:268-273`、`internal/mcpserver/stateTools.go:56-64`
- 推理：`runTick.go` 用 `tickMu` 串行化 RunTick/WithStateWrite；`entity_state_set` 走 `WithStateWrite`，但 `entity_create`、`state_machine_resolve` 直接 `store.Create/Save`、`machine.ResolveConflictWithEffects/Save`，未串行。若 MCP 会话并发调用它们与正在进行的 tick，会交错读写实体状态与待仲裁冲突列表。标疑似（假设 `engine.StateMachine` 无跨操作互斥）。
- 建议：两个 handler 也走 `s.WithStateWrite(...)`。

### M12. Store.Save 两文件顺序写非原子 + 无 fsync
- 位置：`internal/entity/store.go:136-139`、`:171-180`
- 推理：先写公有 `entities.json` 再写 `user/entities.json`，第二步失败则分裂不一致；`atomicWrite` 用固定 `.tmp` + `Rename` 单文件原子但无 `fsync`，掉电可能得到空/截断文件，并被 H2 的静默吞错放大为数据丢失。
- 建议：两文件先各自 Marshal 成功；`atomicWrite` 增加 `Sync()` + 关闭 + `Rename`，必要时对目录 fsync，失败清理 `.tmp`。

### M13. Store.Create 持有 s.mu 写锁时调用公开回调 OnCreate（潜在死锁）
- 位置：`internal/entity/store.go:260-274`
- 推理：`OnCreate` 是公开字段，在 `s.mu`（不可重入）写锁持有期间执行；未来接线若在 `OnCreate` 里调 `s.Save()/Get()`（创建后立即持久化/广播）即死锁。对照 `Entity.Set` 刻意先解锁再回调，此处不一致。
- 建议：`Create` 先入 map、`s.mu.Unlock()` 再回调，或文档明确「回调内禁止访问 store」。

### M14. Store.Templates 返回内部 map 引用（无锁无拷贝）
- 位置：`internal/entity/store.go:419-423`
- 推理：返回 `s.templates` 本体，`s.mu` 已释放；调用方若增删改会与 `AddTemplate`（锁内 DeepMerge/deepCopyMap）并发读写同一 map。对比 `GetTemplate` 正确返回 `deepCopyMap`。
- 建议：返回深拷贝（或改名 `TemplatesSnapshot`）。

### M15. parser 块边界扫描不跳过字符串字面量，字符串内花括号错分块
- 位置：`internal/neko/parserMulti.go:96-103`、`:240-247`、`:351-358`、`:373-380`
- 推理：`parseMulticond`/`parseRuleRange`/`parseMultiIf` 用裸花括号深度计数定位子块闭合，不像 `findBalanced`/`findActionBrace` 那样跳过字符串。动作值含字符串内不成对花括号（`x = "a}b"`）时边界错位，后续规则被吞或错切分。
- 建议：统一封装 `scanBalanced(rs, start)` 复用字符串跳过逻辑。

### M16. CompileScript 同名 @Data 类型直接覆盖丢字段（疑似）
- 位置：`internal/compiler/compiler.go:78-84`
- 推理：`result.Templates[def.TypeName] = expanded` 直接覆盖；而 `Store.AddTemplate` 对同名模板是 `DeepMerge`。若同一脚本重复定义同名类型，编译阶段覆盖丢字段，与存储阶段合并预期相悖。是否构成缺陷取决于「编译产物是否还会经 AddTemplate 再合并」及「是否允许重复定义」——标疑似。
- 建议：确认重复定义契约；若允许，编译阶段同样按 DeepMerge 累积。

---

## LOW（22，表格式）

| # | 位置 | 问题 | 建议 |
|---|---|---|---|
| L1 | `internal/registry/applyCore.go:101-104` | 更新脚本时 `r.order` 重复追加同名（先 delete 后 `!exists` 恒真） | 用 `if old == nil` 作为 append 条件 |
| L2 | `internal/registry/registry.go:264-265` | `Remove` 吞掉 `Save()`/`os.Remove` 错误，内存与磁盘可能不一致 | Save 错误计入结果或记诊断 |
| L3 | `internal/control/socket.go:12,22` | 控制面 socket 路径硬上限 100 字节（沙箱深目录下误拒合法路径，测试 FAIL 根因） | 按运行时实际 `sun_path` 上限计算或给明确提示 |
| L4 | `internal/registry/registry.go:223-230` | `applyScript` Noop 判重在锁外（TOCTOU，仅影响 noop 优化） | noop 判断挪进 commit 锁内二次确认 |
| L5 | `internal/function/valueOps.go:88`、`exec.go:228` | `+=` 对未初始化路径产生 `"None5"` 字符串拼接，`-=` 却返回 nil，行为不一致 | 对 nil/非数值显式报错或按 0 处理 |
| L6 | `internal/engine/engine.go:222,245` | `MAX_RULE_EVAL` 判定用 `>`，实际允许 3001 次（off-by-one） | 统一用 `>=` |
| L7 | `internal/function/builtins.go:22` | `随机(a,b)` 区间长度整数溢出 → `rand.IntN` panic（极端输入） | 用 uint64 计算并 clamp |
| L8 | `internal/engine/conditions.go:388-396` | 集合字面量 `!=` 未实现「不在集合」语义，恒 false/恒 true | 为 `!=` 增加非成员分支 |
| L9 | `internal/monitor/auth/auth.go:90`、`ws/handler.go:26` | token 用 `!=` 非常量时间比较，本地时序侧信道 | 改用 `crypto/subtle.ConstantTimeCompare` |
| L10 | `internal/monitor/api/respond.go:86-92` | `ReadBody` 超 1MB 静默截断，不报 413 | 用 `http.MaxBytesReader` 或比较长度返回 413 |
| L11 | `internal/monitor/ws/handler.go:18-32` | WS 握手 `InsecureSkipVerify:true` + 空 Origin 直接放行 | 保留库级 Origin 校验 + 严格判定 |
| L12 | `internal/driver/routes.go:103-111` | `OnEvent` 每路由 `go dispatchRoute` 无并发上限，瞬时 goroutine 洪峰 | 有界并发（semaphore/worker pool）或去抖 |
| L13 | `internal/monitor/api/health.go:16-21` | `/health` 无 token 形态泄露 dataDir/scriptsDir 绝对路径 + PID | 无鉴权形态脱敏或仅在有 token 时返回 |
| L14 | `internal/neko/parserRule.go:218-229` | `findActionBrace` 不处理反斜杠转义引号（与 findBalanced 不一致） | 处理 `\` 转义 |
| L15 | `internal/neko/lexer.go:54-66` | 多行名续行收集后 `name` 仍取首行，续行被丢弃 | 找到 `{` 时用 `collected` 作为 name |
| L16 | `internal/entity/path.go:109-113` | `BindSelectorToID` 不剥离类型前缀，`"NPC[*].x"` 产出畸形路径（当前无调用，疑似） | 替换 `(TYPE)?[*].` 整体或明确契约 |
| L17 | `internal/neko/parseScalar.go:62-72` | 超 int64 整数字面量退化 float64 丢精度（Python int 任意精度） | 超大整数保留字符串或 math/big |
| L18 | `internal/chat/chat.go:187` | `summarize` 按字节 `trimmed[:120]` 截断，会劈开 UTF-8 多字节字符 | 用 `[]rune` 截断 |
| L19 | `internal/chat/chat.go:154` | `json.Marshal` 错误被忽略，补偿包静默为空注入 prompt | 处理 Marshal 错误 |
| L20 | `internal/chat/chat.go:137` | 错误路径 journal 追加失败被忽略，turn 记录可能丢失 | 至少记日志/诊断 |
| L21 | `internal/compiler/ops.go:20` | `globals` 收集后从不读取，死代码（staticcheck SA4010） | 删除首循环与 `globals` |
| L22 | `internal/expert/persistence.go:32-37` | 原子写无 `fsync`（掉电可能得到空文件） | `WriteFile` 后 `Sync()` |

---

## INFO（6）

| # | 位置 | 说明 |
|---|---|---|
| I1 | `internal/monitor/ws/hub.go:42-47`、`client.go:118-123` | `Register` 持 hub 写锁构建全量快照，新连接建连期间阻塞所有发布 |
| I2 | `internal/driver/client.go:93` | 启动 token 放入 URL 查询串 `?token=`，可能被代理/日志记录（默认回环下影响可忽略） |
| I3 | `internal/engine/resolver.go:215` | `clampValue` 首匹配模板无 min/max 即 return，跳过父类型链后续 min/max（疑似） |
| I4 | `internal/entity/model.go:55-67`、`:230-250` | 递归 DeepMerge/deepCopy 无深度保护（JSON 路径有 10000 层兜底，程序化构造才可能超深） |
| I5 | `internal/function/eval.go:332` | `clampBounds` 未使用（staticcheck U1000，死代码） |
| I6 | `cmd/dsh-mcp/e2eHarness_test.go:78` 等 3 处 | 测试文件未使用符号（staticcheck U1000） |

---

## 附录

### 机械基线复算命令
```bash
export GOCACHE="$PWD/.local/gocache" GOTMPDIR="$PWD/.local/tmp" GOPATH="$PWD/.local/gopath" GOMODCACHE="$PWD/.local/modcache" GOFLAGS="-mod=mod"
go build ./...                 # 通过
go vet ./...                   # 通过
gofmt -l ./internal ./cmd      # 0 未格式化
# staticcheck（需先 go install honnef.co/go/tools/cmd/staticcheck@latest）
STATICCHECK_CACHE="$PWD/.local/staticcheck-cache" .local/bin/staticcheck ./...
# race（并发核心包）
go test -race ./internal/engine/... ./internal/control/... ./internal/monitor/... \
  ./internal/mcpserver/... ./internal/events/... ./internal/driver/... ./internal/registry/...
```

### 沙箱伪缺陷（非代码缺陷）
`cmd/dsh-mcp` 与 `internal/control` 的测试 FAIL 均为「控制面 Unix socket 路径 >100 字节」——沙箱把 `GOTMPDIR` 塞进深路径 `.local/tmp/...` 导致超过 `internal/control/socket.go` 的 100 字节硬上限；正常 `/tmp` 下不触发。已作为健壮性项记入 L3。

### 复核说明
- high 5 条与主要 medium 已由 Lead 逐条核对源码（文件:行 + 代码片段均已在正文引用）。
- 全部条目均为静态审阅 + 推理，未做运行时最小复现；H1/H4/H5 等并发/崩溃项建议以 `-race` 与单测补最小复现后修复。
- 本次仅分析、未改任何源码；唯一新增文件为本报告与 `.local/defect-scan/` 下的分片中间产物（过程产物，不入库）。
