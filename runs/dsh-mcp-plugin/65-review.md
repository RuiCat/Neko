# 独立复核：dsh-mcp 集成证据与契约（dsh-mcp-plugin）

- 复核者：`verify-dsh-mcp`（未参与实现，未接触实现者对话）
- 复核时间：2026-10-25 03:35–03:52（Asia/Shanghai）
- 对象：`runs/dsh-mcp-plugin/60-verification.md`、`40-plan.md`、`30-constraints.md`、`50-subtasks/*`、`.freeze` 与当前工作区
- 工作区基线：`main` @ `51cb2a7`（2026-10-25 00:01:23），工作树脏（21 条 `git status --porcelain`，均为 m1–m4 写域 + `runs/dsh-mcp-plugin/` 规划目录）
- 工具：`go1.27.3-X:nodwarf5 linux/amd64`、`node v26.12.1`
- 口径：下表「实际输出」全部是本复核者**本轮亲自执行**的输出，不是转抄证据表；只读被审产物，除本文件外未改动任何文件、未执行 git 写操作、未联网、未碰 `~/.dsh`/profile。

## 结论

**通过（附 6 条记录/判据层勘误 F1–F6，均不推翻实质结论）。**

独立重跑的关键门（Go 测试与 `-race`、Node 42 例、composition、doctor、模板绝对路径、gitignore、写域、冻结校验）全部复现证据表结论；`TestTickBusyImmediate409` 的 status/code 两种断言成立且语义上确实排除了排队。但证据表有 **2 处事实性不符**（F1：C11 的 `grep schemastery` 计数；F2：负例 MISSING 条数）、**2 处冻结约束判据与实现/工具链漂移**（F3：C4 的"只有一处实现"；F4：C16 的"删掉 bin 后 make doctor 非 0"）、**1 处证据指代错误**（F6：C9 的 1586 行）与 **1 处冻结文本陈旧**（F5：40-plan "待回填/待 freeze"）。这些属勘误与判据可执行性问题，不构成"模块未完成/集成未通过"的证据。若严格要求"证据表每个数字都可复现"，则应先把 F1/F2 勘误后再把结论记为通过；实质门禁本身已全部复现。

## 逐项核对表

