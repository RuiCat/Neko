# 表情工具迁移准备包（Python `cat_find_emote` → Go MCP）

> 状态：**已实施（2026-11-10）**：`internal/emote` + `emote_find` + `astrbox-emote` CLI 已落地，A1–A9 实测结果见 §10。
> 范围经用户 2026-11-10 确认：**查找链路 + 向量库构建维护链路**。
> 冻结基线：`legacy/python-astrbox/` 与 `/home/ruicat/data/catworkspace/` 的现有行为与数据格式；本文所有断言带
> `文件:行` 或实测命令，未实测的标 **假设**。
> 实施轮按 `task-orchestration` 的 L3 走（跨模块接口 + 数据契约冻结 + 多执行体）；本文件只负责把范围、对照、契约、验收先定死。

## 0. 一句话结论

Python 侧的表情工具是**一个薄工具层 + 三个检索脚本 + 四个构建脚本**，核心价值在 `emote_vec_db.search()` 的两层意图融合检索与 R18 过滤；
迁移的难点不在算法（查询核心 525 行 + 向量构建 53 行 Python），而在**数据契约（numpy npz / 105MB JSON）**、**Ollama 768 维嵌入的运行时依赖**、
以及**索引构建脚本已缺失**这三件事。建议：检索与「向量+聚类」构建进 Go；视觉簇分析与场景层不迁移；`emote_index.json` 视为外部输入。

## 1. 范围（用户确认）

| 在范围内 | 说明 |
|---|---|
| 查找链路 | `cat_find_emote` 工具 → 三级降级 → `emote_vec_db.search()` / `emote_matcher_v2.find_emotes_vec()` / `emote_matcher.find_emotes()` → 返回表情文件路径 |
| 构建维护链路 | `build_emote_vectors.py`（描述→向量）+ `emote_vec_db.py build`（KMeans 聚类→npz） |

| 明确不在范围内 | 承担者 |
|---|---|
| 发送表情到 QQ | DSH `qqbot_send_file`（本会话已可用；`plugins/dsh-plugin-qq/README.md:66` 明确该插件不支持媒体，媒体能力在 `plugins/dsh-qqbot/src/media/`） |
| 图片采集、dHash 去重、视觉打标（`*.jpg.txt` 提示词） | 已完成的外部产物，见 `/home/ruicat/data/catworkspace/notes/表情包提示词库.md:4-10` |
| AstrBot `FunctionTool` 适配层 | Python 插件已冻结，Go 侧换 MCP 工具注册 |
| 簇级视觉分析与场景层（见 §2.8 G3） | 低频离线步骤，保留 shell 脚本即可 |

## 2. Python 侧现状（事实基线）

### 2.1 工具定义

- 入口 `build_emote_tool()`：`legacy/python-astrbox/tools/tools.py:1032-1075`；注册名 `cat_find_emote`（`tools.py:1066`）。
- 参数 schema 只有 `text`（必填）与 `top`（int，默认 3）（`tools.py:1071-1074`）；handler 另有一个
  **未进 schema 的 `use_vec`**（`tools.py:1036`）——LLM 传不进来，属遗留参数（缺口 G5）。
- 降级顺序（`tools.py:1043-1055`）：
  1. `emotes/vec_db.npz` 存在且 >1000 字节 → `emote_vec_db.search(text, top)`，再用 `emote_matcher_v2._emote_path` 补 `path`；
  2. 否则 `emotes/emote_vectors.json` 存在 → `emote_matcher_v2.find_emotes_vec(text, top)`；
  3. 否则 → `emote_matcher.find_emotes(text, top)`（纯关键词）。
- 输出是**文本行**：`找到 N 个匹配表情:` + 每行 `  [{score}] {file} | {desc[:50]} | path={path}`（`tools.py:1056-1061`）；
  空结果 `🐱 没找到匹配的表情，换个说法试试喵~`；异常 `❌ 表情匹配失败: {e}`（`tools.py:1062-1063`）。

### 2.2 查询算法（主路径 `emote_vec_db.search`）

`/home/ruicat/data/catworkspace/emote_vec_db.py:82-249`：

1. **查询向量**：`POST http://127.0.0.1:11434/api/embeddings`，body `{"model":"nomic-embed-text","prompt":text}`，
   60s 超时，取 `embedding`（`emote_vec_db.py:32-39`）→ **768 维**。
2. **意图词表**：16 类（开心/难过/生气/惊讶/害羞/撒娇/抱抱/亲亲/摸头/爱/困/疑问/嫌弃/加油/害怕/吃货）（`emote_vec_db.py:105-124`）；
   `NEG_KW` 负向消歧（如「抱头/抱胸/抱枕」不算「抱抱」）（`emote_vec_db.py:125-133`）；查询侧命中负向词则跳过该意图（`:135-143`）。
