# 验证记录（2026-10-24）

环境：Go 1.27.3 linux/amd64；`GOCACHE=$PWD/.local/go-cache`；数据目录 `data/`（186 个注册脚本，262 个 `.neko` 文件）。

## 1. 全量测试

```text
$ make test
ok  github.com/ruicat/astrbox/internal/compiler
ok  github.com/ruicat/astrbox/internal/engine
ok  github.com/ruicat/astrbox/internal/entity
ok  github.com/ruicat/astrbox/internal/mcpserver
ok  github.com/ruicat/astrbox/internal/neko
ok  github.com/ruicat/astrbox/internal/registry
```

## 2. 解析器语料

```text
$ go test ./internal/neko/ -run TestCorpus -v
--- PASS: TestCorpus (0.06s)
```

断言：262 个 `.neko`；260 个零错误（2 个与 Python 同样 JSON 报错）；`data_defs=54`、`rule_defs=1901`、`event_defs=5`、`func_defs=15`；与 Python 逐值比对 262/262 无差异（子代理执行，方法见 00-scope）。

## 3. 重放与真实 Tick

```text
$ go test ./internal/registry/ -run TestReplayAllScripts -v
    integration_test.go:49: tick: transitions=33 conflicts=0 rules=1808
--- PASS: TestReplayAllScripts (4.24s)
```

- 注册脚本 186，`ReplayAll` 失败 0。
- 去重后规则 1808 条；逐脚本规则数与 Python `compile_script` 对照 0 差异（引用合计 1825）。
- 用真实 `entities.json` 执行 `Tick("*")`：33 条 transition、0 conflict，结果可 JSON 序列化。

## 4. MCP 联调

```text
$ go test ./internal/mcpserver/ -v
--- PASS: TestToolListAndInventory
```

使用 MCP SDK 的 in-memory transport 起服务、客户端 `ListTools` 校验 10 个工具全部注册，并实际调用 `entity_state_get`（读 `猫猫.身体.体力`）与 `inventory`。

另外以真实二进制启动：

```text
$ ./bin/astrbox-mcp -data-dir /tmp/astrtest -transport stdio </dev/null
astrbox: 实体 53, 模板 15, 规则 1808, 脚本 186 (replayed=186 skipped=0 failed=0)
```

## 5. 代码质量

```text
$ make lint
go vet ./...        # 无输出
gofmt -l cmd internal   # 无输出

$ lintCodeModularity.sh internal cmd --naming --comments
扫描 41 个源码文件；结果：0 项违规，0 项告警
```

## 6. 数据完整性

- 仓库 `data/scripts.json` 保持迁移前内容（md5 `8e4695ea4d5573f75ac579cfcb9bf989`），未在验证中被改写。
- 所有会写入的测试使用 `t.TempDir()` 或 `/tmp` 副本。
- 迁移前另存 `/home/ruicat/astrbox/scripts.json` 原始副本（md5 相同）。

## 7. 未验证 / 未做

- 函数执行器、事件总线、LLM 决策回填：未实现，无对应验收。
- HTTP 传输未做真实客户端联调（仅 stdio 与 in-memory 验证）。
- 备份到第二块物理盘：目标盘均只读，未完成（见最终回复）。

## 8. 第二轮：函数执行器 + 事件总线（2026-10-24 下午）

```text
$ go test ./... -count=1
ok  github.com/ruicat/astrbox/internal/compiler
ok  github.com/ruicat/astrbox/internal/engine
ok  github.com/ruicat/astrbox/internal/entity
ok  github.com/ruicat/astrbox/internal/events
ok  github.com/ruicat/astrbox/internal/function
ok  github.com/ruicat/astrbox/internal/mcpserver
ok  github.com/ruicat/astrbox/internal/neko
ok  github.com/ruicat/astrbox/internal/registry
```

端到端证据（`internal/registry/integration_test.go`）：

```text
tick: transitions=33 conflicts=0 rules=1808
时段判断("2026-04-01T15:00") = "下午"
猫猫.身体.体力 -= 25  →  @事件:体力管理 → @函数 体力管理  →  状态.身体 = "疲惫"
```

- 15 个语料 `@函数` 全部注册并可执行（`internal/function` 测试从真实 `.neko` 取函数体）。
- 事件桥从模板 `__annotations__` 构建 schema；`Store.AddTemplate` 改为深合并（与 Python 一致），避免后续脚本覆盖掉早先模板的注解。
- `Tick` 经 MCP 调用时写每日日志 `<dataDir>/logs/state_YYYY-MM-DD.log`，保留 7 天。

