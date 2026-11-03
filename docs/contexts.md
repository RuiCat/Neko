# DSH 上下文驱动（contexts）

Neko 作为调用层、DSH 作为 AI 通信层：本页说明 Neko 如何按事件打开/驱动 DSH 角色会话，
以及控制面与 CLI 的用法。实现范围是规划 P1（`runs/dsh-neko-contexts/00-plan.md`）。

## 数据流

```text
Neko 事件 ──▶ POST /context-invoke（或 bin/neko-context）
           └─▶ driver: 选 mode（queue/steer）──▶ DSH POST /api/session/prompt
.neko @事件 ──▶ 事件总线 observer ──▶ driver.OnEvent（routes 表 + 冷却/配额）
           └─▶ 异步投递到匹配的上下文
DSH 轮结束 ──▶ 插件 POST /session-event{kind:turn-end} ──▶ driver 标记 context idle
```

- **一个角色上下文 = 一个 DSH Session + 一个角色 preset**（人设由 preset 里的
  `@deepseek-ai/dsh-persona` 承担，见 DSH 的 `packages/preset/persona/README.md`）。
- Neko 只在上下文还没有 `sessionId` 时调 `session/create`；已有会话的上下文直接
  `session/prompt` 续接。DSH 的 `session/create` 对**已开始**的会话不会续接，而是**新建一个**会话
  （2026-11-01 实测），所以不能每次启动都 create。
- 同一事件重复投递使用同一个 `event_id`：DSH 对已受理的 `requestId` 不重复插入。

## 配置：`contexts.json`

放在仓库根目录（已加入 `.gitignore`，**不要提交**：里面可能含启动 token）。
`dsh-mcp` 只在显式传入 `-contexts-config` 时启用驱动。

```json
{
  "dsh_url": "http://127.0.0.1:3080",
  "dsh_token": "从 dsh web 启动行复制的 token",
  "registry_path": "data/contexts.json",
  "contexts": [
    {
      "context_id": "ctx-cat",
      "character_id": "猫猫",
      "session_id": "neko-cat",
      "preset_id": "standard",
      "cwd": "/home/ruicat/Desktop/Neko",
      "entity": "猫猫"
    }
  ]
}
```

| 字段 | 必填 | 含义 |
|---|---|---|
| `dsh_url` | 是 | DSH Web GUI 地址；`NEKO_DSH_URL` 可覆盖 |
| `dsh_token` | 是 | `dsh web` 启动行里的 `?token=`；`NEKO_DSH_TOKEN` 可覆盖（推荐） |
| `registry_path` | 否 | 运行状态落盘位置，默认 `data/contexts.json`（相对 `-data-dir` 解析） |
| `contexts[].context_id` | 是 | Neko 侧稳定标识，唯一 |
| `contexts[].session_id` | 是 | DSH 会话标识，唯一；复用即续接 |
| `contexts[].cwd` | 是 | 会话工作目录，必须是已存在目录 |
| `contexts[].preset_id` | 否 | DSH agent preset；人设靠它 |
| `contexts[].character_id` / `entity` | 否 | 状态实体；`entity` 缺省取 `character_id` |

`dsh_token` 每进程随机、DSH 重启即失效；长期运行应由 Neko 监管 DSH 进程并从启动行取 token
（规划 D2-A），当前实现只读取配置文件/环境变量。

### 事件路由：`.neko` @事件 → 上下文

`routes` 把引擎事件直接映射到上下文；不配置时只能由控制面/CLI 手动驱动。

```json
"routes": [
  {
    "event": "心情变化",
    "context_id": "ctx-cat",
    "priority": "normal",
    "template": "【neko 事件】{{entity}} 的 {{field}} 从 {{old}} 变成 {{new}}"
  }
]
```

| 字段 | 必填 | 含义 |
|---|---|---|
| `event` | 是 | `.neko` 的 `@事件` 名 |
| `context_id` | 是 | 目标上下文，必须在 `contexts` 中声明 |
| `priority` | 否 | `normal`（默认）/ `urgent` |
| `template` | 否 | 占位符：`{{event}}` `{{entity}}` `{{field}}` `{{old}}` `{{new}}` `{{delta}}`；留空用默认卡片 |

路由在事件分发**之后异步**投递，不阻塞状态机；同一上下文两次路由至少间隔 2s，每分钟最多 10 次，
超限跳过并写 `dsh-mcp` 日志（防止「模型写状态 → 事件 → 再驱动」自激）。投递失败同样只记日志。

### 轮次摘要（turn-end summary）

