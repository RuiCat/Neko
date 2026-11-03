# 单角色自学习扮演系统 · 落地规划（AstrBox 增量版）

- 状态：规划草案 v1（2026-10-31）
- 仓库：`/home/ruicat/Desktop/Neko`（AstrBox，Go 重写版猫娘角色引擎）
- 本次交付形态：**只出规划文档，不改源码**
- 落地范围：**增量叠在现有 Go 引擎上**（复用状态机 / 事件 / 实体 / MCP 面）
- 第一阶段优先级：**评估指标与真值基座**（不是 MoE，也不是训练）

## 0 结论摘要

1. 这套架构可以落，但第一件该做的事是**真值基座**：turn 级硬状态快照 + 交互日志 schema + 指标口径。没有它，后面每个模块（补偿包、门控、夜间修复）都没有可判定验收；这也正是主人选的优先级。
2. 有 **3 个裁决点**必须先定（第 3 节）：D1 本地 MoE v1 是否允许"非神经实现"、D2「状态机唯一写者」是否收窄为硬状态字段子集、D3 远程模型由谁调用/密钥归谁。
3. 与现有引擎有 **3 条硬冲突**，必须写进实现约束，否则约束 #1/#4/#10 只是口号：
   - 实体写入入口不止一个：`@函数`、MCP 工具、控制平面回调都能写（`docs/ARCHITECTURE.md:107-117` 与 `internal/control/sessionEvent.go:64`）。
   - `Tick` 是"被调用才跑"，不是事件驱动：事件总线只做"字段变化 → `@事件` → `@函数`"，不产生 Tick 输入（`internal/events/bus.go:44-80`）。
   - `TickID` 是 8 位随机十六进制（`internal/engine/engine.go:538-544`），不单调、不关联轮次、不能当 `source_snapshot_id` 用。
4. 建议 3 期推进，每期一条可执行闸门：P1 指标与真值基座 → P2 在线闭环 → P3 夜间离线。**闸门不过不进下一期**。

## 1 定级与范围

**定级：L2（单域深做，交付物 = 规划文档）。**
依据：本次只产出一份文档，无并行执行体、无写域冲突、不写源码；接口契约只出**草案**，真正的冻结放在实现期（届时按 L3 走 `task-planning` 的 G0–G4 与契约冻结）。若现在把契约冻死，等于在没跑过一天数据前锁死 schema，返工概率更高。

**本次明确不做**：
- 不改 `internal/engine` 的 Tick 语义、不改 `.neko` 编译器/脚本、不重写 `legacy/python-astrbox/`。
- 不在 Go 仓库里引入 ML 训练依赖、不训练任何模型、不接 GPU。
- 不动 `internal/monitor` 的 Web UI（除未来可能加只读展示）。
- 不替主人裁决 D1–D3；第 3 节给推荐值与推翻条件，规划正文按推荐值书写并标注假设。

## 2 现状核对（可复用 / 冲突 / 缺口）

| # | 规格能力 | 仓库现状 | 锚点 | 结论 |
|---|---|---|---|---|
| 1 | 状态机（硬状态裁决） | 规则表驱动 `Tick`；返回 transitions / conflicts / errors；进程内串行 gate | `internal/engine/engine.go:52-60`（TickResult）、`:177-187`（Tick）、`internal/mcpserver/runTick.go:55-71`（串行执行） | **复用**，仅在其后旁路挂日志与快照，不改语义 |
| 2 | 经历日志（快照序列） | `StateLogger` 写 `data/logs/state_YYYY-MM-DD.log`（JSONL），但**仅在 transitions/conflicts 非空时写**；字段只有 tick/scope/transitions/conflicts/hint/可选 snapshots | `internal/engine/stateLogger.go:30-41` | **改造**：新增独立 journal（不复用 StateLogger，语义不同），日志按"轮"而非"tick" |
| 3 | 状态快照 | `Entity.Snapshot()` 给全量 map；MCP `entity_snapshot` 手动全量归档到 `data/archives/<name>.json` | `internal/entity/model.go:188`、`internal/mcpserver/snapshot.go:72-77` | **改造**：新增 turn 级快照（硬状态子集 + canonical digest + 快照 ID）；现有工具保留做人工备份 |
| 4 | 事件总线 | `events.Bus`：字段变化 → schema 校验 → `@事件` → `@函数`，深度上限 10；控制平面 `/session-event` 把 DSH 会话事件转发进 `.neko` 回调 | `internal/events/bus.go:44-80`、`internal/control/sessionEvent.go:64-108` | **复用 + 新增桥**：事件是齐的，但**不驱动 Tick**；需新增"每轮 → tick"的调用点与事件→状态机候选桥 |
| 5 | 状态机唯一写者 | 多个入口可写实体：`@函数`（事件/条件/动作内部）、MCP state 工具、控制平面回调；MCP Tick 之后统一 `store.Save()` | `docs/ARCHITECTURE.md:107-117`、`internal/mcpserver/runTick.go:67-69`、`internal/control/sessionEvent.go:92-97` | **冲突**：只能把"硬状态"收窄为字段子集并加写入口守卫（见 D2 / C0） |
| 6 | 全局黑板 | 无 | — | **新建** `internal/blackboard`（三类字段、容量 1、只读视图） |
| 7 | 本地 MoE / 专家 / 门控 | 无任何相关代码 | — | **新建**；v1 建议规则/检索型专家（见 D1） |
| 8 | 补偿包组装器 | 无 | — | **新建** `internal/compensation` |
| 9 | 远程大模型 | 仓库内**没有任何 LLM 客户端**（无 provider/SDK/密钥读取） | `grep -rli 'openai\|deepseek\|anthropic' internal cmd` 仅命中历史注释与测试名 | **新建** `internal/llm`（provider 接口 + 环境变量密钥） |
| 10 | 夜间学习 | 无；仅有 `engine.decisionLog`（冲突仲裁落盘 `llm_decisions.json`）与规则回填 `llm_backfill` | `internal/engine/decisionLog.go:11-70` | **新建**；"修复"对象需裁决（见 D1 / 第 3 节），v1 修规则/策略版本而非权重 |
| 11 | 架构级指标 | 无 | — | **P1 新建**：`internal/metrics` + `cmd/astrbox-metrics` |
| 12 | 交互记录（用户输入 + 回答 + 补偿包 + 快照 ID） | 无（`reply_hint` 只是提示串） | `internal/engine/engine.go:57` | **新建** TurnRecord（C6），全线依赖 |

补充事实（影响排期）：
- 仓库 89 个非测试 Go 文件；`make build` / `make test` / `make lint` 是既有入口（`Makefile:32/50/60`）。
- `data/` 是本地运行状态、不进版本库；`runs/<slug>/` 是既有任务产物目录（参考 `runs/astrbox-go/00-scope.md`）。
- 仓库**没有 `.agent-memory/`**，也没有 `.dsh/backupProject.sh`（收尾按同一判据手动备份）。

## 3 裁决点（需主人拍板；正文按推荐值书写）

### D1 本地 MoE 的 v1 实现形态

| 选项 | 含义 | 代价 | 结论 |
|---|---|---|---|
| A（推荐） | 专家 = **规则/检索型组件 + Go 小模型**：隔离上下文（有界向量 + 检索片段）+ 确定性打分；门控/打分等小模型用 `算法导论` 仓库的 `cgraph`+`ml`（纯标准库）离线训练，权重以 JSON 导出，Neko 只做推理 | 训练不依赖 Python；Neko 不背 10k 行依赖（权重 JSON + 自写 forward）；真·大模型 MoE 仍不做 | 推荐 |
| B | 真神经 MoE 全部放进本仓库（vendor `cgraph`+`ml` 或 `require+replace`） | 依赖 10k 行外部代码、无 LICENSE、`go 1.27` 抬升工具链；行数与命名约束冲突 | 不建议 |
| C | 直接调远程模型扮演"专家" | 违反"本地 MoE 在有限上下文预算下组装"的立论，且每轮成本翻倍 | 不做 |

**证据（2026-11-01 实测）**：`/home/ruicat/Desktop/算法导论` 的 `cgraph`（2453 行）+ `ml`（7873 行）共 246 个测试、零外部依赖；`ml` 提供 MLP/逻辑/softmax、SGD/Adam/AdamW、L2、梯度裁剪、断点续训；`require ITA + replace` 探针训练 XOR MLP 成功（训练集 acc 1.000）。限制：`go.mod` 声明 `go 1.27.0`、无 LICENSE、`MLPModel` 无现成 Save/Load、无 embedding/ANN。

**假设（若不成立，结论变为）**：若要求的是"专家模型本身也是 LLM 级"，则超出本仓库能力，须另立 Python/GPU 项目；本规划只覆盖"小模型门控 + 检索型专家"。

### D2「状态机唯一写者」落地口径

| 选项 | 含义 | 代价 | 结论 |
|---|---|---|---|
| A（推荐） | 定义**硬状态字段子集**（关系/信任、在场、知识边界、物品、任务、场景约束），只有 `StateMachine`（及其 `Judge`/转移应用）能写；其余实体字段（情绪、身体、结构等）维持现状 | 需要一份字段清单 + 写入口守卫；与现有 `@函数` 写状态的脚本会冲突，需白名单/迁移期开关 | 推荐 |
| B | 全部实体字段都归状态机写 | 等于重写 `@函数` 与整个脚本体系，风险与工作量远超本任务 | 不做 |

**证据**：现有 `@函数` 直接写实体并触发事件（`docs/ARCHITECTURE.md:115-117`），全部字段收权会立刻打断 186 个脚本的重放与回调。

### D3 远程模型的调用归属

| 选项 | 含义 | 代价 | 结论 |
|---|---|---|---|
| A（推荐） | 引擎内 `internal/llm` 调 provider（OpenAI 兼容协议），密钥只从环境变量读，调用预算/超时/重试可配；DSH 会话模型只作开发期适配器 | 需管理额度与密钥边界（不写盘、不进日志） | 推荐 |
| B | 只走 DSH 会话，由宿主模型作答 | 每轮要过一遍会话层，难以做离线回放与消融（P1 的 D 类指标做不了） | 不推荐 |
| C | 两者都支持 | 接口要抽象得更多；P2 先 A，C 作为后续 | 可后置 |

**假设**：远程模型可提供 OpenAI 兼容接口；若不兼容，`internal/llm` 只保留 `Provider` 接口 + 一个适配器，按实际协议补。

## 4 接口契约草案（C0–C7）

> 原则：schema 先定、实现后补；每条都写"违反后果"与"怎么验"，否则实现期会各写各的。
> 命名遵循仓库约束：文件/函数/变量 camelCase，类型 PascalCase，一个源码文件一个功能，单功能文件 < 500 有效行。

### C0 硬状态子集（一切的基础）

