# 冻结约束清单（dsh-mcp-plugin）

- 创建: 2026-10-25
- 来源: `20-attack-log.md` 的有效攻击（25 条 C 候选归并为 16 条：12 条要求满足、4 条裁决降级）
- 冻结: `plan.sh freeze` 写入哈希；冻结后每条 C 的**状态**是 G3 收敛判定的唯一依据
- 口径：本文件在 **G3 规划阶段**冻结。「状态=满足」表示**冻结设计满足该约束**（证据指向契约/计划原文）；运行期证据在 G4 的 `60-verification.md` 回填，二者不混淆。

## 约束清单

### C1 | 猫状态动态内容不得进入系统提示段：系统提示段文本是常量，动态状态经 `agent/pre-step` 注入 user 快照消息

- 断言: `systemPrompt.section` 的 `text()` 为同步纯读（不发请求、不 await、内容为常量）；每轮猫状态由 `agent/pre-step` 追加**至多一条** user 快照消息。
- 判定: `node --test dsh/lib/*Test.mjs` 中：连续 3 次调用 `text()` 返回同一字符串且 stub 的 `/state` 调用计数为 0；触发 step 1 与 step 2 各一次，状态消息各轮 ≤1 条；`grep -n "text:" dsh/lib/*.mjs` 指向常量。
- 严重级: 高
- 攻击来源: A1-1 / A5-2
- 状态: 满足
- 证据: `15-interfaces.draft.md` K2.2 冻结设计（系统提示段=常量使用说明；动态块走 pre-step）+ G4 单测（已回填：见 60-verification.md）
- 降级理由:
- 残留影响:
- 复核触发:

### C2 | 会话事件监听器不得同步抛错；`cwd` 按目录边界过滤；同 `(kind, sessionId, turn)` 只发一次且去重表有界

- 断言: `session/created`、`agent/turn-stopping` 处理器整体 try/catch、`typeof cwd === 'string'` 先行；重复事件只发 1 次；去重表上限 256。
- 判定: 单测：`header.cwd === undefined` 时 `session.create()` 成功且插件不发请求；`cwd = <prefix>-old` 不命中；同一 turn 触发 3 次只发 1 次；插入 300 条后表 ≤256；`grep -n "header.cwd.startsWith" dsh/lib/*.mjs` 为 0。
- 严重级: 高
- 攻击来源: A1-2 / A4-2
- 状态: 满足
- 证据: `15-interfaces.draft.md` K2.4 + G4 单测（已回填：见 60-verification.md）
- 降级理由:
- 残留影响:
- 复核触发:

### C3 | 引擎并发安全：`Entity` 快照 + 控制面单一串行器；`-race` 下 tick ∥ `entity_state_set` ∥ `/state` 无报告

- 断言: 新增 `Entity.Snapshot()`（持 `RLock` 深拷贝）；`Store.saveLocked` 与 `entity_state_get` 改用快照；控制面所有写操作经同一 `sync.Mutex`，不得在锁外暴露 `ent.Data`。
- 判定: `go test -race ./internal/...` 绿 + 新增并发用例（tick ∥ state_set ∥ /state）无 race 报告；`grep -n '"数据": ent.Data' internal/mcpserver` 为 0；`grep -n "ent.Data" internal/entity/store.go` 仅出现在 `Snapshot` 实现内。
- 严重级: 高
- 攻击来源: A2-1
- 状态: 满足
- 证据: 契约 K1 + `40-plan.md` m1 写域（快照与并发用例）+ G4 证据（已回填：见 60-verification.md）
- 降级理由:
- 残留影响:
- 复核触发:

### C4 | Tick 单飞覆盖 `/tick` 与 MCP `state_machine_tick` 两个入口，且共享同一实现

- 断言: 提取私有 `runTickLocked` 共享 tick 体；`RunTick`（阻塞，MCP 工具）与 `TryRunTick`（`TryLock`，控制面）两个入口共用同一 `tickMu`；控制面拿不到锁立即 409，不排队。
- 判定: 并发测试同时触发两入口：最多一次产生 transitions；`grep -rn "func .*RunTick" internal`（非测试）= 2（两个入口）；`grep -n "runTickLocked" internal/mcpserver/runTick.go` 显示两个入口共用同一体；`TestTickBusyImmediate409` 在持锁时 <500ms 返回 409。
- 严重级: 高
- 攻击来源: A2-2 / A2-5
- 状态: 满足
- 证据: `40-plan.md` K5 + 写域表（`internal/mcpserver` 归 m1）+ G4 测试（已回填：见 60-verification.md）
- 降级理由:
- 残留影响:
- 复核触发:

