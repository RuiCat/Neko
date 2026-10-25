# 集成验证与反作弊核对（dsh-mcp-plugin）

- 创建: 2026-10-25
- 规则：**无证据的「完成」一律按未完成处理**，回退重做。
- 口径：下表「实际输出」均为 **Lead 本人** 在本仓库执行的命令输出（不是执行体自述）；真装部分明确标「待用户」。

## 模块完成证据

| 子任务 | 声称 | 验证命令 | 实际输出 | 结论 |
|---|---|---|---|---|
| m1 | 共享装配 + 快照 + 共享 tick | `go test ./... -count=1`；`go test -race ./internal/entity ./internal/mcpserver ./cmd/astrbox-mcp`；`go vet ./...`；`gofmt -l cmd internal`；`lintCodeModularity.sh internal cmd` | 15 包全 `ok`（cmd/astrbox-mcp 20.3s、registry 5.3s）；`-race` 三包 `ok`；vet/gofmt 空；lint「0 项违规，0 项告警」 | 通过 |
| m2 | `cmd/dsh-mcp` + UDS 控制面 + 生命周期 | `go test ./internal/control/... ./cmd/dsh-mcp/... -count=1`；`go test -race …`；`go build -o bin/dsh-mcp`；`./bin/dsh-mcp -h` | control 0.13s、dsh-mcp 0.69s 全 `ok`；`-race` 两包 `ok`（10.3s）；build/help exit 0；e2e 含 `TestTickBusyImmediate409`（409 用时 <500ms，带 1s 安全释放避免排队回归时挂死）、`TestE2EDoubleInstanceExitsThree`、`TestE2EKillNineSelfHeals`、`TestE2EStdioToolsListAndStdoutPurity` | 通过 |
| m3 | 零构建 bundle + 静态提示段 + pre-step 快照 + Tick/会话事件 | `node --test dsh/lib/*Test.mjs`；`grep -nE '/home/\|/Users/' dsh/cordis.patch.yml.template`；`lintCodeModularity.sh dsh` | `tests 42 / pass 42 / fail 0`；模板绝对路径计数 **0**；dsh lint「12 文件，0 违规」 | 通过 |
| m4 | Makefile/render/verify/doctor/文档 | `make build`、`make test`、`make lint`、`make render-dsh`、`bash scripts/verifyProfile.sh --composition`、`make doctor`、负例 evidence | 全部 exit 0；composition 输出 `OK: composition checks passed (mcp-astrbox=1, astrbox-bridge=1, command=/home/ruicat/Desktop/Neko/bin/dsh-mcp, dump=.../dump.txt)`；doctor 输出 `OK: doctor 通过（17 tools, socket 0600, SIGTERM 清理）`；缺字段 evidence exit 1 且打印 **23** 条 MISSING（2 dump id + 1 socket + 2 日志字段 + 17 工具名 + 1 快照） | 通过 |

## 契约遵守核对

| 契约 | 核对点（错误语义 / 不变量 / 格式） | 结果 | 证据 |
|---|---|---|---|
| K1 | UDS 0600；四端点；400/404/405/409/500(dirty)/503/504；/tick 立即 409；/session-event 可选 function；写经 WithStateWrite；日志按日保留 7 天 | 通过 | `internal/control/tick_test.go:60` 409 <1s；`sessionEvent_test.go:65` named function；`concurrency_test.go:90` 日志清理；`socket_test.go:13` 0600 |
| K2 | 静态 section 常量；动态仅 pre-step ≤1/轮；cwd 边界；去重 256；配置校验指名键；默认 paths 非 R18 | 通过 | `dsh/lib/pluginTest.mjs`、`configTest.mjs`、`sessionEventsTest.mjs`、`promptTest.mjs`；42 tests pass |
| K3 | flags/env 默认；stdout 只有 MCP 帧；退出码 0/2/3/4；flock；stale socket 自愈；SIGTERM 清理 | 通过 | `cmd/dsh-mcp/e2eLifecycle_test.go:16,29,58,96,166,189`；`TestE2EStdioToolsListAndStdoutPurity` |
| K4 | 隔离 DSH_HOME composition；证据字段集；不耗模型调用 | 通过 | `verifyProfile.sh --composition` exit 0；负例 evidence exit 1；脚本未调用任何模型 |
| K5 | Snapshot；RunTick/TryRunTick/WithStateWrite 同一 tickMu；不可重入；MCP tick 语义不变 | 通过 | `internal/mcpserver/runTick_test.go` 六个用例；`go test -race ./internal/mcpserver` ok |
| K6 | 17 工具名冻结 | 通过 | `doctor` 断言 17 名；`cmd/dsh-mcp` e2e tools/list 同样 17 名 |

