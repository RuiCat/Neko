# DSH 插件：`dsh-astrbox`

把 AstrBox 引擎接进 [DSH](https://github.com/deepseek-ai/deepseek-harness)：

- `bin/dsh-mcp`：同一份引擎的第二个入口。MCP stdio 暴露 17 个 `mcp__astrbox__*` 工具，
  另在 `<data-dir>/dsh-control.sock` 上提供本地控制面（Unix domain socket）。
- `dsh/`：DSH bundle。`dsh.bundle.patch` 指向渲染后的 `dsh/cordis.patch.yml`，
  装入 profile 后自动插入两条条目：`mcp-astrbox`（MCP client）与 `astrbox-bridge`（本插件）。
- `dsh/lib/*.mjs`：静态使用说明段、每轮一条 `[astrbox]` 状态快照、定时 `POST /tick`、
  `session/created` 与 `agent/turn-stopping` 的 `POST /session-event`。

安装由**用户**执行。仓库脚本只负责构建、渲染与只读/隔离验证，不运行 `dsh plugin add`，
也不写 `~/.dsh`。

## 前置条件

- Linux（控制面依赖 Unix domain socket 与 `flock`；Windows 不支持，macOS 未验证）。
- Go（构建两个二进制）、Node.js `^22.19.0 || >=24`、pnpm（`dsh plugin` 会转发给 pnpm）。
- 可用的 DSH CLI（本机：`/home/ruicat/Desktop/deepseek-harness/apps/cli/lib/bin.js`）。

## 安装

```bash
# 1) 构建两个二进制：bin/astrbox-mcp 与 bin/dsh-mcp
make build

# 2) 用模板渲染本机绝对路径，生成 dsh/cordis.patch.yml（模板本身不改）
make render-dsh
```

`make render-dsh` 把 `${REPO_ROOT}`、`${DATA_DIR}`、`${SCRIPTS_DIR}`、`${CONFIG}`
替换为 `scripts/renderDshPatch.sh` 看到的路径（默认 `$PWD` 及其下的 `data/`、`scripts/`、
`config.json`；可用同名环境变量覆盖）。生成物 `dsh/cordis.patch.yml` 已进 `.gitignore`，
**必须在安装前渲染**：bundle 加载的是这个文件，未渲染时其中仍是占位符。

```bash
# 3) 用户执行：以 link 方式把本仓库的 dsh/ 装进 web profile
dsh plugin --profile web add link:/home/ruicat/Desktop/Neko/dsh
```

`dsh plugin add` 会把 `dsh-astrbox` 写进 profile 的 `package.json` 的
`dsh.profile.bundles`，并在下次启动时应用它的 `dsh.bundle.patch`。装完重启 DSH。

> 仓库路径变化（移动目录、换机器）后要重新 `make build && make render-dsh` 并重启，
> 因为渲染结果是绝对路径。

## 配置键

配置写在 bundle patch 的 `astrbox-bridge` 条目下（即 `dsh/cordis.patch.yml.template`）。
未知键、类型错误、负数、路径无分隔符会在 `apply` 时抛错并指名键。

| 键 | 默认值 | 含义 |
|---|---|---|
| `socketPath` | `<repo>/data/dsh-control.sock` | 控制面 socket；必须与 `dsh-mcp -data-dir` 推导出的路径一致 |
| `entity` | `猫猫` | 快照读取的实体名 |
| `paths` | 8 条（见下） | 快照白名单点分路径；默认不含 R18 字段 |
| `promptMaxChars` | `1200` | 单条快照消息上限（字符，超限截断并加 `…（截断）`） |
| `promptRefreshMs` | `5000` | 同一窗口内不重复拉 `/state` |
| `tickEveryMs` | `60000` | 定时 tick 间隔；`0` 关闭定时器 |
| `tickScope` | `""` | `POST /tick` 的 scope，空串与 MCP 工具一致按全量处理 |
| `workspacePrefix` | `<repo>` | `cwd` 目录边界匹配前缀；不匹配的会话不发 `/session-event` |
| `functionName` | `dsh_on_event` | `/session-event` 调用的引擎函数名；未注册时只写日志 |
| `requestTimeoutMs` | `2000` | `/state`、`/session-event` 请求超时 |
| `tickTimeoutMs` | `15000` | `/tick` 请求超时 |
| `warningAfterMs` | `30000` | 连续失败后警告节流窗口 |

默认 `paths`：`状态.情绪`、`身体.体力`、`心情.当前`、`关系.亲密`、`位置.场景`、
`睡眠.疲劳`、`社交.互动需求`、`任务.当前任务`。

`mcp-astrbox` 条目里的 `command` 必须指向 `make build` 产出的 `bin/dsh-mcp`，
`args` 的 `-data-dir` / `-scripts-dir` / `-config` 与渲染出的模板一致。

## 验证

三段验证是**独立**的：组合通过 ≠ 加载通过 ≠ 真装通过。逐段留证据，不要跳段。

### 1) 组合验证（不碰真 profile、不耗模型调用）

```bash
bash scripts/verifyProfile.sh --composition
```

它会在 `.local/profile-test` 重建隔离 `DSH_HOME`：只读复制源 profile 的
`package.json` / `cordis.patch.yml` / `cordis.yml` / `pnpm-workspace.yaml`，
`node_modules` 用只读 symlink，然后执行

```bash
DSH_HOME=$PWD/.local/profile-test node <dsh>/apps/cli/lib/bin.js \
  --profile web --patch $PWD/dsh/cordis.patch.yml --dump-config
```

并断言：退出 0、`id: mcp-astrbox` 与 `id: astrbox-bridge` 各 1 行、`command` 指向的
文件可执行。期望输出形如：

```text
OK: composition checks passed (mcp-astrbox=1, astrbox-bridge=1, command=/home/ruicat/Desktop/Neko/bin/dsh-mcp, dump=.../dump.txt)
```

> `--dump-config` 会写 profile 的 `cordis.yml`，所以**必须**带隔离 `DSH_HOME`；
> 不带会因 `~/.dsh` 只读而 `EROFS` 退出 1。

### 2) 二进制健康（`make doctor`）

```bash
make doctor
```

直接 spawn `bin/dsh-mcp`（临时 `data-dir`/`scripts-dir`，不读写 `data/`），完成
`initialize`（`clientInfo.name=doctor`，`protocolVersion=2025-06-18`）与 `tools/list`，
断言 17 个冻结工具名（大小写/顺序无关）、socket 权限 `0600`、`SIGTERM` 后 socket 被清理。
期望输出：

```text
OK: doctor 通过（17 tools, socket 0600, SIGTERM 清理；data=/tmp/dsh-mcp-doctor-.../data）
```

### 3) 真装证据（用户回传）

真装后的会话/日志整理成一个文本文件，逐项包含：

| 字段 | 采集方式 |
|---|---|
| 两个 id 行 | `--dump-config` 输出里的 `id: mcp-astrbox`、`id: astrbox-bridge` |
| socket 0600 | `stat -c '%a %n' data/dsh-control.sock` → `600 .../dsh-control.sock` |
| 两类控制日志 | `data/logs/dsh-control-*.log` 中的 `POST /tick` 与 `POST /session-event` 行 |
| 17 工具名 | 会话里 `tools/list` 的 JSON（或用 `make doctor` 的输出） |
| 会话快照 | 真装会话中出现过的 `[astrbox]` 快照行 |

```bash
bash scripts/verifyProfile.sh --evidence <file>
```

齐全输出 `OK: 23 checks`；缺字段逐条打印 `MISSING: <field>` 并以非 0 退出，例如：

```text
MISSING: socket 0600 (stat line with dsh-control.sock)
MISSING: dsh-control log POST /tick
...
```

## 回滚

```bash
dsh plugin --profile web remove dsh-astrbox     # 从 profile 依赖与 bundles 中移除
make clean                                      # 只删 bin/，不动 data/
```

- 若曾手工往 profile 的 `cordis.patch.yml` 加过 `mcp-astrbox` / `astrbox-bridge` 条目，
  一并删除后再重启。
- `data/`（引擎状态、日志、socket）是本地运行数据，回滚插件时不需要删除；
  确认不再使用时可自行归档。

## 信任边界

- **无 token、无鉴权**：控制面只靠 Unix socket 文件权限 `0600` 与同 uid 隔离。
  同一 uid 的任意本地进程都能 `connect` 并调用 `/health`、`/state`、`/tick`、
  `/session-event`；共享账号即共享权限。
- **不监听 TCP**：没有网络端口，不暴露到局域网；不要用 socat/SSH 转发把它开放出去。
- MCP 侧由 DSH 以子进程方式启动，权限与 DSH 进程相同；`stdout` 只输出 MCP 帧，
  日志走 `stderr` 与 `data/logs/`。
- 控制日志记 `method path status`，`/session-event` 额外记 `kind/session/turn/function`；
  不记录模型对话正文。
- 快照只读 `paths` 白名单；**R18 字段默认不注入**。

## 平台与生命周期

- **Linux-only**：控制面是 Unix domain socket + `flock`；Windows 不支持。
- **单实例**：每个 `data/` 目录同时只允许一个 `dsh-mcp`。启动时对
  `data/.engine.lock` 取非阻塞 `flock`，失败则退出码 `3` 并打印持有者 pid 与
  `rm` 提示；锁随进程退出由内核释放，**锁文件本身保留**是正常的。
- 启动时若 socket 已存在：`connect` 成功 → 退出码 `3`；`ECONNREFUSED`（上次异常退出）
  → `unlink` 后重试一次。
- 退出码：`0` 正常/信号清理，`2` 配置或数据加载失败，`3` 单实例/端口占用，
  `4` socket 无法建立。`SIGTERM`/`SIGINT` 会关闭控制面并删除 socket。

## 排错

| 现象 | 处理 |
|---|---|
| 启动报 socket 已被占用 / 退出码 3 | 另一个 `dsh-mcp` 还在用同一 `data/`：先停 DSH（或对应 MCP 子进程），再 `rm data/dsh-control.sock` 重试 |
| stale socket（上次 `kill -9` 后残留） | 正常路径会自动 `ECONNREFUSED → unlink` 自愈；仍失败时手工 `rm data/dsh-control.sock` |
| 锁被占用但找不到进程 | 只有持有 `flock` 的进程算占用；`kill -9` 后内核已释放，直接重启即可。确认无进程再用 `rm data/.engine.lock` |
| 工具消失 / `tools/list` 不满 17 个 | `make doctor` 先确认二进制；再看 profile `package.json` 的 `dsh.profile.bundles` 是否含 `dsh-astrbox`、`dsh/cordis.patch.yml` 是否已渲染为绝对路径；最后看 DSH 启动日志里的 `mcp-astrbox` 错误 |
| 快照不出现 | 检查 `socketPath` 与 `dsh-mcp` 的 `-data-dir` 是否一致、`workspacePrefix` 是否覆盖当前会话 `cwd`、`paths` 是否在白名单内 |
| 重复 tick / `tick_busy` | `/tick` 用 `TryRunTick`，MCP `state_machine_tick` 或另一个 tick 在飞时返回 `409`，插件按设计跳过本次；持续出现时把 `tickEveryMs` 调大或设 `0` 关闭定时 tick |
| `/session-event` 只有 `logged` | `functionName` 指向的函数未注册；用 `entity`/脚本工具确认函数存在，或改配置指向已注册函数 |

## 相关文件

- `Makefile`：`build` / `test` / `lint` / `render-dsh` / `doctor` / `clean`
- `scripts/renderDshPatch.sh`：模板 → `dsh/cordis.patch.yml`
- `scripts/doctorDsh.mjs`：`make doctor` 的实现
- `scripts/verifyProfile.sh`：`--composition` / `--evidence`
- `dsh/cordis.patch.yml.template`：占位符模板（版本库内，不含机器绝对路径）
- `dsh/lib/`：插件运行时；`docs/mcp-tools.md`：17 个 MCP 工具的参考
