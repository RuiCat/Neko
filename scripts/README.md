# scripts/ 脚本源目录

`.neko` 源文件按职责归档；**注册名 = 文件名去掉 `.neko`**，跨目录不得重名（注册表按文件名索引，重名会让后者不可见）。注册表缺失时启动会递归扫描本目录并引导注册。

| 目录 | 内容 |
|---|---|
| `core/` | 实体模型、身体/器官、状态同步等核心定义 |
| `behavior/` | 手写日常行为、情绪、关系、任务、亲子交互 |
| `rules/` | 独立规则脚本（`rule_*`） |
| `scene/` | 场景资产与移动 |
| `r18/` | 成年向行为与规则 |
| `meta/` | 安全、日志、自检、脚本治理 |
| `generated/auto/` | 自动生成/填充脚本（`cat_auto_*`、`cat_fill_*`） |
| `generated/derive/` | 运行期衍生规则（`derive_*`） |
| `generated/repair/` | 运行期修复脚本（`repair_*`） |
| `tools/` | 非 `.neko` 工具：`doctorDsh.mjs`、`renderDshPatch.sh`、`verifyProfile.sh` |

`archive/scripts-orphans/`（仓库根）存放**未注册**的旧副本文本（`cat_derive_*`、`cat_repair_*`、`t_simple`），**不在 `scripts/` 下**，不参与启动引导；保留仅为可追溯。

`rules.json`（可选）与脚本同级，启动时由 engine 加载。
