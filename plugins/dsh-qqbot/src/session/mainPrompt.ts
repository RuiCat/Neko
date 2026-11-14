/**
 * 主会话提示词段（K28）：把桥接侧「按实体的主会话提示词」注册成机器人主会话（主体）的
 * system prompt 段。
 *
 * 与 QQ 通道提示词（`prompt/install.ts`）分工：主会话没有 QQ 回复目标，qqbot 不为它安装
 * rules/scope/persona/extra；这里只补一段可按实体编辑的补充提示词，内容来自 astrbox 桥接
 * （`astrboxBridge.mainSessionPrompt(entityId)`），面板保存后下一轮生效。
 *
 * 惰性读取：`section.text` 每步组装时现读 provider；清空提示词后返回空串，宿主丢弃空段，
 * 无需重装。`interpolate: false`：提示词是用户可编辑文本，不得把 `{{…}}` 当变量插值。
 */
import type { Context } from '@deepseek-ai/cordis';
// 空导入：仅触发 dsh-agent 的类型声明副作用（与 prompt/install.ts 同口径）。
import type {} from '@deepseek-ai/dsh-agent';
import type { Logger } from '../types.ts';

/** 主会话提示词段名（与 QQ 通道段不冲突）。 */
export const MAIN_PROMPT_SECTION_NAME = 'qqbot:main';

/** 段顺序：排在 persona(440)/extra(460) 之后、宿主动态上下文之前。 */
export const MAIN_PROMPT_ORDER = 470;

/** 已安装的主会话提示词句柄。 */
export interface MainPromptHandle {
  /** 注销段（幂等；fiber 已释放时静默）。 */
  dispose(): void;
}

/**
 * 在主会话 agent 的 ctx 上安装主会话提示词段。
 *
 * 失败降级：宿主缺 `systemPrompt` 服务或注册抛错时返回 `undefined`（只 warn），
 * 主会话照常收发消息。
 * @param agentCtx - 主会话 agent 的 cordis 上下文
 * @param readText - 同步读取当前提示词（空串 = 不注入）；每步组装都会调用
 * @param logger - 可选日志（缺省静默）
 * @returns 句柄；未安装时 `undefined`
 */
export function installMainSessionPrompt(
  agentCtx: Context,
  readText: () => string,
  logger?: Logger,
): MainPromptHandle | undefined {
  const service = agentCtx.get?.('systemPrompt') as
    | { section?: (spec: unknown) => () => void }
    | undefined;
  if (service === undefined || typeof service.section !== 'function') return undefined;
  try {
    const spec = {
      name: MAIN_PROMPT_SECTION_NAME,
      order: MAIN_PROMPT_ORDER,
      interpolate: false,
      text: () => {
        try {
          const text = readText();
          return typeof text === 'string' ? text : '';
        } catch {
          return '';
        }
      },
    };
    const dispose = service.section(spec);
    return {
      dispose() {
        try {
          if (typeof dispose === 'function') dispose();
        } catch {
          // 段可能已随 agent fiber 释放：幂等忽略。
        }
      },
    };
  } catch (err) {
    logger?.warn(`main session prompt install failed (session keeps working): ${err instanceof Error ? err.message : String(err)}`);
    return undefined;
  }
}