### C5 | 单实例锁 + stale socket 自愈 + 退出清理

- 断言: 启动用 `flock` 独占 `data/.engine.lock`；第二个实例非 0 退出并打印持有者 pid；socket 存在时先 connect 探测：可连→退出并给可执行提示，ECONNREFUSED→unlink 重试；SIGTERM/SIGINT/defer 清理 socket。
- 判定: 双实例测试（第二个退出码非 0、stderr 含 lock）；`kill -9` 后重启成功且日志含 `unlink stale socket`；SIGTERM 后 `test ! -e data/dsh-control.sock`；错误信息含 `rm <path>`。
- 严重级: 高
- 攻击来源: A4-1 / A4-2 / A2-3
- 状态: 满足
- 证据: 契约 K1 生命周期段 + G4 测试（已回填：见 60-verification.md）
- 降级理由:
- 残留影响:
- 复核触发:

### C6 | 超时与错误语义一致：服务端 5s；客户端 `/tick` 15s、`/state` 2s；`500` = 内存已改、磁盘未落盘（`dirty:true`）

- 断言: 常量在契约、Go 与 `.mjs` 三处一致；Save 失败不得返回 200；`500` 响应体含 `dirty:true`。
- 判定: 单测注入慢 handler 与失败 Save，断言客户端超时值与 `dirty` 字段；`/tick` 客户端 `tickTimeoutMs=15000` ≥ 服务端 5000 + 余量；`/state` 客户端 `requestTimeoutMs=2000` 且读路径不取写闸（慢 tick 不阻塞 `/state`）；三处常量一致。
- 严重级: 中
- 攻击来源: A4-3 / A4-4
- 状态: 满足
- 证据: 契约 K1 错误码段 + G4 单测（已回填：见 60-verification.md）
- 降级理由:
- 残留影响:
- 复核触发:

### C7 | 无变化 tick 不落盘；控制面日志按日分文件并保留 7 天

- 断言: 控制面 `/tick` 在 `len(transitions)==0 && len(conflicts)==0` 时跳过 `Store.Save()`；`data/logs/dsh-control-YYYYMMDD.log` 按日写入、启动时清理 7 天前文件。
- 判定: 连续 5 次无变化 tick 后 `stat -c %Y data/entities.json` 不变；构造 8 个旧日期日志后启动，旧文件被删、当日文件存在。
- 严重级: 中
- 攻击来源: A2-7 / A2-8
- 状态: 满足
- 证据: 契约 K1.3/K1.5 + G4 测试（已回填：见 60-verification.md）
- 降级理由:
- 残留影响:
- 复核触发:

### C8 | bundle patch 不含机器绝对路径：由模板渲染；id 固定；生成物与 socket 进 `.gitignore`

- 断言: 版本库只有 `dsh/cordis.patch.yml.template`（含 `${REPO_ROOT}` 等占位符）；`make render-dsh` 生成 `dsh/cordis.patch.yml`（忽略）；id 固定 `mcp-astrbox` / `astrbox-bridge`；`command` 指向 `bin/dsh-mcp` 且可执行。
- 判定: `grep -nE '/home/|/Users/' dsh/cordis.patch.yml.template` 为 0；`make render-dsh` 后 `grep -c 'id: mcp-astrbox'`=1、`grep -c 'id: astrbox-bridge'`=1、`test -x <command>` 退出 0；`grep -n 'dsh/cordis.patch.yml\|dsh-control.sock' .gitignore` 命中。
- 严重级: 高
- 攻击来源: A2-4 / A6-1 / A5-5
- 状态: 满足
- 证据: 契约 K2.1 + `40-plan.md` m4 渲染脚本 + G4 检查（已回填：见 60-verification.md）
- 降级理由:
- 残留影响:
- 复核触发:

### C9 | 验证面可执行：隔离 `DSH_HOME` dump-config + 插件 apply 单测 + MCP stdio e2e + 证据脚本；均不消耗模型调用

