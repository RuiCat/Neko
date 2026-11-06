/**
 * 日记工具面：`diary_write` / `diary_read` / `diary_list` 三个模型可调用工具。
 *
 * 工具只做"读写项目内的日记文件"，不做联网、不调模型；目录在**调用时**按会话 cwd 解析，
 * 因此一个 dsh 进程里换项目照样落到各自项目的 `.agent-memory/diary/`。
 */
import { ensureDiaryDir, cwdOf } from "./diaryDir.mjs";
import { makeTool } from "./toolDef.mjs";
import { readDiaryEntries, renderEntries, summarizeDays, writeDiaryEntry } from "./diaryStore.mjs";

const WRITE_PARAMETERS = {
  type: "object",
  properties: {
    title: { type: "string", description: "这一天这件事的一句话标题（必填）" },
    body: { type: "string", description: "正文：做了什么、结论、待办（必填）" },
    tags: {
      type: "array",
      items: { type: "string" },
      description: "可选标签，便于日后检索（例如 插件,调试,发布）",
    },
    date: { type: "string", description: "可选日期 YYYY-MM-DD，默认今天" },
  },
  required: ["title", "body"],
  additionalProperties: false,
};

const READ_PARAMETERS = {
  type: "object",
  properties: {
    date: { type: "string", description: "只看某一天 YYYY-MM-DD" },
    from: { type: "string", description: "起始日期 YYYY-MM-DD（含）" },
    to: { type: "string", description: "结束日期 YYYY-MM-DD（含）" },
    query: { type: "string", description: "关键词，空格或逗号分隔；命中标题/标签/正文" },
    limit: { type: "integer", description: "最多返回多少条，默认 20" },
  },
  required: [],
  additionalProperties: false,
};

const LIST_PARAMETERS = {
  type: "object",
  properties: {
    limit: { type: "integer", description: "最多列多少天，默认 14" },
  },
  required: [],
  additionalProperties: false,
};

/** 写一条日记。 */
function buildWriteTool() {
  return makeTool({
    name: "diary_write",
    description:
      "把一件事写进项目日记（按天一个 markdown 文件，落在项目 .agent-memory/diary/）。" +
      "同一标题+正文重复写会被幂等跳过。适合记录完成的工作、决策与待办。",
    parameters: WRITE_PARAMETERS,
    run(args, exec) {
      const dir = ensureDiaryDir(cwdOf(exec));
      const result = writeDiaryEntry(dir, args);
      const action = result.status === "written" ? "已写入" : "已存在（幂等跳过）";
      return `${action} ${result.date} 的日记：${result.id}\n文件：${result.path}`;
    },
  });
}

/** 按日期/关键词读日记。 */
function buildReadTool() {
  return makeTool({
    name: "diary_read",
    description: "按日期范围或关键词检索项目日记，返回命中条目的标题、时间、标签与正文。",
    parameters: READ_PARAMETERS,
    run(args, exec) {
      const dir = ensureDiaryDir(cwdOf(exec));
      const result = readDiaryEntries(dir, args);
      if (!result.entries.length) return "（没有匹配的日记；可以先用 diary_list 看有哪些天写过）";
      const suffix = result.truncated ? "\n\n（结果已截断，可用 limit 或缩小日期范围）" : "";
      return `命中 ${result.entries.length} 条（扫描 ${result.days} 天）：\n\n${renderEntries(result.entries)}${suffix}`;
    },
  });
}

/** 列出最近写过的日期。 */
function buildListTool() {
  return makeTool({
    name: "diary_list",
    description: "列出项目日记最近写过哪些天、每天几条、标题是什么。",
    parameters: LIST_PARAMETERS,
    run(args, exec) {
      const dir = ensureDiaryDir(cwdOf(exec));
      const days = summarizeDays(dir, Number.isFinite(args.limit) ? args.limit : 14);
      if (!days.length) return `日记目录还是空的：${dir}`;
      const lines = [`日记目录：${dir}`, ""];
      for (const day of days) {
        lines.push(`${day.date}（${day.count} 条）：${day.titles.join("；")}`);
      }
      return lines.join("\n");
    },
  });
}

/**
 * 构造全部日记工具。
 * @returns {object[]} 工具定义数组
 */
export function buildDiaryTools() {
  return [buildWriteTool(), buildReadTool(), buildListTool()];
}