| # | 核对项 | 我实际执行的命令 | 实际输出摘要 | 证据是否支撑结论 |
|---|---|---|---|---|
| 1a | Go 单测（control/mcpserver/dsh-mcp） | `export GOCACHE=$PWD/.local/go-cache GOMODCACHE=$PWD/.local/gomodcache; go test ./internal/control/... ./internal/mcpserver/... ./cmd/dsh-mcp/... -count=1` | `ok internal/control 0.127s`、`ok internal/mcpserver 0.051s`、`ok cmd/dsh-mcp 0.686s`，exit 0 | **支撑**。与 60 表「control 0.13s、dsh-mcp 0.69s 全 ok」一致（时间抖动）；包内含 `tick_test.go`/`state_test.go`/`sessionEvent_test.go`，tick/state/session-event 均被覆盖 |
| 1b | Go `-race`（同三包） | `go test -race ./internal/control/... ./internal/mcpserver/... ./cmd/dsh-mcp/... -count=1` | 三包 `ok`（1.199s / 1.201s / 10.176s），exit 0 | **支撑**。与「-race 两包 ok（10.3s）」及 m1 三包 -race 一致 |
| 1c | 全量 Go + 全量 `-race`（验证 m1/C3 的"15 包/`-race ./...` 全绿"） | `go test ./... -count=1`；`go test -race ./... -count=1` | 15 包全 `ok`（cmd/astrbox-mcp 19.227s、registry 5.039s）；`-race` 15 包全 `ok`（registry 58.499s），exit 0 | **支撑**。包数 15 与证据一致，耗时差异属正常抖动 |
| 2 | Node 单测 | `node --test dsh/lib/*Test.mjs` | `tests 42 / pass 42 / fail 0 / duration_ms 644`，exit 0 | **支撑**，数字完全一致；用例覆盖 C1（静态段 3 次同值+零请求、每轮 ≤1）、C2（cwd undefined/`<prefix>-old`/去重 256/同步与异步异常）、C11（坏键指名）、C12（白名单/非 R18/截断/前缀）、K1/D6（function 字段） |
| 3a | 隔离组合验证 | `bash scripts/verifyProfile.sh --composition` | `OK: composition checks passed (mcp-astrbox=1, astrbox-bridge=1, command=/home/ruicat/Desktop/Neko/bin/dsh-mcp, dump=/home/ruicat/Desktop/Neko/.local/profile-test/dump.txt)`，exit 0 | **支撑**（输出与证据表逐字一致）；脚本只写 `.local/profile-test`，未动真 profile |
| 3b | 加载面/socket 生命周期 | `make doctor` | `OK: doctor 通过（17 tools, socket 0600, SIGTERM 清理；data=/tmp/dsh-mcp-doctor-VlPTaJ/data）`，exit 0 | **支撑**（证据表省略了 `data=` 前缀，其余逐字一致） |
| 4a | 模板无机器绝对路径 | `grep -nE '/home/\|/Users/' dsh/cordis.patch.yml.template` | 无输出（grep exit 1） | **支撑**（C8/K2 的"0"成立） |
| 4b | 生成物被忽略 | `git check-ignore -v dsh/cordis.patch.yml .local/ data/dsh-control.sock data/.engine.lock` | 命中 `.gitignore:36 /dsh/cordis.patch.yml`、`:2 .local/`、`:34 /data/dsh-control.sock`、`:35 /data/.engine.lock` | **支撑** |
| 4c | 未忽略生成物是否混入 | `git status --porcelain` | 21 条：9 个 `M`（`.gitignore`、`Makefile`、`README.md`、`cmd/astrbox-mcp/main.go`、`internal/entity/{model,store}.go`、`internal/mcpserver/{server,snapshot,stateTools}.go`），12 个 `??`（`cmd/dsh-mcp/`、`docs/dsh-plugin.md`、`dsh/`、`internal/bootstrap/`、`internal/control/`、`internal/entity/snapshot_test.go`、`internal/mcpserver/{runTick.go,runTick_test.go}`、`runs/dsh-mcp-plugin/`、`scripts/{doctorDsh.mjs,renderDshPatch.sh,verifyProfile.sh}`）；无 `dsh/cordis.patch.yml`、无 `.local/`、无 `bin/` | **支撑**（生成物均被忽略） |
| 5 | `TestTickBusyImmediate409` 是否真排除排队 | 读 `internal/control/tick_test.go:58-82`、`internal/control/harness_test.go:107-130`；`go test ./internal/control/ -run TestTickBusyImmediate409 -count=1 -v` | `--- PASS: TestTickBusyImmediate409 (0.00s)`；断言为 `status==409` + `code==tick_busy` + `elapsed<=1s`；持锁者是 `env.mcp.WithStateWrite`（同一 `tickMu`，`harness_test.go:114`），其回调 `<-releaseChannel` 阻塞，`release()` 在断言之后（`tick_test.go:76`）才调用；`/tick` 走 `TryRunTick`（`internal/control/tick.go:34`）→ 实际 `TryLock`（`internal/mcpserver/runTick.go:28`） | **基本支撑，判据强度弱于表述**：若实现回退成阻塞 `Lock()`，`env.call` 永不返回，测试不会命中 `elapsed>1s` 分支，而是包级 10 分钟超时 panic——"排除排队"实际由**无界持锁**完成，1s 上界在排队场景不可达。status/code 两断言本身正确。另持锁入口是 `WithStateWrite` 而非真实 MCP tick 调用（分片覆盖，见"未覆盖项"） |
| 6a | D4/D5/D6 是否有裁决记录 | 读 `40-plan.md:145-156` | D4（WithStateWrite，`40-plan.md:152`）、D5（C6/C16 口径 + `*Test.mjs`，`:153`）、D6（TryRunTick + `function` 字段，`:154`）均在裁决记录表，且均标"走推荐/事后追认" | **支撑** |
| 6b | 30/40/50 是否按新契约更新 | `grep -n 'TryRunTick' 30-constraints.md 40-plan.md 50-subtasks/`；`grep -n 'function' 50-subtasks/m{2,3}.md` | K5 已含 `TryRunTick` 与"不排队"（`40-plan.md:79,83`）；m1 写域方法含 `TryRunTick`（`m1.md:30`）；m2 写"/tick 用 TryRunTick，拿不到锁立即 409"（`m2.md:33`）与 `function?` 字段（`m2.md:29`）；m3 含 `functionName`（`m3.md:31`）；C6 已含"`/state` 读路径不取写闸"（`30-constraints.md:73`）、C16 已含 `warningAfterMs` 限频（`:192`） | **部分支撑**：40/50 已按 D4/D6 更新，30 的 C6/C16 已按 D5 更新；但 **C4 的判定未随 D6 更新**（见不符 F3） |
| 6c | `.freeze` 是否最新 | `sha256sum` 前 12 位 vs `.freeze`；`plan.sh status`；`plan.sh validate --slug dsh-mcp-plugin --final` | 6 个文件逐一相等（30=`dd84929e1851`、40=`d3e2566b67d6`、m1=`128cb314dfc0`、m2=`2482442d4a19`、m3=`eb9e4a88f282`、m4=`4c10235e8834`）；status 输出 `[x] 已冻结且未失效`；validate 五段全过，`validate 结果: 通过（警告 0）`，exit 0 | **支撑**（哈希级一致，冻结有效）；但 40-plan 内 6 处"冻结哈希: 待回填"与 6 行"待 freeze"文本陈旧（见不符 F5） |
| 7a | 写域越界（路径集合） | `git status --porcelain` 逐条映射 `40-plan.md:100-105` | 21 条路径全部落在 m1（`cmd/astrbox-mcp`、`internal/{bootstrap,mcpserver,entity}`）、m2（`cmd/dsh-mcp`、`internal/control`）、m3（`dsh/`）、m4（`Makefile`、`scripts/`、`docs/dsh-plugin.md`、`.gitignore`、`README.md`）写域；`runs/dsh-mcp-plugin/` 是规划产物目录本身，不属任何子任务写域 | **支撑**（无路径级越界） |
| 7b | 模板是否被 m4 改过（内容/时间） | `ls -la --time-style=+%H:%M dsh/`；读模板与生成物 | 模板 mtime `02:09`，早于 m4 全部产出（`Makefile` 03:19、`scripts/*` 03:23–03:24、`docs/dsh-plugin.md`/`README.md` 03:29）；模板正文只含 `${REPO_ROOT}/${DATA_DIR}/${SCRIPTS_DIR}/${CONFIG}` 占位符，生成物 51 行 = 模板替换后结构（2 个 `insert`、7 处绝对路径均为 `REPO_ROOT`、id 各 1） | **支撑**（mtime 与内容均不支持"m4 改模板"）；注意 `dsh/` 是 untracked 目录，porcelain 无法区分目录内文件作者，此处只能给"未发现改动证据"，不能给"证明未改" |
| 8 | 反作弊：证据表输出 vs 我的实测 | 见上各条 | m1/m2/m3/m4、K1/K2/K3/K5/K6 的可复算数字全部复现；同时发现 3 处不一致（F1/F2/F6） | **总体支撑，但有逐条勘误**（见下） |