## 拓扑集成记录（按依赖顺序）

| 顺序 | 步骤 | 命令 | 结果 | 备注 |
|---|---|---|---|---|
| 1 | 构建双二进制 | `make build` | exit 0 | `bin/astrbox-mcp` + `bin/dsh-mcp` |
| 2 | 全量 Go 测试 | `make test`（go 部分） | exit 0 | 15 包 ok |
| 3 | Node 单测 | `make test`（node 部分） | exit 0 | 42 pass |
| 4 | 静态检查 | `make lint` | exit 0 | vet/gofmt/lint 全绿（149 文件 0 违规） |
| 5 | 渲染 bundle patch | `make render-dsh` | exit 0 | 生成 `dsh/cordis.patch.yml`（已 gitignore） |
| 6 | 隔离组合验证 | `bash scripts/verifyProfile.sh --composition` | exit 0 | 两 id 各 1，command 可执行 |
| 7 | 加载面 + socket 生命周期 | `make doctor` | exit 0 | 17 tools、0600、SIGTERM 清理 |
| 8 | 证据脚本负例 | `verifyProfile.sh --evidence .local/bad-evidence.txt` | exit 1 | 打印 MISSING，符合判据 |

## 反作弊核对

| 检查 | 结论 |
|---|---|
| 是否有「声称完成但无实际输出」的模块 | 无：上表每条都由 Lead 重跑并记录输出；执行体自述未直接采信 |
| 是否有子任务越界写了别人写域的文件 | `git status --porcelain` 只出现预期路径：m1（`cmd/astrbox-mcp`、`internal/{bootstrap,mcpserver,entity}`）、m2（`cmd/dsh-mcp`、`internal/control`）、m3（`dsh/`）、m4（`Makefile`、`scripts/`、`docs/dsh-plugin.md`、`.gitignore`、`README.md`、生成的 `dsh/cordis.patch.yml`）。m4 报告并实测模板 mtime 未变 |
| 是否有契约被单方面改动（冻结哈希不一致） | 有 3 次**显式**裁决：D4（WithStateWrite）、D5（C6/C16 口径 + 测试命名）、D6（TryRunTick + function 字段）；已同步 30/40/50 并重新 freeze（见 .freeze） |
| 是否有验收命令依赖了其它子任务尚未完成的产物 | 无：m1/m3 独立；m2 依赖 m1 的 K5（先完成）；m4 依赖 m2/m3（先完成） |
| 生成物是否混入版本库 | 无：`git check-ignore` 确认 `dsh/cordis.patch.yml`、`.local/`、socket、lock 均被忽略 |

## 约束终检（对 `30-constraints.md` 逐条复核）

