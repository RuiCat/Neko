# 测试交互历史

当前提交：`b0aa4cb`（猫娘底层状态机）。环境：Go 1.27.3 linux/amd64；数据与脚本使用本地 `data/` + `scripts/`。

## 1. 测试层级与固定命令

| 层 | 命令 | 覆盖 | 当前结果 |
|---|---|---|---|
| 单元/集成 | `make test` | 11 个包、词法/编译/实体/引擎/事件/函数/注册表/安全/配置/MCP | 全绿 |
| 真实 E2E | `go test ./cmd/astrbox-mcp/ -v` | 真实二进制 + stdio MCP 协议、HTTP token、脚本生命周期 | 全绿（~18s） |
| 静态检查 | `make lint` | `go vet` + `gofmt` | 通过 |
| 模块化机检 | `lintCodeModularity.sh internal cmd --naming --comments` | 文件长度/命名/注释 | 93 文件，0 违规 0 告警 |

## 2. 真实 MCP stdio 交互序列

测试由 `TestMain` 构建真实二进制，SDK `CommandTransport` 拉起子进程；数据为仓库副本，不触碰工作区。

| # | 客户端调用 | 服务端实际结果 |
|---|---|---|
| 1 | 启动 `astrbox-mcp -data-dir <tmp> -scripts-dir <tmp>` | 加载 53 实体、15 模板，重放 186 脚本 → 1808 条规则 |
| 2 | `initialize` / `initialized`（SDK 握手） | 协议建立 |
| 3 | `tools/list` | 返回 13 个工具 |
| 4 | `inventory` | `entity_count=53`，模板 15，脚本 186 |
| 5 | `stats` | `rules=1808`，含优先级分布 |
| 6 | `entity_state_get {猫猫, 身体.体力}` | 返回数值 |
| 7 | `entity_state_set {猫猫, 身体.体力, 值-25}` | 写入成功；`@事件:体力管理` 触发 `@函数 体力管理` |
| 8 | `entity_state_get {猫猫, 状态.身体}` | 返回 `疲惫`（验证事件链真实生效） |
| 9 | `state_machine_tick {scope:"*"}` | 返回非空 `transitions`（真实数据 33 条） |
| 10 | `entity_snapshot {name:"e2e_snapshot"}` | 写入 `data/archives/e2e_snapshot.json` |
| 11 | `event_log {limit:5}` | 返回 `validations`/`events` JSON |
| 12 | `neko_script_apply {e2e_script, ...}` | `ok=true`；`templates/测试实体.json` 落盘 |
| 13 | 重启子进程（新会话） | 重放成功；`neko_script_list` 仍含 `e2e_script` |
| 14 | `neko_script_remove {e2e_script}` | 移除成功；模板文件被删除 |

> 注：`entity_state_set` 返回人类可读文本（`✅ 已设置 ...`），测试用文本断言；其余工具返回 JSON。

## 3. 真实 HTTP 交互

| 请求 | 结果 |
|---|---|
| `POST /mcp` 无 token | `401 Unauthorized` |
| `POST /mcp` + `Authorization: Bearer secret` | 非 401（进入 MCP 处理） |
| 启动 `-addr 0.0.0.0:...` 且无 token | 进程拒绝启动（非 0 退出，日志含“拒绝在非回环地址…”） |
| `-addr 127.0.0.1:...` 且无 token | 允许启动，日志警告“HTTP 无鉴权” |

## 4. 历次验证轮次（按 `60-verification.md`）

