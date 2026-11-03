# 集成验证与反作弊核对（web-monitor）

- 执行: m7（gate-docs），2026-10-27（Asia/Shanghai）
- 被验仓库: `/home/ruicat/Desktop/Neko`，git short HEAD = `66ff73a`
- 规则：**无证据的「完成」一律按未完成处理**；`UNVERIFIED` = 没跑/跑不了，不等于通过，不得当作通过。
- 输出口径：下文 `实际输出` 均为原样粘贴（长输出取关键行并标「节选」）。
- 复算环境：`GOCACHE=$PWD/.local/go-cache`、`GOMODCACHE=$PWD/.local/gomodcache`（`Makefile` 已默认导出）。

## 模块完成证据（父体填写）

| 子任务 | 声称 | 验证命令 | 实际输出 | 结论 |
|---|---|---|---|---|
| m1 | — | — | — | 由父体填写 |
| m2 | — | — | — | 由父体填写 |
| m3 | — | — | — | 由父体填写 |
| m4 | — | — | — | 由父体填写 |
| m5 | — | — | — | 由父体填写 |
| m6 | — | — | — | 由父体填写 |
| m7 | Makefile/反例/文档/证据表已交付 | 见 §一 | 见 §一 | 见 §三、§四 |

## 一、总门禁

| 检查 | 命令 | 退出码 | 实际输出（节选） |
|---|---|---|---|
| Go + node 测试 | `make test` | 0 | 16 个 Go 包 `ok`；`node --test`：`tests 42` / `pass 42` / `fail 0` |
| 静态与模块化 | `make lint` | 0 | `go vet ./...` 无输出；`扫描 197 个源码文件`；`结果：0 项违规，0 项告警` |
| MCP 工具集 | `make doctor` | 0 | `OK: doctor 通过（17 tools, socket 0600, SIGTERM 清理；data=/tmp/dsh-mcp-doctor-MMH4vv/data）` |
| 设计门禁（正例） | `bash ~/.dsh/skills/ui-composition/scripts/lint-ui-plan.sh --plan-dir design --tokens design/tokens.json --src-root internal/monitor/web` | 0 | `lint-ui-plan: 通过（退出码 0）`；JSON：`"scan": {"ok": true, "reason": "扫描面已覆盖全部已登记 source"}` |
| 设计门禁（4 反例） | 见 §二 C17 | 4/4 = 1 | 每个反例输出 1 条目标违规（G4/G5/G6/G7），见 C17 |
| 页面与健康检查 | 起 `bin/dsh-mcp` 后 `curl /monitor/`、`/monitor/api/health` | — | `page_code=200`、`health_code=200`、`version=66ff73a` |
| 设计源触发重建 | `touch design/tokens.json && make -q build; echo $?` | 1 | `q_build_rc=1` |
| 页面资产触发重建 | `touch internal/monitor/web/index.html && make -q build; echo $?` | 1 | `q_build_web_rc=1` |
| 构建包含生成步骤 | `make -n build` | — | `python3 design/dist/genTokens.py`；`cp design/dist/tokens.css internal/monitor/web/tokens.css` |
| 离线构建 | `GOPROXY=off make build` | 0 | `go build -ldflags "-X main.version=66ff73a" -o bin/astrbox-mcp …`；`… -o bin/dsh-mcp …` |
| 版本注入 | `curl -s http://127.0.0.1:8765/monitor/api/health` | — | `"version":"66ff73a"`（= `git rev-parse --short HEAD`） |

## 二、约束终检（C1–C24）

### C1 单写者 / 同进程

```bash
$ go test ./cmd/dsh-mcp -run TestMonitorSameRuntimeAndLock -count=1 -v
testing: warning: no tests to run
PASS
ok  	github.com/ruicat/astrbox/cmd/dsh-mcp	0.395s [no tests to run]
```

结论：**UNVERIFIED**。冻结判定引用的 `TestMonitorSameRuntimeAndLock` **不存在**；`go test` 对不存在的测试返回 0（假绿），不能作为证据。
替代证据（实际运行）：

```bash
# 第二个 dsh-mcp 复用同一 data-dir
$ ./bin/dsh-mcp -data-dir /tmp/locktest2.FgEk6J/data -scripts-dir /tmp/locktest2.FgEk6J/scripts -monitor-addr ""
2026/10/27 00:41:08.405162 dsh-mcp: 数据目录已被另一个进程占用（pid 5）：/tmp/locktest2.FgEk6J/data
2026/10/27 00:41:08.405185 dsh-mcp: 如确认无进程使用，可执行 rm /tmp/locktest2.FgEk6J/data/.engine.lock 后重试
second_dsh_rc=3
```

