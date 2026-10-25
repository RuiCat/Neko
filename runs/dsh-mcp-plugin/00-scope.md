# 需求范围（dsh-mcp-plugin）

- 创建: 2026-10-25
- 目标（一句话）: 交付 DSH 专用 Go MCP 入口 `cmd/dsh-mcp` 与 DSH 插件 bundle，让猫引擎在 web profile 里以 `mcp__astrbox__*` 可用，并把猫状态注入系统提示、按周期 Tick、接收 DSH 会话事件。
- 状态: 草稿（待 ★裁决1 后冻结）

## 目标

做完什么算成了（可判定，不用「优化」「完善」这类词）：

> 在 Neko 仓库新增 `cmd/dsh-mcp`（复用现有引擎与 17 个 MCP 工具）+ 一个可安装进 DSH `web` profile 的插件 bundle；安装后：
> ① 模型可调用 `mcp__astrbox__<tool>`；② 系统提示含**静态** astrbox 使用说明，每轮模型请求含一条**动态**猫状态快照消息（有长度上限、失败降级不刷屏）；
> ③ 插件按配置周期触发引擎 Tick；④ DSH 会话事件（创建 / 结束轮次）可送达引擎并留下可查日志。

## 验收

| # | 验收项 | 判定方式（命令 / 标准） | 由谁判 |
|---|---|---|---|
| V1 | Go 构建与测试 | `make build` 产出 `bin/astrbox-mcp` 与 `bin/dsh-mcp`；`make test`、`make lint` 退出码 0 | 父体 + 独立复核者 |
| V2 | dsh-mcp MCP 面 | dsh-mcp 的 stdio e2e：`initialize` 后 `tools/list` 含现有 17 个工具名（集合等于 `cmd/astrbox-mcp` 当前清单），抽样调用 `stats` 成功 | 父体 |
| V3 | 控制面契约 | `internal/control` 单测覆盖 `GET /health` `GET /state` `POST /tick` `POST /session-event` 的正常 / 错误 / 拒绝语义；`go test ./internal/control/...` 绿 | 父体 |
| V4 | profile 组合 | 隔离 `DSH_HOME` 下 `--dump-config` 退出 0，输出含两行 id（`mcp-astrbox` 与 `astrbox-bridge`）各 1 次，`command` 路径 `test -x` 通过；真 profile 只允许用户执行并回传只读证据 | 父体（隔离）/用户（真装） |
| V5 | 真实生效（机器可查） | ① 隔离/真装 profile 的 `--dump-config` 两 id 各 1 次；② `make doctor` 的 MCP `tools/list` 精确等于 17 名清单；③ `grep -c ' POST /tick ' data/logs/dsh-control-*.log` 在 2×周期后 ≥1；④ `grep -c ' POST /session-event ' ...` ≥1；⑤ 请求快照里 `grep -c '\[astrbox\]'` ≥1（真装会话） | 父体（①②③④）+ 用户回传⑤ |
| V6 | 回滚 | 删除 bundle 行后 `--dump-config` 不再出现该行；`make test` 仍绿；仓库工作区无残留 profile 写入 | 父体 |
| V7 | 无回归 | `cmd/astrbox-mcp` 的既有 e2e 与工具清单不变（若做共享装配提取，必须证明行为等价） | 独立复核者 |

## 约束

| 类别 | 内容 | 来源 |
|---|---|---|
| 时间 / 预算 | L3：子智能体 ≤4/批、红队 1 轮、独立复核 1 轮、轮次 ≤8；只规划到 G3，未经 ★裁决3 不进入 G4 | 用户全局 AGENTS.md + task-planning |
| 环境（沙箱 / 只读挂载 / 网络） | 实测：`/home` 在本会话是 `ro` 挂载（`findmnt -T /home/ruicat/.dsh` → `ro,...`，写探针报 `EROFS`），**只有工作区 `/home/ruicat/Desktop/Neko` 单独 rw**；因此 `~/.dsh/profiles/web` 不能从本会话写入，`dsh plugin add` / 真 profile 安装必须由用户（宿主 namespace）执行。已验证可用的替代：把 profile 元数据复制到工作区 `.local/profile-test/`，`DSH_HOME=... node apps/cli/lib/bin.js --profile web --dump-config` 退出码 0（输出以 `# == @deepseek-ai/dsh-base` 开头） | 运行时实测（findmnt / dump-config） |
| 不可动的既有接口 | 现有 17 个 MCP 工具名与返回约定（`docs/mcp-tools.md`）不变；`data/`、`scripts/` 语义不变；`cmd/astrbox-mcp` 行为不变 | `docs/mcp-tools.md`、`cmd/astrbox-mcp/main.go` |
| 合规 / 许可 | 不引入网络服务暴露面（仅本机 Unix socket / loopback）；不读写凭据；不修改 DSH 内核 | 用户全局 AGENTS.md 安全边界 |
| 构建约束 | 源码文件有效行 <500（多功能 <1000，达到即拆）、新命名 camelCase、有编译产物即用 `Makefile` 目标；交付前跑 `lintCodeModularity.sh cmd internal dsh --naming --comments` | `code-modularity` 技能 |
| 依赖 | Go 侧沿用 `github.com/modelcontextprotocol/go-sdk v1.6.0`；DSH 侧优先零构建 `.mjs`，不新增 npm 依赖 | `go.mod`、`dsh-project-memory` 先例 |

## 未知

| # | 未知项 | 为何会改变方案 | 如何消除 | 何时必须消除 |
|---|---|---|---|---|
| U1 | bundle 落点：Neko 仓库 `dsh/` vs 独立仓库 | 决定 package 名、安装形态（link:）、写域与交付清单 | ★裁决1 让用户选（推荐 Neko `dsh/`） | G1 前 |
| U2 | 控制通道形态：Unix socket / loopback HTTP / 插件自起 daemon | 决定 K1 契约、权限与失败语义、平台边界 | G3 决策点（推荐 Unix socket，loopback 兜底写进文档） | G3 前 |
| U3 | Tick 归属与周期：插件 `ctx.interval` 触发 vs 服务端自 tick；默认周期 | 决定谁负责重入保护、进程生命周期与日志口径 | G3 决策点（推荐插件触发，默认 60s，0 = 关） | G3 前 |
| U4 | 会话事件映射：固定实体字段 vs 调用 `.neko` 函数 `dsh_on_event` | 决定「会话↔状态同步」是写死还是脚本域逻辑 | G3 决策点（推荐函数钩子 + 日志兜底） | G3 前 |
| U5 | 真安装由谁执行：`~/.dsh` 是只读挂载，本会话只能做「隔离 DSH_HOME 副本」验证 | 决定 V4/V5 的可判定边界与证据来源 | G3 决策点：接受「本会话验隔离副本 + 用户执行真装并回传输出」，或用户提供可写宿主通道 | G3 前 |
| U6 | 是否允许提取共享装配并小改 `cmd/astrbox-mcp` | 影响 m1 引入、回归范围与写域 | ★裁决1 一并确认（推荐允许，回归门禁） | G1 前 |
| U7 | DSH 插件语言：零构建 `.mjs` vs TS + 构建 | 影响 Makefile、测试方式与依赖 | ★裁决1 一并确认（推荐 `.mjs` + `node --test`） | G1 前 |

## 变更记录（需求中途变更时追加，并把受影响产物标 stale）

| 日期 | 变更 | 受影响产物 | 需重做的门 |
|---|---|---|---|
| 2026-10-25 | 用户确认：目标是 Go 入口 + DSH 接入 bundle，范围含 DSH 会话联动 | 10/20/30/40 | G1 起 |