## 9. 仍然未实现

- LLM 决策落盘与规则回填（`ResolveConflict` 只改状态）。
- 定时状态快照（只有 JSONL 日志）。
- Python 侧非引擎工具（trash/secrets/stats/behavior/restart/emote/session_chat/event_repair/rule_derive）。

## 10. 备份状态（2026-10-24 晚）

- 本地归档：`.local/backups/astrbox-20261024T064820Z.tar.zst`（954 文件）。
- 校验：`zstd -t` 通过；解包后 `rsync -an --checksum`（同排除项）无差异；`sha256sum -c` 通过。
- 异盘目标：`/mnt/bnsvm`、两块 `/run/media` NVMe 盘均只读（`ro`），未完成异盘备份；需先 remount rw 或提供可写盘。

## 11. 第三轮：冲突仲裁落盘 + 重放不写数据（2026-10-24）

- `ResolveConflictWithEffects`：写目标状态 → 追加 `llm_decisions.json` → 回填 `learned_<conflict_id>` 规则（`source=llm_backfill`，优先级原规则 +100）。
- 启动重放改为 `persist=false`：只重建内存规则/函数/事件，**不重写 `scripts.json`**。
- 函数参数里的 `{e}.` 路径（编译器把 `$[NPC].` 重写而来）已由引擎 resolver 解析。

测试证据：

```text
$ go test ./internal/engine/ -run 'TestResolveConflict|TestFuncCallerWithCurrentEntityArg' -v
--- PASS: TestResolveConflictLogsAndBackfills
--- PASS: TestFuncCallerWithCurrentEntityArg

$ go test ./internal/registry/ -run TestReplayAllScripts -v
    integration_test.go:...: tick: transitions=33 conflicts=0 rules=1808
--- PASS: TestReplayAllScripts
```

`TestReplayAllScripts` 额外断言：重放前后 `scripts.json` 字节一致；编译测试断言 `危险度($[NPC].身体.体力) > 50` → `危险度({e}.身体.体力) > 50`。

## 12. 备份快照（2026-10-24 14:59 UTC）

- 归档：`.local/backups/astrbox-20261024T065954Z.tar.zst`（988 文件，1.3M）。
- 校验：`zstd -t` 通过；解包后 `rsync -an --checksum` 无差异；`sha256sum -c` 通过。
- 只读启动实测：启动前后 `scripts.json` sha256 相同（`ba206e71…`），确认重放不写数据。
- 异盘备份仍受阻：`/mnt/bnsvm` 与两块 `/run/media` NVMe 盘均为 `ro`。

## 13. 第四轮：规则 id 与 Python 对齐（2026-10-24）

- 新增 `internal/compiler/pyRepr.go`：复刻 Python `repr`（字符串引号选择/转义、浮点、list/dict、op 字段顺序），`canonicalRule` 生成与 Python 完全相同的哈希输入。
- `engine.Branch` 增加 `ThenPresent` 标记（仅影响 canonical），复刻自动兜底分支的 `{'else': True, 'then': []}`。
- 夹具 `internal/compiler/testdata/python_rule_ids.json` 由 Python `compile_script` 生成（186 脚本，1825 条规则引用）。

```text
$ go test ./internal/compiler/ -run TestRuleIDsMatchPython -v
--- PASS: TestRuleIDsMatchPython (0.08s)
```

结果：186/186 脚本的规则 id 集合与 Python 完全一致；显式 apply/remove 落盘不再改变 `scripts.json` 的 `rules` 键。

## 14. 备份快照（2026-10-24 15:20 UTC）

- 归档：`.local/backups/astrbox-20261024T072041Z.tar.zst`（1014 文件，1.3M）。
- 校验：`zstd -t` 通过；`rsync -an --checksum` 无差异；`sha256sum -c` 通过。
- 异盘备份仍受阻（目标盘只读），详见第 10 节。

## 15. 第五轮：基础配置层（2026-10-24）

