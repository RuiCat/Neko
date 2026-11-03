// appShell.js —— App.Shell：全局布局壳（画布 / 栅格 / 7 个插槽位置），不含任何数据逻辑。
// 契约来源: design/components.json（App.Shell）；样式载体: appShell.css。
import {
  resolveEnvironment,
  createElementWithClass,
  createChildElement,
  setOptionalAttribute,
  setDataState,
  emitComponentEvent,
  subscribeComponentEvent,
} from "../runtime/componentRuntime.mjs";

const COMPONENT_CLASS = "nek-shell";
const SLOT_NAMES = ["topbar", "sidebar", "detail", "diagnostics", "scripts", "logs", "tick"];
const LAYOUT_COLUMNS = { narrow: 1, default: 2, wide: 3 };

/** 创建 App.Shell 元件实例。 */
export function createAppShell(props = {}, options = {}) {
  const { documentRef, windowRef } = resolveEnvironment(options);
  const element = createElementWithClass(documentRef, "div", COMPONENT_CLASS);
  const slotElements = new Map();

  for (const name of SLOT_NAMES) {
    const slotElement = createChildElement(documentRef, element, "div", `${COMPONENT_CLASS}__slot`);
    slotElement.setAttribute("data-slot", name);
    slotElements.set(name, slotElement);
  }

  let theme = "";
  let layout = "";
  let observer = null;

  const readBreakpoint = (name) => {
    const styles = windowRef.getComputedStyle?.(documentRef.documentElement);
    const parsed = Number.parseFloat(styles?.getPropertyValue?.(name) ?? "");
    return Number.isFinite(parsed) ? parsed : null;
  };

  const measureLayout = () => {
    const medium = readBreakpoint("--breakpoint-md");
    const large = readBreakpoint("--breakpoint-lg");
    if (medium === null || large === null) return "default";
    const width = element.clientWidth ?? 0;
    if (width >= large) return "wide";
    if (width >= medium) return "default";
    return "narrow";
  };

  const applyLayout = (nextLayout) => {
    if (nextLayout === layout) return;
    layout = nextLayout;
    setDataState(element, layout);
    emitComponentEvent(windowRef, element, "layoutChange", { columns: LAYOUT_COLUMNS[layout] ?? 1 });
  };

  const update = (nextProps = {}) => {
    const nextTheme = nextProps.theme ?? "dark";
    setOptionalAttribute(element, "data-theme", nextTheme);
    setOptionalAttribute(element, "data-density", nextProps.density ?? "comfortable");
    if (nextTheme !== theme) {
      theme = nextTheme;
      emitComponentEvent(windowRef, element, "themeChange", nextTheme);
    }
    applyLayout(measureLayout());
  };

  const observerCtor = windowRef.ResizeObserver;
  if (typeof observerCtor === "function") {
    observer = new observerCtor(() => applyLayout(measureLayout()));
    observer.observe?.(element);
  }

  const instance = {
    element,
    update,
    destroy() {
      observer?.disconnect?.();
      observer = null;
    },
    on(type, handler) {
      return subscribeComponentEvent(element, type, handler);
    },
    slots: Object.fromEntries(
      SLOT_NAMES.map((name) => [name, (node) => slotElements.get(name).append(node)]),
    ),
  };

  update(props);
  return instance;
}
