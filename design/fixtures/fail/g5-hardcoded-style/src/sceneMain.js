// 组合层：只装配已登记元件，不手写原生控件。
import { createWidgetBox } from "./widgetBox.js";

export function mountPanel(root) {
  root.appendChild(createWidgetBox({ text: "monitor" }));
}
