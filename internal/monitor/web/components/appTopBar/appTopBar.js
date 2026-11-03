// appTopBar.js —— App.TopBar：标题 / 版本 / 连接状态 / tick 与重连入口；连接态色只用 intent.success。
// 契约来源: design/components.json（App.TopBar）；样式载体: appTopBar.css。
import {
  resolveEnvironment,
  createElementWithClass,
  createChildElement,
  setOptionalAttribute,
  setDataState,
  emitComponentEvent,
  subscribeComponentEvent,
} from "../runtime/componentRuntime.mjs";

const COMPONENT_CLASS = "nek-topbar";
const CONNECTION_LABELS = { live: "已连接", reconnecting: "重连中", offline: "离线" };

/** 创建 App.TopBar 元件实例。 */
export function createAppTopBar(props = {}, options = {}) {
  const { documentRef, windowRef } = resolveEnvironment(options);
  const element = createElementWithClass(documentRef, "header", COMPONENT_CLASS);

  const identity = createChildElement(documentRef, element, "div", `${COMPONENT_CLASS}__identity`);
  const titleElement = createChildElement(documentRef, identity, "span", `${COMPONENT_CLASS}__title`);
  const versionElement = createChildElement(documentRef, identity, "span", `${COMPONENT_CLASS}__version`);

  const statusElement = createChildElement(documentRef, element, "span", `${COMPONENT_CLASS}__connection`);
  const actions = createChildElement(documentRef, element, "div", `${COMPONENT_CLASS}__actions`);
  const tickButton = createChildElement(documentRef, actions, "button", `${COMPONENT_CLASS}__action`);
  const reconnectButton = createChildElement(documentRef, actions, "button", `${COMPONENT_CLASS}__action`);

  tickButton.textContent = "运行 tick";
  reconnectButton.textContent = "重连";

  let scrolled = false;
  let readOnly = false;
  let connection = "offline";

  const resolveState = () => {
    if (readOnly) return "read-only";
    if (connection === "offline") return "offline";
    if (connection === "reconnecting") return "connecting";
    return scrolled ? "scrolled" : "default";
  };

  const render = () => {
    setDataState(element, resolveState());
    setOptionalAttribute(element, "data-connection", connection);
    statusElement.setAttribute("data-state", connection);
    statusElement.textContent = CONNECTION_LABELS[connection] ?? connection;
    tickButton.disabled = readOnly || connection === "offline";
    reconnectButton.disabled = connection === "live";
  };

  const update = (nextProps = {}) => {
    readOnly = Boolean(nextProps.readOnly);
    connection = nextProps.connection ?? "offline";
    titleElement.textContent = nextProps.title ?? "";
    versionElement.textContent = nextProps.version ?? "";
    setOptionalAttribute(versionElement, "data-present", Boolean(nextProps.version));
    render();
  };

  tickButton.addEventListener("click", () => emitComponentEvent(windowRef, element, "tickRequest"));
  reconnectButton.addEventListener("click", () => emitComponentEvent(windowRef, element, "reconnect"));

  const instance = {
    element,
    update,
    destroy() {},
    on(type, handler) {
      return subscribeComponentEvent(element, type, handler);
    },
    setScrolled(value) {
      scrolled = Boolean(value);
      render();
    },
  };

  update(props);
  return instance;
}
