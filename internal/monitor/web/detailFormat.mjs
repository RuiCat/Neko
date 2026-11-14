/**
 * detailFormat.mjs —— 实体详情字段值的人类可读格式化（纯函数，无 DOM 依赖）。
 *
 * 监视器以前直接把 `JSON.stringify(value).slice(0, 160)` 塞进卡片：嵌套对象被截断在半个引号里、
 * 字符串带着转义引号、长文本没有任何层级。本模块只做「取值 → 文本」这一件事；
 * 夹取行数、悬停全文、跨列由 `monitorView.js` 与 `index.html` 的样式决定。
 */

/** 标量判定：字符串 / 数字 / 布尔（null 当缺失，不算可内联值）。 */
function isScalar(value) {
  return typeof value === "string" || typeof value === "number" || typeof value === "boolean";
}

/** 标量 → 文本：null 记「—」，空字符串记「（空）」。 */
function scalarText(value) {
  if (value === null || value === undefined) return "—";
  if (typeof value === "string") return value === "" ? "（空）" : value;
  return String(value);
}

/** 缩进 JSON 文本（结构化值的兜底展示）。 */
function jsonText(value) {
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

/** 数组 → 文本：全标量用「、」连接；含结构则缩进 JSON；空数组「（空）」。 */
function arrayText(value) {
  if (value.length === 0) return "（空）";
  if (value.every((item) => item === null || isScalar(item))) return value.map(scalarText).join("、");
  return jsonText(value);
}

/** 对象 → 文本：全标量用 `键: 值 · 键: 值` 内联；含结构则缩进 JSON；空对象「（空）」。 */
function objectText(value) {
  const entries = Object.entries(value);
  if (entries.length === 0) return "（空）";
  if (entries.every(([, item]) => item === null || isScalar(item))) {
    return entries.map(([key, item]) => `${key}: ${scalarText(item)}`).join(" · ");
  }
  return jsonText(value);
}

/**
 * 值分类：字符串 / 数字 / 布尔 / 缺失（null）/ 数组 / 对象。
 * @param {unknown} value 字段值
 * @returns {"string" | "number" | "boolean" | "null" | "array" | "object"} 分类
 */
export function valueKind(value) {
  if (value === null || value === undefined) return "null";
  if (Array.isArray(value)) return "array";
  if (typeof value === "object") return "object";
  if (typeof value === "string" || typeof value === "number" || typeof value === "boolean") return typeof value;
  return "object";
}

/**
 * 字段值 → 人类可读文本（唯一出口）。
 * @param {unknown} value 字段值
 * @returns {string} 文本
 */
export function formatFieldValue(value) {
  const kind = valueKind(value);
  if (kind === "array") return arrayText(value);
  if (kind === "object") return objectText(value);
  return scalarText(value);
}

/**
 * 长值判定：超过 90 字符或含换行 → 卡片跨两列并在样式里夹取行数（悬停看全文）。
 * @param {unknown} text 格式化后的文本
 * @returns {boolean} 是否长值
 */
export function isLongFieldText(text) {
  if (typeof text !== "string") return false;
  return text.length > 90 || text.includes("\n");
}