- 形态（P1 已实现）：版本化 `Schema{Version, Group, Entities}`——按 `config` 的状态组捕获，`Entities` 为空表示全部实体；字段级清单（entityType/path/kind）留到关系/信任字段真正落地时再加。
- 硬约束：快照、补偿包 `state`、`judge` 裁决、夜间比对**只认这份清单**；清单变更必须升版本号并写迁移说明。
- 违反后果：同一字段两处口径 → 指标 #4 永远算不平。
- 验收（P1 已实现）：快照只收状态组字段，非状态组字段不泄漏（`internal/hardstate/snapshot_test.go`）；字段存在性差异由指标的分母/不匹配暴露，不静默跳过。

### C1 快照与快照 ID

```go
// internal/hardstate/snapshot.go
type Snapshot struct {
    SnapshotID string         `json:"snapshot_id"` // evt_<yyyymmdd>_<seq4>，单调递增
    TurnID     string         `json:"turn_id"`
    TickID     string         `json:"tick_id,omitempty"`
    TakenAt    time.Time      `json:"taken_at"`
    Digest     string         `json:"digest"` // sha256(canonicalJSON(hardState))
    State      map[string]any `json:"state"`
}
```

- **不复用 `TickID`**：8 位随机十六进制不可排序、不关联轮次（`internal/engine/engine.go:538-544`）。
- `State` 的键固定为 `<实体ID>.<点分路径>`（如 `猫猫.状态.情绪`），值只取状态组字段；这样多实体快照也能唯一定位，不需要二次查表。
- digest 规则：键排序、数字归一化（`1` 与 `1.0` 同值）、数组保序、UTF-8；同一状态必须得到同一 digest（黄金夹具断言）。
- 违反后果：`source_snapshot_id` 回溯不到 → 约束 #10 失效。
- 验收：同一 fixture 两次生成 digest 相等；改一个字段 digest 必变；seq 跨重启单调（读日志尾部恢复）。

### C2 全局黑板

```go
// internal/blackboard/board.go
type Event struct { ID, Kind, Summary string; Payload map[string]any }
type Constraints struct { Location, Time string; Present []string; KnowledgeBoundary []string }
type Board struct { Event Event; Snapshot hardstate.Snapshot; Constraints Constraints }
type Reader interface { Event() Event; Snapshot() hardstate.Snapshot; Constraints() Constraints }
```

- 写者仅两个：tick 管线（写快照）、事件分发器（写当前事件）；专家只拿 `Reader`。
- 硬约束：容量 1、无历史、不镜像向量库、序列化 ≤ 8 KiB（测试断言）。
- 违反后果：黑板膨胀成第二个向量库 → 上下文预算失控（规格 2.2 明确视为架构违规）。
- 验收：`blackboard_test.go` 断言"写两次只剩最后一次"、"专家拿不到写方法"（用接口类型限制）。

### C3 转移候选裁决

```go
// internal/engine/judge.go
type Candidate struct { EntityID, Path string; Proposed any; Source string; Evidence []string }
type Verdict struct { Status string /* legal|illegal|uncovered */; RuleID, Reason, SnapshotID string }
func (m *StateMachine) Judge(c Candidate) (Verdict, error) // 只读，不改状态
```

- `uncovered` 进候选队列（带衰减/回滚），不阻塞主流程；低置信进人工队列（离线文件，不进 TurnRecord 主路径）。
- 违反后果：候选直接改状态 → 绕过唯一写者（约束 #1）。
- 验收：表驱动测试覆盖 legal/illegal/uncovered；`Judge` 前后 `Digest` 不变。

### C4 补偿包

```go
type Fragment struct { Content, SourceSnapshotID, Type string; Weight float64 } // Type: retrieved|reconstructed
type Package struct {
    Memory             []Fragment    `json:"memory"`
    State              map[string]any `json:"state"`
    StateSource        string        `json:"state_source"` // 必须 == "state_machine_snapshot"
    PersonaConstraints []string      `json:"persona_constraints"`
    Situation          string        `json:"situation"`
    TurnID             string        `json:"turn_id"`
    SnapshotID         string        `json:"snapshot_id"`
}
```

- 硬约束：`state` 的键必须是 `<实体ID>.<点分路径>`，每个值必须能在同 `snapshot_id` 的快照里找到且相等；`reconstructed` 片段 `weight` 必须低于同源 `retrieved`；`source_snapshot_id` 必须能在 journal 中解析。
- 违反后果：本地模型自由编状态 → 人格信号矛盾（约束 #3）。
- 验收：组装后立即跑 `validatePackage(pkg, snapshot)`，失败不发给模型（fail-closed）。

### C5 专家 I/O 与门控

```go
type Input struct { UserInput string; Private PrivateContext; Board blackboard.Reader }
type Expert interface { ID() string; Compensate(ctx context.Context, in Input) ([]Fragment, error) }
type Gate interface { Activate(features Features) (active []string) } // 返回 2–3 个
```

- 专家只写自己的隔离上下文；上下文有界（每专家容量上限）+ 衰减算法在夜间调用。
- 门控 v1 是确定性打分（事件类型 + 硬状态 + 情感向量 + 摘要），接口保持"可替换为小 dense 网络"。
- 违反后果：专家写黑板/写状态 → 隔离失效、真值链断裂（约束 #4/#5）。

### C6 TurnRecord（交互日志，P1 的核心）

```json
{
  "record_version": 1,
  "turn_id": "evt_20261031_0001",
  "session_id": "dsh-session-id",
  "status": "incomplete",
  "started_at": "2026-10-31T23:00:00+08:00",
  "ended_at": "2026-10-31T23:00:01.2+08:00",
  "user_input": "...",
  "snapshot": {"snapshot_id": "evt_20261031_0001", "digest": "sha256:...", "tick_id": "ab12cd34"},
  "gate": {"active": ["关系记忆", "表达偏好"], "version": "gate-v1"},
  "compensation": { "memory": [], "state": {}, "state_source": "state_machine_snapshot", "...": "..." },
  "answer": "...",
  "answer_model": "provider/model",
  "prompt_version": "persona-v1",
  "cites": [{"event_id": "evt_20261030_0003", "kind": "event"}],
  "feedback": {"kind": "none|correct|preference", "note": ""},
  "latency_ms": 1234,
  "cost": {"prompt_tokens": 0, "completion_tokens": 0}
}
```

- 落盘：`data/journal/YYYY-MM-DD.jsonl`，只追加、异步写、**不持有 `tickMu`**（`internal/mcpserver/runTick.go:55` 的 gate 不能被磁盘 IO 拖住）。
- `answer == ""` 且 `status == "incomplete"` 允许存在（P1 还没有 LLM 时也要能落日志，否则指标没法先跑）。
- 违反后果：没有 turn 级记录 → 9 项指标中 8 项无法计算。

### C7 教师信号（远程副职）

```json
{
  "semantic_gate": "accept|reject",
  "affect_delta": {"trust": -0.05},
  "transition_candidates": [{"entity_id": "...", "path": "...", "proposed": "..."}],
  "rebuild": {"source_snapshot_id": "evt_...", "summary": "..."}
}
```

- 远程模型必须输出结构化信号（同一次调用或独立调用均可）；解析失败记 `teacher_signal_missing`，不阻塞回答。
- 这些信号**不是真值**：`affect_delta` 只能进候选队列，必须由 `Judge` 裁决后才可能成为状态（C3）。

## 5 指标口径（P1 的核心）

### 5.1 为什么要给指标分级

规格说"唯一不依赖模型自评的真值源是状态机快照"。但 9 项指标里只有不到一半能由快照直接判定：
"补偿覆盖率/精确率"要判断"模型是否使用了补偿"（自评），"难例比例"要本地与远程对比（模型对模型），
"消融敏感度"要重跑并评分。若把自评指标当训练闸门，等于用被测系统裁决自己——这会让真值链名存实亡。
因此指标分 5 级，**A 类是硬闸门，B–E 只作趋势或抽样统计**。

| 级 | 真值来源 | 能否机器判定 | 用途 |
|---|---|---|---|
| A | 快照/日志结构比对 | 是，零模型自评 | **硬闸门**（必须 100%） |
| B | 远程模型自报 + 抽样人工 | 部分（可机算，但含自评） | 趋势 + 抽样报告 |
| C | 冻结参照集 + 人工/评审 | 否 | 上线前评审 |
| D | 重跑对比（烧远程额度） | 是（成本高） | 抽样统计 |
| E | 修复后长窗口观察 | 是（需时间） | 版本复盘 |

### 5.2 九项指标

| # | 指标 | 定义/公式 | 真值来源 | 级 | P1 能否算 | 成本 |
|---|---|---|---|---|---|---|
| 1 | 补偿覆盖率 | `\|{e ∈ cites(answer) : e ∈ memory(pkg) ∧ consistent(e,snap)}\| / \|cites\|` | cites + 快照 | B | **部分**：可算"引用可回溯率"（cites 能否解析到 journal/快照），不宣称覆盖率 | 0 额外调用 |
| 2 | 补偿精确率 | `\|{f ∈ memory : used(f) ∧ consistent(f,snap)}\| / \|memory\|`；`used` 需标注 | 人工/自报 | B | 否（`not_computable` + 缺 `used` 标注） | 抽样人工 |
| 3 | 重建一致率 | `\|{f : f.type=reconstructed ∧ fieldsMatch(f,snap)}\| / \|{reconstructed}\|` | 快照 | A | 是（前提：重建摘要结构化，C7） | 0 |
| 4 | 跨轮状态一致率 | `1 - \|{f ∈ pkg.state : f ≠ snap[f]}\| / \|pkg.state\|` | 快照 | A | **是，硬闸门 = 100%** | 0 |
| 5 | 难例比例 | `\|{legal 样本 : localVerdict ≠ remoteSemanticGate}\| / \|legal 样本\|` | 本地打分 + 远程门 | D | 否（P2 才有本地门控） | 0 额外调用 |
| 6 | 修复收敛率 | `1 - 同类错误在 N=20 轮观察窗内复现次数 / 修复次数` | 修复版本 + 日志 | E | 否（P3） | 0 |
| 7 | 修复副作用率 | 非目标类别错误率相对基线的增量 | 同上 | E | 否（P3） | 0 |
| 8 | 参照集偏离度 | `1 - sim(answer, referenceReactions)`，阈值 `T` | 参照集 + 评审 | C | 否（P1 只建参照集 v0） | 评审人力 |
| 9 | 消融敏感度 | `(Q(full) - Q(without c)) / Q(full)`，按专家类别 c 抽样 | 重跑 + 评分 | D | 否（P2 后） | 每样本 × 5 类远程调用 |

### 5.3 抽样与标注协议（B/C 类的可执行口径）

- B 类：每日随机抽 **20 轮**；两名标注者独立标 `used(relevant)` 与 `correct`；报告 Cohen's κ；**κ < 0.6 时该指标不得作为闸门**，只进附录。
- C 类：参照集由人工编写并锁定（C 见 5.4），评审按"偏离/不偏离"三档（符合/可接受/偏离），只对偏离项计数；模型版本必须入日志，否则参照集偏离度会被模型升级污染。
- D 类预算：默认**每日 ≤ 50 次额外远程调用**（10 轮 × 5 类），超出即停并报告；不做全量消融。
- A 类：全量、逐轮、零成本，任何低于阈值的情况视为发布阻断。

### 5.4 冻结参照集协议（P1 必做）