| 约束 | 终检判定 | 证据 |
|---|---|---|
| C1 | 满足 | `dsh/lib/plugin.mjs` 静态段 `text: () => STATIC_USAGE_TEXT`；动态仅 `agent/pre-step`；`node --test` 42 pass（含静态段 3 次同值/零请求、每轮 ≤1） |
| C2 | 满足 | `sessionEventsTest.mjs`：cwd undefined 不抛、`<prefix>-old` 不命中、重复事件 1 次、表 300→256、同步抛错被吞 |
| C3 | 满足 | `Entity.Snapshot`（model.go）、`saveLocked`/`entity_state_get` 改快照；`go test -race ./...` 全绿；`grep '"数据": ent.Data' internal/mcpserver` = 0 |
| C4 | 满足 | `RunTick`/`TryRunTick`/`WithStateWrite` 共用 `tickMu`；`TestTickBusyImmediate409`（<1s）；`TestRunTickConcurrentWithSetAndSnapshot` |
| C5 | 满足 | e2e 双实例 exit 3（含 pid）、`kill -9` 自愈、SIGTERM 后 socket 消失；doctor 复验 |
| C6 | 满足 | `config.mjs` tick 15000 / state 2000；`TestTickSaveFailureIsDirty`；K1 500 `dirty:true` |
| C7 | 满足 | `TestTickUnchangedSkipsSave`；`TestLoggerPrunesOldControlLogs`；日志名 `dsh-control-YYYYMMDD.log` |
| C8 | 满足 | 模板绝对路径 0；生成 patch 两 id 各 1；`command` `/home/ruicat/Desktop/Neko/bin/dsh-mcp` 可执行；生成物 gitignore |
| C9 | 满足 | composition/doctor/负例 evidence 三条命令 exit 0/0/1；全流程无模型调用 |
| C10 | 满足 | doctor 与 dsh-mcp e2e 均断言 17 名（冻结清单） |
| C11 | 满足 | `configTest.mjs` 坏键/坏类型/负数均抛错并指名键；`grep -rn "@deepseek-ai/schemastery" dsh` = 0（`config.mjs:4` 有一处注释提及，不构成依赖，`package.json` 无 dependencies） |
| C12 | 满足 | `promptTest.mjs` 默认 paths 无 R18、超长截断、固定前缀、只遍历白名单 |
| C13 | 降级 | socket 0600（doctor `stat`）；无 token；同 uid 信任域文档见 `docs/dsh-plugin.md` |
| C14 | 降级 | 未 chmod `data/`；socket 0600 由 `TestListenAppliesSocketPermission` 覆盖 |
| C15 | 降级 | 无 schemastery 依赖；`pluginTest.mjs` 用 stub ctx 做 import+apply 加载面 |
| C16 | 降级 | patch 未设 `failOnStartupError`；`node scripts/doctorDsh.mjs` 在 `bin/dsh-mcp` 缺失时非 0（临时移走二进制实测，见「复核发现与处置」F4），正常路径断言 17 tools + socket 0600 + SIGTERM 清理 |

## 独立复核与处置（`65-review.md`）

独立复核者 `verify-dsh-mcp`（未参与实现）独立重跑关键门后结论：**通过（附 6 条勘误，均不推翻实质结论）**。Lead 逐条处置如下：

