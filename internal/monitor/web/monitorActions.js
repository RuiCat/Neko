// monitorActions.js —— 监视器动作层：REST 取数、tick/写入、WS 帧分发（不碰布局）。
import { api, parseValue, today, TOKEN, TOPICS } from "./apiClient.js";
import { createWsClient } from "./wsClient.js";

export const createActions = ({ viewApi, inputs }) => {
  const { view } = viewApi;
  const { renderStatus, renderTree, renderDetail, renderDiagnostics, renderScripts, renderLogs, renderTick } = viewApi;

  const loadHealth = async () => {
    try {
      const body = await api("/health");
      view.version = body.version ?? "";
      view.readOnly = Boolean(body.monitor?.readOnly);
    } catch (error) {
      view.readOnly = true;
    }
    renderStatus();
  };
  const loadEntities = async () => {
    try {
      view.entities = ((await api("/entities")).items ?? []).map(({ id, type }) => ({ id, type }));
    } catch (error) {
      view.entities = [];
    }
    renderTree();
  };
  const loadDiagnostics = async () => {
    try {
      view.diagnostics = (await api("/diagnostics")).items ?? [];
    } catch (error) {
      view.diagnostics = [];
    }
    renderDiagnostics();
  };
  const loadScripts = async () => {
    try {
      view.scripts = (await api("/scripts")).items ?? [];
    } catch (error) {
      view.scripts = [];
    }
    renderScripts();
  };
  const loadLogs = async () => {
    try {
      view.lines = (await api(`/logs?kind=${view.logKind}&date=${view.date}`)).lines ?? [];
    } catch (error) {
      view.lines = [];
    }
    renderLogs();
  };
  const loadDetail = async () => {
    if (view.selectedId === "") {
      view.entity = null;
      renderDetail();
      return;
    }
    try {
      const body = await api(`/entities/${encodeURIComponent(view.selectedId)}`);
      view.entity = body.data ?? null;
      view.truncated = Boolean(body.truncated);
    } catch (error) {
      view.entity = null;
    }
    renderDetail();
  };
  const loadLatestTick = async () => {
    try {
      const body = await api("/tick/latest");
      if (body.result !== null) applyTick(body.result);
    } catch (error) {
      renderTick();
    }
  };
  const refreshAll = () => {
    void Promise.all([loadHealth(), loadEntities(), loadDiagnostics(), loadScripts(), loadLogs(), loadLatestTick()]);
  };

  const applyTick = (payload) => {
    view.latestTick = payload ?? null;
    view.tickError = payload?.errors?.[0] ?? "";
    renderTick();
    void loadEntities();
    void loadDiagnostics();
  };
  const runTick = async () => {
    try {
      const body = await api("/tick", {
        method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ scope: "" }),
      });
      applyTick(body);
    } catch (error) {
      view.tickError = error.code === "tick_busy" ? "已有 tick 在飞" : error.message;
      renderTick();
    }
  };
  const submitWrite = async () => {
    if (view.selectedId === "") return;
    const path = inputs.pathInput.getValue().trim();
    if (path === "") return;
    try {
      await api(`/entities/${encodeURIComponent(view.selectedId)}/set`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ path, value: parseValue(inputs.valueInput.getValue()) }),
      });
      await loadDetail();
    } catch (error) {
      view.tickError = `写入失败：${error.message}`;
      renderTick();
    }
  };
  const selectEntity = (id) => {
    view.selectedId = id;
    void loadDetail();
  };

  const applySnapshot = (topic, data) => {
    if (topic === "state") {
      view.entities = (data?.entities ?? []).map(({ id, type }) => ({ id, type }));
      renderTree();
    }
    if (topic === "diagnostics") {
      view.diagnostics = data?.items ?? [];
      renderDiagnostics();
    }
    if (topic === "tick" && data?.result != null) applyTick(data.result);
  };
  const handleFrame = (frame) => {
    switch (frame.type) {
      case "hello":
        view.version = frame.version || view.version;
        view.readOnly = Boolean(frame.readOnly);
        renderStatus();
        return;
      case "snapshot":
        applySnapshot(frame.topic, frame.data);
        return;
      case "delta":
        if (frame.entity === view.selectedId) void loadDetail();
        return;
      case "tick":
        applyTick(frame.result);
        return;
      case "diagnostics": {
        const touchedId = frame.id ?? frame.record?.id ?? "";
        view.diagnostics = view.diagnostics.filter((item) => item.id !== touchedId);
        if (frame.op !== "removed" && frame.record != null) view.diagnostics = [frame.record, ...view.diagnostics];
        renderDiagnostics();
        return;
      }
      case "log":
        if (frame.kind === view.logKind) {
          view.lines = [...view.lines, { raw: frame.line, level: "info" }];
          renderLogs();
        }
        return;
      default:
        return;
    }
  };

  const ws = createWsClient({
    url: `${location.protocol === "https:" ? "wss" : "ws"}://${location.host}/monitor/ws`,
    token: TOKEN,
    topics: TOPICS,
    onFrame: handleFrame,
    onConnectionChange: (connection) => {
      view.connection = connection;
      renderStatus();
    },
  });

  const boot = () => {
    viewApi.render();
    void refreshAll();
    ws.connect();
  };

  return { refreshAll, runTick, submitWrite, selectEntity, handleFrame, boot };
};
