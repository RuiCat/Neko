// entityTree.js —— Entity.Tree：实体树容器（过滤 / 选中 / 行与空态由组合层填充），只负责状态与事件转交。
// 契约来源: design/components.json（Entity.Tree）；样式载体: entityTree.css。
import {
  resolveEnvironment,
  createElementWithClass,
  createChildElement,
  setOptionalAttribute,
  setDataState,
  emitComponentEvent,
  subscribeComponentEvent,
} from "../runtime/componentRuntime.mjs";

const COMPONENT_CLASS = "nek-tree";

/** 创建 Entity.Tree 元件实例。 */
export function createEntityTree(props = {}, options = {}) {
  const { documentRef, windowRef } = resolveEnvironment(options);
  const element = createElementWithClass(documentRef, "nav", COMPONENT_CLASS);

  const filterElement = createChildElement(documentRef, element, "input", `${COMPONENT_CLASS}__filter`);
  filterElement.setAttribute("type", "search");
  filterElement.setAttribute("placeholder", "按 id / type 过滤");

  const content = createChildElement(documentRef, element, "div", `${COMPONENT_CLASS}__content`);
  content.setAttribute("data-role", "content");

  let entities = [];
  let filter = "";
  let status = "idle";

  const visibleCount = () => {
    const keyword = filter.trim().toLowerCase();
    if (keyword === "") return entities.length;
    return entities.filter((entity) =>
      `${entity.id ?? ""} ${entity.type ?? ""}`.toLowerCase().includes(keyword)).length;
  };

  const resolveState = () => {
    if (status === "error") return "error";
    if (status === "loading" || status === "idle") return "loading";
    return visibleCount() === 0 ? "empty" : "ready";
  };

  const update = (nextProps = {}) => {
    entities = Array.isArray(nextProps.entities) ? nextProps.entities : [];
    filter = nextProps.filter ?? "";
    status = nextProps.status ?? "idle";
    if (filterElement.value !== filter) filterElement.value = filter;
    setOptionalAttribute(element, "data-selected", nextProps.selectedId);
    setDataState(element, resolveState());
  };

  filterElement.addEventListener("input", () => {
    filter = filterElement.value;
    emitComponentEvent(windowRef, element, "filterChange", filter);
    setDataState(element, resolveState());
  });

  content.addEventListener("activate", (event) => {
    emitComponentEvent(windowRef, element, "select", event.detail);
  });
  content.addEventListener("retry", () => {
    emitComponentEvent(windowRef, element, "retry");
  });
  content.addEventListener("action", () => {
    emitComponentEvent(windowRef, element, "retry");
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