3. **第一层**：内容簇。`centers` 归一化后与查询向量余弦，取 Top20 簇（`emote_vec_db.py:149-152`）。
4. **第二层候选池**：上述 20 簇的全部成员 ∪ **意图直查命中**（desc/categories 含意图词，每意图最多新增 `top*20` 条，
   并标记 `intent_hit` 以豁免低相似度过滤）（`:154-181`）。
5. **过滤与打分**：R18 词表命中 desc 直接丢弃（`:195-200`）；`sim < 0.35` 且非意图直查 → 丢弃（`:202`）；
   `intent_bonus`（desc+簇分析命中意图，每意图 +0.5）、`strong`（desc 直接命中 +0.3），
   最终 `final_score = sim*0.5 + min(intent_bonus+strong, 2.5)*0.5`（`:204-223`）；按分降序取 top（`:225-239`）。
6. 返回字段：`file / desc[:100] / score / categories / intent_bonus / cluster`（`:229-239`）。
7. **每次查询都 `np.load(vec_db.npz)` 重新加载**（`emote_vec_db.py:89-92`），无缓存 → Go 侧可缓存（mtime 失效）。

### 2.3 回退实现

- `emote_matcher_v2.find_emotes_vec()`：同样调 Ollama，遍历 **105MB** `emote_vectors.json` 做余弦，阈值 0.35，暧昧过滤（`emote_matcher_v2.py:28-37,56-97`；空文本短路 `:58-59`；暧昧过滤 `:81-82`）。
- `emote_matcher.find_emotes()`：纯关键词打分——意图类别命中 ×10、关键词交集 ×3、desc 包含 ×2、
  「开心+爱」−1、「其他」−2、非亲密意图跳过 `暧昧` 表情（`emote_matcher.py:37-120`；暧昧过滤 `:102-103`；空文本短路 `:58-59`）。
- 路径解析 `_emote_path()`：`img/<file>` 优先，`.jpg/.gif/.png/.jpeg/.webp` 试扩展名，缺失回退 `/home/ruicat/Desktop/QQ表情`（`emote_matcher_v2.py:100-110`）。

### 2.4 构建链路（4 步，全部离线批处理）

| 步 | 脚本 | 作用 | 关键参数 | 产物 |
|---|---|---|---|---|
| 1 | `/home/ruicat/data/catworkspace/build_emote_vectors.py:1-53` | `desc` → 向量 | 串行；每 100 条原子落盘（`:44`）；收尾原子写（`:49-51`）；**断点续传**（已有向量跳过；失败写 `null`） | `emote_vectors.json`（105MB） |
| 2 | `emote_vec_db.py:41-80` | KMeans 聚类 | `N_CLUSTERS=128`（`:29`）、seed 42、随机初始化、≤20 轮 | `vec_db.npz` + `vec_db.json` |
| 3 | `build_cluster_analysis.py:1-111` | 每簇代表图 → qwen2.5vl 分析 | 代表 = 簇内 `members[0]`；`/api/generate` + base64 图；`num_predict:120`；断点续传 | `cluster_analysis.json` |
| 4 | `finalize_emote_vec.sh:1-54` | 128 簇分析向量 → 16 场景簇 | seed 7 | `scene_db.npz` + `scene_db.json` |

编排外壳：`auto_build_vecdb.sh` / `finalize_emote_vec.sh` 用 `pgrep` 轮询等待上一步进程结束——**无状态、无法重入**（Go 化时应换成显式子命令）。

### 2.5 数据清单（2026-11-10 实测，路径 `/home/ruicat/data/catworkspace/emotes/`）

| 文件 | 大小 | 条目 | 结构 | 生产者 | 查询期消费者 |
|---|---|---|---|---|---|
| `emote_index.json` | 3,358,248 B | 6846 | `{file: {desc, categories[], keywords[]}}`；categories 共 **25** 类 | **缺失（G1）** | 三级全部依赖 |
| `emote_vectors.json` | 109,207,915 B | 6846 | `{file: [768 float] \| null}` | 步 1 | 二级回退 |
| `vec_db.npz` | 19,860,130 B | — | `X(6846,768) float32`、`centers(128,768) float32`、`labels(6846,) int64` | 步 2 | 一级（主路径） |
| `vec_db.json` | 164,362 B | 6846 names | `{names[], n_clusters:128, dim:768, count:6846}` | 步 2 | 一级必需 |
| `cluster_analysis.json` | 2,066,925 B | 128 簇 | `{cluster: {name, analysis, vec[768], desc}}` | 步 3 | **可选**（存在则参与意图加分） |
| `emote_scenes.json` | 426,612 B | 6846 | `{file: [场景标签]}` | 来源未记录 | **不读** |
| `emote_categories.json` | 1,659,941 B | 6843 | `{categories:{18 类计数}, index:{file:{desc,tags}}}` | 采集期 | **不读** |
| `scene_db.npz` / `scene_db.json` | 407,841 / 14,151 B | 128→16 | 场景聚类 | 步 4 | **无消费者（G3）** |
| `img/` | ≈888 MB | 6843 文件 | jpg/gif/png… | 采集 + dHash 去重 | 返回路径解析必需 |
| `*.jpg.txt` 等 | — | 6846 | 视觉模型生成的 `desc` 原文 | `notes/表情包提示词库.md:4-10` | 不读（打标链） |