- 位置：`fixtures/reference-set/v0.jsonl`（**进版本库**，是外部锚点，不可重算）。
- 字段：`{id, scenario, context, expected_reactions[], forbidden_reactions[], author, created_at, locked: true}`。
- 规模：v0 先 30 个场景 × 3 条反应 = 90 条，覆盖：关系升温/降温、知识边界被试探、回避话题、拒绝越界、情绪转折。
- 纪律：参照集**永不参与训练与自动修复**；任何修改升版本（v1、v2）并保留旧版；`v0.sha256` 旁证，指标计算时校验 hash。
- 验收：`referenceSet` 校验器（id 唯一、locked 全 true、场景数/条数符合预期、hash 匹配）。

## 6 阶段计划

### P1 指标与真值基座（当前优先级；预计 5 个实现步骤）

| 步 | 产物（新文件/新包） | 验收命令/判据 | 失败怎么办 |
|---|---|---|---|
| 1 | `internal/hardstate/schema.go`：硬状态清单 + 版本 | `go test ./internal/hardstate`：清单非空、字段存在性校验（无 `data/` 时 SKIP 并打印） | 清单与真实实体不符 → 只报告差异，不带进 P2 |
| 2 | `internal/hardstate/snapshot.go`：canonical digest + SnapshotID 生成 | 黄金夹具：同输入同 digest；改字段 digest 变；seq 单调 | digest 不稳定（浮点/键序）→ 先修归一化，不绕过 |
| 3 | `internal/journal/record.go` + `writer.go`：TurnRecord + 异步 JSONL | 10 条夹具落盘可解析、字段齐、顺序稳定；写失败不 panic | 磁盘错误 → 记诊断并丢该条，不阻塞 tick |
| 4 | 埋点：`internal/mcpserver/runTick.go:55` 后旁路写 journal；`internal/control/sessionEvent.go:64` 记 turn start | `make test` 全绿 + 本地起服务跑一次 tick 产生一条记录（无 LLM 时 `status=incomplete`） | 埋点导致 tick 变慢 → 改异步队列，不持 `tickMu` |
| 5 | `internal/metrics` + `cmd/astrbox-metrics`：A 类 + 引用可回溯率 | 3 个夹具（全一致/有不一致/缺快照）→ 期望 `metrics.json`；未知字段不静默忽略 | 指标定义歧义 → 回到 5.2 改口径，不改代码迁就 |
| 6 | `fixtures/reference-set/v0.jsonl` + 校验器 + `v0.sha256` | 校验器通过；hash 匹配 | 场景写不出 90 条 → 降到 60 条并记录缺口，不伪造 |

**P1 闸门（全过才进 P2）**：
- `跨轮状态一致率` 在夹具上可算且公式与文档一致；真实日志上能复算（哪怕样本只有个位数）。
- `引用可回溯率` 有值；`重建一致率` 标 `not_computable` 并给出缺失字段名（不假装算了）。
- 参照集 v0 锁定且 hash 校验通过。
- **不变量**：`git diff` 只新增文件 + 两处调用，`Tick`/规则/脚本零语义改动；`make test` 全绿。

### P2 在线闭环（黑板 + 专家 + 补偿包 + 远程模型）

| 步 | 产物 | 验收 |
|---|---|---|
| 1 | `internal/blackboard`（三类字段、容量 1、只读接口） | 覆盖测试：写两次只留最后一次；专家侧只能用 `Reader` |
| 2 | `internal/expert`（5 个规则型专家 + 隔离上下文 + 门控 v1） | 每专家上下文有界；门控稳定返回 2–3 个；无专家写黑板/状态 |
| 3 | `internal/compensation`（组装 + `validatePackage` fail-closed） | `state_source` 必为快照；不一致的包被拒且记诊断 |
| 4 | `internal/llm`（Provider 接口、env 密钥、超时/重试/预算计数） | 无密钥时启动明确报错；密钥不进日志/不进 TurnRecord；重试上限可配 |
| 5 | 在线接入 + `cmd/astrbox-replay`（从 TurnRecord 重建补偿包并比对） | 20 轮真实对话；回放 20/20 可重建（归一化 JSON 相等） |

**P2 闸门**：
- 20 轮真实对话里 `跨轮状态一致率 = 100%`；回放 20/20 成功；人工抽检 5 轮 ≥ 4 轮"可用"。
- tick 增量 P99 < 5 ms（异步日志不阻塞 gate）。
- 候选转移（`uncovered`）有落盘队列且不阻塞主流程。

### P3 夜间离线（纠错三分流 → 修复 → 回归闸门）

| 步 | 产物 | 验收 |
|---|---|---|
| 1 | `cmd/astrbox-nightly`：日终扫描（A 类全量 + B 类抽样） | 对一日日志产出 `triage.jsonl`，A 类问题逐条带快照 ID |
| 2 | 纠错三分流：事实/一致性/偏好 → 三类队列 | 偏好样本不进修复队列（约束 #9）有断言 |
| 3 | 回溯链 + 修复候选（**规则/策略/提示词版本补丁**，不训练权重） | 每条修复带 `source_snapshot_id` 与负例原包；无快照来源的样本拒收 |
| 4 | 回归闸门：参照集 + 最近 N 天回放；通过上线 / 失败回滚 | 修复版本化（hash + 可回滚）；失败样本留待重试 |
| 5 | 上下文衰减/清空 | 夜间结束后隔离上下文按容量/衰减规则收缩，有测试 |

**P3 闸门**：跑通一次真实闭环（发现错误 → 修复 → 回归 → 上线或回滚），全过程有记录。
**真模型训练**：不在本仓库；P1 之后额外产出 `training-samples-v1.jsonl` 导出格式（含快照锚点与负例），交给独立 Python 项目。

## 7 落点与数据规划（四类）

| 类别 | 内容 | 落点 | 进版本库 |
|---|---|---|---|
| 交付物/文档 | 本规划 | `runs/selflearn-arch/00-plan.md` | ✅ |
| 交付物（锚点） | 冻结参照集 v0 + hash | `fixtures/reference-set/v0.jsonl`、`v0.sha256` | ✅（外部锚点，不可重算） |
| 交付物（代码） | P1–P3 新包与新命令 | `internal/{hardstate,journal,blackboard,expert,compensation,llm,metrics}`、`cmd/astrbox-metrics`、`cmd/astrbox-nightly` | ✅ |
| 过程产物 | 交互日志、指标输出、学习中间物 | `data/journal/`、`data/metrics/`、`data/learning/`（均在已 gitignore 的 `data/` 下） | ❌ |
| 知识沉淀 | 真实坑与复算命令 | 建议 `.agent-memory/topics/`（当前仓库无此目录） | ✅（若建） |

## 8 风险清单（不美化）

| # | 风险 | 表现 | 处置 |
|---|---|---|---|
| R1 | 训练栈在另一个仓库 | 规格里的"参数更新/定向修复训练"不在本仓库；`算法导论` 的 `ml` 只覆盖小模型 | D1 推荐 A：小模型在 ITA 离线训练、权重 JSON 导入；Neko 不直接依赖 ITA。**不许**把"改了规则"说成"训练了模型" |
| R2 | 指标自评污染 | 覆盖率/精确率依赖模型自报 | 5.1 分级；B 类只做趋势；κ<0.6 不得当闸门 |
| R3 | 统计功效低 | 单角色系统样本少，收敛率/副作用率波动大 | 只做趋势与版本对比，不做显著性断言；观察窗固定 N=20 |
| R4 | 成本不可控 | 消融 ×5 类 × 每日轮次 | 5.3 预算（默认每日 ≤50 次额外调用），超预算即停并报告 |
| R5 | 并发/阻塞 | 日志/LLM 调用拖住 `tickMu`（`runTick.go:55`） | 异步 writer + 独立文件；LLM 在 tick 之外；压测 P99 闸门 |
| R6 | 写入口多 | 约束 #1 难落地 | D2 硬状态子集 + 写入口守卫 + 静态检查（禁止在 `hardstate` 包外写硬状态字段） |
| R7 | 现有回归面大 | 186 脚本 / 1808 规则（README 验证节） | P2 接入前后都跑 `make test`；不改 Tick 语义 |
| R8 | 快照 ID 语义 | 误用 8 位 TickID 做回溯 ID | C1 明令不复用；测试断言格式 `evt_<date>_<seq>` |
| R9 | 内容边界 | `data/` 含 r18 本地数据 | 不导出到指标样本/训练样本；导出器先过脱敏清单（P3 再定） |
| R10 | 契约返工 | 样板数据前冻死 schema | 本规划只出草案；P2 开工前按 L3 走 `task-planning` 冻结，冻结后变更需升版本 |

## 9 验收与复算

本规划（P0）的验收：

```bash
test -s runs/selflearn-arch/00-plan.md          # 文档存在且非空
grep -c '^## ' runs/selflearn-arch/00-plan.md   # 章节数符合预期（≥9）
# 锚点校验：文档中每个 `路径:行` 都指向真实存在的行（只读探针）
python3 - <<'PY'
import re,sys,pathlib
root=pathlib.Path('/home/ruicat/Desktop/Neko')
text=pathlib.Path(root/'runs/selflearn-arch/00-plan.md').read_text(encoding='utf-8')
bad=[]
for m in re.finditer(r'((?:internal|cmd|docs|scripts|runs|fixtures)/[\w./-]+\.(?:go|md|json|jsonl|neko)):(\d+)', text):
    p=root/m.group(1); ln=int(m.group(2))
    if not p.exists(): bad.append((m.group(0),'missing file')); continue
    if ln>len(p.read_text(encoding='utf-8',errors='replace').splitlines()): bad.append((m.group(0),'line out of range'))
print('anchors checked; bad=',bad)
sys.exit(1 if bad else 0)
PY
```

后续每期的验收命令已在第 6 节按步骤给出；P1 起以 `make test` / `make lint` 为回归基线。

## 10 与原始规格的差异（需确认，不静默改需求）

1. **本地 MoE v1 不追求全神经**（D1 推荐值）：门控/打分可用 `算法导论` 的 Go 小模型离线训练（权重 JSON），专家仍是检索/规则型；"专家模型也是 LLM 级"另立项目。
2. **夜间"参数更新"v1 改为规则/策略版本修复**：Go 仓库没有训练栈；"修复"以可回滚的版本补丁落地，收敛率定义在版本上。
3. **指标分级**：9 项指标全部保留，但只有 A 类作硬闸门；B–E 明示成本与污染。
4. **"状态机唯一写者"收窄为硬状态子集**（D2）：否则会打断现有 `@函数` 写状态体系。
5. **夜间上下文衰减先于参数固化**：v1 只做上下文衰减/清空与版本固化，"模型参数固化"留到独立训练项目。

> 以上 5 条若与主人预期不符，请在 P1 开工前指出；按"用户本轮要求 > 已核实证据 > 规划"的优先级改。

## 11 收尾动作（本次规划之后）

