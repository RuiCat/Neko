# 红队攻击日志（dsh-mcp-plugin）

- 创建: 2026-10-25
- 红队执行体: `red-team-dsh-mcp`（独立上下文，非实现者；结论是攻击结论，不是「方案可用」自证）
- 输入: `00-scope.md`（01:05 版，含只读 `/home` 事实）`10-generalization.md` `15-interfaces.draft.md`（含 K1/K2/K3/K4 与安装段更新）—— **不含教师的自辩理由**
- 只读取证: Neko 仓库源码、DSH checkout `packages/mcp/mcp-client/src`、`packages/core/{system-prompt,session,agent}`、`apps/cli/src`、`packages/boot/{app-boot,plugin-manager}`、`vendor/cordis`、`~/.dsh/profiles/web`、`.local/profile-test`
- 处置: 见 `40-plan.md` → 裁决记录 `D2`

## 六攻击面结论（缺一不可）

> **填写规则（C10）**：结论列只能填 `击穿` 或 `未击穿` **二者之一**。

| 面 | 结论 | 依据（反例 / 命令 / 证据） |
|---|---|---|
| A1 假假设 | 击穿 | K2.2 用**同步** `text()` 承载需异步拉取的 `/state` 快照（`system-prompt/src/index.ts:52-72` 只允许 `(ctx)=>string`），且刷新驱动完全未定义 → 按契约写出的实现只能返回空/陈旧文本；同类：`session.header.cwd` 实为可选（`session/src/types.ts:106`）；`--dump-config` 实为**写操作**（`profile-boot.ts:198`），实测 EROFS 退出 1；「Tick 日志按周期出现」在无状态变化时既有 logger **不写任何行**（`stateLogger.go:30-34`）。详见 A1-1..A1-6 |
| A2 隐藏耦合 | 击穿 | 控制面 `/state` `/tick` 与 MCP 17 工具在同一进程并发读写 `*entity.Entity`，但 `Store.Save()` 读 `ent.Data` 不加 `Entity.mu`（`store.go:122-140` vs `model.go:104,135`）、`entity_state_get` 直读 `ent.Data`（`server.go:120`）→ Go 运行时 fatal `concurrent map read and map write` 不可 recover；另有「MCP 工具 tick 绕过 K1 单飞」「两进程共写 `data/`」「版本库内 9 处绝对路径」「tick 语义复用点落在 m2 写域之外」等 11 条。详见 A2-1..A2-11 |
| A3 循环依赖 | 未击穿 | 核查了 4 条真实依赖边：m1→无上游、m2→m1（`internal/bootstrap`）、m3→K1 契约（非代码）、m4→m2+m3；`internal/bootstrap`、`internal/control`、`internal/mcpserver`、`cmd/*` 之间不存在可构成环的 import（装配方向单向：cmd → bootstrap → {entity,engine,events,function,registry,mcpserver}）。唯一残余风险是「谁创建 control listener」在 K1 中未指定，若不放在 m2 而放进 bootstrap(m1)、且 control 反过来 import bootstrap，则会形成 m1↔m2 环——已列入 H1 观察。详见 A3-1 |
| A4 边界 / 失败 / 回滚 | 击穿 | 崩溃/SIGKILL 后残留 socket 文件按 K1 会**永久拒绝启动**且无自愈分支（K1:10）；`session/created` 监听器同步抛错会**回滚整个会话 attach**（`session/src/index.ts:1114,1285-1288`），而 K2.4 的「若 `header.cwd` 以前缀开头」在 `cwd?: string` 下必抛 TypeError；客户端 1500 ms < 服务端 5 s 期限；`failOnStartupError` 未置 true（默认 false，`mcp-client/src/index.ts:128`）→ Go 侧失败被静默吞成重连。详见 A4-1..A4-6 |
| A5 资源与权限 | 击穿 | 控制面无鉴权，且 socket 路径写在**版本库内且可读**的 `dsh/cordis.patch.yml` 里 → 模型自己的 bash 工具可 `curl --unix-socket` 直接 `POST /tick` / `POST /session-event`，绕过插件的单飞与去重、并可借 `.neko` 的 `dsh_on_event` 改实体（A5-1）；`dsh-control.log` 无轮转（A2-8，对照 `stateLogger.go:44-52`）；每次 Tick 无条件全量重写两份实体文件（A2-7，`stateTools.go:37` + `store.go:136-139`）；提示块位置击穿前缀缓存（A5-2）；只读 home 下「真装」不可由父体执行（A5-4）。详见 A5-1..A5-5 |
| A6 交付可验证性 | 击穿 | V4 断言的两行 id 与 K2.1 不一致（`00-scope.md:22` 写 `neko-bridge`，`15-interfaces.draft.md:77` 是 `astrbox-bridge`）；V4 的命令在本会话**实测退出 1**（EROFS）；`--dump-config` 不 import 插件、不校验 schema、不 spawn 子进程（`dump-config.ts:5,39-41`）；K3 的隔离副本当前 `dump.txt` 里 `astrbox` 出现 **0** 次（1586 行），node_modules 是指向只读真 profile 的 symlink，`link:` 条目根本放不进去；K3「或 dump/config 面证据」允许把加载面降级为零增量；「用户回传证据」无可判定字段集。详见 A6-1..A6-8 |

> 六面全部「未击穿」时先怀疑红队输入太少或与教师同源——本表 A3 为「未击穿」，其余五面均击穿。

## 攻击明细

### A1-1 K2.2 把「需网络等待的状态快照」放进**同步** systemPrompt section：契约不可实现

- 反例（可复现最优）:
  ```
  // DSH 的 section.text 只接受同步返回值
  packages/core/system-prompt/src/index.ts:52-72
    readonly text: string | ((context: AssembleContext) => string)
  packages/core/system-prompt/src/index.ts:646-652   // assemble() 里同步调用 section.text(context)
  packages/core/agent-loop/src/agent.ts:272          // 每个 pre-step 都 assemble → 每步都调 text()
  ```
  K2.2（`15-interfaces.draft.md:109`）要求「最多每 `promptRefreshMs` 拉一次 `/state`；读取失败返回上一次成功文本」——拉取是异步动作，而 `text()` 不能 `await`。契约没有规定刷新由谁驱动（`ctx.interval`？`system-prompt/assemble` 钩子？开一个 fire-and-forget 循环？），也没规定 `text()` 在首次刷新完成前返回什么。
- 红队结论: 实现者只能二选一：① `text()` 永远返回 `''`（首帧无数据、且没有驱动 → 永久空）；② 自己发明一个未写进契约的后台刷新器。前者让 V5「系统提示含状态块」永远不可达，后者让「每 5 s 拉一次」变成未受控行为（可能每步都发一次 socket 请求）。
- 建议处置: 固化为 `C-R1`（刷新驱动进入 K2.2）；同时要求 `text()` 为纯读缓存。
- 建议严重级: **高**

### A1-2 `session.header.cwd` 是可选字段，K2.4 的判据按必填写

- 反例:
  ```
  packages/core/session/src/types.ts:106      readonly cwd?: string
  15-interfaces.draft.md:121                  「若 session.header.cwd 以 workspacePrefix 开头 → POST」
  ```
  按字面实现即 `session.header.cwd.startsWith(...)`；`cwd === undefined` 时抛 TypeError。
