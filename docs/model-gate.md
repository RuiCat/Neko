# 门控模型（gate-mlp）接入说明

门控（`internal/gate`）决定每轮激活哪 2–3 个专家。v1 是确定性规则门控 `RuleGate`；
训练好的 MLP 通过**同一条 `Gate` 接口**替换规则门控，业务代码零改动。

## 数据流

```text
每轮 Activate(features)
   └─ 规则门控 或 模型门控（二选一）
训练侧（算力/代码在「算法导论」仓库）：
   gate 样本 JSONL ──► training/mlp_jsonl CLI ──► weights JSON（mlp-format v1）
                       （-data {"x":[...],"y":int}，-out weights.json）
部署：把 weights JSON 放到 <data-dir>/models/gate-mlp-v1.json，重启 dsh-mcp。
```

- 权重格式契约：算法导论仓库 `docs/mlp-format.md`（`formatVersion=1`）。
- Neko 侧消费端：`internal/learn/mlp`（加载/校验/前向/softmax，纯推理）。
- 模型文件缺失、维数不符、推理失败 ⇒ **fail-open** 回落 `RuleGate`；fallback 也缺失才报错。

## 特征向量 v1（冻结）

由 `internal/gate/features.go` 的 `FeatureVectorV1` 唯一实现，**训练样本导出与推理共用同一函数**，
所以两侧不会漂移；改特征必须同时升模型版本。

| 槽位 | 含义 |
|---|---|
| 0–8 | 事件类型 one-hot：`relationship, conflict, apology, question, knowledge, emotion, self, event, turn` |
| 9 | 未知/空事件类型 |
| 10 | 摘要非空 |
| 11 | 摘要长度 / 200（封顶 1） |
| 12–13 | `Affect.trust` 是否出现 / 数值 |
| 14–15 | `Affect.valence` 是否出现 / 数值 |
| 16 | `HardState` 是否含「知识边界」类键 |

维数 = `gate.FeatureSizeV1`（17）。模型输出层必须等于 **4**（`gate.SEMANTIC_EXPERTS`：
关系记忆 / 事件记忆 / 自我记忆 / 知识边界）；「表达偏好」不参与分类，由门控固定加入。

## 选专家规则

模型输出 4 类语义专家概率后：

1. 概率最高的语义专家必选；
2. 第 2 名概率 ≥ `gate.MIN_MODEL_PROBABILITY`（0.2）时一并激活；
3. 固定加入「表达偏好」（`gate.PHRASING_EXPERT`）——规则门控也总是给它最低分 1，保证措辞层不缺失；
4. 结果按 `DEFAULT_EXPERT_ORDER` 归位写 journal（每轮 2–3 个专家）。

> 训练标签用「主专家」而不是 active 首位：`active` 按 `DEFAULT_EXPERT_ORDER` 归位，
> 事件记忆会排在知识/自我前面，只看首位会把知识、自我回合也标成事件记忆（实测整批样本单类）。
> 现在 `RuleGate.PrimaryExpert` 取分数最高的语义专家、`ModelGate.PrimaryExpert` 取概率最高的语义专家；
> `RecordingGate` 把它写进样本的 `primary` 字段，导出时 `samples.LabelFor` 优先用它，
> 没有 `primary` 的旧样本退回 active 首位。主专家只在 4 类语义专家里选，
> 「表达偏好」不参与分类；若把它当第 5 类，训练数据永远只有 4 个标签，
> ITA CLI 会推出 4 类模型而门控却期待 5 类（2026-11-09 独立端到端测试实测到这个失败）。

## 可观测性

- `journal.Gate.Version` 通过 `gate.VersionOf(g)` 取门控自报版本：模型门控写 `gate-mlp-v1`，
  规则门控写 `gate-v1`（历史记录口径不变）。
- 启动日志：`astrbox: 门控模型已加载 <path>（层宽 [...]）` / 加载失败时回落提示。

## 样本采集与训练（已接）

- **记录器默认开启**：每轮 `Activate` 成功后，`RecordingGate` 追加一行到
  `<data-dir>/training/gate-samples.jsonl`（`{at,event_kind,summary,affect,hard_state_keys,active,primary}`；
  单文件 64MiB 轮转为 `.1`；写失败只记日志、不阻断回合）。
