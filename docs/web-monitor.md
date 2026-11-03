# Web 监控台（web-monitor）

监视器是 `bin/dsh-mcp` 进程内的本地页面与 REST/WS 服务，默认监听 `127.0.0.1:8765`。
它与被监视的引擎**同进程**：共享 `bootstrap.Runtime`、`Store`、诊断层与 `mcpserver` 的 tick/写 gate，
因此不新增第二个可写 data-dir 的入口。

- 页面：`http://127.0.0.1:8765/monitor/`
- REST：`/monitor/api/*`
- WebSocket：`/monitor/ws`
- 设计真源：`design/tokens.json` → `design/dist/tokens.css` → `internal/monitor/web/tokens.css`（`make design` 重建）

## 启动

```bash
make build
make monitor
```

`make monitor` 等价于：

```bash
./bin/dsh-mcp -monitor-addr 127.0.0.1:8765
```

数据目录与脚本目录默认是当前目录下的 `data/`、`scripts/`；可用参数或环境变量覆盖：

```bash
./bin/dsh-mcp -data-dir data -scripts-dir scripts -config config.json -monitor-addr 127.0.0.1:8765
ASTRBOX_MONITOR_ADDR=127.0.0.1:8765 ./bin/dsh-mcp -data-dir data -scripts-dir scripts
```

启动成功的日志行（stderr）：

```text
dsh-mcp: monitor 就绪 http://127.0.0.1:8765/monitor/ (readOnly=true)
dsh-mcp: control 就绪 data/dsh-control.sock，MCP stdio 启动（data=data scripts=scripts）
```

前台运行时保持 stdin 打开即可（`make monitor` 就是这样）；`Ctrl-C` 会清理控制面 socket 并关闭页面服务。
禁用监视器：`-monitor-addr ""`（或 `ASTRBOX_MONITOR_ADDR=`）。

## 鉴权

两种形态，由 `config.json` 的 `http_token` 决定：

| 形态 | `http_token` | 页面/静态资源 | `/monitor/api/*` 读 | 写（`tick` / `entities/{id}/set`） | WS |
|---|---|---|---|---|---|
| 只读 | 空 | 免 token | 免 token | `403 write_disabled` | 免 token |
| 带 token | 16–128 位 `[A-Za-z0-9_-]` | 免 token | 需 token | 需 token | 需子协议 token |

- REST token 取 `X-Auth-Token: <token>`，或 `Authorization: Bearer <token>`。
- WS token 走子协议 `astrbox.token.<token>`（服务端回显该子协议）；浏览器写法：

```js
const socket = new WebSocket("ws://127.0.0.1:8765/monitor/ws", ["astrbox.token.abcdefghijklmnop1234"]);
```

- 所有 `/monitor/*` 请求先过 Host 白名单：`127.0.0.1[:port]`、`localhost[:port]`、`[::1][:port]`，否则 `403 forbidden`。
- WS 若带 `Origin`，必须精确等于 `http://` + `Host`；`Origin: null` 或异源一律 `403`。
- 页面与 API 均不返回 `Access-Control-Allow-Origin`；不提供跨源访问。

带 token 的完整示例（`config.json` 与请求命令可直接复制）：

```bash
printf '{"http_token":"abcdefghijklmnop1234"}\n' > config.json
./bin/dsh-mcp -data-dir data -scripts-dir scripts -config config.json -monitor-addr 127.0.0.1:8765
```

```bash
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8765/monitor/
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8765/monitor/api/entities
curl -s -o /dev/null -w '%{http_code}\n' -H 'X-Auth-Token: abcdefghijklmnop1234' http://127.0.0.1:8765/monitor/api/entities
curl -s -o /dev/null -w '%{http_code}\n' -H 'Authorization: Bearer abcdefghijklmnop1234' http://127.0.0.1:8765/monitor/api/entities
```

预期依次为 `200`、`401`、`200`、`200`。

## 端点表

`maxBytes` 取值 `1024..1048576`（默认 `65536`），越界 `400`。`path` 可重复，空 `path` 为 `400`。
所有成功响应为 `200 {"ok":true,...}`，失败为 `{"ok":false,"error":{"code","message"}}`。