- 红队结论: 不是「少发一个事件」而是**打挂会话创建**：`session/created` 的同步抛错会否决发布并回滚 attach（`session/src/index.ts:1109-1118` 注释 + `1285-1288` `Synchronous throws intentionally propagate and veto publication`）。这属于插件把宿主功能弄坏的最高等级耦合。
- 建议处置: 固化为 `C-R2`（监听器体必须 `typeof cwd === 'string'` 先行、整体 try/catch、且断言「不得同步抛错」）。
- 建议严重级: **高**

### A1-3 「Tick 日志按周期出现」在无状态变化时没有行可看

- 反例:
  ```
  internal/engine/stateLogger.go:30-34
    if len(result.Transitions) == 0 && len(result.Conflicts) == 0 { return nil }
  ```
  `data/logs/` 目录当前不存在（`ls -la data/logs` → 无此目录），即既有 Tick 日志是按「有变化才写」的 JSONL。
- 红队结论: V5 的「Tick 日志按周期出现」若按直觉查 `data/logs/state_*.log`，在猫状态稳定的一小时里**一行都没有**，验收会被判失败；若改成查 `data/logs/dsh-control.log`，则必须把这一点写进判据，否则实现者会用「为了有日志而强行制造 transition」去蒙门。
- 建议处置: 固化为 `C-R3`（把 V5 的 Tick 证据钉到 `dsh-control.log` 的 `/tick` 行，并给出 grep 条件）。
- 建议严重级: 中

### A1-4 V4 的命令在本会话**必然失败**（`--dump-config` 是写操作），且失败原因与插件无关

- 反例（实际输出，本会话执行）:
  ```
  $ node /home/ruicat/Desktop/deepseek-harness/apps/cli/lib/bin.js --profile web --dump-config
  EXIT=1
  Error: EROFS: read-only file system, open '/home/ruicat/.dsh/profiles/web/cordis.yml'
      at writeFileSync (node:fs:3028:20)
      at prepareProfile (.../profile-boot-C14i-8IN.js:220:2)
      at runDumpConfig (.../dump-config-CWs_HdOc.js:25:17)
  # 源码对应：apps/cli/src/profile-boot.ts:194-200 prepareProfile() 无条件
  #   writeFileSync(join(profile.dir, PROFILE_ROOT_FILENAME), PROFILE_ROOT_CONFIG)
  ```
  `findmnt -T /home/ruicat/.dsh` → `btrfs ro,...`；`findmnt -T /home/ruicat/Desktop/Neko` → `rw`。
- 红队结论: `00-scope.md:22` 的 V4 判定命令按字面**不可执行**；`--dump-config` 亦非只读，会改写 profile 的 `cordis.yml`（并在旧版链接后端下 `removeLinkProjections` 删 symlink，`profile.ts:287-301`）。任何「先跑 V4 再说」的流程会得到一条与插件无关的 EROFS 错误，极易被误读成组合错误。
- 建议处置: 固化为 `C-R4`（V4 命令必须带 `DSH_HOME` 且断言退出码 0；真 profile 只允许只读证据）。
- 建议严重级: **高**

### A1-5 `5000` 字符/`1200` 字符上限之外，`/state` 的默认上限与截断语义未定义「部分值」

- 反例:
  ```
  15-interfaces.draft.md:26  「范围 1024..1048576；超出时返回 `413`？不：见下」
  15-interfaces.draft.md:29-30  「data 序列化超过 maxBytes 时截断为 {"__truncated__":true}」
  ```
- 红队结论: 契约正文残留了自问自答的**未决标记**（「413？不：见下」），且「未指定 path 时整体截断为 `__truncated__`」让调用方在数据膨胀后拿到零信息（不是部分信息），而插件侧只会显示「缺失」——这条路径没有测试断言点。
- 建议处置: 记为疑点 H2（见待观察），或固化为 `C-R5`（禁止残留「？」式未决文本；截断必须有单测）。
- 建议严重级: 低

### A1-6 「数据目录 0700 / socket 0600」的父目录权限与现状不符

- 反例:
  ```
  15-interfaces.draft.md:8   「文件模式 0600，父目录 0700」
  $ ls -ld data           → drwxr-xr-x（0755）
  internal/entity/store.go:171-173  MkdirAll(..., 0o755)   // 既有写路径从不收紧权限
  ```
- 红队结论: `data/` 已存在且为 0755，`MkdirAll` 不会改既有目录权限；要实现 0700 必须显式 `chmod`，契约没写。0600 的 socket 本身仍能挡住其他 uid（connect 受 socket 文件写权限约束），所以这是「契约描述与现状不一致」而非直接越权，但会让「依赖 socket 文件权限」这句安全论证的前提不成立。
- 建议处置: 固化为 `C-R6`（启动时显式 chmod 或改断言口径）。
- 建议严重级: 低

### A2-1 控制面与 MCP 工具并发写同一 `Entity`：`Store.Save()` 无实体锁 → Go fatal 崩溃

- 反例（代码路径可核对）:
  ```
  # 读：Entity.Get 有 e.mu.RLock
  internal/entity/model.go:99-113
  # 写：Entity.Set 有 e.mu.Lock（只保护 path 写入）
  internal/entity/model.go:126-147
  # 但整库落盘时直接读 ent.Data / ent.Keys / ent.Meta，**不取 e.mu**
  internal/entity/store.go:122-140   saveLocked(): record := {... "data": ent.Data ...}
  internal/entity/store.go:142-149   writeEntitiesFile(): json.MarshalIndent(payload)  // 遍历 ent.Data
  # MCP 工具直读 ent.Data（连拷贝都没有）
  internal/mcpserver/server.go:119-122  payload := map[string]any{..., "数据": ent.Data}
  # MCP 工具写：先 ent.Set 再 store.Save
  internal/mcpserver/server.go:135-145
  ```
  构造序列：会话 A 由模型调用 `entity_state_set`（写 `状态.情绪`），同一时刻控制面 `POST /tick` 或插件每 60 s 的 `/tick` 进入 `store.Save()` → `json.MarshalIndent` 在遍历 `ent.Data` 时与 `Entity.Set` 的写并发 → Go 运行时 `fatal error: concurrent map read and map write`（**不可 recover**，进程直接死）。
- 红队结论: 这是本方案**新引入**的并发面（此前只有 MCP 会话内并发；现在常驻的 60 s tick + 5 s 状态轮询 + 模型工具调用三股流量共存）。c2 声明的不变量「引擎访问串行化」在 K1 里并未落到 `/state` 与 `/tick` 之间，K1.3 的单飞只覆盖 `/tick` 自身。进程一死就触发 A4-1 的残留 socket 自锁，形成「崩一次 → 再也起不来」的级联。
- 建议处置: 固化为 `C-R7`（进程内单一 `serializer`/互斥覆盖 **引擎 tick + store.Save + ent 读写**；`-race` 下必须有并发用例；禁止在锁外暴露 `ent.Data`）。
- 建议严重级: **高**

### A2-2 K1.3 的「同一时刻只允许一个 tick」被 MCP 工具 `state_machine_tick` 绕过

- 反例:
  ```
  internal/mcpserver/stateTools.go:32-40   // MCP 工具体：Tick + recordTick + logger + store.Save()，无任何单飞标志
  15-interfaces.draft.md:37               「同一时刻只允许一个 tick；已有 tick 在飞 → 409」
  internal/engine/engine.go:177-179        // Tick 自己拿 m.mu，所以第二个 tick 会**阻塞排队**而不是 409
  ```
  构造序列：插件在 T 时刻 `POST /tick`（拿到 200 后仍在执行）→ 模型同轮调用 `state_machine_tick` → 引擎锁让它排队，本周期内执行了两次 tick，且两次 `store.Save()` 的写入顺序不确定。