1. 备份：仓库无 `.dsh/backupProject.sh`，按同一判据手动 `tar --zstd` 到不同于 `/home` 的物理盘（如 `/run/media/ruicat/3ce0adc2-3284-475f-9780-5ed762f474cd`），并做 `zstd -t` + 解包 `rsync --checksum` + 文件数 + sha256 校验。排除 `.local/`（3.1G）、`bin/`、`data/`。
2. 记忆：仓库无 `.agent-memory/`；若要建，建议写入本文件第 0/3/5 节的 3 条结论（触发词：状态机真值、指标口径、MoE 训练边界）。
3. 下一步：P1 已实现（见第 12 节）；下一步是 P2 前的契约冻结（L3）与硬状态写入口守卫。

## 12 P1 实现记录（2026-11-02）

**状态**：P1 步骤 1–6 完成；`make build` / `make test` / `make lint` 全绿。未启用远程 LLM，因此记录均为 `status=incomplete`，指标只验证结构与口径，不宣称角色质量。

### 12.1 新增与修改

| 文件 | 职责 |
|---|---|
| `internal/hardstate/schema.go` | 版本化 `Schema{Version, Group, Entities}` 与 `DefaultSchema`（读 `config` 状态组） |
| `internal/hardstate/snapshot.go` | `Capture`（状态组扁平化、实体限定键）/ `DigestOf`（canonical JSON + sha256） |
| `internal/hardstate/sequence.go` | `evt_<yyyymmdd>_<seq>` 生成，跨日重置，`SetLast` 重启恢复 |
| `internal/journal/record.go` | TurnRecord（C6 全字段 + `record_version`/`status`） |
| `internal/journal/turn.go` | tick + 快照 → 记录；`TurnID == SnapshotID` |
| `internal/journal/writer.go` | 异步队列 writer：满即丢并计数，不阻塞 tick；`Close` 排空 |
| `internal/journal/reader.go` | `ReadDir`（排序、坏行报 file:line）/ `MaxSequence`（重启恢复） |
| `internal/metrics/report.go` | 指标词汇与 `computed` / `not_computable` 状态 |
| `internal/metrics/compute.go` | 状态一致率、重建一致率、引用可回溯率 |
| `cmd/astrbox-metrics/main.go` | 离线指标 CLI（`-journal` / `-out`，`-` 输出 stdout） |
| `internal/referenceset/referenceSet.go` | 参照集加载/校验/哈希核对 |
| `fixtures/reference-set/v0.jsonl` + `v0.sha256` | 30 场景 × 2 反应锚点（缺口见 12.2） |
| `internal/mcpserver/tickJournal.go` + `server.go` + `runTick.go` | tick 后写记录；`SetJournal` / `SetSessionID` |
| `internal/control/sessionEvent.go` | 会话事件写入 session id，供后续 tick 记录携带 |
| `internal/bootstrap/bootstrap.go` | 装配 journal、从日志尾部恢复序号、`Runtime.Close` |
| `Makefile` | 新增 `bin/astrbox-metrics` 目标并纳入 `build` |

### 12.2 与冻结前契约的偏差（已回写 C0/C1/C4/C6）

1. `C0` 未做字段级清单，v1 = 状态组 + 实体过滤——关系/信任/知识边界字段在当前数据里还不存在，提前列清单会造假字段。
2. `TurnID == SnapshotID`：P1 一轮一快照；多快照（前/后）留到补偿包落地时。
3. P1 记录 `status=incomplete`、`answer` 为空：没有远程模型，不伪造回答。
4. 参照集 v0 为 30 场景 × 2 反应 = 60 条，第三反应缺口按计划下限记录；30 场景数达标。
5. `session_id` 由控制平面 `/session-event` 设置；纯 stdio MCP 无会话事件时为空。

### 12.3 验证（实际输出）

```bash
make build        # 4 个二进制，含 bin/astrbox-metrics（4.8 MB）
make test         # exit 0；Go 全包 ok，node 测试 47 pass
make lint         # 238 个源码文件，0 项违规、0 项告警（vet/gofmt 亦通过）
go test ./internal/hardstate ./internal/journal ./internal/metrics ./internal/referenceset  # ok
./bin/astrbox-metrics -journal .local/metrics-probe -out -   # 2 条记录：
#   cross_turn_state_consistency = 0.5000 (1/2)
#   rebuild_consistency = not_computable
#   citation_traceability = 1.0000 (1/1)
./bin/astrbox-metrics -journal .local/metrics-probe-empty -out -  # 三项均 not_computable
```

参照集（30 行）JSONL 合法，`v0.sha256` 校验通过：`b174521974c54b1b185e9a6db34e95752fe27c65935336064199381bca4f5949`。

### 12.4 副作用与环境说明

- `data/dsh-control.sock`（10-27 遗留的 Unix socket）会让 e2e/registry 测试的 `CopyFS` 报 `invalid argument`；已**移动**到 `.local/stale-runtime/dsh-control.sock.20261027`（未删除），移动后全量测试通过。
- 探针与临时目录都在 `.local/`（已 gitignore）：`metrics-probe*`、`ita-probe/`、`ita-use-probe/`。
- 未写项目记忆（仓库无 `.agent-memory/`）；未做备份（外接盘全部只读，见第 11 节）。

## 13 P2 前置记录（2026-11-03）

**状态**：契约冻结 v1、gate 接口与规则门控、硬状态写入口守卫（告警级）完成；`make test` / `make lint` 全绿。P2 的黑板/专家/补偿包/LLM 实现**未开始**。

### 13.1 新增

| 文件 | 职责 |
|---|---|
| `runs/selflearn-arch/01-contracts-freeze.md` | C0–C7 冻结契约、D1–D3 决策、写域与变更流程 |
| `internal/gate/gate.go` | `Features`、`Gate` 接口、5 个专家 ID（与 journal 对齐） |
| `internal/gate/ruleGate.go` | 确定性 `RuleGate`（`gate-v1`）：2–3 个专家、规范顺序、默认回退 |
| `internal/gate/ruleGate_test.go` | 空特征回退、问题路由、低信任、上限 3、确定性 |
| `internal/hardstate/guard.go` | `CheckWrite(schema, entityID, path, source)`：只有状态机可写硬状态 |
| `internal/hardstate/guard_test.go` | 来源矩阵、组外路径、实体过滤、未配置停用、`Covers` |
| `internal/hardstate/schema.go` | 新增 `Schema.Covers(path)`，守卫与补偿包共用同一判定 |
| `internal/mcpserver/server.go` | `entity_state_set` 写硬状态时记告警（不阻断，D2 迁移期） |

### 13.2 修复：journal 改为显式启用

异步 writer 曾在控制平面测试里与 `t.TempDir` 清理赛跑，导致 `TempDir RemoveAll cleanup: directory not empty` 偶发失败。修法：`bootstrap.Options.Journal bool` 显式开启，默认关闭；两个生产入口（`cmd/astrbox-mcp`、`cmd/dsh-mcp`）置 `true`，测试与嵌入方不再起后台 writer。`internal/control` 连跑 8 次无复现。

### 13.3 验证（实际输出）

```bash
make test    # exit 0；Go 全包 ok，node 测试 47 pass
make lint    # 243 个源码文件，0 项违规、0 项告警（vet/gofmt 通过）
go test ./internal/gate ./internal/hardstate ./internal/mcpserver  # ok
```

### 13.4 下一步（P2 实现批次）

按 `01-contracts-freeze.md` 第 1 节写域并行：`blackboard` / `expert` / `compensation` / `llm` 四包互不重叠，接线（bootstrap、mcpserver、Makefile）由 Lead 统一改；每包先交付接口 + 单元测试，再接入 20 轮真实对话验证 P2 闸门。未走独立红队复核（仍是单执行体），若要求 L3 需另开一轮。

## 14 P2 在线闭环实现记录（2026-11-03）

**状态**：四包（黑板 / 专家 / 补偿包 / LLM 适配器）实现并单测通过；每次 tick 的 dry-run 已把补偿包写进 journal。**P2 闸门未达成**：没有真实 provider key，也没有 20 轮真实对话；`cmd/astrbox-replay` 尚未实现。

### 14.1 新增

| 文件 | 职责 |
|---|---|
| `internal/blackboard/board.go` | C2 黑板：容量 1、无历史、8 KiB 上限、`Reader` 只读接口 |
| `internal/expert/expert.go` | C5 `Entry`/`Fragment`/`Expert` 接口与有序注册表 |
| `internal/expert/context.go` | 每专家有界隔离上下文（默认 8 条）+ 夜间 `Decay` |
| `internal/expert/recall.go` | 5 个规则型专家（按 gate ID 注册），只返回自己的最新记忆 |
| `internal/compensation/assemble.go` | 门控 → 专家 → 片段上限 → 快照投影 → 校验（fail-closed） |
| `internal/compensation/validate.go` | `state_source`、状态值对快照、片段来源/类型/权重、reconstructed 低权重 |
| `internal/llm/provider.go` | `Provider` 接口、请求/响应/用量、缺 key/预算错误 |
| `internal/llm/httpProvider.go` | OpenAI 兼容适配器：429/5xx 重试、超时、key 只从 env、错误不泄漏 key |
| `internal/llm/budget.go` | token 预算（调用前 Check、调用后 Add） |
| `internal/mcpserver/tickJournal.go` | dry-run 接线：tick → 黑板快照 → 补偿包 + `gate.active` 入 journal |
| `internal/bootstrap/bootstrap.go` | Runtime 装配 Board/Experts/Assembler，并接入 MCP server |

### 14.2 验证（实际输出）

```bash
make test    # exit 0；Go 全包 ok，node 测试 47 pass
make lint    # 256 个源码文件，0 项违规、0 项告警（vet/gofmt 通过）
go test ./internal/blackboard ./internal/expert ./internal/compensation ./internal/llm ./internal/mcpserver  # ok
```

`TestRunTickWritesJournalRecord` 现同时断言：补偿包 `state_source=state_machine_snapshot`、状态值来自快照、激活专家 2–3 个、门控版本 `gate-v1`。

### 14.3 与判据的差距（不美化）

- **P2 闸门未过**：20 轮真实对话 / 回放 20/20 / 人工抽检 5 轮均未做；`cmd/astrbox-replay` 未实现。
- **LLM 只有适配器**：没有接入在线回答链路，`Provider` 尚无调用方；密钥环境变量名待定。
- **专家是规则型**：记忆来源只有 `Remember` 手工写入，真实对话检索（事件/关系片段抽取）未做；因此 journal 里的 `memory` 目前为空。
- **写入口守卫仍为告警**：D2 迁移未完成；`@函数` 写状态仍会触发告警日志。
- **未走独立红队/复核**：仍是单执行体实现。

## 15 C3 裁决与回放审计（2026-11-03）

**状态**：C3 `Judge` 与 `cmd/astrbox-replay` 完成；`make test` / `make lint` 全绿。LLM 在线接入仍未做——现有 MCP 工具面（doctor 校验的冻结工具集）与 K1 控制平面都不允许自行加对话接口，需先定入口与密钥环境变量。

### 15.1 新增

