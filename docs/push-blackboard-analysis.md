# 推送 / 黑板实现分析与重规划

> **历史文档（已被取代）**：旧推送/黑板实现已按 [observation-v2-api.md](observation-v2-api.md) 整体切到
> Observation v2 并删除；本文保留为迁移前的分析与决策记录，其中 `blackboardText.mjs` / `subscriptionFilter.mjs` /
> `config.subscription*` 等模块与配置键均**已不存在**。当前行为见 [dsh-plugin.md](dsh-plugin.md) 的「状态变化推送」节。

> 范围：`internal/{eventboard,blackboard,sessionboard,compensation}`、`internal/control` 的 board 路由、
> `plugins/dsh-astrbox/lib/{prompt,blackboardText,sessionBoard*,dialogFeed,subscription*}*`。
> 结论：**三套「板」+ 四种「推送出口」+ 43 个配置键**，概念与时间语义重叠，建议按「观测 → 过滤 → 渲染 → 出口」统一。

## 1. 现状组件

| 模块 | 数据形状 | 生产者 | 消费者 | 触发 |
|---|---|---|---|---|
| `eventboard` | 每实体变更板：`entry{path,from,to,rule_ids,version}`、`board{version,tick_id}` | 引擎 tick 写监听（含 `@函数`/事件写入） | 控制面 `/blackboard`、bridge 订阅、面板 | 每 tick |
| `blackboard` | 每轮上下文：`event{kind,summary,payload}` + `hardstate.Snapshot` + `constraints` | 引擎/控制面组装 | 专家（只读 Reader）、补偿包 | 每轮 |
| `sessionboard` | 键值板：`region{owner,version}`、`entry{key,from,to,by,version,deleted}` | 会话/工具写入 | 面板 `/boards*`、bridge 订阅 | 按写入 |
| `compensation` | 补偿包（gate/专家/记忆/约束） | `Assembler` | bridge 的 turn-context 注入 | 每轮 |

插件侧出口：

| 出口 | 用途 | 配置 |
|---|---|---|
| pre-step 快照 | 每轮注入 `[astrbox] …状态快照` | `paths`、`promptMaxChars`、`promptRefreshMs` |
| pre-step 补偿包 | 有 `/turn-context` 时替换快照 | `paths`、`personaPaths`、`personaMaxChars` |
| 异步订阅 | `[astrbox-subscription]` 推送主会话 | `subscriptionEnabled/MainIntervalMs/MaxChars` |
| 会话黑板订阅 | `astrbox_board` 工具 + 子会话注入 | `boardEnabled`、`boardSubscriptionMaxChars` |
| 面板路由 | `/astrbox/blackboard`、`/astrbox/boards*` | `workspacePrefix` |

## 2. 数据流

```
tick ──► engine.writeListeners ──► TickResult.Writes ──► eventboard(version++)
                                                      ├─► GET /blackboard ─► 面板
                                                      └─► bridge 订阅 ─► formatSubscriptionText ─► 主会话

会话/工具写入 ──► sessionboard(region.version) ──► /boards* ─► 面板
                                            └─► bridge 订阅 ─► sessionBoardText ─► 子会话注入

turn ──► /turn-context(compensation) ─┐
                                    ├─► pre-step 注入（二选一）
   └─► /state(paths) ────────────────┘
```

## 3. 问题清单

1. **命名与语义错位**：`blackboard` 实际是「每轮上下文」，`eventboard` 才是「变更板」，`sessionboard` 是「会话键值板」。三个名字共享 "board"，但数据、生命周期、消费者都不同。
2. **时间语义三套**：pre-step（每轮）、eventboard version（每 tick）、subscription interval（按分钟）。同一份变化可能在不同节奏被读两次。
3. **过滤能力不一致**：快照有 `paths` 白名单；**订阅没有**路径过滤（全量黑板），字段一多就吵；`sessionboard` 只有 region 隔离，无字段过滤。
4. **配置键 43 个、未分组、有重复**（`promptMaxChars/personaMaxChars/…` 在解析列表里重复出现），职责混在一起：快照、订阅、看板、时钟、超时。
5. **状态来源双写**：pre-step 先试 `/turn-context`（补偿包），失败回退 `/state`（paths），两条路径内容不同但共用一个注入位，难以断言。
6. **游标/版本三处实现**：eventboard `version`、sessionboard `region.version`、订阅间隔 store；去重与截断策略各不相同。
7. **按 K 号切模块**：每个需求号一套逻辑与测试，边界按历史需求而非职责划分，重复的「渲染 + 截断 + 前缀」散落在 `prompt.mjs` / `blackboardText.mjs` / `sessionBoardText.mjs`。
8. **缺统一的「观测」模型**：字段变化、会话键变化、状态快照，本质都是 `{scope, path, from, to, ts, source}`，现在三种类型各自渲染。

