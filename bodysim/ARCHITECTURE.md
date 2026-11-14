# 人体生理 / 代谢模拟系统 — 架构设计

> 独立设计文档。与宿主工程（Neko/AstrBox）约定解耦：本目录只放设计，不依赖、不修改任何既有代码。
> 状态：v0.1 草案。所有医学数值为游戏近似量级，落库前须按你选的资料来源校准。

---

## 0. 文档定位

| 项 | 内容 |
|---|---|
| 解决什么 | 一个能同时支撑 **代谢 / 性行为 / 损伤 / 怀孕 / 非常规路径器官损害** 的生理内核 |
| 不解决什么 | 具体内容呈现、渲染、剧情、数值平衡定稿 |
| 受众 | 引擎开发者（实现）、策划（调参）、内容分级审查（开关边界） |
| 前置讨论 | 数据分层、器官账本、屏障/破口模型、路径图模型（见对话历史结论） |

---

## 1. 目标、非目标与约束

### 1.1 目标

1. **一套内核覆盖四种玩法需求**：日常代谢、性行为结算、外伤/损伤、怀孕全过程；非常规路径只是路径图的另一条边，不是新系统。
2. **数据驱动**：新增器官、路径、激素、行为 = 改数据包，不改内核代码。
3. **可确定性回放**：同一初始状态 + 同一输入序列 ⇒ 逐字段一致的结果；支持录像、回归、平衡性测试。
4. **可分级**：内核只产出结构化事实（`breach`/`contamination`/`ischemia`），呈现层按分级决定"写不写、写多细"。
5. **可演进**：先跑通最小闭环，再逐层加病理；每层有独立验收。

### 1.2 非目标

- 不做医学级精度（只要求"量级正确、方向正确、因果链正确"）。
- 不做实时物理（碰撞、流体仿真交给表现层，内核只吃归一化后的 `severity input`）。
- 不做完整 HPG 轴微分方程（用相位 + 脉冲 + 一阶滞后替代）。

### 1.3 硬约束

| 约束 | 判据 |
|---|---|
| 单源码文件 < 500 有效代码行，一个文件一个功能 | 沿用 `code-modularity` 技能判据 |
| 命名 camelCase / PascalCase / UPPER_SNAKE | 同上，禁止 snake_case、kebab-case |
| 内核无 IO、无全局状态 | `kernel` 包不得 import 文件/网络/时钟真实实现 |
| 所有数值常数外置 | 内核源码里不出现"魔法数字"，一律来自 `content/` |
| 每 tick 每字段只有一个写者 | 写权表 + debug 模式断言 |

---

## 2. 设计原则

1. **分层不解耦**：分层的目的是"每层只回答一类问题"，不是"层间不通信"。允许单向依赖：`adapter → sim → kernel/domain → content`。
2. **派生量不存储**：休克分级、昏迷指数、可行动性等全部由原始量算出，避免双源不同步。只有"难以重算且昂贵"的量才落盘。
3. **事件驱动 + 固定 tick 混合**：连续量按 tick 积分；离散事件（穿透、分娩、出血加速）进优先队列，在 tick 边界按序结算。
4. **因果序不可逆**：管线阶段是 DAG，禁止回边；需要回写的用"下一 tick 事件"实现。
5. **内容可插拔**：`ContentPack` 带版本号 + 迁移函数；旧存档能加载新内容包。
6. **事实与呈现分离**：内核输出 `Fact`（结构化、可序列化），表现层决定文案/美术/分级。

---

## 3. 总体架构