- 新增 `internal/config`：`state_group` / `state_definition_field` / `default_state_subgroup`，默认值与旧行为一致；支持 `<dataDir>/config.json` 或 `-config` 覆盖，缺失文件回落默认值，非法名称拒绝。
- 替换点：engine 的 Tick/冲突/状态快照、compiler 的 `状态:` 重写与裸字段默认分组、parser 的分支状态糖、effects/assign 别名表、MCP `entity_query` 输出键。
- 默认配置下全量测试与规则 id 对照保持不变；自定义状态组有独立测试。

```text
$ go test ./internal/config/ ./internal/engine/ ./internal/compiler/ ./internal/neko/ -count=1
ok  github.com/ruicat/astrbox/internal/config
ok  github.com/ruicat/astrbox/internal/engine
ok  github.com/ruicat/astrbox/internal/compiler
ok  github.com/ruicat/astrbox/internal/neko
```

新增测试：`TestConfiguredStateGroup`（状况.身体 冲突/仲裁）、`TestConfiguredStateVocabulary`（状态条件与裸字段映射）、`TestConfiguredStateSugar`（分支 `状况 =` 糖）、`TestLoadMergesDefaults`。

## 16. 备份快照（2026-10-24 15:44 UTC）

- 归档：`.local/backups/astrbox-20261024T074440Z.tar.zst`（1060 文件，1.4M）。
- 校验：`zstd -t`、`rsync -an --checksum` 无差异、`sha256sum -c` 通过。
- 异盘目标仍只读，未完成异盘备份（见第 10 节）。

## 17. 第六轮：安全层与 tick 快照（2026-10-24）

- `internal/safety`：移植 Python `safety.py` 词表；`config.safety_enabled` 默认 `false`（与 Python 默认一致）。开启后 `neko_script_apply` 扫描 AST/全文/脚本名，命中返回 `ok=false`（块 `R18G安全检查`）；重放路径 `skipSafety=true` 不拦截存量脚本。
- `StateLogger.Log` 增加 `snapshots`：MCP tick 时按转移实体记录 体力/身体状态/情绪，写入同一条 JSONL。
- 新增测试：`internal/safety`（默认放行、开启拦截、AST 扫描、文件名命中）、`registry.TestSafetyRejectsSensitiveScript`、`engine.TestStateLoggerWritesDailyJSONL`（snapshots 字段）。

```text
$ go test ./internal/safety/ ./internal/registry/ ./internal/engine/ -count=1
ok  github.com/ruicat/astrbox/internal/safety
ok  github.com/ruicat/astrbox/internal/registry
ok  github.com/ruicat/astrbox/internal/engine
```

## 18. 备份快照（2026-10-24 15:59 UTC）

- 归档：`.local/backups/astrbox-20261024T075910Z.tar.zst`（1092 文件，1.4M）。
- 校验：`zstd -t`、`rsync -an --checksum` 无差异、`sha256sum -c` 通过。
- 异盘目标仍只读（见第 10 节）。

## 19. 第七轮：stats 与实体快照（2026-10-24）

- 新增 MCP 工具 `stats`：实体总数/按类型、规则总数/启用数/优先级分布、模板数、脚本数（对应 Python `cat_stats`）。
- 新增 MCP 工具 `entity_snapshot`：把全部实体写入 `<dataDir>/archives/<name>.json`，返回路径/实体数/字节数；名称校验阻止路径穿越。
- `cmd/astrbox-mcp` 启动时注入 `data-dir` 给快照工具。

```text
$ go test ./internal/mcpserver/ -count=1 -v
--- PASS: TestToolListAndInventory   # 覆盖 stats 调用与 snapshot 落盘
```

工具总数 12；`make test`/`make lint`/模块化机检均为绿。

## 20. 备份快照（2026-10-24 16:11 UTC）

- 归档：`.local/backups/astrbox-20261024T081128Z.tar.zst`（1114 文件，1.4M）。
- 校验：`zstd -t`、`rsync -an --checksum` 无差异、`sha256sum -c` 通过。
- 异盘目标仍只读（见第 10 节）。

## 21. 第八轮：解释器语法边界（2026-10-24）

- 新增 `while`/`当` 循环（受 `MAX_EXEC_STEPS` 限制）。
- 链式比较按 Python 语义（`1 < a < b < c` = 逐对比较且短路）。
- `for` 支持列表、文本（逐字符）与映射（按键，排序稳定）。
- 切片支持步长：`[a:b:c]`，含负步长 `[::-1]`；负索引按 Python 规则归一化。