| 文件 | 职责 |
|---|---|
| `internal/engine/judge.go` | `Candidate`/`Verdict`/`Judge`：只读裁决 `legal`/`illegal`/`uncovered`；`coversPath` 按启用规则的 `set/emit` 目标路径判覆盖；不写状态、不改规则 |
| `internal/engine/judge_test.go` | 覆盖命中、实体限定/`{e}` 路径、未覆盖、实体不存在、禁用规则、只读性（快照与规则数不变） |
| `internal/replay/replay.go` | 审计 journal：包对快照重校验、`turn_id`/`snapshot_id` 一致、`gate.active` 2–3 |
| `internal/replay/replay_test.go` | 有效记录、篡改状态、专家数异常、无补偿包的旧记录只计数 |
| `cmd/astrbox-replay/main.go` | 审计 CLI：有 finding 即退出码 1，可写 JSON 报告 |
| `Makefile` | 新增 `bin/astrbox-replay` 并纳入 `build` |

### 15.2 验证（实际输出）

```bash
make build   # 5 个二进制，含 bin/astrbox-replay
make test    # exit 0；Go 全包 ok，node 测试 47 pass
make lint    # 261 个源码文件，0 违规 0 告警
./bin/astrbox-replay -journal .local/replay-probe-good
# records=1 with_compensation=1 valid=1 invalid=0   → exit 0
./bin/astrbox-replay -journal .local/replay-probe-bad
# records=1 with_compensation=1 valid=0 invalid=1
#   - evt_20261101_0003 [compensation] 状态字段 "猫猫.状态.情绪" 与快照不一致   → exit 1
```

### 15.3 差距与下一步

- `Judge` 尚无调用方：教师信号（C7）接入后才产生候选；`uncovered` 候选队列（衰减/回滚/人工队列）未实现。
- `replay` 是**审计**（重校验已记录包），不是"从记录重建补偿包并逐字节比对"；后者需要把专家上下文也持久化，否则不可复现。
- P2 闸门仍未过：20 轮真实对话、LLM 在线回答、人工抽检都还缺；入口需主人裁决（MCP 新工具 vs 控制平面新端点 vs 独立 chat 进程）。

## 16 独立对话进程与 20 轮 dry-run（2026-11-03）

**入口裁决已按推荐执行**：独立 `cmd/neko-chat` 进程，不动冻结的 17 个 MCP 工具与 K1 控制平面；密钥只读 `NEKO_LLM_ENDPOINT` / `NEKO_LLM_API_KEY` / `NEKO_LLM_MODEL`。

### 16.1 新增

| 文件 | 职责 |
|---|---|
| `internal/chat/chat.go` | 一轮对话：取快照 → 黑板 → 补偿包 → 远程模型 → 完整 TurnRecord（输入/回答/包/用量/耗时） |
| `internal/chat/persona.go` | 内置人格提示（只依据补偿包作答，不编造过去事件） |
| `internal/chat/chat_test.go` | 完整记录、dry-run、provider 失败写 error 记录、历史有界、提示顺序（人格/补偿包/输入） |
| `cmd/neko-chat/main.go` | stdin 一行一轮的独立进程；`-dry-run` 不调模型；每轮先 `RunTick(SkipJournal)` 再写一条完整记录 |
| `internal/journal/turn.go` | 新增 `NewTurnRecord`（chat 用已有快照建记录），tick 路径行为不变 |
| `internal/mcpserver/runTick.go` | `TickOptions.SkipJournal`（chat 避免重复写空记录） |
| `Makefile` | 新增 `bin/neko-chat` |

### 16.2 20 轮端到端 dry-run（实际输出）

```bash
cp -r data .local/chat-run
printf '...20 行...' | ./bin/neko-chat -data-dir .local/chat-run -scripts-dir scripts -dry-run
# → 20 条 [dry-run] 输出，active=[事件记忆 表达偏好]，state_fields=34
./bin/astrbox-replay -journal .local/chat-run/journal
# records=20 with_compensation=20 valid=20 invalid=0          → exit 0
./bin/astrbox-metrics -journal .local/chat-run/journal -out .local/chat-run/metrics.json
# cross_turn_state_consistency = 1.0000 (680/680)
# rebuild_consistency = not_computable（无 reconstructed 片段）
# citation_traceability = not_computable（无回答引用）
```

`make test` exit 0（node 47 pass）、`make lint` 265 文件 0 违规 0 告警。

### 16.3 P2 闸门逐条对照

| 闸门项 | 状态 |
|---|---|
| 20 轮记录 / 补偿包齐 | **dry-run 达成**：20/20 带包 |
| 跨轮状态一致率 100% | **达成**：680/680 |
| 回放 20/20 | **dry-run 达成**：replay 20/20 valid |
| 真实 LLM 回答 + 人工抽检 5 轮 | **未达成**：无 API key，20 轮里没有 `status=complete` 的记录 |
| tick 增量 P99 < 5ms | **未测** |

### 16.4 下一步

1. 配好 `NEKO_LLM_*` 后去掉 `-dry-run` 跑 20 轮真实对话，得到 `status=complete`、回答、用量与人工抽检。
2. `memory` 仍为空：需要把真实对话/事件写入专家上下文（检索与抽取尚未实现），否则覆盖类指标无法计算。
3. 候选队列与 C7 教师信号仍未实现。

## 17 修正：模型就是 DSH，补偿包走插件注入（2026-11-03）

**修正**：上一轮把"远程大模型"实现成引擎内 HTTP provider 是方向错误。本仓库已集成 DSH（`dsh/` bundle、`/session-event`、`agent/pre-step` 注入），模型由 DSH 提供；正确做法是把补偿包送到 DSH 的每轮提示里，而不是再建一个模型客户端。

### 17.1 实现

| 文件 | 职责 |
|---|---|
| `internal/control/turnContext.go` | `GET /turn-context`：按实体生成 `ctx_` 投影快照 → 黑板 → 补偿包（**不写 journal**） |
| `internal/control/router.go` | 注册端点（K1 附加，不改既有端点语义） |
| `internal/control/turnContext_test.go` | 包形状、缺实体 400/404、不写实体 |
| `dsh/lib/controlClient.mjs` | `getTurnContext` |
| `dsh/lib/prompt.mjs` | `composePackage`：状态按 `paths` 过滤 + 记忆 + 约束 + 情境 + 激活专家 |
| `dsh/lib/plugin.mjs` | `agent/pre-step` 优先取补偿包；旧引擎/失败回退 `/state` |
| `dsh/lib/pluginTest.mjs` | 新增"优先注入补偿包且不再请求 /state"用例 |

### 17.2 语义与取舍

- `ctx_` 快照是**投影**，不进入经历日志；因此它不能作为 `source_snapshot_id` 的最终回溯锚点（等答案回写后再升级为 `evt_`）。
- `cmd/neko-chat` 与 `internal/llm` 保留为**可选回退/本地 dry-run**，不再声称是在线路径；真实在线路径 = DSH 插件注入。
- 控制面新增端点是附加的：既有 `/state`、`/tick`、`/session-event` 语义不变。

### 17.3 验证（实际输出）

```bash
go test ./internal/control -run 'TestTurnContext'   # PASS（包形状/400/404/不写实体）
node --test dsh/lib/*Test.mjs                       # 48 pass（新增补偿包注入用例）
make test                                           # exit 0
make lint                                           # 267 文件，0 违规 0 告警
```

### 17.4 差距

- **DSH 轮的 TurnRecord 回写**：见第 18 节（已把 `turn-end` 摘要写成 complete 记录，tick 记录被指标/回放排除）。
- 真实 20 轮现在**不需要 API key**（模型由 DSH 提供），但还没在已装插件的 DSH 会话里实跑。
- `memory` 仍为空：专家上下文没有写入源（真实对话检索未实现）。

## 18 DSH 轮记录回写（2026-11-03）

**状态**：`turn-end` 的助手摘要现在会写成一条 `source=dsh-turn` 的 complete TurnRecord；引擎周期 tick 记录标 `source=tick`，指标与回放跳过它们，避免把状态快照当对话轮重复计数。

### 18.1 实现

| 文件 | 职责 |
|---|---|
| `internal/journal/record.go` | 新增可选 `source` 字段与 `SOURCE_TICK` / `SOURCE_DSH_TURN` / `SOURCE_CHAT`；tick 记录写 `tick`，chat 写 `chat` |
| `internal/control/turnRecord.go` | `recordDSHTurn`：`turn-end` + `summary.assistantDigest` → 用新 `evt_` ID 建快照/补偿包 → append complete 记录（`session_id`、`answer`、`gate`、`ended_at`） |
| `internal/control/sessionEvent.go` | 回调缺失/成功两条路径都调用回写；回调失败不写（避免记录不一致状态） |
| `internal/control/turnContext.go` | 抽出 `buildTurnContext(ctx, id, entity, now)`，`/turn-context` 与轮末回写共用 |
| `internal/metrics/compute.go`、`internal/replay/replay.go` | 跳过 `source=tick`；`Records` 仍统计全部 |
| `internal/control/turnRecord_test.go`、metrics/replay 测试 | 回写记录、tick 标记、指标/回放排除 |

### 18.2 验证（实际输出）

```bash
go test ./internal/control -run 'TestTurnEnd|TestTickRecords'   # PASS
go test ./internal/metrics ./internal/replay                    # PASS（tick 排除）
make test                                                       # exit 0（node 48 pass）
make lint                                                       # 269 文件，0 违规 0 告警
```

### 18.3 语义与残留

- 回写时机在 `.neko` 回调之后（回调缺失也回写），快照是轮末状态；`user_input` 由 `summary.userDigest` 提供（第 19 节），旧插件没有该字段时仍为空。
- 回写记录与 tick 记录共享同一个 `evt_` 序列，但只有回写记录参与指标/回放。
- **真实 20 轮仍未实跑**（需要重启 DSH 让插件加载新代码）；`memory` 由第 19 节的轮末写入填充；覆盖率/精确率仍缺样本。

## 19 用户原文上报与专家记忆写入（2026-11-03）

**状态**：`turn-end` 摘要新增 `userDigest`；回写记录带 `user_input`；轮末把本轮回写入专家隔离上下文，下一轮 `/turn-context` 的补偿包能召回（`memory` 不再为空）。

### 19.1 实现

| 文件 | 职责 |
|---|---|
| `dsh/lib/turnSummary.mjs` | 反向扫描同一轮事件，提取最后一条 `user/message` 文本为 `userDigest`（≤1200 字） |
| `dsh/lib/turnSummaryTest.mjs` | 覆盖用户原文提取与旧字段兼容 |
| `internal/control/sessionEvent.go` | `sessionEventSummary.UserDigest` |
| `internal/control/turnRecord.go` | `record.UserInput = summary.userDigest`；`rememberTurn` 把本轮写入专家上下文；`memoryTargets` 规则路由（事件/表达必记；问句→知识；关系词→关系；情绪词→自我） |
| `internal/expert/expert.go` | `Registry.Remember(id, Entry)`（未知/非召回专家返回 false） |
| `internal/control/turnRecord_test.go` | 断言 `user_input`、下一轮补偿包召回本轮记忆且 `source_snapshot_id == turn_id` |

### 19.2 验证（实际输出）

