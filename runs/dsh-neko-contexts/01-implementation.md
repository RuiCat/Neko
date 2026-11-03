# P1 实现记录：DSH 上下文驱动核心

- 日期：2026-11-01
- 仓库：`/home/ruicat/Desktop/Neko`
- 范围：规划 P1（单角色闭环的 Neko 侧核心）——不改 DSH、不改 `~/.dsh` profile
- 上游规划：`runs/dsh-neko-contexts/00-plan.md`

## 0 交付摘要

| 能力 | 状态 | 入口 |
|---|---|---|
| 打开/续接角色上下文（`session/create`，采纳返回的 sessionId） | 已实现 | `internal/driver` |
| 事件投递（`session/prompt`，`queue`/`steer` 选择） | 已实现 | `internal/driver` |
| 停轮复位（`turn-end` → 上下文 `idle`） | 已实现 | `internal/control` 的 `/session-event` |
| 控制面查询/投递 | 已实现 | `GET /contexts`、`POST /context-invoke` |
| CLI 单次驱动 | 已实现 | `bin/neko-context` |
| 401 自动重连（重取 token 换 Cookie） | 已实现 | `internal/driver/client.go` |
| 注册表持久化与重启恢复 | 已实现 | `data/contexts.json` |
| `.neko` @事件 → 上下文路由（含冷却/配额） | 已实现 | `internal/driver/routes.go`、`internal/events/bus.go` |
| turn-end 摘要（助手文本/工具数/步数） | 已实现 | `dsh/lib/turnSummary.mjs`、`internal/control/sessionEvent.go` |
| 跨上下文投递 `context_send`（MCP，来源显式） | 已实现 | `internal/driver/send.go`、`internal/mcpserver/contextTools.go` |
| 状态/轮次校准（`session/list`） | 已实现 | `internal/driver/status.go` |
| 原生工具自动归属 | **未做**（P3，规划 D4-B） | — |

## 1 文件与职责

| 文件 | 职责 |
|---|---|
| `internal/driver/config.go` | `contexts.json` 加载、env 覆盖、校验、注册表路径解析 |
| `internal/driver/registry.go` | 上下文记录（C0）与原子持久化 |
| `internal/driver/client.go` | DSH HTTP 客户端：`/connect` 换 Cookie、四个 Remote 端点、401 重试 |
| `internal/driver/driver.go` | 打开/续接、事件投递与 `queue|steer` 策略、turn-end 复位、per-context 串行 |
| `internal/driver/routes.go` | `.neko` @事件 → 上下文路由：模板渲染、异步投递、冷却与配额 |
| `internal/events/bus.go` | 事件分发后的 observer 挂点（`SetObserver`，不持锁回调） |
| `dsh/lib/turnSummary.mjs` | 从 Session 事件尾窗提取 turn 摘要（助手文本/工具数/步数） |
| `internal/driver/send.go` | 跨上下文投递：来源/目标校验、60s 去重、每来源每分钟 10 条 |
| `internal/mcpserver/contextTools.go` | `context_list` / `context_send` 两个 MCP 工具（仅 dsh-mcp 注册） |
| `internal/driver/status.go` | 用 `session/list` 校准 status 与 lastTurn |
| `internal/control/sessionEvent.go` | `turn-end` 摘要转成 `.neko` 回调第 5 个 JSON 参数字符串 |
| `internal/control/contexts.go` | `GET /contexts`、`POST /context-invoke`（未启用返回 503） |
| `internal/control/sessionEvent.go` | `turn-end` 时同步更新 driver 状态（无回调也更新） |
| `cmd/dsh-mcp/main.go` | `-contexts-config` 开关、启动时预开上下文（失败只记日志） |
| `cmd/neko-context/main.go` | 单次驱动 CLI（`-list` / `-context -text`） |
| `docs/contexts.md`、`docs/contexts.example.json` | 配置、端点、契约与限制 |
| `dsh/cordis.patch.yml.template` | MCP client 参数追加 `-contexts-config ${REPO_ROOT}/contexts.json` |

