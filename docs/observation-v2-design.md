# Observation v2：替换旧推送 / 黑板的全新设计

> 状态：**已实现**（P1/P5 完成，P2/P4 由桥接插件完成；旧模块已删除，无兼容层）。
> 接口细节以 [observation-v2-api.md](observation-v2-api.md) 为准。
> 目标：一条**变化流**驱动所有出口；只推变化；按重要性与预算裁剪；旧模块在切换后删除。

## 1. 需求与目标

用户要求（原文转述）：

- neko 的变化要推送到**主体**；
- 不能全部推送，大量推送消耗上下文；
- **只推变化的内容**。

设计目标：

1. **单一变化源**：任何被脚本/规则/函数写入的字段 → 一条 `ObservationEvent`（append-only）。
2. **单一过滤策略**：按 tier / 路径 / 阈值 / 合并窗口决定「值不值得推」，只此一处。
3. **单一渲染**：前缀、编码、截断、缺失标记只实现一次。
4. **多出口复用**：pre-step 注入、主会话推送、子会话注入、面板、工具查询都消费同一份数据。
5. **预算硬上限**：每条推送 `maxItems` + `maxChars` + `coalesceWindow`，超限按 tier 优先级裁剪。
6. **可回放**：同一事件流 + 同一策略 → 逐字节一致的渲染；支持 golden 对比。

非目标：不改 neko 语言；不重做补偿包（gate/专家/记忆）的业务内容，只替换它的状态段来源。

## 2. 旧实现退役清单

| 旧模块 | 现状 | 去向 |
|---|---|---|
| `plugins/dsh-astrbox/lib/prompt.mjs` 快照/补偿渲染 | `paths` 白名单拉全量状态 | 删除，由 v2 Renderer 取代 |
| `blackboardText.mjs` 订阅文本 | 全量变化、无过滤 | 删除，由 v2 Renderer 取代 |
| `internal/eventboard` | 每实体变更板 | 由 `internal/observation` 取代 |
| `internal/sessionboard` 推送部分 | 会话板游标/订阅 | 面板保留只读视图；推送由 v2 统一 scope |
| `plugins/.../dialogFeed` | 对话摘要推送 | 归入 Narrative Sink（同一条消息附带，不再是独立链路） |
| `config.paths/personaPaths/subscription*` | 43 个键混在一起 | 由 `observation.json` 单一策略文件取代 |

保留：引擎的写监听钩子（`internal/engine` 的 `attachWriteListeners` / `TickResult.Writes`，作为 v2 的原始数据源）、控制面 socket、补偿包内容、面板只读路由。

## 3. 新概念模型

```
ObservationEvent   { scope, path, from, to, ruleIds, tickId, at, seq }
Observation        = 折叠(事件流) 得到的当前值（只读投影，不落盘）
Policy             { rules: [{ pattern, tier, threshold, coalesceMs }], budget: {...} }
Stream             = 一个 scope 的事件序列（entity:猫 | turn:猫 | workspace:web | global）
Cursor             { consumer, scope, seq, pushedAt }
Sink               { prepend | main | subagent | panel | tool }
```

- **scope**：`entity:<id>`、`turn:<id>`、`workspace:<id>`、`global`。所有事件必带 scope。
- **tier**：`critical`（状态词/死亡/怀孕/休克，必推）、`normal`（合并后推）、`noise`（默认不推，面板可见）。
- **threshold**：数值字段变化小于阈值不产生「可推事件」（仍进事件流，供面板/查询）。
- **coalesce**：同一 path 在窗口内的多次变化折叠为最后一次（`from` 保留窗口起点、`to` 取最新）；
  折叠后 `from == to`（净零，比如 79→78 再 78→79）的变化**不推送**——它不代表任何净变化。

## 4. 数据流

```
规则/函数写入
   └─ engine write listener ──► ObservationBus.Append(scope, path, from, to, ruleIds, tickId)
                                   │
                                   ├─► Stream（append-only，版本 = seq）
                                   │
        ┌──────────────────────────┴───────────────────────────┐
        ▼                          ▼                           ▼
   pre-step 卡               main push                    panel / tool
   （当前值投影，            （since cursor 的 delta，       （全量查询，
     tier=critical+normal）    policy 过滤 + 预算）           不过滤）
```

pre-step 卡不再读 `paths`：它是对 Stream 的**当前值投影**（最近 N 条 critical + normal 的 `to`），预算固定。

## 5. 存储与接口

### 5.1 引擎侧（新包 `internal/observation`）

```go
type Event struct {
    Scope  string   `json:"scope"`
    Seq    int64    `json:"seq"`
    Path   string   `json:"path"`
    From   any      `json:"from"`
    To     any      `json:"to"`
    RuleIDs []string `json:"rule_ids,omitempty"`
    TickID string   `json:"tick_id"`
    At     string   `json:"at"`
}

type Store interface {
    Append(event Event)                 // 写路径全部走这里
    Stream(scope string, since int64, limit int) []Event
    Version(scope string) int64
    Snapshot(scope string) map[string]any  // 折叠出的当前值（只读）
}
```

- 内存环形缓冲（每 scope 保留最近 N 条，默认 512）+ 可选落盘 `data/observations.jsonl`；
- 版本 = 每 scope 的 `seq`，游标比较 `seq > cursor.seq`；
- 不按 path 去重（append-only），折叠在读取时做。

### 5.2 控制面 v2

```
GET /v2/observations?scope=entity:猫&since=123&limit=50   # 已过滤的变化流
GET /v2/observations/state?scope=entity:猫                # 当前值投影
GET /v2/observations/streams?entity=猫                    # 该实体关联 scope 列表
```