## 附加核对（契约/不变量抽样）

| 契约点 | 命令/证据 | 结果 |
|---|---|---|
| K1 七种错误码 | `grep -n 'CODE_' internal/control/response.go` | `bad_request/not_found/method_not_allowed/tick_busy/internal/shutting_down/deadline_exceeded` 全在（`:13-25`），映射 400/404/405/409/500/503/504（`:46-83`） |
| K1 四端点 + 方法门 | `internal/control/router.go:9-26` | `/health` GET、`/state` GET、`/tick` POST、`/session-event` POST，错误方法 405 |
| K1 写经 `WithStateWrite` | `internal/control/sessionEvent.go:92`、`internal/control/tick.go:34` | `/session-event` 回调+Save 在写闸内；`/tick` 经 `TryRunTick`（同 `tickMu`） |
| K5 单一共享 tick 体 | `internal/mcpserver/runTick.go:18,27,38,62` | `RunTick`/`TryRunTick` 共用 `runTickLocked`，`WithStateWrite` 同 `tickMu`；`grep -rn 'func .*RunTick' internal`（非测试）= 2（`RunTick`+`TryRunTick`） |
| K5 六个用例 | `internal/mcpserver/runTick_test.go` | 6 个 `Test*`（SaveOptions / StateMachineTickToolAlwaysSaves / ConcurrentWithSetAndSnapshot / WithStateWriteReturnsCallbackError / WithStateWriteSerializesWithRunTick / TryRunTickBusyWhileGateHeld），与"六个用例"一致 |
| K6 17 工具名三方一致 | `scripts/doctorDsh.mjs:17-23`、`cmd/dsh-mcp/e2eStdio_test.go:13-56`、`40-plan.md:90` | 三处逐名一致，e2e 为排序后逐项比较（非仅计数） |
| C2 无 `startsWith` 过滤 | `grep -rn 'header.cwd.startsWith' dsh/lib/*.mjs` | 0 命中（exit 1） |
| C3 无锁外 `ent.Data` | `grep -rn 'ent.Data' internal/mcpserver` = 0；`internal/mcpserver/server.go:124` 用 `ent.Snapshot()`；`internal/entity/store.go:128` 用 `ent.Snapshot()`；`Snapshot()` 在 `internal/entity/model.go:188-192` | 不变量成立；但 30 约束原文"`ent.Data` 仅出现在 Snapshot 实现内"的指代有误（Snapshot 在 model.go，store.go 内 `ent.Data` 为 0 次）——不影响判定 |
| C6 客户端超时 | `dsh/lib/config.mjs:36-37` | `requestTimeoutMs: 2000`、`tickTimeoutMs: 15000`，与服务端 5s 契约一致 |
| C7 日志命名/轮转 | `internal/control/logger.go:16,25`；`TestLoggerPrunesOldControlLogs`（`concurrency_test.go:89`） | `dsh-control-YYYYMMDD.log` + 7 天清理，测试在本次全量跑中通过 |
| C13 信任边界文档 | `docs/dsh-plugin.md:155` | 明确"无 token、无鉴权，只靠 socket 0600 + 同 uid 隔离" |
| C16 patch 未设 `failOnStartupError` | `grep -n failOnStartupError dsh/cordis.patch.yml.template dsh/cordis.patch.yml` | 两文件均无该键 |
| 代码规范门 | `make lint` | `go vet` 无输出；`lintCodeModularity.sh internal cmd dsh` → "扫描 149 个源码文件；0 项违规，0 项告警"，exit 0；单跑 `dsh` → "12 个源码文件；0 项违规，0 项告警"（与 m3 证据一致） |

