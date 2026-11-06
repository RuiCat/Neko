/**
 * 日记插件自测：格式往返、幂等写入、范围/关键词检索、汇总、工具定义合法性、插件挂载。
 *
 * 工具定义合法性优先用**宿主真实校验器**（dsh-tools 的 assertSupportedJsonSchema）复验；
 * 找不到宿主包时退化为本文件自带的最小断言，并在输出里标明降级——不假装已验证。
 * 用法：`node lib/test/testDiary.mjs`（退出码 0 全过）。
 */
import assert from "node:assert/strict";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { entryId, formatDate, parseDate, parseDay, renderEntry } from "../diaryFormat.mjs";
import { listDates, readDiaryEntries, summarizeDays, writeDiaryEntry } from "../diaryStore.mjs";
import { buildDiaryTools } from "../diaryTools.mjs";
import { apply } from "../plugin.mjs";

let passed = 0;
const failures = [];

/** 跑一条用例，失败也继续，最后统一报告。 */
async function check(name, fn) {
  try {
    await fn();
    passed += 1;
    console.log(`ok   ${name}`);
  } catch (error) {
    failures.push({ name, error });
    console.log(`FAIL ${name}: ${error.message}`);
  }
}

/** 造一个临时日记目录。 */
function tempDir() {
  return mkdtempSync(join(tmpdir(), "dsh-diary-"));
}

const DIR = tempDir();

await check("日期严格校验：合法与非法", () => {
  assert.equal(formatDate(parseDate("2026-02-28")), "2026-02-28");
  assert.throws(() => parseDate("2026-02-30"), /不是合法日期/);
  assert.throws(() => parseDate("2026/02/28"), /YYYY-MM-DD/);
});

await check("条目渲染→解析往返一致", () => {
  const draft = { date: "2026-10-25", time: "14:32", title: "写插件", body: "第一行\n第二行" };
  const id = entryId(draft);
  const text = `# 2026-10-25（周六）\n\n${renderEntry({ ...draft, id, tags: ["插件", "日记"] })}`;
  const [entry] = parseDay(text, "2026-10-25");
  assert.equal(entry.id, id);
  assert.equal(entry.time, "14:32");
  assert.equal(entry.title, "写插件");
  assert.deepEqual(entry.tags, ["插件", "日记"]);
  assert.equal(entry.body, "第一行\n第二行");
});

await check("写入幂等：同输入第二次 skipped", () => {
  const first = writeDiaryEntry(DIR, { title: "幂等", body: "同一内容", tags: ["t"], date: "2026-10-01", time: "10:00" });
  const second = writeDiaryEntry(DIR, { title: "幂等", body: "同一内容", tags: ["t"], date: "2026-10-01", time: "10:00" });
  assert.equal(first.status, "written");
  assert.equal(second.status, "skipped");
  assert.equal(first.id, second.id);
});

await check("换正文 → 新条目（同 id 不重复）", () => {
  const same = writeDiaryEntry(DIR, { title: "幂等2", body: "内容A", date: "2026-10-01", time: "11:00" });
  const changed = writeDiaryEntry(DIR, { title: "幂等2", body: "内容B", date: "2026-10-01", time: "11:00" });
  assert.equal(changed.status, "written");
  assert.notEqual(changed.id, same.id);
});

await check("拒绝空标题/空正文", () => {
  assert.throws(() => writeDiaryEntry(DIR, { title: " ", body: "x" }), /title/);
  assert.throws(() => writeDiaryEntry(DIR, { title: "x", body: " " }), /body/);
});

await check("按日期范围检索", () => {
  writeDiaryEntry(DIR, { title: "范围内", body: "hello", date: "2026-10-05" });
  writeDiaryEntry(DIR, { title: "范围外", body: "hello", date: "2026-09-01" });
  const result = readDiaryEntries(DIR, { from: "2026-10-01", to: "2026-10-31" });
  const titles = result.entries.map((entry) => entry.title);
  assert.ok(titles.includes("范围内"));
  assert.ok(!titles.includes("范围外"));
});

await check("关键词排序：标题命中优先于正文命中", () => {
  const dir = tempDir();
  writeDiaryEntry(dir, { title: "普通标题", body: "这里提到 插件 一次", date: "2026-10-02" });
  writeDiaryEntry(dir, { title: "插件 标题", body: "无关正文", date: "2026-10-02" });
  const result = readDiaryEntries(dir, { query: "插件" });
  assert.equal(result.entries[0].title, "插件 标题");
});

