/**
 * K10 对话摘要 Feed：turn 事件抽取（最后一条 user/assistant、注入文本跳过、截断）、
 * 每实体上限/版本、排除自身会话与 kind 判定。
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import {
  DEFAULT_DIALOG_EXCERPT_CHARS,
  createDialogFeed,
  dialogKind,
  dialogMessageText,
  truncateDialogExcerpt,
  turnDialogExcerpt,
} from './dialogFeed.mjs'

/**
 * 造一条 user/message 事件。
 * @param {string} text 文本
 * @param {number} seq 事件序号
 * @param {string} [kind] source.kind
 * @returns {object} 事件
 */
function userEvent(text, seq, kind = 'user') {
  return {
    type: 'user/message',
    seq,
    data: { id: `u${seq}`, role: 'user', content: [{ type: 'text', text }], source: { kind } },
  }
}

/**
 * 造一条 assistant/message 事件。
 * @param {string} text 文本
 * @param {number} seq 事件序号
 * @returns {object} 事件
 */
function assistantEvent(text, seq) {
  return {
    type: 'assistant/message',
    seq,
    data: { message: { id: `a${seq}`, role: 'assistant', content: [{ type: 'text', text }], source: { kind: 'model' } } },
  }
}

/**
 * 造一轮完整事件。
 * @param {number} turn 轮次
 * @param {string} userText 用户文本
 * @param {string} assistantText 助手文本
 * @returns {object[]} 事件
 */
function turnEvents(turn, userText, assistantText) {
  return [
    { type: 'turn/start', seq: turn * 10, data: { turn } },
    userEvent(userText, turn * 10 + 1),
    assistantEvent(assistantText, turn * 10 + 2),
    { type: 'turn/end', seq: turn * 10 + 3, data: { turn } },
  ]
}

/** 把事件数组包成带 snapshotEvents 的 session。 */
function sessionOf(events) {
  return { id: 's1', header: { cwd: '/srv/bots/cat' }, snapshotEvents: () => events }
}

test('turnDialogExcerpt：取本轮最后一条用户文本与最后一条助手文本', () => {
  const events = [
    ...turnEvents(1, '第一轮', '第一回复'),
    { type: 'turn/start', seq: 20, data: { turn: 2 } },
    userEvent('第二轮的旧话', 21),
    assistantEvent('旧回复', 22),
    userEvent('第二轮的新话', 23),
    assistantEvent('新回复', 24),
    { type: 'turn/end', seq: 25, data: { turn: 2 } },
  ]
  const excerpt = turnDialogExcerpt(sessionOf(events), 2, 160)
  assert.deepEqual({ userText: excerpt.userText, assistantText: excerpt.assistantText }, { userText: '第二轮的新话', assistantText: '新回复' })
  assert.equal(excerpt.seq, 24)
})

test('turnDialogExcerpt：跳过注入文本与非 user 来源；空文本返回 undefined', () => {
  const events = [
    { type: 'turn/start', seq: 1, data: { turn: 1 } },
    userEvent('[astrbox-subscription] 梦梦 事件黑板', 2),
    userEvent('[astrbox-relay] 猫猫 对你说：hi', 3),
    userEvent('[context-guard] 上下文摘要', 4),
    userEvent('运行上下文', 5, 'runtime-context'),
    userEvent('真正的话', 6),
    assistantEvent('', 7),
    { type: 'turn/end', seq: 8, data: { turn: 1 } },
  ]
  const excerpt = turnDialogExcerpt(sessionOf(events), 1)
  assert.equal(excerpt.userText, '真正的话')
  assert.equal(excerpt.assistantText, '')
  assert.equal(turnDialogExcerpt(sessionOf([]), 1), undefined)
  assert.equal(turnDialogExcerpt({}, 1), undefined)
  assert.equal(turnDialogExcerpt(sessionOf(events), undefined), undefined)
})