```bash
$ go test ./cmd/dsh-mcp -run 'TestAcquireEngineLockIsExclusive|TestE2EDoubleInstanceExitsThree' -count=1 -v
--- PASS: TestE2EDoubleInstanceExitsThree (0.03s)
--- PASS: TestAcquireEngineLockIsExclusive (0.00s)
PASS
ok  	github.com/ruicat/astrbox/cmd/dsh-mcp	0.447s
```

**越界发现 F1**：C1 的另一条判定（`astrbox-mcp` 在已被 `dsh-mcp` 锁住的 data-dir 上必须失败）实测**不成立**：

```bash
$ ./bin/astrbox-mcp -data-dir /tmp/locktest.903IGI/data -scripts-dir /tmp/locktest.903IGI/scripts -transport stdio < /dev/null
astrbox_on_locked_data_rc=0
```

`cmd/astrbox-mcp/main.go:1-27` 只解析参数并 `bootstrap.New`，未取 `.engine.lock`；同一 data-dir 仍可能有两个可写进程（`dsh-mcp` + `astrbox-mcp`）。只提交，未处理（m3 写域）。

### C2 OnCreate 回调不取 Store 锁

```bash
$ go test ./internal/monitor -run TestOnCreateCallbackNoStoreCall -count=1 -v
--- PASS: TestOnCreateCallbackNoStoreCall (0.00s)
PASS
ok  	github.com/ruicat/astrbox/internal/monitor	0.019s
```

「把回调改为 `store.ListAll()` 时必须超时失败」的**有牙验证未跑**（UNVERIFIED：需改 `internal/monitor/server.go`，超出 m7 写域）。

### C3 被观察包不 import monitor

```bash
$ grep -rn 'astrbox/internal/monitor' internal/mcpserver internal/diagnostics internal/bootstrap
（无输出，rc=1）

$ go list -f '{{join .Imports "\n"}}' ./internal/mcpserver ./internal/diagnostics ./internal/bootstrap | grep -c 'internal/monitor'
0
```

结论：通过。

### C4 tick 观察者非阻塞

```bash
$ go test ./internal/monitor -run TestTickObserverNonBlocking -race -count=1 -v
--- PASS: TestTickObserverNonBlocking (0.01s)
PASS
ok  	github.com/ruicat/astrbox/internal/monitor	1.042s
```

### C5 保存失败回滚或 dirty

```bash
$ go test ./internal/monitor -run TestSetSaveFailureRollback -count=1 -v
--- PASS: TestSetSaveFailureRollback (0.00s)
PASS
```

测试断言：保存失败必须 `500`、不得 `saved:true`，且「内存值回到写前值」或响应含 `dirty:true`（`internal/monitor/write_test.go:35-55`）。

### C6 gate 争用有界

```bash
$ go test ./internal/monitor -run TestGateBusyBounded -count=1 -v
--- PASS: TestGateBusyBounded (2.01s)
PASS
```

### C7 鉴权矩阵

```bash
$ go test ./internal/monitor -run TestAuthMatrix -count=1 -v
--- PASS: TestAuthMatrix (0.00s)
PASS
```

运行期矩阵（token=`abcdefghijklmnop1234`）：

```text
page_no_token=200
entities_no_header=401
entities_xauth=200
entities_bearer=200
health_readonly={..., "monitor":{"readOnly":false}, ...}
# 无 token 形态（只读）
tick_no_token_code=403
tick_no_token_body={"error":{"code":"write_disabled","message":"未配置 token，监视器为只读"},"ok":false}
```

### C8 反 DNS rebinding / 跨源

```bash
$ go test ./internal/monitor -run TestHostAllowlist -count=1 -v
--- PASS: TestHostAllowlist (0.00s)
$ go test ./internal/monitor -run TestWSOriginRejected -count=1 -v
--- PASS: TestWSOriginRejected (0.00s)
```

```text
api_host_evil=403
acao_count=0
```

### C9 WS 序号契约

```bash
$ go test ./internal/monitor -run TestWSSeqContract -race -count=1 -v
--- PASS: TestWSSeqContract (0.01s)
PASS
```

### C10 监听器只注册一次

```bash
$ grep -rn "AddChangeListener" internal/monitor/*.go
internal/monitor/server.go:107:		ent.AddChangeListener(listener)
internal/monitor/server.go:110:	ent.AddChangeListener(listener)
```