| 轮次 | 主题 | 关键结果 |
|---|---|---|
| 1–7 | Go 重写基线 | 包全绿；262 个 `.neko` 与 Python 逐值一致；186/186 重放；1808 规则；MCP 联调 |
| 8 | 函数执行器 + 事件总线 | 15 个语料函数可执行；体力 -25 → 事件 → `状态.身体=疲惫` |
| 11 | 冲突仲裁落盘 + 重放只读 | `llm_decisions.json`、`learned_*` 回填；重放不改 `scripts.json` |
| 13 | 规则 id 对齐 Python | 186 脚本逐 id 一致（含 `then: []` 兜底分支） |
| 15 | 基础配置层 | 状态组/定义字段/默认子组可配置，默认回归 |
| 17 | 安全层 + tick 快照 | 可选 R18G 词表；日志含 `snapshots` |
| 19 | stats + 实体快照 | 新增 `stats`、`entity_snapshot`（工具数 12） |
| 21 | 解释器边界 | while、链式比较、文本/映射 for、切片步长 |
| 23 | map 字面量/方法 | map 字面量 + list/text/map 方法 |
| 25 | 推导/内置 | 列表推导、range/enumerate/zip/sum/sorted/reversed |
| 27 | try/解包/严格类型 | try/except、元组赋值、Python 类型错误语义、跨实体冲突 |
| 29 | break/continue/finally | 循环控制 + finally 返回覆盖 |
| 31 | 类型化 except | `值错误/类型错误/名称错误`；更多方法；隐式写冲突 |
| 33 | 条件算术 | `变化量 + 1 > 0` 等可判定 |
| 35 | 方法补全 | 文本方法集、dict pop/popitem |
| 37 | str.format | `{}`/`{0}`/`{{`/`}}` |
| 39 | P1/P2 实施 | 空 tick 不写日志、模板落盘、event_log、HTTP token、配置扩展 |
| 41 | P3 | format 规格、scripts.json 键序保留 |
| 43 | 真实 E2E | 13 工具、事件链、脚本 apply/重启/remove、HTTP 鉴权 |
| 45 | 脚本/数据分离 | `scripts/` + `data/` + `-scripts-dir` |
| 47 | 运行数据不提交 | `data/` 运行文件 gitignore；注册表缺失时从脚本引导 |
| 49 | 脚本归档 + 用户实体 | `core/behavior/auto/...`；NPC → `data/user/`（不发布） |

备份快照每轮记录在 `60-verification.md` 的偶数小节（第 10/12/14/…/50 节）。

## 5. 测试中发现并修复的问题

| 现象 | 根因 | 修复 |
|---|---|---|
| Python 与 Go 规则 id 不一致 | canonical 使用 Python `repr`，Go 结构体格式化不同 | `pyRepr` + `canonicalRule`；补 `then: []` 标记 |
| 事件不触发 | `AddTemplate` 覆盖导致 `__annotations__` 丢失 | 改为深合并模板 |
| 体力骤降未改状态 | 函数执行器/事件桥未接 | 新增 `internal/function`、`internal/events` |
| `更新可达` 在 Go 返回字符串 | Python 对 `字符串+列表` 抛 TypeError | 严格运算符语义，测试改为断言 TypeError |
| `scripts.json` 每次启动被重写 | 重放持久化 | `persist=false` 重放；后续保留键序 |
| 空 tick 刷日志 | 无转移也写 | `StateLogger` 空转移/冲突直接返回 |
| 模板 apply 后不落盘 | Store 只改内存 | `SaveTemplateFile/DeleteTemplateFile` + persist 控制 |
| 配置只覆盖状态词汇 | 全局实体/场景/分类硬编码 | 扩展 config 并贯穿 parser/compiler/engine/entity |
| 用户数据可能被提交 | `data/` 被跟踪 | untrack + gitignore；`data/user/` 不发布；bootstrap 兜底 |

## 6. 当前验证快照（本轮实测）

```text
$ go test ./... -count=1
ok  github.com/ruicat/astrbox/cmd/astrbox-mcp        18.6s
ok  github.com/ruicat/astrbox/internal/compiler       0.006s
ok  github.com/ruicat/astrbox/internal/config         0.004s
ok  github.com/ruicat/astrbox/internal/engine         0.115s
ok  github.com/ruicat/astrbox/internal/entity         0.013s
ok  github.com/ruicat/astrbox/internal/events         0.005s
ok  github.com/ruicat/astrbox/internal/function       0.021s
ok  github.com/ruicat/astrbox/internal/mcpserver      0.018s
ok  github.com/ruicat/astrbox/internal/neko           0.099s
ok  github.com/ruicat/astrbox/internal/registry       5.028s
ok  github.com/ruicat/astrbox/internal/safety         0.007s

$ go test ./cmd/astrbox-mcp/ -run TestMCPStdioEndToEnd -v
--- PASS: TestMCPStdioEndToEnd (8.90s)

$ make lint            # OK
模块化机检：93 文件，0 违规 0 告警
```

## 7. 复算方式

```bash
make test
go test ./cmd/astrbox-mcp/ -v
make lint
bash ~/.dsh/skills/code-modularity/scripts/lintCodeModularity.sh internal cmd --naming --comments
```

E2E 使用 `t.TempDir()` 副本；仓库 `data/`、`scripts/` 不会被测试写坏。
