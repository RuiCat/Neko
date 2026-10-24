# 架构

## 总览

```text
cmd/astrbox-mcp
      │
internal/mcpserver ──► internal/registry ──► internal/compiler ──► internal/neko
      │                      │                     │
      │                      ▼                     ▼
      └────────────► internal/engine ◄────── internal/entity
                             ▲
                  internal/function ◄── internal/events
```

依赖方向单向：`neko` 与 `entity` 不依赖其他内部包；`engine` 只依赖 `entity`；`function` 只依赖 `neko`/`entity`；`events` 依赖 `engine`/`entity`/`neko`；`compiler` 依赖 `neko`/`engine`/`entity`；`registry` 依赖 `compiler`/`engine`/`entity`；`mcpserver` 依赖前几者与 `entity`；`cmd` 只做装配。

## 各包职责

| 包 | 职责 | 不做什么 |
|---|---|---|
| `internal/neko` | `.neko` 顶层块扫描（`ScanBlocks`）、AST（`ScriptAST`）、语法分析（`Parse`），fail-soft 收集错误 | 不编译、不解析条件语义 |
| `internal/entity` | 实体模型与模板继承、点分路径读写、选择器/查询、`entities.json` 持久化 | 不做事件与规则 |
| `internal/engine` | 条件编译/求值、效果文本应用、规则 CRUD、`Tick`、冲突收集与仲裁、值限幅 | 不解析 `.neko`、不落盘 |
| `internal/compiler` | AST → 模板（展开注解）、规则（条件重写、动作降级、分支）、函数/事件透传 | 不注册、不执行 |
| `internal/function` | `@函数` 解释执行：变量、分支、循环、列表/字符串操作、实体路径读写、函数互调 | 不做事件分发 |
| `internal/events` | 从模板注解构建 schema，字段变化触发 `@事件`，按条件调用 `@函数` 处理器 | 不定义规则语义 |
| `internal/config` | 基础配置层：状态组、状态定义字段、默认子组的进程级配置与 JSON 加载 | 不持有实体或规则 |
| `internal/registry` | 脚本 hash、增量应用、引用计数、源文件落盘、`scripts.json`、重启重放 | 不定义规则语义 |
| `internal/mcpserver` | MCP 工具 schema 与处理器，把引擎能力暴露给客户端 | 不实现领域逻辑 |
| `cmd/astrbox-mcp` | 装配：加载数据 → 重放脚本 → 选择 stdio/HTTP 传输 | 无业务逻辑 |

## 启动流程

1. `config.Load(config.json)` + `config.Set(...)`：加载域词汇配置；文件缺失时用默认值（状态/状态定义/身体）。
2. `entity.NewStore(dataDir).Load()`：读取 `data/templates/*.json`（文件名 `_` 替换为 `:`）与 `data/entities.json`。
3. `engine.New(store)`：空规则集；若存在 `scripts/rules.json`，解析并注册。
4. `function.New(store)` + `events.NewSystem(store, funcs)`：创建函数执行器与事件系统；`machine.SetFuncCaller(funcs)` 让条件/动作能调用函数；`store.OnCreate` 挂到事件桥，新建实体自动绑定变更钩子。
5. `registry.New(...)`：读取 `data/scripts.json` 条目，脚本源目录为 `scripts/`；`reg.SetFunctionSink(eventSystem)` 让 apply/remove 同步注册函数与事件。
6. `registry.ReplayAll()`：对每个条目从 `scripts/<name>.neko` 重新解析、编译、注册；源文件缺失记 `skipped`，失败记 `failed`。
7. 若注册表为空，`registry.BootstrapFromSources()` 扫描 `scripts/*.neko` 并注册（本地 `data/` 不进版本库，首次运行靠它引导）。
8. `mcpserver.RunStdio` 或 HTTP handler 开始服务。

重放是必需的：规则只存在于内存，`scripts.json` 只记录每个脚本引用的模板与规则 id。

## 配置层

`internal/config` 保存三项域词汇，默认值与旧行为一致：

| 配置 | 默认 | 使用方 |
|---|---|---|
| `state_group` | `状态` | engine 写状态/冲突；compiler 重写 `状态:`；parser 语法糖；MCP 查询输出 |
| `state_definition_field` | `状态定义` | engine 的 `branch_race` 冲突检测 |
| `default_state_subgroup` | `身体` | compiler 的裸字段默认分组；parser 的分支状态糖 |