2 处调用（构造循环 + `OnCreate`），均不在连接处理文件里。**UNVERIFIED**：冻结判定引用的 `TestReconnectNoLeak` 不存在；实测 `go test -run TestReconnectNoLeak` 输出 `testing: warning: no tests to run` + `PASS`（假绿）。goroutine/监听器不增长无测试证据。

### C11 诊断帧语义

```bash
$ go test ./internal/monitor -run TestDiagnosticsFrameSemantics -count=1 -v
--- PASS: TestDiagnosticsFrameSemantics (0.00s)
```

### C12 诊断通知锁外回调

```bash
$ go test ./internal/monitor -run TestNotifyNotUnderLock -count=1 -v
--- PASS: TestNotifyNotUnderLock (0.00s)
```

「改成锁内回调必须超时」的有牙验证未跑（UNVERIFIED：需改 `internal/diagnostics/store.go`，超出 m7 写域）。

### C13 日志读取护栏

```bash
$ go test ./internal/monitor -run TestLogsGuards -count=1 -v
--- PASS: TestLogsGuards (0.02s)
```

### C14 path 语义与 /state 一致

```bash
$ go test ./internal/monitor -run TestPathParityWithControl -count=1 -v
--- PASS: TestPathParityWithControl (0.00s)
```

### C15 maxBytes 截断确定性

```bash
$ go test ./internal/monitor -run TestMaxBytesDeterministic -count=1 -v
--- PASS: TestMaxBytesDeterministic (0.00s)
```

### C16 脚本源只走 registry.ReadScript

```bash
$ grep -rn "os.ReadFile" internal/monitor/*.go
internal/monitor/write_test.go:116:	raw, err := os.ReadFile(today)
```

唯一命中在测试断言里，不是脚本源读取路径。`TestScriptSourceGuards`：

```bash
$ go test ./internal/monitor -run TestScriptSourceGuards -count=1 -v
--- PASS: TestScriptSourceGuards (0.00s)
```

### C17 前端门禁可执行且扫真实源码

正例：

```bash
$ bash ~/.dsh/skills/ui-composition/scripts/lint-ui-plan.sh --plan-dir design --tokens design/tokens.json --src-root internal/monitor/web
lint-ui-plan: 通过（退出码 0）
PASS_RC=0
```

4 个反例（`--plan-dir <fixture> --src-root <fixture>`，退出码全 1）：

```text
design/fixtures/fail/g4-unregistered-component/ rc=1
G4	…/composition.json:10	节点 'n1' 的 component='Ghost.Panel' 未在 components.json 登记（P1）
design/fixtures/fail/g5-hardcoded-style/ rc=1
G5	…/src/widgetBox.js:5	具体视觉值 #ff0000（应改为 semantic 令牌 / getToken）
design/fixtures/fail/g6-primitive-ref/ rc=1
G6	…/src/widgetBox.js:5	CSS 变量 --primitive-blue-500 对应已登记的 primitive 令牌 'primitive.blue.500'（元件层不得直连原始层）
design/fixtures/fail/g7-composition-atomic/ rc=1
G7	…/src/sceneMain.js:5	组合层用 createElement('button') 手搓原生控件（应改为已登记元件）
```

**UNVERIFIED**：判定里「向真实 `internal/monitor/web/*.css|*.js` 注入硬编码色值后必须 exit 1」未跑——该动作要写 m6 写域文件，超出 m7 权限；门禁扫描面由正例 JSON 的 `scan.ok=true`（`扫描面已覆盖全部已登记 source`）与 4 反例间接证明。

### C18 `go:embed` 生成物入库且可复现

```bash
$ test -n "$(git ls-files internal/monitor/web)"; echo $?
1

$ make design && git diff --exit-code -- internal/monitor/web design/dist; echo $?
design_diff_rc=0

$ strings bin/dsh-mcp | grep -c '<title>'
1
$ strings bin/astrbox-mcp | grep -c '<title>'
0

$ tmp=$(mktemp -d); git archive HEAD | tar -x -C "$tmp"; cd "$tmp" && go test ./internal/monitor/ -run TestNoop
# ./internal/monitor
stat /tmp/clean.FUfXUn/internal/monitor: directory not found
FAIL	./internal/monitor [setup failed]
```

