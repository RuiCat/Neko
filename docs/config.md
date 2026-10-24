# 基础配置层（域词汇）

状态机器相关的一组字符串原先硬编码在引擎、编译器和解析器里。现在集中到 `internal/config`，默认值与原行为完全一致。

## 配置项

| JSON 键 | 默认值 | 用途 |
|---|---|---|
| `state_group` | `状态` | 实体里承载状态机的顶层字段；Tick 写状态、冲突检测、解析器 `状态 = "x"` 语法糖都用它 |
| `state_definition_field` | `状态定义` | 声明每个状态组允许值的字段；`branch_race` 冲突检测读取它 |
| `default_state_subgroup` | `身体` | 分支写 `状态 = "昏厥"` 时省略的子组；也是裸字段默认挂载的分组 |
| `safety_enabled` | `false` | 打开 R18G 词表扫描；`false` 时与 Python 默认行为一致（全部放行） |
| `global_entities` | `["场景","世界","时间","天气","全局"]` | 不加 `{e}` 前缀、按全局实体解析的名称；解析器/编译器都读它 |
| `default_scene` | `场景` | `emit` 动作未指定场景时的落点 |
| `scene_event_field` | `事件` | 场景实体上保存事件列表的字段 |
| `classification_field` | `分类` | 实体的多分类标签字段（选择器 `成年[*]` 用它匹配） |
| `http_token` | 空 | HTTP 传输的 Bearer token；空 = 无鉴权，仅允许绑定回环地址 |

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
  "safety_enabled": false,
  "global_entities": ["场景", "世界", "时间", "天气", "全局"],
  "default_scene": "场景",
  "scene_event_field": "事件",
  "classification_field": "分类",
  "http_token": ""
}
```

## 影响范围

| 位置 | 行为 |
|---|---|
| `engine.Tick` / `ResolveConflict` | 写入 `<state_group>.<subgroup>`；冲突上下文键名同步 |
| 冲突检测 | 读 `<state_definition_field>` 判断哪些组可以竞争 |
| 编译器 | `状态:` 条件的重写、裸字段默认分组、`BareFieldMap` 里的 `状态.*` / `身体.*` 前缀 |
| 解析器 | 分支 `状态 = "x"` / `状态.情绪 = "x"` 语法糖 |
| 效果/赋值别名 | `效果词 → 状态.* / 身体.*` 的映射在应用时按配置归一化 |
| MCP `entity_query` | 返回行里的状态/身体键名 |

## 安全扫描（`safety_enabled`）

- `false`（默认）：不扫描，行为与 Python `SAFETY_ENABLED=False` 一致。
- `true`：`neko_script_apply` 先扫描 AST（规则/事件/函数/数据名），再扫描全文与脚本名；命中则返回 `ok=false`，错误块为 `R18G安全检查`。
- 启动重放（`skipSafety=true`）不重复扫描，避免把主人确认过的存量脚本卡住。
- 词表与 Python `script/safety.py` 一致；扫描实现见 `internal/safety`。

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
