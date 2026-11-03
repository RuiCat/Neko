// listPanel.js —— List.Panel：诊断 / 脚本两个列表共用的面板容器（标题与列由 props 区分，行由组合层填充）。
// 契约来源: design/components.json（List.Panel）；样式载体: listPanel.css。
import {
  resolveEnvironment,
  createElementWithClass,
  createChildElement,
  setOptionalAttribute,
  setDataState,
  emitComponentEvent,
  subscribeComponentEvent,
} from "../runtime/componentRuntime.mjs";

const COMPONENT_CLASS = "nek-panel";

/** 创建 List.Panel 元件实例。 */
export function createListPanel(props = {}, options = {}) {
  const { documentRef, windowRef } = resolveEnvironment(options);
  const element = createElementWithClass(documentRef, "section", COMPONENT_CLASS);

  const header = createChildElement(documentRef, element, "div", `${COMPONENT_CLASS}__header`);
  const titleElement = createChildElement(documentRef, header, "h2", `${COMPONENT_CLASS}__title`);
  const badgeElement = createChildElement(documentRef, header, "span", `${COMPONENT_CLASS}__badge`);
  const loadMoreButton = createChildElement(documentRef, header, "button", `${COMPONENT_CLASS}__more`);

  const stateElement = createChildElement(documentRef, element, "div", `${COMPONENT_CLASS}__state`);
  const content = createChildElement(documentRef, element, "div", `${COMPONENT_CLASS}__content`);
  content.setAttribute("data-role", "content");
  loadMoreButton.textContent = "加载更多";

  let status = "idle";
  let items = [];
  let truncated = false;

  const resolveState = () => {
    if (status === "error") return "error";
    if (status === "loading" || status === "idle") return "loading";
    if (items.length === 0) return "empty";
    return truncated ? "truncated" : "ready";
  };

  const update = (nextProps = {}) => {
    status = nextProps.status ?? "idle";
    items = Array.isArray(nextProps.items) ? nextProps.items : [];
    truncated = Boolean(nextProps.truncated);
    const state = resolveState();
    titleElement.textContent = nextProps.title ?? "";
    badgeElement.textContent = String(items.length);
    setOptionalAttribute(element, "data-kind", nextProps.kind ?? "diagnostics");
    setOptionalAttribute(element, "data-selected", nextProps.selectedId);
    setOptionalAttribute(loadMoreButton, "data-present", truncated);
    stateElement.textContent = state === "loading" ? "加载中…" : state === "error" ? "加载失败" : "";
    setOptionalAttribute(stateElement, "data-present", state === "loading" || state === "error");
    setDataState(element, state);
  };

  content.addEventListener("activate", (event) => {
    emitComponentEvent(windowRef, element, "select", event.detail);
  });
  content.addEventListener("retry", () => {
    emitComponentEvent(windowRef, element, "retry");
  });
  content.addEventListener("action", () => {
    emitComponentEvent(windowRef, element, "retry");
  });
  loadMoreButton.addEventListener("click", () => {
    emitComponentEvent(windowRef, element, "loadMore");
  });

  const instance = {
    element,
    content,
    update,
    destroy() {},
    on(type, handler) {
      return subscribeComponentEvent(element, type, handler);
    },
  };

  update(props);
  return instance;
}