### 2.6 运行依赖（实测）

- Ollama 在跑（`curl -s http://127.0.0.1:11434/api/tags` 返回 200）：
  `nomic-embed-text:latest`（768 维，embedding）、`bge-m3:latest`（**1024 维**，不可替换现有索引）、`qwen2.5vl:3b`（视觉，2048）、`qwen3-reranker-0.6b`。
- `catworkspace/models/qwen3-reranker.gguf`（610MB）**不被表情链路使用**（`rerank-server.sh` 独立）。
- Python 侧除标准库外只依赖 `numpy`（npz + KMeans）。

### 2.7 行为基线（2026-11-10 本机实测）

- `python3 /home/ruicat/data/catworkspace/verify_emote_negkw.py` → **8/8 通过，PASS**（正/负向意图、R18 过滤、困/难过）。
  这是**唯一现成的行为回归集**，迁移时须 1:1 转成 Go 测试。
- 三查询 top-3（`time python3 -c ...`，三连查共 1.24s、含首轮 npz 加载；单查询热态实测 0.25s）：

  | 查询 | top-3（file, score） |
  |---|---|
  | 主人我好想你呀 | `8773171715714f8e.jpg` 0.7888 / `0b4fcd65c87436b2.jpg` 0.7876 / `355970d48c5b5303.jpg` 0.7841 |
  | 今天好开心 | `423393662c2cc3e3.jpg` 0.808 / `17170d0f5b232e19.jpg` 0.8001 / `0d0c1d174fcb4b0b.jpg` 0.7881 |
  | 有点困了想睡觉 | `6595981a190faba6.jpg` 0.8029 / `3351494b4527068e.jpg` 0.7984 / `0911215999972759.jpg` 0.7915 |

  **口径（2026-11-10 实测快照）**：`vec_db.npz` sha256 `b6d1bfa8a786…d68fe3`、`cluster_analysis.json` `712e5f61a8b9…7d6032`、`emote_index.json` `90b1061ad7c3…0c2bf3`（复算见 §7）。重建索引后该基线失效，须重新采样。

### 2.8 已知缺口（迁移前必须承认的现状）

| 编号 | 缺口 | 证据 | 迁移处置 |
|---|---|---|---|
| G1 | **`emote_index.json` 无构建脚本**：`keywords` 是 `desc` 的机械 4 字切片（如 `绿色头发`/`的卡通女`/`孩闭眼微`），且 categories 25 类 ⊃ `emote_categories.json` 的 18 类（多出 `爱/吃货/困/疑问/暧昧/嫌弃/其他`）→ 无法从现存数据完整重建 | 实测样本 `emote_index.json["00001898b8b692d8.jpg"]`；`emote_categories.json` 键集 | **视为外部输入**（只读）；不复刻 builder（决策 d5） |
| G2 | 3 个索引项无图：`07c5cbcbcfe7e7f3.bmp`、`677761f2721a190b.bmp`、`f0d8d0d0f9e860f0.webp` | `os.path.exists(img/<file>)` 实测 6843/6846 | 路径解析返回 `path:""` 并在结果里标 `missing:true`（Python 的 `_emote_path` 返回 `None`，格式化后打印 `path=None`，见 `tools.py:1060`） |
| G3 | 场景层（步 4）**无查询期消费者** | `grep -rn "scene_db" catworkspace` 只命中 `finalize_emote_vec.sh` | 不迁移（保留 shell） |
| G4 | 口径不一致：6846 索引 / 6843 图 / 6846 提示词；文档写「6846 提示词对应 6567 张去重图」 | §2.5 计数；`notes/表情包提示词库.md:4` | 验收一律带「快照口径」，不写裸计数 |
| G5 | `use_vec` 未进 schema（`tools.py:1036` vs `1071-1074`） | 同左 | Go 侧**不保留**该参数（要么有向量要么降级，由服务端决定） |
| G6 | 返回是**文本行**，与 Go 侧 MCP「成功返回 JSON、失败 `❌` 前缀」约定不同 | `tools.py:1056-1063`；`docs/mcp-tools.md:29-31` | 决策 d4 |

