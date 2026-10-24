# 剩余项深度分析（2026-10-24）

> 范围：Go 引擎 + MCP 服务。用户已明确「非引擎工具不在处理」，所以 `trash/secrets/stats/behavior/restart/emote/session_chat/event_repair/rule_derive` 不计入待办，只在第 8 节列明。本文只做分析，不改代码。

## 0. 结论摘要

- **正确性关键项已闭合**：186 脚本重放、1808 条规则、规则 id 与 Python 一致、事件/函数/状态机/冲突/条件算术均有测试。
- **剩余项分三类**：① 有真实行为差异但不影响当前语料（模板持久化、日志噪声、校验日志）；② 架构一致性（配置层只覆盖状态词汇）；③ 运行环境阻塞（异盘备份）。
- **P0：无**（除环境阻塞的备份）。**P1：模板持久化一致性、HTTP 安全边界（若暴露端口）**。**P2：可观测性、配置覆盖面、日志噪声**。**P3：scripts.json 字节序、format 规格、冷门方法**。
- **明确不做**：bytes/`encode`、任意对象方法（.neko 无类模型）、非引擎工具。

## 1. 清单总览

| ID | 项 | 类型 | 影响 | 证据 |
|---|---|---|---|---|
| A1 | `str.encode` / bytes 类型缺失 | 值模型 | 低（语料无） | `internal/function/methodsText.go` 无 encode |
| A2 | `translate/format_map/fromkeys` 缺失 | 值模型 | 低 | 同上；`internal/function/methods.go` |
| A3 | `format` 不支持格式规格 `{:.2f}`/`!r`/命名参数 | 值模型 | 低 | `methodFormat` 只解析空字段与整数下标 |
| B1 | 模板不落盘（Python 会写 `templates/*.json`） | 数据一致性 | 中低 | `internal/entity/store.go:146`；`legacy/.../store.py:219,250` |
| B2 | `scripts.json` 键顺序变化（Go map 排序） | 数据形态 | 低 | `internal/registry/registry.go:127`；`legacy/.../registry.py:114` |
| B3 | 空 Tick 也写日志（Python 跳过） | 日志噪声 | 低 | `internal/engine/stateLogger.go:30`；`legacy/.../state_logger.py:72` |
| C1 | schema 校验结果被丢弃（Python 记日志） | 可观测性 | 中低 | `internal/events/bridge.go:75`；`legacy/.../events.py:86` |
| C2 | 同名事件冲突日志缺失 | 可观测性 | 低 | Python `EventSink._conflict_log`，Go `Bus.Register` 静默 |
| D1 | 全局实体/默认场景/事件字段/分类字段仍硬编码 | 配置覆盖 | 中 | `compiler/rewrite.go:15`、`compiler/ops.go:75,86`、`engine/ops.go:108-119`、`entity/store.go:245` |
| E1 | HTTP 无鉴权/无 TLS | 安全边界 | 高（若暴露） | `cmd/astrbox-mcp/main.go:75-80` |
| E2 | 无「主人白名单」概念 | 安全边界 | 低（本地 stdio） | 文档已声明任何客户端全权限 |
| F1 | 规则匹配线性扫描 | 性能 | 低（当前规模） | `Tick` 遍历 `m.rules` |
| G1 | 异盘备份无法完成 | 环境 | 高（合规） | `findmnt`：`/mnt/bnsvm`、三块 `/run/media` 均 `ro` |

## 2. 逐项分析

### A1/A2/A3 解释器值模型边界

`.neko` 的值模型是封闭的：`nil/bool/int/float64/string/[]any/map[string]any`。由此产生边界：

- **bytes/`encode`**：没有 `[]byte` 类型，`str.encode` 无处可放；补它要同时改 `pyText`、比较、JSON、方法分派，收益是零语料使用 ⇒ **不做**。
- **`translate/maketrans/format_map`**：依赖映射表/命名参数；`fromkeys` 是类方法，语法上是 `dict.fromkeys`，而 `dict` 在值模型里不是可调用对象 ⇒ **不做**（除非将来脚本使用）。
- **`format` 规格**：当前只支持 `{}`、`{0}`、`{{`/`}}`；`{:.2f}` 会被当作未知字段原样输出，不会报错。语料无使用 ⇒ 记录为已知边界；若需要，按字段冒号后规格补 `d/f/s` 即可（S）。

### B1 模板持久化（最值得处理的行为差异）

- Python `add_template`：内存合并后写 `templates/<type>.json`；`remove_template` 同时删文件（`legacy/.../store.py:219,250`）。
- Go `AddTemplate/RemoveTemplate`：只改内存（`internal/entity/store.go:146,157`）。
- 后果：
  1. 用 `neko_script_apply` 新增脚本后，Python 数据目录会多出模板文件；Go 不会。重启时 Go 靠 `scripts/*.neko` 重放恢复，通常等价。
  2. **源文件缺失时不等价**：若 `scripts/<name>.neko` 丢失，Python 仍有模板文件（`_replay_events` 还能重放事件定义），Go 的模板会一起丢失。
  3. 外部工具/人工检查按 `templates/*.json` 找类型时会看不到 Go 新增的模板。
