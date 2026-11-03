// app.js —— 监视器入口：装配 PetUI 布局、视图与动作（实现分别在 monitorLayout/monitorView/monitorActions）。
import { createLayout } from "./monitorLayout.js";
import { createView } from "./monitorView.js";
import { createActions } from "./monitorActions.js";

const handlers = {};
const layout = createLayout(handlers);
const viewApi = createView(layout.refs);
const actions = createActions({ viewApi, inputs: layout.inputs });

Object.assign(handlers, {
  onTick: actions.runTick,
  onRefresh: actions.refreshAll,
  onWrite: actions.submitWrite,
  onSelect: actions.selectEntity,
  onFilter: (value) => {
    viewApi.view.filter = value;
    viewApi.renderTree();
  },
});

actions.boot();
