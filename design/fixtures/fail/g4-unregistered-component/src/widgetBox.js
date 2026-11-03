// 干净元件：反例的违规点只在组合树引用了未登记元件。
export function createWidgetBox(props) {
  const node = document.createElement("div");
  node.className = "widget-box";
  node.textContent = String(props.text);
  return node;
}
