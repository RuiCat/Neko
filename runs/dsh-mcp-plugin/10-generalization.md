# 泛化与能力谱系（dsh-mcp-plugin）

- 创建: 2026-10-25
- 教师技能: /home/ruicat/.dsh/skills/teacher-dsh-plugin-packaging/SKILL.md
- 教师复用理由: 全局域教师，覆盖 c5「包形态与 bundle patch」与 c9「安装形态与生效验证」，其「已知坑」适用于本任务；c2–c4、c6–c8 属该域教师明确排除的「插件内部业务逻辑」，按任务专有细节内联。
- 复用声明: 复用了已有域技能 `teacher-dsh-plugin-packaging`（命中理由：其能力集合覆盖本次 c5「包形态与 bundle patch」、c9「安装形态与生效验证」，且其「已知坑」直接适用于本任务）；c2–c4、c6–c8 属该域教师明确排除的「插件内部业务逻辑」，按任务专有细节内联，不写进域技能。
- 裁决: 见 `40-plan.md` → 裁决记录 `D1`

## 泛化阶梯

```
具体需求      为 Neko 猫引擎做 cmd/dsh-mcp，并接进 DSH web profile（提示注入 / 定时 Tick / 会话同步）
   ↓ 泛化
任务类别      为一个独立引擎进程做宿主侧运行时接入（MCP 暴露 + 宿主事件联动 + 可验证安装）
   ↓ 泛化
能力类别      装配复用 / 进程内控制面 / 宿主插件接线 / 生效验证
   ↓ 实例化
本次能力单元  c1 共享装配提取、c2 控制面契约、c3 Go 入口与控制服务、
              c4 并发与持久化安全、c5 bundle 包形态、c6 提示注入、
              c7 周期 Tick、c8 会话事件同步、c9 安装验证与文档
```

**为什么第一单元不是「写 cmd/dsh-mcp」**：现有 `cmd/astrbox-mcp/main.go:34-100` 已有完整装配（store → engine → funcs → events → registry → diagnostics → mcpserver），
新入口若复制这段接线会产生第二份分叉（违反 C10「≥2 目标共用逻辑必须提取」）；先把装配提升为 `internal/bootstrap`，两个命令只留 flag 与生命周期。

## 粒度候选（给用户 2–3 个，各带代价与推荐）

### 候选 A：4 模块（粗切，推荐）

| 模块 | 能力单元 | 输入 → 输出 | 并行度 | 集成风险 | 契约成本 |
|---|---|---|---|---|---|
| m1 装配复用 | c1 | 现有 main 接线 → `internal/bootstrap` + 两命令共用 | 与 m3 可并行 | 低（只动装配，回归门禁明确） | 无新增契约 |
| m2 Go 入口与控制面 | c2 c3 c4 | K1 契约 + `cmd/dsh-mcp` + `internal/control` → stdio MCP + Unix socket JSON + 串行化 | 依赖 m1 | 中（并发与错误语义是主风险） | K1 |
| m3 DSH bundle 与联动 | c5 c6 c7 c8 | K1 + K2 → `dsh/` 包（双 insert、prompt section、tick、会话事件） | 与 m1 可并行（按 K1 编码） | 中（提示注入不得刷屏 / 不得重复） | K1 K2 |
| m4 安装验证与文档 | c9 | 产物 + 真实 profile → Makefile 目标、验证脚本、`docs/dsh-plugin.md` | 依赖 m2 m3 | 低（动作需用户批准） | 无（消费 K1 K2） |

- 推荐 / 代价：**推荐 A**。写域天然互斥（`internal/bootstrap`+`cmd/astrbox-mcp` / `internal/control`+`cmd/dsh-mcp` / `dsh/` / `Makefile`+`docs/`），
  2 个契约（K1 控制面、K2 bundle 配置与提示格式），3 批完成（m1+m3 → m2 → m4），子智能体 4 个，正好在 L3 预算内。
  反面：m2 同时承载入口与控制面，若并发问题比预期深，需在 m2 内部再拆（升级触发：m2 实现超过 1 个文件的功能边界或验收连续失败）。

