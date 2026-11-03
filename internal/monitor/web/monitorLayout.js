// monitorLayout.js —— 监视器布局：只用 PetUI 组件（Panel/SplitPane/Navbar/TreeView/Tabs/DataTable/LogConsole）。

const { PetUIExpand: P } = window;

export const createLayout = (handlers) => {
  const root = document.getElementById("monitor-root");
  const app = P.createPanel({ tag: "div", bare: true, className: "monitor-app" });
  root.appendChild(app);

  const statusPill = P.createStatusPill("连接中…", "info");
  const tickButton = P.createButton({ text: "运行 Tick", type: "primary", onClick: () => handlers.onTick?.() });
  const refreshButton = P.createButton({ text: "刷新", type: "secondary", onClick: () => handlers.onRefresh?.() });
  const navbar = P.createNavbar({ items: ["Neko Monitor"] });
  app.body.appendChild(P.createPanel({
    tag: "div", bare: true, className: "monitor-header", body: [navbar, statusPill, tickButton, refreshButton],
  }));

  const splitHost = P.createPanel({ tag: "div", bare: true, className: "monitor-split" });
  app.body.appendChild(splitHost);

  const filterInput = P.createInput({
    placeholder: "过滤实体…", onChange: (value) => handlers.onFilter?.(value),
  });
  const treeView = P.createTreeView([], {
    defaultExpandDepth: 2, onSelect: (path) => handlers.onSelect?.(String(path).split("/").pop()),
  });
  const treePanel = P.createPanel({ title: "实体", bodyFlex: true, body: [filterInput, treeView] });

  const detailList = P.createDescriptionList({ items: [], minColumnWidth: 150 });
  const pathInput = P.createInput({ placeholder: "点分路径，如 身体.体力" });
  const valueInput = P.createInput({ placeholder: "值（JSON 或纯文本）" });
  const writeButton = P.createButton({ text: "写入字段", type: "primary", onClick: () => handlers.onWrite?.() });
  const detailPanel = P.createPanel({
    title: "实体详情",
    bodyFlex: true,
    body: [
      detailList,
      P.createFormRow({ label: "路径", control: pathInput }),
      P.createFormRow({ label: "值", control: valueInput }),
      writeButton,
    ],
  });

  const diagTable = P.createDataTable({
    columns: [
      { key: "severity", title: "级别", width: "var(--space-64)" },
      { key: "code", title: "代码", width: "var(--space-160)" },
      { key: "message", title: "消息" },
    ],
    rows: [], emptyText: "暂无诊断",
  });
  const scriptTable = P.createDataTable({
    columns: [
      { key: "name", title: "脚本", width: "var(--space-160)" },
      { key: "rules", title: "规则", width: "var(--space-64)", align: "right" },
      { key: "hash", title: "hash", width: "var(--space-200)" },
    ],
    rows: [], emptyText: "暂无脚本",
  });
  const logConsole = P.createLogConsole({ max: 400, label: "日志" });
  const tickList = P.createDescriptionList({ items: [{ label: "Tick", value: "尚未运行" }] });
  const tabs = P.createTabs({
    tabs: [
      { label: "诊断", content: diagTable },
      { label: "脚本", content: scriptTable },
      { label: "日志", content: logConsole },
      { label: "Tick", content: tickList },
    ],
  });
  const sidePanel = P.createPanel({ title: "监视", bodyFlex: true, body: [tabs] });
  // Tabs 自身没有高度契约：组合层给它列布局 + 内容区/每个 pane 独立滚动。
  tabs.style.cssText = "display:flex;flex-direction:column;flex:1 1 auto;min-height:0;";
  const panesHost = tabs.children[1];
  if (panesHost instanceof HTMLElement) {
    panesHost.style.cssText = "flex:1 1 auto;min-height:0;overflow:hidden;display:flex;flex-direction:column;padding:var(--space-8);";
  }
  for (let index = 0; index < 4; index += 1) {
    const pane = tabs.getPane(index);
    if (pane !== null) {
      pane.style.flex = "1 1 auto";
      pane.style.minHeight = "0";
      pane.style.overflowY = "scroll";
      pane.style.overflowX = "auto";
    }
  }

  const split = P.createSplitPane({
    container: splitHost.body, direction: "horizontal", reflow: true, containerHeight: null,
    persistKey: "neko.monitor.split",
    panels: [
      { id: "tree", size: "24%", min: 160, content: treePanel },
      { id: "detail", size: "40%", min: 200, content: detailPanel },
      { id: "side", size: "36%", min: 220, content: sidePanel },
    ],
  });
  if (split === null) splitHost.body.append(treePanel, detailPanel, sidePanel);

  return {
    refs: { statusPill, treeView, detailList, diagTable, scriptTable, logConsole, tickList },
    inputs: { pathInput, valueInput },
  };
};
