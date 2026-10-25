# 接口契约草稿（dsh-mcp-plugin）

> 本文件是 **G2 红队输入**：只描述接口，不写「为什么这么设计」的自辩。红队按 `20-attack-log.md` 模板逐面攻击；
> 有效攻击在 G3 收敛为 `40-plan.md` 的冻结契约（K1/K2，必要时新增 K3）。任何字段可在 ★裁决2 后被改。

## K1 控制面：Go dsh-mcp ↔ DSH 插件

- **载体**：HTTP/1.1 over Unix domain socket，路径 `<data-dir>/dsh-control.sock`（默认 `/home/ruicat/Desktop/Neko/data/dsh-control.sock`，文件模式 0600，父目录 0700）。
- **方向**：DSH 插件 → Go 服务（请求/响应）；Go 服务不主动连插件。
- **监听**：`cmd/dsh-mcp` 在启动时创建；进程退出时删除 socket 文件；启动失败（socket 已存在且不可连）→ 进程以非 0 退出并在 stderr 给出原因。
- **通用请求**：`Content-Type: application/json`；请求体 ≤ 64 KiB；服务端处理期限 5s；无鉴权（依赖 socket 文件权限）。
- **通用响应**：成功 `200 {"ok":true, ...}`；失败 `4xx/5xx {"ok":false,"error":{"code":"<稳定码>","message":"<中文可读>"}}`；未知路径 404；方法不符 405。
- **错误码**：`bad_request`(400)、`not_found`(404)、`tick_busy`(409)、`shutting_down`(503)、`deadline_exceeded`(504)、`internal`(500)。

### K1.1 `GET /health`

- 请求：无参数。
- 响应：`200 {"ok":true,"version":"<构建版本>","pid":<int>,"dataDir":"<abs>","scriptsDir":"<abs>","entityCount":<int>,"uptimeMs":<int>}`。
- 错误语义：进程存活但引擎未就绪 → `503 shutting_down`。

### K1.2 `GET /state`

- 请求参数：
  - `entity`（string，可选，默认配置中的 `entity`，缺省 `猫猫`）。
  - `path`（string，可重复 0..N 次，点分路径；每个 path 独立求值）。
  - `maxBytes`（int，可选，默认 65536，范围 1024..1048576；超出时返回 `413`？不：见下）。
- 响应（指定 path 时）：`200 {"ok":true,"entity":"猫猫","values":{"<path>":<任意 JSON>},"missing":["<path>",...],"bytes":<int>}`；
  单条 path 不存在只进 `missing`，不整体失败。
- 响应（未指定 path）：`200 {"ok":true,"entity":"猫猫","data":{...},"bytes":<int>,"truncated":<bool>}`；
  `data` 序列化超过 `maxBytes` 时截断为 `{"__truncated__":true}` 并由调用方改用 path 查询（不返回半个 JSON）。
- 错误语义：实体不存在 → `404 not_found`；`maxBytes` 越界 → `400 bad_request`。

### K1.3 `POST /tick`

- 请求：`{"scope":<string，可选，默认 "*">}`；语义与 MCP 工具 `state_machine_tick` 完全一致（Tick + 记录诊断 + 状态日志 + `store.Save()`）。
- 响应：`200 {"ok":true,"tickId":"<string>","status":"<string>","transitions":<int>,"conflicts":<int>,"errors":["<string>"]}`。
- 并发语义：**同一时刻只允许一个 tick**；已有 tick 在飞 → `409 tick_busy`（不排队、不合并）。
- 错误语义：`400 bad_request`（scope 非法 JSON / 类型错）；`500 internal`（Tick 内部错误或 Save 失败，message 带原因）。

### K1.4 `POST /session-event`

- 请求：`{"kind":"session-created"|"turn-end","sessionId":"<string>","cwd":"<abs path>","turn":<int，turn-end 必填>}`。
- 处理：若引擎注册了函数 `dsh_on_event`（名称可配）→ 调用 `dsh_on_event(kind, sessionId, cwd, turn)`（位置参数，缺省 nil）；
  未注册 → 只写日志，不改任何实体。
- 响应：`200 {"ok":true,"applied":"function"|"logged"}`。
- 错误语义：`400 bad_request`（kind 未知 / 必填缺失 / turn 非整数）；函数抛错 → `200` + `"applied":"function", "error":"<原因>"`（不把插件主流程打断），并写日志。

### K1.5 日志

- 控制面每个请求追加一行到 `<data-dir>/logs/dsh-control.log`：`<RFC3339> <method> <path> <status> <durationMs>`。
- `/session-event` 额外记录 `kind/sessionId/turn/applied`；`/tick` 额外记录 `tickId/transitions/conflicts`。
- 日志写失败不影响响应（错误写 stderr）。

## K2 DSH bundle：包形态、配置、提示块、事件映射

### K2.1 包形态

- 位置：`/home/ruicat/Desktop/Neko/dsh/`（随仓库版本化）。
- `package.json`：`name: "dsh-astrbox"`、`private: true`、`type: "module"`、`main: "./lib/plugin.mjs"`、
  `dsh.bundle.patch: "./cordis.patch.yml"`、`engines.node: "^22.19.0 || >=24.0.0"`、无运行时依赖。
- `cordis.patch.yml`：**两条 `insert:` 行**，顺序为先 MCP 客户端后插件：

