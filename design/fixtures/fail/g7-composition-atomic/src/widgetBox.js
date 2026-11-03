// 干净元件：反例的违规点只在组合层手搓原生控件。
export function createWidgetBox(props) {
  const node = document.createElement("div");
  node.className = "widget-box";
  node.textContent = String(props.text);
  return node;
}
