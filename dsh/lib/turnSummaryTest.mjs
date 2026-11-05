/**
 * 轮次摘要与 turn-end 携带摘要的测试：只读事件窗口、跨轮过滤、截断、异常回退。
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { DIGEST_MAX_CHARS, summarizeTurn } from './turnSummary.mjs'
import { createSessionEventSender } from './sessionEvents.mjs'

const PREFIX = '/repo/neko'
const CONFIG = Object.freeze({ workspacePrefix: PREFIX, functionName: 'dsh_on_event' })

/** 造一个只提供 snapshotEvents 的假 Session。 */
function sessionWith(events) {
  return { id: 's1', header: { cwd: PREFIX }, snapshotEvents: () => events }
}

/** 造一条 assistant/message 事件。 */
function assistantEvent(turn, text) {
  return { type: 'assistant/message', data: { turn, step: 1, message: { content: [{ type: 'text', text }] } } }
}

/** 造一条 user/message 事件（真实契约：payload 就是消息本身，没有 turn 字段）。 */
function userEvent(text) {
  return { type: 'user/message', data: { content: [{ type: 'text', text }] } }
}

test('summarizeTurn 取本轮最后一条助手文本，忽略其他轮', () => {
  const events = [
    { type: 'turn/start', data: { turn: 1 } },
    assistantEvent(1, '上一轮'),
    { type: 'turn/start', data: { turn: 2 } },
    userEvent('用户的问题'),
    assistantEvent(2, '第一句'),
    assistantEvent(2, '最后一句'),
    { type: 'step/start', data: { turn: 2, step: 1 } },
    { type: 'tool/call', data: { turn: 2, step: 1 } },
  ]
  const summary = summarizeTurn(sessionWith(events), 2)
  assert.equal(summary.assistantDigest, '最后一句')
  assert.equal(summary.userDigest, '用户的问题')
  assert.equal(summary.toolCalls, 1)
  assert.equal(summary.steps, 1)
})

test('summarizeTurn 只取本轮 user/message（真实 payload 无 turn 字段）', () => {
  const events = [
    { type: 'turn/start', data: { turn: 1 } },
    userEvent('上一轮的问题'),
    assistantEvent(1, '上一轮回答'),
    { type: 'turn/start', data: { turn: 2 } },
    userEvent('本轮的问题'),
    assistantEvent(2, '本轮回答'),
  ]
  const summary = summarizeTurn(sessionWith(events), 2)
  assert.equal(summary.userDigest, '本轮的问题')
  assert.equal(summary.assistantDigest, '本轮回答')
})

test('summarizeTurn 无可用事件时返回 undefined', () => {
  assert.equal(summarizeTurn(undefined, 1), undefined)
  assert.equal(summarizeTurn({ id: 's1' }, 1), undefined)
  assert.equal(summarizeTurn(sessionWith([]), 1), undefined)
  const throwing = { id: 's1', snapshotEvents: () => { throw new Error('boom') } }
  assert.equal(summarizeTurn(throwing, 1), undefined)
})

test('summarizeTurn 对超长文本截断到上限', () => {
  const long = '猫'.repeat(DIGEST_MAX_CHARS + 10)
  const summary = summarizeTurn(sessionWith([{ type: 'turn/start', data: { turn: 1 } }, assistantEvent(1, long)]), 1)
  assert.equal(Array.from(summary.assistantDigest).length, DIGEST_MAX_CHARS + 1)
  assert.ok(summary.assistantDigest.endsWith('…'))
})

test('turn-end 事件携带摘要，且保持旧字段', async () => {
  const calls = []
  const client = { postSessionEvent: (event) => { calls.push(event); return Promise.resolve({}) } }
  const sender = createSessionEventSender({
    client,
    config: CONFIG,
    logger: { warnOnce() {} },
  })
  sender.turnStopping({
    turn: 3,
    agent: { session: sessionWith([
      { type: 'turn/start', data: { turn: 3 } },
      assistantEvent(3, '本喵知道了'),
      { type: 'tool/call', data: { turn: 3, step: 1 } },
    ]) },
  })
  assert.equal(calls.length, 1)
  assert.equal(calls[0].kind, 'turn-end')
  assert.equal(calls[0].turn, 3)
  assert.deepEqual(calls[0].summary, { assistantDigest: '本喵知道了', toolCalls: 1, steps: 0 })
})

test('没有 snapshotEvents 时 turn-end 不带 summary（向后兼容）', () => {
  const calls = []
  const client = { postSessionEvent: (event) => { calls.push(event); return Promise.resolve({}) } }
  const sender = createSessionEventSender({ client, config: CONFIG, logger: { warnOnce() {} } })
  sender.turnStopping({ turn: 1, agent: { session: { id: 's1', header: { cwd: PREFIX } } } })
  assert.equal(calls.length, 1)
  assert.equal('summary' in calls[0], false)
})
