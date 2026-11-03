// 反例 G5：元件层把颜色写成具体值，而不是引用语义令牌。
export function createWidgetBox(props) {
  const node = document.createElement("div");
  node.className = "widget-box";
  node.style.color = "#ff0000";
  node.textContent = String(props.text);
  return node;
}