```text
$ go test ./internal/function/ -run 'TestChained|TestWhile|TestForOver|TestSliceStep' -v
--- PASS: TestChainedComparison
--- PASS: TestWhileLoop
--- PASS: TestForOverStringAndMap
--- PASS: TestSliceStep
```

仍未支持：map 字面量、`try`、推导式、任意方法；`+` 对非数值仍做字符串拼接、`int()` 非法值仍回 0（与 Python 的异常语义不同，已在 parity 注明）。

## 22. 备份快照（2026-10-24 17:07 UTC）

- 归档：`.local/backups/astrbox-20261024T090710Z.tar.zst`（1137 文件，1.4M）。
- 校验：`zstd -t`、`rsync -an --checksum` 无差异、`sha256sum -c` 通过。
- 异盘目标仍只读（见第 10 节）。

## 23. 第九轮：map 字面量与常用方法（2026-10-24）

- 解释器新增 map 字面量 `{"键": 值, ...}`（键为文本或裸标识符），与 `{e}` 占位区分。
- 新增常用 list/text/map 方法（append/extend/insert/remove/pop/clear/count/index/join/split/strip/replace/lower/upper/startswith/endswith/keys/values/items/get）。
- `join` 按 Python 语义以目标为分隔符、参数为列表；文本 `index` 返回字符索引。

```text
$ go test ./internal/function/ -run 'TestMapLiteral|TestListMethods|TestTextAndMapMethods' -v
--- PASS: TestMapLiteral
--- PASS: TestListMethods
--- PASS: TestTextAndMapMethods
```

## 24. 备份快照（2026-10-24 17:12 UTC）

- 归档：`.local/backups/astrbox-20261024T091221Z.tar.zst`（1159 文件，1.4M）。
- 校验：`zstd -t`、`rsync -an --checksum` 无差异、`sha256sum -c` 通过。
- 异盘目标仍只读（见第 10 节）。

## 25. 第十轮：列表推导与内置函数（2026-10-24）

- 解释器新增列表推导 `[表达式 for 变量 in 列表 if 条件]`（步数上限沿用 `MAX_EXEC_STEPS`）。
- 新增内置函数：`range`（最多 10 万项）、`enumerate`、`zip`、`sum`、`sorted`、`reversed`。

```text
$ go test ./internal/function/ -run 'TestListComprehension|TestExtraBuiltins' -v
--- PASS: TestListComprehension
--- PASS: TestExtraBuiltins
```

## 26. 备份快照（2026-10-24 17:17 UTC）

- 归档：`.local/backups/astrbox-20261024T091730Z.tar.zst`（1182 文件，1.5M）。
- 校验：`zstd -t`、`rsync -an --checksum` 无差异、`sha256sum -c` 通过。
- 异盘目标仍只读（见第 10 节）。

## 27. 第十一轮：try/解包/严格类型/跨实体冲突（2026-10-24）

- 解释器：`try { ... } except [变量] { ... }`；`for i, v in ...` 多变量解包；元组赋值 `a, b = 1, 2`。
- 运算符按 Python 类型规则：`+` 仅接受数字/文本/列表同类组合，字符串+列表报错；`- * / %` 要求数字；`int()/float()` 非法输入报错（不再静默回 0）。
- `branch_race` 跟踪扩展到显式 `entity`/`subject` 目标（跨实体写同一声明状态组会报冲突）。
- 语料测试同步：`更新可达` 的 `字符串 + 列表` 现按 Python TypeError 断言（赋值副作用仍保留）。

```text
$ go test ./internal/function/ ./internal/engine/ -count=1
ok  github.com/ruicat/astrbox/internal/function
ok  github.com/ruicat/astrbox/internal/engine
```

## 28. 备份快照（2026-10-24 17:31 UTC）

- 归档：`.local/backups/astrbox-20261024T093132Z.tar.zst`（1211 文件，1.5M）。
- 校验：`zstd -t`、`rsync -an --checksum` 无差异、`sha256sum -c` 通过。
- 异盘目标仍只读（见第 10 节）。

## 29. 第十二轮：break/continue 与 finally（2026-10-24）

- 循环支持 `break` / `continue`（`for` 与 `while`），用哨兵错误传播，`try` 不会误捕获。
- `try` 支持 `except` 与/或 `finally`；`finally` 总会执行，`finally` 中的 `返回` 覆盖原返回值。
- 新增测试：`TestBreakContinue`、`TestTryFinally`（返回值覆盖 + 副作用）。

