# data/（本地运行数据，不进版本库）

此目录保存运行时状态，默认被 `.gitignore` 排除：

- `entities.json`：实体状态（写入即变化）。
- `scripts.json`：脚本注册表（apply/remove 时变化）。
- `templates/`：由 `scripts/kernel/` 的模板定义落盘的派生索引（可删，启动会重建）。
- `archives/`：实体快照。
- `logs/`：tick 日志（保留 7 天）。
- `llm_decisions.json`：冲突仲裁记录。
- `training/`：门控训练样本与导出的训练集（含对话摘要，仅本机）。
- `models/`：训练好的门控权重（`gate-mlp-v1.json` 等，仅本机）。
- `emotes/`：本机 QQ 表情库（`emote_index.json`、`vec_db.*`、`img/`，GB 级，gitignored）。
  **不由安装拷贝**；路径是用户设置（`config.json` 的 `emote_dir`，留空 = 未配置），
  安装器只在配置已写相对路径时把它转成绝对路径。

## 首次运行

注册表缺失时，服务会递归扫描 `../scripts/kernel/**/*.neko` 并引导注册（`BootstrapFromSources`），
同时在本地生成 `scripts.json` 与 `templates/`。`../scripts/entities/` 与 `../scripts/scenes/`
是内容蓝图，不会被注册。

实体状态有两类来源：

- **内容蓝图**（不进版本库）：`neko-data -data-dir data -scripts-dir ../scripts apply`
  读仓库根 `private/`（角色/事件数据，gitignored）把 `@实例` 块物化成实体；
  已存在的实体默认跳过，不覆盖运行时状态。
- **运行时存档**（本目录，需要单独备份）：tick 产生的状态变化写入 `entities.json` / `user/entities.json`。

## 备份

`data/` 不进 git，必须单独备份：

```bash
tar --zstd -cf data-$(date -u +%Y%m%dT%H%M%SZ).tar.zst data
```