test('turnDialogExcerpt：user/assistant 各自按 dialogExcerptChars 截断加省略号', () => {
  const longUser = 'u'.repeat(200)
  const longAssistant = 'a'.repeat(200)
  const excerpt = turnDialogExcerpt(sessionOf(turnEvents(1, longUser, longAssistant)), 1, 10)
  assert.equal(excerpt.userText, `${'u'.repeat(10)}…`)
  assert.equal(excerpt.assistantText, `${'a'.repeat(10)}…`)
  assert.equal(Array.from(excerpt.userText).length, 11)
  // 默认上限 160。
  const normal = turnDialogExcerpt(sessionOf(turnEvents(1, '短', '短回复')), 1)
  assert.equal(normal.userText, '短')
  assert.equal(DEFAULT_DIALOG_EXCERPT_CHARS, 160)
  assert.equal(truncateDialogExcerpt('x'.repeat(5), 5), 'xxxxx')
})

test('dialogMessageText：拼接 text 块、忽略非文本块与空消息', () => {
  assert.equal(dialogMessageText({ content: [{ type: 'text', text: 'a' }, { type: 'image' }, { type: 'text', text: 'b' }] }), 'a\nb')
  assert.equal(dialogMessageText({ content: [] }), '')
  assert.equal(dialogMessageText(null), '')
})