## 实质不符清单（只记录，不修复）

### F1【证据失真｜中】C11 的 `grep schemastery dsh = 0` 实际为 1

- 位置：`60-verification.md:64`（"`grep schemastery dsh` = 0"），判据原文见 `30-constraints.md:133`
- 最小复现：
  ```bash
  cd /home/ruicat/Desktop/Neko
  grep -rn 'schemastery' dsh | wc -l        # 1
  grep -rn '@deepseek-ai/schemastery' dsh | wc -l  # 0
  ```
  实际命中：`dsh/lib/config.mjs:4` 的**注释**"零依赖、不引入 schemastery……"。
- 影响面：C11 的冻结判据按字面**不成立**，60 表却记为已通过；但 `dsh/package.json` 无任何 `dependencies`，实质"不引入 schemastery 依赖"成立，C11 的状态"满足"不改。
- 建议：把 C11 判据改成与 C15 相同的 `grep -rn '@deepseek-ai/schemastery' dsh`，或在 60 表如实记"1 处注释命中，无依赖引入"。

### F2【计数失真｜低】负例 evidence 的 MISSING 条数：声称 20，实际 23

- 位置：`60-verification.md:14`（"缺字段 evidence exit 1 且打印 20 条 MISSING"）
- 最小复现：
  ```bash
  bash scripts/verifyProfile.sh --evidence .local/bad-evidence.txt 2>&1 | grep -c '^MISSING'   # 23
  ```
  23 = 2（两个 dump id）+1（socket 0600）+2（POST /tick、/session-event 日志）+17（17 工具名）+1（`[astrbox]` 快照）。