- 红队结论: 「Tick 不重入」这条 c4 不变量的定义域只覆盖了一个入口；模型可以从工具面合法地让同一周期推进两次状态机。契约要么把单飞提升到 engine 层，要么明确「MCP 工具不受约束」并放弃该不变量。
- 建议处置: 固化为 `C-R8`（单飞必须覆盖两个入口，或删除「Tick 不重入」声明）。
- 建议严重级: 中

### A2-3 两个进程共写 `data/`：无 flock、`atomicWrite` 用固定 `.tmp` 名

- 反例:
  ```
  internal/entity/store.go:171-180   tmp := path + ".tmp"; WriteFile(tmp); Rename(tmp,path)
  internal/entity/store.go:116-120   Save() 只拿进程内 s.mu（跨进程无效）
  DSH 侧：每个 DSH 进程起一个 mcp-client 实例（mcp-client/src/index.ts:154-203，作用域是 ctx.root），
          即 DSH 多开（web + tui、或 web + headless）就会有多个 dsh-mcp 子进程。
  ```
  构造序列：两个 DSH 进程 → 两个 `dsh-mcp` → 各自 `Load()` 全量实体到内存 → 任一方 tick 后 `Save()` 用**整文件覆盖**，后写者抹掉前者全部新状态；更坏的是两者共用 `entities.json.tmp`，一个的 `Rename` 可能搬走另一个**写了一半**的临时文件。
- 红队结论: 「同一份 `data/` 只被一个进程持有」是方案未声明的隐含前提；K1 只处理了 socket 冲突的**一半**（见 A4-1），没有处理数据面。多开是 DSH 的正常用法（桌面端 + CLI）。
- 建议处置: 固化为 `C-R9`（单实例锁：`data/.lock` 的 flock 或 socket 语义扩展；`.tmp` 名带 pid/随机后缀）。
- 建议严重级: **高**

### A2-4 9 处机器绝对路径进了版本库内的 bundle patch，与 profile 现有先例相反

- 反例:
  ```
  15-interfaces.draft.md:70-89  command / args(-data-dir,-scripts-dir,-config) / cwd / socketPath / workspacePrefix
                                共 9 处 /home/ruicat/Desktop/Neko/...
  00-scope.md:58  「位置：/home/ruicat/Desktop/Neko/dsh/（随仓库版本化）」
  反例（本机先例）：~/.dsh/profiles/web/cordis.patch.yml 尾部注释——
    「绝对路径不进版本库——本文件本身是可交付的作者副本，保持占位符」，由 .tools/installMcp.sh 渲染
  ```
  另：`config.json` 在仓库中**不存在**（只有 `docs/config.example.json`），`internal/config/config.go:103-114` 对缺失文件返回默认值 → 该参数是静默空转。
- 红队结论: 仓库搬迁/换机/别人 clone 后，bundle 仍指向旧路径：MCP 子进程 `command` 不存在 → 重连 10 次后工具消失（A4-4），bridge 连不上 socket → 提示块 3 次失败后变空——整套失败都是**静默降级**，没有任何一条门会红。这与本机「占位符 + 渲染脚本」的既定做法直接冲突。
- 建议处置: 固化为 `C-R10`（bundle patch 只允许占位符；安装/验证脚本负责渲染；`grep -n '/home/' dsh/cordis.patch.yml` 必须为 0）。
- 建议严重级: 中

### A2-5 「`/tick` 与 MCP 工具语义完全一致」的复用点在 m2 写域之外

- 反例:
  ```
  internal/mcpserver/stateTools.go:27-41   // tick 体（Tick+recordTick+logger+snapshot+Save）写死在 AddTool 闭包里，未导出
  10-generalization.md:33,37               // m2 写域 = internal/control + cmd/dsh-mcp；m1 写域 = internal/bootstrap + cmd/astrbox-mcp
  ```
- 红队结论: 要满足 C10（≥2 目标共用逻辑必须提取）就得改 `internal/mcpserver/stateTools.go`——不在任何人写域内；不改就复制一份 tick 体，直接违反 C10 且让「完全一致」变成两份会漂移的实现。这是**写域划分缺口**，不是实现细节。
- 建议处置: 固化为 `C-R11`（把共享 tick 体显式划给 m1，或新增第三个写域并写进 40-plan）。
- 建议严重级: 中

### A2-6 插件不导出 `Config` schema 时配置键静默失效

- 反例:
  ```
  vendor/cordis/src/fiber.ts:45-50  resolveConfig(): 「returns the validated config,
                                    or `config` unchanged if the runtime has no schema」
  15-interfaces.draft.md:129        // K2.1 只规定 package.json 字段，未规定导出 Config
  ```
- 红队结论: `socketPath` / `promptMaxChars` / `tickEveryMs` / `requestTimeoutMs` 拼错（或层级写错）时不会报错，插件用默认值跑；V4 只看 patch 文本、V5 只看「有没有状态块」，两者都发现不了「配置没生效」。
- 建议处置: 固化为 `C-R12`（包必须导出 schemastery `Config`；`--dump-config-schema` 必须 `complete: true`）。
- 建议严重级: 中

### A2-7 每次 Tick 无条件全量重写两份实体文件（写放大）

- 反例:
  ```
  internal/mcpserver/stateTools.go:37-39   if err := s.store.Save(); err != nil ...   // 无条件
  internal/entity/store.go:122-139         遍历**全部**实体，MarshalIndent 后写 entities.json + user/entities.json
  $ ls -l data/entities.json data/user/entities.json → 47823 B / 47408 B
  ```
- 红队结论: 60 s 一次 → 1440 次/天 × ~95 KB（缩进 JSON，压缩前）≈ 130 MB/天 的持续写入，且每次都是全量 `MarshalIndent`；状态没有变化时也照写（`Tick` 只改内存，`Save` 无条件）。长跑（数天）会显著放大 btrfs 写入与快照体积，而 c4/c7 的资源预算里没有这一项。
- 建议处置: 固化为 `C-R13`（tick 无 transition 时跳过 Save；或 Save 走 dirty 标记）。
- 建议严重级: 中

### A2-8 控制面日志无轮转，且请求频率被 5 s 轮询放大

- 反例:
  ```
  15-interfaces.draft.md:48-52   「控制面每个请求追加一行到 <data-dir>/logs/dsh-control.log」——无轮转/无上限
  对照：internal/engine/stateLogger.go:44-52  按日分文件 + pruneLocked（保留 7 天）
  K2.2 promptRefreshMs = 5000 → 每 5 s 至多一次 /state
  ```
- 红队结论: 8 h 会话 ≈ 5760 次 `/state` + 480 次 `/tick` 的行；单文件无限增长，与仓库既有「按日 + 保留」约定不一致，且无人清理。
- 建议处置: 并入 `C-R3`（日志口径统一）。
- 建议严重级: 低

### A2-9 装配提取后的共享文件长度风险

- 反例:
  ```
  cmd/astrbox-mcp/main.go:38-90   // 53 行接线，提取后 internal/bootstrap 会成为「多功能装配」文件
  00-scope.md:35 约束：单功能 <500 / 多功能 <1000 有效行
  ```
- 红队结论: 目前不是违规（距离阈值很远），但 `bootstrap` 很容易变成「什么都往里塞」的装配桶；评审时应要求它只做接线、不承载 tick/control 逻辑。
- 建议处置: 记为疑点 H3。
- 建议严重级: 低