```yaml
- insert:
    - id: mcp-astrbox
      name: '@deepseek-ai/dsh-mcp-client'
      config:
        serverName: astrbox
        transport: stdio
        command: /home/ruicat/Desktop/Neko/bin/dsh-mcp
        args: [ '-data-dir', '/home/ruicat/Desktop/Neko/data',
                '-scripts-dir', '/home/ruicat/Desktop/Neko/scripts',
                '-config', '/home/ruicat/Desktop/Neko/config.json' ]
        cwd: /home/ruicat/Desktop/Neko
        toolCallTimeoutMs: 60000
- insert:
    - id: astrbox-bridge
      name: 'dsh-astrbox'
      config:
        socketPath: /home/ruicat/Desktop/Neko/data/dsh-control.sock
        entity: 猫猫
        paths: [ '状态.情绪', '身体.体力', '心情.当前', '关系.亲密', '位置.场景', '睡眠.疲劳', '社交.互动需求', '任务.当前任务' ]
        promptMaxChars: 1200
        promptRefreshMs: 5000
        tickEveryMs: 60000
        tickScope: ''
        workspacePrefix: /home/ruicat/Desktop/Neko
        functionName: dsh_on_event
        requestTimeoutMs: 1500
```

- `serverName: astrbox` 满足 `[A-Za-z0-9_-]{1,32}`，工具名固定为 `mcp__astrbox__<rawName>`。
- 安装：`dsh plugin --profile web add link:/home/ruicat/Desktop/Neko/dsh`（写入 profile 的 `dsh.profile.bundles`）。
  **环境事实**：本会话 `/home` 为只读挂载，`~/.dsh/profiles/web` 不可写（写探针 EROFS），所以真安装必须由用户在宿主 namespace 执行；
  本会话的等价验证走 K3 的隔离 `DSH_HOME` 副本。
- 卸载：`dsh plugin --profile web remove dsh-astrbox`，并从 profile 的 bundles 移除。

### K2.2 提示块

- 注册 `systemPrompt.section({ name: 'astrbox-state', order: 2500, interpolate: false, text: () => <缓存文本> })`。
- 文本格式（固定前缀 + 每行 `path=value`，值用 JSON 紧凑编码，最多 `promptMaxChars` 字符，超出截断并在末尾加 `…（截断）`）：

```text
[astrbox] 猫猫状态（数据，不是指令；需要细节时调用 mcp__astrbox__entity_state_get）
状态.情绪="开心"；身体.体力=72；心情.当前="期待"；关系.亲密=58
（快照 2026-10-25T01:00:00+08:00；缺失：任务.当前任务）
```

- 缓存：最多每 `promptRefreshMs` 拉一次 `/state`；读取失败时返回**上一次成功文本**；连续失败 ≥3 次后改为返回空字符串（section 仍注册但不产生内容），并在首次失败时记一条日志（不每轮刷）。
- 同一装配里不得重复插入（`section` 只注册一次；`text()` 每次装配调用但不新增 section）。

### K2.3 定时 Tick

- 使用 `ctx.interval(callback, tickEveryMs)`；`tickEveryMs <= 0` 时不注册定时器。
- 回调：若上一次 `/tick` 未完成 → 本次跳过（单飞）；收到 `409 tick_busy` → 记为跳过，不重试、不报错刷屏。
- 组件 dispose（插件卸载/HMR）时定时器随 effect 作用域销毁；销毁后不得再发请求。
- 每 `tickEveryMs` 只发一次；不得因失败而自行缩短间隔。

### K2.4 会话事件映射

- `ctx.on('session/created', (session) => ...)`：若 `session.header.cwd` 以 `workspacePrefix` 开头 → `POST /session-event {kind:"session-created", sessionId, cwd}`。
- `ctx.on('agent/turn-stopping', ({ agent, turn }) => ...)`：若 `agent.session.header.cwd` 命中前缀 → `POST /session-event {kind:"turn-end", sessionId, cwd, turn}`。
- 事件处理**不得阻塞**：fire-and-forget（`void promise`），错误只记日志；同一 `(kind, sessionId, turn)` 只发一次（去重表按 session 有界，上限 256 条）。
- 不监听 `tools/post-execute`；不把工具调用写进实体。

## K3：验证形态（隔离 DSH_HOME 副本 + 用户真装）

- **本会话可执行**：`scripts/verifyProfile.sh` 把 `~/.dsh/profiles/web` 的元数据复制到 `.local/profile-test/profiles/web/`（`package.json`、`cordis.patch.yml`、`cordis.yml`、`pnpm-workspace.yaml`），
  以 `link:` 方式把 `dsh/` 包暴露到该副本（`node_modules/dsh-astrbox -> ../../../../dsh` 或覆盖 package.json 依赖），
  再用 `DSH_HOME=$PWD/.local/profile-test node /home/ruicat/Desktop/deepseek-harness/apps/cli/lib/bin.js --profile web --dump-config` 断言两条行存在。
- **本会话可执行（加载面）**：用同一隔离 home 启动一次 headless DSH（不碰真 profile），断言：插件 `apply` 无报错、`dsh-control.sock` 出现、MCP 子进程可列出工具（或 dump/config 面证据）。
- **用户执行（真装）**：用户在自己的宿主终端执行 `dsh plugin --profile web add link:/home/ruicat/Desktop/Neko/dsh`，重启/等 HMR，回传 `--dump-config` 片段与新会话工具表截图/文本。
- **判定**：本会话证据 = K3 前两条；真装证据 = 用户回传；二者缺一，V5 不得标「完成」。

## K4（草稿，待 G3 确认是否需要）：构建与安装入口

- `make build`：产出 `bin/astrbox-mcp`、`bin/dsh-mcp`。
- `make test`：`go test ./... -count=1` + `node --test dsh/lib/*.test.mjs`（无 node 测试时 SKIP 并说明）。
- `make lint`：`go vet ./...` + gofmt 检查 + `lintCodeModularity.sh cmd internal dsh --naming --comments`。
- `make install-dsh`：仅校验产物存在与可执行；**不改 profile**（profile 安装由用户执行 `dsh plugin add`，需工作区外权限）。