- **导出训练集**（给算法导论 `training/mlp_jsonl` 用）：

  ```bash
  make build
  ./bin/astrbox-train -data-dir data
  # 写出 data/training/gate-v1.jsonl（每行 {"x":[17 维],"y":专家序号}），并打印主专家标签分布
  ```

- **训练**（算法导论仓库）：

  ```bash
  cd /home/ruicat/Desktop/算法导论/training/mlp_jsonl
  go run . -data /path/to/gate-v1.jsonl -out gate-mlp-v1.json -hidden 16 -act relu -epochs 300 -optimizer adam -holdout 0.2
  ```

- **部署前自检**（输入 17 维 / 输出 4 类语义专家 / 版本 1）：

  ```bash
  ./bin/astrbox-train -data-dir data -check-model data/models/gate-mlp-v1.json
  ```

  通过后把 weights 放到 `<data-dir>/models/gate-mlp-v1.json`，重启 dsh-mcp；启动日志会打印
  `门控模型已加载 …`，journal 里 `Gate.Version` 变为 `gate-mlp-v1`。模型损坏/维数不符时自动回落规则门控。

### 一键训练 + 安全部署

```bash
make train-gate                                          # 默认 data/（要求 ≥50 样本、4 类各 ≥5、最大类 ≤70%、holdout ≥0.8）
bash scripts/tools/trainGateModel.sh --data-dir data --dry-run
bash scripts/tools/trainGateModel.sh --data-dir data --min-samples 200 --min-accuracy 0.85
```

流程：样本检查 → `bin/astrbox-train` 导出 → **样本类别覆盖门槛**（默认 4 类语义专家各 ≥5 条，
`--min-classes` / `--min-class-samples` 可调）→ **类别占比失衡门槛**（默认最大类占比 ≤0.7，
`--max-class-share` 可调）→ 算法导论训练 → `-check-model` 自检 → 留出集准确率门槛
（低于门槛拒绝，`--force` 例外）→ 旧模型备份为 `gate-mlp-v1.json.bak-<UTC>` → 安装新权重。
过程产物（训练集、权重、日志）落 `.local/train-gate/<UTC>/`。回滚：把 `.bak-<UTC>` 覆盖回
`data/models/gate-mlp-v1.json` 后重启。

#### 特征来源（实时信号 → v1 特征）

门控特征由 `internal/gate/derive.go` 的 `DeriveFeatures` 在装配器
（`compensation.AssembleWithActive`）里统一补全——所有入口都经过这一处，
**特征布局仍是冻结的 v1（17 维）**，只改输入值：

| 特征 | 来源 | 规则 |
|---|---|---|
| `EventKind` | 显式提示 → 用户文本 → 强情绪状态 → `turn` | 文本关键词映射到 9 类 one-hot；tick 记录按本轮状态转移路径映射（冲突→`conflict`、`关系.*`→`relationship`、`情绪`/`心情`→`emotion`、`睡眠`/`身体`/`疼痛`→`self`） |
| `Summary` | 调用方摘要，缺失时取用户文本 | 截断 120 字（与 `chat.summarize` 一致） |
| `Affect.trust` | 单实体快照 `关系.信赖`（无则 `关系.亲密`） | 0–100 归一到 0–1；多实体快照不猜，避免张冠李戴 |
| `Affect.valence` | 单实体快照 `状态.情绪` / `心情.当前` | 情绪词典映射到 [-1,1]（强 ±0.8 / 温和 ±0.4 / 中性 0；否定翻转） |
| `HardState` 知识边界 | 场景约束 `KnowledgeBoundary` 或快照含「知识边界」键 | 置键即点亮 v1 第 17 维 |

用户文本来源：DSH 每轮 turn-end 的 `summary.userDigest`（`POST /session-event`），经
`control.buildTurnContext` 传给装配器；预注入 `/turn-context` 没有用户文本，按快照情绪派生；
tick 记录按状态转移派生；`neko-chat` 路径本来就传用户输入。

**行为变化（预期）**：规则门控的激活专家会随事件类型变化（问句→知识边界、亲密话→关系记忆、
强情绪→自我记忆），预注入给猫猫的补偿包同样随之变化；journal 的 `gate.active` 可对照。
回退只需撤掉装配器里那一处 `DeriveFeatures` 调用，无数据迁移、无契约变更。

#### 为什么单靠准确率门槛不够

样本标签来自线上每轮真实的主专家（规则门控取分数最高、模型取概率最高；`samples.LabelFor`），
所以数据分布取决于实时特征。
在特征来源接通之前，线上事件类型恒为 `turn`，自然积累的样本全是「事件记忆」：