## 3. Python → Go 对照（落点建议）

| 能力单元 | Python 位置 | Go 落点（建议） | 备注 |
|---|---|---|---|
| MCP 工具注册 | `tools.py:1032-1075` | `internal/mcpserver/emoteTools.go` + `internal/mcpserver/server.go:140-158` 挂一行 | 注册模式见 `internal/mcpserver/stateTools.go:24-47`；工具总数将 29→30 |
| 参数/返回契约 | `tools.py:1036,1071-1074` | 参数 `text`(必填)、`top`(默认 3，上限夹紧)；返回 JSON（决策 d4） | 项目返回约定 `docs/mcp-tools.md:29-31` |
| 表情目录配置 | `RuntimeConfig.catworkspace_dir`（`legacy/python-astrbox/runtime_config.py:11`） | `-emote-dir` + `ASTRBOX_EMOTE_DIR`，默认 `~/data/catworkspace/emotes`；`cmd/astrbox-mcp/main.go:16-23,57-69` 现有 flag/env 模式 | 数据**不进仓库**（888MB 图 + 105MB JSON） |
| Ollama 嵌入客户端 | `emote_vec_db.py:32-39` | `internal/emote/embedder.go`（`net/http`，60s 超时，可复用 `internal/llm/httpProvider.go:17-38` 的超时/重试风格） | 超时/非 2xx/维度≠meta dim 都要显式错误 |
| npz 读取 | `np.load` | `internal/emote/npz.go`（zip + `.npy` 头解析，f32 LE，C order，只读） | **唯一非标准库工作量**（决策 d2） |
| 检索 | `emote_vec_db.py:82-249` | `internal/emote/search.go`（意图表/负向词/R18/打分逐项复刻）+ `internal/emote/data.go`（索引加载与缓存） | 词表硬编码照抄，不得「顺便优化」 |
| 关键词回退 | `emote_matcher.py:37-120` | `internal/emote/keyword.go` | 决策 d3 |
| 向量回退 | `emote_matcher_v2.py:56-97` | 并入 `search.go` 的二级分支 | 105MB JSON 仅回退时加载 |
| 构建：向量 | `build_emote_vectors.py` | `cmd/` 新子命令 + `internal/emote/build.go` | 并发 + 断点续传 + 原子落盘；记录 `model/dim/built_at/index_sha256` |
| 构建：聚类 | `emote_vec_db.py:41-80` | 同上 | KMeans 复刻 k=128/seed/20 轮（风险 R2） |
| 构建：簇分析 | `build_cluster_analysis.py` | **不迁移**（保留 Python/shell） | 视觉模型低频 |
| 构建：场景层 | `finalize_emote_vec.sh` | **不迁移**（G3） | — |
| 测试基线 | `verify_emote_negkw.py` | `internal/emote/*_test.go` + 小夹具（判定词与期望，不含图片） | 见 §5 A3 |
| 文档 | `docs/parity.md:52` 现状「未迁移」 | 实施时更新该行 + `docs/mcp-tools.md` 工具条目 | — |

**模块化约束**（`code-modularity`，动手即生效）：单功能文件 <500 有效行、多功能 <1000；命名 `camelCase`（类型 `PascalCase`）；
一个文件一个独立功能 → 预计 5-7 个文件（`npz.go` / `data.go` / `embedder.go` / `search.go` / `keyword.go` / `build.go` / `emoteTools.go`）；
交付前跑 `lintCodeModularity.sh internal cmd --naming --comments`。

## 4. 数据契约（冻结候选）

### 4.1 目录契约

```
<emote-dir>/
  emote_index.json            只读输入（G1：视为外部产物）
  emote_vectors.json          可重建（步 1 产物）
  vec_db.npz + vec_db.json    可重建（步 2 产物；一级检索）
  cluster_analysis.json       可选（缺省 = 无意图加分，检索仍可用）
  img/                        只读资源（路径解析）
```

- 仓库内**只放代码 + 测试 + 文档**；数据目录用 `-emote-dir` 指向本机外部路径（默认 `~/data/catworkspace/emotes`）。
- 构建产物写入采用 **tmp + rename 原子替换**（沿用 `build_emote_vectors.py:49-51` 的做法）；构建必须支持 `--out <临时目录>` 以便验收不碰线上数据。
- **实现差异（已确认）**：Python 用「`vec_db.npz` 存在且 >1000 字节」探测可用性；Go 改为「存在即尝试解析，解析失败报错」——
  小体积/损坏文件不再静默回退到二级，符合决策 d3 的「不静默降级」。

### 4.2 文件 schema 与兼容性

