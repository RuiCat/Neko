# dsh-plugin-diary

按天记录「做了什么 / 结论 / 待办」的 DSH 插件：文件落在项目的 `.agent-memory/diary/YYYY-MM-DD.md`，
并提供 `diary_write` / `diary_read` / `diary_list` 三个模型可调用工具。

- **零依赖**：只用 Node 内置模块，不 import `@deepseek-ai/*`。
- **纯本地**：不联网、不调模型、不改 `INDEX.md`。
- **默认可用**：装上就有工具，不需要配置；`DSH_DIARY=off` 可整体关闭。

## 安装

```bash
dsh plugin --profile <profile> add /root/dsh-skills-project/plugins/dsh-plugin-diary -w
# 或在 profile 的 package.json 里加依赖 + 把包名追加到 dsh.profile.bundles（link: 形态）
```

装完用 `dsh --profile <profile> --dump-config | grep -A2 diary` 确认挂载行。

## 工具

| 工具 | 参数 | 说明 |
|---|---|---|
| `diary_write` | `title`、`body`、`tags?`、`date?` | 写一条；同一天同标题同正文幂等跳过 |
| `diary_read` | `date?`、`from?`、`to?`、`query?`、`limit?` | 按日期范围或关键词检索 |
| `diary_list` | `limit?` | 最近写过哪些天、几条、标题 |

## 目录与格式

目录解析顺序：`DSH_DIARY_DIR` → 向上找 `.agent-memory/`（用其 `diary/`）→ `<cwd>/.dsh-diary`。

```markdown
# 2026-10-25（周六）

## 14:32 | 今天把插件装进了 profile
- id: D-20261025-1432-9f3a1b2c
- tags: 插件,安装

正文…
```

## 设计取舍（来自上游实测，见 `../../.local/research/upstream-plugin-designs.md`）

- **写入幂等靠内容指纹**：`sha256(日期+时间+标题+正文)` 前 8 位做 id，重试不会写重复条目
  （对照 `crease123/dsh-journal-calendar` 的「按 id 幂等」）。
- **坏文件硬报错**：文件有内容却解析不出条目时直接抛错，绝不「修成空文件」再回写。
- **原子替换**：整份重写走「临时文件 + rename」，不留半截文件。
- **日期白名单**：日期串必须先过 `YYYY-MM-DD` 严格校验才拼路径（防路径穿越）。
- **粒度按天**，不按会话；跨天的事记在发生那天。
- **有意不做的**：不做日历 UI、不生成周期报告、不把 todo 与宿主 `todo_write` 混在一起
  （避免语义重叠）。

## 自测

```bash
node lib/test/testDiary.mjs     # 16 项：格式往返 / 幂等 / 检索 / 坏文件 / 原子写 / 工具形状 / 挂载
```
