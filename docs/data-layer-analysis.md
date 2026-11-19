# 数据层分析：实现、功能与定义

> 基于当前工作树（HEAD 9805007 + 未提交改动）。结论先行：**脚本层已经是一套通用内核，数据层还停留在「旧模板 + 旧实例 + 无迁移」的状态**，两者之间缺一层「类型契约与迁移」。

## 1. 边界

| 层 | 负责 | 不负责 |
|---|---|---|
| 脚本层（`scripts/**/*.neko`） | 类型定义（`@定义`/`@实体`）、规则、函数、事件 | 具体角色的值、持久化、投影 |
| **数据层** | 实例（实体）、类型模板、注册表、快照、迁移、投影配置 | 生理逻辑、文案 |
| 表现层（DSH 插件/UI） | 快照注入、事件订阅、渲染 | 真值计算 |

数据层是**真值的容器与通道**：脚本算出值，数据层负责存住、查得到、迁得动、投得出去。

## 2. 现有实现（证据）

| 组件 | 位置 | 形态 |
|---|---|---|
| 环境实体 | `data/entities.json` | `{entities:{id:{type,keys,data,_meta}}}`，场景/世界/素材 |
| 角色实体 | `data/user/entities.json` | 同上，`猫猫`/`梦梦` |
| 模板 | `data/templates/*.json` | 文件名 `_` ↔ 类型名 `:`；含 `__annotations__` |
| 脚本注册表 | `data/scripts.json` | `{hash, templates, rules, funcs, events}` + 引用计数 |
| 快照 | `data/archives/<name>.json` | `entity_snapshot` 产物 |
| 投影 | `plugins/dsh-astrbox/lib/prompt.mjs` | `paths` 白名单、1200 字上限、缺失写 `缺失:` |
| 绑定 | `plugins/dsh-astrbox/lib/entityBindings.mjs` | 按 cwd 最长前缀解析生效实体 |
| 访问面 | `docs/mcp-tools.md` | `entity_create/get/set/delete/query/snapshot`、`state_machine_tick`、`neko_script_*` |

关键语义（`docs/neko-semantics.md`）：

- 模板合并：`DeepMerge`，后写覆盖；`@实体` 子不得覆盖父，字段严格单所有者。
- 读取：实例优先，缺失回退模板链。
- **没有模板迁移**：模板新增字段对已有实体只「可读」，不写实例（§1.4、§9）。
- 启动重放只恢复内存，不写回 `scripts.json` / 模板文件。

## 3. 数据层应提供的功能（定义）

| 编号 | 功能 | 判据 |
|---|---|---|
| F1 | 类型与组合管理 | 类型可声明、组合父列表一致、字段不重叠；能列出「某类型有哪些内核」 |
| F2 | 实例化 | `create(type,id,data)` = 模板链合并 + data 覆盖 + 初始化触发；失败不落盘 |
| F3 | 字段读写 | 点分路径、类型/限幅/只读校验、批量写、原子落盘 |
| F4 | 查询 | 按 id/类型/key/字段值；支持按内核能力筛选 |
| F5 | 持久化 | 临时文件 + rename；快照/恢复；并发锁 |
| F6 | **迁移** | dry-run、版本化、可回滚；补齐缺失字段、映射旧值 |
| F7 | 投影 | 白名单 paths、长度预算、缺失策略、事件黑板 |
| F8 | 生命周期 | 删除/归档/引用检查（父模板被引用时拒绝删除） |
| F9 | 一致性 | 模板文件 ↔ 脚本 `@定义` ↔ 实例三方核对 |
| F10 | 校验 | schema 版本、类型、注解完整性、种族键集一致 |

## 4. 缺口（对照当前实现）