| # | 复核发现 | 处置 | 复核证据 |
|---|---|---|---|
| F1 | C11 称 `grep schemastery dsh`=0，实际 1（`config.mjs:4` 注释） | 已把判据改为 `grep -rn "@deepseek-ai/schemastery" dsh`=0，并在 60 表注明“注释提及不算依赖”；`package.json` 无 dependencies | `grep -rn '@deepseek-ai/schemastery' dsh` 无输出 |
| F2 | 负例 evidence 的 MISSING 声称 20，实际 23 | 60 表改为 23 并列出构成 | `verifyProfile.sh --evidence .local/bad-evidence.txt` → exit 1，23 条 MISSING |
| F3 | C4 判定“只有一处 RunTick”在 D6 后不成立 | 30 的 C4 判定改为“两个入口共用 `runTickLocked` 与 `tickMu`”；D6/D7 记录 | `grep -rn 'func .*RunTick' internal`（非测试）= 2；`grep -n runTickLocked internal/mcpserver/runTick.go` 命中两处调用 |
| F4 | C16 的“删 bin 后 make doctor 非 0”被 Makefile 依赖重建抵消 | C16 判定改为直接 `node scripts/doctorDsh.mjs`；实测移走 `bin/dsh-mcp` 后 exit 1、提示“缺少二进制…（先运行 make build）”，随后恢复 | `doctor_missing_exit=1`；`bin/dsh-mcp` 已恢复（14387645 bytes） |
| F5 | 40-plan 的“冻结哈希待回填/待 freeze”陈旧 | K1–K6 改为“见 `.freeze`（文件级哈希）”；变更记录改为“已 freeze”；随后重新 freeze | `.freeze` 时间戳与 6 文件哈希 |
| F6 | C9 证据的 1586 行属于 probe dump，不是 composition dump | 30 的 C9 证据去掉行数（行数随 profile 内容变化，不作判据） | `.local/profile-test/dump.txt` 由 composition 重写（1625 行），两 id 计数仍各 1 |
| 附加 | `TestTickBusyImmediate409` 在排队回归时会挂起而非失败；持锁入口是 `WithStateWrite` 而非真实 MCP tick | 测试加 1s 安全释放与 500ms 上界：排队回归时返回后超时失败；`TestWithStateWriteSerializesWithRunTick`/`TestTryRunTickBusyWhileGateHeld` 分片覆盖两入口共用 `tickMu` | `go test ./internal/control -run TestTickBusyImmediate409 -count=1 -v` → PASS 0.00s |

复核者未覆盖项（不判通过）：真装 `dsh plugin add` + 新会话快照（用户执行）；`make build/test/render` 整条链由 Lead 执行；真实 MCP tick 在飞时的 409 单测（由等价分片覆盖）；`evidence-good` 真实性；写者归属（`git status` 只能证路径集合）。

## 未通过项与回退

| 未通过项 | 回退层级（G0 / G1 / G3 / 重做本模块） | 处理 |
|---|---|---|
| 真装（`dsh plugin add` + 新会话工具表/快照） | 不适用（本会话 `/home` 只读） | 由用户执行 `docs/dsh-plugin.md` 安装段，并用 `verifyProfile.sh --evidence` 回传证据；未回传前 V5 的真装部分不算完成 |
| `doctor` 在二进制缺失时的失败路径 | 不适用（C16 降级） | 由 `make doctor` 依赖构建 + 17 名断言覆盖正常路径；缺失路径未构造（删除 bin 会被 make 重建） |

## 验证台账（只跑受影响的）

| 门 | 状态（CLEAN/STALE/NEVER/ALWAYS） | 输入指纹 | 上次结果 | 本次动作（跑/SKIP） |
|---|---|---|---|---|
| go-test | CLEAN | `cmd/**/*.go`、`internal/**/*.go`、`go.mod` | 15 包 ok | 跑 |
| go-race | CLEAN | 同上 | 15 包 ok | 跑 |
| go-lint | CLEAN | 同上 + lint 脚本 | 0 违规 | 跑 |
| node-test | CLEAN | `dsh/lib/*.mjs` | 42 pass | 跑 |
| compose | CLEAN | `dsh/cordis.patch.yml(.template)`、profile 元数据 | 两 id 各 1 | 跑 |
| doctor | CLEAN | `bin/dsh-mcp`、`dsh/cordis.patch.yml` | 17 tools/0600 | 跑 |

## 输入变更与失效记录

| 日期 | 变更的输入文件 | 因此失效的门 | 重跑结果 |
|---|---|---|---|
| 2026-10-25 | D4：`internal/mcpserver` 加 WithStateWrite | go-test/go-race/lint | 全绿 |
| 2026-10-25 | D5：测试文件改名 `*Test.mjs`、C6/C16 口径 | node-test | 42 pass；lint 0 |
| 2026-10-25 | D6：TryRunTick + `/session-event` function 字段 | go-test/go-race、node-test、compose | 全绿；composition 仍两 id 各 1 |