进程级生效：`config.Current()` 在读取时取当前值，`config.Set` 校验后原子替换。加载入口在 `cmd/astrbox-mcp`：`-config` 或默认当前目录 `config.json`，文件缺失不报错。细节见 [config.md](config.md)。

## 数据格式

`data/` 是本地运行状态，默认不进版本库；首次启动无注册表时从 `scripts/` 递归引导。环境实体写 `data/entities.json`，NPC 角色实体单独写 `data/user/entities.json`（仅本地，不发布）。

### `data/entities.json`

```json
{
  "version": 1,
  "entities": {
    "猫猫": {
      "id": "猫猫",
      "type": "NPC:猫猫",
      "keys": {},
      "data": { "身体": { "体力": 90 } },
      "_meta": {}
    }
  }
}
```

`type` 用 `:` 表示继承链（`NPC:猫娘:猫猫` → 依次合并 `NPC`、`NPC:猫娘`、`NPC:猫娘:猫猫` 的模板）。

### `data/templates/<name>.json`

文件名 `NPC_猫猫.json` 对应类型 `NPC:猫猫`。模板可含注解键（`"体力[int,@体力管理,min:0,max:100]"`），编译脚本的 `@Data` 块时展开为普通字段与 `__annotations__` 表。

### `data/scripts.json`

```json
{
  "version": 1,
  "scripts": {
    "cat_core": {
      "hash": "<sha1 of source>",
      "templates": { "NPC": 1 },
      "rules": { "r_ab12cd34": 1 },
      "funcs": [], "events": [],
      "func_defs": [], "event_defs": []
    }
  }
}
```

`templates`/`rules` 的值是引用计数。应用脚本时先释放旧条目的引用（归零才真正删除），再注册新条目。同 hash 且未 `force` 时是 no-op。

## 一次工具调用的数据流

```text
MCP 请求
  └─► mcpserver 参数校验
        ├─ 读：store.Get / entity.Get
        ├─ 写：entity.Set → OnChange → events.Bridge
        │        ├─ schema 校验（模板 __annotations__）
        │        ├─ 命中 @事件 → events.Bus.Fire（条件用 变化量/旧值/新值）
        │        └─ 调用 @函数 → 可能继续写实体（事件总线深度上限 10）
        ├─ 规则：engine.Tick(scope) → 内存状态变化 → 调用方 store.Save
        ├─ 仲裁：ResolveConflictWithEffects → 写状态 → llm_decisions.json
        │        └─ 有 side_effects ⇒ 回填 source=llm_backfill 的高优先级规则
        ├─ 脚本：registry.Apply → neko.Parse → compiler.CompileScript
        │        → store.AddTemplate / engine.AddRule / 函数+事件注册
        │        → data/scripts.json + scripts/ 源文件落盘 + schema 重建
        └─ 日志：tick 结果追加到 data/logs/state_YYYY-MM-DD.log
```

`engine.Tick` **不落盘**（与 Python 不同）：服务端在 Tick 成功后显式 `store.Save()`；库使用者若直接调用 `Tick`，需要自己保存。启动重放同样不写 `data/scripts.json`，只有 apply/remove 才持久化。

## 并发与一致性

- `entity.Store` 用 `sync.RWMutex` 保护模板与实体表；`Entity` 自带读写锁保护 `Data`。
- `registry.Registry` 串行化应用/移除（单把互斥锁）。
- `StateMachine` 的规则表在注册阶段加锁，Tick 读取；当前实现假设 Tick 不被并发调用。
- 事件触发是同步递归：处理器写实体可能再次触发事件，`events.Bus` 用深度计数（上限 10）阻断循环。
- `data/entities.json`、`data/scripts.json` 均以「写临时文件 + rename」方式原子替换。

## 代码约束

- 每个源码文件只承载一个功能；单功能文件 < 500 有效代码行，多功能文件 < 1000。
- 文件名 / 函数 / 方法 / 参数 / 变量用 `camelCase`，类型用 `PascalCase`，常量 `UPPER_SNAKE`。
- 每个文件头一句话职责；导出符号有文档注释。
- 构建 / 测试 / 检查统一走 `make`（见 [development.md](development.md)）。