- 实现注意：重放（`applyScript(..., persist=false)`）也会调用 `AddTemplate`；若直接加落盘，会在每次启动重写模板文件。需要给 `AddTemplate` 增加 persist 参数或由 registry 控制，保持「只读启动不写数据」的现有承诺。
- 工作量 S–M，风险中低；建议 P1。

### B2 scripts.json 字节序

- 规则 id 已与 Python 完全一致，语义无差异；Go 用 `map` + `MarshalIndent`，键按字典序输出，Python 保持插入序。
- 影响仅限「文件 diff/格式化」；若外部有按行 diff 的流程会造成噪声。补有序编码器 S，但收益低 ⇒ P3。

### B3 空 Tick 日志

- Python `log_tick` 在 `transitions` 与 `conflicts` 都为空时直接返回（`state_logger.py:72`）。
- Go `Log` 无条件写一行（`stateLogger.go:30`），每次空 tick 都产生一条记录。长时间运行会积累噪声。
- 修复 S：`Log` 开头判断 `len(result.Transitions)==0 && len(result.Conflicts)==0` 时返回 nil；测试补一条。

### C1 校验结果被丢弃

- Python `EntityEventBridge._handle_change` 校验失败会写入环形日志（`events.py:86-94`）。
- Go `events.Bridge.HandleChange` 校验后 `_ = err`（`bridge.go:75-82`），既不阻断也不记录。
- 影响：越界写入静默通过（与 Python 一致，Python 也不阻断，只记录）；区别是无处查询。补一个 `event_log` MCP 工具或把最近校验错误挂在 `Bridge` 上暴露，S–M。

### C2 同名事件冲突日志

- Python `EventSink` 在「带条件 vs 无条件」同名事件覆盖时记 `_conflict_log`；Go `Bus.Register` 静默按规则处理（带条件优先），无记录。
- 影响纯可观测性；S。

### D1 配置覆盖面（架构一致性）

- 配置层目前只有 `state_group/state_definition_field/default_state_subgroup/safety_enabled`。
- 仍然硬编码：
  - 全局实体名 `场景/世界/时间/天气/全局`（`compiler/rewrite.go:15`、`parserRule.go:15`）；
  - emit 默认场景 `场景`、场景事件字段 `事件`（`compiler/ops.go:75,86`、`engine/ops.go:108-119`）；
  - 实体分类字段 `分类`（`entity/store.go:245`）。
- 影响：换世界模型（不同全局实体名/事件字段）必须改代码；当前数据完全使用默认名，所以不是 bug。
- 修复 M：`config.Config` 增加 `global_entities []string`、`default_scene`、`scene_event_field`、`classification_field`，按状态配置同样的注入与校验；默认值保持兼容。建议 P2，除非要做多世界。

### E1 HTTP 安全边界

- `-transport http` 直接 `http.ListenAndServe`，无 token、无 TLS、无来源限制（`main.go:75-80`）。任何能连到端口的人都能读写实体、应用脚本。
- 文档要求只绑 `127.0.0.1`，但代码不强制。
- 选项：
  1. 仅保留 stdio（S，风险最低）；
  2. 增加静态 token 中间件（M），从 `config.json`/环境变量读；
  3. 反向代理负责 TLS/鉴权（运维层，文档写明）。
- 若 HTTP 只在 localhost 用，风险可接受；建议至少把「不要暴露公网」写进启动日志。P1 视使用方式。

### F1 性能

- 重放 186 脚本约 4–5 s（集成测试整体耗时），Tick 在真实 1808 条规则上很快（测试级）。
- 规则匹配是线性扫描，实体写监听按实体数量线性 attach/detach（53 个实体，可忽略）。
- 只有在规则数/实体数大幅增长时才需要按 `match` 建索引；当前不值得优化。P3。

### G1 异盘备份（环境阻塞）

- `findmnt` 实测：`/mnt/bnsvm`（btrfs）、`/run/media/...` 三块盘全部 `ro`。
- 本地 `.local/backups/*.tar.zst` 已做 zstd 完整性 + 解包 `rsync --checksum` + sha256 校验，但同盘归档不满足「另一块物理盘」判据。
- 需要用户把任一盘 remount rw 或提供可写挂载点；命令已在 `docs/development.md` 给出。

## 3. 优先级矩阵

| 优先级 | 项 | 理由 | 工作量 | 风险 |
|---|---|---|---|---|
| **P0** | — | 无代码级 P0；G1 是环境阻塞 | — | — |
| **P1** | B1 模板持久化 | 真实数据目录差异，源文件缺失时丢模板 | S–M | 中低（注意 replay 不写盘） |
| **P1** | E1 HTTP 鉴权 | 暴露端口即全权限 | S（仅 stdio）/ M（token） | 中 |
| **P2** | B3 空 tick 日志 | 长跑噪声，修复极小 | S | 低 |
| **P2** | C1 校验日志 | 越界写入无迹可查 | S–M | 低 |
| **P2** | D1 配置扩展 | 多世界/改名的前提 | M | 中（跨 parser/compiler/engine/store） |
| **P3** | B2 键序、A3 format 规格、A2 冷门方法 | 无当前影响 | S | 低 |
| **不做** | A1 bytes/encode、任意对象方法、非引擎工具 | 值模型/用户决策 | — | — |