await check("limit 截断标记", () => {
  const dir = tempDir();
  for (let index = 0; index < 5; index += 1) {
    writeDiaryEntry(dir, { title: `条目${index}`, body: "x", date: "2026-10-03", time: `1${index}:00` });
  }
  const result = readDiaryEntries(dir, { limit: 2 });
  assert.equal(result.entries.length, 2);
  assert.equal(result.truncated, true);
});

await check("日期列表与汇总（新→旧）", () => {
  const dates = listDates(DIR);
  assert.deepEqual(dates, [...dates].sort().reverse());
  const summary = summarizeDays(DIR, 2);
  assert.ok(summary.length <= 2);
  assert.ok(summary.every((day) => day.count >= 1 && Array.isArray(day.titles)));
});

await check("工具定义数量与命名", () => {
  const names = buildDiaryTools().map((tool) => tool.name);
  assert.deepEqual(names, ["diary_write", "diary_read", "diary_list"]);
});

await check("工具定义合法：宿主校验器（不可用则降级断言）", async () => {
  const hostPath =
    "/usr/lib/node_modules/@deepseek-ai/dsh/node_modules/@deepseek-ai/dsh-tools/lib/index.js";
  let checker = null;
  try {
    const host = await import(hostPath);
    checker = host.assertSupportedJsonSchema;
  } catch {
    console.log("     （宿主 dsh-tools 不可用，降级为最小断言）");
  }
  for (const tool of buildDiaryTools()) {
    assert.equal(typeof tool.name, "string");
    assert.equal(typeof tool.description, "string");
    assert.equal(typeof tool.output.render, "function");
    assert.equal(typeof tool.execute, "function");
    if (checker) {
      checker(tool.parameters);
      checker(tool.output.schema);
    }
  }
});

await check("工具执行：返回 ok/text 且形状符合输出声明", async () => {
  const tools = Object.fromEntries(buildDiaryTools().map((tool) => [tool.name, tool]));
  const exec = { agent: { cwd: DIR } };
  const written = await tools.diary_write.execute(
    { title: "工具层", body: "经 execute 写入", tags: ["测试"] },
    exec,
  );
  assert.equal(written.ok, true);
  assert.match(written.text, /已写入/);
  const listed = await tools.diary_list.execute({ limit: 5 }, exec);
  assert.equal(listed.ok, true);
  assert.match(listed.text, /工具层/);
  const read = await tools.diary_read.execute({ query: "工具层" }, exec);
  assert.equal(read.ok, true);
  assert.match(read.text, /经 execute 写入/);
  for (const value of [written, listed, read]) {
    const content = tools.diary_write.output.render({}, value);
    assert.equal(content[0].type, "text");
    assert.equal(typeof content[0].text, "string");
  }
});

await check("工具失败不抛：返回 ok=false 且带原因", async () => {
  const tools = Object.fromEntries(buildDiaryTools().map((tool) => [tool.name, tool]));
  const result = await tools.diary_write.execute({ title: "", body: "x" }, { agent: { cwd: DIR } });
  assert.equal(result.ok, false);
  assert.match(result.text, /title/);
});

await check("插件挂载：注册 3 个工具，DSH_DIARY=off 时不注册", () => {
  const registered = [];
  const ctx = { tools: { register: (definition) => registered.push(definition.name) } };
  apply(ctx, {});
  assert.deepEqual(registered, ["diary_write", "diary_read", "diary_list"]);

  const off = [];
  process.env.DSH_DIARY = "off";
  apply({ tools: { register: (definition) => off.push(definition.name) } }, {});
  delete process.env.DSH_DIARY;
  assert.deepEqual(off, []);
});


await check("坏文件硬报错：不重建、不返回空数组", async () => {
  const dir = tempDir();
  const { writeFileSync } = await import("node:fs");
  writeFileSync(join(dir, "2026-10-09.md"), "这是一段被手改坏的内容\n没有条目头\n", "utf8");
  assert.throws(() => readDiaryEntries(dir, { date: "2026-10-09" }), /无法解析/);
});

await check("写入用临时文件+rename：不残留 .tmp 文件", async () => {
  const dir = tempDir();
  writeDiaryEntry(dir, { title: "原子写", body: "内容", date: "2026-10-10" });
  const { readdirSync } = await import("node:fs");
  const leftovers = readdirSync(dir).filter((name) => name.includes(".tmp-"));
  assert.deepEqual(leftovers, []);
});

rmSync(DIR, { recursive: true, force: true });

console.log(`\n${passed} 项通过，${failures.length} 项失败`);
if (failures.length) {
  for (const failure of failures) console.error(`- ${failure.name}: ${failure.error.stack}`);
  process.exit(1);
}