## 2 契约落地口径

- **C0 记录**：`ContextID / CharacterID / SessionID / PresetID / Cwd / Entity / Status / LastTurn / LastRequestID`。
- **C1 打开**：只认 `session/create` 返回值；DSH 因冲突换 id 时以返回值为准并持久化。
  进程首次打开会把上次进程遗留的 `running` 复位为 `idle`（本进程尚未投递过）。
- **C2 投递**：`requestId = neko:<contextID>:<eventID>`；`running` + `urgent` 才用 `steer`，
  其余 `queue`；同一 `contextID` 串行（进程内锁）。
- **C3 观测**：沿用 `/session-event` 的 `turn-end`；未扩展 payload（助手摘要留 P2）。

## 3 验证证据

```text
$ make build
go build ... ./cmd/astrbox-mcp
go build ... ./cmd/dsh-mcp
go build ... ./cmd/neko-context          # 退出 0

$ make lint
扫描 210 个源码文件 ... 结果：0 项违规，0 项告警   # 退出 0

$ make doctor
OK: doctor 通过（17 tools, socket 0600, SIGTERM 清理；...）

$ make test
17 个 Go 包 ok，0 FAIL，退出 0（node --test 42 项全过）
```

**测试环境说明**：`make test` 会用 `os.CopyFS` 复制仓库 `data/`；该目录里有一个 2026-10-27 的**无监听残留 socket**
`data/dsh-control.sock`（`ss -xl` 无匹配），CopyFS 复制 socket 会报 `invalid argument`，导致 3 个既有测试失败。
验证时把它临时移到 `/tmp`、跑完再移回（已恢复原位），**未删除**任何运行数据。该测试脆弱点是既有问题，未改测试代码。

### 3.1 CLI 冒烟（对本地假 DSH）

```text
$ ./bin/neko-context -config .local/contexts-smoke.json -context ctx-cat -event evt-1 -text '【neko 事件】smoke'
{
  "ok": true,
  "result": {
    "contextId": "ctx-cat",
    "sessionId": "neko-cat",
    "requestId": "neko:ctx-cat:evt-1",
    "mode": "queue",
    "status": "running"
  }
}
```

注册表落盘：`status=running, lastRequestId=neko:ctx-cat:evt-1`（`.local/contexts-smoke-registry.json`）。

### 3.2 控制面端到端（真实 `bin/dsh-mcp` + 假 DSH）

```text
dsh-mcp: 上下文驱动就绪（1 个上下文，registry=.local/contexts-smoke-registry.json）
GET  /contexts        -> {"ok":true,"contexts":[{"contextId":"ctx-cat",...,"status":"running",...}]}
POST /context-invoke  -> {"ok":true,"result":{"contextId":"ctx-cat","sessionId":"neko-cat",
                         "requestId":"neko:ctx-cat:evt-2","mode":"steer","status":"running"}}
```

（`priority=urgent` 且账本为 `running` → `steer`，符合 C2。）

### 3.3 事件路由（P2 增量）

```text
$ go test ./internal/driver ./internal/events
ok   github.com/ruicat/astrbox/internal/driver
ok   github.com/ruicat/astrbox/internal/events
```

覆盖：模板占位符渲染、默认卡片、2s 冷却跳过、未注册事件忽略、投递失败经 `SetErrorHandler` 上报、
`routes` 指向未声明上下文/非法 priority 被配置校验拒绝。路由在事件分发之后异步执行，不阻塞状态机。

### 3.4 turn-end 摘要（P2 增量）

```text
$ node --test dsh/lib/*Test.mjs
ℹ tests 47 ... pass 47 fail 0
$ go test ./internal/control
ok   github.com/ruicat/astrbox/internal/control
```

覆盖：反向扫描取本轮最后一条助手文本（跨轮过滤）、工具数/步数统计、1200 字符截断、无
`snapshotEvents` 时省略字段（旧请求体兼容）、5 参 `.neko` 回调收到摘要 JSON 字符串。

### 3.5 跨上下文 `context_send`（P3 前移）