```text
$ go test ./internal/function/ -run 'TestBreakContinue|TestTryFinally' -v
--- PASS: TestBreakContinue
--- PASS: TestTryFinally
```

## 30. 备份快照（2026-10-24 17:46 UTC）

- 归档：`.local/backups/astrbox-20261024T094647Z.tar.zst`（1230 文件，1.5M）。
- 校验：`zstd -t`、`rsync -an --checksum` 无差异、`sha256sum -c` 通过。
- 异盘目标仍只读（见第 10 节）。

## 31. 第十三轮：类型化 except、更多方法、隐式写入冲突（2026-10-24）

- 解释器错误带 kind（value/type/name）；`except 值错误/类型错误/名称错误 as e { ... }` 按类型捕获，不匹配则继续抛出；`Exception/错误` 捕获全部。
- 新增方法：`sort/reverse/copy/update/setdefault/find/rfind/title/capitalize/isdigit/isalpha/isnumeric`。
- `branch_race` 改用 tick 级实体写监听：规则 ops、`@函数`、`@事件` 对状态组的写入都参与冲突检测（同一 tick、已声明状态组）。

```text
$ go test ./internal/function/ ./internal/engine/ -count=1
ok  github.com/ruicat/astrbox/internal/function
ok  github.com/ruicat/astrbox/internal/engine
```

## 32. 备份快照（2026-10-24 18:01 UTC）

- 归档：`.local/backups/astrbox-20261024T100144Z.tar.zst`（1262 文件，1.5M）。
- 校验：`zstd -t`、`rsync -an --checksum` 无差异、`sha256sum -c` 通过。
- 异盘目标仍只读（见第 10 节）。

## 33. 第十四轮：事件条件算术（2026-10-24）

- 条件求值新增算术操作数解析（`+ - * / %`、括号、一元负号、数字/文本/路径/函数调用回退）。
- 事件条件如 `变化量 + 1 > 0` 现在可判定；规则条件同样受益。
- 新增测试：`TestArithmeticConditionOperands`、`TestEventConditionArithmetic`。

```text
$ go test ./internal/engine/ ./internal/events/ -count=1
ok  github.com/ruicat/astrbox/internal/engine
ok  github.com/ruicat/astrbox/internal/events
```

## 34. 备份快照（2026-10-24 18:05 UTC）

- 归档：`.local/backups/astrbox-20261024T100558Z.tar.zst`（1283 文件，1.5M）。
- 校验：`zstd -t`、`rsync -an --checksum` 无差异、`sha256sum -c` 通过。
- 异盘目标仍只读（见第 10 节）。

## 35. 第十五轮：内建类型方法补全（2026-10-24）

- 文本方法拆分到 `methodsText.go`（保持单文件 <500 有效行），补齐 `rsplit/splitlines/count/index/rindex/ljust/rjust/center/zfill/partition/rpartition/removeprefix/removesuffix/swapcase/isalnum/isdecimal/isspace/islower/isupper/casefold/istitle/isascii/isprintable/expandtabs`。
- 映射补 `pop/popitem`；list 方法已全量。
- 新增测试覆盖 `index` 未命中（值错误）、`zfill/rsplit/partition/count`、`pop/popitem`、`istitle/isascii/expandtabs`。

```text
$ go test ./internal/function/ -count=1
ok  github.com/ruicat/astrbox/internal/function
```

## 36. 备份快照（2026-10-24 18:23 UTC）

- 归档：`.local/backups/astrbox-20261024T102327Z.tar.zst`（1302 文件，1.5M）。
- 校验：`zstd -t`、`rsync -an --checksum` 无差异、`sha256sum -c` 通过。
- 异盘目标仍只读（见第 10 节）。

## 37. 第十六轮：str.format（2026-10-24）

- 文本方法新增 `format`：支持位置字段 `{}`、显式下标 `{0}`/`{1}`、转义 `{{`/`}}`；未匹配字段原样保留。
- 测试：`TestFormatMethod`（自动下标、显式下标、转义）。

```text
$ go test ./internal/function/ -run TestFormatMethod -v
--- PASS: TestFormatMethod
```

## 38. 备份快照（2026-10-24 18:26 UTC）

- 归档：`.local/backups/astrbox-20261024T102646Z.tar.zst`（1319 文件，1.6M）。
- 校验：`zstd -t`、`rsync -an --checksum` 无差异、`sha256sum -c` 通过。
- 异盘目标仍只读（见第 10 节）。