### A2-10 插件刷新失败降级为「空字符串」后无法区分「没数据」与「服务挂了」

- 反例:
  ```
  15-interfaces.draft.md:107-109  「连续失败 ≥3 次后改为返回空字符串（section 仍注册但不产生内容）」
  ```
- 红队结论: 空 section 与「猫状态全缺失」在提示里完全同形；V5 的「系统提示含状态块」在服务挂掉时也只是变成「没有这一块」，没有任何可查信号（日志只有首次失败一条）。
- 建议处置: 并入 `C-R1`（降级时保留一行固定告警文本或计数）。
- 建议严重级: 中

### A2-11 `data/dsh-control.sock` 未进 `.gitignore`，`make clean` 也不清

- 反例:
  ```
  .gitignore        含 /data/logs/、/data/entities.json、*.tmp，**不含** data/dsh-control.sock
  Makefile:24-26    clean: rm -rf bin（不碰 data/）
  ```
- 红队结论: 每次运行后工作区出现一个未跟踪的 socket 文件；V6「仓库工作区无残留」的检查会把它算进来（或反之被忽略），且崩溃残留的 socket 就是 A4-1 的自锁源文件，没人清。
- 建议处置: 并入 `C-R9`（socket 落点与清理写进契约）。
- 建议严重级: 低

### A3-1 谱系 A 的模块边界：找不到真正的循环依赖，但「谁创建 control listener」未定，存在一条潜在环

- 反例（枚举的可构成环的边）:
  ```
  m1 internal/bootstrap  → {entity,engine,events,function,registry,mcpserver}   （无上游）
  m2 internal/control    → mcpserver（若要复用 tick 体）/ bootstrap（若要 Runtime 类型）
  cmd/dsh-mcp            → bootstrap + control                                  （单向）
  m3 dsh/*.mjs           → 只经 K1 HTTP 契约，不 import Go 包                   （无代码边）
  m4 docs/Makefile       → 产物                                                （无代码边）
  # 唯一可能成环的组合：bootstrap(m1) 里创建 control listener → import control(m2)，
  # 而 control(m2) 又 import bootstrap(m1) 取 Runtime → m1↔m2 环。
  ```
  当前 `15-interfaces.draft.md` 未规定 listener 由谁创建（c3 说「cmd/dsh-mcp（stdio MCP + control listener）」，倾向放在 m2，则不构成环）。
- 红队结论: **未击穿**。A 谱系的四条边是单向的，m3 与 Go 侧只通过 K1 文本契约耦合（无 import 边）；只要 listener 归 m2 且 `bootstrap` 不反向 import `control`，不会成环。条件是 G3 冻结时必须写明「listener 归 m2」。
- 建议处置: 不改谱系；写入 H1 观察项（冻结时确认归属）。
- 建议严重级: 低

### A4-1 崩溃/SIGKILL 残留的 socket 会让 dsh-mcp **永久拒绝启动**，且 mcp-client 会静默放弃

- 反例:
  ```
  15-interfaces.draft.md:10  「启动失败（socket 已存在且不可连）→ 进程以非 0 退出并在 stderr 给出原因」
                             // 全文没有任何「尝试连接→ECONNREFUSED→unlink→重试」的自愈分支
  # 进程被 SIGKILL 时不可能执行「退出时删除 socket 文件」（K1:10 的对称义务）
  packages/mcp/mcp-client/src/connection.ts:211-245  重连默认 10 次后 giving up，工具全部 unregister
  packages/mcp/mcp-client/src/index.ts:128           failOnStartupError 默认 false
  ```
  构造序列：`kill -9 dsh-mcp`（或 OOM / 宿主强杀 / A2-1 的 fatal）→ `data/dsh-control.sock` 残留 → 下次 DSH 启动 spawn → dsh-mcp 见 socket 存在且 `connect` 得 ECONNREFUSED → 非 0 退出 → mcp-client 重试 10 次（500ms→30s 退避，约 2-3 分钟）→ 放弃并注销工具 → 本会话 `mcp__astrbox__*` 全部消失，提示块 3 次失败后变空。**恢复只能手工 `rm` 那个 socket**，而契约没有要求 dsh-mcp 打印这个可执行提示（只说「在 stderr 给出原因」）。
- 红队结论: 这是一条「一次崩溃 → 永久失效」的自锁路径，且失败完全静默（宿主启动成功、无红色门）。它是本方案最尖锐的运维缺口。
- 建议处置: 固化为 `C-R14`（stale socket 自愈：connect 探测 → ECONNREFUSED/ENOENT 时 unlink 重试一次；`runtime` 内进程退出删 socket 用 defer+信号；错误消息必须含 `rm <path>` 指令）；并 `C-R15`（K2.1 加 `failOnStartupError: true`）。
- 建议严重级: **高**

### A4-2 K1 未定义「socket 已存在且**可连**」的第二个实例行为

- 反例:
  ```
  15-interfaces.draft.md:10  只写了「已存在且不可连 → 非 0 退出」
  K1:9                        「方向：DSH 插件 → Go 服务（请求/响应）；Go 服务不主动连插件」
  ```
- 红队结论: 第二个 dsh-mcp 启动时（DSH 多开，或前一个进程仍活着但 mcp-client 已重连 spawn 了新的）会遇到「socket 存在且可连」。契约没写：抢锁退出？unlink 抢占（→ 第一个进程仍在跑却失去路径，插件连到第二个进程，两进程共写库 → A2-3）？还是静默复用？实现者任选其一都可能造成数据面分裂。
- 建议处置: 并入 `C-R14`（明确「可连 → 非 0 退出并可读原因」；或改 lock 文件 flock）。
- 建议严重级: **高**

### A4-3 客户端 1500 ms 与服务端 5 s 期限不一致：慢 tick 期间插件必判失败，但服务端日志是 200

- 反例:
  ```
  15-interfaces.draft.md:11   「服务端处理期限 5s」
  15-interfaces.draft.md:89   requestTimeoutMs: 1500
  15-interfaces.draft.md:109  「连续失败 ≥3 次后改为返回空字符串」
  15-interfaces.draft.md:115  「收到 409 tick_busy → 记为跳过」
  ```
- 红队结论: 一次超过 1.5 s 的 `/tick`（`.neko` 函数执行、大库 `Save()`）会让插件的 `/state` 请求超时 → 提示块变空；而服务端 K1.5 记的是 `200`。两边日志对同一事实给出相反结论，排障时无法定位。此外插件的「单飞」在 1.5 s 就解除，服务端仍在跑，下一次 tick 会拿到 409——被记为「跳过」，于是「Tick 每 60 s 一次」在慢机器上静默变成「每 120 s 一次」。
- 建议处置: 固化为 `C-R16`（客户端超时 ≥ 服务端期限 + 余量，如 6000 ms；或服务端 `/state` 走独立短期限并明确 503）。
- 建议严重级: 中

### A4-4 `Save()` 失败返回 500，但内存已改：无回滚语义

- 反例:
  ```
  15-interfaces.draft.md:38  「500 internal（Tick 内部错误或 Save 失败，message 带原因）」
  internal/engine/engine.go:174-176  「Tick mutates in-memory state only and never persists」
  internal/mcpserver/stateTools.go:32-39  Tick 后再 Save，失败只 fail()
  ```
