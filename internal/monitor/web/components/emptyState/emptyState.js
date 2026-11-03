// emptyState.js —— Empty.State：空 / 错误 / 加载 / 只读拒绝共用的状态块，由 props.kind 选择状态。
// 契约来源: design/components.json（Empty.State）；样式载体: emptyState.css。
import {
  resolveEnvironment,
  createElementWithClass,
  createChildElement,
  setOptionalAttribute,
  setDataState,
  emitComponentEvent,
  subscribeComponentEvent,
} from "../runtime/componentRuntime.mjs";

const COMPONENT_CLASS = "nek-empty";

/** 创建 Empty.State 元件实例。 */
export function createEmptyState(props = {}, options = {}) {
  const { documentRef, windowRef } = resolveEnvironment(options);
  const element = createElementWithClass(documentRef, "div", COMPONENT_CLASS);

  const titleElement = createChildElement(documentRef, element, "div", `${COMPONENT_CLASS}__title`);
  const hintElement = createChildElement(documentRef, element, "p", `${COMPONENT_CLASS}__hint`);
  const actionButton = createChildElement(documentRef, element, "button", `${COMPONENT_CLASS}__action`);

  let kind = "empty";

  const update = (nextProps = {}) => {
    kind = nextProps.kind ?? "empty";
    titleElement.textContent = nextProps.title ?? "";
    hintElement.textContent = nextProps.hint ?? "";
    const actionLabel = nextProps.actionLabel ?? (kind === "error" ? "重试" : "");
    actionButton.textContent = actionLabel;
    setOptionalAttribute(hintElement, "data-present", Boolean(nextProps.hint));
    setOptionalAttribute(actionButton, "data-present", actionLabel !== "");
    setDataState(element, kind);
  };

  actionButton.addEventListener("click", () => {
    if (kind === "error") {
      emitComponentEvent(windowRef, element, "retry");
      return;
    }
    emitComponentEvent(windowRef, element, "action");
  });

  const instance = {
    element,
    update,
    destroy() {},
    on(type, handler) {
      return subscribeComponentEvent(element, type, handler);
    },
  };

  update(props);
  return instance;
}
