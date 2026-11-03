// rowItem.js —— Row.Item：实体行 / 诊断行 / 脚本行 / 日志行共用行元件；严重级用 tone 表达，不新造徽标原子。
// 契约来源: design/components.json（Row.Item）；样式载体: rowItem.css。
import {
  resolveEnvironment,
  createElementWithClass,
  createChildElement,
  setOptionalAttribute,
  setDataState,
  emitComponentEvent,
  subscribeComponentEvent,
} from "../runtime/componentRuntime.mjs";

const COMPONENT_CLASS = "nek-row";
const TONE_STATES = new Set(["error", "warning"]);

/** 创建 Row.Item 元件实例。 */
export function createRowItem(props = {}, options = {}) {
  const { documentRef, windowRef } = resolveEnvironment(options);
  const element = createElementWithClass(documentRef, "div", COMPONENT_CLASS);

  const primaryElement = createChildElement(documentRef, element, "span", `${COMPONENT_CLASS}__primary`);
  const secondaryElement = createChildElement(documentRef, element, "span", `${COMPONENT_CLASS}__secondary`);
  const metaElement = createChildElement(documentRef, element, "span", `${COMPONENT_CLASS}__meta`);

  let primary = "";
  let disabled = false;

  const resolveState = (nextProps) => {
    if (nextProps.disabled) return "disabled";
    if (TONE_STATES.has(nextProps.tone)) return nextProps.tone;
    return nextProps.selected ? "selected" : "default";
  };

  const update = (nextProps = {}) => {
    primary = nextProps.primary ?? "";
    disabled = Boolean(nextProps.disabled);
    primaryElement.textContent = primary;
    secondaryElement.textContent = nextProps.secondary ?? "";
    metaElement.textContent = nextProps.meta ?? "";
    setOptionalAttribute(secondaryElement, "data-present", Boolean(nextProps.secondary));
    setOptionalAttribute(metaElement, "data-present", Boolean(nextProps.meta));
    setOptionalAttribute(element, "data-tone", nextProps.tone ?? "neutral");
    setOptionalAttribute(element, "role", "listitem");
    setDataState(element, resolveState(nextProps));
  };

  element.addEventListener("click", () => {
    if (disabled) return;
    emitComponentEvent(windowRef, element, "activate", primary);
  });

  element.addEventListener("contextmenu", (event) => {
    emitComponentEvent(windowRef, element, "contextMenu", {
      id: primary,
      x: event.clientX ?? 0,
      y: event.clientY ?? 0,
    });
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
