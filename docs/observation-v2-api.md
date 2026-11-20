# Observation v2：冻结接口契约

> 状态：**冻结**。Go 控制面与桥接插件按本文件实现，任何字段变更必须先改本文件。
> 上层设计见 [observation-v2-design.md](observation-v2-design.md)。

## 1. 数据形状（Go 侧 JSON）

### 1.1 ObservationEvent

```json
{
  "scope": "entity:猫",
  "seq": 12,
  "path": "状态.身体",
  "from": "疲惫",
  "to": "虚弱",
  "rule_ids": ["r-疲劳"],
  "tick_id": "tick-8f3a",
  "at": "2026-11-14T08:00:00Z"
}
```

- `seq` 在 scope 内单调递增，从 1 开始；游标比较 `seq > cursor`。
- `from` / `to` 为任意 JSON 值（string / number / bool / null / object / array）。
- `at` 为 UTC RFC3339（秒精度）。
- `tick_id` 在直接写入时为 `direct:<source>`。

### 1.2 Scope

- `entity:<id>`：一个实体的状态变化（v2 唯一生产 scope）。
- 其它 scope（`turn:`、`workspace:`、`global`）预留，当前不产生事件；请求未知 scope 返回空流，不报错。

## 2. 策略文件

路径：`<data-dir>/observation.json`（生产环境 `~/.dsh-neko/Neko/data/observation.json`）。

```json
{
  "version": 2,
  "defaultTier": "normal",
  "rules": [
    { "pattern": "状态.*", "tier": "critical" },
    { "pattern": "伤害.*", "tier": "critical", "threshold": 0 },
    { "pattern": "生殖.怀孕状态", "tier": "critical" },
    { "pattern": "位置.*", "tier": "normal" },
    { "pattern": "身体.*", "tier": "noise" },
    { "pattern": "代谢.*", "tier": "noise" },
    { "pattern": "循环.心率", "tier": "noise", "threshold": 2 }
  ],
  "budget": { "maxItems": 12, "maxChars": 1200, "coalesceMs": 60000 }
}
```

- `pattern` 用 `*` 通配（匹配任意字符序列，含 `.`）；首个匹配的规则生效（数组顺序）。
- `tier` ∈ `critical` | `normal` | `noise`。
- `threshold` 只对两侧都是数字的事件生效：`|to-from| < threshold` 时不进推送（事件仍进流）。
- 旧兼容别名（`身体.体力` 这类字段与规范账本 `角色状态.体力` 同值双写）设为 `noise`：
  否则每个 tick 会推两行数值相同的变化，看起来像重复。
- `defaultTier` 缺省 `normal`；文件缺失时使用**中立内置策略（无路径规则）**，路径分级由 `observation.json` 提供（installer 从 `observation.example.json` 播种）。
- `coalesceMs`：同一 path 在窗口内的多次变化合并为一条（`from` 取窗口起点、`to` 取最新、`tier` 取最高）；
  合并后 `from == to` 的净零变化不返回，并计入 `dropped`。
- 文件解析失败：**保留上一份可用策略**（启动时即内置默认），记日志，不阻断服务。

## 3. 控制面接口

### 3.1 `GET /v2/observations?scope=<scope>&since=<seq>&limit=<n>`

请求参数：

| 参数 | 必填 | 默认 | 说明 |
|---|---|---|---|
| `scope` | 是 | — | 例如 `entity:猫`；空/缺失 → 400 `bad_request` |
| `since` | 否 | `0` | 只返回 `seq > since` 的事件 |
| `limit` | 否 | `50` | 1..200；同时受 `budget.maxItems` 上限约束 |

成功响应 200：

```json
{
  "ok": true,
  "scope": "entity:猫",
  "version": 12,
  "since": 0,
  "cursor": 12,
  "events": [
    { "scope": "entity:猫", "seq": 12, "path": "状态.身体",
      "from": "疲惫", "to": "虚弱", "rule_ids": ["r-疲劳"],
      "tick_id": "tick-8f3a", "at": "2026-11-14T08:00:00Z", "tier": "critical" }
  ],
  "truncated": false,
  "dropped": 0,
  "budget": { "maxItems": 12, "maxChars": 1200, "coalesceMs": 60000 },
  "policyVersion": 2
}
```

字段语义：

- `tier`：服务端按策略判定的等级（客户端不再自行判定）。
- `events` 已按策略过滤/合并/排序：**先 critical 后 normal**（同级按 `seq` 升序），`noise` 永不出现。
- `cursor`：调用方下一次的 `since`。默认 = `version`（本次已评估到流末尾，被策略丢弃的事件不再重扫）；
  仅当 `truncated` 为真（预算截断、后面还有没推的事件）时 = 最后一条返回事件的 `seq`，下次接着推。
