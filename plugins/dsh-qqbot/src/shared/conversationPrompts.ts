/**
 * 单会话独立提示词（K26）— 按 (scope, peerId) 追加在场景提示词（directPrompt/groupPrompt）之后的配置项。
 *
 * 本模块是宿主与浏览器设置页共用的**纯函数 + 无 node 依赖**校验/序列化层：
 * 配置 Schema 归一化、POST 白名单校验与设置页 JSON 文本域解析都走同一套规则，
 * 避免两端口径漂移（client bundle 会内联本文件）。
 *
 * 校验规则（fail-fast）：scope ∈ {c2c,group}；peerId trim 后非空；prompt 非空且
 * ≤80000 字符；同一列表内 (scope, peerId) 不得重复（报错带重复键与项序号）。
 */

/**
 * 单条提示词长度上限（字符数）：与 `src/prompt/extra.ts` 的 `EXTRA_MAX_CHARS` 保持一致。
 * （不直接 import 是为了保持本模块零宿主依赖，双方一致性由单测断言兜住。）
 */
export const CONVERSATION_PROMPT_MAX_CHARS = 80_000;

/** 提示词作用域：私聊（c2c，对端为用户 openid）/ 群聊（group，对端为群 openid）。 */
export type ConversationPromptScope = 'c2c' | 'group';

/** 一条单会话提示词配置。 */
export interface ConversationPromptEntry {
  /** 私聊 / 群聊 */
  scope: ConversationPromptScope;
  /** 对端 openid（c2c=用户、group=群），匹配时忽略首尾空白 */
  peerId: string;
  /** 追加在该会话场景提示词之后的文本（1–80000 字符） */
  prompt: string;
}

/** 校验错误：带字段路径与原因（TypeError 便于路由层直接回 400）。 */
function invalid(label: string, index: number | undefined, reason: string): TypeError {
  const where = index === undefined ? label : `${label}[${index}]`;
  // reason 以 '.' 开头时直接拼接（conversationPrompts[0].scope），否则空格分隔（conversationPrompts 必须是数组）。
  const separator = reason.startsWith('.') ? '' : ' ';
  return new TypeError(`${where}${separator}${reason}`);
}

/** 归一化并校验单会话提示词列表（fail-fast；规则见文件头；label 用于 `bots[i]` 等字段路径，返回 trim 后的新数组）。 */
export function normalizeConversationPrompts(input: unknown, label = 'conversationPrompts'): ConversationPromptEntry[] {
  if (input === undefined || input === null) return [];
  if (!Array.isArray(input)) throw invalid(label, undefined, '必须是数组');
  const out: ConversationPromptEntry[] = [];
  const seen = new Map<string, number>();
  input.forEach((raw, index) => {
    if (typeof raw !== 'object' || raw === null || Array.isArray(raw)) {
      throw invalid(label, index, '必须是对象');
    }
    const record = raw as Record<string, unknown>;
    const scope = record.scope;
    if (scope !== 'c2c' && scope !== 'group') {
      throw invalid(label, index, `.scope 必须是 'c2c' 或 'group'（收到 ${JSON.stringify(scope)}）`);
    }
    const peerId = typeof record.peerId === 'string' ? record.peerId.trim() : '';
    if (peerId === '') throw invalid(label, index, '.peerId 不能为空');
    const prompt = typeof record.prompt === 'string' ? record.prompt : '';
    if (prompt.trim() === '') throw invalid(label, index, '.prompt 不能为空');
    if (prompt.length > CONVERSATION_PROMPT_MAX_CHARS) {
      throw invalid(label, index, `.prompt 超长（${prompt.length} > ${CONVERSATION_PROMPT_MAX_CHARS} 字符）`);
    }
    const key = `${scope}:${peerId}`;
    const previous = seen.get(key);
    if (previous !== undefined) {
      throw invalid(label, index, `与第 ${previous + 1} 项重复（(scope,peerId) = ${key}），同一会话只能配置一条`);
    }
    seen.set(key, index);
    out.push({ scope, peerId, prompt });
  });
  return out;
}

/** 取某次会话命中的独立提示词（运行时宽松读取：未命中/空值/非法条目 → undefined，绝不抛错）。 */
export function selectConversationPrompt(
  entries: unknown,
  scope: ConversationPromptScope,
  peerId: string,
): string | undefined {
  if (!Array.isArray(entries) || peerId === '') return undefined;
  const target = peerId.trim();
  for (const entry of entries) {
    if (typeof entry !== 'object' || entry === null) continue;
    const record = entry as Partial<ConversationPromptEntry>;
    if (record.scope !== scope || typeof record.peerId !== 'string' || typeof record.prompt !== 'string') continue;
    if (record.peerId.trim() !== target) continue;
    const prompt = record.prompt.trim();
    return prompt === '' ? undefined : prompt;
  }
  return undefined;
}

/**
 * 把列表序列化为设置页文本域的 JSON 文本。
 * @param entries - 当前生效配置
 * @returns pretty JSON（空列表为 `[]`）
 */
export function formatConversationPromptsJson(entries: ReadonlyArray<ConversationPromptEntry> | undefined): string {
  return JSON.stringify(entries ?? [], null, 2);
}

/** 设置页 JSON 文本解析结果。 */
export type ConversationPromptsParseResult =
  | { ok: true; value: ConversationPromptEntry[] }
  | { ok: false; error: string };

/**
 * 解析设置页文本域 JSON（空文本 = 空列表；语法/语义错误给可读信息，不抛错）。
 * @param text - 文本域内容
 * @returns 解析成功后的合法列表，或可读错误
 */
export function parseConversationPromptsJson(text: string): ConversationPromptsParseResult {
  const trimmed = text.trim();
  if (trimmed === '') return { ok: true, value: [] };
  let parsed: unknown;
  try {
    parsed = JSON.parse(trimmed);
  } catch (err) {
    return { ok: false, error: `JSON 解析失败：${err instanceof Error ? err.message : String(err)}` };
  }
  try {
    return { ok: true, value: normalizeConversationPrompts(parsed) };
  } catch (err) {
    return { ok: false, error: err instanceof Error ? err.message : String(err) };
  }
}