结论：**未通过 / UNVERIFIED**。`design/`、`internal/monitor/web/` 当前**未被 git 跟踪**（`git ls-files` 为空），HEAD 里没有 `internal/monitor`，干净树编译不成立；`make design` 后 `git diff` 为 0 只说明未跟踪文件不出现在 diff 里，**不能**当作「已入库」。需由 Lead 在集成时 `git add -A design internal/monitor/web` 后复算本条。

### C19 build 前置含设计真源与 web 资产

```bash
$ touch design/tokens.json && make -q build; echo $?
1
$ touch internal/monitor/web/index.html && make -q build; echo $?
1
$ make -n build
python3 design/dist/genTokens.py
cp design/dist/tokens.css internal/monitor/web/tokens.css
mkdir -p bin
go build -ldflags "-X main.version=66ff73a" -o bin/astrbox-mcp ./cmd/astrbox-mcp
mkdir -p bin
go build -ldflags "-X main.version=66ff73a" -o bin/dsh-mcp ./cmd/dsh-mcp
```

### C20 版本可追溯 + 命令无占位符

版本：

```text
health.version=66ff73a
git rev-parse --short HEAD=66ff73a
```

占位符检查：

```bash
$ grep -nE '<[a-z][a-z-]*>' runs/web-monitor/00-scope.md runs/web-monitor/15-interfaces.draft.md | grep -v '<string>\|<int>\|<bool>\|<abs>\|<JSON>\|<token>\|<id>\|<path>\|<name>\|<rel>\|<sha1>\|<text'
runs/web-monitor/00-scope.md:27:| V6 | 前端分解可机检 | `bash <ui-composition>/scripts/lint-ui-plan.sh --plan-dir design` 退出 0；至少 4 条反例退出非 0 | 主智能体 |
runs/web-monitor/15-interfaces.draft.md:126:- 客户端重连可带 `?resumeSeq=<n>`；服务端只保证 `seq > resumeSeq` 的增量，缺口过大（> 1024）时改发 `snapshot`。
```

结论：**未通过（2 处占位符）**。两文件不在 m7 写域，未处理；**越界发现 F2**：需由 m1 修订 `00-scope.md:27`、`15-interfaces.draft.md:126`（或由 Lead 裁决这两处是否为文档性残留）。

### C21 WS 依赖离线可复现

```bash
$ grep -n 'github.com/coder/websocket' go.mod go.sum | head -3
go.mod:6:	github.com/coder/websocket v1.8.14
go.sum:1:github.com/coder/websocket v1.8.14 h1:9L0p0iKiNOibykf283eHkKUHHrpG7f65OE3BhhO7v9g=
go.sum:2:github.com/coder/websocket v1.8.14/go.mod h1:NX3SzP+inril6yawo5CQXx8+fk145lPDC6pumgx0mVg=

$ GOPROXY=off make build; echo $?
offline_build_rc=0
```

### C22 回归门禁三命令 + 路径可移植

```bash
$ make test && make lint && make doctor; echo $?
TEST_RC=0
LINT_RC=0
DOCTOR_RC=0
OK: doctor 通过（17 tools, socket 0600, SIGTERM 清理；data=/tmp/dsh-mcp-doctor-MMH4vv/data）
```

`MODULARITY_LINT` 可覆盖、且覆盖后与 `$HOME` 无关：

```bash
$ HOME=/tmp/nohome MODULARITY_LINT=/home/ruicat/.dsh/skills/code-modularity/scripts/lintCodeModularity.sh make lint; echo $?
结果：0 项违规，0 项告警
home_lint_rc=0
```

**未通过项 / 冲突声明**：`HOME=/tmp/nohome make lint`（不给覆盖）退出 2：

```text
缺少 /tmp/nohome/.dsh/skills/code-modularity/scripts/lintCodeModularity.sh（code-modularity 技能未安装；可用 MODULARITY_LINT=... 覆盖）
make: *** [Makefile:62：lint] 错误 1
```

这是按本轮明确要求「默认仍 `$(HOME)/.dsh/...`」执行的直接结果，与 C22「在 `HOME=/tmp/nohome` 下仍可运行」冲突。按规则优先级取用户本轮要求；C22 的可移植性通过 `MODULARITY_LINT` 环境变量满足（已给实际输出）。若要无覆盖也可运行，需把技能装进仓库内路径或改默认解析。

### C23 审计日志轮转 / 不泄 token / 写失败不挡请求

```bash
$ go test ./internal/monitor -run TestAuditLog -count=1 -v
--- PASS: TestAuditLog (0.00s)
```

### C24 tick 载荷单一来源