## 4. 目标架构（重规划）

### 4.1 统一模型

```
Observation {
  kind:   'snapshot' | 'delta' | 'board'
  scope:  entity | workspace | global
  path:   string            // 会话板用 key
  from, to: any
  version: int
  at:     string
  source: 'state' | 'tick' | 'session'
}
```

### 4.2 单一管道

```
Sources ─► Filter ─► Render ─► Sinks
  state snapshot   scope+paths+importance   text/json   pre-step 注入
  tick delta       同一份过滤规则            text        主会话推送
  session delta                             text        子会话注入
                                                        面板路由 / 工具
```

- **Filter 只有一份**：`observation.paths`（按 scope 前缀匹配）+ `minImportance`；
- **Render 只有一份**：前缀（`[astrbox]` / `[astrbox-subscription]`）、编码、截断、缺失标记；
- **Sink 只决定去哪**，不做格式。

### 4.3 配置分组（兼容旧键别名）

```yaml
observation:
  snapshotPaths: [...]      # 原 paths
  personaPaths: [...]
  scenePaths: [...]
  snapshotMaxChars: 1200
subscription:
  enabled: true
  paths: [...]              # 新增：订阅过滤（空 = 全量）
  mainIntervalMs: 600000
  maxChars: 2000
  boardEnabled: true
  boardMaxChars: 2000
panel:
  workspacePrefix: ...
runtime:
  tickEveryMs: 60000
  clockStepMinutes: 10
  ...
```

旧键在 `normalizeConfig` 里映射到新键，保留一个版本周期。

### 4.4 单一游标

统一 `cursorStore`：`{scope} → {version, lastPushAt}`。eventboard/sessionboard 共用，避免三套去重逻辑。

## 5. 迁移阶段

| 阶段 | 内容 | 验收 |
|---|---|---|
| P0 | 本分析 | 组件/出口/问题清单确认 |
| P1 | 配置归一（分组 + 去重 + 旧键别名），**不改行为** | 现有 K4/K5/K7/K8/K11/K15/K17/K28 测试全过 |
| P2 ✅ | 订阅路径过滤（`subscriptionPaths/ignore/maxEntries`） | 已实现：`subscriptionFilter.mjs` + 4 个单测；58 个插件测试全过 |
| P3 | 统一 Observation 与渲染器（内部重构） | 输出文本与 P1 逐字节一致（golden 对比） |
| P4 | 可选：neko 提供 `推送.白名单/重要性` | 脚本可控制过滤，但渲染仍在 bridge |

## 6. P2 实现说明（已落地）

配置（`bridge.config.json`）：

```json
{
  "subscriptionPaths": ["角色状态.*", "伤害.*"],
  "subscriptionIgnore": ["代谢.*", "睡眠.疲劳"],
  "subscriptionMaxEntries": 12
}
```

- `subscriptionPaths` 空 = 不过滤；支持 `a.b.*` 与 `*`；
- `subscriptionIgnore` 优先于白名单，用于排除每 tick 都变的连续字段；
- `subscriptionMaxEntries` 截断单条推送条目数（0 = 不限）；
- 过滤只作用于订阅渲染，面板 `/blackboard` 仍返回全量。

## 7. 待确认

1. 订阅是否要路径过滤（P2）？还是维持全量、只靠间隔降噪？
2. 快照与补偿包是否二选一保留？现在双路径难以测试。
3. `sessionboard` 是否纳入统一游标，还是保持独立（它是「会话键值」，语义不同）？
4. 配置归一是否接受「旧键别名保留一个版本」？
