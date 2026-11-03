// monitorView.js —— 监视器视图状态与渲染：把 REST/WS 数据写进 PetUI 组件（不碰布局与网络）。
import { today } from "./apiClient.js";

export const createView = (refs) => {
  const view = {
    connection: "offline", version: "", readOnly: false, entities: [], filter: "", selectedId: "",
    entity: null, truncated: false, diagnostics: [], scripts: [], lines: [], logKind: "state",
    date: today(), latestTick: null, tickError: "",
  };

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

  const renderDetail = () => {
    const selected = view.entities.find((entity) => entity.id === view.selectedId);
    if (view.entity === null) {
      refs.detailList.setItems([{ label: "提示", value: view.selectedId === "" ? "选择左侧实体查看状态" : (view.truncated ? "内容被截断" : "该实体暂无数据") }]);
      return;
    }
    const items = [
      { label: "实体", value: view.selectedId },
      { label: "类型", value: selected?.type ?? "" },
      { label: "字段数", value: String(Object.keys(view.entity).length) },
    ];
    for (const [key, value] of Object.entries(view.entity).slice(0, 24)) {
      items.push({ label: key, value: JSON.stringify(value).slice(0, 160) });
    }
    refs.detailList.setItems(items);
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