```
┌─────────────────────────────────────────────────────────────────────┐
│                        表现 / 交互层（可变）                          │
│   UI、文案、分级滤镜、动画、语音                                        │
└───────────────▲─────────────────────────────────────────────────────┘
                │ Fact[]（结构化事实）
┌───────────────┴─────────────────────────────────────────────────────┐
│                        适配层 adapters/                              │
│   EngineAdapter（角色映射）  PresentationAdapter（分级）  Serializer   │
└───────────────▲─────────────────────────────────────────────────────┘
                │ BodyState / Event
┌───────────────┴─────────────────────────────────────────────────────┐
│                        编排层 sim/                                   │
│   World、调度器（LOD）、事件队列、多角色批次                            │
└───────────────▲─────────────────────────────────────────────────────┘
                │ StepContext
┌───────────────┴─────────────────────────────────────────────────────┐
│                        领域层 domain/                                │
│  metabolism · fluids · circulation · respiration · thermo            │
│  endocrine · reproductive · injury · pathology                       │
└───────────────▲─────────────────────────────────────────────────────┘
                │ 只读 ContentPack
┌───────────────┴─────────────────────────────────────────────────────┐
│                        内核层 kernel/                                │
│  State · Pipeline · Clock · Rng · EventBus · Snapshot                │
└───────────────▲─────────────────────────────────────────────────────┘
                │ 加载
┌───────────────┴─────────────────────────────────────────────────────┐
│                        内容层 content/                               │
│  organs · barriers · cavities · routes · tissues · hormones ·        │
│  events · tuning（YAML + JSON Schema）                                │
└─────────────────────────────────────────────────────────────────────┘
```

**依赖方向只有一条**：上层依赖下层，下层不知道上层存在。

---

## 4. 目录与包结构

```
bodysim/
├── kernel/                 # 与领域无关的机械层
│   ├── state/              # BodyState 容器、字段注册、脏标记
│   ├── pipeline/           # Phase 接口、拓扑排序、写权校验（debug）
│   ├── clock/              # 固定步长 + 余量累加
│   ├── rng/                # 分流域随机（可注入种子）
│   ├── events/             # 优先队列、事件类型注册
│   └── snapshot/           # 序列化、版本、迁移
├── domain/
│   ├── anthropometry/      # 体格、体成分、体表面积、器官质量缩放
│   ├── metabolism/         # BMR/TEF/活动、底物、能量平衡
│   ├── fluids/             # 体液、电解质、渗透压、口渴/尿量
│   ├── circulation/        # 血容量、血红蛋白、心率血压、灌注分配
│   ├── respiration/        # 氧合、通气、窒息倒计时
│   ├── thermoregulation/   # 产热散热、体温、出汗
│   ├── endocrine/          # 激素相位与脉冲
│   ├── reproductive/       # 月经、受孕、怀孕状态机、分娩
│   ├── injury/             # 屏障、破口、出血、污染、缺血、修复
│   ├── pathology/          # 脓毒症、休克、MODS、后遗症账本
│   └── routes/             # 路径图解释器（口/肛/阴道/尿道/创口）
├── content/                # 数据包（不进内核）
│   ├── pack/               # 版本化内容包
│   └── schema/             # JSON Schema
├── sim/                    # 编排：World、LOD 调度、场景脚本
├── adapters/               # 引擎/呈现/存档适配
└── tests/
    ├── golden/             # 剧本化回放（输入 → 期望区间）
    ├── property/           # 不变量性质测试
    └── fuzz/               # 事件序列模糊测试
```

> 约定：`domain/*` 每个包只依赖 `kernel` 与 `content` 的只读接口，**领域包之间不互相 import**，跨领域的数据交换一律经 `StepContext` 与事件总线。这是防止 20 个包织成网的关键。

---

## 5. 核心数据模型

### 5.1 三层拆分

```text
Body = Statics（不变/极少变） + State（每 tick 变） + Ledger（事件累积）
```

| 层 | 内容 | 变化 |
|---|---|---|
| `Statics` | 身份、体格、器官基准质量、生殖配置、体质系数 | 创建时/长期变化 |
| `State` | 能量、体液、循环、呼吸、体温、激素 | 每 tick 积分 |
| `Ledger` | 损伤、破口、移位、怀孕、后遗症、既往史 | 事件驱动 |

### 5.2 Statics

```yaml
statics:
  id: "chr_01"
  physiologicalSex: female        # 与"社会性别/表现性别"分离
  reproductiveConfig:
    hasOvaries: true
    hasUterus: true
    hasTestes: false
    sterile: false
  ageYears: 24
  heightCm: 162
  weightKg: 54
  bodyFatPct: 0.24
  bodySurfaceAreaM2: 1.55
  organMassTable: liver=1400g, heart=260g, kidney=280g, brain=1350g, ...
  traits:
    metabolicEfficiency: 1.0
    painTolerance: 0.5
    bleedingTendency: 1.0
    immuneStrength: 1.0
  history:
    parity: 0
    priorCesarean: false
    scarCount: 0
    priorInjuries: []
```

