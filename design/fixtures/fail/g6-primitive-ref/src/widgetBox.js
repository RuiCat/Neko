// 反例 G6：元件层直接引用原始色阶变量，绕过语义令牌。
export function createWidgetBox(props) {
  const node = document.createElement("div");
  node.className = "widget-box";
  node.style.color = "var(--primitive-blue-500)";
  node.textContent = String(props.text);
  return node;
}