桥接插件在 `turn-end` 里附带本轮摘要（只读 Session 事件窗口尾部 200 条）：

```json
"summary": { "assistantDigest": "本喵知道了", "toolCalls": 3, "steps": 1 }
```

- `.neko` 回调 `dsh_on_event(kind, sessionId, cwd, turn, summaryJson)`：第 5 个参数是摘要的
  JSON 字符串；旧的四参数函数不受影响。
- 模型侧工具：`mcp__astrbox__context_list` 查看上下文，`mcp__astrbox__context_send` 跨上下文投递
  （60s 去重、每来源每分钟 10 条）。
- 摘要取不到时字段省略；助手文本截断到 1200 字符。

## 控制面端点

挂在 `dsh-mcp` 的 Unix socket（默认 `data/dsh-control.sock`）上：

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/contexts` | 返回全部上下文记录（status / lastTurn 等） |
| POST | `/context-invoke` | 投递一个事件到指定上下文 |

`GET /contexts` 会先用 `session/list` 校准每个上下文的 `status` 与 `lastTurn`（2s 超时；
失败时仍返回上次记录并标 `refreshed:false`）。`session/list` 带全部会话投影、响应可达数 MB，
客户端按流式解码（上限 64 MiB）。

```bash
curl --unix-socket data/dsh-control.sock http://localhost/contexts

curl --unix-socket data/dsh-control.sock \
  -H 'content-type: application/json' \
  -d '{"contextId":"ctx-cat","eventId":"evt-1","text":"【neko 事件】主人回家了","priority":"normal"}' \
  http://localhost/context-invoke
```

`priority=urgent` 且会话正在运行时使用 `steer`（在最近 step 边界插入）；否则 `queue`。
未启用驱动时两个端点返回 `503 contexts_disabled`；DSH 调用失败返回 `502 dsh_failed`。

## CLI

```bash
make build
./bin/neko-context -config contexts.json -list
./bin/neko-context -config contexts.json -context ctx-cat -event evt-1 -text '【neko 事件】主人回家了'
```

## 与 DSH 的契约（现成 HTTP，未改动 DSH）

| 动作 | DSH 端点 | 失败码示例 |
|---|---|---|
| 首次打开（仅首次） | `session/create`（`cwd`/`agentPreset`/`sessionId`） | `session/conflict`、`session/writer-held` |
| 续接/投递 | `session/prompt`（对已存在会话直接投递会恢复它） | `session/agent-busy`、`session/conflict` |
| 取消 | `session/cancel` | — |
| 列表 | `session/list` | — |

Remote 调用封套（实测）：请求体是 `{"type":"client-request","rpcId":…,"method":"session/prompt","payload":{"args":{"request":{…}}}}`；
`session/list` 的命名参数是 `_request`。

鉴权：`GET /connect?token=…&interactions=none` 换签名 API Cookie，之后带 Cookie 调 `/api/*`；
token 交换失败返回 `connect/rejected`，Cookie 失效会自动重连一次。

## 安全

- `contexts.json` 可能含 token：权限应为 `0600`，已在 `.gitignore`；优先用 `NEKO_DSH_TOKEN`。
- 控制面沿用现有信任边界：Unix socket `0600`、同 uid 可调用；不监听 TCP。
- 不读取 `~/.dsh/.credentials.yaml`，不代持模型密钥。

## 已知限制（P2/P3 待做）

- `session/prompt` 只返回 `{accepted:true}`，**没有 messageId**；事件与轮次只能按 `turn-end` 与
  `requestId` 对账。
- DSH 没有 close/delete 会话 API：`closed` 只是本地状态，会话仍留在 DSH。
- 反向工具（MCP）**不带会话归属**：多角色时无法判定「谁在写状态」，见规划 D4。
- 事件回传目前只有 `session-created`/`turn-end`；`turn-end` 已带助手摘要，人类插话事件（`user-message`）待扩展。
- 跨上下文互发现有两条路：`context_send` MCP 工具（来源需调用方显式给 `source_context_id`，因为 MCP
  调用本身不带会话身份），或「会话内工具写状态 → `@事件` → `routes` → 另一上下文」；自动归属仍待 P3 原生工具。

## 验证

```bash
make test    # Go 单测覆盖：打开/续接、queue/steer 选择、turn-end 复位、401 重连、错误码、持久化
make lint    # go vet + gofmt + code-modularity（命名/注释/长度）
```

真实 DSH 联调前先确认：`dsh web` 已启动、`contexts.json` 的 token 是当前进程的、上下文目录存在。
