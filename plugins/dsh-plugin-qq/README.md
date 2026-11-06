# dsh-plugin-qq

经 **OneBot 11 正向 HTTP** 与 QQ 后端（NapCat / go-cqhttp / LLOneBot）收发消息的 DSH 插件。

- **双向**：`qq_send` 发消息；插件按配置轮询群/好友历史把入站消息收进队列（`qq_inbox` 取），
  也可以配置 `forwardSessionId` 直接投递进某个 DSH 会话。
- **安全默认**：`enabled=false`、`dryRun=true`。没手动打开前不发任何网络消息、不起定时器。
- **零依赖**：只用 Node 内置 `fetch`；不引入 OneBot SDK。

## 安装与配置

```bash
dsh plugin --profile <profile> add /root/dsh-skills-project/plugins/dsh-plugin-qq -w
```

在 profile 的 bundle 行写 config（见 `cordis.patch.yml` 注释里的完整示例）：

| 配置 | 环境变量 | 默认 | 说明 |
|---|---|---|---|
| `enabled` | — | `false` | 总开关：不开则不发消息、不轮询 |
| `dryRun` | — | `true` | 干跑：允许目标也只返回「本应发送」 |
| `apiBase` | `DSH_QQ_API_BASE` | `http://127.0.0.1:3000` | OneBot HTTP 服务地址 |
| `token` | `DSH_QQ_TOKEN` | 空 | 非空时发 `Authorization: Bearer <token>`；空则不发该头 |
| `watchGroups` | — | `[]` | 轮询哪些群（白名单） |
| `watchUsers` | — | `[]` | 轮询哪些好友（白名单） |
| `allowAnyTarget` | — | `false` | 允许发往白名单外的目标（默认禁止） |
| `pollIntervalMs` | — | 4000 | 轮询间隔 |
| `forwardSessionId` | — | 空 | 配置后入站消息直接投递给该会话（`agent.followup`） |
| `maxTextChars` / `segmentDelayMs` | — | 1500 / 400 | 长文本切段上限与段间间隔 |
| — | `DSH_QQ=off` | — | 整体关闭 |

## 工具

| 工具 | 参数 | 说明 |
|---|---|---|
| `qq_send` | `target`（group/user）、`id`、`text` | 发送；受 enabled/dryRun/白名单三重约束 |
| `qq_inbox` | `limit?`、`peek?` | 取出（或只看）入站队列 |
| `qq_status` | `checkLogin?` | 启用状态、后端地址、监听数、队列与最近错误 |
| `qq_login` | `action`（help/qrcode/refresh/status） | 走 NapCat WebUI 做扫码登录：取二维码内容、刷新、查登录状态 |

## 接入形态

```
QQ 客户端 ──► NapCat (OneBot 11) ──► HTTP API   ◄── 插件 POST send_group_msg / send_private_msg
                                    history API ◄── 插件每 4s 轮询 get_*_msg_history
```

- 收信去重靠 `message_id`；**空消息也记账**，避免每轮重复处理同一个空包。
- 没有反向上报/WS：轮询有延迟，且依赖 history 接口的保留窗口（上游同款取舍）。
- 只有显式配置 `forwardSessionId` 才会把消息投进会话；否则留在本机队列等 `qq_inbox` 取。

## 自测

```bash
node lib/test/testQq.mjs   # 18 项：折文本 / 切段 / 去重 / 鉴权头 / 干跑 / 白名单 / 轮询 / 转发 / 工具形状
```

自测全程使用假后端，**不代表真机可用**；真机联调需要有可用的 OneBot 后端与白名单会话。

## 设计出处与边界

沿用 `Yuuyuko-uu/dsh-qq-bridge` 的形态（正向 HTTP + 轮询 + Bearer 可选 + `message_id` 去重 + 默认干跑）；
对照形态（反向 WS）见 `cheesehaqi/dsh-qq-onebot-bridge`。**未复制其代码**。
调研记录：`../../.local/research/upstream-plugin-designs.md` §4。

不支持：媒体文件收发、表情/卡片富文本、多账号、群管理动作。这些需要协议层能力，超出「文本双向」范围。