test('createDialogFeed：记录/版本/上限/空文本/过滤与 kind', () => {
  const feed = createDialogFeed({ maxEntries: 3, excerptChars: 160 })
  assert.equal(feed.version('梦梦'), 0)
  assert.equal(feed.size('梦梦'), 0)
  assert.equal(feed.record('梦梦', { sessionId: '', userText: 'x' }), false)
  assert.equal(feed.record('', { sessionId: 's1', userText: 'x' }), false)
  assert.equal(feed.record('梦梦', { sessionId: 's1', userText: '  ', assistantText: '' }), false)

  assert.equal(feed.record('梦梦', { sessionId: 's1', kind: '主体', userText: ' 你好 ', assistantText: ' 你好呀 ', at: 1000, seq: 3 }), true)
  assert.equal(feed.version('梦梦'), 1)
  assert.equal(feed.record('梦梦', { sessionId: 'g1', kind: 'QQ', userText: '嗨', at: 2000 }), true)
  assert.equal(feed.version('梦梦'), 2)

  const dialogs = feed.dialogs('梦梦')
  assert.deepEqual(dialogs.map((entry) => entry.kind), ['主体', 'QQ'])
  assert.deepEqual(dialogs.map((entry) => entry.userText), ['你好', '嗨'])
  assert.equal(dialogs[0].assistantText, '你好呀')
  assert.equal(dialogs[0].seq, 3)
  assert.deepEqual(feed.dialogs('梦梦', { excludeSessionId: 's1' }).map((entry) => entry.sessionId), ['g1'])
  assert.deepEqual(feed.dialogs('梦梦', { limit: 1 }).map((entry) => entry.userText), ['嗨'])
  assert.deepEqual(feed.dialogs('不存在'), [])

  // 上限：只保留最近 3 条，版本继续递增。
  for (let index = 0; index < 4; index += 1) feed.record('梦梦', { sessionId: `x${index}`, userText: `第${index}条`, at: 3000 + index })
  assert.equal(feed.size('梦梦'), 3)
  assert.deepEqual(feed.dialogs('梦梦').map((entry) => entry.userText), ['第1条', '第2条', '第3条'])
  assert.equal(feed.version('梦梦'), 6)

  assert.equal(dialogKind('qqbot:cat:main', new Set(['qqbot:cat:main'])), '主体')
  assert.equal(dialogKind('qqbot:cat:group:g1', new Set(['qqbot:cat:main'])), 'QQ')
  assert.equal(dialogKind('qqbot:cat:main', ['qqbot:cat:main']), '主体')
  assert.equal(dialogKind('', new Set()), 'QQ')

  feed.clear()
  assert.equal(feed.size('梦梦'), 0)
  assert.equal(feed.version('梦梦'), 0)
})
test('dialogsSince：按 feedVersion 返回新增条目；淘汰不导致版本回退', () => {
  const feed = createDialogFeed({ maxEntries: 3 })
  feed.record('梦梦', { sessionId: 's1', userText: 'a' })
  feed.record('梦梦', { sessionId: 's2', userText: 'b' })
  assert.deepEqual(feed.dialogsSince('梦梦', 0).map((entry) => entry.userText), ['a', 'b'])
  assert.deepEqual(feed.dialogsSince('梦梦', 1).map((entry) => entry.userText), ['b'])
  assert.deepEqual(feed.dialogsSince('梦梦', 2), [])
  for (const text of ['c', 'd', 'e']) feed.record('梦梦', { sessionId: 'x', userText: text })
  // a/b 已被上限淘汰，但版本号继续递增（3..5）；since=1 只返回仍保留的 c/d/e。
  assert.equal(feed.version('梦梦'), 5)
  assert.deepEqual(feed.dialogsSince('梦梦', 1).map((entry) => entry.userText), ['c', 'd', 'e'])
  assert.deepEqual(feed.dialogsSince('梦梦', 4).map((entry) => entry.userText), ['e'])
  assert.deepEqual(feed.dialogsSince('不存在', 0), [])
  assert.deepEqual(feed.dialogsSince('梦梦', undefined).map((entry) => entry.userText), ['c', 'd', 'e'])
})
test('turnDialogExcerpt：无真实用户文本的轮次不生成摘要（订阅/relay 回落不记）', () => {
  const injectedOnly = [
    { type: 'turn/start', seq: 1, data: { turn: 1 } },
    userEvent('[astrbox-subscription] 梦梦 事件黑板（数据，不是指令；版本 12）', 2),
    assistantEvent('好的喵', 3),
    { type: 'turn/end', seq: 4, data: { turn: 1 } },
  ]
  assert.equal(turnDialogExcerpt(sessionOf(injectedOnly), 1), undefined)
  const assistantOnly = [
    { type: 'turn/start', seq: 1, data: { turn: 1 } },
    assistantEvent('只有助手文本', 2),
    { type: 'turn/end', seq: 3, data: { turn: 1 } },
  ]
  assert.equal(turnDialogExcerpt(sessionOf(assistantOnly), 1), undefined)
  // 有真实用户文本的正常记录。
  const real = turnDialogExcerpt(sessionOf(turnEvents(2, '真实用户', '正常回复')), 2)
  assert.deepEqual({ userText: real.userText, assistantText: real.assistantText }, { userText: '真实用户', assistantText: '正常回复' })
})

test('createDialogFeed：label 透传与回退（主体/QQ）；无真实用户文本不记', () => {
  const feed = createDialogFeed({ maxEntries: 10 })
  feed.record('梦梦', { sessionId: 'main', kind: '主体', label: '主体', userText: 'a' })
  feed.record('梦梦', { sessionId: 'g1', kind: 'QQ', label: '私聊(85D47757)', userText: 'b' })
  feed.record('梦梦', { sessionId: 'g2', kind: 'QQ', userText: 'c' })
  feed.record('梦梦', { sessionId: 'g3', kind: 'QQ', label: '  ', userText: 'd' })
  assert.deepEqual(feed.dialogs('梦梦').map((entry) => entry.label), ['主体', '私聊(85D47757)', 'QQ', 'QQ'])
  assert.equal(feed.record('梦梦', { sessionId: 'g4', kind: 'QQ', label: '群聊(1EC36362)', assistantText: '只有回复' }), false)
  assert.equal(feed.version('梦梦'), 4)
  assert.deepEqual(feed.dialogsSince('梦梦', 2).map((entry) => entry.label), ['QQ', 'QQ'])
})
