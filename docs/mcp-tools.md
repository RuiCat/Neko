# MCP 工具参考

服务以 MCP（Model Context Protocol）暴露 17 个工具。默认 stdio；`-transport http` 时使用 Streamable HTTP。

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
| `diagnostics` | string | 否 | `none` / `summary`（默认）/ `full`；附加当前状态机诊断 |

返回 `{"实体","类型","数据"}` 或 `{"实体","路径","值"}`。完整数据查询在诊断非空时额外带
`diagnostics` 字段：`{"generation","summary":{"total","error","warning","info","overflow"},"items":[...],"truncated"}`。
`items` 默认最多 3 条（`full` 为 50 条），单条消息截断到 200 字；`none` 完全不出现该字段。

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

无参数，返回 `{"scripts":[...],"items":[{name,path,hash,rules,templates,funcs,events}]}`；
`scripts` 保持旧版名字数组兼容。

### `neko_script_read`

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `name` | string | 是 | 脚本名 |

返回 `{"name","path","source","hash","actual_hash","entry"}`。只读；拒绝符号链接、越界路径与超过
256KB 的源文件。

### `neko_script_validate`

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `name` | string | 否 | 脚本名（用于安全检查上下文） |
| `script` | string | 是 | 完整脚本原文 |

dry-run：解析 + 安全 + 编译，**不注册、不落盘**。返回
`{"ok","errors","warnings","templates","rules","funcs","events"}`。

### `neko_script_write`

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `name` | string | 是 | 脚本名 |
| `script` | string | 是 | 完整脚本原文 |
| `expected_hash` | string | 否 | 当前脚本 hash；覆盖已有脚本时必填 |

事务式写入：先编译，成功后才注册并原子写源文件；`expected_hash` 不匹配直接拒绝，不覆盖。
编译失败不落盘、不注册。返回与 `neko_script_apply` 相同的 `ApplyResult`。

### `diagnostics_query`

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `severity` | string | 否 | `error` / `warning` / `info` |
| `source` | string | 否 | `registry` / `engine` / `bridge` / `consistency` |
| `entity` | string | 否 | 按实体过滤 |
| `script` | string | 否 | 按脚本过滤 |
| `code` | string | 否 | 按错误码过滤 |
| `since` | string | 否 | 只返回该时间之后出现的（RFC3339） |
| `limit` | int | 否 | 最多条数，默认 20，上限 200 |

返回 `{"generation","summary":{...},"items":[...],"truncated"}`。诊断在内存环形缓冲区中按
`code+source+script+entity+line+message` 去重，重复只累加 `count`，容量 512，溢出计入 `overflow`。
常见错误码：`script.apply.failed`、`script.replay.skipped`、`script.safety.blocked`、
`script.source.missing`、`script.source.orphan`、`script.hash.mismatch`、`tick.rule.error`、
`tick.conflict.pending`、`tick.budget.exceeded`、`event.unregistered`、`schema.validation.failed`。

## 库存

### `inventory`

无参数，返回实体列表与计数、模板列表与计数、规则列表与计数、脚本列表与计数。

### `stats`

无参数，返回实体总数与按类型分布、规则总数/启用数/优先级分布、模板数、脚本数；
接诊断存储时额外返回 `diagnostics` 汇总计数。

### `event_log`

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `limit` | int | 否 | 最近条数，默认 20 |

返回 `{"validations":[...],"events":[...],"history":[...]}`：schema 校验失败（越界/类型不符）、
最近一次事件派发记录，以及最近若干条结构化派发历史（`name/entity/field/status/message/time`）。

### `entity_snapshot`

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `name` | string | 否 | 快照文件名（不含路径/扩展名），缺省用 UTC 时间戳 |

把当前全部实体写入 `<dataDir>/archives/<name>.json`，返回 `{"path","entities","bytes"}`。名称不允许包含路径分隔符、点或空格。

## 权限与安全

- HTTP 模式支持 `http_token` Bearer 鉴权；token 为空时只允许回环地址。stdio 模式由启动方控制。
- 服务没有 AstrBot 的「主人白名单」概念：**任何能连上 MCP 服务的客户端都拥有完整读写权限**。stdio 模式下由启动方控制；HTTP 模式请只绑定到 `127.0.0.1`，不要暴露到公网。
- 写操作会直接改 `data/` 下的 JSON 与 `scripts/` 下的源文件。生产数据请先备份，或用只读副本调试。
- `neko_script_write` 覆盖已有脚本必须带 `expected_hash`，只接受脚本名（不接受任意路径）；编译失败不注册、不落盘。
- 诊断保存在内存（容量 512），进程重启即清空；它不替代 `data/logs/` 与 `llm_decisions.json` 的持久记录。
- `side_effects` 会被编译成回填规则的 `then` 操作；`reason` 与冲突上下文写入 `<dataDir>/llm_decisions.json`。
