/**
 * 会话文本工具 — 错误文案与消息纯文本提取（供会话导出/统计复用）。
 */
import type { SessionEventLike } from './types.ts';

/**
 * 错误文案（日志用）：Error 取 message，其余 String()。
 * @param err - 任意抛出值
 * @returns 可读文本
 */
export function errorText(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

/**
 * 从消息对象中提取纯文本（用于导出/统计）。
 * @param message - 会话消息（content 为 ContentBlock 数组）
 * @returns 拼合的纯文本；无文本块时为空串
 */
export function extractMessageText(
  message: SessionEventLike['message'],
): string {
  const blocks = message?.content;
  if (!blocks || !Array.isArray(blocks)) return '';

  const parts: string[] = [];
  for (const block of blocks) {
    if (block.type === 'text' && block.text) {
      parts.push(block.text);
    }
  }
  return parts.join('\n').trim();
}
