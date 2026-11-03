// 组合层：按组合树装配元件；本反例引用了未登记元件 Ghost.Panel。
import { createWidgetBox } from "./widgetBox.js";

export function mountPanel(root) {
  root.appendChild(createWidgetBox({ text: "monitor" }));
}
