# P2 契约冻结（v1）

- 冻结日期：2026-11-03
- 范围：P2 在线闭环的**接口与数据契约**（C0–C7）；不冻结实现内部。
- 依据：`00-plan.md` 第 3–6 节；P1 已实现部分（hardstate / journal / metrics / referenceset）。
- 决策：D1/D2/D3 采用规划推荐值，**P2 实现开工前可推翻**；推翻需在本文件升版本并写明理由。

## 0 决策记录

| 决策 | 采用值 | 可推翻条件 |
|---|---|---|
| D1 本地 MoE v1 | 规则/检索型专家 + Go 小模型（`算法导论` 的 `cgraph`+`ml` 离线训练，权重 JSON；Neko 只推理） | 要求专家本身是 LLM 级 → 另立 Python/GPU 项目 |
| D2 唯一写者 | 硬状态 = `config.StateGroup` 下的字段；只有 `state_machine` 可写；**P2 先告警不阻断** | 现存 `@函数` 写状态导致告警量过大 → 改为白名单迁移 |
| D3 远程模型 | 模型就是 DSH：桥接插件 `agent/pre-step` 调控制面 `GET /turn-context`，把补偿包注入本轮 user 消息（`/turn-context` 是 K1 的**附加**端点，旧引擎回退 `/state`）；`internal/llm` 的 OpenAI 兼容适配器与 `cmd/neko-chat` 降级为可选回退/本地 dry-run，不承接真实在线路径 | 若要求改 MCP 工具面或改动既有 K1 端点语义 → 需升版本；DSH 插件不加载时 `neko-chat -dry-run` 仍可验链路 |

## 1 写域与所有权（下一批并行实现时使用，互不重叠）

| 包 | 写域 | 所有者（实现期） | 依赖 |
|---|---|---|---|
| `internal/blackboard` | 新目录 | 执行体 A | `hardstate` |
| `internal/expert` | 新目录 | 执行体 B | `hardstate`、`blackboard`、`gate` |
| `internal/compensation` | 新目录 | 执行体 C | `expert`、`hardstate`、`journal` |
| `internal/llm` | 新目录 | 执行体 D | 无（只依赖标准库 + HTTP） |
| `internal/gate` | 已实现 | 冻结，只允许加实现不改接口 | `hardstate` |
| `internal/hardstate` | 已实现 | 冻结；`guard.go` 已加 | 无 |

共享写点：`internal/bootstrap/bootstrap.go`、`internal/mcpserver/*`、`cmd/*`、`Makefile` 由 Lead 统一接线，执行体不得直接改。

## 2 冻结契约

### C0 硬状态（已实现）

- 签名：`hardstate.Schema{Version, Group, Entities}`；`Covers(path) bool`；`CheckWrite(schema, entityID, path, source) WriteCheck`。
- 规则：`CheckWrite` 只放行 `SOURCE_STATE_MACHINE`；未配置 schema 时守卫停用（零值）。
- 验收：`internal/hardstate/guard_test.go`（来源矩阵、组外路径、实体过滤、停用）。
- 变更：字段清单从"状态组 + 实体过滤"升级为显式字段列表时必须升 `SCHEMA_VERSION`。

### C1 快照（已实现）

- 签名：`hardstate.Capture(entityData, schema, IDs{SnapshotID,TurnID,TickID}, now)`；`Snapshot{... State map[string]any}`；`DigestOf(state)`。
- 规则：`State` 键 = `<实体ID>.<点分路径>`；digest = canonical JSON 的 `sha256:`；`SnapshotID = evt_<yyyymmdd>_<seq>`，不复用 `TickID`。
- 验收：`internal/hardstate/snapshot_test.go`、`sequence_test.go`（digest 稳定/敏感、跨日重置、SetLast 恢复）。
- 变更：新增 `Snapshot` 字段必须可选（`omitempty`），否则旧日志不可读。

### C2 黑板（待实现）

```go
// internal/blackboard
type Event struct { ID, Kind, Summary string; Payload map[string]any }
type Constraints struct { Location, Time string; Present []string; KnowledgeBoundary []string }
type Board struct { Event Event; Snapshot hardstate.Snapshot; Constraints Constraints }
type Reader interface { Event() Event; Snapshot() hardstate.Snapshot; Constraints() Constraints }
```
- 规则：容量 1、无历史、序列化 ≤ 8 KiB；写者仅 tick 管线与事件分发器；专家只拿 `Reader`。
- 验收：写两次只留最后一次；专家侧无法写（接口类型限制）；大小断言。

### C3 转移裁决（待实现）

```go
type Candidate struct { EntityID, Path string; Proposed any; Source string; Evidence []string }
type Verdict struct { Status string /* legal|illegal|uncovered */; RuleID, Reason, SnapshotID string }
func (m *StateMachine) Judge(c Candidate) (Verdict, error) // 只读
```
- 规则：`uncovered` 进候选队列（带衰减/回滚）；低置信进人工队列；`Judge` 前后 digest 不变。
- 验收：表驱动 legal/illegal/uncovered；`Judge` 不产生状态写入。

