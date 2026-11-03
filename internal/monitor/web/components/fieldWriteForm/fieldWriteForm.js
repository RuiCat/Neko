// fieldWriteForm.js —— Field.WriteForm：字段写表单（自带提交与校验态），只消费 detail 的 fieldForm 插槽。
// 契约来源: design/components.json（Field.WriteForm）；样式载体: fieldWriteForm.css。
import {
  resolveEnvironment,
  createElementWithClass,
  createChildElement,
  setOptionalAttribute,
  setDataState,
  emitComponentEvent,
  subscribeComponentEvent,
} from "../runtime/componentRuntime.mjs";

const COMPONENT_CLASS = "nek-form";
const ACTIVE_SUBMIT_STATES = new Set(["submitting", "success", "error"]);
const STATE_HINTS = {
  idle: "",
  dirty: "有未提交改动",
  submitting: "提交中…",
  success: "已保存",
  error: "保存失败",
  "read-only": "只读模式",
};

/** 创建 Field.WriteForm 元件实例。 */
export function createFieldWriteForm(props = {}, options = {}) {
  const { documentRef, windowRef } = resolveEnvironment(options);
  const element = createElementWithClass(documentRef, "form", COMPONENT_CLASS);

  const pathField = createChildElement(documentRef, element, "input", `${COMPONENT_CLASS}__path`);
  pathField.setAttribute("type", "text");
  pathField.setAttribute("placeholder", "字段路径，如 engine.rules[0].enabled");

  const valueField = createChildElement(documentRef, element, "textarea", `${COMPONENT_CLASS}__value`);
  valueField.setAttribute("rows", "3");
  valueField.setAttribute("placeholder", "新值");

  const stateElement = createChildElement(documentRef, element, "p", `${COMPONENT_CLASS}__hint`);
  const actions = createChildElement(documentRef, element, "div", `${COMPONENT_CLASS}__actions`);
  const submitButton = createChildElement(documentRef, actions, "button", `${COMPONENT_CLASS}__submit`);
  const cancelButton = createChildElement(documentRef, actions, "button", `${COMPONENT_CLASS}__cancel`);
  submitButton.setAttribute("type", "button");
  cancelButton.setAttribute("type", "button");
  submitButton.textContent = "提交";
  cancelButton.textContent = "取消";

  let entityId = "";
  let readOnly = false;
  let submitState = "idle";
  let dirty = false;

  const resolveState = () => {
    if (readOnly) return "read-only";
    if (ACTIVE_SUBMIT_STATES.has(submitState)) return submitState;
    return dirty ? "dirty" : "idle";
  };

  const render = () => {
    const state = resolveState();
    setDataState(element, state);
    pathField.disabled = readOnly;
    valueField.disabled = readOnly;
    submitButton.disabled = readOnly || submitState === "submitting";
    cancelButton.disabled = readOnly;
    stateElement.textContent = STATE_HINTS[state] ?? "";
    setOptionalAttribute(stateElement, "data-present", Boolean(STATE_HINTS[state]));
  };

  const update = (nextProps = {}) => {
    entityId = nextProps.entityId ?? "";
    readOnly = Boolean(nextProps.readOnly);
    submitState = nextProps.submitState ?? "idle";
    const nextPath = nextProps.path ?? "";
    const nextValue = nextProps.value ?? "";
    if (pathField.value !== nextPath) pathField.value = nextPath;
    if (valueField.value !== nextValue) valueField.value = nextValue;
    dirty = false;
    render();
  };

  const notifyValidate = () => {
    dirty = true;
    emitComponentEvent(windowRef, element, "validate", { path: pathField.value, value: valueField.value });
    render();
  };

  pathField.addEventListener("input", notifyValidate);
  valueField.addEventListener("input", notifyValidate);
  submitButton.addEventListener("click", () => {
    emitComponentEvent(windowRef, element, "submit", {
      entity: entityId,
      path: pathField.value,
      value: valueField.value,
    });
  });
  cancelButton.addEventListener("click", () => {
    dirty = false;
    emitComponentEvent(windowRef, element, "cancel");
    render();
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
