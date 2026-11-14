// monitorView.js —— 监视器视图状态与渲染：把 REST/WS 数据写进 PetUI 组件（不碰布局与网络）。
import { today } from "./apiClient.js";
import { formatFieldValue, isLongFieldText, valueKind } from "./detailFormat.mjs";
import { createRowBalancer } from "./detailGrid.mjs";

export const createView = (refs) => {
  const view = {
    connection: "offline", version: "", readOnly: false, entities: [], filter: "", selectedId: "",
    entity: null, truncated: false, diagnostics: [], scripts: [], lines: [], logKind: "state",
    date: today(), latestTick: null, tickError: "",
  };
  // 详情网格的行高平衡（同行卡片等高、值不裁切）；每次重渲染后重新量一次。
  const detailBalancer = createRowBalancer(refs.detailList);

  const renderStatus = () => {
    const kind = view.connection === "online" ? (view.readOnly ? "warning" : "success") : "error";
    refs.statusPill.set(`${view.connection} · ${view.readOnly ? "只读" : "可写"} · ${view.version || "?"}`, kind);
  };

  const renderTree = () => {
    const keyword = view.filter.trim().toLowerCase();
    const nodes = view.entities
      .filter((entity) => keyword === "" || `${entity.id} ${entity.type}`.toLowerCase().includes(keyword))
      .map((entity) => ({ key: entity.id, label: `${entity.id} · ${entity.type}`, value: entity.id }));
    refs.treeView.setNodes(nodes);
  };

  /**
   * 字段值 → 展示节点：人类可读文本 + 类型钩子；长值挂 data-long（样式夹取行数）并加 title（悬停全文）。
   * @param {unknown} value 字段值
   * @returns {HTMLElement} 值节点
   */
  const createValueNode = (value) => {
    const text = formatFieldValue(value);
    const node = document.createElement("span");
    node.className = `nek-detail-value nek-detail-value--${valueKind(value)}`;
    node.textContent = text;
    // 完整文本照常显示（不再夹取行数）；长值额外挂 title，方便悬停瞟一眼。
    if (isLongFieldText(text)) node.title = text;
    return node;
  };

  /**
   * 字段注解（`__annotations__`）→ 摘要节点：48 条字段定义不摊开成一大坨 JSON，
   * 只给条数，悬停看字段名清单（完整 JSON 仍可从实体原始数据取）。
   * @param {unknown} value 注解对象
   * @returns {HTMLElement} 摘要节点
   */
  const createAnnotationNode = (value) => {
    const keys = value !== null && typeof value === "object" ? Object.keys(value) : [];
    const node = document.createElement("span");
    node.className = "nek-detail-value nek-detail-value--object";
    node.setAttribute("data-summary", "true");
    node.textContent = `${keys.length} 个字段定义`;
    if (keys.length > 0) node.title = keys.join("、");
    return node;
  };

  const renderDetail = () => {
    const selected = view.entities.find((entity) => entity.id === view.selectedId);
    if (view.entity === null) {
      refs.detailList.setItems([{ label: "提示", value: view.selectedId === "" ? "选择左侧实体查看状态" : (view.truncated ? "内容被截断" : "该实体暂无数据") }]);
      return;
    }
    const entries = Object.entries(view.entity);
    const items = [
      { label: "实体", value: view.selectedId },
      { label: "类型", value: selected?.type ?? "" },
      { label: "字段数", value: String(entries.length) },
    ];
    for (const [key, value] of entries) {
      if (key === "__annotations__") {
        items.push({ label: "字段注解", value: createAnnotationNode(value) });
        continue;
      }
      // 每张卡片等宽（不再为长值跨列），行高由网格统一拉伸对齐。
      items.push({ label: key, value: createValueNode(value) });
    }
    refs.detailList.setItems(items);
    detailBalancer.schedule();
  };

  const renderDiagnostics = () => {
    refs.diagTable.setData(view.diagnostics.map((item) => ({
      id: item.id, severity: item.severity, code: item.code, message: item.message,
    })));
  };

  const renderScripts = () => {
    refs.scriptTable.setData(view.scripts.map((item) => ({
      id: item.name, name: item.name, rules: item.rules, hash: item.hash,
    })));
  };

  const renderLogs = () => {
    refs.logConsole.clear();
    for (const line of view.lines.slice(-400)) refs.logConsole.log(line.raw, line.level ?? "info");
  };

  const renderTick = () => {
    const payload = view.latestTick;
    if (payload === null) {
      refs.tickList.setItems([{ label: "Tick", value: view.tickError || "尚未运行" }]);
      return;
    }
    refs.tickList.setItems([
      { label: "tickId", value: payload.tickId ?? "" },
      { label: "status", value: payload.status ?? "" },
      { label: "transitions", value: String((payload.transitions ?? []).length) },
      { label: "conflicts", value: String((payload.conflicts ?? []).length) },
      { label: "rules", value: String(payload.rules_evaluated ?? 0) },
      { label: "errors", value: (payload.errors ?? []).join("; ") },
      { label: "hint", value: payload.hint ?? "" },
    ]);
  };

  const render = () => {
    renderStatus();
    renderTree();
    renderDetail();
    renderDiagnostics();
    renderScripts();
    renderLogs();
    renderTick();
  };

  return {
    view, render, renderStatus, renderTree, renderDetail,
    renderDiagnostics, renderScripts, renderLogs, renderTick,
  };
};