### 5.3 State（按域分组，字段清单见对话前文，此处给结构）

```yaml
state:
  metabolism: { bmrKcalD, glycogenLiverG, glycogenMuscleG, bloodGlucoseMmolL,
                fatMassKg, leanMassKg, energyBalanceKcalD }
  fluids:     { totalBodyWaterL, extracellularL, plasmaL, bloodVolumeMl,
                sodiumMmolL, potassiumMmolL, osmolality, thirst, urineMlMin }
  circulation:{ heartRateBpm, strokeVolumeMl, cardiacOutputLMin,
                systolicMmHg, diastolicMmHg, perfusion: { liverMlMin, kidneyMlMin, ... } }
  respiration:{ respiratoryRate, tidalVolumeMl, spo2, paco2, vo2MlMin }
  thermo:     { coreTempC, skinTempC, sweatRateMlMin, heatProductionW, heatLossW }
  endocrine:  { estrogen, progesterone, testosterone, prolactin, cortisol,
                oxytocin, gonadotropin, cyclePhase, cycleDay }
  neuro:      { consciousness, pain, fatigue, nausea, dizziness }
```

### 5.4 账本实体

```yaml
organ:
  id: liver
  massG: 1400
  perfusionMlMin: 1350
  functionFraction: 1.0
  integrity: 100
  inflammation: 0.0
  healingRate: 0.05
  vulnerability: { mechanical: 0.2, ischemic: 0.8, infectious: 0.4 }
  removed: false

barrier:
  id: uterineWallLower
  between: [uterineCavity, peritonealCavity]
  thicknessMm: 8
  tensileStrength: 1.0
  perforationThreshold: 0.8
  lacerationThreshold: 0.3
  isScar: false
  hasSerosa: true
  integrity: 100
  healingRate: 0.02

cavity:
  id: peritonealCavity
  sterile: true
  contents: []
  pressureMmHg: 0

route:
  id: anal
  chain: [analCanal, rectum, rectosigmoid]
  branchPoints:
    - at: rectum
      options: [peritonealCavity, retroperitonealSpace, rectovaginalSeptum]
      select: locationBased
  floraProfile: colonic
  timeCritical: { peritoneal: 6h, ischemia: 4h }

injury:
  id: inj_001
  targetOrgan: rectum
  mechanism: mechanical
  severityInput: { force: 0.8, areaCm2: 3.0, durationS: 120,
                   lubrication: 0.2, foreignBody: false, repeated: 1 }
  outcome: { tissueDamage: 42, bleedingMlMin: 35,
             infectionRisk: 0.6, perforationRisk: 0.15 }
  createdTick: 12043
  status: active        # active / healing / healed / chronic

breach:
  id: br_001
  barrier: uterineWallLower
  diameterCm: 3.5
  communicating: [uterineCavity, peritonealCavity]
  hemorrhageMlMin: 180
  contamination: 0.9
  containment: communicating   # intact / communicating / containedHernia / eviscerated
  createdTick: 12043

displacement:
  organ: uterus
  referencePlane: introitus
  offsetCm: 4.0
  grade: IV
  reducible: false
  strangulated: false
  ischemiaMinRemaining: 240

pregnancy:
  state: secondTrimester      # 见 §8.2
  gestationalWeek: 22
  placentaPosition: posterior
  membraneIntact: true
  cervicalDilationCm: 0
  bloodVolumeMultiplier: 1.4

condition:                    # 派生 + 累积的长期状态
  id: chronicPelvicPain
  severity: 0.4
  source: inj_001
  onsetTick: 12100
```

### 5.5 派生量（不存储，每 tick 重算）

`shockClass`、`consciousness`（由 SpO₂/灌注/失血/疼痛合成）、`actionability`、`fertilityWindow`、`incontinenceRisk`、`sepsisScore`。

**规则**：凡是能由原始量算出的，一律不写进 state。debug 模式下提供 `derive()` 全量重算并与缓存比对，发现漂移即报错。

---

## 6. 结算管线

### 6.1 Phase 契约