### 候选 B：6 模块（细切）

| 模块 | 能力单元 | 输入 → 输出 | 并行度 | 集成风险 | 契约成本 |
|---|---|---|---|---|---|
| m1 | c1 | 同 A.m1 | 可并行 | 低 | 无 |
| m2 | c2 | 契约文本 + 契约测试 | 可并行 | 低 | K1 |
| m3 | c3 c4 | 按 K1 实现 | 依赖 m2 | 中 | K1 K2 |
| m4 | c5 | bundle 骨架 + dump-config | 可并行 | 低 | K2 |
| m5 | c6 c7 c8 | 按 K1 K2 实现运行时 | 依赖 m2 m4 | 中 | K1 K2 K3 |
| m6 | c9 | 安装/验证/文档 | 依赖 m3 m5 | 低 | 无 |

- 推荐 / 代价：适合「并发安全」确实需要独立执行体长期打磨的情况；代价是 3 个契约、2 条串行链（m2→m3、m4→m5）、6 个执行体，
  协调开销明显高于 A，且 m2 只产契约文本会让契约与实现对撞的风险后移。**不推荐作为首选**。

### 候选 C：2 模块（最粗）

| 模块 | 能力单元 | 输入 → 输出 | 并行度 | 集成风险 | 契约成本 |
|---|---|---|---|---|---|
| m1 | c1 c2 c3 c4 | Go 侧全部 | 与 m2 可并行 | 高（模块内跨多个包） | K1 |
| m2 | c5 c6 c7 c8 c9 | DSH 侧全部 | 与 m1 可并行 | 高（安装依赖 m1 产物） | K1 K2 |

- 推荐 / 代价：只有 2 个执行体、1 轮并行，最省协调；但单模块远超「一个功能」粒度，失败定位困难，
  且 m2 的安装验证必须等 m1 产物，实际并不能真并行。**不推荐**。

## 用户裁决（★裁决1）

- 选定: 候选 A（4 模块：m1 装配复用 / m2 Go 入口+控制面 / m3 DSH bundle+联动 / m4 安装验证文档）
- 用户裁决: 接受（D1）
- 附带裁决: U1 bundle 落点 = Neko 仓库 `dsh/`；U6 允许提取共享装配并小改 `cmd/astrbox-mcp`（回归门禁）；U7 插件用零构建 `.mjs` + `node --test`
- 修改内容 / 暂缓原因: 无
- 时间: 2026-10-25

## 本次能力单元（选定候选的实例化）