```bash
go test ./internal/control -run 'TestTurnEnd|TestTurnContext|TestTickRecords'   # PASS
node --test dsh/lib/turnSummaryTest.mjs dsh/lib/sessionEventsTest.mjs           # 17 pass
make test    # exit 0（node 48 pass）
make lint    # 269 文件，0 违规 0 告警
```

### 19.3 语义与残留

- 记忆内容 = `用户: … | 助手: …`（各截断），权重 0.7，每专家容量 8；`At` 已写入但不参与排序（按插入序召回）。
- 路由是**确定性关键词**，不是训练模型；`Decay` 已有实现但夜间衰减尚未接线。
- 覆盖率/精确率指标仍未实现（只做了引用可回溯率）；`memory` 里是 `retrieved`，重建一致率仍 `not_computable`。
- **真实 20 轮仍未实跑**；需要重启 DSH 加载插件新代码，然后看 replay 的 20 条 `dsh-turn` 记录与补偿包 `memory`。

## 20 控制面 20 轮端到端与记忆去重（2026-11-03）

**状态**：用真实 `bin/dsh-mcp` 的 Unix socket 控制面跑了 20 条 DSH 轮末事件（**模拟插件上报，不是真实模型对话**），验证回写、记忆召回、回放与指标；同时修掉"同一轮被多个专家重复召回"的问题。

### 20.1 端到端步骤与结果

```bash
# 1) 复制数据、启动真实控制面（临时目录，避开仓库 data/）
cp -r data .local/dsh-e2e2
./bin/dsh-mcp -data-dir .local/dsh-e2e2 -scripts-dir scripts -monitor-addr ""
# 2) 向 <data>/dsh-control.sock POST 20 次 turn-end（带 userDigest/assistantDigest）
# 3) 观察补偿包召回
curl --unix-socket .local/dsh-e2e2/dsh-control.sock \
  "http://localhost/turn-context?entity=猫猫"
# → memory 2 条，来源 evt_..._0019 / evt_..._0020（去重后）
# 4) 审计与指标
./bin/astrbox-replay -journal .local/dsh-e2e2/journal
# records=20 with_compensation=20 valid=20 invalid=0            → exit 0
./bin/astrbox-metrics -journal .local/dsh-e2e2/journal -out .local/dsh-e2e2/metrics.json
# cross_turn_state_consistency = 1.0000 (120/120)
# rebuild_consistency / citation_traceability = not_computable
```

行记录核对：20 条全部 `source=dsh-turn`、`status=complete`、`user_input` 与 `answer` 非空、补偿包带 `memory`。

### 20.2 去重修复

- `internal/compensation/assemble.go` 新增 `dedupeFragments`：按 `(source_snapshot_id, type, content)` 去重并保留最高权重；上限在去重后应用。
- 原因：激活的多个专家各自持有隔离上下文，同一轮会被多次召回（修复前 4 条重复，修复后 2 条）。
- 测试：`TestDedupeFragmentsKeepsHighestWeight`。

### 20.3 验证（实际输出）

```bash
make test    # exit 0（node 48 pass）
make lint    # 269 文件，0 违规 0 告警
```

### 20.4 诚实边界

- 这不是"真实 20 轮对话"：模型没有参与，20 条 turn-end 是本喵按插件载荷手工 POST 的；**真实对话仍需重启 DSH、让插件加载新代码后在 GUI 里聊**。
- 覆盖率/精确率仍缺标注协议；`memory` 是 `retrieved`，重建一致率不可算。
- 探针目录在 `.local/dsh-e2e`、`.local/dsh-e2e2`（gitignored），可删。

## 21 夜间衰减接线（2026-11-03）

**状态**：控制面新增 `POST /nightly`，对全部专家上下文做权重衰减；`Registry.DecayAll` 汇总每个专家被移除的条数。这是 P3"隔离上下文衰减"的引擎侧入口。

### 21.1 实现

| 文件 | 职责 |
|---|---|
| `internal/control/nightly.go` | `POST /nightly`：可选 `{"factor":0.5}`，校验 `(0,1)`；返回 `removed` 与 `per_expert` |
| `internal/control/router.go` | 注册端点（K1 附加） |
| `internal/expert/expert.go` | `Registry.DecayAll(factor) map[string]int` |
| `internal/expert/recall.go` | `RecallExpert.Decay(factor)` 委托给上下文 |
| `internal/control/nightly_test.go` | 默认 0.5、非法 factor 400、按专家统计移除数 |

### 21.2 验证（实际输出）

```bash
go test ./internal/control -run 'Nightly'      # PASS（3 个用例）
go test ./internal/expert -run 'Decay|Context' # PASS
make test                                      # exit 0（node 48 pass）
make lint                                      # 271 文件，0 违规 0 告警
```

### 21.3 残留（重要）

- **没有调度器调用它**：`POST /nightly` 目前只能手工触发；夜间 cron/定时器未接线。
- **专家上下文在内存里**：进程重启即丢失，衰减只对当前进程有效；P3 要真正"夜间巩固"还需要把上下文/记忆持久化。
- 覆盖率/精确率指标、候选队列、C7 教师信号仍未实现；真实 DSH 20 轮仍需重启 GUI 后人工跑。

## 22 专家上下文持久化（2026-11-03）

**状态**：专家隔离上下文落盘到 `<dataDir>/experts.json`，启动时加载、轮末与夜间衰减后保存、`Runtime.Close` 兜底保存；补上第 21 节"重启即丢"的缺口。

### 22.1 实现

| 文件 | 职责 |
|---|---|
| `internal/expert/persistence.go` | `SaveFile`（原子写）/ `LoadFile`（缺失=空、坏文件报错）；`PERSIST_VERSION` |
| `internal/expert/context.go` | `Context.Replace`：容量内恢复，超出丢最旧 |
| `internal/expert/expert.go` | `Registry.Snapshot` / `Registry.Load` |
| `internal/expert/recall.go` | `RecallExpert.Entries` / `Replace` |
| `internal/bootstrap/bootstrap.go` | 启动加载 `experts.json`；新增 `Runtime.SaveExperts`；`Close` 先保存再关 journal |
| `internal/control/turnRecord.go` | 轮末记忆写入后立即保存 |
| `internal/control/nightly.go` | 衰减后保存，失败返回 500（不静默丢） |
| 测试 | 持久化往返、缺失/损坏文件、bootstrap 加载与 Close 保存、nightly 保存 |

### 22.2 验证（实际输出）

```bash
go test ./internal/expert ./internal/bootstrap ./internal/control   # PASS
make test    # exit 0（node 48 pass）
make lint    # 274 文件，0 违规 0 告警
```

### 22.3 语义与残留

- 落盘是**整文件原子替换**：单角色上下文很小（每专家 ≤8 条），当前规模可接受；条目数上涨后要换增量存储。
- 损坏的 `experts.json` 会**报错并跳过加载**（不静默清空）；启动日志会指明失败。
- 仍**没有调度器**调用 `/nightly`；夜间触发需要外部 cron 或 Host 定时。
- 覆盖率/精确率指标、候选队列、C7 教师信号未实现；真实 DSH 20 轮仍需重启 GUI 后人工跑。

## 23 夜间衰减调度器（2026-11-03）

**状态**：运行时内置按本地时间每日触发的衰减调度；两个入口都加了 `-nightly-at` / `-nightly-factor`。手工 `POST /nightly` 仍保留。

### 23.1 实现

| 文件 | 职责 |
|---|---|
| `internal/bootstrap/nightly.go` | `parseNightlyAt`（HH:MM）、`nextNightly`（严格晚于 now，跨日）、`startNightly`/`runNightly`/`stopNightly` |
| `internal/bootstrap/bootstrap.go` | `Options.NightlyAt`/`NightlyFactor`；Runtime 持有 stop/done 通道；`Close` 先停调度再落盘/关日志 |
| `cmd/dsh-mcp/main.go`、`cmd/astrbox-mcp/main.go` | 新 flag，传入 Runtime |
| `internal/bootstrap/nightly_test.go` | 解析、跨日/当日/等于 now 的排期、Close 停止、非法时间不启用 |

触发时执行：`Experts.DecayAll(factor)` → `SaveExperts()` → 日志（移除条数）。默认 `factor=0.5`；不在 `(0,1)` 或时间非法则**不启用**并记录原因。

### 23.2 验证（实际输出）

```bash
go test ./internal/bootstrap -run 'Nightly|NextNightly|ParseNightly'   # PASS
make test    # exit 0（node 48 pass）
make lint    # 276 文件，0 违规 0 告警
```

### 23.3 语义与残留

- 调度是**进程内**的：进程必须在该时刻存活；**错过的触发点不补跑**（没有 catch-up）。
- 多进程/多副本会各自衰减一次；本仓库是单实例（dsh-mcp 有单实例锁），当前不冲突。
- 时间段使用宿主本地时区；DST 边界沿用 `time.Date` 语义，未单独测试。
- 覆盖率/精确率指标、候选队列、C7 教师信号仍未实现；真实 DSH 20 轮仍需重启 GUI 后人工跑。

## 24 标注协议与 B 类指标（2026-11-03）

**状态**：新增人工标注协议（JSONL）与 `compensation_coverage` / `compensation_precision` 两个 B 类指标；无标注时显式 `not_computable`，不伪造数值。

### 24.1 标注格式（`data/annotations/*.jsonl`）

```json
{"turn_id":"evt_...","used_fragments":[0],"cites":[{"event_id":"evt_...","kind":"event"}],
 "correct":true,"annotator":"lead","created_at":"2026-11-03"}
```

- `used_fragments`：`TurnRecord.Compensation.Memory` 的 0 基下标，人工判定"确实被回答使用"。
- `cites`：人工从回答里抽出的过去事件引用。
- 加载器校验：`turn_id` 唯一、`annotator`/`created_at` 非空、下标非负、`event_id` 非空；坏行/重复直接报错。

### 24.2 公式与边界

| 指标 | 分子 | 分母 | 说明 |
|---|---|---|---|
| 补偿覆盖率 | cites 中既在补偿包 memory 里、又能解析到 journal 快照的条数 | 已标注的 cites 总数 | 无标注/无 cites → `not_computable` |
| 补偿精确率 | used_fragments 中来源可解析且内容非空的条数 | 已标注的 used_fragments 总数 | 语义"正确"仍需人工；机器只验来源可溯 |

- 未标注的 turn 不参与 B 类指标；`Report.annotated` 给出标注轮数。
- 精确率这里是**可溯源性代理**，不代表语义正确；与 A 类状态一致率分开看。

### 24.3 实现与验证

| 文件 | 职责 |
|---|---|
| `internal/annotations/annotations.go` | `LoadDir` / `Validate`，重复与坏行 fail-loud |
| `internal/metrics/compute.go` | `ComputeWithAnnotations`、`compensationCoverage`、`compensationPrecision` |
| `cmd/astrbox-metrics/main.go` | `-annotations`（默认 `data/annotations`，缺失=无标注） |
| 测试 | annotations 4 例；metrics 标注用例与"无标注 not_computable" |

