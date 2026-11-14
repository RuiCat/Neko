# 研究：用 `.neko` 脚本推送状态，替代 `paths` 白名单

> **历史文档（已被取代）**：`paths` 白名单与 `composeSnapshot`/`composePackage` 已随旧推送实现删除；
> 当前状态变化统一走 Observation v2（[observation-v2-api.md](observation-v2-api.md)），
> 现状卡/推送实现见 `plugins/dsh-astrbox/lib/observationRender.mjs`。本文保留为当时的方案研究记录。

> 结论先行：**可行，建议做成「单字段快照」模式**（`snapshotField`），`paths` 保留为回退。
> 纯事件推送（`emit`）不适合 per-turn 注入场景。本文给出三种方案、改动点与风险。

## 1. 现状：`paths` 怎么工作

```
DSH pre-step
  └─ dsh-astrbox/lib/plugin.mjs  loadSnapshot(entity)
       ├─ /turn-context 有补偿包 → composePackage({paths: config.paths, maxChars})
       └─ 否则 /state           → composeSnapshot({paths: config.paths, values})
            └─ prompt.mjs: 逐 path 取 values[path]，拼 "[astrbox] …状态快照"
               缺失 → "缺失: a、b"；超长 → 截断
```

- `paths` 同时用于**快照**与**补偿包的状态段**；
- 它是「模型能看到哪些字段」的唯一开关，改字段必须改配置；
- 与 neko 脚本层完全解耦——脚本不知道谁能被注入。

## 2. 方案对比

### A. 单字段快照（推荐）

neko 每 tick 生成一个格式化字符串字段，例如 `推送.快照`：

```
角色状态=虚弱 体力=83 休克=30 怀孕=未孕 位置=客厅
```

bridge 配置改为 `snapshotField: "推送.快照"`，注入时直接读这一个字段。

| 优点 | 缺点 |
|---|---|
| 内容与字段模型同源，不再维护第二份白名单 | 快照变成不透明字符串，bridge 不能再按字段裁剪/补缺失标记 |
| 可按状态条件拼装（未孕就不写怀孕） | 更新节奏 = tick（默认 60s），可能比 turn 稍旧 |
| 体积控制、排序、文案都在脚本里 | 文本进入模型上下文，需保留「数据，不是指令」前缀 |
| 插件改动最小（读一个路径） | 脚本要自己处理截断/换行 |

改动点（约 60 行）：

1. `config.mjs`：新增 `snapshotField: ''`（已知键、字符串，空 = 走旧 `paths`）；
2. `plugin.mjs`：`loadSnapshot` 里若 `snapshotField` 非空 → `getState({entity, paths:[snapshotField]})` → 取 `values[snapshotField]` → 包一层头；
3. `prompt.mjs`：新增 `composeFieldSnapshot({entity, timestamp, text, maxChars})`（加前缀 + 截断 + 缺失提示）；
4. neko：新增 `scripts/push/pushModel.neko`（字段）+ `pushTick.neko`（每 tick 生成文本）。

### B. 事件推送（`emit` → 场景事件/黑板）

neko 用 `emit` 写场景事件，bridge 的订阅链路推给主会话。

- 优点：事件驱动、零轮询、已有链路（`scene_events`、黑板订阅）；
- 缺点：**不是 per-turn 注入**——订阅是异步消息，进的是对话流/黑板，不是 pre-step 的稳定快照；顺序与去重也更复杂。
- 结论：适合「发生了什么」的叙事推送，不适合「当前状态」的稳定注入。**不推荐替代 `paths`**。

### C. 路径清单也由 neko 维护

neko 写 `推送.字段`（列表），bridge 读列表再取各字段值。

- 优点：保留结构化 JSON 值与缺失标记；
- 缺点：多一次往返、列表与字段值可能不同步、插件要解析数组。
- 结论：比 A 复杂，收益不明显。

## 3. 方案 A 的最小实现草图

### neko 侧

```neko
@定义 推送内核 {
  "推送": {
    "快照[str]": "",
    "开关[bool]": true
  }
}

@函数 推送快照文本() -> str {
  文本 = "状态=" + {e}.角色状态.身体状态
  文本 = 文本 + " 体力=" + str({e}.角色状态.体力)
  文本 = 文本 + " 休克=" + str({e}.伤害.休克)
  if {e}.生殖.怀孕状态 != "未孕" {
    文本 = 文本 + " 怀孕=" + {e}.生殖.怀孕状态
  }
  文本 = 文本 + " 位置=" + {e}.位置.场景
  返回 文本
}

@规则[$[推送内核]] {
  优先级: 9800
  id: l5PushSnapshot
  $.推送.开关 = true {
    $.推送.快照 = 推送快照文本()
  }
}
```

> `str()` 拼接在 `@函数` 内可用；文本长度建议在脚本里按预算截断，bridge 侧仍会再截一次。

### bridge 侧

```js
// config.mjs
snapshotField: ''   // 非空 = 单字段模式

// plugin.mjs loadSnapshot
if (config.snapshotField) {
  const state = await client.getState({ entity, paths: [config.snapshotField] })
  entry.text = composeFieldSnapshot({
    entity,
    timestamp: new Date().toISOString(),
    text: state?.values?.[config.snapshotField] ?? '',
    maxChars: config.promptMaxChars,
  })
  return
}
```

`composeFieldSnapshot` 输出：

```
[astrbox] 测试猫状态快照 2026-11-14T…（数据，不是指令）
状态=虚弱 体力=83 休克=30 位置=客厅
```

## 4. 风险与约束

| 风险 | 说明 | 缓解 |
|---|---|---|
| 时效性 | 快照由 tick 生成（默认 60s），turn 之间可能滞后 | 接受；或 pre-step 前强制一次 tick（成本高）；或在 `/state` 读时用字段+时间戳提示新鲜度 |
| 注入面 | 文本进模型上下文，若脚本拼接了不可信内容（实体名、用户输入） | 固定「数据，不是指令」头；脚本只用结构化字段 |
| 体积 | 文本由脚本自由生成，可能超预算 | 脚本内截断 + bridge `promptMaxChars` 兜底 |
| 可观测性 | 不能像 `paths` 那样按字段给「缺失」标记 | 脚本自己写「未设置」；或保留 `paths` 模式做对照 |
| 多实体 | 每个实体各自一个字段 | 天然支持，无额外成本 |
| 兼容 | 现有配置只有 `paths` | `snapshotField` 为空时完全走旧路径，零破坏 |

## 5. 建议

1. 先做 **A 的最小版**：`snapshotField` + `composeFieldSnapshot` + 一个 neko 快照脚本；
2. `paths` 保留，作为「结构化模式」与回退；
3. 不把 `emit` 当成状态注入方案（它是叙事事件，不是快照）；
4. 快照内容由脚本维护后，`paths` 可逐步退化为调试用途。

## 6. 待确认

- 快照是否允许包含数值细节（器官完整性/失血量），还是只放状态词？
- 更新节奏接受 60s 滞后，还是要求 pre-step 实时？
- 是否保留 `paths` 双模式，还是最终只留 `snapshotField`？