`/blackboard` 已删除；面板路由 `/astrbox/blackboard` 保留路径与响应形状，改由 v2 供数。
`/boards*` 属于会话黑板（`internal/sessionboard`），本次不动。

## 6. 过滤与预算算法

```
for event in stream(scope, since):
    if event.tier == noise: continue
    if event.tier == normal and withinCoalesceWindow(event.path): merge
    if numeric(event) and |to-from| < threshold: skip（仅影响推送，不影响面板）
    append candidate

sort candidates by (tier desc, at asc)
take maxItems
render → truncate maxChars
cursor = last seq
```

- 阈值与 tier 来自 `observation.json`，不写死在代码；
- 合并只发生在渲染前，事件流本身不丢数据；
- 超预算时先丢 `normal`，再丢较早的 `critical`（附「另有 N 条未显示」）。

## 7. neko 契约

1. **脚本只负责写字段**，不负责推送格式；新增字段必须在策略文件登记 pattern，否则默认 `noise`。
2. **叙事事件**（发生了什么）走 `emit` → Narrative Sink，与状态变化分开；两者可合并进同一条推送的「变化 / 叙事」两段。
3. **重要变化**由策略按路径判定，例如：

```yaml
rules:
  - { pattern: '角色状态.*',        tier: critical }
  - { pattern: '伤害.*',            tier: critical, threshold: 0 }
  - { pattern: '生殖.怀孕状态',      tier: critical }
  - { pattern: '位置.场景',          tier: normal }
  - { pattern: '代谢.*',            tier: noise }
  - { pattern: '循环.心率',          tier: noise }
budget: { maxItems: 12, maxChars: 1200, coalesceMs: 60000 }
```

## 8. 配置 v2（单文件）

`~/.dsh-neko/Neko/data/observation.json`：

```json
{
  "version": 2,
  "rules": [ ... ],
  "budget": { "maxItems": 12, "maxChars": 1200, "coalesceMs": 60000 }
}
```

旧 `paths`/`subscription*` 不再读取；安装时从 `observation.example.json` 生成
`~/.dsh-neko/Neko/data/observation.json`（已存在则不覆盖）。推送间隔等桥接行为仍由
`bridge.config.json` 控制，策略文件只决定「值不值得推」。

## 9. 替换步骤

| 阶段 | 内容 | 验收 |
|---|---|---|
| P0 | 冻结本设计 + 策略文件示例 | 评审通过 |
| P1 | 新包 `internal/observation`（事件/流/折叠/游标）+ 单测 + `/v2/observations` | 引擎测试全绿；回放一致 |
| P2 | 新渲染器 `lib/observationRender.mjs` + 策略加载 + 单测 | golden：给定事件流输出稳定 |
| P3 | **shadow 双跑**：新链路只记录不注入，对比旧推送输出 | 差异清单（预期：噪声更少） |
| P4 | 切换：pre-step / 主推送 / 子会话改走 v2；旧键映射一个版本 | 真实会话验证；预算达标 |
| P5 | 删除旧模块与旧配置键、路由（§2 清单） | `grep` 无引用；测试全绿 |

## 10. 验收

- 同一事件流 + 同一策略 → 渲染逐字节一致（golden）；
- 预算：条目 ≤ maxItems、字符 ≤ maxChars，超限有「另有 N 条」；
- 去重：同一事件不重复推送（cursor 单调）；
- 噪声：`noise` tier 的字段永不进主推送；
- 面板：仍可看到全量事件（不过滤）；
- 回归：现有 K4/K5/K7/K8/K11/K15/K17/K28 行为在 shadow 阶段被覆盖，切换后旧测试删除或改写。

## 11. 风险

1. **shadow 期双写成本**：事件流内存与日志增长；用环形缓冲 + 采样。
2. **策略漂移**：新增字段未登记 → 默认不推，可能「悄悄少了推送」；用启动校验（未知路径告警）。
3. **scope 爆炸**：多实体/多会话；上限与 TTL。
4. **兼容期**：旧面板路由与 v2 并存；明确只读。

## 12. 已确认决策

1. **未登记路径默认 `normal`**（不是 `noise`）：内置策略把高频数值面（`代谢.*` / `循环.*` / `呼吸.*` /
   `内分泌.*` / `器官.*` / `内脏.*` / `体温.*`）显式降为 `noise`。这样新增字段不会「悄悄不推」，
   噪声由 `observation.json` 显式控制。
2. **pre-step 保留当前值投影**：读 `/v2/observations/state`，启动时对每个实体的非噪声路径写一次
   `seeded` 基线事件（不进变化推送），保证身份、状态等长期不变字段仍在卡片里。
3. **旧模块切换后立即删除**，不做兼容层（`internal/eventboard`、`/blackboard`、`blackboard_get`、
   `blackboardText.mjs`、`prompt.mjs` 快照函数、`paths`/`subscription*` 配置键、`subscriptionFilter.mjs`）。
   旧 `bridge.config.json` 里残留的退役键**读取即忽略**（否则升级后插件起不来）。
5. **补偿包保留、状态段换源**：`/turn-context` 的记忆/约束/情境/专家仍按 `/turn-context` 注入
   `[astrbox-compensation]`，状态段由 `[astrbox-state]` 现状卡取代，不重复。
4. **直接写入**（MCP / 控制面）与 tick 写入同源：都进 `entity:<id>` 流，`tick_id` 记为 `direct:<source>`。