```text
$ go test ./internal/driver
ok   github.com/ruicat/astrbox/internal/driver
$ make doctor
OK: doctor 通过（19 tools, socket 0600, SIGTERM 清理；...）
$ ./bin/astrbox-mcp  # 仍为 17 个引擎工具（context 工具只在 dsh-mcp 注册）
```

覆盖：来源/目标校验（未知上下文、自发送、空正文）、同一 `(来源,目标,正文)` 60s 去重、
每来源每分钟 10 条上限、投递到目标会话（正文到达 prompt）。

### 3.6 真实 DSH 联调（一次性隔离实例，2026-11-01）

用隔离 `DSH_HOME` 起一个一次性 `dsh web --port 0 --no-open`，从启动行取 token，实测：

```text
GET  /connect?token=…  -> 302 + dsh-auth-… Cookie
POST /api/session/create -> {"value":{"sessionId":"neko-live-probe"}}   # 首次采纳请求 id
POST /api/session/prompt (evt-1) -> accepted
POST /api/session/prompt (evt-2) -> accepted；session/list 的 sessionStats.turns 1 -> 2
POST /api/session/create 同名 id（会话已开始） -> 返回新的 session-b8da18e9-…
```

结论与修正（已落到代码/文档）：

1. Remote 封套必须是 `{"args":{"request":{…}}}`；`session/list` 的命名参数是 `_request`。
   原实现直接发业务字段，真实 DSH 返回 `gateway/arguments-invalid`（假服务器测不出）。
2. `session/create` 对**已开始**的会话会新建会话，不能当续接用；Neko 改为只在无 `sessionId` 时 create，
   已有会话直接 `session/prompt` 续接（`internal/driver/driver.go` 的 `openLocked`）。
3. 隔离 home 没有 provider 凭据，所以本轮只验证到「投递被受理 + 轮次闭合」，**模型回复未验证**。

### 3.7 运行中 DSH（3080）真机验证（2026-11-01）

拿到用户提供的启动 token 后，对 `http://127.0.0.1:3080` 实测：

```text
neko-context invoke (evt-1) -> accepted, session=neko-probe-3080
neko-context invoke (evt-2) -> accepted, 同一 session（未新建）
session/list: turns 1 -> 2, running=false
neko-context -list          -> status=idle, lastTurn=2, refreshed=true
```

又修两处只有真机才暴露的问题：

4. `session/list` 响应带全部会话投影，实测单次 **3.6 MB**；原 1 MiB 截断导致 JSON 解析失败
   （`unexpected end of JSON input`）。改为流式 `json.Decoder` + 64 MiB 上限。
5. `GET /contexts` / `context_list` / `neko-context -list` 现在先用 `session/list` 校准
   `status` 与 `lastTurn`（`internal/driver/status.go`），没有桥接插件也不会卡在 `running`。

## 4 未做与阻塞

1. 真实 DSH 已联调：隔离实例（§3.6）+ 运行中 `http://127.0.0.1:3080`（§3.7，真实模型跑通 2 轮、续接与状态校准）。
2. 桥接插件未安装到 web profile，`turn-end` 回流未在真实链路验证。
3. 人类插话事件（`user-message`）未扩展；助手摘要已随 `turn-end` 回传。
4. MCP 工具无会话归属：`context_send` 的来源要调用方显式给 `source_context_id`；自动归属仍要走 P3 原生工具。
5. 事件路由是进程内 observer：不重放 Neko 重启前的事件，也没有持久化投递队列。
6. 验证由实现者本人执行（构建/测试/lint/冒烟），**未做独立复核**；按验证纪律，交付前应由非实现者复核一次。

## 5 下一步

- 跑 P0 探针（S1/S5 只需 token；S2 需模型可用）：命令模板见 `docs/contexts.md`。
- P2：扩展桥接 `turn-end` 摘要、`neko_context_send`、`.neko` 路由规则、循环防护。
- P3：桥接原生工具带 `sessionId`（替代 MCP 无归属路径）。
