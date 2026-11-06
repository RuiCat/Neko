/**
 * 日记存储：按天一个 markdown 文件，写入幂等（同一天同标题同正文只写一次）。
 *
 * 只做本地文件读写：不联网、不调模型、不改 INDEX。目录由 `diaryDir.mjs` 决定，
 * 本模块不自己猜路径——这样测试可以用临时目录直接构造。
 */
import { existsSync, mkdirSync, readdirSync, readFileSync, renameSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import {
  dateFromFileName,
  entryId,
  formatDate,
  formatTime,
  parseDate,
  parseDay,
  renderEntry,
  scoreEntry,
} from "./diaryFormat.mjs";

/** 一天文件的绝对路径。 */
export function dayFilePath(dir, date) {
  return join(dir, `${date}.md`);
}

/**
 * 写一条日记（幂等）。
 * @param {string} dir 日记目录（不存在则创建）
 * @param {{title:string, body:string, tags?:string[], date?:string, time?:string}} input
 * @returns {{status:"written"|"skipped", id:string, date:string, path:string}}
 */
export function writeDiaryEntry(dir, input) {
  const title = String(input.title || "").trim();
  const body = String(input.body || "").trim();
  if (!title) throw new Error("title 不能为空");
  if (!body) throw new Error("body 不能为空（日记正文为空就没有记录价值）");
  const date = input.date ? formatDate(parseDate(input.date)) : formatDate();
  const time = /^\d{2}:\d{2}$/.test(input.time || "") ? input.time : formatTime();
  const tags = (input.tags || []).map((tag) => String(tag).trim()).filter(Boolean);
  const draft = { date, time, title, body };
  const id = entryId(draft);

  mkdirSync(dir, { recursive: true });
  const file = dayFilePath(dir, date);
  const existingText = existsSync(file) ? readFileSync(file, "utf8") : "";
  const existing = existingText ? parseDay(existingText, date) : [];
  const duplicate = existing.find((entry) => entry.id === id);
  if (duplicate) return { status: "skipped", id, date, path: file };

  const head = existingText ? "" : `# ${date}（${weekdayOf(date)}）\n\n`;
  const merged = `${existingText}${head}${renderEntry({ ...draft, id, tags })}`;
  const temporary = `${file}.tmp-${process.pid}-${Date.now()}`;
  writeFileSync(temporary, merged, "utf8");
  renameSync(temporary, file);
  return { status: "written", id, date, path: file };
}

/** 中文星期（仅用于标题行）。 */
export function weekdayOf(date) {
  const names = ["周日", "周一", "周二", "周三", "周四", "周五", "周六"];
  return names[parseDate(date).getDay()];
}

/**
 * 列出日记目录里已有的日期（新→旧）。
 * @param {string} dir
 * @returns {string[]}
 */
export function listDates(dir) {
  if (!existsSync(dir)) return [];
  return readdirSync(dir)
    .map(dateFromFileName)
    .filter(Boolean)
    .sort()
    .reverse();
}

/**
 * 读一段日期范围内的条目。
 * @param {string} dir
 * @param {{date?:string, from?:string, to?:string, query?:string, limit?:number}} [options]
 * @returns {{entries:Array<object>, days:number, truncated:boolean}}
 */
export function readDiaryEntries(dir, options = {}) {
  const limit = Number.isFinite(options.limit) && options.limit > 0 ? Math.floor(options.limit) : 20;
  const maxChars = Number.isFinite(options.maxChars) && options.maxChars > 0 ? Math.floor(options.maxChars) : 8000;
  const words = String(options.query || "")
    .split(/[\s,，、]+/)
    .map((word) => word.trim())
    .filter(Boolean);
  let dates = listDates(dir);
  if (options.date) dates = dates.filter((date) => date === formatDate(parseDate(options.date)));
  if (options.from) {
    const from = formatDate(parseDate(options.from));
    dates = dates.filter((date) => date >= from);
  }
  if (options.to) {
    const to = formatDate(parseDate(options.to));
    dates = dates.filter((date) => date <= to);
  }

  const collected = [];
  for (const date of dates) {
    const file = dayFilePath(dir, date);
    const entries = parseDay(readFileSync(file, "utf8"), date);
    for (const entry of entries) {
      const score = words.length ? scoreEntry(entry, words) : 1;
      if (score > 0) collected.push({ ...entry, score });
    }
  }
  if (words.length) collected.sort((left, right) => right.score - left.score || right.date.localeCompare(left.date));
  return {
    entries: collected.slice(0, limit),
    days: dates.length,
    truncated: collected.length > limit,
  };
}

/**
 * 汇总最近若干天：日期 + 条目数 + 当天标题列表。
 * @param {string} dir
 * @param {number} [limit] 最多返回多少天（新→旧）
 * @returns {Array<{date:string, count:number, titles:string[]}>}
 */
export function summarizeDays(dir, limit = 14) {
  const dates = listDates(dir).slice(0, Math.max(1, Math.floor(limit)));
  return dates.map((date) => {
    const entries = parseDay(readFileSync(dayFilePath(dir, date), "utf8"), date);
    return { date, count: entries.length, titles: entries.map((entry) => entry.title) };
  });
}

/**
 * 把条目渲染成一段给模型看的纯文本。
 * @param {Array<object>} entries
 * @returns {string}
 */
export function renderEntries(entries) {
  if (!entries.length) return "（没有匹配的日记）";
  const blocks = entries.map((entry) => {
    const tags = entry.tags.length ? `  #${entry.tags.join(" #")}` : "";
    return `【${entry.date} ${entry.time}】${entry.title}${tags}\n${entry.body}`;
  });
  return blocks.join("\n\n---\n\n");
}
