// entityDetail.js —— Entity.Detail：只读 JSON 视图 + fieldForm 插槽；写表单是插槽里的独立元件。
// 契约来源: design/components.json（Entity.Detail）；样式载体: entityDetail.css。
import {
  resolveEnvironment,
  createElementWithClass,
  createChildElement,
  setOptionalAttribute,
  setDataState,
  emitComponentEvent,
  subscribeComponentEvent,
} from "../runtime/componentRuntime.mjs";

const COMPONENT_CLASS = "nek-detail";
const STATE_LABELS = {
  ready: "",
  loading: "加载中…",
  "missing-path": "路径不存在",
  empty: "暂无实体数据",
  error: "实体加载失败",
};

/** 创建 Entity.Detail 元件实例。 */
export function createEntityDetail(props = {}, options = {}) {
  const { documentRef, windowRef } = resolveEnvironment(options);
  const element = createElementWithClass(documentRef, "section", COMPONENT_CLASS);

  const header = createChildElement(documentRef, element, "div", `${COMPONENT_CLASS}__header`);
  const titleElement = createChildElement(documentRef, header, "h2", `${COMPONENT_CLASS}__title`);
  const typeElement = createChildElement(documentRef, header, "span", `${COMPONENT_CLASS}__type`);
  const actions = createChildElement(documentRef, header, "div", `${COMPONENT_CLASS}__actions`);
  const refreshButton = createChildElement(documentRef, actions, "button", `${COMPONENT_CLASS}__action`);
  const copyButton = createChildElement(documentRef, actions, "button", `${COMPONENT_CLASS}__action`);
  refreshButton.textContent = "刷新";
  copyButton.textContent = "复制";

  const pathsElement = createChildElement(documentRef, element, "div", `${COMPONENT_CLASS}__paths`);
  const stateElement = createChildElement(documentRef, element, "div", `${COMPONENT_CLASS}__state`);
  const codeElement = createChildElement(documentRef, element, "pre", `${COMPONENT_CLASS}__code`);
  const truncatedElement = createChildElement(documentRef, element, "span", `${COMPONENT_CLASS}__truncated`);
  truncatedElement.textContent = "输出已截断";

  const slotElement = createChildElement(documentRef, element, "div", `${COMPONENT_CLASS}__slot`);
  slotElement.setAttribute("data-slot", "fieldForm");

  const content = createChildElement(documentRef, element, "div", `${COMPONENT_CLASS}__content`);
  content.setAttribute("data-role", "content");

  let payload = "";
  let selectedPaths = [];

  const renderPaths = () => {
    pathsElement.replaceChildren();
    for (const pathValue of selectedPaths) {
      const chip = createChildElement(documentRef, pathsElement, "button", `${COMPONENT_CLASS}__path`);
      chip.setAttribute("type", "button");
      chip.textContent = pathValue;
      chip.addEventListener("click", () => emitComponentEvent(windowRef, element, "pathToggle", pathValue));
    }
    setOptionalAttribute(pathsElement, "data-present", selectedPaths.length > 0);
  };

  const update = (nextProps = {}) => {
    const status = nextProps.status ?? "ready";
    payload = nextProps.payload ?? "";
    selectedPaths = Array.isArray(nextProps.selectedPaths) ? nextProps.selectedPaths : [];
    titleElement.textContent = nextProps.entityId ?? "";
    typeElement.textContent = nextProps.entityType ?? "";
    setOptionalAttribute(typeElement, "data-present", Boolean(nextProps.entityType));
    codeElement.textContent = payload;
    setOptionalAttribute(truncatedElement, "data-present", Boolean(nextProps.truncated));
    stateElement.textContent = STATE_LABELS[status] ?? "";
    setOptionalAttribute(stateElement, "data-present", Boolean(STATE_LABELS[status]));
    setDataState(element, status);
    renderPaths();
  };

  refreshButton.addEventListener("click", () => emitComponentEvent(windowRef, element, "refresh"));
  copyButton.addEventListener("click", () => emitComponentEvent(windowRef, element, "copy", payload));
  content.addEventListener("retry", () => emitComponentEvent(windowRef, element, "refresh"));
  content.addEventListener("action", () => emitComponentEvent(windowRef, element, "refresh"));

  const instance = {
    element,
    content,
    update,
    destroy() {},
    on(type, handler) {
      return subscribeComponentEvent(element, type, handler);
    },
    slots: {
      fieldForm(node) {
        slotElement.append(node);
      },
    },
  };

  update(props);
  return instance;
}
