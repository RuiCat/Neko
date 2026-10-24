# 任务范围：AstrBox Python → Go + MCP

## 目标

把原 AstrBot Python 插件（`legacy/python-astrbox/`）重写为 Go 引擎，并以独立 MCP 服务暴露；完全替换 AstrBot 入口；Python 版冻结为只读参考。

## 验收（用户确认）

- 最小闭环 + `.neko` 全量解析：Go MCP server 可启动；实体/脚本/规则读取与查询、脚本应用、状态机单步评估可用；go test 通过；MCP 客户端实际调通工具。
- 现有 186 个已注册脚本可重放；262 个 `.neko` 文件可解析。
- 单仓库（用户明确要求；与全局「一子项目一仓库」规则冲突，按用户要求执行）。
- Python 实现移入 `legacy/`。

## 约束

- 数据格式（`entities.json` / `templates/*.json` / `scripts.json` / `scripts/*.neko`）保持兼容。
- 不修改用户数据（测试一律复制到临时目录）。
- 代码：单文件 < 500 有效行（单功能）/ < 1000（多功能）；命名 camelCase；构建/测试走 `make`。
- 本机 Go 缓存只读 → `GOCACHE`/`GOMODCACHE` 固定到 `.local/`。

## 已知风险

- Go 的规则 id 哈希与 Python 不逐字节一致 → 首次启动会重写 `scripts.json` 的 rules 键；迁移前须备份。
- 函数执行器、事件总线、LLM 决策回填不在本轮验收内。

## 过程记录

- 2026-10-24：加载 `task-orchestration`、`code-modularity`、`task-planning`；用户确认范围与里程碑。
- 冻结契约：`internal/neko/ast.go`、`internal/engine/rule.go`、`internal/entity` 模型。
- 两个执行体并行：`internal/neko`（解析器）、`internal/engine`（状态机）；主智能体实现 entity/compiler/registry/mcpserver/cmd 并集成。
- 未开红队轮次：范围由用户固定，风险由「与 Python 逐值比对 + 真实数据重放 + Tick」覆盖；这是流程上的有意精简，记录在此以备回溯。
