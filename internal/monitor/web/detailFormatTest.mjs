/**
 * detailFormatTest.mjs —— 实体详情格式化纯函数的回归测试（node --test，无浏览器依赖）。
 *
 * 覆盖：标量原样、缺失/空值占位、扁平对象内联、结构化值缩进 JSON、数组两种形态、长值判定。
 */
import assert from "node:assert/strict";
import { test } from "node:test";
import { formatFieldValue, isLongFieldText, valueKind } from "./detailFormat.mjs";

test("标量：字符串去引号原样、数字/布尔转文本、空串与缺失有占位", () => {
  assert.equal(formatFieldValue("主人"), "主人");
  assert.equal(formatFieldValue("迫不及待想去阳台晒太阳"), "迫不及待想去阳台晒太阳");
  assert.equal(formatFieldValue(92), "92");
  assert.equal(formatFieldValue(false), "false");
  assert.equal(formatFieldValue(""), "（空）");
  assert.equal(formatFieldValue(null), "—");
  assert.equal(formatFieldValue(undefined), "—");
  assert.equal(valueKind(null), "null");
  assert.equal(valueKind("x"), "string");
  assert.equal(valueKind(1), "number");
  assert.equal(valueKind(true), "boolean");
});

test("扁平对象：`键: 值 · 键: 值` 内联（不再出现转义引号与截断）", () => {
  assert.equal(
    formatFieldValue({ 出血: 2, 创伤: 0 }),
    "出血: 2 · 创伤: 0",
  );
  assert.equal(
    formatFieldValue({ 亲密: 100, 妈妈: "猫猫", 害羞: 78 }),
    "亲密: 100 · 妈妈: 猫猫 · 害羞: 78",
  );
  assert.equal(formatFieldValue({ 主人同场景: true }), "主人同场景: true");
  assert.equal(formatFieldValue({}), "（空）");
});

test("结构化对象：缩进 JSON（换行 + 两空格），可被样式夹取后仍可悬停看全文", () => {
  const text = formatFieldValue({ 状态: { 触发: "被抱起来", 等级: 2 } });
  assert.ok(text.includes("\n"), text);
  assert.ok(text.includes('"状态"'), text);
  assert.deepEqual(JSON.parse(text), { 状态: { 触发: "被抱起来", 等级: 2 } });
});

test("数组：全标量用「、」连接；含结构走 JSON；空数组占位", () => {
  assert.equal(formatFieldValue(["梦梦", "幼崽"]), "梦梦、幼崽");
  assert.equal(formatFieldValue([1, 2, 3]), "1、2、3");
  assert.equal(formatFieldValue([]), "（空）");
  const nested = formatFieldValue([{ a: 1 }]);
  assert.ok(nested.includes("\n"), nested);
  assert.deepEqual(JSON.parse(nested), [{ a: 1 }]);
});

test("isLongFieldText：>90 字符或含换行判长值；短文本与非法输入不长", () => {
  assert.equal(isLongFieldText("短文本"), false);
  assert.equal(isLongFieldText("x".repeat(91)), true);
  assert.equal(isLongFieldText("x".repeat(90)), false);
  assert.equal(isLongFieldText("第一行\n第二行"), true);
  assert.equal(isLongFieldText(undefined), false);
  assert.equal(isLongFieldText(42), false);
});

test("真实实体片段：关系/形态/任务/分类 都能读成一行或结构化文本", () => {
  const relation = formatFieldValue({ 亲密: 100, 信赖: 100, 好感度: 90, 妈妈: "猫猫", 安全感: 100, 害羞: 78, 探索欲: 70, 撒娇欲: 92, 爱意: 100, 黏人: 100 });
  assert.ok(relation.startsWith("亲密: 100 · 信赖: 100"), relation);
  assert.ok(!relation.includes('{"'), relation);
  assert.equal(formatFieldValue(["梦梦", "幼崽"]), "梦梦、幼崽");
  assert.ok(formatFieldValue({ 完成标记: false, 当前任务: "日记写好（2026-11-07 22:40）", 状态: "" }).includes("当前任务: 日记写好"));
});