```text
interface Phase {
    name: string
    dependsOn: string[]        // 拓扑序，决定执行顺序
    writes: FieldPath[]        // 写权声明；debug 下越权写 => panic
    apply(ctx: StepContext): void
}
```

- 管线在启动时按 `dependsOn` 做**拓扑排序**，启动即报环。
- `writes` 声明用于 **debug 写权校验**：同一 tick 内两个 Phase 写同一字段即报错。
- Phase 只能通过 `ctx.emit(event)` 影响后续阶段；**不能直接写别人的字段**。

### 6.2 阶段表

| # | Phase | 读 | 写 | 可否 emit |
|---|---|---|---|---|
| 1 | `intake` | 外部输入事件 | `intakeBuffer` | 是 |
| 2 | `digestion` | intakeBuffer、胃肠状态 | `gut`、`absorbedNutrients` | 是 |
| 3 | `energy` | absorbedNutrients、活动、organ 功能 | 底物储备、`heatProduction` | 是（低血糖） |
| 4 | `fluids` | intake、损耗、肾 | TBW、电解质、渗透压 | 是（脱水） |
| 5 | `circulation` | 血容量、Hb、organ 功能 | HR、BP、CO、灌注分配 | 是（休克） |
| 6 | `respiration` | 氧需求、肺功能、气道 | SpO₂、PaCO₂、RR | 是（窒息） |
| 7 | `thermoregulation` | 产热、环境 | 核心温、出汗 | 是（热射病） |
| 8 | `organFunction` | 灌注、integrity | 各器官 `functionFraction` | 是（器官衰竭） |
| 9 | `injury` | 事件、integrity、免疫 | integrity、breach、inflammation | 是（破裂/感染） |
| 10 | `endocrine` | 状态、应激、昼夜 | 激素 | 是（排卵） |
| 11 | `reproductive` | 激素、怀孕状态 | 周期、孕周、分娩 | 是（临产） |
| 12 | `derived` | 以上全部 | 派生量（不落盘） | 否 |

**允许的 emit 边（防止隐式环）**：

```
intake → digestion → energy → fluids → circulation → respiration → thermo
                                              ↓
                                       organFunction
                                              ↓
                                          injury
                                              ↓
                                        endocrine → reproductive
```

- 允许**前向** emit（下游能读上游本 tick 产生的事件）；
- 禁止**后向** emit（下游想影响上游 ⇒ 必须投递到 `nextTick` 队列）；
- `derived` 只读。

### 6.3 一次 tick 的时序

```text
tick N:
  clock.advance(dt)
  1. drain(eventQueue, dueTick == N) → 按 (priority, seq) 排序
  2. for phase in topoOrder: phase.apply(ctx)
       - 可读 ctx.state（本 tick 已更新的部分）
       - 可 emit 到 sameTick（仅前向）或 nextTick
  3. snapshotCondensed()（可选，供 UI/存档）
  4. 若跨存档点：snapshot.save()
```

---

## 7. 事件系统

### 7.1 事件类型

| 类型 | 产生者 | 例 |
|---|---|---|
| `InputEvent` | 适配层 | 进食、饮水、行为开始/结束、用药 |
| `TraumaEvent` | 适配层/领域 | 外力、穿透、异物 |
| `StateEvent` | 领域 | 低血糖、休克、感染、排卵、临产 |
| `TimerEvent` | 事件总线 | 缺血倒计时、脓毒症潜伏期、愈合完成 |
| `LifeEvent` | 领域 | 受孕、分娩、死亡 |

### 7.2 队列与确定性

```text
priorityQueue orderedBy (dueTick, priority, seq)
seq 单调递增，由事件总线分配 ⇒ 同 tick 内顺序确定
```

- 不允许 `map` 遍历决定顺序；所有遍历对象按 id 排序。
- RNG 按 `(streamId, tick)` 分流域：`rng.stream("injury").at(tick)`，避免"多消费一次随机数导致全盘偏移"。

---

## 8. 状态机

### 8.1 急症状态机（与损伤/穿透强相关）

三条**并行轨道**：同一个 `breach` 同时启动三条，互不等待；治疗只关闭对应那条。

