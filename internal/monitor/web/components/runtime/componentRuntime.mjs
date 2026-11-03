// componentRuntime.mjs —— 元件共用运行时：DOM 环境解析、根节点构造、属性同步与事件派发。
// 全部元件只经本模块取 DOM 能力（C10：同一逻辑被 10 个元件使用，提取为公用实现）。

/** 解析元件所需的 document / window；缺 document 时立即失败。 */
export function resolveEnvironment(options = {}) {
  const documentRef = options.document ?? globalThis.document;
  if (!documentRef) {
    throw new Error("元件需要 DOM：请提供 options.document 或全局 document");
  }
  const windowRef = options.window ?? documentRef.defaultView ?? globalThis;
  return { documentRef, windowRef };
}

/** 创建带类名的元素；元素不带行内样式，视觉值只来自 CSS 令牌。 */
export function createElementWithClass(documentRef, tagName, className) {
  const element = documentRef.createElement(tagName);
  element.className = className;
  return element;
}

/** 创建子元素并挂到父元素末尾。 */
export function createChildElement(documentRef, parent, tagName, className) {
  const child = createElementWithClass(documentRef, tagName, className);
  parent.append(child);
  return child;
}

/** 写可选属性：undefined/null/false 表示未设置。 */
export function setOptionalAttribute(element, name, value) {
  if (value === undefined || value === null || value === false) {
    element.removeAttribute(name);
    return;
  }
  element.setAttribute(name, value === true ? "true" : String(value));
}

/** 写统一状态通道 data-state（每个元件的状态机都渲染在此属性上）。 */
export function setDataState(element, state) {
  element.setAttribute("data-state", state);
}

/** 派发带 detail 的冒泡 CustomEvent，组合层可逐层转交。 */
export function emitComponentEvent(windowRef, element, type, detail) {
  const eventCtor = windowRef.CustomEvent;
  element.dispatchEvent(new eventCtor(type, { detail, bubbles: true }));
}

/** 订阅元素事件并返回退订函数。 */
export function subscribeComponentEvent(element, type, handler) {
  element.addEventListener(type, handler);
  return () => element.removeEventListener(type, handler);
}