- 影响面：仅证据表计数失真；"缺字段 exit 1 并逐条打印 MISSING"的行为本身已复现。
- 建议：改为 23，或改为"按字段逐条打印（17 工具名 + 6 项字段检查）"。

### F3【冻结判据漂移｜中】C4 的"只有一处实现"在 D6 后不成立，且 30 未随 D6 更新

- 位置：`30-constraints.md:46-53`（C4 判定："`grep -rn "func .*RunTick" internal` 只有一处实现、两处调用"）
- 最小复现：
  ```bash
  grep -rn 'func .*RunTick' internal | grep -v _test
  # internal/mcpserver/runTick.go:18: func (s *Server) RunTick(...)
  # internal/mcpserver/runTick.go:27: func (s *Server) TryRunTick(...)
  ```
- 影响面：D6 新增 `TryRunTick` 后该机器判据按字面不成立；实质不变量（两入口共用同一 `tickMu` 与同一 `runTickLocked` 体）经 `runTick_test.go` 六个用例 + 本次 `-race` 验证成立。40-plan K5 已更新，**30 的 C4 判定文本未更新**，属"冻结后契约文本漂移"。
- 建议：补一条 D 裁决（D6 已覆盖语义，可在 D6 下追加一句），把 C4 判据改为"仅一个共享 tick 体 `runTickLocked`；`RunTick`/`TryRunTick` 两入口共用 `tickMu`"，并重新 freeze。

### F4【判据不可执行｜中】C16 的"删掉 bin/dsh-mcp 后 make doctor 非 0"与 Makefile 依赖冲突

- 位置：`30-constraints.md:193`（C16 判定）；`Makefile`（`doctor: $(DSH_BIN)`）
- 最小复现（推理 + 读 Makefile，不实际删 bin）：`make doctor` 的依赖链会先重建 `bin/dsh-mcp`，因此"删掉 bin 再跑 make doctor"**永远走不到缺失路径**。
- 影响面：C16 的失败路径按原文不可构造；60 表 `:76` 已部分披露（"缺失路径未构造（删除 bin 会被 make 重建）"），但结论栏仍写"由 `make doctor` 依赖构建 + 17 名断言覆盖正常路径"，读起来像判据已满足。该条本身是**降级**状态，故不阻断交付；但判据应改成可执行形式（例如直接 `node scripts/doctorDsh.mjs` 且 `bin/dsh-mcp` 不存在时断言非 0，或在 Makefile 加 `doctor-nobuild` 目标）。
- 建议：修判据或显式记"该判据在本 Makefile 结构下不可构造，失败可观测性由 doctor 的正常路径 + 启动日志承担"，并同步 60 表的结论措辞。

### F5【文档陈旧｜低】40-plan 的"冻结哈希: 待回填"与"待 freeze"未随 `.freeze` 更新