```text
轨道 A 出血
  bleeding ──失血15%──► compensated ──失血30%──► shock ──► MODS ──► death

轨道 B 污染
  contaminated ──潜伏(数小时~1天)──► systemicInflammation ──► sepsis ──► MODS

轨道 C 缺血
  ischemic ──超过组织耐受──► necrosis ──► 切除 / 永久失能

取消条件：A 需止血，B 需清创+抗感染，C 需再灌注；任一轨道进入 MODS ⇒ 全局进入危急态。
```

- 每个节点持有 **deadlineTick**；`derived` 阶段只读地计算"剩余窗口"。
- 节点转移由 `TimerEvent` + 治疗 `InputEvent` 共同驱动。
- 转移条件全部是可观察量（失血百分比、体温、感染分），不允许纯随机转移。

### 8.2 怀孕状态机

```text
unpregnant ─受孕判定→ firstTrimester → secondTrimester → thirdTrimester
     → labor(singleton|preterm) → postpartum → lactating → unpregnant
  任一节点 → miscarriage / ectopic / preeclampsia / abruption / rupture
```

- `gestationalWeek` 是总线变量，驱动所有下游（血容量、能量需求、子宫壁厚度）。
- 并发症是**旁路节点**，可回到主链或终止。

### 8.3 性反应 / 月经周期

- 性反应：`arousal → plateau → orgasm → refractory`，每态有激素脉冲与心血管系数。
- 月经周期：相位变量 `cyclePhase ∈ [0,1)` + 4 个相位事件；受孕窗口由相位派生。

---

## 9. 路径图与损伤结算（系统核心算法）

### 9.1 图模型

- **节点** = 体腔 / 解剖段（口腔、咽、食管、胃、肛管、直肠、直乙交界、阴道、宫腔、腹膜腔、腹膜后、纵隔、气道……）
- **边** = 屏障（有厚度、抗张强度、阈值、是否有浆膜）
- **一次穿透** = 在图上从入口节点走一条路径，每跨一条边做一次阈值判定

```text
口：exterior → oralCavity → oropharynx ⇉ larynx→airway | hypopharynx→esophagus→stomach
肛：exterior → analCanal → rectum ⇉ peritonealCavity | retroperitonealSpace | rectovaginalSeptum→vagina
阴道：exterior → vagina → cervix → uterineCavity
```

### 9.2 结算算法

```text
resolveRoute(body, routeId, vec, opts) -> Outcome:
    node = route.entry
    outcome = Outcome{}

    for edge in route.chain:
        if vec.depth < edge.startDepth: break
        barrier = content.barrier(edge.id)

        stress = vec.force * vec.areaFactor
               * (1 + barrier.vulnerability[vec.mechanism])
               * (1 + barrier.priorDamage) * pregnancyThinning(barrier, body)

        if stress <= barrier.lacerationThreshold:
            outcome.add(laceration(barrier, stress))
            continue

        if stress <= barrier.perforationThreshold:
            barrier.integrity -= lacerationDamage(stress)
            outcome.add(bleedingEstimate(barrier, stress))
            continue

        breach = createBreach(barrier, stress)
        outcome.add(breach)
        schedule(Hemorrhage(breach), rate = vesselFlow(barrier) * stress)
        schedule(Contamination(breach), degree = flora(route) * breachAge)

        if breach.reachesExterior and transferable(breach):
            outcome.containment = eviscerated
            schedule(Ischemia(contents), tolerance = tissueTolerance)

        node = breach.nextNode
        if node.branchPoint: node = branch(node, vec, body)

    return outcome
```

### 9.3 严重度分层（对策划友好）

不追求精确数值，先给**三档强制分层**，避免"全随机"：

| 档 | 判据 | 结果 |
|---|---|---|
| 轻 | 只到 laceration | 出血少量、疼痛、自愈 |
| 中 | 破口但未污染关键腔 | 感染风险、需治疗、可后遗症 |
| 重 | 破口 + 污染 + 移位/外露 | 急症倒计时、可能死亡/切除 |
| 极重 | 多腔连通 + 大血管 | 黄金窗口极短、结局不可逆 |

### 9.4 与非常规路径的关系

**非常规路径不是新系统**：口/肛/阴道/尿道/创口都只是 `route` 数据。新增一种路径 = 在 `content/routes/*.yaml` 加一条链 + 一个菌群配置 + 一个分叉选择函数。

---

## 10. 接口契约