| 单元 | 能力类别 | 输入 | 输出 | 不变量 | 验收（怎么判成了） | 边界（不管什么） |
|---|---|---|---|---|---|---|
| c1 | 装配复用 | `cmd/astrbox-mcp/main.go:34-100` 现有接线 | `internal/bootstrap` 的 Runtime 构造函数；两命令只做 flag 与生命周期 | astrbox-mcp 行为与工具清单不变；只读启动不写数据 | `make test` 绿；astrbox-mcp e2e 绿；工具清单集合不变 | 不改引擎 / 注册表 / MCP 工具语义 |
| c2 | 控制面契约 | DSH 侧需要的状态 / Tick / 会话事件语义 | `internal/control` 的端点和错误码契约文本 + 契约测试 | 引擎访问串行化；请求有超时；错误可区分「不可用 / 参数错 / 内部错」 | `go test ./internal/control/...` 覆盖正常 / 错误 / 拒绝 | 不做鉴权体系、不做多租户 |
| c3 | Go 入口与控制服务 | c1 Runtime + c2 契约 | `cmd/dsh-mcp`（stdio MCP + control listener） | 控制面只绑本机；MCP 面工具集合与 astrbox-mcp 一致 | stdio e2e：tools/list 集合相等 + stats 调用成功 | 不做 TLS / 远程访问 |
| c4 | 并发与持久化安全 | 并发调用场景 | 串行化 / 单飞实现与并发测试 | 同一时刻只有一个引擎写操作；Tick 不重入 | `-race` 下并发测试绿；并发 Tick 只产生一次执行 | 不改引擎既有锁语义 |
| c5 | bundle 包形态 | `teacher-dsh-plugin-packaging` 的验收清单 | `dsh/package.json` + `dsh/cordis.patch.yml` + 入口 | patch 必须是 `insert:`；`main` 可 import；包能被 profile 收编 | `--dump-config` 见两行；入口可加载 | 不把技能打进插件 |
| c6 | 提示注入 | 控制面状态快照 | 静态系统提示段（常量说明）+ `agent/pre-step` 动态快照消息 | 有长度上限；失败降级为上一版 / 空且不刷屏；同轮不重复；动态内容不进系统提示段 | 单测：静态段 3 次同值且零请求；pre-step 每轮 ≤1 条；超限截断；服务不可用不抛错 | 不改用户其它 prompt section；不做多租户隔离 |
| c7 | 周期 Tick | 配置周期 + 控制面 /tick | `ctx.interval` 定时器 | 单飞；组件 dispose 时停止；0 = 关闭 | 单测：周期触发、重入跳过、dispose 后不再触发 | 不做跨进程调度 / 不做 cron |
| c8 | 会话事件同步 | DSH 事件载荷 + K1 | POST /session-event + `.neko` 函数钩子 / 日志 | 只处理本工作区相关会话；事件不阻塞主流程 | 单测 + 真启动日志：事件送达且按 `dsh_on_event` 或日志兜底 | 不写死猫字段映射（留给脚本域） |
| c9 | 安装验证与文档 | 全部产物 | Makefile 目标、`docs/dsh-plugin.md`、验证记录 | 安装 / 卸载可回滚；文档命令可直接复制执行 | V4–V6 全部有实际输出 | 不改 DSH 内核 / 不发布 npm 包 |

## 内联结论（仅精简模式：未建域技能时填写跨任务成立的知识）

本次不新建域技能（打包域已有 `teacher-dsh-plugin-packaging`；其余属任务专有业务逻辑）。可复用结论先记录在此，供日后提升进域技能：

- **DSH 侧三个可用接缝（已核实源码）**：`systemPrompt.section({ name, order, interpolate, text: () => string })` 是动态系统提示的标准入口
  （`packages/core/system-prompt/src/index.ts:478`），`ctx.on('agent/pre-step', ...)` 走 message 注入（`dsh-project-memory` 先例），
  `ctx.interval(callback, delay)` 是 effect 作用域定时器（`api-catalog.ts:3343`）。
- **MCP 客户端行必须 `insert:`**（直接 `id:` 会被当成覆盖行而报 entry not found，circuit 项目已踩过）；同一行 config 是整体替换不是深合并。
- **MCP 命名空间**：`mcp__<serverName>__<rawName>`，`serverName` 必须匹配 `[A-Za-z0-9_-]{1,32}`（`packages/mcp/mcp-client/src/index.ts:39-60`）。
- **MCP server instructions 是连接期快照**（`server-context.ts:32-45` 的 `text: () => connection.instructions()`），不适合每轮变化的猫状态；
  动态状态改走 `agent/pre-step` 的 user 快照消息（`packages/context/time-context/src/index.ts:185-232` 是同款先例），系统提示只保留常量说明段（D2 裁决）。
- **`systemPrompt.section` 的 `text` 只能是同步返回**（`packages/core/system-prompt/src/index.ts:66`），不能在 provider 里 await 网络；状态刷新必须由定时器/事件驱动的后台缓存完成。
- **MCP 客户端重连上限**：默认 10 次后注销全部工具（`packages/mcp/mcp-client/src/connection.ts:211-245`），所以 `bin/dsh-mcp` 缺失是「静默丢工具」，需要 `make doctor` 可观测面。