- 位置：`40-plan.md:39,50,63,74,85,96`（K1–K6 均写"冻结哈希: 待 `plan.sh freeze` 回填"）；`40-plan.md:173-178`（6 行变更记录"待 freeze"）
- 实测：`.freeze` 冻结于 `2026-10-25T03:35:10+08:00`，6 个文件哈希与当前内容逐一相等，`plan.sh status` 报"已冻结且未失效"，`plan.sh validate --final` 通过。
- 影响面：实际冻结有效，仅文本误导读者"契约尚未冻结/哈希未回填"。
- 建议：回填文件级哈希或改述为"哈希见 `.freeze`（文件级）"。

### F6【指代错误｜低】C9 证据"`.local/profile-test/dump.txt` 1586 行"的 1586 不属于该文件

- 位置：`60-verification.md:24`（C9 证据）
- 实测：1586 行对应 `.local/probe-dump.txt`（03:16 的 probe 产物）；`.local/profile-test/dump.txt` 在本轮 `--composition` 重跑后为 **1625 行**（我这次运行重写了该文件，属正常隔离区写入）。
- 影响面：仅证据指代错位，数字可在 probe 产物上复现；不推翻 C9。
- 建议：把证据行改成 `.local/probe-dump.txt`（1586 行）并注明 probe 与 composition 是两份 dump。

## 未覆盖项（不判为通过，也不判为失败）

1. **真装（V5）未验证**：`dsh plugin add` + 新会话工具表/快照 `[astrbox]` 未执行，60 表 `:75` 已明确"由用户执行、未回传前 V5 真装部分不算完成"。本复核的"通过"只到 composition / 单元 / stdio e2e 层，**不等于真装通过**。
2. **`make build` / `make test` / `make render-dsh` 未由我整条执行**：它们会写 `bin/` 与 `dsh/cordis.patch.yml`，超出我的写域（只允许写本文件）。其组成命令已分别执行（`go test ./...`、`node --test`、composition 消费现有生成物）；render 的可复现性仅做模板↔生成物人工比对（51 行结构一致、7 处绝对路径=REPO_ROOT），未做"删生成物→重渲染→diff"。
3. **`TestTickBusyImmediate409` 未覆盖"真实 MCP tick 在飞"**：持锁者是 `WithStateWrite`，不是通过 MCP `state_machine_tick` 工具进入的 `RunTick`。两者共用 `tickMu` 使等价性成立（`TestWithStateWriteSerializesWithRunTick`、`TestTryRunTickBusyWhileGateHeld` 分片覆盖），但没有一个测试同时触发两个真实入口。
4. **`.local/evidence-good.txt` 的真装证据未验证真实性**：我只跑了负例；正例文件由谁、在哪个宿主会话产出无法从工作区判定，未采信。
5. **写者归属不可证**：`git status` 只能证明"路径集合在写域内"，不能证明"谁写了哪条"；且 mtime 显示 D6 修复窗口（03:05–03:15）同时改写了 m1 域（`internal/mcpserver/runTick.go`、`runTick_test.go`）与 m2 域（`internal/control/*`），40-plan 未记录该轮修改的写者/写域归属。这属可追溯性缺口，不构成本轮可见的越界（Lead 本身可写全域）。
6. **未纳入本次范围**：`00-scope.md`、`10-generalization.md`、`20-attack-log.md` 的实质内容；`docs/` 全文正确性（仅抽查信任边界与命令）；17 工具 `inputSchema` 的逐字段兼容性。

## 复核方法说明（独立性）

- 未阅读实现者对话，未复用证据表的命令包装：所有数字来自本轮独立执行。
- 对证据表采取"逐条复现 + 反向找反例"：先复现，再用 `grep`/源码阅读检查证据行引用的锚点（如 `tick_test.go:60`、`sessionEvent_test.go:65`、`concurrency_test.go:90`、`socket_test.go:13`、`e2eLifecycle_test.go:16,29,58,96,166,189`），全部命中。
- 唯一由我造成的工作区/隔离区变化：`bash scripts/verifyProfile.sh --composition` 重写 `.local/profile-test/`（含 `dump.txt` 变为 1625 行）与 `make doctor` 使用 `/tmp` 临时目录；均为任务允许的写入范围。
