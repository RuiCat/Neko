# MCP 工具参考

服务以 MCP（Model Context Protocol）暴露工具。`astrbox-mcp` 暴露 31 个引擎工具；
`dsh-mcp` 在此基础上增加 `context_list` / `context_send`（共 33 个，用于角色上下文驱动）。
默认 stdio；`-transport http` 时使用 Streamable HTTP。

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
      "command": "/path/to/Neko/bin/astrbox-mcp",
      "args": ["-data-dir", "/path/to/Neko/data", "-scripts-dir", "/path/to/Neko/scripts"]
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

写入实体状态并落盘 `entities.json`。单路径（`path`+`value`）与批量（`values`）二选一。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `entity` | string | 是 | 实体 ID |
| `path` | string | 否 | 点分路径（与 `values` 二选一） |
| `value` | string | 否 | JSON 或纯文本；能解析为 JSON 就按 JSON 存 |
| `values` | object | 否 | 批量 `{点分路径: 值}`（与 `path/value` 二选一） |

返回 `{"ok":true,"entity":"…","paths":["…"]}`（K20 起由文本改为 JSON；旧 CLI 侧无消费者）。

### `entity_delete`

删除实体并同步删除其事件黑板，落盘 `entities.json` / `blackboard.json`。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `entity` | string | 是 | 实体 ID |

返回 `{"ok":true,"deleted":"实体 ID"}`；不存在返回 `❌`。

### `blackboard_get`

读取事件黑板（每实体「当前/上一次状态」变化记录）。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `entity` | string | 否 | 实体 ID；留空返回全部实体黑板 |

返回 `{"ok":true,"board":{...}|null}` 或 `{"ok":true,"boards":[...]}`，形状与 `GET /blackboard` 一致
（`entity`/`version`/`updated_at`/`tick_id`/`entries[{path,from,to,rule_ids,updated_at}]`）。

### `world_clock`

读取 / 推进 / 设定世界时钟（`世界.环境.当前时间`、`当前月份`、`状态.阶段`）。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `minutes` | integer | 否 | 推进分钟数（1..1440），与 `time` 二选一 |
| `time` | string | 否 | RFC3339 时间（如 `2026-08-10T09:00:00+08:00`），与 `minutes` 二选一 |

无参读取 `{"ok":true,"worldTime","month","phase"}`；写入后落盘并返回同一形状。
`phase` 是**脚本层**派生的标签（`scripts/kernel/world/worldPhase.neko` 的 `时段判断`/`世界时段` 规则，tick 时校准）；
工具只写时间与月份并回显实体里当前的 `状态.阶段`，不自己算时段。

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

## 流程

flow 引擎（P4 的 K1–K4）的显式执行入口：只在被调用时执行一次，不是 tick 钩子。

### `flow_run`

读 `<data-dir>/flow-spec.json` → 按 id 找 flow → 前置判定（实时实体与素材视图）→ 步骤推到 `done` → 结算 → 任务推进 → 落盘。全程在状态写门内，与 tick 互斥。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `entity` | string | 是 | 执行流程的实体 ID |
| `flow` | string | 是 | `flow-spec.json` 的 `flows[].id` |
| `execution_id` | string | 否 | K3 执行 ID；留空自动生成 |

返回 JSON：

- 成功：`{"ok":true,"completion":{"flowID","entity","result","tick"},"report":{"changes":[{"op","entity","scene","path","kind","value","item"}],"errors":[]},"task":{"flowID","entity","tick","fromStage","toStage","stages","progress","rewards","replay"},"verdict":{"ok":true,"reasons":[]}}`。
- 前置判定失败（零写入）：`{"ok":false,"report":{"changes":[],"errors":[]},"verdict":{"ok":false,"reasons":[{"code","path","want","got"}]}}`，`completion`/`task` 省略，`error` 不出现。
- 其它失败（实体不存在/规格文件缺失/flow 不存在/步骤、结算或任务推进失败）：`{"ok":false,...,"error":"原因"}`；规格文件缺失时 `error` 带 `data/flow-spec.json` 的放置路径。

幂等：同一 `(entity, flow)` 一旦在 `<data-dir>/flow-ledger.json` 有结算记录就不再结算（不重复扣物品、不重复发奖励），只回放既有 `completion` 与 `task`（`task.replay=true`）并返回空 `report`；回放仍会读一次当前快照，`verdict` 只描述当前状态，`ok` 表示回放本身成功。flow 没有同名 `taskTemplates` 条目时跳过任务推进，返回体不含 `task`。结算只写 C3 源层字段，`交易.货币` 不由本工具扣减（C5，留给交易 tick）。`completion.tick` 固定 `0`（工具不带世界 tick 序号），步骤耗时按 `0` 记账。

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

## 表情

### `emote_find`

根据文本语义在 QQ 表情库中检索，返回图片路径（可直接交给 `qqbot_send_file` 发送）。
表情库路径由配置指定：`-emote-dir` > `ASTRBOX_EMOTE_DIR` > `config.json` 的 `emote_dir`
（默认留空 = 未配置；相对值按配置文件目录解析）。安装**不拷贝**表情库、也不自动填 `emote_dir`；
只有配置里写了相对路径时，`make install-runtime` 才把它按仓库根转成绝对路径。
表情数据（图片、向量、索引）**不进版本库**。

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `text` | string | 是 | 想表达的内容/情感，如 `主人我好想你呀` |
| `top` | integer | 否 | 返回数量，默认 3，夹紧到 1–20 |

