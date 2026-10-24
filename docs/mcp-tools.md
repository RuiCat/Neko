# MCP 工具参考

服务以 MCP（Model Context Protocol）暴露 13 个工具。默认 stdio；`-transport http` 时使用 Streamable HTTP。

## 启动

```bash
./bin/astrbox-mcp -data-dir data -scripts-dir scripts          # stdio
./bin/astrbox-mcp -data-dir data -scripts-dir scripts -transport http -addr 127.0.0.1:8765
```

环境变量 `ASTRBOX_DATA_DIR` / `ASTRBOX_SCRIPTS_DIR` 可替代对应参数。

客户端配置（以 DSH / Claude Desktop 的 stdio 形式为例）：

```json
{
  "mcpServers": {
    "astrbox": {
      "command": "/home/ruicat/Desktop/Neko/bin/astrbox-mcp",
      "args": ["-data-dir", "/home/ruicat/Desktop/Neko/data", "-scripts-dir", "/home/ruicat/Desktop/Neko/scripts"]
    }
  }
}
```

## 返回约定

所有工具返回文本内容：成功是 JSON 字符串；失败以 `❌` 开头并带原因。数值、路径、实体均按 UTF-8 中文原样返回。

## 实体

### `entity_state_get`

读取实体状态。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `entity` | string | 是 | 实体 ID，如 `猫猫` |
| `path` | string | 否 | 点分路径，如 `身体.体力`；留空返回完整 `data` |

返回 `{"实体","类型","数据"}` 或 `{"实体","路径","值"}`。

### `entity_state_set`

写入实体状态并落盘 `entities.json`。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `entity` | string | 是 | 实体 ID |
| `path` | string | 是 | 点分路径 |
| `value` | string | 是 | JSON 或纯文本；能解析为 JSON 就按 JSON 存 |

### `entity_query`

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `query` | string | 是 | `NPC[*]`（类型通配）/ `猫猫`（ID）/ `key:场景=客厅`（Key）/ `状态.身体=疲惫`（字段值） |

返回 `{"query","count","entities":[{"id","type","keys","状态","身体"}]}`。

### `entity_create`

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `type` | string | 是 | 类型名，需存在模板或继承链 |
| `name` | string | 是 | 唯一实体 ID |
| `data` | string | 否 | 初始数据 JSON，覆盖模板 |

## 状态机

### `state_machine_tick`

推进一个周期。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `scope` | string | 否 | `*`（默认）/ `NPC[*]` 等选择器 |

返回 `TickResult`（snake_case）：`tick_id`、`status`、`transitions[]`（`entity`/`rule`/`from`/`to`）、`conflicts[]`、`hint`。Tick 成功后服务端自动保存实体，并把结果追加到 `data/logs/state_YYYY-MM-DD.log`（保留 7 天）；日志行带 `snapshots`（本 tick 涉及实体的体力/身体状态/情绪）。

### `state_machine_resolve`

仲裁 `tick` 返回的冲突。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `conflict_id` | string | 是 | `conflicts[]` 里的 id |
| `target_state` | string | 是 | JSON 对象，如 `{"身体":"疲惫"}` |
| `reason` | string | 否 | 决策理由，写入 `llm_decisions.json` |
| `side_effects` | string | 否 | JSON 数组，元素为 `{"op","path","value"}`；提供时回填一条高优先级 `learned_*` 规则 |

## 脚本

### `neko_script_apply`

解析并应用一段 `.neko` 脚本：注册模板、规则、函数与事件，写 `scripts/<name>.neko` 与 `scripts.json`。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `name` | string | 是 | 脚本名，允许字母/数字/下划线/中文/连字符 |
| `script` | string | 是 | 完整脚本原文 |

返回 `{"ok","script","applied":{...},"errors":[...],"warnings":[...],"noop"}`。

### `neko_script_remove`

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `name` | string | 是 | 脚本名 |

释放模板/规则引用（引用归零才删除），删除源文件与条目。

### `neko_script_list`

无参数，返回 `{"scripts":[...]}`。

## 库存

### `inventory`

无参数，返回实体列表与计数、模板列表与计数、规则列表与计数、脚本列表与计数。

### `stats`

无参数，返回实体总数与按类型分布、规则总数/启用数/优先级分布、模板数、脚本数。

### `event_log`

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `limit` | int | 否 | 最近条数，默认 20 |

返回 `{"validations":[...],"events":[...]}`：schema 校验失败（越界/类型不符）与最近一次事件派发记录。

### `entity_snapshot`

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `name` | string | 否 | 快照文件名（不含路径/扩展名），缺省用 UTC 时间戳 |

把当前全部实体写入 `<dataDir>/archives/<name>.json`，返回 `{"path","entities","bytes"}`。名称不允许包含路径分隔符、点或空格。

## 权限与安全

- HTTP 模式支持 `http_token` Bearer 鉴权；token 为空时只允许回环地址。stdio 模式由启动方控制。
- 服务没有 AstrBot 的「主人白名单」概念：**任何能连上 MCP 服务的客户端都拥有完整读写权限**。stdio 模式下由启动方控制；HTTP 模式请只绑定到 `127.0.0.1`，不要暴露到公网。
- 写操作会直接改 `data/` 下的 JSON 与 `scripts/` 下的源文件。生产数据请先备份，或用只读副本调试。
- `side_effects` 会被编译成回填规则的 `then` 操作；`reason` 与冲突上下文写入 `<dataDir>/llm_decisions.json`。
