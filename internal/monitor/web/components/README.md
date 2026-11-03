# 监视器前端元件实现（m5）

按 `design/components.json` 的 10 个元件契约实现，纯原生 ES module + CSS，无框架、无打包器、无 npm 依赖。
每个元件一个目录，目录名与契约 `source` 路径一致：

```text
internal/monitor/web/components/
  appShell/        appShell.js        appShell.css        verify.sh
  appTopBar/       appTopBar.js       appTopBar.css       verify.sh
  entityTree/      entityTree.js      entityTree.css      verify.sh
  rowItem/         rowItem.js         rowItem.css         verify.sh
  entityDetail/    entityDetail.js    entityDetail.css    verify.sh
  fieldWriteForm/  fieldWriteForm.js  fieldWriteForm.css  verify.sh
  listPanel/       listPanel.js       listPanel.css       verify.sh
  logsStream/      logsStream.js      logsStream.css      verify.sh
  tickResult/      tickResult.js      tickResult.css      verify.sh
  emptyState/      emptyState.js      emptyState.css      verify.sh
  runtime/         componentRuntime.mjs     共用运行时（10 个元件共用，C10 提取）
  test/            fakeDom.mjs / verifyComponent.mjs   本轮验收器（不进页面）
  package.json     type=module 标记（仅让 node 按 ESM 解析元件 .js，不引入依赖）
```

## 元件模块协议（m6 组合层按此装配）

每个元件导出 `create<ComponentName>(props, options?)`，`options = { document?, window? }`（浏览器可省略）。
实例统一为：

| 返回字段 | 说明 |
|---|---|
| `element` | 根元素；样式类名 `nek-*`，状态一律写在 `data-state` |
| `update(nextProps)` | 全量更新 props（不重建根元素） |
| `destroy()` | 释放监听/观察器 |
| `on(eventName, handler)` | 订阅元件事件，返回退订函数；等价于在 `element` 上听同名冒泡 `CustomEvent` |
| `slots[name](node)` | 仅契约声明了 slots 的元件（App.Shell / Entity.Detail）；把子元件挂进插槽容器 |
| `content` | 仅容器类元件（Entity.Tree / Entity.Detail / List.Panel / Logs.Stream）；repeat / conditional 子元件挂这里 |

事件名 = 契约 `onXxx` 的小驼峰形式（`onTickRequest` → `tickRequest`），载荷在 `event.detail`。
`Row.Item` 的 `activate` 载荷是 `props.primary`（组合层用 primary 承载实体 id），`contextMenu` 载荷是 `{ id, x, y }`。

`App.TopBar` 额外提供 `setScrolled(boolean)`：页面滚动到顶栏离开视口时由组合层调用。

## 样式约定

- 元件 CSS 只允许 `var(--<语义令牌>)`（令牌集见 `design/dist/tokens.css`）；颜色/间距/字号/圆角/阴影/动效均为令牌。
- 栅格与布局只使用 `0` / `auto` / `1fr`，不出现 px/rem/em 字面量；无行内 style、无 `!important`、无原始色阶。
- 状态选择器统一为 `[data-state="<契约 states>"]`；`hover` 用 `:hover`。

## m6 装配要点

1. `index.html` 里先引 `design/dist/tokens.css`，再按上面清单逐元件引 `*.css`。
2. `app.js` 用相对路径 import 各元件模块，按 `design/composition.json` 组装；`repeat` / `conditional` 由组合层渲染后挂到 `instance.content`。
3. `App.Shell` 的 7 个插槽：`slots.topbar/sidebar/detail/diagnostics/scripts/logs/tick(node)`；根元素 `data-theme` 切换主题（令牌层按 `[data-theme="light"]` 生效）。
4. 组合层不放具体视觉值（除布局），不改元件内部样式。

## 元件独立验收

```bash
bash internal/monitor/web/components/<dir>/verify.sh
```

`verify.sh` 调用共享验收器 `test/verifyComponent.mjs`，按 `design/components.json` 的真实契约逐项检查：
source/CSS/verify.sh 存在、模块头与工厂导出、CSS 令牌全部已登记且非 primitive、无硬编码视觉值、
契约声明 style 令牌全部被使用、每个 state 有对应选择器、事件与插槽在源码中可查、实例可创建且
`data-state` 合法、插槽容器与 `content` 容器存在、`on()` 事件通路可用、`update()` / `destroy()` 不抛错。