- 红队结论: 500 之后内存已被推进、磁盘没有；下一次 tick 从「已推进」的内存继续，重启后回退到上一次成功落盘的状态。契约没规定「500 = 本次 Tick 未生效」，调用方（插件）也无法区分，只能当失败重试（但重试会再推一次）。
- 建议处置: 固化为 `C-R17`（500 的语义必须写成「内存已变更、磁盘未落盘」并暴露 dirty 标志，或 Save 失败时回滚内存）。
- 建议严重级: 中

### A4-5 mcp-client 子进程失败被静默吞成重连（未置 `failOnStartupError`）

- 反例:
  ```
  packages/mcp/mcp-client/src/index.ts:128     failOnStartupError: z.boolean().default(false)
  packages/mcp/mcp-client/src/index.ts:199-202 if (outcome.error !== undefined && config.failOnStartupError) throw
  15-interfaces.draft.md:65-75                 K2.1 的 mcp-astrbox 配置**没有**这一项
  ```
- 红队结论: `bin/dsh-mcp` 不存在、不可执行、参数拼错、socket 冲突（A4-1）时，DSH 启动**依然成功**，只在日志里 warn；门禁（V1/V2/V4）全部可以绿。`command` 指向 `bin/dsh-mcp` 只是文本，V4 也不 stat 它。
- 建议处置: 并入 `C-R15`（并在 V4 加 `test -x bin/dsh-mcp`）。
- 建议严重级: **高**

### A4-6 `--dump-config` 与 `--dump-config-schema` 都会改写 profile，回滚动作本身有副作用

- 反例:
  ```
  apps/cli/src/profile-boot.ts:194-200   prepareProfile 无条件 writeFileSync(cordis.yml)
  apps/cli/src/dump-config-schema.ts:30  runDumpConfigSchema 也调用 prepareProfile
  packages/boot/app-boot/src/profile.ts:287-301  removeLinkProjections 会 unlink profile 内的 symlink
  ```
- 红队结论: V6 说「仓库工作区无残留 profile 写入」，但真正的写入点在 profile（`cordis.yml` 每次被重写）；只读挂载下这条路径直接抛 EROFS（A1-4 实测）。回滚验证必须明确「profile 内容按字节不变」的基线，而不是「仓库干净」。
- 建议处置: 并入 `C-R4`。
- 建议严重级: 中

### A5-1 控制面无鉴权 + socket 路径公开：模型自己的 bash 工具就是合法攻击面

- 反例:
  ```
  15-interfaces.draft.md:11   「无鉴权（依赖 socket 文件权限）」
  15-interfaces.draft.md:80   socketPath 明文写在版本库内的 dsh/cordis.patch.yml
  ~/.dsh/profiles/web/cordis.patch.yml 现成先例：mcp-client 的 command/args 也是明文绝对路径
  会话本身带 bash 工具（本会话可用），curl 支持 --unix-socket
  ```
  构造序列：模型（被提示注入、或自主）执行
  `curl --unix-socket /home/ruicat/Desktop/Neko/data/dsh-control.sock -X POST -d '{"kind":"session-created","sessionId":"x","cwd":"/home/ruicat/Desktop/Neko"}' http://localhost/session-event`
  → 若 `.neko` 注册了 `dsh_on_event`，同一请求可改写实体状态；`POST /tick` 可无限推进状态机（每次还带一次全量落盘）。
- 红队结论: 「仅本机暴露面」在**智能体**威胁模型下不成立：同 uid 的进程/工具就是「本机」。0600 socket 挡得住其他用户，挡不住模型自己的 shell。K1 未定义速率限制、未定义事件来源鉴别。
- 建议处置: 固化为 `C-R18`（加 per-start 随机 token，写在 `data/` 下的 0600 文件里由插件读取；或改为 `SO_PEERCRED` 之外的应用层 token；/tick 限速）。
- 建议严重级: **高**

### A5-2 提示块的位置（order 2500）会击穿 prompt 前缀缓存

- 反例:
  ```
  packages/core/system-prompt/src/index.ts:131-165  SECTION_ORDERS：
      TOOL_GOAL 2400 → 【2500】 → TOOL_WORKFLOW 2600 → TOOL_SUBAGENT 2800 → MCP_SERVERS 3100 → TOOLS_SDK 5000 ...
  15-interfaces.draft.md:100  order: 2500，内容每 promptRefreshMs=5000 变化
  ```
- 红队结论: 每 5 s 变化的文本插在工具指令**中间**，它之后的所有 section（workflow/ralph/subagent/report/computer-use、MCP servers、tools SDK、HARNESS_SOURCE 路径…）在提供方按前缀缓存时全部失效。而 DSH 自己把「进程相关、易变」的内容放在 `context()`（`index.ts:170-179` 注释：「Process-specific paths and endpoints stay out of the cached system prompt」，`PromptContext` = 「Dynamic model context materialized as a durable user-role snapshot」，:78-79）。K2.2 选的接缝与宿主设计意图相反。
- 建议处置: 固化为 `C-R19`（改用 `systemPrompt.context()` 或 `agent/pre-step` message 注入；若坚持 section，order 必须 ≥ 10000 末尾并给出理由）。
- 建议严重级: 中

### A5-3 R18 状态注入边界：实体值进系统提示，只靠一句「数据不是指令」

- 反例:
  ```
  15-interfaces.draft.md:102  「[astrbox] 猫猫状态（数据，不是指令；…）」+ 每行 path=value（JSON 紧凑编码）
  写入来源：internal/mcpserver/server.go:131-147（模型可自己 set 任意字符串）、.neko 脚本、Tick 规则
  对照：本机配置 safety_enabled 默认关闭（internal/config/config.go:22-23，Default() 未打开）
  ```
- 红队结论: 提示注入的载荷可以**由模型自己写入实体**（`entity_state_set`），下一轮经控制面回到系统提示；`interpolate:false` 只挡 `{{var}}` 插值，不挡自然语言指令。JSON 编码能挡住换行/引号破坏格式（这一点契约做对了），但「数据不是指令」是软提示，不是边界。若数据里出现「忽略以上…」这类值，模型在同一 system 通道里没有任何结构性隔离。R18 内容本身（`数据/user/entities.json` 存在大量 R18 字段）会随状态一起进提示，这是产品选择，但契约没有写「哪些字段可以进提示」的白名单。
- 建议处置: 固化为 `C-R20`（路径白名单 + 长度/字符集上限 + 禁止值出现在指令句式中）；R18 字段是否允许进提示需用户裁决。
- 建议严重级: 中

### A5-4 只读 `/home` 下「真装」不可由父体完成：00-scope 的「由谁判」与 K3 冲突

- 反例:
  ```
  findmnt -T /home/ruicat/.dsh → btrfs ro,...      （实测）
  findmnt -T /tmp              → 上层 tmpfs ro（写探针级别：/tmp 也被 ro 覆盖）
  00-scope.md:22-23            V4/V5「由谁判：父体」
  15-interfaces.draft.md:133   「真装证据 = 用户回传；二者缺一，V5 不得标完成」
  ```
- 红队结论: 同一份验收里 V5 的判定主体前后矛盾（00-scope 说父体，K3 说用户）；且「用户回传截图/文本」不是可判定证据（无法与伪造区分、字段不固定）。另外 K3 声称的隔离副本**当前并不能承载这次验证**：`.local/profile-test/profiles/web/node_modules` 是指向 `~/.dsh/profiles/web/node_modules` 的 symlink（只读），`dsh-astrbox` 的 link 条目无处安放；`package.json` 是原样拷贝（`bundles` 里没有 `dsh-astrbox`），`dump.txt` 1586 行里 `astrbox` 出现 **0** 次。
- 建议处置: 固化为 `C-R21`（证据字段集：dump-config 两行原文 + `ls -l` socket 权限 + `dsh-control.log` 两类行 + 工具清单 JSON；缺任一字段即「未验」）。
- 建议严重级: **高**