| 编号 | 缺口 | 影响 | 证据 |
|---|---|---|---|
| G1 | **无迁移机制** | 旧实体拿不到 12 个内核字段（`基础/代谢/循环/器官/伤害/…`），首 tick 会把函数名写进字段（已在探针复现） | `docs/neko-semantics.md:53,190` |
| G2 | 模板双源 | `data/templates/*.json` 与 `scripts/**/@定义` 可能漂移；重放不写模板文件 | `docs/ARCHITECTURE.md` 启动流程 |
| G3 | 新旧字段并存 | `角色基础.json` 有 40+ 旧组，新内核是另一套顶层组；无所有权表校验 | `data/templates/角色基础.json` |
| G4 | 创建不初始化 | `entity_create` 只合并模板；B 层初始化要等第一次 tick | `baseInitTick.neko` |
| G5 | 投影未覆盖新字段 | `paths` 白名单仍是旧字段（`状态.情绪` 等），`角色状态.*`/`伤害.*` 未投 | `prompt.mjs:4` |
| G6 | 查询无能力筛选 | 只能按类型/字段，不能「所有组合了代谢内核的实体」 | `entity_query` 契约 |
| G7 | 快照无 schema 版本 | 恢复时无法判断字段口径 | `entity_snapshot` |
| G8 | 删除父模板即拒绝 | 没有「先迁移再删」流程 | `docs/neko-semantics.md:191` |
| G9 | 绑定不持久化 | 实体↔会话映射只在插件内存/配置里 | `entityBindings.mjs` |

## 5. 建议冻结的定义

### 5.1 数据目录契约

```text
data/
  entities.json        # 环境实体
  user/entities.json   # 角色实体
  templates/<name>.json + <name>.parents.json
  archives/<snapshot>.json
  migrations/<version>.json   # 新增
  scripts.json
```

### 5.2 实体记录 v2

```json
{
  "schemaVersion": 2,
  "entities": {
    "猫猫": {
      "type": "角色基础:猫猫",
      "keys": {},
      "data": {},
      "_meta": { "createdAt": "...", "updatedAt": "...", "templateHash": "..." }
    }
  }
}
```

### 5.3 迁移文件

```json
{
  "version": 1,
  "description": "补齐生理内核字段",
  "ops": [
    { "op": "setDefault", "when": "missing", "path": "代谢.流逝分钟", "value": 60 },
    { "op": "setDefault", "when": "missing", "path": "基础.性别", "value": "未知" }
  ],
  "verify": ["代谢.流逝分钟", "基础.已初始化"]
}
```

- `when: missing` 只补缺、不覆盖；`when: always` 强制；`mapping` 做旧值→新值。
- 先 `--dry-run` 输出将写的路径与条数，再 `apply`；写前自动快照。

### 5.4 投影契约

```yaml
paths:
  - 角色状态.体力
  - 角色状态.身体状态
  - 伤害.休克
  - 生殖.怀孕状态
maxChars: 1200
missing: marker   # marker | omit
```

### 5.5 工具契约（新增）

| 工具 | 行为 |
|---|---|
| `entity_migrate` | `dryRun=true` 只读核对；`false` 写盘并快照 |
| `entity_validate` | 模板/实例/注解/种族键集三方核对，返回差异清单 |
| `entity_capabilities` | 列出实体组合了哪些内核与特征 |

## 6. 实施顺序

| 阶段 | 内容 | 验收 |
|---|---|---|
| M0 | 只读核对工具：实体 vs 模板 vs 内核字段，输出缺失矩阵 | 对 `猫猫/梦梦` 输出缺失字段清单，不写盘 |
| M1 | 迁移器：dry-run → apply → 自动快照 → 回滚 | 迁移后 `已初始化=false`，首 tick 完成初始化且数值合理 |
| M2 | 创建即初始化（可选）：`entity_create` 后立即跑一次初始化 | 新实体创建后 `基础.已初始化=true` |
| M3 | 投影扩展：`paths` 覆盖 `角色状态.*`，带长度预算 | `[astrbox]` 快照出现新字段 |
| M4 | schema 版本 + 校验 + 能力查询 | `entity_validate` 全绿；旧档可识别版本 |

## 7. 风险

1. **迁移不可逆**：必须写前快照、支持回滚。
2. **双源漂移**：模板文件与脚本 `@定义` 需定期核对（M4 的 validate）。
3. **旧字段别名**：`身体.体力` 与 `角色状态.体力` 双写期，读取以新为准，删除别名要等 v2。
4. **投影超长**：新字段多，1200 字预算需要按优先级裁剪（先状态、后细节）。
5. **初始化时机**：创建即初始化 vs 首 tick 初始化要二选一并写进契约，避免「半初始化实体」。