实测（20 轮控制面 journal + 1 条人工标注）：
```bash
./bin/astrbox-metrics -journal .local/dsh-e2e2/journal -annotations .local/ann -out .local/ann/report.json
# annotated=1
# compensation_coverage   computed 1.0000 (1/1)
# compensation_precision  computed 1.0000 (1/1)
# 无标注目录：两者均 not_computable
make test  # exit 0（node 48 pass）
make lint  # 278 文件，0 违规 0 告警
```

### 24.4 残留

- **没有做标注者间一致性 κ**：计划 5.3 要求「κ<0.6 不得当闸门」，当前只支持单标注者；双标与 κ 计算未实现。
- 标注是手工 JSONL，没有标注 UI/校验工作流；`used_fragments` 依赖人读日志。
- 覆盖率/精确率之外：消融、难例、修复类指标仍未实现；真实 DSH 20 轮仍需重启 GUI 后人工跑。

## 25 标注者间一致性 κ（2026-11-03）

**状态**：标注加载支持同一轮多标注者（同 turn 同 annotator 仍拒绝）；新增 `annotation_agreement`（Cohen's κ，基于双标的 `correct`）。B 类指标改为取**字典序第一个标注者**作为主标注，避免多标注重复计数。

### 25.1 实现

| 文件 | 职责 |
|---|---|
| `internal/annotations/annotations.go` | `LoadDir` 返回 `map[turnID][]Record`；同 (turn, annotator) 重复报错 |
| `internal/metrics/compute.go` | `annotationAgreement`：2×2 计数、po/pe、κ=(po−pe)/(1−pe)；`primaryLabel`/`sortedLabels` |
| `internal/metrics/compute_test.go` | κ=0.5 的四轮夹具；无标注时 `annotation_agreement` 也 `not_computable` |
| `internal/annotations/annotations_test.go` | 多标注者允许、同标注者重复拒绝 |

### 25.2 实测（20 轮 journal + 2 轮双标）

```bash
./bin/astrbox-metrics -journal .local/dsh-e2e2/journal -annotations .local/ann-multi -out .local/ann-multi/report.json
# annotated=2
# compensation_coverage   computed 1.0000 (1/1)   ← 主标注者 a
# compensation_precision  computed 1.0000 (1/1)
# annotation_agreement    computed 0.0000 (1/2)   ← 2 轮双标，1 轮一致
make test  # exit 0（node 48 pass）
make lint  # 278 文件，0 违规 0 告警
```

### 25.3 语义与残留

- κ 只算 `correct` 字段（有 `null` 的标注对跳过）；`used_fragments`/`cites` 的一致性未单独算 κ。
- **κ<0.6 不应作为闸门**的规则目前只写在文档，没有在指标输出里自动降级；门禁逻辑未接线。
- 主标注者规则是确定性字典序，不是共识；共识（如多数票）未实现。
- 标注仍为手工 JSONL；无 UI 与双标任务分配。真实 DSH 20 轮仍未跑。

## 26 κ 闸门接线（2026-11-03）

**状态**：`κ<0.6 不得作闸门` 从文档落到代码：一致性低于阈值时，B 类指标自动降为 `not_computable`，数值清零并写明原因。

### 26.1 实现

- `internal/metrics/compute.go`：`MIN_KAPPA = 0.6`；`ComputeWithAnnotations` 先算 agreement，若 `computed && value<MIN_KAPPA`，用 `blockedMetric` 把 coverage/precision 降级（`status=not_computable`、`value=0`、`reason=标注一致性 κ=… < 0.60，不作闸门`）。
- 测试：`TestLowKappaBlocksBClass`（κ=0 → 两者降级且原因含 κ）、`TestHighKappaKeepsBClass`（κ≈0.8 → coverage 仍 computed 1/1）。

### 26.2 实测（20 轮 journal + 2 轮双标，κ=0）

```bash
./bin/astrbox-metrics -journal .local/dsh-e2e2/journal -annotations .local/ann-multi -out .local/ann-multi/report2.json
# compensation_coverage   not_computable  标注一致性 κ=0.00 < 0.60，不作闸门
# compensation_precision  not_computable  标注一致性 κ=0.00 < 0.60，不作闸门
# annotation_agreement    computed 0.0000
make test  # exit 0（node 48 pass）
make lint  # 278 文件，0 违规 0 告警
```

### 26.3 残留

- **只有 κ 可算时才降级**：单标注者（无配对）时 agreement 为 `not_computable`，coverage/precision 仍会 `computed`——这是当前口径（未知 κ 不等于 κ<0.6），作为闸门使用前仍需双标。
- κ 只基于 `correct`；`used_fragments`/`cites` 的一致性未纳入。
- 真实 DSH 20 轮仍未跑；消融/难例/修复类指标未实现。

## 27 单标注者不再构成闸门（2026-11-03）

**状态**：修复第 26 节的缺口——只要存在标注但无法计算 κ（没有双标），B 类指标同样降级为 `not_computable`，原因写"缺少双标（无法计算 κ），不作闸门"。

### 27.1 实现

- `internal/metrics/compute.go`：`switch` 两条降级路径——`κ<MIN_KAPPA`、或"有标注但 agreement 不可算"；`blockedMetric` 仍清零 value。
- `internal/metrics/compute_test.go`：`TestSingleAnnotatorBlocksBClass`（单标注 → coverage/precision 均不可算，reason 含"双标"）；原有高 κ 用例仍验证可算。

### 27.2 实测（单标注 `.local/ann`）

```bash
./bin/astrbox-metrics -journal .local/dsh-e2e2/journal -annotations .local/ann -out .local/ann/report3.json
# compensation_coverage   not_computable  缺少双标（无法计算 κ），不作闸门
# compensation_precision  not_computable  缺少双标（无法计算 κ），不作闸门
# annotation_agreement    not_computable  没有双标的 correct 标签
make test  # exit 0（node 48 pass）
make lint  # 278 文件，0 违规 0 告警
```

### 27.3 语义与残留

- 现在 B 类指标只有在**至少一对双标且 κ≥0.6** 时才 `computed`；单标/无标/低 κ 一律不作闸门。
- κ 仍只算 `correct`；`used_fragments`/`cites` 的一致性未纳入。
- 真实 DSH 20 轮仍未跑；消融/难例/修复类指标未实现。

## 28 C3 候选队列（2026-11-03）

**状态**：`uncovered` 候选不再只返回裁决，而是落盘到 `<dataDir>/candidates.jsonl` 待审；新增 `POST /judge` 与 `GET /candidates`，不阻塞主流程。

### 28.1 实现

| 文件 | 职责 |
|---|---|
| `internal/candidates/candidates.go` | JSONL 追加存储：`pending`/`approved`/`rejected`；`Append` 返回规范化记录（自动 ID/时间/版本），`List(status)` 读回 |
| `internal/control/judge.go` | `POST /judge`：调用只读 `Machine.Judge`；`uncovered` 入队并返回 `candidate_id`，`legal`/`illegal` 不入队；`GET /candidates` 列 pending |
| `internal/control/router.go` | 注册两个端点（K1 附加） |
| `internal/engine/judge.go` | `Verdict` 加 JSON 标签（`status`/`rule_id`/`reason`） |
| 测试 | candidates 追加/校验/缺失文件；control 的 uncovered 入队、legal 不入队、坏输入 400 |

### 28.2 实测（真实 `bin/dsh-mcp` + UDS 控制面）

```bash
curl --unix-socket .local/judge-e2e/dsh-control.sock -X POST http://localhost/judge \
  -d '{"entity_id":"猫猫","path":"状态.信任值","proposed":0.3,"source":"teacher"}'
# {"candidate_id":"cand_...","ok":true,"queued":true,
#  "verdict":{"status":"uncovered","reason":"没有启用规则覆盖该路径"}}
curl --unix-socket .local/judge-e2e/dsh-control.sock http://localhost/candidates
# {"count":1,"items":[{"status":"pending",...}]}
make test  # exit 0（node 48 pass）
make lint  # 282 文件，0 违规 0 告警
```

### 28.3 残留

- 队列只有 `pending` 写入路径：**没有 approve/reject/衰减/回滚端点**，人工处置仍需改文件。
- 没有置信度字段与低置信分流；`source`/`evidence` 只是透传。
- C7 教师信号仍未实现（`/judge` 目前只能由外部手工调用）；真实 DSH 20 轮仍未跑。

## 29 候选处置端点（2026-11-03）

**状态**：新增 `POST /candidates/resolve`，可把候选置为 `approved`/`rejected`；存储保持追加式，读取时按 ID 取最新状态。

### 29.1 实现

| 文件 | 职责 |
|---|---|
| `internal/candidates/candidates.go` | `List` 折叠同 ID 的最新行再按状态过滤；`Resolve(id,status,note)` 追加新状态（未知 ID/非法状态报错） |
| `internal/control/candidateResolve.go` | `POST /candidates/resolve`：`{id,status,note}`；400 非法状态、404 未知候选 |
| `internal/control/router.go` | 注册端点 |
| 测试 | Resolve 折叠/备注/pending 移除、坏输入；control 的 resolve/400/404 |

### 29.2 实测（Go 测试）

```bash
go test ./internal/candidates ./internal/control -run 'Resolve|Candidate|Judge'   # PASS
make test  # exit 0（node 48 pass）
make lint  # 283 文件，0 违规 0 告警
```

### 29.3 残留

- `approved` 只是状态，**没有"应用到状态机"的路径**（approved 候选不会变成规则/状态写入）；回滚与衰减也没有。
- 无置信度字段；`source`/`evidence` 仍只是透传。
- C7 教师信号未实现；真实 DSH 20 轮仍未跑。

## 30 候选队列衰减（2026-11-03）

**状态**：`/nightly` 顺带把超过 TTL 的 pending 候选标记为 `rejected`（原因 `expired`），避免队列无限堆积；记录保持追加式。

### 30.1 实现

| 文件 | 职责 |
|---|---|
| `internal/candidates/candidates.go` | `DEFAULT_TTL = 7 天`；`Expire(cutoff)` 遍历 pending，调用 `Resolve(id, rejected, "expired")` 并返回条数 |
| `internal/control/nightly.go` | 衰减专家后调用 `Expire(now - DEFAULT_TTL)`；失败返回 500；响应新增 `expired` |
| 测试 | candidates 的过期计数/pending 清空/rejected 原因；nightly 原有用例仍通过 |

### 30.2 实测

```bash
go test ./internal/candidates ./internal/control -run 'Expire|Nightly|Resolve'   # PASS
make test  # exit 0（node 48 pass）
make lint  # 283 文件，0 违规 0 告警
```

### 30.3 残留

- TTL 固定 7 天，**没有配置项/命令行参数**；过期只在 nightly 触发（进程内或手工 POST）。
- `approved` 仍没有"应用到状态机"的路径；回滚没有。
- 无置信度字段；C7 教师信号未实现；真实 DSH 20 轮仍未跑。

## 31 P3 夜间离线实现记录（2026-11-03）

**状态**：`cmd/astrbox-nightly` + `internal/triage` + `internal/repair` + `internal/regression` 完成；`make build` / `make test` / `make lint` 全绿。P3 闸门仍未达成（缺真实 DSH 20 轮与人工抽检；参照集为 v0 结构校验）。