- 单类数据训练出的模型会退化成只选一个专家，而准确率门槛仍可能是 1.0（全猜同一类也对）：
  实测 46 条单类样本训练 400 轮，训练集与留出集准确率都是 1.0000。
- `-check-model` 只有在输出层 ≠ 4 时才能拦下（该例标签最大值 = 1 → 输出 2 类，被拒）；
  若单类样本恰好是「知识边界」（标签 3），输出层仍是 4 类，自检与准确率门槛都会通过。
- 所以部署前加两道门槛（见「一键训练 + 安全部署」）：**类别覆盖**（4 类语义专家都要有足够样本）
  与**类别占比**（默认最大类占比 ≤0.7，避免多数类单独撑起整体准确率）。

### 智慧体主动训练（MCP 工具）

引擎把训练能力直接暴露给智慧体（agent），训练数据由智慧体自己提供：

| 工具 | 作用 |
|---|---|
| `gate_status` | 自动/教师样本数、可导出样本、主专家标签分布、覆盖门槛是否满足、模型是否已安装、当前门控版本 |
| `gate_teach` | 提交教师样本（一次 1–200 条）：每条 = 输入信号（`event_kind`/`summary`/`affect`/`hard_state_keys`）+ 目标语义专家 → `<data>/training/gate-teacher.jsonl` |
| `gate_train` | 跑一键训练：样本门槛 → 类别覆盖门槛 → 训练 → `-check-model` 自检 → 准确率门槛；**默认 dry-run**，`deploy=true` 才安装权重（重启 dsh-mcp 生效） |

`gate_teach` 请求示例：

```json
{"samples": [
  {"target_expert": "知识边界", "event_kind": "question", "summary": "猫猫的设定是什么？", "hard_state_keys": ["知识边界"]},
  {"target_expert": "关系记忆", "event_kind": "relationship", "summary": "我想你了", "affect": {"trust": 0.4}}
]}
```

- 教师样本与自动记录样本**分文件存**，导出训练集时合并（`bin/astrbox-train` 同时读 `gate-samples.jsonl` 与 `gate-teacher.jsonl`），
  教师记录带 `source=teacher`、`primary=<目标专家>`，可追溯。
- `gate_teach` 任一条非法（未知专家 / 未知事件类型 / 超过 200 条）**整批拒绝**，不落半批。
- `gate_train` 复用 `scripts/tools/trainGateModel.sh`，通过 `NEKO_ROOT` 或 `bin/dsh-mcp` 的父目录定位仓库根；
  需要本机 `go` 与算法导论仓库（`ita_dir` 或 `$ITA_DIR`）。
- 部署语义：`deploy=false` 只训练+校验并返回输出尾部；`deploy=true` 过全部门槛才安装，权重仍需重启 `dsh-mcp` 生效。
- 注意：**新增工具本身也要重启 dsh-mcp** 后才会出现在 agent 的工具列表里。

## 独立端到端验证

```bash
make build
make verify-training                                  # 默认用 /home/ruicat/Desktop/算法导论
bash scripts/tools/verifyTrainingPipeline.sh --ita-dir /path/to/ita --samples 200
```

覆盖整条链路（不复用单元测试）：生成带标签样本 → `bin/astrbox-train` 导出 → 算法导论
`training/mlp_jsonl` 真训 → `-check-model` 自检 → 启动 `bin/dsh-mcp` 断言
`gate.version=gate-mlp-v1` 且样本记录器 +1 → 移走模型后断言回落 `gate-v1`。
过程产物落 `.local/e2e/work`。

## 现状

- 已接：特征向量 v1、`ModelGate`（fail-open）、bootstrap 模型加载、journal 版本口径、
  样本记录器（默认开启）、实时特征派生（`gate.DeriveFeatures`：事件类型 / affect / 知识边界）、
  `bin/astrbox-train` 导出/自检（自动样本 + 教师样本合并）、单测（gate + samples + mlp + derive）、
  一键训练/部署工具（`make train-gate`，含样本类别覆盖门槛）、
  智慧体主动训练工具（`gate_status` / `gate_teach` / `gate_train`，`gate_train` 默认 dry-run）。
- 待重启：运行中的 `dsh-mcp` 还是旧二进制，重启后新样本才带丰富特征；
  重启前积累的样本仍是单类，训练门槛会如实拒绝。
