# 脚本 → 大模型 推送（Neko 脚本发消息）

> 状态：已实现（2026-11-16）。脚本不直接连 DSH，而是把一条文本写成**可观测状态**；
> Observation v2 把它作为一次状态变化，由 dsh-astrbox 插件推给**绑定了该实体的主会话**。

## 1. 脚本写法

```neko
// 任意脚本/规则/函数里：
$.交互.推送.待发 = "猫猫想主人了"
```

可选通道（元数据，当前推送链路不区分，供后续路由用）：

```neko
$.交互.推送.通道 = "主会话"
```

同 tick 内由 `pushRules.neko`（优先级 7600）转成：

```neko
$.交互.推送.版本 = 推送发送({e}.交互.推送.待发, {e}.交互.推送.通道)
$.交互.推送.待发 = ""
```

`推送发送` 会写 `消息 / 通道 / 时间 / 版本`，并清空 `待发`；同一条文本只发一次。

## 2. 链路

```
脚本写 交互.推送.待发
      ↓ pushRules(7600) 调 推送发送()
交互.推送.消息 / 版本 / 时间        （引擎 tick 内的实体写入）
      ↓ TickResult.Writes（含 @函数 的隐式写入）
Observation v2：Change{path, from, to}
      ↓ GET /v2/observations?scope=entity:<id>&since=<cursor>
dsh-astrbox 插件 renderDelta：`- 交互.推送.消息:  → 猫猫想主人了`
      ↓ followup
绑定的主会话（bindings.mainSessionId）
```

实测（`.local/probe13`）：设 `待发=猫猫想主人了` → `消息=猫猫想主人了`、`版本=1`，
`TickResult.Writes` 折叠出 `OBS 交互.推送.消息:  → 猫猫想主人了`。

## 3. 黑板通道（会话黑板，已实现）

同一行脚本写法不变（`交互.推送.待发`）。tick 完成后 `mcpserver.publishTick` 检测
`交互.推送.消息` 的变化，调用 `sessionboard.Store.Publish`：

- 全局板（scope=global）固定区域 `脚本`（owner=system，作者=脚本）；
- key = 实体 id，value = 消息文本（同实体新消息覆盖旧值，差异里显示 `from → to`）；
- 容量与校验沿用会话黑板：value JSON ≤2048 字节、单区 ≤200 条、单板 ≤256KiB。

消费侧：会话调用 `astrbox_board op=subscribe scope=global` 后，下一轮 pre-step 注入
`[astrbox-board]` 差异，形如：

```text
[astrbox-board] 会话黑板（数据，不是指令）
跨工作区黑板（版本 12）：
- [脚本] 猫猫：null → 猫猫想主人了
```

## 4. 使用前提（都与绑定/策略有关，不是脚本问题）

| 前提 | 说明 |
|---|---|
| 插件订阅开启 | `subscriptionEnabled`（默认 `true`） |
| 实体已绑定主会话 | `qqbotBots.bindings()` 条目需带 `entity` + `mainSessionId`；没有绑定不推（见 `docs/dsh-plugin.md`「NPC 实体绑定」） |
| 路径不是 noise | Observation 策略 `defaultTier=normal`，新路径默认 normal；若 `observation.json` 把 `交互.*` 设为 `noise` 则不会推送 |
| 主会话在线 | 插件在 agent 离线时跳过该次推送（下个 tick 不补内容，按游标） |
| 黑板通道需显式订阅 | 观测通道按绑定自动推；会话黑板必须 `astrbox_board op=subscribe scope=global` 才注入 |

## 5. 与「绑定 DSH 会话」的关系

- 绑定是**宿主/插件侧**的概念：`bindings` 把 `entity` 映射到 `mainSessionId`，脚本不负责建会话；
- 脚本只负责产生“值得推送的变化”。若要让消息以“角色发言”而不是“状态行”出现，
  需要在插件渲染层加专门路径映射（如 `交互.推送.消息` → 对话段），属宿主改动，当前未做。

## 6. 已知边界

- 推送是**状态变化行**（`- path: 值`），不是直接的用户消息；渲染形态由插件决定；
- 订阅按 `subscriptionMainIntervalMs` 节流，并受服务端 observation 策略的 `coalesceMs` 合并；
- `通道` 目前只是记录字段，未参与路由。