### 31.1 新增

| 文件 | 职责 |
|---|---|
| `internal/triage/finding.go` | `Finding` + 三类队列（fact / consistency / preference） |
| `internal/triage/scan.go` | A 类全量（快照 / turn_id / 补偿包 / gate 数）+ B 类标注抽样；判错→fact、未判定→preference |
| `internal/triage/write.go` | `triage.jsonl` 原子落盘 |
| `internal/repair/candidate.go` | 修复候选：consistency→policy、fact→rule、preference/无快照拒收（约束 #9） |
| `internal/repair/versions.go` | 版本化 v1/v2 + hash + 回滚 |
| `internal/regression/gate.go` | 确定性回归闸门：replay 审计 + 参照集结构（空集跳过、不静默通过语义） |
| `cmd/astrbox-nightly/main.go` | 夜间流水线：扫描→三分流→候选→闸门→上线未通过则不应用→衰减→报告；`-dry-run` / `-rollback` |
| `Makefile` | 新增 `bin/astrbox-nightly` |

### 31.2 实测（实际输出）

```bash
make build   # 6 个二进制（含 bin/astrbox-nightly）
go test ./internal/triage ./internal/repair ./internal/regression   # ok
go test -race ./internal/triage ./internal/repair ./internal/regression  # ok

# 干净 20 轮 journal（.local/chat-run 样例）：
./bin/astrbox-nightly -data-dir .local/nightly-smoke -dry-run
# 扫描 20 条，findings=0，replay valid=20 invalid=0，参照集 30 条有效 → exit 0

# 篡改 1 条 compensation：
./bin/astrbox-nightly -data-dir .local/nightly-bad
# findings=1（compensation_invalid, consistency），候选=1，replay invalid=1 → 不应用、exit 1

# 回滚（无版本时如实报错）：
./bin/astrbox-nightly -data-dir .local/nightly-smoke -rollback
```

合规机检：`internal/triage internal/repair internal/regression cmd/astrbox-nightly` 共 10 文件，0 违规 0 告警。

### 31.3 残留

- P3 闸门未跑真实闭环：参照集只做结构校验，未做语义评分（需模型）；真实 DSH 20 轮与人工抽检仍缺。
- 修复补丁是**规则/策略版本描述**，尚无「应用到状态机规则表」的执行器（与 §30.3 的 approved→apply 同一缺口）。
- 无标注目录时 B 类为空，triage 只有 A 类结构 findings。
- `cmd/astrbox-nightly` 未接 DSH 定时器；当前靠 cron/手工调用。

## 32 P4 规划：教师信号与闭环收尾（2026-11-03）

**状态**：规划草案（本步只出规划，不改源码）。依据 §25.3–§31.3 的残留与 `01-contracts-freeze.md` §5「C7 未实现」。

### 32.1 缺口清单（全部来自现有计划的残留）

| # | 缺口 | 来源 |
|---|---|---|
| G1 | C7 教师信号未实现：`/judge` 只能外部手工调用 | 契约 C7、§28.3/§29.3 |
| G2 | 修复补丁无「应用到状态机规则表」执行器 | §30.3、§31.3 |
| G3 | approved 候选无 apply/回滚闭环 | §30.3 |
| G4 | 候选 TTL 固定 7 天，无配置 | §30.3 |
| G5 | nightly 未接定时器 | §31.3 |
| G6 | 真实 DSH 20 轮 + 语义闸门（需环境/模型） | §25–§31 |
| G7 | 消融/难例/修复类指标未实现 | §26.3/§27.3 |

### 32.2 范围与不做

- **做**：G1–G5（确定性、可测、不依赖真实模型）。
- **不做**：G6（需真实 LLM key/环境）、G7（需真实对话样本）、真模型训练（独立 Python/GPU 项目）。

### 32.3 接口契约草案（待冻结）

**C7 教师信号**（沿用 `01-contracts-freeze.md` C7，补传输与语义）：

```go
type TeacherSignal struct {
    SemanticGate         string
    AffectDelta          map[string]float64
    TransitionCandidates []engine.Candidate
    Rebuild              *Rebuild
}
```

- 传输：**建议控制面新增 `POST /teacher-signal`**（不动冻结的 17 个 MCP 工具；与 `/turn-context` 同属 K1 附加端点）。
- 处理：逐候选 `machine.Judge` → `uncovered` 入候选队列（`candidates.Store.Append`）；`illegal` 记诊断 `teacher_signal_rejected`；`legal` 也**不**自动写状态（信号不是真值）。
- 解析失败：记 `teacher_signal_missing`，不阻塞回答。

**apply 执行器**（G2/G3）：

- 目标：把 `approved` 候选变成可回滚的「规则/策略版本」——不做权重训练。
- 建议契约：`POST /candidates/{id}/apply` → 生成 `repair.Candidate`（复用 P3 版本化）→ 版本 vN → 回归闸门 → 通过上线 / 失败回滚。
- 规则侧执行：`engine.AddRule/RemoveRule`（已有），由 apply 执行器调用。

### 32.4 需拍板的决策点

| 决策 | 推荐值 | 推翻条件 |
|---|---|---|
| D-A 传输 | 控制面 `POST /teacher-signal` | 要求走 MCP 工具面 → 需改冻结工具集，升版本 |
| D-B AffectDelta | 候选裁决后再经效果通道应用 | 只要求记录、不要求应用 |
| D-C Rebuild | 定义为「重建补偿包的输入」 | 只要求记录 |
| D-D apply 目标 | 规则/策略版本补丁（与 P3 一致） | 要求直接改 `scripts/*.neko` |

### 32.5 验收（每步一条）

| 步 | 产物 | 验收 |
|---|---|---|
| 1 | `/teacher-signal` 端点 + C7 类型 | 合法信号 → uncovered 入队；illegal → 诊断；坏 JSON → 400 且记 missing |
| 2 | apply 执行器 | approved → 规则版本 vN；回归失败 → 回滚；有测试 |
| 3 | TTL 配置化 | 可配（默认 7 天） |
| 4 | nightly 定时器 | 可配，不阻塞主流程 |
| 5 | 闭环端到端 | teacher-signal → candidate → approved → apply → gate → 上线/回滚 全程有记录 |

### 32.6 下一步

按 D-A 推荐值先实现步 1（教师信号接收 + Judge → 队列），再步 2（apply 执行器）。G6/G7 待真实环境。

### 32.7 实施记录（2026-11-03）

**状态**：步 1–3 完成并通过全量验证；步 4（P3 流水线的进程内定时器）未做，仍由 cron/手工调用 `bin/astrbox-nightly`。apply 目前生成**版本补丁记录**，尚未回写到 `engine` 规则表（D-D 的「规则侧执行」仍待 engine 的 apply API）。

| 文件 | 职责 |
|---|---|
| `internal/control/teacherSignal.go` | `POST /teacher-signal`：C7 信号 → 逐候选 `Judge` → uncovered 入候选队列；illegal 记 `teacher_signal_rejected`；坏 JSON 记 `teacher_signal_missing` |
| `internal/control/repairApply.go` | `POST /candidates/apply`（approved + 快照锚点 → repair 版本 → 回归闸门 → 失败回滚）、`POST /repairs/rollback` |
| `internal/control/router.go` | 新增 `/teacher-signal`、`/candidates/apply`、`/repairs/rollback` 路由 |
| `internal/control/server.go` | `Options.CandidatesTTL` + `-candidates-ttl` 接线 |
| `internal/repair/candidate.go` | 新增 `NewCandidate`（apply 与 triage 共用身份） |
| `internal/control/nightly.go` | TTL 改用 `s.candidatesTTL` |
| `cmd/dsh-mcp/main.go`、`cmd/astrbox-nightly/main.go` | `-candidates-ttl` 旗标 |

**实测**：`make test` 全绿（34 个 Go 包 ok + node 48 pass / 0 fail）；`make lint` 296 文件 0 违规 0 告警；新增 8 条 teacher-signal / apply / rollback 测试全过。

**残留**：① 步 4 定时器未接（cron）；② apply 未回写 engine 规则表；③ 真实 DSH 20 轮 + 语义闸门仍缺环境。

### 32.8 步 4 实施记录：流水线定时器（2026-11-03）

**状态**：P3 流水线抽成可复用包 `internal/nightly`，CLI 变薄壳，`bootstrap` 夜间定时器可选择性调用（`-nightly-pipeline`）。

| 文件 | 职责 |
|---|---|
| `internal/nightly/nightly.go` | `Run(Options) (Report, error)`：扫描→triage→repair→回归→衰减→报告；`SkipDecay` 让进程内定时器先衰减真值注册表、再只跑流水线，避免双重衰减 |
| `cmd/astrbox-nightly/main.go` | 薄壳 CLI（只解析旗标、调用 `nightly.Run`、打印） |
| `internal/bootstrap/nightly.go` | 定时器在进程内衰减后按 `nightlyPipeline` 调用 `nightly.Run` |
| `internal/bootstrap/bootstrap.go` | `Options.NightlyPipeline` + `Runtime.nightlyPipeline` |
| `cmd/dsh-mcp/main.go` | `-nightly-pipeline` 旗标 |

**实测**：`make test` 全绿（35 个 Go 包 ok + node 48 pass / 0 fail）；`make lint` 297 文件 0 违规 0 告警；薄壳 CLI 对 20 轮 journal 仍输出 findings=0 / valid=20。

**残留**：apply 仍未回写 engine 规则表；真实 DSH 20 轮 + 语义闸门仍缺环境。

### 32.9 步 2 补全：apply 回写 engine 规则表（2026-11-03）

**状态**：apply 不再只记录版本，而是把修复落成一条**带条件的可回滚 engine 规则**；重启后由 bootstrap 重新加载。

| 文件 | 职责 |
|---|---|
| `internal/repair/rules.go` | `<dataDir>/repairs/rules.json` 的 `LoadRules` / `UpsertRule` / `RemoveRule`（原子写） |
| `internal/control/repairApply.go` | apply 请求新增 `when` / `match`；**`when` 必填**（避免常开规则）；`Machine.AddRule` + 持久化；闸门失败或手动回滚时 `Machine.RemoveRule` + 删文件 |
| `internal/bootstrap/bootstrap.go` | replay 之后加载 `repairs/rules.json` 并 `AddRules`（修复在重启后仍生效） |

**安全约束**：`when` 必填——apply 产生的规则是带条件的 `set` 补丁，不会成为不过条件的常开写者；`Judge` 仍是只读裁决，覆盖关系不因 apply 而失真。

**实测**：`make test` 全绿（Go 全包 ok + node 48 pass / 0 fail）；`make lint` 299 文件 0 违规 0 告警。新增测试：apply 后 `Machine.Judge` 对目标路径返回 `legal`、rollback 后无残留 `repair` 规则、`rules.json` round-trip。

**残留**：真实 DSH 20 轮 + 语义闸门仍缺环境；规则补丁的**语义**正确性（`when` 是否合适）取决于教师信号质量，本实现只保证机制正确。