| 文件 | 必需字段 | 兼容要求 | 校验 |
|---|---|---|---|
| `emote_index.json` | `file → {desc:string, categories:[string], keywords:[string]}` | 保持不变；缺失 `keywords` 时按空集处理（fail-soft） | 键数 ≥1；`desc` 非空 |
| `vec_db.npz` | `X:float32[N,dim]`、`centers:float32[k,dim]`、`labels:int64[N]` | **必须能读现有 npz**；Go 构建产物继续写 npz（Python 仍可 `np.load`） | 维度 = `vec_db.json.dim`；`len(labels)==N` |
| `vec_db.json` | `names:[string]`、`n_clusters:int`、`dim:int`、`count:int` | 现有字段不改；Go 新增可选 `model`、`built_at`、`indexSha256`（旧文件缺省则跳过校验） | `count==len(names)` |
| `emote_vectors.json` | `file → [float] \| null` | 不变 | 向量长度 = dim |
| `cluster_analysis.json` | `cluster → {name, analysis, vec, desc}` | 可选；损坏则忽略并告警 | `vec` 长度 = dim |

### 4.3 运行语义契约

| 场景 | 行为（与 Python 对齐） |
|---|---|
| 空 `text` / 全空白 | Python 侧 **`search()` 不检查空文本**（`emote_vec_db.py:82-92`，会照常请求 embedding），只有 `find_emotes`/`find_emotes_vec` 短路返回空（`:58-59`）。Go 侧在**工具层**统一短路（新增保护，标注为行为差异） |
| `top` ≤ 0 / 超大 | 夹紧到 `[1, 20]`（Go 侧新增保护；Python 未夹紧，风险 R8） |
| 一级 data 缺失 | 降级二级；二级缺失 → 三级关键词；三级也缺 `emote_index.json` → 错误 `❌ 表情库索引缺失: <path>` |
| Ollama 不可用 | **决策 d3**：维持当前「一级 `search()` 抛异常 → 工具层返回 `❌`」（Python 不自动降级，`tools.py:1043-1063`）；备选是自动降级关键词并标 `degraded` |
| 结果为空 | 成功返回 `{"results": [], "reason": "no_match"}`（Go JSON 化后），不是错误 |
| 缺图 | 结果仍返回，`path:""`、`missing:true`（G2） |
| 查询文本落盘 | **不落盘、不进日志**（查询可能含私密表达；R18 词表只用于过滤不用于审计） |

### 4.4 冻结边界（迁移不得改变）

R18 过滤词表与语义、意图词表/负向词、`sim 0.35` 阈值、打分公式 `sim*0.5 + min(bonus,2.5)*0.5`、
`top20` 簇、意图直查 `top*20` 上限、暧昧表情过滤（`emote_matcher_v2.py:81-82`）。
这些是调参结果；任何「更合理」的改动都会让 §5 A2/A3 基线失效，必须走单独变更。

## 5. 验收清单（实施轮逐条可执行）

| 编号 | 验收项 | 命令 / 判据 |
|---|---|---|
| A1 | 工具注册 | `go test ./internal/mcpserver -run TestEmoteToolRegistered -v`：`tools/list` 含新工具，`required=["text"]`，`top` 有默认说明 |
| A2 | 检索一致性 | 固定 3 查询（§2.7）× top-3，Go 与 Python 基线 **file 交集 ≥2/3**；全部命中关系记入夹具 |
| A3 | 行为回归 | 把 `verify_emote_negkw.py` 8 条判定 1:1 转 Go 测试（正/负向意图、R18、困/难过）：`go test ./internal/emote -run TestEmoteRegression -v` 全 PASS |
| A4 | 降级链 | 临时目录复制 `vec_db.npz`→改名，验证走二级；`emote_vectors.json` 也移走 → 三级关键词；三级缺索引 → `❌` 且文案含路径 |
| A5 | 构建可复算 | `go run ./cmd/<构建入口> emote build --index <tmp>/emote_index.json --out <tmp>/emotes --model nomic-embed-text`（子命令名 Phase 0 冻结）：产出 `vec_db.npz`（Python `np.load` 可读）+ `vec_db.json`；**中断后续跑**不重复嵌入（断点续传） |
| A6 | 确定性 | 同参数构建两次，`X` 数组逐元素相等、`labels` 相等；同查询两次结果完全相同 |
| A7 | 性能 | 10 次查询 p95 < 2s（基线：单查询热态 0.25s、三连查 1.24s，§2.7）；首次加载 npz 不重复（缓存命中断言） |
| A8 | 仓库卫生 | `git status --porcelain` 不含 `emotes/`、`*.npz`、`emote_vectors.json`；`make test`、`make lint` exit 0（实测见 §10，`make test` 有 2 个**既存**数据漂移失败不属于本次改动） |
| A9 | 文档对齐 | `docs/mcp-tools.md` 工具表 + `docs/parity.md:52` 从「未迁移」改为「已迁移（链接本文件）」 |

