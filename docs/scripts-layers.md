# 脚本层总览（内核 + 内容两层）

`scripts/` 分两块（目录结构与内容文件格式见 [scripts-content.md](scripts-content.md)）：

```text
scripts/
  kernel/     # 通用内核：@定义/@实体/@函数/@规则（本文档描述的部分）
  entities/   # 内容：具体实体 @data 实例块（角色/物品/食物）
  scenes/     # 内容：具体场景 @data 实例块
```

`kernel/` 是**通用底层定义**：所有规则只针对「内核模板」，不含任何角色 ID 或角色数据。
`entities/` 与 `scenes/` 是内容蓝图，**不注册为脚本**，由 `neko-data apply` 物化成实体
（默认不覆盖已有实体的运行时状态）。

## 组合契约

任何需要完整生理模拟的类型，组合以下内核（顺序无关）：

```neko
@实体 你的类型 [基础内核, 行为内核, 代谢内核, 循环内核, 呼吸内核,
                内分泌内核, 器官内核, 伤害内核, 路径内核, 生殖内核, 状态内核,
                交互内核, 行动内核, 规划内核, 教育内核, 交易内核, 记忆内核] {}
```

部分组合只保证该层自身规则可用；跨层读取的字段若缺失会按 0/字符串处理，**不要做部分组合后再断言数值**。

## 层与优先级

| 层 | 优先级 | 内核模板 | 目录 |
|---|---|---|---|
| B 基础数据 | 500–899 | 基础内核 | `scripts/kernel/base/` |
| 行为 | 900 | 行为内核 | `scripts/kernel/behavior/` |
| L0 代谢 | 1000–1999 | 代谢内核 | `scripts/kernel/metabolism/` |
| L1 生命轴 | 2000–2999 | 循环/呼吸/内分泌内核 | `scripts/kernel/physiology/` |
| L2 器官账本 | 3000–3999 | 器官内核 | `scripts/kernel/organs/` |
| L3 伤害/路径 | 4000–4999 | 伤害内核 / 路径内核 | `scripts/kernel/injury/` |
| L4 生殖 | 5000–5999 | 生殖内核 | `scripts/kernel/reproductive/` |
| L5 状态投影 | 9000–9999 | 状态内核 | `scripts/kernel/status/` |
| L6 物品/交互 | 7000–7999 | 素材/食物/饮品/工具/容器/衣物/交互内核 | `scripts/kernel/material/`、`scripts/kernel/interact/` |
| L7 行动 | 8000–8099 | 行动内核 | `scripts/kernel/action/` |
| L8 规划 | 8100–8199 | 规划内核 | `scripts/kernel/plan/` |
| L9 教育 | 8200–8299 | 教育内核 | `scripts/kernel/education/` |
| L10 交易 | 8300–8399 | 交易内核 | `scripts/kernel/trade/` |
| L11 记忆 | 8400–8499 | 记忆内核 | `scripts/kernel/memory/` |
| L6 物品/交互 | 7000–7999 | 素材/食物/饮品/工具/容器/衣物/交互内核 | `scripts/kernel/material/`、`scripts/kernel/interact/` |

非角色实体类型（独立实例，不组合进 NPC）：

| 类型 | 文件 | 说明 |
|---|---|---|
| 世界 | `scripts/kernel/world/worldModel.neko` | 单例：时间/天气/季节（兼容 `world_clock`） |
| 场景 | `scripts/kernel/scene/sceneModel.neko` | 地点：环境温（0.1°C）/湿度/光照/物品/在场 |
| 素材 | `scripts/kernel/material/materialModel.neko` | 物品公有账本：场景/持有人/数量/耐久/品质 |
| 食物 / 饮品 / 工具 / 容器 / 衣物 | 同上 | 素材的 5 个实例类型，各带自己的内核账本 |
| 交互 | `scripts/kernel/interact/interactModel.neko` | 角色侧动作意图与结算结果（见 [material-layer.md](material-layer.md)） |
| 关系 | `scripts/kernel/relation/relationModel.neko` | 主体→客体 的关系记录 |
| 任务 | `scripts/kernel/task/taskModel.neko` | 任务记录：阶段/条件/奖励 |

位置内核（`scripts/kernel/location/`）组合进 NPC，提供 `位置.场景`；场景温度经 950 段规则写入 `代谢.环境温`。

认知/社会层（L7–L11，8000–8499）负责行动状态机（空闲→执行→完成/中断）、规划（目标/阶段/紧迫度）、
教育（学习效率与熟练度）、交易（买卖与货币）、记忆（衰减/巩固/回忆强度）。只维护状态与派生量，
决策内容由上层写入意图字段；字段与公式见 [cognition-layers.md](cognition-layers.md)。

物品/交互层（L6，7000–7999）负责素材被动结算（变质/耗尽/磨损）与角色×素材的交互结算
（拾取/放下/给予/查看/使用/穿戴/吃/喝）；物品侧字段由 `$2` 写、角色侧由 `$1` 写。
字段、动作词表与引擎坑见 [material-layer.md](material-layer.md)。

规则按 `优先级` 升序执行（数值小的先跑），同优先级按注册顺序。

## 扩展方式

- **新路径**（如尿道/创口）：在 `scripts/kernel/injury/pathTick.neko` 加一条参数规则（阈值/菌量/出血系数）+ 一条「类型 → 靶器官应力」规则，不改函数。
- **新器官**：在 `organModel.neko` 加一块账本 + 一个功能函数；若要参与 MODS，在 `伤害器官衰竭()` 里加权。
- **新种族**：在 `baseInitTick.neko` 加一条 500 段规则（11 个 `种族参数.*`），键集必须与现有预设完全一致。
- **新状态词**：在 `statusFunctions.neko` 的 `状态身体分类()` 里加分支，保持阈值链顺序。

## 单位约定

kcal / mL / 0.1°C / mmHg / mg/dL / g / 分钟；比例用千分比（‰），质量占比用万分比。

## 验证

```bash
GOCACHE=$PWD/.local/gocache go run ./.local/probe
GOCACHE=$PWD/.local/gocache go test ./internal/engine -count=1
```

探针会：扫描 `scripts/**/*.neko` → 按「模板→组合→函数→规则」应用 → 建一个测试实体 → 跑常规/孕期/并发症/分娩/日 tick 多阶段。
