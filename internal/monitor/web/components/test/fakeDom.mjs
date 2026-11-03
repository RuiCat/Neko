// fakeDom.mjs —— 逐元件验收用的最小 DOM 替身（只服务 verifyComponent.mjs，不进页面运行时）。
// 覆盖元件实际用到的 API：元素树、data-* 属性、文本、属性、事件冒泡与尺寸占位。

/** 事件替身：带 detail/bubbles，支持 preventDefault、stopPropagation 与逐层冒泡。 */
export class FakeCustomEvent {
  constructor(type, init = {}) {
    this.type = type;
    this.detail = init.detail;
    this.bubbles = Boolean(init.bubbles);
    this.defaultPrevented = false;
    this.propagationStopped = false;
    this.target = null;
    this.currentTarget = null;
  }

  preventDefault() {
    this.defaultPrevented = true;
  }

  stopPropagation() {
    this.propagationStopped = true;
  }
}

class FakeClassList {
  constructor(owner) {
    this.owner = owner;
  }

  names() {
    return this.owner.className.split(" ").filter(Boolean);
  }

  add(...names) {
    const merged = new Set([...this.names(), ...names]);
    this.owner.className = [...merged].join(" ");
  }

  remove(...names) {
    const dropped = new Set(names);
    this.owner.className = this.names().filter((name) => !dropped.has(name)).join(" ");
  }

  contains(name) {
    return this.names().includes(name);
  }
}

class FakeElement {
  constructor(documentRef, tagName) {
    this.ownerDocument = documentRef;
    this.tagName = tagName.toUpperCase();
    this.nodeType = 1;
    this.className = "";
    this.textContent = "";
    this.value = "";
    this.disabled = false;
    this.title = "";
    this.clientWidth = 0;
    this.clientHeight = 0;
    this.scrollTop = 0;
    this.scrollHeight = 0;
    this.parentNode = null;
    this.children = [];
    this.attributes = new Map();
    this.listeners = new Map();
    this.classList = new FakeClassList(this);
  }

  setAttribute(name, value) {
    this.attributes.set(name, String(value));
  }

  getAttribute(name) {
    return this.attributes.has(name) ? this.attributes.get(name) : null;
  }

  hasAttribute(name) {
    return this.attributes.has(name);
  }

  removeAttribute(name) {
    this.attributes.delete(name);
  }

  append(...nodes) {
    for (const node of nodes) this.appendChild(node);
  }

  appendChild(node) {
    node.parentNode = this;
    this.children.push(node);
    return node;
  }

  removeChild(node) {
    const index = this.children.indexOf(node);
    if (index >= 0) this.children.splice(index, 1);
    node.parentNode = null;
    return node;
  }

  remove() {
    if (this.parentNode) this.parentNode.removeChild(this);
  }

  replaceChildren(...nodes) {
    this.children = [];
    for (const node of nodes) this.appendChild(node);
  }

  addEventListener(type, handler) {
    const handlers = this.listeners.get(type) ?? [];
    handlers.push(handler);
    this.listeners.set(type, handlers);
  }

  removeEventListener(type, handler) {
    const handlers = this.listeners.get(type) ?? [];
    this.listeners.set(type, handlers.filter((item) => item !== handler));
  }

  dispatchEvent(event) {
    event.target = this;
    let current = this;
    while (current) {
      event.currentTarget = current;
      const handlers = current.listeners.get(event.type) ?? [];
      for (const handler of [...handlers]) handler.call(current, event);
      if (event.propagationStopped || !event.bubbles) break;
      current = current.parentNode;
    }
    return !event.defaultPrevented;
  }

  focus() {}

  select() {}

  setSelectionRange() {}
}

/**
 * 创建最小 document 替身。
 * @param {Record<string, string>} computedValues getComputedStyle 可返回的 CSS 变量表
 * @returns {object} document 替身（含 defaultView / documentElement / body）
 */
export function createDocument(computedValues = {}) {
  const documentRef = {};
  const documentElement = new FakeElement(documentRef, "html");
  const body = new FakeElement(documentRef, "body");
  documentElement.appendChild(body);

  const windowRef = {
    CustomEvent: FakeCustomEvent,
    getComputedStyle() {
      return {
        getPropertyValue(name) {
          return computedValues[name] ?? "";
        },
      };
    },
    ResizeObserver: class {
      constructor(callback) {
        this.callback = callback;
      }

      observe() {}

      disconnect() {}
    },
  };

  documentRef.defaultView = windowRef;
  documentRef.documentElement = documentElement;
  documentRef.body = body;
  documentRef.createElement = (tagName) => new FakeElement(documentRef, tagName);
  return documentRef;
}
