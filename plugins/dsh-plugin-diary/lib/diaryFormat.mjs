/**
 * 日记格式：把一天拆成可机读的条目块，并提供稳定 id 与渲染。
 *
 * 文件形态（人可读、机可解析，字段与 `memory.sh` 的条目协议同风格）：
 *
 *   # 2026-10-25（周六）
 *
 *   ## 14:32 | 标题
 *   - id: D-20261025-1432-ab12cd34
 *   - tags: 插件,日记
 *
 *   正文…
 *
 * 为什么不用 JSON：日记是给人翻的；为什么仍然机读：id/时间/tags 要能用来检索与去重。
 */
import { createHash } from "node:crypto";

const ENTRY_HEADING = /^##\s+(\d{2}:\d{2})\s*\|\s*(.+)$/;
const FIELD_LINE = /^-\s*([A-Za-z][A-Za-z0-9]*)\s*:\s*(.*)$/;
const DATE_FILE = /^(\d{4}-\d{2}-\d{2})\.md$/;

/**
 * 本地日期（YYYY-MM-DD）。
 * @param {Date} [date]
 * @returns {string}
 */
export function formatDate(date = new Date()) {
  const pad = (value) => String(value).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}

/**
 * 本地时间（HH:MM）。
 * @param {Date} [date]
 * @returns {string}
 */
export function formatTime(date = new Date()) {
  const pad = (value) => String(value).padStart(2, "0");
  return `${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

/** 严格校验日期串并返回 Date（跨月/跨年非法值一律拒绝）。 */
export function parseDate(text) {
  if (typeof text !== "string" || !/^\d{4}-\d{2}-\d{2}$/.test(text)) {
    throw new Error(`日期格式必须是 YYYY-MM-DD：${text}`);
  }
  const [year, month, day] = text.split("-").map(Number);
  const date = new Date(year, month - 1, day);
  if (date.getFullYear() !== year || date.getMonth() !== month - 1 || date.getDate() !== day) {
    throw new Error(`不是合法日期：${text}`);
  }
  return date;
}

/** 文件名里的日期（不合规返回 null，便于跳过杂项文件）。 */
export function dateFromFileName(name) {
  const match = DATE_FILE.exec(name);
  return match ? match[1] : null;
}

/**
 * 条目 id：日期 + 时间 + 内容指纹前 8 位（同一分钟内不同内容也不会撞）。
 * @param {{date:string, time:string, title:string, body:string}} entry
 * @returns {string}
 */
export function entryId(entry) {
  const digest = createHash("sha256")
    .update(`${entry.date}\n${entry.time}\n${entry.title}\n${entry.body}`)
    .digest("hex")
    .slice(0, 8);
  return `D-${entry.date.replace(/-/g, "")}-${entry.time.replace(":", "")}-${digest}`;
}

/**
 * 渲染一条条目块（末尾带空行，便于追加）。
 * @param {{id:string, time:string, title:string, tags:string[], body:string}} entry
 * @returns {string}
 */
export function renderEntry(entry) {
  const lines = [
    `## ${entry.time} | ${entry.title}`,
    `- id: ${entry.id}`,
    `- tags: ${entry.tags.join(",") || "-"}`,
    "",
    entry.body,
    "",
  ];
  return lines.join("\n");
}

/**
 * 解析一天的文件全文为条目数组。
 *
 * 坏文件**必须硬报错**（上游 journal-calendar 的教训：把读不动的文件当空对象重建，
 * 等于静默毁掉那天的记录）。判据：文件有内容、却解析不出任何条目。
 * @param {string} text
 * @param {string} date
 * @returns {Array<{date:string,time:string,title:string,id:string,tags:string[],body:string}>}
 */
export function parseDay(text, date) {
  const entries = [];
  let current = null;
  const bodyLines = [];
  const flush = () => {
    if (!current) return;
    current.body = bodyLines.join("\n").trim();
    entries.push(current);
    bodyLines.length = 0;
  };
  for (const line of String(text || "").split(/\r?\n/)) {
    const heading = ENTRY_HEADING.exec(line);
    if (heading) {
      flush();
      current = { date, time: heading[1], title: heading[2].trim(), id: "", tags: [] };
      continue;
    }
    if (!current) continue;
    const field = FIELD_LINE.exec(line);
    if (field) {
      const key = field[1].toLowerCase();
      const value = field[2].trim();
      if (key === "id") current.id = value;
      else if (key === "tags") current.tags = value === "-" ? [] : value.split(/[,，、]/).map((tag) => tag.trim()).filter(Boolean);
      continue;
    }
    bodyLines.push(line);
  }
  flush();
  const source = String(text || "");
  if (source.trim() && entries.length === 0) {
    throw new Error(`日记文件内容无法解析为条目（不会重建，请人工检查）：${date}`);
  }
  return entries.map((entry) => ({ ...entry, id: entry.id || entryId(entry) }));
}

/**
 * 关键词命中打分：标题 2 分、正文 1 分、tags 2 分（大小写不敏感）。
 * @param {object} entry
 * @param {string[]} words
 * @returns {number}
 */
export function scoreEntry(entry, words) {
  const title = entry.title.toLowerCase();
  const body = entry.body.toLowerCase();
  const tags = entry.tags.join(" ").toLowerCase();
  let score = 0;
  for (const word of words) {
    const needle = word.toLowerCase();
    if (!needle) continue;
    if (title.includes(needle)) score += 2;
    if (tags.includes(needle)) score += 2;
    else if (body.includes(needle)) score += 1;
  }
  return score;
}