## 39. 第十七轮：按分析顺序实现 P1/P2（2026-10-24）

- B3：空 tick（无 transitions/conflicts）不再写日志；新增 `TestStateLoggerSkipsEmptyTick`。
- B1：模板落盘 —— `Store.SaveTemplateFile/DeleteTemplateFile`；registry 仅在 `persist=true` 时写/删，replay 不写。测试覆盖 apply 写文件、replay 哈希不变、remove 删文件。
- C1：事件桥记录 schema 校验失败（环形 200 条）；新增 MCP `event_log` 工具（校验失败 + 最近事件记录，工具总数 13）。
- E1：HTTP 加 `http_token` Bearer 鉴权；token 为空时只允许回环地址，否则拒绝启动。新增中间件/回环判定测试。
- D1：配置扩展 `global_entities/default_scene/scene_event_field/classification_field`，parser/compiler/engine/entity 全部改读配置；默认值回归不变，新增自定义全局/场景/分类测试。

```text
$ go test ./... -count=1
# 全部包通过
```

## 40. 备份快照（2026-10-24 18:49 UTC）

- 归档：`.local/backups/astrbox-20261024T104930Z.tar.zst`（1380 文件，1.6M）。
- 校验：`zstd -t`、`rsync -an --checksum` 无差异、`sha256sum -c` 通过。
- 异盘目标仍只读（见第 10 节）。

## 41. 第十八轮：P3 format 规格与 scripts.json 键序（2026-10-24）

- A3：`str.format` 支持 `!r`、填充/对齐/宽度、精度与 `d/f` 类型（`{:.2f}`、`{:>5}`、`{:05d}`、`{:^7}`）。
- B2：registry 加载时记录 `scripts` 键顺序，保存时保留，新增条目追加；`TestScriptOrderPreserved` 覆盖 b→a→c 顺序。
- 测试：`TestFormatMethod` 覆盖新增规格。

```text
$ go test ./internal/function/ ./internal/registry/ -count=1
ok  github.com/ruicat/astrbox/internal/function
ok  github.com/ruicat/astrbox/internal/registry
```

## 42. 备份快照（2026-10-24 19:01 UTC）

- 归档：`.local/backups/astrbox-20261024T110145Z.tar.zst`（1404 文件，1.6M）。
- 校验：`zstd -t`、`rsync -an --checksum` 无差异、`sha256sum -c` 通过。
- 异盘目标仍只读（见第 10 节）。

## 43. 第十九轮：真实 MCP 端到端测试（2026-10-24）

新增 `cmd/astrbox-mcp/e2e_test.go`（TestMain 先构建真实二进制，再用 SDK CommandTransport 起子进程）：

- stdio：13 个工具；inventory/stats/entity_state_get；`entity_state_set` 后事件处理器把 `状态.身体` 改为 `疲惫`；tick 有 transitions；entity_snapshot 落盘；event_log 返回 JSON。
- 脚本生命周期：apply 后 `templates/测试实体.json` 存在；重启后脚本仍在；remove 后模板文件删除。
- HTTP：带 token 启动，无 token 返回 401，Bearer token 通过；`0.0.0.0` 无 token 启动被拒绝（退出码非 0 + 明确错误）。

```text
$ go test ./cmd/astrbox-mcp/ -count=1 -v
--- PASS: TestMCPStdioEndToEnd (8.6s)
--- PASS: TestMCPHTTPAuth (4.4s)
--- PASS: TestHTTPRefusesNonLoopbackWithoutToken (4.2s)
--- PASS: TestTokenMiddleware
--- PASS: TestIsLoopbackAddr
```

## 44. 备份快照（2026-10-24 19:15 UTC）

- 归档：`.local/backups/astrbox-20261024T111544Z.tar.zst`（1420 文件，1.6M）。
- 校验：`zstd -t`、`rsync -an --checksum` 无差异、`sha256sum -c` 通过。
- 异盘目标仍只读（见第 10 节）。

## 45. 第二十轮：脚本与数据目录分离（2026-10-24）

