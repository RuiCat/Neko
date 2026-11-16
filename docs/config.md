# 基础配置层（域词汇）

状态机器相关的一组字符串原先硬编码在引擎、编译器和解析器里。现在集中到 `internal/config`，默认值与原行为完全一致。

## 配置项

| JSON 键 | 默认值 | 用途 |
|---|---|---|
| `state_group` | `状态` | 实体里承载状态机的顶层字段；Tick 写状态、冲突检测、解析器 `状态 = "x"` 语法糖都用它 |
| `state_definition_field` | `状态定义` | 声明每个状态组允许值的字段；`branch_race` 冲突检测读取它 |
| `default_state_subgroup` | `身体` | 分支写 `状态 = "昏厥"` 时省略的子组；也是裸字段默认挂载的分组 |
| `global_entities` | `["场景","世界","时间","天气","全局"]` | 不加 `{e}` 前缀、按全局实体解析的名称；解析器/编译器都读它 |
| `default_scene` | `场景` | `emit` 动作未指定场景时的落点；库里没有同名实体时按 NPC 当前场景解析 |
| `current_scene_path` | `位置.场景` | 实体上记录当前所在场景 id 的字段；默认场景名解析成当前场景时读它（留空 = 用默认） |
| `scene_event_field` | `事件` | 场景实体上保存事件列表的字段 |
| `scene_event_limit` | `50` | 场景事件列表保留条数上限（超出丢最旧；`0` = 不限）。事件黑板只投影最新一条，推送与存储都有界 |
| `scene_occupancy_field` | `在场` | 场景实体上记录「场内是谁」的 id 列表；每 tick 由各实体的 `current_scene_path` 重新推导并写回，谁进/出场景就是一次场景变化（留空 = 关闭同步） |
| `classification_field` | `分类` | 实体的多分类标签字段（选择器 `成年[*]` 用它匹配） |
| `http_token` | 空 | HTTP 传输的 Bearer token；空 = 无鉴权，仅允许绑定回环地址 |
| `emote_dir` | 空 | 表情库目录（`emote_index.json`、`vec_db.*`、`img/`）。**这是用户设置**：留空 = 未配置（不指向任何仓库路径，走环境变量/旧默认）；相对值按配置文件所在目录解析。安装**不拷贝**表情库 |

## 加载方式

```bash
./bin/astrbox-mcp -data-dir data -scripts-dir scripts -config config.json
```

- `-config` 缺省为当前目录的 `config.json`。
- 文件不存在 ⇒ 使用默认值（配置是可选的）。
- 文件只写部分键 ⇒ 其余键取默认值。
- 校验：三个值都不能为空，且不能包含 `.`、`[`、`]` 或空格（它们会被拼进点分路径）。

示例：

```json
{
  "state_group": "状态",
  "state_definition_field": "状态定义",
  "default_state_subgroup": "身体",
  "global_entities": ["场景", "世界", "时间", "天气", "全局"],
  "default_scene": "场景",
  "current_scene_path": "位置.场景",
  "scene_event_field": "事件",
  "scene_event_limit": 50,
  "scene_occupancy_field": "在场",
  "classification_field": "分类",
  "http_token": "",
  "emote_dir": ""
}
```

> 上面 `emote_dir` 是**用户设置的示例值**（相对配置文件目录）；不需要就直接删掉该键。

## 影响范围

| 位置 | 行为 |
|---|---|
| `engine.Tick` / `ResolveConflict` | 写入 `<state_group>.<subgroup>`；冲突上下文键名同步 |
| 冲突检测 | 读 `<state_definition_field>` 判断哪些组可以竞争 |
| 编译器 | `状态:` 条件的重写、裸字段默认分组、`BareFieldMap` 里的 `状态.*` / `身体.*` 前缀 |
| 解析器 | 分支 `状态 = "x"` / `状态.情绪 = "x"` 语法糖 |
| 效果/赋值别名 | `效果词 → 状态.* / 身体.*` 的映射在应用时按配置归一化 |
| MCP `entity_query` | 返回行里的状态/身体键名 |
| 场景解析（`default_scene` / `current_scene_path`） | `场景.事件 += ...` 与 `emit` 的目标：有同名实体用实体，否则读 actor 的当前场景路径 |
| 场景事件流（`scene_event_field` / `scene_event_limit`） | 事件写入的路径与保留条数；黑板把该字段投影成 `<字段>.最新` |
| 场景在场（`scene_occupancy_field`） | 每 tick 末尾按各实体当前位置重算并写回；位置没变不写。写入会被黑板当作场景变化推送 |

## 数据驱动的业务词汇（不写进源码）

以下字段都是**数据**：源码只保留通用运行时，具体角色/字段词表由配置提供。
为空时行为如下：

| 字段 | 类型 | 空值行为 |
|---|---|---|
| `effect_map` | `{词: 路径}` | 文本效果（`体力+5`）按目标实体自身叶子字段名解析（最短路径优先）；解析不到就跳过 |
| `assign_map` | `{词: 路径}` | 文本赋值（`身体状态=疲惫`）只认点分路径；词表为空则不解析该词 |
| `bare_field_map` | `{词: 路径}` | `.neko` 裸字段保持默认状态组前缀（`default_state_subgroup`） |
| `hard_state_paths` | `[...]` | 硬状态只含 `state_group`，不额外保护字段 |
| `default_entity` | `str` | 会话事件没有显式实体时不默认任何实体（调用方必须给） |
| `persona` | `str` | 本地 chat 用中性兜底提示词，不含角色 |
| `world_entity` | `str` | 世界时钟实体 id（默认沿用世界模板的 `世界`） |
| `world_time_path` / `world_month_path` / `world_phase_path` | `str` | 世界时钟字段路径（默认 `环境.当前时间` / `环境.当前月份` / `状态.阶段`） |

```json
{
  "effect_map": {"撒娇": "关系.撒娇欲"},
  "assign_map": {"身体状态": "状态.身体"},
  "bare_field_map": {"体力": "身体.体力"},
  "hard_state_paths": ["身体.体力", "位置.场景"],
  "default_entity": "你的角色实体ID",
  "persona": "你自己的人设文本"
}
```

## HTTP 传输

- `http_token` 非空时，请求必须带 `Authorization: Bearer <token>` 或 `X-Auth-Token`。
- token 为空时只允许 `127.0.0.1`/`localhost`/`::1`；绑定 `0.0.0.0` 或其它地址会直接拒绝启动。

## 修改词汇的注意事项

默认值就是现有数据与 187 个 `.neko` 脚本使用的词汇。改成别的值（例如 `状况`）时，**数据文件和脚本里的路径也要一起改**：脚本中显式写出的 `状态.身体` 不会被自动翻译，它会被当作字面路径。推荐只在做新世界/新数据时改，或保持默认值。

## 代码入口

```go
cfg, err := config.Load(path)   // 缺失文件返回默认值
config.Set(cfg)                 // 校验并生效（进程级）
config.Current()                // 读取当前配置
config.Reset()                  // 测试用：恢复默认
```

`config.Config.NormalizePath` 负责把历史别名表里的 `状态.` / `身体.` 前缀换到当前词汇。
