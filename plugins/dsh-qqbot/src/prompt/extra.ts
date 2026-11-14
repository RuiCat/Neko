/**
 * 补充提示词（按场景分离 + 单会话追加）。
 *
 * - group/direct：场景级补充词（groupPrompt/directPrompt）；
 * - conversation：命中当前会话 (scope, peerId) 的独立提示词（K26），追加在场景词之后。
 * 由调用方每步现读 config，改配置即时生效。
 */
export const EXTRA_SECTION_NAME = 'qqbot:extra';
export const EXTRA_ORDER = 460;

export const EXTRA_MAX_CHARS = 80_000;

export interface ScopePrompts {
  group?: string;
  direct?: string;
  /** K26：命中当前会话 scope+peerId 的独立提示词（追加在 group/direct 之后，空行分隔） */
  conversation?: string;
}

/**
 * 取当次场景的补充提示词：场景词在前，命中的会话级提示词以空行分隔追加在后。
 * 未命中/空值时行为与旧版一致（只返回场景词）；长度已在配置写入时校验，注入时保留完整内容。
 * @param prompts - 场景词 + 会话级提示词
 * @param scope - 当前会话场景（group / c2c）
 * @returns 注入 system prompt 的完整文本（两端空白已清理）
 */
export function selectExtraPrompt(prompts: ScopePrompts, scope: import('./scope.ts').QQScope): string {
  const scenario = ((scope === 'group' ? prompts.group : prompts.direct) ?? '').trim();
  const conversation = (prompts.conversation ?? '').trim();
  if (conversation === '') return scenario;
  // K26：追加而非替换；空行分隔让模型能区分场景词与会话专属指令。
  return scenario === '' ? conversation : `${scenario}\n\n${conversation}`;
}