- `truncated`：候选事件多于返回条数（预算或 `limit` 截断）时为 `true`。
- `dropped`：范围内被策略排除的原始事件数（noise / 阈值 / 合并），仅供诊断。
- `version`：该 scope 当前最大 `seq`；`since` 大于 `version` 时按 `version` 处理。

### 3.2 `GET /v2/observations/state?scope=<scope>`

把事件流折叠成「每个 path 的当前值」，用于 pre-step 卡片与面板。

```json
{
  "ok": true,
  "scope": "entity:猫",
  "version": 12,
  "entries": [
    { "path": "状态.身体", "value": "虚弱", "from": "疲惫", "tier": "critical", "seq": 12, "at": "2026-11-14T08:00:00Z" }
  ],
  "truncated": false,
  "budget": { "maxItems": 12, "maxChars": 1200, "coalesceMs": 60000 }
}
```

- 每个 path 只保留最新值；`from` 是该 path 最后一次变化之前的值。
- 只包含 `critical` 与 `normal`，`noise` 不出现。
- 排序：`critical` 在前，同级按 `seq` 降序（越新越前）。
- 条数上限 `budget.maxItems`（`truncated` 标记是否被截断）。

### 3.3 `GET /v2/observations/streams?entity=<id>`

```json
{ "ok": true, "entity": "猫", "scopes": ["entity:猫"] }
```

- 返回该实体关联的 scope 列表（无事件也返回 `entity:<id>`）。

### 3.4 错误

统一形状：

```json
{ "ok": false, "error": { "code": "bad_request", "message": "scope 必填" } }
```

- 400 `bad_request`：参数非法；404 `not_found`：未知路由；405 `method_not_allowed`：方法不符。

## 4. 客户端渲染契约（桥接插件）

模块：`plugins/dsh-astrbox/lib/observationRender.mjs`，纯函数，无 I/O。

### 4.1 值格式化 `formatValue(value, maxChars = 200)`

- `null` / `undefined` → `null`
- 字符串原样；布尔 → `真` / `假`；数字不带多余小数
- 对象/数组 → 紧凑 JSON
- 超过 `maxChars` 截断并追加 `…`

### 4.2 变化推送 `renderDelta(payload, options)`

输入是 §3.1 的响应体；`options = { maxChars }`（缺省取 `payload.budget.maxChars`，再缺省 1200）。

```
[astrbox-observation] 猫 · 2 处变化
- 状态.身体: 疲惫 → 虚弱
- 位置.场景: 书房 → 卧室
另有 3 条未显示
```

- 头行：`[astrbox-observation] <entity> · <N> 处变化`（`entity` 从 `scope` 的 `entity:` 之后取）。
- 每行：`- <path>: <from> → <to>`；`from` 为 `null`/缺失时只写当前值。
- 超过字符预算时从**尾部**丢行（尾部即 normal），追加 `另有 <N> 条未显示`。
- `events` 为空 → 返回空字符串（调用方不推送）。

### 4.3 现状卡 `renderState(payload, options)`

输入是 §3.2 的响应体。

```
[astrbox-state] 猫
状态.身体: 虚弱
位置.场景: 卧室
```

- 头行 `[astrbox-state] <entity>`，`maxItems` 缺省 8，`maxChars` 缺省 1200。
- 超出条目上限追加 `另有 <N> 条未显示`。

## 5. 退役清单（删除，不留兼容层）

| 模块 | 处置 |
|---|---|
| `internal/eventboard` 整包 | 删除，由 `internal/observation` 取代 |
| `internal/control/blackboard.go`、`/blackboard` 路由 | 删除 |
| `internal/mcpserver/blackboardTool.go`（`blackboard_get`） | 删除，由 `observation_get` 工具取代（读 §3.1/§3.2） |
| `plugins/dsh-astrbox/lib/blackboardText.mjs` + 测试 | 删除，由 `observationRender.mjs` 取代 |
| `prompt.mjs` 的 `composeSnapshot` / `composePackage` / `createSnapshotMessage` / `SNAPSHOT_SOURCE_KIND` | 删除；`STATIC_USAGE_TEXT` 与 `truncateText` 保留（主提示词仍用） |
| `config.paths` / `personaPaths` / `scenePaths` / `subscription*` 键 | 删除（改由 `observation.json` 驱动）；旧配置文件里出现时**读取即忽略**，不报错 |
| `subscriptionFilter.mjs` + 测试 | 删除（过滤在服务端策略里） |
| `dialogFeed` 独立推送链路 | 保留数据，随变化推送合并发出（不再单独成链） |

保留：`internal/sessionboard`（多会话协作黑板，独立子系统）、`internal/blackboard`（回合内专家协调）、面板只读路由（改由 v2 供数）。
