// tickResult.js —— Tick.Result：tick 结果摘要（tick_id / 规则数 / 实体数 / 耗时），busy 对应 409 tick_busy。
// 契约来源: design/components.json（Tick.Result）；样式载体: tickResult.css。
import {
  resolveEnvironment,
  createElementWithClass,
  createChildElement,
  setOptionalAttribute,
  setDataState,
  emitComponentEvent,
  subscribeComponentEvent,
} from "../runtime/componentRuntime.mjs";

const COMPONENT_CLASS = "nek-tick";
const STATUS_LABELS = {
  idle: "待运行",
  running: "运行中",
  success: "成功",
  failed: "失败",
  busy: "正在运行",
};

/** 创建 Tick.Result 元件实例。 */
export function createTickResult(props = {}, options = {}) {
  const { documentRef, windowRef } = resolveEnvironment(options);
  const element = createElementWithClass(documentRef, "div", COMPONENT_CLASS);

  const statusElement = createChildElement(documentRef, element, "span", `${COMPONENT_CLASS}__status`);
  const tickElement = createChildElement(documentRef, element, "span", `${COMPONENT_CLASS}__id`);
  const rulesElement = createChildElement(documentRef, element, "span", `${COMPONENT_CLASS}__metric`);
  const entitiesElement = createChildElement(documentRef, element, "span", `${COMPONENT_CLASS}__metric`);
  const durationElement = createChildElement(documentRef, element, "span", `${COMPONENT_CLASS}__metric`);
  const errorElement = createChildElement(documentRef, element, "span", `${COMPONENT_CLASS}__error`);
  const retryButton = createChildElement(documentRef, element, "button", `${COMPONENT_CLASS}__retry`);
  retryButton.textContent = "重试";

  let tickId = "";

  const update = (nextProps = {}) => {
    const status = nextProps.status ?? "idle";
    tickId = nextProps.tickId ?? "";
    statusElement.textContent = STATUS_LABELS[status] ?? status;
    tickElement.textContent = tickId === "" ? "无 tick" : tickId;
    rulesElement.textContent = `规则 ${nextProps.ruleCount ?? 0}`;
    entitiesElement.textContent = `实体 ${nextProps.entityCount ?? 0}`;
    durationElement.textContent = `耗时 ${nextProps.durationMs ?? 0}`;
    errorElement.textContent = nextProps.error ?? "";
    setOptionalAttribute(errorElement, "data-present", Boolean(nextProps.error));
    setOptionalAttribute(retryButton, "data-present", status === "failed");
    setOptionalAttribute(element, "data-status", status);
    setDataState(element, status);
  };

  retryButton.addEventListener("click", (event) => {
    event.stopPropagation();
    emitComponentEvent(windowRef, element, "retry");
  });
  element.addEventListener("click", () => {
    emitComponentEvent(windowRef, element, "openDetails", tickId);
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