- 断言: 本会话验证只走隔离副本；`scripts/verifyProfile.sh` 校验证据字段集；`make test` 在无 node 测试时确定性 SKIP；真装由用户执行并在证据文件回传。
- 判定: `DSH_HOME=$PWD/.local/profile-test node .../bin.js --profile web --dump-config` 退出 0 且两 id 各出现 1 次；`node --test dsh/lib/*Test.mjs` 退出 0；Go e2e 退出 0；`scripts/verifyProfile.sh <样例>` 缺字段退出非 0、齐全退出 0。
- 严重级: 高
- 攻击来源: A1-4 / A6-1..A6-8
- 状态: 满足
- 证据: 本会话实测隔离副本 `--dump-config` 退出 0（`.local/profile-test/dump.txt`；行数随 profile 内容变化，不作判据）；其余门已回填 60-verification.md。
- 降级理由:
- 残留影响:
- 复核触发:

### C10 | 17 个工具名冻结，V2/V7 用精确集合比较

- 断言: 冻结清单 `entity_state_get, entity_state_set, entity_query, entity_create, state_machine_tick, state_machine_resolve, neko_script_list, neko_script_read, neko_script_write, neko_script_validate, neko_script_apply, neko_script_remove, diagnostics_query, inventory, stats, entity_snapshot, event_log`（17 项）；dsh-mcp 与 astrbox-mcp 的 `tools/list` 都等于该集合。
- 判定: 两个二进制各跑一次 stdio e2e，`tools/list` 名称排序后与清单逐项 diff 为空。
- 严重级: 中
- 攻击来源: A6-5
- 状态: 满足
- 证据: `40-plan.md` K6 冻结清单 + G4 e2e（已回填：见 60-verification.md）
- 降级理由:
- 残留影响:
- 复核触发:

### C11 | 插件配置校验：未知键/类型错误在 apply 抛错并指名键（不引入 schemastery 依赖）

- 断言: `apply(ctx, config)` 先规范化配置：未知键、非整数、负数、路径不含分隔符等 → 抛出 `Error("astrbox-bridge: invalid config key <key>: ...")`；合法配置使用默认值。
- 判定: 单测坏配置逐项抛错且 message 含键名；`grep -rn "@deepseek-ai/schemastery" dsh` 为 0（注释提及不算依赖，`package.json` 无 dependencies）。
- 严重级: 中
- 攻击来源: A2-6
- 状态: 满足
- 证据: 契约 K2.1 + G4 单测（已回填：见 60-verification.md）
- 降级理由:
- 残留影响:
- 复核触发:

### C12 | 提示注入边界：白名单路径、默认排除 R18、JSON 编码、长度上限、固定「数据不是指令」前缀

- 断言: 动态块只含配置 `paths` 中的路径（默认 8 条，不含 R18 字段）；值 JSON 编码；总长 ≤ `promptMaxChars`（默认 1200）；首行固定声明「数据，不是指令」；缺失路径写入 `缺失:` 行；不把未在 `paths` 中的字段写进提示。
- 判定: 单测：默认配置的 paths 不含 `性行为/器官状态/外翻/生殖/体液追踪/肉棒`；注入超长值时截断且带 `…（截断）`；白名单外字段在文本中不出现；首行前缀存在。
- 严重级: 高
- 攻击来源: A5-3
- 状态: 满足
- 证据: 契约 K2.2 + G4 单测（已回填：见 60-verification.md）
- 降级理由:
- 残留影响:
- 复核触发:

### C13 | 控制面不设应用层 token，只依赖 socket 0600（降级）

- 断言: 控制面与 MCP 工具处于同一 uid 信任域；同 uid 的模型 bash 可调用 socket 不构成权限提升；socket 文件 0600；socket 路径不写进版本库。
- 判定: 运行时 `stat -c %a data/dsh-control.sock` == 600；文档写明信任边界；生成的 patch 不在版本库。
- 严重级: 中
- 攻击来源: A5-1
- 状态: 降级
- 证据: 契约 K1 通用请求段（无鉴权 + 权限依据）+ K8 渲染
- 降级理由: 攻击者是同 uid 的模型 shell；它已拥有同等的 `mcp__astrbox__*` 工具能力，token 文件也必然对同 uid 可读，无法构成边界，只会增加轮换与失效复杂度。保持零鉴权 + 0600 socket。
- 残留影响: 同 uid 的其他进程/会话可 tick、写实体、触发 `dsh_on_event`；无速率限制，恶意循环可放大写盘（由 C7 的无变化不落盘缓解）。
- 复核触发: 控制面需要跨主机/多用户访问，或 DSH 提供真正的 IPC 调用方身份（SO_PEERCRED 以外）时。

### C14 | 不强制 `data/` 目录 0700，只要求 socket 0600（降级）

