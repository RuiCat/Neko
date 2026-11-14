/**
 * 会话标题命名（工作区显示用）：主会话 = `主体`，QQ 对端 = `私聊(xxx)` / `群聊(xxx)`。
 *
 * 标题用宿主 `sessionTitle.rename()` 写成 user-owned（pins，停止自动标题生成），
 * 因此不会在首轮之后被标题插件改成「主会话启动报到」之类的随机文案。
 * 用户手动改过的标题（user-owned 且不符合本模块命名）保持不动。
 */

/** 主会话（Neko 主体）的标题。 */
export const MAIN_SESSION_TITLE = '主体'

/** 私聊标题前缀。 */
export const DIRECT_TITLE_PREFIX = '私聊('

/** 群聊标题前缀。 */
export const GROUP_TITLE_PREFIX = '群聊('

/**
 * 取对端标识的短形式（openid 较长，标题里只显示前 8 位；过短原样返回）。
 * @param peerId - QQ openid / group openid
 * @returns 展示用短标识；空入参返回「未知」
 */
export function shortPeerId(peerId: string): string {
  const text = String(peerId ?? '').trim()
  if (text === '') return '未知'
  return text.length <= 8 ? text : text.slice(0, 8)
}

/**
 * 主会话标题。
 * @returns `主体`
 */
export function mainSessionTitle(): string {
  return MAIN_SESSION_TITLE
}

/**
 * QQ 对端会话标题：私聊优先用发送者昵称（群消息才有用户名；私聊只有 openid），
 * 群聊用群 openid 短标识（QQ 群消息事件不携带群名）。
 * @param scope - `c2c` / `group`
 * @param peerId - 对端 openid（群聊为 groupOpenid）
 * @param senderName - 发送者昵称（可选；私聊通常为空）
 * @returns `私聊(xxx)` / `群聊(xxx)`
 */
export function peerSessionTitle(scope: 'c2c' | 'group', peerId: string, senderName?: string): string {
  if (scope === 'group') return `${GROUP_TITLE_PREFIX}${shortPeerId(peerId)})`
  const name = String(senderName ?? '').trim()
  return `${DIRECT_TITLE_PREFIX}${name !== '' ? name : shortPeerId(peerId)})`
}

/**
 * 判断标题是否由本模块命名（用于识别/更新自己的标题）。
 * @param title - 当前标题
 * @returns 属于 主体 / 私聊(...) / 群聊(...) 之一
 */
export function isOwnSessionTitle(title: string): boolean {
  const text = String(title ?? '').trim()
  return text === MAIN_SESSION_TITLE || text.startsWith(DIRECT_TITLE_PREFIX) || text.startsWith(GROUP_TITLE_PREFIX)
}

/** 宿主标题投影的最小形状（title 服务 get() 的返回值）。 */
export interface SessionTitleSnapshotLike {
  readonly title?: string
  readonly source?: { readonly kind?: string }
}

/**
 * 是否需要写入本模块标题：没有标题、空标题、标题来自 fallback/provider，
 * 或标题本来就是本模块命名的（允许随昵称更新）；只有「用户手动改过且不是本模块格式」的标题跳过。
 * @param current - 当前标题快照
 * @returns true = 可以写入
 */
export function shouldApplySessionTitle(current: SessionTitleSnapshotLike | undefined): boolean {
  if (current === undefined) return true
  const title = String(current.title ?? '').trim()
  if (title === '') return true
  if (current.source?.kind === 'user' && !isOwnSessionTitle(title)) return false
  return true
}