**准备包自身的验收（本轮）**：P1 文件在 `docs/`；P2 §1-§7 的每个 `file:line` 经 `grep`/`sed` 复核（§7 复算）；
P3 三条实测基线命令可复现（`verify_emote_negkw.py`、三查询、Ollama tags）；P4 决策清单每条带推荐与推翻条件。

## 6. 决策清单（2026-11-10 用户按推荐拍板，均已按推荐实施）

| 编号 | 问题 | 推荐 | 备选与影响 |
|---|---|---|---|
| d1 | 工具名 | `emote_find`（Go 侧工具名统一 `<域>_<动作>`，无 `cat_` 前缀；Python 已冻结无线上消费者） | 保留 `cat_find_emote`：与旧提示词/对话历史一致，但与其余 29 个工具命名不一致 |
| d2 | npz 兼容 | **读现有 npz + 构建产物继续写 npz**（Python 可反向验证，验收成本最低） | Go 原生二进制格式：更快但失去 Python 互验，且需迁移一次全部数据 |
| d3 | Ollama 不可用 | 保持现状：一级检索失败 → 工具返回 `❌`（**不静默降级**，避免「看起来能用其实退化成关键词」） | 自动降级到关键词并返回 `degraded:true`：可用性更好，但结果质量变化对 LLM 不透明 |
| d4 | 返回形态 | JSON 数组（`file/desc/score/categories/path/missing/intent_bonus/cluster`），符合 `docs/mcp-tools.md:29-31` | 保持 Python 文本行：对旧 LLM 提示零改动，但与 MCP 返回约定冲突 |
| d5 | 构建范围 | 向量 + 聚类进 Go；视觉簇分析与场景层保留 shell；`emote_index.json` 视为外部输入 | 全链路进 Go（含 qwen2.5vl 视觉调用）：工作量 +1 个执行体，收益低（G3 产物无消费者） |

**默认（可推翻，无需拍板）**：数据目录外置 `-emote-dir` 默认 `~/data/catworkspace/emotes`，仓库不落数据；
构建命令默认 `--dry-run`，写入必须显式 `--out`。

## 7. 复算命令（准备阶段事实的来源；实施验收命令见 §10）

```bash
# 工具定义与三级降级
sed -n '1032,1075p' legacy/python-astrbox/tools/tools.py
# 查询算法
sed -n '82,249p' /home/ruicat/data/catworkspace/emote_vec_db.py
# 构建链
sed -n '1,53p' /home/ruicat/data/catworkspace/build_emote_vectors.py
sed -n '41,80p' /home/ruicat/data/catworkspace/emote_vec_db.py
sed -n '1,54p' /home/ruicat/data/catworkspace/finalize_emote_vec.sh
# 数据清单与结构
python3 - <<'PY'
import json, os, numpy as np, collections
E='/home/ruicat/data/catworkspace/emotes'
idx=json.load(open(f'{E}/emote_index.json')); cats=collections.Counter()
for v in idx.values(): cats.update(v['categories'])
print('index', len(idx), 'categories', len(cats))
print('missing img', sum(1 for n in idx if not os.path.exists(f'{E}/img/{n}')))
d=np.load(f'{E}/vec_db.npz'); print({k:(d[k].shape, str(d[k].dtype)) for k in d.files})
for f in ['emote_index.json','emote_vectors.json','vec_db.npz','vec_db.json','cluster_analysis.json','emote_scenes.json','emote_categories.json','scene_db.npz','scene_db.json']:
    print(f, os.path.getsize(f'{E}/{f}'))
PY
# 行为基线与性能
python3 /home/ruicat/data/catworkspace/verify_emote_negkw.py        # 期望 8/8 PASS
time python3 -c "import sys;sys.path.insert(0,'/home/ruicat/data/catworkspace');from emote_vec_db import search;print([r['file'] for r in search('今天好开心',top=3)])"
# 运行依赖
curl -s http://127.0.0.1:11434/api/tags | python3 -c "import json,sys;[print(m['name'],m['details'].get('embedding_length')) for m in json.load(sys.stdin)['models']]"
# 基线快照（写进验收夹具，替代裸计数）
sha256sum /home/ruicat/data/catworkspace/emotes/{vec_db.npz,cluster_analysis.json,emote_index.json}
```

## 8. 风险与坑