| 方法 | 路径 | 参数 / 请求体 | 说明 |
|---|---|---|---|
| GET | `/monitor/` | — | 页面入口（静态，免 token） |
| GET | `/monitor/{asset}` | — | 内嵌静态资源（`app.js`、`app.css`、`tokens.css`、`components/**`） |
| GET | `/monitor/api/health` | — | `version/pid/dataDir/scriptsDir/uptimeMs/engine{...}/diagnostics{...}/monitor{readOnly}` |
| GET | `/monitor/api/entities` | `maxBytes` | `items[{id,type,keys}]/bytes/truncated`，按 id 字典序 |
| GET | `/monitor/api/entities/{id}` | `path`（可重复）、`maxBytes` | 无 `path` 返回 `data`；有 `path` 返回 `values/missing` |
| POST | `/monitor/api/entities/{id}/set` | `{"path":"状态.情绪","value":"愤怒"}` | 共享 gate，最多等 2s；保存失败 `500` 并回滚内存或返回 `dirty:true` |
| GET | `/monitor/api/diagnostics` | `severity`、`source`、`entity`、`script`、`code`、`limit` | `generation/summary/items` |
| GET | `/monitor/api/scripts` | — | `items[{name,path,hash,rules,templates,funcs,events}]` |
| GET | `/monitor/api/scripts/{name}` | `source`（`1`/`true`/`yes`/`on`） | 带 `source` 时附源码；超 256KiB 为 `413 too_large` |
| GET | `/monitor/api/logs` | `kind=state\|control`、`date=YYYY-MM-DD`、`limit` | 尾部有界读取（≤1 MiB），只返回完整行；`limit` 默认 200、上限 2000 |
| GET | `/monitor/api/tick/latest` | — | 最近一次 tick 的同一载荷 |
| POST | `/monitor/api/tick` | `{"scope":"场景"}`（可空） | `TryRunTick`；已有 tick 在飞为 `409 tick_busy` |
| GET | `/monitor/ws` | `topics`、`entity`、`paths` | WebSocket；`hello` 后必发 `snapshot` |

错误码：`400 bad_request`、`401 unauthorized`、`403 forbidden` / `write_disabled`、
`404 not_found`、`409 tick_busy` / `write_busy`、`413 too_large`、`500 internal`。
`409 write_busy` 表示写请求等待共享 gate 超过 2 秒。

## 日志

| 路径 | 内容 | 保留 |
|---|---|---|
| `data/logs/state_YYYY-MM-DD.log` | 每次 tick 的状态快照行 | 7 天（引擎自带 prune） |
| `data/logs/dsh-control-YYYYMMDD.log` | 控制面请求文本行 | 7 天（控制面自带 prune） |
| `data/logs/monitor-audit-YYYYMMDD.log` | 写操作审计（动作/实体/path/结果/来源 IP，不含 token） | 7 天（监视器自带 prune） |

页面「日志」面板读取前两类（只读、拒 symlink、只返回完整行）。审计日志不通过 API 暴露。
审计单行上限 1024 字节；写审计失败只记 stderr，不影响业务响应。

## 故障排查

| 现象 | 原因 | 处理 |
|---|---|---|
| 日志出现 `dsh-mcp: 监视器监听 127.0.0.1:8765 失败，MCP 继续运行` | 端口被占用 | `ss -ltnp 'sport = :8765'` 找到进程并停掉，或换 `-monitor-addr 127.0.0.1:8766` |
| 进程退出码 `3`，日志 `数据目录已被另一个进程占用` | 同一 data-dir 已有 `bin/dsh-mcp` 持有 `.engine.lock` | 停掉旧进程，或给新进程换 `-data-dir`；确认无进程后再删 `data/.engine.lock` |
| 进程退出码 `2`，日志 `http_token 字符集不合法` | `http_token` 不满足 `^[A-Za-z0-9_-]{16,128}$` | 改成 16–128 位合法字符（例：`abcdefghijklmnop1234`） |
| `curl /monitor/` 返回 `404` | 页面未内嵌（`internal/monitor/web/index.html` 缺失） | 检出完整仓库后 `make design && make build`，确认 `internal/monitor/web/index.html` 存在 |
| 页面打开但无样式 | `internal/monitor/web/tokens.css` 缺失或过期 | `make design` 重新生成并 `make build` |
| REST 返回 `401` | 配置了 `http_token` 但请求没带 token | 加 `-H 'X-Auth-Token: abcdefghijklmnop1234'` |
| REST 返回 `403 forbidden` | Host 不在回环白名单 | 用 `127.0.0.1:8765` 或 `localhost:8765` 访问，不要用其它域名 |
| 写请求返回 `403 write_disabled` | 没有配置 `http_token`（只读形态） | 在 `config.json` 配 `http_token` 后重启 |
| 写请求返回 `409 tick_busy` / `409 write_busy` | 有 tick 在飞或 gate 等待超时 | 等 tick 完成重试；持续出现说明 tick 卡住，查日志与 MCP 工具调用 |
| WS 握手 `401` | 子协议 token 缺失或不匹配 | 用 `new WebSocket(url, ["astrbox.token.abcdefghijklmnop1234"])` |
| WS 握手 `403` | `Origin` 与 Host 不同源（或为 `null`） | 从 `http://127.0.0.1:8765/monitor/` 页面连接，不要跨源 |

## 门禁与验收

```bash
make test && make lint && make doctor
bash ~/.dsh/skills/ui-composition/scripts/lint-ui-plan.sh --plan-dir design --tokens design/tokens.json --src-root internal/monitor/web
```

四个反例各自必须非 0：

```bash
for d in design/fixtures/fail/*/; do
  bash ~/.dsh/skills/ui-composition/scripts/lint-ui-plan.sh --plan-dir "$d" --src-root "$d" >/dev/null 2>&1
  printf '%s rc=%d\n' "$d" "$?"
done
```

设计源或页面改动必须触发重建：

```bash
touch design/tokens.json
make -q build
echo $?
```

预期 `1`（需要重建）。
