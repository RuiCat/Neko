// logsStream.js —— Logs.Stream：日志流（等宽字体、尾部跟随）；行由 Row.Item 以 repeat 填充。
// 契约来源: design/components.json（Logs.Stream）；样式载体: logsStream.css。
import {
  resolveEnvironment,
  createElementWithClass,
  createChildElement,
  setOptionalAttribute,
  setDataState,
  emitComponentEvent,
  subscribeComponentEvent,
} from "../runtime/componentRuntime.mjs";

const COMPONENT_CLASS = "nek-logs";

/** 创建 Logs.Stream 元件实例。 */
export function createLogsStream(props = {}, options = {}) {
  const { documentRef, windowRef } = resolveEnvironment(options);
  const element = createElementWithClass(documentRef, "section", COMPONENT_CLASS);

  const toolbar = createChildElement(documentRef, element, "div", `${COMPONENT_CLASS}__toolbar`);
  const kindElement = createChildElement(documentRef, toolbar, "span", `${COMPONENT_CLASS}__kind`);
  const dateField = createChildElement(documentRef, toolbar, "input", `${COMPONENT_CLASS}__date`);
  dateField.setAttribute("type", "date");
  const pauseButton = createChildElement(documentRef, toolbar, "button", `${COMPONENT_CLASS}__pause`);
  const metaElement = createChildElement(documentRef, toolbar, "span", `${COMPONENT_CLASS}__meta`);

  const content = createChildElement(documentRef, element, "div", `${COMPONENT_CLASS}__content`);
  content.setAttribute("data-role", "content");

  const loadMoreButton = createChildElement(documentRef, element, "button", `${COMPONENT_CLASS}__more`);
  loadMoreButton.textContent = "加载更多";

  let paused = false;
  let follow = true;
  let lines = [];
  let truncated = false;

  const resolveState = () => {
    if (!Array.isArray(lines)) return "error";
    if (lines.length === 0) return "empty";
    if (paused) return "paused";
    if (truncated) return "truncated";
    return "live";
  };

  const render = () => {
    pauseButton.textContent = paused ? "继续" : "暂停";
    metaElement.textContent = `${Array.isArray(lines) ? lines.length : 0} 行`;
    setOptionalAttribute(loadMoreButton, "data-present", truncated);
    setDataState(element, resolveState());
    if (follow && !paused) content.scrollTop = content.scrollHeight;
  };

  const update = (nextProps = {}) => {
    paused = Boolean(nextProps.paused);
    follow = nextProps.follow !== false;
    lines = Array.isArray(nextProps.lines) ? nextProps.lines : null;
    truncated = Boolean(nextProps.truncated);
    kindElement.textContent = nextProps.kind ?? "";
    if (dateField.value !== (nextProps.date ?? "")) dateField.value = nextProps.date ?? "";
    setOptionalAttribute(element, "data-kind", nextProps.kind);
    render();
  };

  pauseButton.addEventListener("click", () => {
    paused = !paused;
    emitComponentEvent(windowRef, element, "pauseToggle", paused);
    render();
  });
  dateField.addEventListener("change", () => {
    emitComponentEvent(windowRef, element, "dateChange", dateField.value);
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