## 4. 建议实施顺序

**批次 1（P1，一次可验证）**
1. B3：空 tick 不写日志 + 测试。
2. B1：模板落盘（`AddTemplate/RemoveTemplate` 增加 persist 语义；replay 不写；apply/remove 写；测试覆盖「apply 写文件、replay 不重写、remove 删文件」）。
3. C1：`Bridge` 保存最近校验错误，并加一个 `event_log` 查询工具（或先只存内存 + 测试）。

**批次 2（P2，架构/安全）**
4. E1：若 HTTP 保留，加 `http_token` 配置与中间件；否则启动日志强调仅限 localhost。
5. D1：配置扩展全局实体/默认场景/事件字段/分类字段，默认值不变。

**批次 3（P3，可选）**
6. A3 format 规格；B2 有序 JSON 编码；A2 冷门方法（按实际脚本需求）。

## 5. 每个批次的验证方式

- B3：`TestStateLoggerSkipsEmptyTick`（0 transition 不产生文件行）。
- B1：临时 dataDir → `registry.Apply` → 断言 `templates/<type>.json` 存在；`ReplayAll` 前后文件哈希不变；`registry.Remove` 删除文件。
- C1：写入越界值 → `Bridge.LastValidation()` 有记录；MCP 工具可查询。
- E1：启动 HTTP → 无 token 请求 401/403（若实现 token）；或仅 stdio 的构建不注册 HTTP。
- D1：`config.Set` 自定义 `global_entities/default_scene` → 编译 `场景.事件`/emit 行为按新配置；默认配置回归全绿。

## 6. 为什么有些项「不做」

- **bytes/encode**：引入新值类型会波及比较、JSON、pyText、方法分派，当前语料零使用；投入产出比最低。
- **任意对象方法**：`.neko` 没有 class/对象定义，值只有内建类型；已覆盖 list 全量、dict（除 `fromkeys`）、str 常用集。继续补 `translate/format_map` 只是长尾。
- **非引擎工具**：用户明确排除；且多数绑定 AstrBot 运行时（事件、命令、LLM 上下文），MCP 化需要重新设计接口，不属于「补齐引擎」。

## 7. 风险提示

- B1 若实现不当会让「只读启动不写数据」的承诺失效（replay 也会调用 AddTemplate），必须用 persist 参数隔离，并在测试里断言重放前后文件哈希一致。
- D1 会让配置面扩大；所有新配置项必须默认等于现有硬编码值，否则现有 187 个脚本/数据会失配。
- E1 是安全边界，不是功能项；在未加鉴权前不要把 HTTP 端口暴露到非本机网络。

## 8. 附：非引擎工具清单（用户已排除，仅登记）

`trash`、`secrets`/`api_login`、`stats`（已迁移为 MCP `stats`）、`behavior`、`restart`、`emote`、`session_chat`、`event_repair`、`rule_derive`。其中 `stats` 已实现，其余多为 AstrBot 运行时绑定；若后续要做，应先定 MCP 语义（谁触发、权限、返回形态），不能直接照搬事件/命令签名。

## 9. 实施状态（2026-10-24 收尾）

| ID | 状态 | 说明 |
|---|---|---|
| B3 空 tick 日志 | **已完成** | `StateLogger.Log` 空转移/空冲突直接返回；`TestStateLoggerSkipsEmptyTick` |
| B1 模板落盘 | **已完成** | `SaveTemplateFile/DeleteTemplateFile`；registry 仅在 persist=true 时写/删；测试覆盖 apply/replay/remove |
| C1 校验日志 | **已完成** | Bridge 环形日志 + MCP `event_log` 工具（工具总数 13） |
| E1 HTTP 鉴权 | **已完成** | `http_token` Bearer；空 token 仅允许回环地址；中间件/回环测试 |
| D1 配置覆盖 | **已完成** | `global_entities/default_scene/scene_event_field/classification_field`；parser/compiler/engine/entity 改读配置 |
| A3 format 规格 | **已完成** | `!r`、`:.2f`、`:>5`、`:05d`、`:^7` 等 |
| B2 scripts.json 键序 | **已完成** | 加载时记录顺序，保存时保留；新增条目追加 |
| A2 冷门方法 | 未做（P3） | `translate/format_map/fromkeys` 依赖映射/类模型；不做 |
| A1 bytes/encode | 不做 | 值模型无 bytes |
| 非引擎工具 | 不做 | 用户决策 |
| G1 异盘备份 | 阻塞 | 目标盘全部只读 |