| 编号 | 风险 | 影响 | 处置 |
|---|---|---|---|
| R1 | 嵌入模型漂移：现有索引是 `nomic-embed-text` **768** 维；本机还有 `bge-m3`（1024） | 用错模型 → 相似度全崩 | 构建与启动都校验 `vec_db.json.dim` 与模型维度；换模型 = 全量重建 |
| R2 | KMeans 实现差异（numpy 随机初始化 / 20 轮 / 收敛判定） | 簇边界不同 → 候选池不同 → top-k 变化 | 现成 npz 优先只读；验收 A2 用「交集」而非「完全相同」；重建后基线必须重采样 |
| R3 | 105MB `emote_vectors.json` 的 JSON 解析 | 首载数百 MB 内存与秒级延迟 | 仅回退路径加载；构建期可用流式解析（json.Decoder） |
| R4 | 每次查询重载 npz（Python 现状） | 与 Go 缓存实现结果应一致但性能差异大 | Go 缓存 + mtime/size 失效；A7 断言缓存命中 |
| R5 | 词表硬编码在 Python，改词表无版本号 | 迁移后两侧行为悄悄分叉 | 词表随代码进 Go，冻结；变更走 §4.4 单独流程 |
| R6 | R18 过滤是产品约束 | 放宽 → 事故 | A3 必须覆盖；Go 侧保留完整词表，禁止「按需关闭」开关 |
| R7 | `img/` 888MB 与 3 个缺图 | 路径返回空串/误发 | `missing:true` 显式返回；不复制图库进仓库 |
| R8 | Python 侧 `top` 未夹紧（传 1000 会走全量打分） | 迁移若照抄有性能风险 | Go 侧夹紧 `[1,20]` 并在文档写明（属新增保护，不算行为变更） |
| R9 | 数据目录在仓库外（`~/data/catworkspace`） | 换机/备份易丢 | 验收要求：仓库内不放数据；数据目录单独备份（`tar --zstd`），迁移后写进 `docs/` |
| R10 | 工具返回形态变化（文本→JSON） | LLM 提示若依赖文本格式会退化 | d4 决策后同步更新提示/文档；A2/A3 用解析后的结构化字段判定 |

## 9. 下一步（实施轮草案，**批准后才动手**）

1. **Phase 0 冻结**：拍板 d1-d5；把 §4 契约落成 `internal/emote/contract_test.go` 的夹具常量（词表/阈值/字段名）。
2. **Phase 1 只读检索**：`npz.go` + `data.go` + `embedder.go` + `search.go` + `emoteTools.go`（不写任何数据文件）；A1/A2/A3/A6 通过。
3. **Phase 2 构建**：`build.go` + 子命令（并发+断点续传+原子写）；A5 在临时目录跑通，不碰线上。
4. **Phase 3 收口**：A4/A7/A8/A9 + `runs/emote-tool/60-verification.md` 记录实测输出；更新 `docs/parity.md` 与 `docs/mcp-tools.md`。
5. **写域划分（L3 用）**：执行体 1 = `internal/emote/{npz,data,embedder}.go`；执行体 2 = `internal/emote/{search,keyword}.go` + `emoteTools.go`；
   执行体 3 = `internal/emote/build.go` + `cmd/` 子命令；三片写域互斥，`go test ./internal/emote` 为共享推进判据。

---

**时间**：2026-11-10（实测数据均来自当日本机）。**维护**：实施合并后本文件转为 `docs/mcp-tools.md` 的工具条目 + `docs/parity.md` 一行，本文件归档。

## 10. 实施记录（2026-11-10）

### 10.1 代码落点

| 文件 | 内容 | 有效行量级 |
|---|---|---|
| `internal/emote/emote.go` | 类型/常量/错误/Config/DefaultDir/top 夹紧/分数舍入 | ~230 |
| `internal/emote/npz.go` | npy/npz 读写（numpy 兼容，f4/i8，C order） | ~430 |
| `internal/emote/data.go` | 有序索引加载、meta/npz/簇分析加载、路径解析、mtime 缓存指纹 | ~300 |
| `internal/emote/embedder.go` | Ollama `/api/embeddings` 客户端（60s 超时，失败即错） | ~90 |
| `internal/emote/search.go` | 主检索（Top20 簇 + 意图直查 + 负向 + R18 + 融合打分）与二级向量检索 | ~330 |
| `internal/emote/keyword.go` | 三级关键词打分（INTENT_MAP / 2-4 汉字切片 / 暧昧过滤） | ~130 |
| `internal/emote/client.go` | 三级降级编排、缓存、Summary/Validate | ~140 |
| `internal/emote/build.go` | 向量构建（并发+断点续传+原子写）与 KMeans（seed 42 / 128 簇 / 20 轮） | ~400 |
| `internal/mcpserver/emoteTools.go` | `emote_find` 工具（空文本短路、❌/JSON 约定） | ~60 |
| `cmd/astrbox-emote/main.go` | CLI：`find` / `build` / `info` | ~190 |
| 接线 | `server.go`、`bootstrap.go`、两个 `cmd/*/main.go`（`-emote-dir`/`ASTRBOX_EMOTE_DIR`）、`Makefile`（`bin/astrbox-emote`） | 少量 |