### A5-5 K3 的 `node_modules/dsh-astrbox -> ../../../../dsh` 相对路径差一级

- 反例（实测计算）:
  ```
  $ node -e 'const p=require("node:path");const b=p.resolve(".local/profile-test/profiles/web/node_modules");
    for (const r of ["../../../../dsh","../../../../../dsh"]) console.log(r,"->",p.resolve(b,r))'
  ../../../../dsh -> /home/ruicat/Desktop/Neko/.local/dsh        ← K3 文本给的
  ../../../../../dsh -> /home/ruicat/Desktop/Neko/dsh            ← 实际需要
  ```
- 红队结论: 按 K3 字面建出的 symlink 指向不存在的 `.local/dsh`，`pnpm`/Node 都会报模块找不到；而 `loadProfile` 对不可解析的 bundle **只打一条 skipped 警告**（`packages/boot/app-boot/src/profile.ts:760-775` + `reportSkippedBundles:118-124`），不会让 `--dump-config` 失败 → 验证者会看到「dump 成功但没有那两行」并误以为插件没写对。
- 建议处置: 并入 `C-R10`（路径由脚本计算，不写死在文档里）。
- 建议严重级: 中

### A6-1 V4 断言的两行 id 与 K2.1 不一致，且该命令在本会话不可执行

- 反例:
  ```
  00-scope.md:22    「输出含 bundle 两行（`neko-bridge` 与 `mcp-astrbox`）」
  15-interfaces.draft.md:65,77   实际 id：`mcp-astrbox` 与 **`astrbox-bridge`**
  $ node .../bin.js --profile web --dump-config  → EXIT=1, EROFS（见 A1-4）
  ```
- 红队结论: 按 00-scope 的判据 grep `neko-bridge`，**永远 grep 不到**；这条门在当前环境按字面执行也跑不通。两个缺陷叠加，V4 的实际状态是「写了判据、没有可执行判定」。
- 建议处置: 固化为 `C-R22`（判据用 K2.1 的 id，命令带 DSH_HOME，断言退出码 0 + 两个 id 各出现一次 + `command` 路径 `test -x`）。
- 建议严重级: **高**

### A6-2 `--dump-config` 不 import 插件、不校验 schema、不 spawn 子进程：绿了也不代表能加载

- 反例:
  ```
  apps/cli/src/dump-config.ts:5     「without booting or evaluating !!js」
  apps/cli/src/dump-config.ts:39-41 collectConfigDumpLayers → renderConfigDump（纯文本合成）
  对照：apps/cli/src/dump-config-schema.ts:6,9,38  「Imports and lazy schema builders execute trusted module code」
                                                    且 `if (!dump['x-cordis'].complete) process.exitCode = 1`
  ```
- 红队结论: V4 能证明的是「bundle 的 package.json 可解析 + patch 文本里有两行 insert」，**不能**证明 `main` 可 import、`Config` schema 合法、`apply()` 不抛错、子进程能 spawn。现成的、无需模型与网络的加载面探针是 `--dump-config-schema`（会 import 插件模块并给出 `complete`），K3 却把它漏了、退而写「headless 启动（或 dump/config 面证据）」。
- 建议处置: 固化为 `C-R23`（加载面 = `--dump-config-schema` 且 `x-cordis.complete === true` + 直接 spawn `bin/dsh-mcp` 跑 MCP `initialize`/`tools/list`；「或 dump/config 面证据」这句删掉）。
- 建议严重级: **高**

### A6-3 V5 的四条「真实生效」没有一条给出机器可查条件

- 反例:
  ```
  00-scope.md:23  「新会话工具表出现 mcp__astrbox__*；系统提示含状态块；Tick 日志按周期出现；会话事件日志有记录」
  ```
- 红队结论: 「工具表出现」要看模型请求头；「系统提示含状态块」没有 CLI 出口；「Tick 日志」口径不明（A1-3）；「会话事件日志有记录」可查（`dsh-control.log`）。四条里三条只能靠人看。可机器化的替代路径已经存在：`grep -c mcp__astrbox__` 在会话 request-header/日志里、`GET /state` 直打 socket、`grep ' POST /tick '` 在 `dsh-control.log`。
- 建议处置: 固化为 `C-R24`（V5 拆成 4 条命令，各自给退出码；真装只补「新会话工具表」一条）。
- 建议严重级: **高**

### A6-4 K3「或 dump/config 面证据」把加载面降级成零增量

- 反例:
  ```
  15-interfaces.draft.md:131  「…断言：插件 apply 无报错、dsh-control.sock 出现、MCP 子进程可列出工具（**或 dump/config 面证据**）」
  ```
- 红队结论: 括注允许用「已经在 V4 做过的 dump」替代「加载面」，于是新增门可以合法退化为零；同时 K3 的 headless 启动若真的做，会发起一次模型调用（需要网络/额度），与 00-scope 的「不引入网络服务」和本会话「禁止联网」冲突。
- 建议处置: 并入 `C-R23`（删「或」，并明确加载面不消耗模型调用）。
- 建议严重级: **高**

### A6-5 V2 的「集合等于 cmd/astrbox-mcp 当前清单」是循环判据

- 反例:
  ```
  00-scope.md:20   「tools/list 含现有 17 个工具名（集合等于 cmd/astrbox-mcp 当前清单）」
  # 实际清单（17 个，grep mcp.AddTool 计数=17）：
  entity_state_get, entity_state_set, entity_query, entity_create, state_machine_tick,
  state_machine_resolve, neko_script_list, neko_script_read, neko_script_write,
  neko_script_validate, neko_script_apply, neko_script_remove, diagnostics_query,
  inventory, stats, entity_snapshot, event_log
  ```
- 红队结论: 「等于当前清单」在两侧同源时可同时丢一个工具而仍相等；V7 虽然写了「工具清单不变」，但同样没给冻结文本。红队在此把 17 名清单钉死，供 G3 直接抄进断言。
- 建议处置: 固化为 `C-R25`（清单文本进入 40-plan/60-verification；断言用精确集合比较）。
- 建议严重级: 中

### A6-6 「用户回传证据」不可判定：没有字段集、没有校验方式

- 反例:
  ```
  15-interfaces.draft.md:132-133  「回传 --dump-config 片段与新会话工具表截图/文本」+「二者缺一，V5 不得标完成」
  ```
- 红队结论: 「片段」允许截取、截图不可机验、也没有要求时间戳/pid/日志行；任何「看起来有」的回传都能过。这对 L3 的 G4 门禁是空心判据。
- 建议处置: 并入 `C-R21`（最小字段集 + 校验脚本 `scripts/verifyProfile.sh <evidence.txt>` 的退出码）。
- 建议严重级: 中

### A6-7 `--dump-config` 的输出规模与本仓库 profile 现状（16 个 bundle）会让「肉眼比对」不可行

- 反例:
  ```
  $ ls ~/.dsh/profiles/web/package.json → dsh.profile.bundles 16 项
  $ wc -l .local/profile-test/dump.txt → 1586 行
  ```