- `scripts/`：`.neko` 源文件（含 `derivations/`、`repairs/`）与可选 `rules.json`。
- `data/`：`entities.json`、`scripts.json`（注册表）、`templates/`、`archives/`、`logs/`、`llm_decisions.json`。
- 新增 `-scripts-dir` / `ASTRBOX_SCRIPTS_DIR`；`-config` 默认当前目录 `config.json`。
- registry 读取 `data/scripts.json`，源文件从 `scripts/` 读；模板与快照仍写 `data/`。
- 测试、E2E、Makefile、README、ARCHITECTURE、development、config、MCP 文档与 `.gitignore` 全部更新；清理 legacy 的 `__pycache__/.pytest_cache`。

```text
$ make test
ok  github.com/ruicat/astrbox/cmd/astrbox-mcp (E2E)
ok  ... (其余 10 包)
$ make lint  # OK
模块化机检：91 文件，0 违规 0 告警
```

## 46. 备份快照（2026-10-24 19:33 UTC）

- 归档：`.local/backups/astrbox-20261024T113311Z.tar.zst`（1456 文件，1.7M）。
- 排除项：`.local/`、`bin/`、`data/logs/`、`data/llm_decisions.json`。
- 校验：`zstd -t`、`rsync -an --checksum` 无差异、`sha256sum -c` 通过。
- 异盘目标仍只读（见第 10 节）。

## 47. 第二十一轮：运行数据不提交（2026-10-24）

- `data/` 的运行数据（entities.json、scripts.json、templates/、archives/）从 git 索引移除并加入 `.gitignore`；本地文件保留。
- 新增 `data/README.md` 说明本地状态与备份方式。
- 新增 `registry.BootstrapFromSources`：注册表缺失时启动扫描 `scripts/*.neko` 引导注册，写入本地 data；`TestBootstrapFromSources` 覆盖且断言不重写源文件。
- 启动日志会报告引导数量；README/ARCHITECTURE/development 同步说明。

```text
$ make test   # 全绿（含 E2E）
$ make lint   # OK
```

## 48. 备份快照（2026-10-24 19:42 UTC）

- 归档：`.local/backups/astrbox-20261024T114210Z.tar.zst`（1482 文件，1.7M，含本地 data 运行状态）。
- 排除：`.local/`、`bin/`、`data/logs/`、`data/llm_decisions.json`。
- 校验：`zstd -t`、`rsync -an --checksum` 无差异、`sha256sum -c` 通过。
- 异盘目标仍只读（见第 10 节）。

## 49. 第二十二轮：脚本按职责归档 + 用户实体下放（2026-10-24）

- `scripts/` 顶层 187 个脚本按职责移入 `core/ behavior/ auto/ r18/ rules/ scene/ meta/`；`derivations/`、`repairs/` 保留。注册名仍为 basename（262 个脚本无重名）。
- registry 增加递归源索引：replay/bootstrap/remove/apply 都按源索引定位，新增脚本仍写根目录。
- `data/entities.json` 只保留环境实体（50 个）；NPC 角色实体（猫猫/梦梦等 3 个）写入 `data/user/entities.json`，`/data/user/` 已 gitignore，不发布；Store 加载合并、保存自动拆分，启动时迁移旧合并文件。
- `data/user/` 不进 git 与发布包，但本地备份包含；新增 `TestUserEntitiesSplit`、`TestMigrateUserEntitiesFromLegacy`。
- 测试语料路径、E2E 拷贝、README/development/ARCHITECTURE 全部更新。

```text
$ make test   # 全绿（含 E2E）
$ make lint   # OK
模块化机检：93 文件，0 违规 0 告警
```

## 50. 备份快照（2026-10-24 20:00 UTC）

- 归档：`.local/backups/astrbox-20261024T120044Z.tar.zst`（1525 文件，1.7M，含 `data/user/` 本地角色实体）。
- 排除：`.local/`、`bin/`、`data/logs/`、`data/llm_decisions.json`。
- 校验：`zstd -t`、`rsync -an --checksum` 无差异、`sha256sum -c` 通过。
- 异盘目标仍只读（见第 10 节）。

## 51. 测试交互历史文档与备份（2026-10-24 20:25 UTC）

- 新增 `runs/astrbox-go/test-interaction-history.md`（stdio 调用序列、HTTP 鉴权、历次轮次、问题修复、当前快照）。
- 本轮实测：`go test ./...` 11 包全绿（含 E2E 18.6s）；`make lint` OK；模块化 93 文件 0 违规 0 告警。
- 备份：`.local/backups/astrbox-20261024T122509Z.tar.zst`（912 文件，1.0M；zstd/rsync/sha256 通过）。