测试：`npz_test.go`、`data_test.go`、`search_test.go`、`keyword_test.go`、`client_test.go`、`build_test.go`、`integration_test.go`（真实数据回归，`ASTRBOX_EMOTE_INTEGRATION=1` 才跑）、`mcpserver/emoteTools_test.go`；夹具 `internal/emote/testdata/sample*.npz`（numpy 生成）。

### 10.2 A1–A9 实测

| 项 | 结果 | 证据 |
|---|---|---|
| A1 工具注册 | PASS | `go test ./internal/mcpserver -run TestEmoteToolRegistered`：`emote_find` 在 `tools/list`，空文本 → `❌ text 不能为空`，缺数据 → `❌ …表情库索引缺失` |
| A2 检索一致性 | PASS（3/3 精确命中） | `ASTRBOX_EMOTE_INTEGRATION=1 go test ./internal/emote -run TestIntegrationEmoteBaseline -v`：三查询 top-3 与 Python 基线**完全相同**，top 分数 0.7888 / 0.8080 / 0.8029 |
| A3 行为回归 | PASS（8/8） | `-run TestIntegrationEmoteRegression -v`：抱抱/抱头/亲亲/害羞/R18/摸头/困/难过 全部 PASS |
| A4 降级链 | PASS（三级） | 临时目录 symlink：有 npz → `source=vec_db`；去 npz → `source=vectors`；只留 index → `source=keyword`；空目录 → `❌ 表情库索引缺失: <路径>` |
| A5 构建可复算 | PASS | `astrbox-emote build -limit 20 -clusters 4 -iterations 5`：20 条嵌入 0.6s；Python `np.load` 读出 `X(20,768) f4 / centers(4,768) f4 / labels(20,) i8`；`vec_db.json` 含 `model / dim / count / built_at / indexSha256`（sha 与 `emote_index.json` 实测 `90b1061a…` 一致）；二次运行 `embedded=0 skipped=20` |
| A6 确定性 | PASS（逐位相同） | 两次全新构建（同参数）Python 比较：`X equal True / centers equal True / labels equal True`，max abs diff 0.0 |
| A7 性能 | PASS | 进程内 p95 = **22.9ms**（10 次热查询，阈值 2s） |
| A8 仓库卫生 | PASS（2026-11-10 收尾复跑） | `make test` exit 0（Go 全包 ok + node 274 pass / 0 fail）；`make lint` exit 0（0 违规；仅既存 `dsh/lib/client.js` no-split 告警）；新增代码无数据文件入库 |
| A9 文档对齐 | PASS | `docs/mcp-tools.md`（30/32 工具 + `emote_find` 条目）、`docs/parity.md:52` 改为已迁移 |

### 10.3 有意差异（相对 Python，均已确认）

1. npz 可用性判定改为「存在即解析」（见 §4.1 注）；损坏报错而非静默回退。
2. 工具层空文本短路；`top` 夹紧到 1–20（Python 不夹紧）。
3. 缺图行给 `path:""` + `missing:true`（Python 打印 `path=None`）；不再回退 `/home/ruicat/Desktop/QQ表情`。
4. 工具返回 JSON（决策 d4），字段名固定。
5. KMeans 随机序列与 numpy 不同 → 重建后的簇编号与 Python 不一致（只读现有 npz 时结果一致；重建后 A2 基线需重采样，风险 R2 已登记）。

### 10.4 遗留

- **`make test` 全量已全绿**（2026-11-10 收尾）：此前 2 个既存失败来自 `scripts/` 新增脚本造成的语料基线漂移
  （`scripts/behavior/cat_metabolism.neko`、`scripts/cat_metabolism.neko`），已按实测刷新冻结基线：
  `internal/neko` 语料 188→190 文件、clean 186→188、totals `[56,1843,5,16,0]`→`[58,1851,5,16,0]`；
  `internal/registry` 注册 186→187。复跑 `make test` exit 0（node 274 pass）。
- `internal/registry` 的注册数来自本地 `data/scripts.json`（gitignore），后续新增脚本注册后需同步该基线（注释已写明来源）。
- 视觉簇分析（`build_cluster_analysis.py`）与场景层（`finalize_emote_vec.sh` / `scene_db.*`）保留 Python/shell，不迁移（G3/d5）。
- 线上 `dsh-mcp` 已重启并验证：`emote_find` 作为桥接工具可用（见 10.2 A1-live 行）。
- 异盘备份仍被 ro 挂载阻塞（见最终回复的复算命令）。
