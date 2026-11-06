# dsh-plugin-bilibili

B 站**只读**素材插件：给一个链接/BV 号，取回视频信息与字幕正文，供模型总结或检索。

- **零依赖**：只用 Node 22+ 内置 `fetch` 与 `node:crypto`。
- **只读**：不下载视频、不点赞、不登录（登录态由你自己提供的 cookie 决定）。
- **七个工具**：详情、字幕、评论、弹幕、播放地址、扫码登录、发评论（见下表）。

## 安装与配置

```bash
dsh plugin --profile <profile> add /root/dsh-skills-project/plugins/dsh-plugin-bilibili -w
```

| 配置 | 环境变量 | 默认 | 说明 |
|---|---|---|---|
| `cookie` | `DSH_BILIBILI_COOKIE` | 空 | 值形如 `SESSDATA=...`；留空时用凭据文件，再没有就访问首页拿 buvid3 等公开 cookie |
| `statePath` | `DSH_BILIBILI_STATE` | `~/.dsh/bilibili-state.json` | 扫码登录后的凭据落点（0600，不进仓库） |
| `allowPost` | — | `false` | 是否允许发评论（写操作第一道闸） |
| `dryRun` | — | `true` | 发评论干跑（第二道闸）：允许也只返回预览 |
| `timeoutMs` | — | 15000 | 单次请求超时 |
| `maxChars` | — | 20000 | 字幕正文上限（返回值会标记是否截断） |
| — | `DSH_BILIBILI=off` | — | 整体关闭 |

> cookie 属于凭据：**不要**提交进版本库；插件不会把 cookie 写进任何返回值或日志。

## 接口链（实测）

```
view ──► bvid/cid/pages/title/owner/stat
   └──► nav(wbi_img) ──► wbi 签名 ──► player/wbi/v2 ──► subtitle_url ──► 字幕 JSON(body[].content)
                                    └─(签名失败)─► player/v2 明文兜底
```

- 未带 cookie 直连 `x/web-interface/view` 会被风控（HTTP 412 + HTML 错误页）；
  插件先访问一次首页拿公开 cookie，再用同一请求重试一次。
- 字幕接口可能返回 `need_login_subtitle: true`：这表示「有字幕但要登录」，
  与「该稿件没有字幕」是两种不同结果，插件分别给不同提示（不返回空数组冒充成功）。

## 工具

| 工具 | 参数 | 返回 | 需要登录 |
|---|---|---|---|
| `bilibili_video_info` | `input`（链接/BV/av）、`page?` | 标题、UP、时长、发布日、播放/弹幕/点赞、分 P 清单、简介 | 否 |
| `bilibili_subtitles` | `input`、`page?`、`lang?`、`maxChars?` | 选中字幕轨的语言与带 `[mm:ss]` 的正文 | 视稿件 |
| `bilibili_comments` | `input`、`root?`、`page?`、`pageSize?`、`sort?` | 主楼评论（含置顶）或某条评论的楼中楼 | 否 |
| `bilibili_danmaku` | `input`、`page?`、`limit?`、`keyword?` | 带 `[mm:ss]` 的弹幕文本，可按关键词过滤 | 否 |
| `bilibili_play_urls` | `input`、`page?`、`qn?` | 清晰度清单 + dash 视频/音频流地址 | 否（1080P+ 需登录） |
| `bilibili_login` | `action`（generate/poll/status）、`qrcodeKey?` | 二维码链接与轮询结果；**不回显 cookie** | — |
| `bilibili_comment_post` | `input`、`message`、`root?`、`confirm?` | 发表结果（rpid）或"未发送 + 原因" | 是 |

## 扫码登录

1. `bilibili_login action=generate` → 拿到二维码内容链接与 `qrcode_key`（可用任意二维码工具渲染）；
2. 手机 B 站 App 扫码并确认；
3. `bilibili_login action=poll` → 成功后凭据写入 `~/.dsh/bilibili-state.json`（0600），**内存中的 cookie 立即生效**；
4. `bilibili_login action=status` → 看是否已登录、账号 ID（不显示 cookie 值）。

## 发评论的三道闸

1. 必须已登录（cookie 里有 `SESSDATA` 与 `bili_jct`，后者就是 csrf）；
2. 配置必须 `allowPost: true` **且** `dryRun: false`；
3. 本次调用必须显式 `confirm: true`。

任何一道不过都返回「未发送 + 原因 + 将发送的内容预览」，不会假装成功。

## 自测

```bash
node lib/test/testBilibili.mjs   # 20 项：引用解析 / cookie / 412 重试 / WBI 签名 / 降级 / 字幕二分 / 工具形状
```

真机联调（会发真实请求，需网络）：

```bash
node -e "import('./lib/bilibiliApi.mjs').then(async ({createBilibiliApi})=>{const i=await createBilibiliApi({}).videoInfo('BV1GJ411x7h7');console.log(i.title,i.stat.view)})"
```

## 设计出处

参考 `CZX2244/dsh-bilibili` 的接口链与 WBI 签名、`Chenfan2/dsh-bilibili-subtitles` 的降级思路；
**未复制其代码**，也未引入其可选重依赖（sherpa-onnx / whisper.cpp / llama-cpp / ffmpeg）——
ASR 与视觉属于「未配置就必须明确报不可用」的能力，不在本插件范围内。
调研记录：`../../.local/research/upstream-plugin-designs.md` §3。

## 边界

- 只支持公开 web 接口；需要风控签名以外的接口（如私信、投币）不在范围内。
- 字幕语言由接口返回的轨道决定；`lang` 匹配不到时按「中文优先、否则第一条」选择。
- 网络错误、风控、登录态失效都会以明确文本返回，不会静默返回空结果。
