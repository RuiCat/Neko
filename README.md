# AstrBox

猫娘「猫猫」角色引擎：实体状态机 + `.neko` 脚本方言 + MCP 服务。

本仓库是原 AstrBot Python 插件的 **Go 重写版**：引擎核心（实体存储、`.neko` 词法/语法/编译、状态机、注册表）用 Go 实现，通过 [MCP](https://modelcontextprotocol.io) 暴露工具，不再依赖 AstrBot 运行时。原 Python 实现完整保留在 `legacy/python-astrbox/` 作为只读参考。

## 仓库布局

| 路径 | 内容 |
|---|---|
| `cmd/astrbox-mcp/` | MCP 服务入口（stdio / HTTP） |
| `internal/neko/` | `.neko` 词法分析、语法分析、AST |
| `internal/compiler/` | AST → 模板 / 规则 / 函数 / 事件 |
| `internal/registry/` | 脚本注册表：增量应用、引用计数、重启重放 |
| `internal/entity/` | 实体模型、模板继承、路径解析、持久化 |
| `internal/engine/` | 条件求值、效果应用、状态机 Tick、冲突 |
| `internal/function/` | `@函数` 解释器（变量/分支/循环/实体读写） |
| `internal/events/` | 字段变化 → `@事件` → `@函数` 的桥接与事件总线 |
| `internal/config/` | 基础配置层：状态词汇、全局实体、默认场景/事件字段/分类字段、HTTP token |
| `internal/diagnostics/` | 诊断环形存储：脚本/tick/校验告警的去重与作用域查询 |
| `internal/mcpserver/` | MCP 工具注册与处理器 |
| `data/` | **本地运行数据（不进版本库）**：`entities.json`（环境实体）、`user/entities.json`（NPC）、`scripts.json`、`templates/`、`archives/`、`logs/` |
| `scripts/` | `.neko` 脚本源文件，按 `core/behavior/auto/r18/rules/scene/meta/` 归档（含 `derivations/`、`repairs/`）；可选 `rules.json` |
| `legacy/python-astrbox/` | 原 Python 实现（冻结参考，不参与构建） |
| `docs/` | 架构、脚本语言、MCP 工具、迁移对照、开发说明 |
| `runs/astrbox-go/` | 本次迁移的范围与验证证据 |

## 快速开始

```bash
make build                      # 产物: bin/astrbox-mcp
make test                       # 全量 Go 测试
./bin/astrbox-mcp -data-dir data -scripts-dir scripts          # stdio MCP 服务
./bin/astrbox-mcp -data-dir data -scripts-dir scripts -config config.json
./bin/astrbox-mcp -data-dir data -scripts-dir scripts -transport http -addr 127.0.0.1:8765
```

配置层默认读当前目录的 `config.json`（缺失即用默认词汇），见 [docs/config.md](docs/config.md)。

MCP 客户端（stdio）配置示例：

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

工具清单与参数见 [docs/mcp-tools.md](docs/mcp-tools.md)。

## 数据兼容性

- `data/entities.json`、`data/templates/*.json`、`data/scripts.json`、`scripts/*.neko` 格式与 Python 版一致（Python 版把这些放在同一目录，Go 版拆成数据与脚本两处）。
- `data/` 是本地运行状态，已加入 `.gitignore`；注册表缺失时启动会从 `scripts/*.neko` 自动引导注册（`BootstrapFromSources`）。
- 启动时按 `data/scripts.json` 的条目从 `scripts/*.neko` 重放（replay）全部脚本，恢复规则、函数与事件到内存；字段注解（`@事件`）在写入时触发对应 `@函数`。
- **规则 id 与 Python 一致**：Go 复刻了 Python 的 canonical repr（含自动兜底分支的 `then: []`），186 个脚本逐 id 比对通过。只读启动不写 `scripts.json`；显式 apply/remove 落盘时 id 也不变。

## 验证

```bash
make test        # 单元 + 集成测试（真实数据：186 脚本重放、1808 条规则、Tick）
make lint        # go vet + gofmt 检查
```

迁移完整度、与 Python 的逐项对照和已知缺口见 [docs/parity.md](docs/parity.md)。

## 文档

- [架构](docs/ARCHITECTURE.md)
- [.neko 脚本语言](docs/neko-script.md)
- [MCP 工具参考](docs/mcp-tools.md)
- [基础配置层](docs/config.md)
- [与 Python 版对照](docs/parity.md)
- [开发与测试](docs/development.md)