以下以 Go 语法示意，语义与语言无关。

```go
// kernel
type Sim interface {
    Step(dt Duration) ([]Fact, error)
    Offer(ev Event) error
    Snapshot() (Snapshot, error)
    Restore(Snapshot) error
}

type Phase interface {
    Name() string
    DependsOn() []string
    Writes() []FieldPath
    Apply(ctx *StepContext) error
}

// content
type ContentPack interface {
    Version() string
    Organ(id string) (OrganSpec, bool)
    Barrier(id string) (BarrierSpec, bool)
    Route(id string) (RouteSpec, bool)
    Validate() []ValidationError
}

// adapters
type EngineAdapter interface {
    ApplyFacts(bodyID string, facts []Fact) error
    ReadVisualState(bodyID string) (VisualState, error)
}

type PresentationAdapter interface {
    Render(fact Fact) (Optional[RenderedText], error)  // 分级过滤器在这里
}
```

**契约纪律**：`kernel` 不得 import `domain`；`domain` 不得 import `sim`/`adapters`；跨域交换只经 `StepContext`。

---

## 11. 内容包与版本迁移

```text
content/pack/
  manifest.yaml        # packVersion, schemaVersion, dependencies
  organs.yaml
  barriers.yaml
  cavities.yaml
  routes/{oral,anal,vaginal,urethral,wound}.yaml
  tissues.yaml         # 各组织灌注/愈合/易感
  hormones.yaml
  tuning.yaml          # 全局调参（系数、阈值）
  schema/*.json        # 与上面一一对应的 JSON Schema
```

- 加载即校验（Schema + 引用完整性 + 阈值单调性）。
- `packVersion` 进存档；加载旧存档时按 `migrations/` 逐版本升级。
- 开发期支持热重载（只重建内容，不重建状态）。

---

## 12. 确定性、回放与测试

| 手段 | 做法 |
|---|---|
| 固定步长 | `accumulator += realDt; while accumulator >= fixedDt: step(fixedDt)`；余量保留 |
| 分流域 RNG | `rng.at(stream, tick, seq)`，禁止共享全局流 |
| 遍历有序 | 所有 map 先转 `[]` 再按 id 排序 |
| 快照 | 全量 + 压缩；存档点每 N tick |
| 回放 | 记录 `(seed, contentPackVersion, inputEvents[])`，重放断言逐字段一致 |
| golden | 10+ 剧本（低血糖、失血、脓毒症、早产、穿透、脱出……），断言**区间**而非精确值 |
| property | 不变量：体积非负、能量守恒误差 < ε、integrity ∈ [0,100]、伤口愈合单调、无事件丢失 |
| fuzz | 随机事件序列 × 1000 轮，断言不崩、不出现 NaN、不死锁 |

---

## 13. 性能与 LOD

| 层级 | 步长 | 适用 |
|---|---|---|
| fine | 1 游戏分钟 | 在场角色、急症中角色 |
| coarse | 1 游戏小时 | 同场景但不互动 |
| daily | 1 游戏日 | 远处/后台角色 |

- 调度器按"是否在场 + 是否有活跃急症"动态升降级。
- 升级时用**积分近似**补齐：`daily → fine` 不重算历史，只从当前状态继续。
- 目标：单机 200 个后台角色 + 10 个精细角色，单 tick < 2 ms（需实测校准）。

---

## 14. 内容分级边界（架构内建）

```yaml
contentPolicy:
  allowExplicit: false
  goreLevel: "abstract"     # abstract / moderate / detailed
  enableUnconventionalRoutes: false
  enablePregnancy: false
  enablePermanentInjury: false
```

- 内核**永远**产出同一套结构事实；`PresentationAdapter` 按策略过滤。
- 灰度：`enableUnconventionalRoutes` 关闭时，路径图对应边不加载 ⇒ 行为直接返回 `refused`，不产生任何结算。
- 这套开关必须**在内容包里**而非代码里，便于发行地区差异化。

---

## 15. 实施路线（每阶段可独立验收）