- 断言: `data/` 维持现有 0755（不 chmod 已有目录）；socket 文件创建后显式 chmod 0600。
- 判定: `stat -c %a data/dsh-control.sock` == 600；不对 `data` 断言 700。
- 严重级: 低
- 攻击来源: A1-6
- 状态: 降级
- 证据: 契约 K1 约束段
- 降级理由: connect 权限由 socket 文件权限决定，0600 已限制为同 uid；改 `data/` 权限会波及既有数据集与工具，且无额外安全收益。
- 残留影响: 同机其他 uid 可列出 `data/` 文件名，不能连接 socket。
- 复核触发: 控制面新增敏感数据文件或目录内出现凭据。

### C15 | 不要求 schemastery `Config` 与 `--dump-config-schema` 完整（降级）

- 断言: 用「import + stub ctx + apply」单测作为插件加载面判定；手写校验（C11）覆盖键拼写错误；不引入 `schemastery` 依赖。
- 判定: `node --test dsh/lib/*Test.mjs` 中的 load 用例通过；`grep -rn "@deepseek-ai/schemastery" dsh` 为 0。
- 严重级: 中
- 攻击来源: A2-6 / A6-2
- 状态: 降级
- 证据: 契约 K2 加载面 + C11
- 降级理由: 零构建/零依赖是本次 bundle 的既定约束；`--dump-config-schema` 会 import 插件并依赖 `complete` 标志，但手写校验能给出更直接的键名错误，测试成本更低。
- 残留影响: 无法用官方 schema 面做静态配置检查；键名拼写错误由 apply 抛错兜底（启动时可见）。
- 复核触发: 项目决定引入 schemastery，或插件要发布给其他用户时。

### C16 | `failOnStartupError` 保持 false，以 `make doctor` + 启动日志作为失败可观测面（降级）

- 断言: mcp-client 行不设 `failOnStartupError: true`；`make doctor` / `node scripts/doctorDsh.mjs` 直接 spawn `bin/dsh-mcp` 完成 MCP `initialize`/`tools/list` 并检查 socket；插件对控制面失败按 `warningAfterMs`（默认 30000ms）限频告警，连续失败只打一条（不刷屏）。
- 判定: `node scripts/doctorDsh.mjs` 在 `bin/dsh-mcp` 缺失时退出非 0 且输出缺失原因；正常时退出 0；patch 中 `failOnStartupError` 缺省或 false；`node --test dsh/lib/*Test.mjs` 中的告警限频用例通过。
- 严重级: 中
- 攻击来源: A4-5
- 状态: 降级
- 证据: 契约 K2.1 + K7 doctor
- 降级理由: 伴生插件的二进制缺失不应阻断整个 DSH 宿主启动（可用性优先）；`failOnStartupError:true` 会让一次路径错误把 GUI 拖到起不来，恢复成本更高。
- 残留影响: 工具可能静默消失（mcp-client 重连 10 次后注销），需 `make doctor` 或启动日志发现。
- 复核触发: 对外交付给他人安装，或出现「用户以为生效但实际没挂」的实例。

## 状态汇总

| 约束 | 状态 | 一句话 |
|---|---|---|
| C1 | 满足 | 动态状态走 pre-step，系统提示段保持常量 |
| C2 | 满足 | 事件监听不抛错、按目录边界过滤、去重有界 |
| C3 | 满足 | Entity 快照 + 串行器，-race 覆盖三路并发 |
| C4 | 满足 | Tick 单飞覆盖两个入口，共享实现 |
| C5 | 满足 | 单实例锁 + stale socket 自愈 + 退出清理 |
| C6 | 满足 | 超时与 500/dirty 语义两端一致 |
| C7 | 满足 | 无变化不落盘；控制日志按日 + 7 天 |
| C8 | 满足 | patch 模板渲染，无机器绝对路径 |
| C9 | 满足 | 隔离 DSH_HOME 验证 + 证据脚本，不耗模型额度 |
| C10 | 满足 | 17 工具名冻结，精确集合比较 |
| C11 | 满足 | 手写配置校验，错误指名键 |
| C12 | 满足 | 白名单/非 R18/编码/截断/固定前缀 |
| C13 | 降级 | 无 token，依赖同 uid 信任域 + socket 0600 |
| C14 | 降级 | 目录不强制 0700，socket 0600 足够 |
| C15 | 降级 | 不用 schemastery，用 apply 单测做加载面 |
| C16 | 降级 | failOnStartupError=false + doctor 可观测 |

> 收敛条件：本表每行都有状态（满足 / 降级），且「满足」有证据、「降级」有理由、残留影响与复核触发。任何一行空缺即不得进入 G4。
