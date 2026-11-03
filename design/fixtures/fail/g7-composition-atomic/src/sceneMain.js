// 反例 G7：组合层手搓原生按钮（造原子），而不是使用已登记元件。
import { createWidgetBox } from "./widgetBox.js";

export function mountPanel(root) {
  const button = document.createElement("button");
  button.textContent = "tick";
  root.appendChild(button);
  root.appendChild(createWidgetBox({ text: "monitor" }));
}