| 阶段 | 内容 | 验收判据 |
|---|---|---|
| **M0** 骨架 | kernel + pipeline + content 加载 + snapshot | 空 Body 跑 1000 tick 无异常；存档往返一致 |
| **M1** 代谢 | 能量/体液/体温/循环/呼吸 | 剧本"禁食 3 天"体重下降落在合理区间；"脱水"触发口渴与尿量下降 |
| **M2** 损伤 | barrier/breach/出血/污染/修复 | 剧本"钝击肝脏"失血曲线与休克分级正确；伤口 N 天愈合 |
| **M3** 生殖 | 月经/受孕/怀孕/分娩 | 剧本"受孕→40 周→分娩"状态机无死锁；能量需求曲线正确 |
| **M4** 路径 | 口/肛/阴道路径图 + 分叉 | 剧本"口→误入气道"触发窒息；"肛→直乙交界破裂"触发腹膜炎 |
| **M5** 病理 | 脓毒症/MODS/后遗症账本 | 剧本"破口未处理"在窗口后进入休克→MODS→死亡 |
| **M6** 调参 | 平衡性、LOD、分级 | 200 角色性能达标；内容包切换不重启 |

---

## 16. 风险与取舍

| 风险 | 表现 | 缓解 |
|---|---|---|
| 参数爆炸 | 300+ 可调参数，无法平衡 | 分层调参：先调"档位阈值"，再调系数；提供敏感度分析脚本 |
| 过度建模 | 写了没人玩的细节 | 每加一个系统必须绑定一个玩法判据（"玩家能观察到什么"） |
| 双源不同步 | state 与派生量冲突 | 派生量一律现算 + debug 校验 |
| 确定性破坏 | 回放不一致 | 排序遍历、分流域 RNG、禁止时间/随机直读 |
| 存档迁移 | 加字段炸旧档 | 内容包版本 + migration 链，M0 就建 |
| 内容合规 | 分级差异 | 策略进内容包，呈现层过滤，内核只出事实 |

**明确的取舍**（先写死，避免反复）：

1. 激素用相位 + 脉冲，不写微分方程。
2. 灌注分配用固定比例表 + 应激重分配，不做流体求解。
3. 愈合用线性/指数衰减，不做细胞级。
4. 多角色交互只做"事件级"（一次行为产生一次结算），不做物理耦合。

---

## 17. 假设与待定决策

> 这些是当前设计的**假设**；每一条都有推翻条件，不成立时要回来改结构。

| # | 假设 | 若不成立 ⇒ 影响 |
|---|---|---|
| A1 | 目标平台支持**固定步长 + 存档** | 若纯实时无存档 ⇒ 去掉 snapshot/migration，M0 简化 |
| A2 | 单角色为主，多角色交互是事件级 | 若需角色间持续物理耦合 ⇒ 需要独立"交互解算器"，管线要加耦合阶段 |
| A3 | 内容分级由发行方决定 | 若内核也要按分级裁剪 ⇒ 分级进 ContentPack 而非仅呈现层 |
| A4 | 团队规模小（1–3 人） | 若是大团队 ⇒ 可拆成独立服务，接口从进程内调用改为 RPC |
| A5 | 语言为 Go（与宿主工程一致）或任意静态语言 | 若为 TypeScript/C# ⇒ 接口语义不变，`kernel/state` 的 SoA 优化方式要换 |

**待你拍板的三件事**：

1. **落地语言/引擎**（Go 内嵌？Unity/C#？TS？）——决定 kernel 的存储布局（AoS vs SoA）与序列化方案。
2. **单角色还是多角色**——决定 `sim/World` 的调度器复杂度。
3. **交付形态**——只要这份设计，还是要我接着产出 `content/schema/*.json` + M0 骨架代码（可跑空 Body 1000 tick）。

---

## 附录 A：与原讨论的对应关系

| 讨论中提出 | 本文档落点 |
|---|---|
| 四层数据模型 | §5 三层 + §6 管线 |
| 器官账本 8 字段 | §5.4 `organ` |
| 统一损伤账本 | §9 `injury` / `breach` |
| 体腔—屏障—内容物三元 | §5.4 `cavity` / `barrier` / `breach` |
| 路径图（口/肛/阴道） | §9 `route` + 结算算法 |
| 怀孕状态机 | §8.2 |
| 急症倒计时 | §8.1 |
| 内容分级 | §14 |
| 先做数据后做代码 | §15 M0–M6 |