```bash
$ go test ./internal/monitor -run TestTickPayloadParity -count=1 -v
--- PASS: TestTickPayloadParity (0.00s)
```

## 三、约束汇总

| 约束 | 结论 | 证据位置 |
|---|---|---|
| C1 | 部分通过（单写者实测；原判定测试缺失） | §二 C1 + F1 |
| C2 | 通过（有牙验证未跑） | §二 C2 |
| C3 | 通过 | §二 C3 |
| C4 | 通过（-race） | §二 C4 |
| C5 | 通过 | §二 C5 |
| C6 | 通过 | §二 C6 |
| C7 | 通过 | §二 C7 |
| C8 | 通过 | §二 C8 |
| C9 | 通过（-race） | §二 C9 |
| C10 | 部分通过（原判定测试缺失） | §二 C10 |
| C11 | 通过 | §二 C11 |
| C12 | 通过（有牙验证未跑） | §二 C12 |
| C13 | 通过 | §二 C13 |
| C14 | 通过 | §二 C14 |
| C15 | 通过 | §二 C15 |
| C16 | 通过 | §二 C16 |
| C17 | 通过（注入子项 UNVERIFIED） | §二 C17 |
| C18 | **未通过 / UNVERIFIED（生成物未入库）** | §二 C18、F3 |
| C19 | 通过 | §二 C19 |
| C20 | **未通过（2 处占位符）** | §二 C20、F2 |
| C21 | 通过 | §二 C21 |
| C22 | 三命令通过；HOME 无覆盖子项冲突 | §二 C22 |
| C23 | 通过 | §二 C23 |
| C24 | 通过 | §二 C24 |

## 四、越界发现（只提交，不处理）

| # | 发现 | 最小复现 | 影响面 | 建议 |
|---|---|---|---|---|
| F1 | `astrbox-mcp` 不取 `.engine.lock`，可与 `dsh-mcp` 同写一个 data-dir | 见 C1 实际输出 | C1 单写者保证只覆盖 `dsh-mcp` | m3 决定是否让 `astrbox-mcp` 也取锁，或把该入口标注为 legacy |
| F2 | `00-scope.md:27`、`15-interfaces.draft.md:126` 含 `<ui-composition>`、`<n>` 占位符 | 见 C20 | C20 判定不过、命令不可复制 | m1 修订（不在 m7 写域） |
| F3 | `design/`、`internal/monitor/web/` 未被 git 跟踪 | `git ls-files internal/monitor/web` 为空 | C18 不成立、干净树无法构建 | Lead 集成时提交生成物与页面 |
| F4 | C1/C10 冻结判定引用的测试不存在，`go test -run` 返回 0（假绿） | 见 C1/C10 | 门禁会静默放行 | m3 补 `TestMonitorSameRuntimeAndLock` / `TestReconnectNoLeak`，或改判定命令 |
| F5 | `HOME=/tmp/nohome make lint`（无覆盖）失败 | 见 C22 | 换用户/CI 环境 lint 失败 | 与用户确认是否把默认路径改为仓库内解析 |

## 五、反作弊核对

| 检查 | 结论 |
|---|---|
| 是否有「声称完成但无实际输出」的模块 | 本文件所有「通过」均附实际命令输出；未跑项标 `UNVERIFIED` |
| 是否有子任务越界写了别人写域的文件 | m7 未写实现代码/design 三件套；仅写 `design/fixtures`、`Makefile`、`docs/web-monitor.md`、本文件、生成的 `internal/monitor/web/tokens.css` |
| 是否有契约被单方面改动（冻结哈希不一致） | 未改 `40-plan.md` / `30-constraints.md` / 三件套；`.freeze` 未动 |
| 是否有验收命令依赖其它子任务尚未完成的产物 | C18 依赖 Lead 提交；其余不依赖未完成产物 |
| 是否放宽既有判据 | 未放宽 `make test/lint/doctor`；未使用 `|| true` |

## 六、输入变更与失效记录

| 日期 | 变更的输入文件 | 因此失效的门 | 重跑结果 |
|---|---|---|---|
| 2026-10-27 | `Makefile`（前置/ldflags/design/monitor/MODULARITY_LINT） | build、lint、doctor | 全部重跑，见 §一 |
| 2026-10-27 | `design/fixtures/fail/**`（新增 4 反例） | C17 反例门 | 4/4 非 0，见 C17 |
| 2026-10-27 | `internal/monitor/web/tokens.css`（`make design` 生成） | 页面 curl、build | `page_code=200`、build 0 |
