# data/（本地运行数据，不进版本库）

此目录保存运行时状态，默认被 `.gitignore` 排除：

- `entities.json`：实体状态（写入即变化）。
- `scripts.json`：脚本注册表（apply/remove 时变化）。
- `templates/`：由脚本 `@Data` 落盘的模板。
- `archives/`：实体快照。
- `logs/`：tick 日志（保留 7 天）。
- `llm_decisions.json`：冲突仲裁记录。

## 首次运行

注册表缺失时，服务会扫描 `../scripts/*.neko` 并引导注册（`BootstrapFromSources`），
同时在本地生成 `scripts.json` 与 `templates/`。实体状态不会凭空生成；需要时从备份恢复
`entities.json`。

## 备份

`data/` 不进 git，必须单独备份：

```bash
tar --zstd -cf data-$(date -u +%Y%m%dT%H%M%SZ).tar.zst data
```