### C4 补偿包（待实现）

```go
type MemoryFragment struct { Content, SourceSnapshotID, Type string; Weight float64 } // retrieved|reconstructed
type Package struct {
  Memory []MemoryFragment; State map[string]any
  StateSource string // == "state_machine_snapshot"
  PersonaConstraints []string; Situation string
  TurnID, SnapshotID string
}
```
- 规则：`State` 键为实体限定路径，值必须与同 `SnapshotID` 快照相等；`reconstructed` 权重低于同源 `retrieved`；`SourceSnapshotID` 必须能在 journal 解析。
- 验收：`validatePackage` fail-closed；不一致包被拒并记诊断。

### C5 专家与门控（门控已实现，专家待实现）

```go
type Input struct { UserInput string; Private PrivateContext; Board blackboard.Reader }
type Expert interface { ID() string; Compensate(ctx context.Context, in Input) ([]MemoryFragment, error) }
type Gate interface { Activate(Features) ([]string, error) } // 2–3 个
```
- 已实现：`gate.Features`、`gate.Gate`、`gate.RuleGate`（`gate-v1`，确定性、2–3 个、规范顺序）；专家 ID 常量与 journal 对齐。
- 规则：专家只写自己的隔离上下文；上下文有界；不得写黑板/硬状态。
- 验收：`internal/gate/ruleGate_test.go`；专家侧上下文上限测试（待实现）。

### C6 TurnRecord（已实现）

- 字段冻结：`record_version`、`turn_id`、`session_id`、`status`、`source`、`started_at`、`ended_at`、`user_input`、`snapshot`、`gate`、`compensation`、`answer`、`answer_model`、`prompt_version`、`cites`、`feedback`、`latency_ms`、`cost`。`source` 区分 `tick`（引擎周期快照）与 `dsh-turn`/`chat`（对话轮），指标与回放跳过 `tick`。
- 规则：只追加、异步写、不持 `tickMu`；P1 记录 `status=incomplete`。
- 验收：`internal/journal/writer_test.go`、`turn_test.go`、`internal/mcpserver/tickJournal_test.go`。
- 变更：新增可选字段必须 `omitempty`；删除/改名必须升 `RECORD_VERSION` 并写迁移说明。

### C7 教师信号（待实现）

```go
type TeacherSignal struct {
  SemanticGate string; AffectDelta map[string]float64
  TransitionCandidates []Candidate; Rebuild *Rebuild
}
```
- 规则：信号不是真值；`AffectDelta` / 候选转移必须经 C3 裁决后才可能成为状态；解析失败记 `teacher_signal_missing`，不阻塞回答。

## 3 变更流程

1. 任何冻结字段的删除/改名/语义变化：先升版本（`SCHEMA_VERSION` / `RECORD_VERSION` / 文档版本），写迁移说明与消费者测试。
2. 新增字段：必须可选（`omitempty`），旧日志/包仍可解析。
3. 契约变更必须有对应测试变更；无测试的契约变更视为未完成。
4. 本文件与 `00-plan.md` 冲突时以本文件为准（本文件更新更晚）。

## 4 未冻结（实现自由）

- 黑板/专家/补偿包/LLM 的内部结构、缓存策略、存储布局。
- gate v2 的模型与特征工程（接口不变即可）。
- `internal/llm` 的 provider 协议细节（接口 `Provider` 冻结，适配器自由）。

## 5 实现进度（2026-11-03）

| 契约 | 状态 | 位置 |
|---|---|---|
| C0 硬状态 | 已实现 | `internal/hardstate/schema.go`、`guard.go` |
| C1 快照 | 已实现 | `internal/hardstate/snapshot.go`、`sequence.go` |
| C2 黑板 | 已实现 | `internal/blackboard/board.go`（容量 1、8 KiB 上限、只读 Reader） |
| C3 转移裁决 | 已实现（只读裁决，未接候选队列） | `internal/engine/judge.go`（legal/illegal/uncovered；不写状态） |
| C4 补偿包 | 已实现 | `internal/compensation/{assemble,validate}.go`（fail-closed） |
| C5 专家/门控 | 门控+规则型专家已实现 | `internal/gate`、`internal/expert`（隔离上下文有界+衰减） |
| C6 交互日志 | 已实现 | `internal/journal`，tick 埋点 `internal/mcpserver/tickJournal.go` |
| C7 教师信号 | 未实现 | 补偿包注入已走 DSH（`/turn-context` + 插件），教师信号仍缺 |

在线 dry-run（不调远程模型）：每次 tick 用黑板 + 门控 + 专家组装补偿包写入 journal，`gate.active` 记录激活专家。真实 20 轮对话与 P2 闸门仍未达成（缺 provider key 与真实对话入口）。