返回 JSON：`{"source":"vec_db|vectors|keyword","results":[{"file","desc","score","categories","intent_bonus","cluster","path","missing"}]}`。

- 三级降级：`vec_db.npz`（128 簇两层检索）→ `emote_vectors.json`（余弦 ≥0.35）→ 关键词打分；
  三级都不可用时返回 `❌ 表情库索引缺失: <路径>`。
- 检索含意图融合；R18 与「暧昧」内容过滤已于 2026-11-16 按维护者要求移除（历史见 `docs/emote-tool-migration.md`）；
  Ollama `nomic-embed-text` 不可用时返回 `❌`，不静默降级（决策 d3）。
- 缺图行返回 `path:""` + `missing:true`；查询文本不落盘、不进日志。
- 构建/巡检用 CLI（`make build` 产出 `bin/astrbox-emote`）：
  `astrbox-emote info` / `find [-top N] [-json] <文本>` / `build [-dry-run] [-limit N] [-workers N] [-out DIR]`。

## 运维（K20 工具化）

以下工具是控制面运维路由的薄封装，行为与对应 HTTP 端点一致；失败返回 `❌` 文本。

| 工具 | 参数 | 说明 |
|---|---|---|
| `nightly_run` | `factor?`（number） | 专家上下文夜间衰减并清理过期项 |
| `judge_run` | `entity_id`、`path`、`proposed?`、`source?`、`evidence?` | 对一条状态变更做裁决，写入候选 |
| `candidates_list` | — | 列出待处置候选 |
| `candidate_resolve` | `id`、`status`、`note?` | 处置候选（accept/reject 等） |
| `candidate_apply` | 与 `POST /candidates/apply` 请求体一致 | 应用已接受候选（含快照/回归闸门；409 body 冲突） |
| `repair_rollback` | 与 `POST /repairs/rollback` 请求体一致 | 回滚修复历史 |

## 上下文（跨角色，仅 `dsh-mcp`）

### `context_list`

无参数，返回 `{"contexts":[{contextId,characterId,sessionId,status,lastTurn,...}]}`。
未配置 `contexts.json` 时返回 `❌ 上下文驱动未启用`。

### `context_send`

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `target_context_id` | string | 是 | 目标上下文（先查 `context_list`） |
| `text` | string | 是 | 投递正文 |
| `priority` | string | 否 | `normal`（默认）/ `urgent`（对方思考中插入） |
| `source_context_id` | string | 否 | 来源上下文；用于循环防护与署名 |

保护：同一 `(来源,目标,正文)` 60 秒内只发一次；每个来源每分钟最多 10 条。
MCP 调用本身不携带会话身份，来源需要调用方显式给出（P3 的桥接原生工具才能自动归属）。

## 训练（智慧体主动训练）

智慧体可以直接提交训练数据并触发训练（默认 dry-run，安装权重需显式 `deploy`）。

### `gate_status`

无参数。返回自动/教师样本数、可导出样本、主专家标签分布、覆盖/失衡门槛是否满足、
模型是否已安装与当前门控版本（`gate-mlp-v1` / `gate-v1`）。

### `gate_teach`

| 参数 | 说明 |
|---|---|
| `samples[]` | 教师样本（1–200 条，整批校验，一条非法整批拒绝） |
| `samples[].target_expert` | 目标语义专家：关系记忆 / 事件记忆 / 自我记忆 / 知识边界 |
| `samples[].event_kind` | 事件类型（缺省 `turn`） |
| `samples[].summary` | 对话摘要（训练特征） |
| `samples[].affect` | 情感向量，如 `{"trust":0.4,"valence":-0.8}` |
| `samples[].hard_state_keys` | 硬状态键，如 `["知识边界"]` |

写入 `<data>/training/gate-teacher.jsonl`（`source=teacher`、`primary=目标专家`），
导出训练集时与自动样本合并。

### `gate_train`

| 参数 | 说明 |
|---|---|
| `deploy` | 默认 false：只训练+校验不安装；true：过全部门槛才安装（重启 `dsh-mcp` 生效） |
| `min_samples` / `min_classes` / `min_class_samples` | 样本与类别覆盖门槛（默认 50 / 4 / 5） |
| `max_class_share` | 最大类别占比门槛（0-1，默认 0.7；超过拒绝，避免多数类撑起准确率） |
| `min_accuracy` | 留出集准确率门槛（默认 0.8） |
| `epochs` | 训练轮数（默认 400） |
| `ita_dir` | 算法导论仓库路径（默认 `$ITA_DIR`） |

返回 `ok` / `exit_code` / `output_tail`；门槛不过时 `ok=false` 并附训练日志尾部。

## 权限与安全

- HTTP 模式支持 `http_token` Bearer 鉴权；token 为空时只允许回环地址。stdio 模式由启动方控制。
- 服务没有 AstrBot 的「主人白名单」概念：**任何能连上 MCP 服务的客户端都拥有完整读写权限**。stdio 模式下由启动方控制；HTTP 模式请只绑定到 `127.0.0.1`，不要暴露到公网。
- 写操作会直接改 `data/` 下的 JSON 与 `scripts/` 下的源文件。生产数据请先备份，或用只读副本调试。
- `neko_script_write` 覆盖已有脚本必须带 `expected_hash`，只接受脚本名（不接受任意路径）；编译失败不注册、不落盘。
- 诊断保存在内存（容量 512），进程重启即清空；它不替代 `data/logs/` 与 `llm_decisions.json` 的持久记录。
- `side_effects` 会被编译成回填规则的 `then` 操作；`reason` 与冲突上下文写入 `<dataDir>/llm_decisions.json`。