- 红队结论: 验收必须用 `grep -c` 精确计数与行号，不能「看一下输出里有」；否则「两行存在」与「两行重复插入」无法区分（重复 insert 会 panic 或双份工具，见 mcp-client 的 serverName 独占检查 `index.ts:162-176`）。
- 建议处置: 并入 `C-R22`（每 id 出现次数 = 1）。
- 建议严重级: 低

### A6-8 K4 的 `make test` 在无 node 测试时对 glob 的行为未定义

- 反例:
  ```
  15-interfaces.draft.md:138  「node --test dsh/lib/*.test.mjs（无 node 测试时 SKIP 并说明）」
  # shell 在无匹配时把字面量传给 node，node 报 Cannot find module 并非 0 退出
  ```
- 红队结论: 「SKIP 并说明」需要显式 `[ -e ... ] || exit 0` 或 `shopt -s nullglob`，契约没写；否则 m2 先合入、m3 未合入的批间状态会让 `make test` 红，误判为回归。
- 建议处置: 并入 `C-R23`（给出确定的 SKIP 写法）。
- 建议严重级: 低

## C 候选（可判定，供 G3 冻结）

> 每条：断言 + 判定命令 / 机器可查条件 + 建议严重级。编号 `C-Rn` 仅为本轮红队候选，冻结后按 `40-plan.md` 重编号为 `C1..Cn`。

| # | 断言 | 判定命令 / 机器可查条件 | 严重级 |
|---|---|---|---|
| C-R1 | `systemPrompt.section` 的 `text()` 必须是纯读缓存；`/state` 刷新由显式驱动（`ctx.interval` 或 assemble 钩子）完成，且首次刷新前返回固定占位文本而不是空串 | 单测：连续 3 次 `assemble()` 期间 stub 的 `/state` 只被调用 0 次（缓存未过期）；推进假时钟后 1 次；`text()` 不得返回 Promise | 高 |
| C-R2 | `session/created` / `agent/turn-stopping` 监听器**不得同步抛错**；`cwd` 必须先判 `typeof === 'string'` | 单测：构造 `header.cwd === undefined` 的会话 → `create()` 仍成功、插件不发请求；`grep -n 'header.cwd.startsWith' dsh/lib/*.mjs` 必须为 0（需先判类型） | 高 |
| C-R3 | Tick 证据钉在 `dsh-control.log` 的 `/tick` 行；日志按日分文件并保留 N 天（与 `stateLogger` 一致） | `grep -c ' POST /tick ' data/logs/dsh-control.log` 在 2×周期后 ≥1；`ls data/logs/dsh-control-*.log` 存在 | 中 |
| C-R4 | 任何 `--dump-config` / `--dump-config-schema` 调用必须带隔离 `DSH_HOME` 且退出码 0；真 profile 只允许只读证据 | `DSH_HOME=$PWD/.local/profile-test node /home/ruicat/Desktop/deepseek-harness/apps/cli/lib/bin.js --profile web --dump-config > .local/profile-test/dump.txt; echo $?` == 0；不带 `DSH_HOME` 的调用必须被判为「不可执行」而不是「组合失败」 | 高 |
| C-R5 | `/state` 的截断语义不得返回「只有 `__truncated__`」；残留的「413？不：见下」式未决文本必须清零 | `grep -nE '？|不：见下' 15-interfaces.draft.md` 为 0；`internal/control` 单测覆盖 maxBytes 越界与截断 | 低 |
| C-R6 | socket 文件 0600 且父目录 0700（显式 chmod，容忍已存在的 0755） | `stat -c '%a' data/dsh-control.sock` == 600；`stat -c '%a' data` == 700 | 低 |
| C-R7 | 进程内**单一串行器**覆盖：引擎 Tick、`store.Save()`、`Entity` 读写；`Store` 不得在锁外暴露 `ent.Data` | `go test -race ./internal/...` 绿；新增并发用例（tick ∥ entity_state_set ∥ /state）在 `-race` 下无报告；`grep -n '"数据": ent.Data' internal/mcpserver` 为 0（改为深拷贝） | 高 |
| C-R8 | Tick 单飞覆盖 `/tick` 与 MCP `state_machine_tick` 两个入口（或删除「Tick 不重入」声明） | 并发测试：同时发起两者，`transitions` 只应来自一次 tick，或第二次返回 409/明确错误 | 中 |
| C-R9 | 单实例锁（`data/` 级 flock 或 socket 语义扩展）；临时文件名带 pid/随机串；socket 落点写进 `.gitignore` | 并发启动两个 dsh-mcp：第二个非 0 退出并打印原因；`grep -n 'dsh-control.sock' .gitignore` 命中 | 高 |
| C-R10 | bundle patch 内不得出现机器绝对路径；路径由安装脚本渲染；符号链接目标由脚本计算 | `grep -nE '/home/|/Users/' dsh/cordis.patch.yml` 为 0（或只允许占位符）；`readlink -f` 后目标存在 | 中 |
| C-R11 | 共享 tick 体必须有明确写域归属（m1 或新增写域），禁止复制 | `grep -rn 'func.*Tick' internal/control internal/mcpserver` 指向同一实现；40-plan 写域表包含该路径 | 中 |
| C-R12 | 插件包必须导出 schemastery `Config`，加载面必须 `x-cordis.complete === true` | `--dump-config-schema` 输出中 `x-cordis.complete` 为 true；把 `socketPath` 改成拼错键 → schema 校验应报错（或诊断 `complete:false`） | 中 |
| C-R13 | 无 transition/conflict 的 tick 不得全量落盘 | 单测：连续 5 次无变化 tick，`entities.json` 的 mtime 不变 | 中 |
| C-R14 | stale socket 自愈：connect 探测 → ECONNREFUSED/ENOENT 时 unlink 重试一次；「可连」时非 0 退出并给原因；错误消息含 `rm <path>` | 手工构造：`kill -9` 子进程后重启，dsh-mcp 能自愈（或提示可执行命令）；第二个实例退出码非 0 | 高 |
| C-R15 | K2.1 的 mcp-astrbox 必须 `failOnStartupError: true`，V4 必须 `test -x bin/dsh-mcp` | `grep -n 'failOnStartupError' dsh/cordis.patch.yml` 命中 true；`test -x bin/dsh-mcp` 退出 0 | 高 |
| C-R16 | 客户端超时必须 ≥ 服务端处理期限 + 余量（如 6000 ms），且慢 tick 期间的状态读取走独立短期限 | `grep requestTimeoutMs` ≥ 6000；/state 在 tick 期间返回 200 或 503，不得让插件侧超时；两侧日志对同一次请求状态一致 | 中 |
| C-R17 | `500 internal` 的语义必须写明「内存已变更、磁盘未落盘」并暴露 dirty 标志，或 Save 失败时回滚内存 | 单测：注入 Save 失败 → 下一次 tick 的输入状态 == 失败前状态（回滚）或响应含 `dirty:true` | 中 |
| C-R18 | 控制面必须有应用层 token（`data/` 下 0600 随机文件）与 `/tick` 速率限制；无 token 请求 401 | 不带 token 的 `curl --unix-socket` 返回 401；带 token 返回 200；插件从同一文件读取 token | 高 |
| C-R19 | 猫状态不得进系统提示的中间段：改用 `systemPrompt.context()` 或 `agent/pre-step` 注入；若用 section，order ≥ 10000 | `grep -n 'order: 2500' dsh/lib/*.mjs` 为 0；或单测断言 section 列表顺序在 `HARNESS_SOURCE` 之后 | 中 |
| C-R20 | 进提示的路径必须白名单、值长度 ≤ N、字符集受限；R18 字段默认不进提示 | 单测：注入超长/含指令句式的值 → 提示文本被截断且只含白名单路径；`grep -n '性行为\|器官' dsh/lib/*.mjs` 为 0 | 中 |
| C-R21 | 真装证据必须是最小字段集，且由脚本判定 | `scripts/verifyProfile.sh <evidence.txt>` 退出 0 需要：两个 id 各 1 行 + `stat` 输出 600 + `dsh-control.log` 的 `/tick` 与 `/session-event` 行 + 工具清单 17 名 | 高 |
| C-R22 | V4 断言用 K2.1 的 id（`mcp-astrbox` / `astrbox-bridge`），每 id 出现次数 == 1，`command` 可执行 | `DSH_HOME=... --dump-config \| grep -c 'id: mcp-astrbox'` == 1；同 `astrbox-bridge`；`test -x $(dump \| yq '.command')` | 高 |
| C-R23 | 加载面 = `--dump-config-schema`（`complete:true`）+ 直接 spawn `bin/dsh-mcp` 的 MCP `initialize`/`tools/list`；删除「或 dump/config 面证据」；无 node 测试时 `make test` 必须确定性 SKIP | `--dump-config-schema` 退出 0 且 `jq '.x-cordis.complete'` == true；stdio e2e 退出 0；`make test` 在只有 Go 测试时退出 0 | 高 |
| C-R24 | V5 拆成 4 条命令，各自退出码 | ①`grep -c mcp__astrbox__ <session 头/日志>`≥17；②`GET /state` 200 且 `values` 含 8 path；③`grep -c ' POST /tick '`≥1；④`grep -c ' POST /session-event '`≥1 | 高 |
| C-R25 | 17 个工具名冻结为文本清单（本日志 A6-5 已给出）；V2/V7 用精确集合比较 | e2e 断言 `tools/list` 名称集合 == 冻结清单（17 项，排序后逐项 diff） | 中 |

