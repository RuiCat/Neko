# 与 Python 版对照

Python 原实现（`legacy/python-astrbox/`）是行为基线。本文件记录 Go 版已对齐、未对齐与已变更的部分。

## 验证证据

均在 2026-10-24 的本机数据上实测：

| 项 | 结果 | 复算 |
|---|---|---|
| 全语料解析 | 262 个 `.neko`；260 个零错误（2 个与 Python 同样 JSON 报错，fail-soft 后规则仍解析） | `go test ./internal/neko/ -run TestCorpus -v` |
| AST 逐值比对 | 262/262 与 Python 无差异（主语、conditions、actions、branches、state_when、priority、pos、data、event/func） | 解析器迁移报告（2026-10-24），方法：`cp -a legacy/python-astrbox /tmp/astrbox` 后 Python 侧 dump JSON |
| 规则语料 | `data_defs=54`、`rule_defs=1901`、`event_defs=5`、`func_defs=15` | 同上 |
| 编译规则数 | 186 个注册脚本逐脚本规则数与 Python **0 差异**；引用合计 1825，去重后 1808 条 | `go test ./internal/registry/ -v` + Python `compile_script` 对照 |
| 重放 | 186/186 成功，0 failed / 0 skipped | 启动日志 |
| 真实 Tick | 加载 1808 条规则后 `Tick("*")`：33 条 transition、0 conflict | `go test ./internal/registry/ -run TestReplayAllScripts -v` |
| 函数 + 事件 | 15 个语料 `@函数` 全部可执行；设置 `猫猫.身体.体力 -= 25` 后 `@事件:体力管理` 触发，`状态.身体` 变为 `疲惫` | 同上（集成测试） |
| Go 测试 | `go test ./...` 全绿 | `make test` |
| 模块化机检 | `lintCodeModularity.sh internal cmd --naming --comments`：0 违规 0 告警 | `make lint` 之外单独执行 |

## 已对齐

- `.neko` 词法/语法：`@Data`/`@data`/`@规则`/`#规则`/`@事件`/`@func`/元数据、多主体、`@多规则`、阈值分档、块内多 `if`、legacy `key: value`。
- 编译：条件重写、裸字段映射、动作降级（set/add/append/emit）、分支与状态糖、注解展开、规则 id 显式指定。
- 规则 id：复刻 Python 的 canonical repr（`match|subjects|when|branches`，含自动兜底分支的 `then: []`），186 个注册脚本逐 id 与 Python 比对一致（夹具 `internal/compiler/testdata/python_rule_ids.json`）。
- 实体：模板继承链、点分路径、模板回退、选择器与 `key:`/字段查询、`entities.json` 原子保存。
- 状态机：条件表达式、效果文本、分支短路、多主体绑定、优先级、scope、模板 min/max 限幅、冲突收集与 `ResolveConflict`。
- 函数：变量、`if/else if/else`、`返回`、`for ... in`、列表 `.append`、字符串拼接/切片、`int()`、`in`/`contains`、函数互调、实体路径读写、`{e}`。
- 事件：模板注解 `@事件` → 字段变化 → 条件（`变化量`/`旧值`/`新值`）→ `@函数`；多脚本同名事件按「带条件优先」合并；深度上限 10。
- 日志：`Tick` 通过 MCP 工具调用时追加 `<dataDir>/logs/state_YYYY-MM-DD.log`（保留 7 天），并对每条记录做清理。
- 注册表：hash、引用计数、增量替换、源文件落盘、`scripts.json`、重启重放；**只读启动不写 `scripts.json`**。
- 冲突仲裁：`state_machine_resolve` 把目标状态写回实体，追加记录到 `<dataDir>/llm_decisions.json`；带 `side_effects` 时回填一条 `source=llm_backfill`、优先级 +100 的规则。
- 事件条件：支持比较、`∧/∨/!`、`in/contains` 与算术操作数（`变化量 + 1 > 0`、`(1+2)*3 == 9`）；不执行任意代码，语义比 Python `_safe_eval` 更窄但无代码执行面。
- 冲突检测：`branch_race` 通过 tick 级实体写监听覆盖规则 ops 与 `@函数/事件` 的隐式写入（同一 tick、声明状态组）。
- 数值输出：效果增量在内存里用 `float64` 计算，但 JSON 序列化对整数值输出一致（`90` 而非 `90.0`），字段类型与 Python 写回结果等价。
- 域词汇：状态组/状态定义字段/默认子组/全局实体/默认场景/场景事件字段/分类字段从 `internal/config` 读取，默认值（`状态`/`状态定义`/`身体`）与 Python 完全一致；可通过 `<dataDir>/config.json` 覆盖。
- 安全层：`safety_enabled=true` 时按 Python 同款词表扫描 AST/全文并拒绝；默认关闭（与 Python `SAFETY_ENABLED=False` 一致），重放路径跳过检查。
- 统计与快照：`stats` 对应 Python `cat_stats`；`entity_snapshot` 把全部实体写入 `archives/<name>.json`（Python 的「保存状态」只调用 store.save，Go 版额外提供显式归档文件）。

## 已变更（有意）

1. **Tick 不落盘**：Python `StateMachine.tick` 内部会 `store.save()`；Go 版由调用方保存（MCP 服务已代做），避免库使用者误写数据。
2. **`reply_hint`**：Python 在无变化时会把 hint 覆盖成固定文案（已知缺陷），Go 返回真实 hint。
3. **通配路径解析顺序**：Go 实体按 id 排序，Python 按插入顺序；选择器匹配到多个实体时的取值顺序可能不同。
4. **错误文本**：JSON 解码错误措辞随 Go 标准库；`ParseError` 暂不带 Python 的 `块:'...'` 字段。

## 未实现（明确缺口）

| 能力 | 状态 | 影响 |
|---|---|---|
| 函数解释器语法面 | Python 子集 | 已支持 `while`、`try/except/finally`、`break/continue`、链式比较、`for` 遍历列表/文本/映射键与多变量解包、元组赋值、切片步长 `[a:b:c]`、map 字面量、常用 list/text/map 方法、列表推导与 `range/enumerate/zip/sum/sorted/reversed`；算术与 `int()/float()` 按 Python 类型/异常语义（字符串+列表报错）。`except` 可用 `值错误/类型错误/名称错误 as 变量`（含 `Exception`）。内建类型方法：list 全量，dict 除 `fromkeys`，str 常用集（含 `casefold/istitle/expandtabs/rsplit/partition` 等）；`str.format` 支持位置字段 `{}`/`{0}`、`{{`/`}}`、`!r` 与常用规格（`:.2f`、`:>5`、`:05d`、`:^7`）；未支持 `str.encode/translate/format_map` 与 `dict.fromkeys`（无 bytes/类模型） |
| Python 侧非引擎工具 | 未迁移 | `trash`、`secrets`/`api_login`、`behavior`、`restart`、`emote`、`session_chat`、`event_repair`、`rule_derive` |

这些缺口在 MCP 工具层都不可见（没有注册对应工具），不会让客户端误以为可用。

## 已知数据特例

- `cat-repro-cycle-advance.neko`：`@Data` 体内 JSON 键未加引号，Python/Go 都报 JSON 错误；后者仍解析出 1 条规则。
- `cat_anatomy_eversion.neko`：`@Data` 体是 YAML 式文本，Python/Go 都报错；后者仍解析出 7 条规则。

两例均按 Python 的 fail-soft 语义处理，注册表把错误降级为 warning。
