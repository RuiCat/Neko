# 开发与测试

## 环境

- Go 1.25+（本机 1.27）。
- 本机默认 Go 构建缓存可能只读，`Makefile` 默认把 `GOCACHE`/`GOMODCACHE` 指到本仓库 `.local/`（已 gitignore）。手工执行时同理：

```bash
export GOCACHE=$PWD/.local/go-cache GOMODCACHE=$PWD/.local/gomodcache
```

## 常用命令

```bash
make build     # 构建 bin/astrbox-mcp
make test      # go test ./... -count=1
make lint      # go vet + gofmt 检查
make run       # 构建并以 stdio 启动（使用 data/ + scripts/）
make clean     # 删除 bin/
```

单独跑某个包：

```bash
go test ./internal/neko/ -run TestCorpus -v
go test ./internal/registry/ -run TestReplayAllScripts -v
go test ./internal/mcpserver/ -v
```

## 测试分层

| 包 | 测什么 | 是否依赖真实数据 |
|---|---|---|
| `internal/neko` | 词法/语法、全语料（262 个 `.neko`）计数与错误 | 是（`../../scripts`） |
| `internal/compiler` | 条件重写、规则编译、模板注解 | 部分（内联脚本） |
| `internal/entity` | 模板继承、路径、查询、保存/重载 | 是 |
| `internal/function` | `@函数` 解释器：15 个语料函数 + 语法特性 | 是（从 .neko 取真实函数体） |
| `internal/events` | schema 构建、事件条件、处理器调用、深度保护 | 否（临时模板） |
| `internal/engine` | 条件、效果、分支、多主体、冲突、真实实体集成 | 是 |
| `internal/registry` | 应用/移除/引用计数、186 脚本重放、真实 Tick | 是（复制到临时目录） |
| `internal/mcpserver` | 工具注册、in-memory MCP 客户端调用 | 是 |
| `internal/config` | 默认值、JSON 合并、非法名称校验、状态组覆盖 | 否 |
| `cmd/astrbox-mcp` | 真机 E2E：真实子进程 + stdio MCP 协议、HTTP token、脚本 apply/重启/remove、事件触发 | 是（复制到临时目录） |

涉及写入的测试一律复制到 `t.TempDir()`，不会改仓库里的 `data/` 与 `scripts/`。

## 新增 MCP 工具

1. 在 `internal/mcpserver/` 新建或扩展工具文件，定义参数 struct（`json` + `jsonschema` tag）。
2. `mcp.AddTool(server, &mcp.Tool{...}, handler)` 注册；返回 `textResult`/`jsonResult`。
3. 失败走 `fail(...)`，返回以 `❌` 开头的文本，不 panic。
4. 补 `internal/mcpserver/server_test.go` 的调用断言。
5. 更新 `docs/mcp-tools.md`。

## 代码约束

- 一个文件一个功能；单功能 < 500 有效代码行、多功能 < 1000（空行/注释不计）。
- 命名：文件/函数/方法/参数/变量 `camelCase`，类型 `PascalCase`，常量 `UPPER_SNAKE`。
- 每个文件头写一句话职责；导出符号写文档注释。
- 交付前跑：

```bash
bash ~/.dsh/skills/code-modularity/scripts/lintCodeModularity.sh internal cmd --naming --comments
```

## 数据与脚本目录

- **数据目录**：默认 `data/`，可用 `-data-dir` 或 `ASTRBOX_DATA_DIR` 覆盖；**本地运行数据，不进版本库**。
- **脚本目录**：默认 `scripts/`，可用 `-scripts-dir` 或 `ASTRBOX_SCRIPTS_DIR` 覆盖。
- **配置文件**：默认当前目录 `config.json`，可用 `-config` 覆盖；缺失即用默认值。

```text
data/
├── entities.json       # 环境实体（场景/世界/素材…）
├── user/entities.json  # 用户角色实体（NPC，本地不发布）
├── scripts.json        # 脚本注册表（源文件在 scripts/）
├── templates/*.json    # 模板（由 @Data 落盘）
├── archives/           # 快照
├── logs/               # tick 日志（运行时）
└── llm_decisions.json  # 仲裁记录（运行时）

scripts/
├── core/ behavior/ auto/ r18/ rules/ scene/ meta/  # 按职责归档，注册名=basename
├── derivations/ repairs/                           # 运行期产物
└── rules.json                                      # 可选，启动时加载手写规则

config.json             # 可选，域词汇/安全/HTTP 配置
```

Python 版把脚本与数据放在同一目录；Go 版拆成两处，重放时从 `scripts/` 读源文件、向 `data/` 写状态。
`data/` 已在 `.gitignore` 中排除；注册表缺失时启动会从 `scripts/*.neko` 自动引导注册，实体状态需从备份恢复。

启动重放不写 `data/scripts.json`；只有显式写操作（apply/remove/实体写入）才落盘，规则 id 与 Python 一致（`internal/compiler/testdata/python_rule_ids.json` 是对照夹具）。迁移前仍建议备份：

```bash
cp -a data data.pre-go
cp -a scripts scripts.pre-go
```

## 备份

仓库不提供备份脚本；迁移/升级前手工打包：

```bash
tar --zstd -cf astrbox-$(date -u +%Y%m%dT%H%M%SZ).tar.zst data scripts
zstd -t astrbox-*.tar.zst
```