## 已撤回（反挑战通道）

| 攻击 | 质疑方 | 撤回原因（无法给出可复现反例） |
|---|---|---|
| K1 的 Unix socket + Node 客户端不可行 | 本喵自查 | 实测可行：`node -e 'http.request({socketPath:"/x",path:"/health"}).on("error",e=>console.log(e.code)).end()'` → `connect-error: ENOENT`（仅在 connect 阶段失败，说明 `socketPath` 合法；Node v26.12.1）。注意 `fetch()` 不支持 unix socket，插件必须用 `node:http`，这一点应写进 K2 的实现约束 |
| `@deepseek-ai/dsh-mcp-client` 在 web profile 里不可解析 | 本喵自查 | 该包不在 profile 的 `node_modules`（`ls @deepseek-ai` 只有 cosmokit/schemastery），但 `~/.dsh/profiles/web/cordis.patch.yml:57,543` 已有两处 `insert: name: '@deepseek-ai/dsh-mcp-client'` 且本会话确有 `mcp__dshtools__*` 工具 → 解析走安装锚点（`apps/cli/src/profile-boot.ts:77-78` `INSTALL_ANCHOR`），先例成立 |
| 实体 `猫猫` 不存在（K1.2 默认值会 404） | 本喵自查 | `data/user/entities.json` 的 `entities` 含 `猫猫`、`梦梦`、`梦梦成年`（public `data/entities.json` 只有 S00x/世界/场景）。`Store.Load` 会合并两份（`store.go:74-79`） |
| K2.1 的 8 条 `paths` 有不存在项 | 本喵自查 | 逐条求值全部 True：`状态.情绪 身体.体力 心情.当前 关系.亲密 位置.场景 睡眠.疲劳 社交.互动需求 任务.当前任务`（数据见 `data/user/entities.json`） |
| Tick 会把仓库工作区写脏（`data/` 被版本控制） | 本喵自查 | `.gitignore` 已忽略 `/data/entities.json`、`/data/user/`、`/data/templates/`、`/data/logs/` 等；`git status --porcelain` 当前只有 `?? runs/dsh-mcp-plugin/`。仅 socket 文件未忽略（已降级为 A2-11） |
| 功能谱系 A 的 m1↔m2 形成 import 环 | 本喵自查 | 未找到实证：`internal/bootstrap` 的上游集合里没有 `internal/control`；环只在「bootstrap 创建 listener」这一未定的归属选择下成立 → 已降级为 A3-1/H1 |
| 17 个工具数不对 | 本喵自查 | `grep -rc mcp.AddTool internal/mcpserver/*.go` 合计 **17**，与 00-scope 一致；`cmd/astrbox-mcp/e2e_test.go:21-38` 确实构建并 spawn 真实二进制，V7 的「既有 e2e」前提成立 |

> 取证说明：为捕获 A1-4 的 stderr，本喵在 `runs/dsh-mcp-plugin/.tmp-dump.err` 建过一个临时文件并立即删除（该目录在写域边缘，未改动任何既有产物）。

## 待观察假设（写不出判定方法的条目落这里，不计入约束）

| # | 假设 | 为何无法判定 | 何时复核 |
|---|---|---|---|
| H1 | control listener 由 m2（`cmd/dsh-mcp`）创建，`internal/bootstrap` 不反向 import `internal/control` | 15-interfaces 未规定归属；只有 G3 冻结时才能定，无法用现有文件判定 | G3 冻结 K1 时 |
| H2 | 未指定 `path` 时 `/state` 的整体截断语义是否够用（当前只剩 `__truncated__`） | 取决于猫实体大小与插件用法；现有 `data/user/entities.json` 47 KB，尚未触及 64 KiB 默认上限，无法构造真实反例 | 实体超过 64 KiB 时 / G4 前补测 |
| H3 | `internal/bootstrap` 不会长成多功能装配桶（<500 有效行） | 取决于 m1 的实现，红队看不到代码 | m1 交付后跑 `lintCodeModularity.sh cmd internal --naming --comments` |
| H4 | 同一 patch 层内两条 `insert` 的挂载顺序（先 mcp-client 后 bridge） | 挂载顺序由外部 `@deepseek-ai/cordis-plugin-include` 决定，该包不在本 checkout 的 `packages/` 内，无法读源码判定 | 真启动一次，观察 bridge 首批 `/state` 是否 ENOENT |
| H5 | `dsh plugin --profile web add link:...` 在半离线环境（无 registry 访问）能成功 | 需要 pnpm 与网络，本会话禁止联网/禁止写 profile，无法实测；`apps/cli/src/plugin.ts:70-107` 只是把参数转发 pnpm | 用户执行真装时，记录 pnpm 退出码与 stderr |
| H6 | 第二份 DSH_HOME 副本能承载 `dsh-astrbox` 的 link 解析（当前副本不能：node_modules 是只读真 profile 的 symlink，bundles 未改） | 需要写 `.local/profile-test`（当前由主智能体持有），红队只读 | m4 交付 `verifyProfile.sh` 后，`grep -c astrbox .local/profile-test/dump.txt` 必须 ≥2 |
| H7 | `ctx.interval` 在回调返回 rejected promise 时不会让宿主进程崩溃 | `interval(callback: () => void, delay)` 只声明同步回调（`tool-cordis/src/api-catalog.ts:3343`），异步 reject 的处置在实现里，需实测 | m3 交付后，注入一次 `fetch` 失败并观察宿主 | 
