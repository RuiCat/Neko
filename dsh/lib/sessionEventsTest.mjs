/**
 * C2：cwd 目录边界过滤、同 (kind, sessionId, turn) 去重、去重表 256 上限、
 * 监听器绝不向宿主同步抛错（含 GETTER 抛错、客户端同步抛错、异步拒答）。
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { DEDUPE_LIMIT, createSessionEventSender, matchesWorkspace } from './sessionEvents.mjs'
import { nextMacroTask } from './testHarness.mjs'

const PREFIX = '/repo/neko'

/** 默认配置：functionName 使用 K2 默认值，随 K1 请求体的 `function` 字段发出。 */
const CONFIG = Object.freeze({ workspacePrefix: PREFIX, functionName: 'dsh_on_event' })

/** 记录事件的假客户端。 */
function fakeClient() {
  return {
    calls: [],
    postSessionEvent(event) {
      this.calls.push(event)
      return Promise.resolve({ ok: true, applied: 'logged' })
    },
  }
}

/** 假 logger：只统计 warnOnce。 */
function fakeLogger() {
  return {
    warnings: [],
    warnOnce(key, message) {
      this.warnings.push([key, message])
    },
  }
}

test('matchesWorkspace 按目录边界匹配', () => {
  assert.equal(matchesWorkspace(undefined, PREFIX), false)
  assert.equal(matchesWorkspace(null, PREFIX), false)
  assert.equal(matchesWorkspace('', PREFIX), false)
  assert.equal(matchesWorkspace(PREFIX, PREFIX), true)
  assert.equal(matchesWorkspace(`${PREFIX}/sub`, PREFIX), true)
  assert.equal(matchesWorkspace(`${PREFIX}/`, PREFIX), true)
  assert.equal(matchesWorkspace(`${PREFIX}-old`, PREFIX), false)
  assert.equal(matchesWorkspace(`${PREFIX}extra`, PREFIX), false)
  assert.equal(matchesWorkspace(`${PREFIX}/sub`, `${PREFIX}/`), true)
  assert.equal(matchesWorkspace(`/repo2/neko/sub`, '/repo'), false)
})

test('session/created：cwd undefined 不发请求也不抛错', async () => {
  const client = fakeClient()
  const sender = createSessionEventSender({ client, config: CONFIG, logger: fakeLogger() })
  assert.doesNotThrow(() => sender.sessionCreated({ id: 's1', header: {} }))
  await nextMacroTask()
  assert.equal(client.calls.length, 0)
})

test('cwd = <prefix>-old 不命中', async () => {
  const client = fakeClient()
  const sender = createSessionEventSender({ client, config: CONFIG, logger: fakeLogger() })
  sender.sessionCreated({ id: 's1', header: { cwd: `${PREFIX}-old` } })
  await nextMacroTask()
  assert.equal(client.calls.length, 0)
})

test('重复 session-created 只发一次', async () => {
  const client = fakeClient()
  const sender = createSessionEventSender({ client, config: CONFIG, logger: fakeLogger() })
  const session = { id: 's1', header: { cwd: `${PREFIX}/sub` } }
  sender.sessionCreated(session)
  sender.sessionCreated(session)
  sender.sessionCreated(session)
  await nextMacroTask()
  assert.equal(client.calls.length, 1)
  assert.deepEqual(client.calls[0], {
    kind: 'session-created',
    sessionId: 's1',
    cwd: `${PREFIX}/sub`,
    function: 'dsh_on_event',
  })
})

test('请求体携带配置的 function 字段（K1 / D6）', async () => {
  const client = fakeClient()
  const sender = createSessionEventSender({
    client,
    config: { ...CONFIG, functionName: 'custom_on_event' },
    logger: fakeLogger(),
  })
  sender.sessionCreated({ id: 's1', header: { cwd: PREFIX } })
  sender.turnStopping({ agent: { session: { id: 's1', header: { cwd: PREFIX } } }, turn: 1 })
  await nextMacroTask()
  assert.deepEqual(
    client.calls.map((call) => call.function),
    ['custom_on_event', 'custom_on_event'],
  )
})

test('同一 turn 的 turn-end 只发一次，不同 turn 各发一次', async () => {
  const client = fakeClient()
  const sender = createSessionEventSender({ client, config: CONFIG, logger: fakeLogger() })
  const agent = { session: { id: 's1', header: { cwd: PREFIX } } }
  sender.turnStopping({ agent, turn: 1 })
  sender.turnStopping({ agent, turn: 1 })
  sender.turnStopping({ agent, turn: 1 })
  sender.turnStopping({ agent, turn: 2 })
  await nextMacroTask()
  assert.deepEqual(
    client.calls.map((call) => call.turn),
    [1, 2],
  )
  // session-created 与 turn-end 的键空间互不影响。
  sender.sessionCreated(agent.session)
  await nextMacroTask()
  assert.equal(client.calls.length, 3)
})

test('turn 非整数不发请求', async () => {
  const client = fakeClient()
  const sender = createSessionEventSender({ client, config: CONFIG, logger: fakeLogger() })
  const agent = { session: { id: 's1', header: { cwd: PREFIX } } }
  sender.turnStopping({ agent, turn: 1.5 })
  sender.turnStopping({ agent })
  await nextMacroTask()
  assert.equal(client.calls.length, 0)
})

test('去重表按会话上限 256', async () => {
  const client = fakeClient()
  const sender = createSessionEventSender({ client, config: CONFIG, logger: fakeLogger() })
  const agent = { session: { id: 's1', header: { cwd: PREFIX } } }
  for (let turn = 1; turn <= 300; turn += 1) sender.turnStopping({ agent, turn })
  await nextMacroTask()
  assert.equal(DEDUPE_LIMIT, 256)
  assert.equal(sender.dedupeSize('s1'), 256)
  // 最早的 turn 1 已被淘汰，重新发送会再次上报。
  sender.turnStopping({ agent, turn: 1 })
  await nextMacroTask()
  assert.equal(client.calls.at(-1).turn, 1)
})

test('客户端同步抛错被吞掉，只记一条警告', async () => {
  const logger = fakeLogger()
  const client = {
    postSessionEvent() {
      throw new Error('sync boom')
    },
  }
  const sender = createSessionEventSender({ client, config: CONFIG, logger })
  assert.doesNotThrow(() => sender.sessionCreated({ id: 's1', header: { cwd: PREFIX } }))
  assert.equal(logger.warnings.length, 1)
})

test('客户端异步拒答不产生未处理拒绝', async () => {
  const logger = fakeLogger()
  const client = {
    postSessionEvent() {
      return Promise.reject(new Error('async boom'))
    },
  }
  const sender = createSessionEventSender({ client, config: CONFIG, logger })
  sender.sessionCreated({ id: 's1', header: { cwd: PREFIX } })
  await nextMacroTask()
  assert.equal(logger.warnings.length, 1)
})

test('session.header 的 GETTER 抛错也不向宿主同步抛错', () => {
  const logger = fakeLogger()
  const sender = createSessionEventSender({ client: fakeClient(), config: CONFIG, logger })
  const evil = {
    id: 's-evil',
    get header() {
      throw new Error('getter boom')
    },
  }
  assert.doesNotThrow(() => sender.sessionCreated(evil))
  assert.equal(logger.warnings.length, 1)
})

test('dispose 后不再发送', async () => {
  const client = fakeClient()
  const sender = createSessionEventSender({ client, config: CONFIG, logger: fakeLogger() })
  sender.sessionCreated({ id: 's1', header: { cwd: PREFIX } })
  sender.dispose()
  sender.sessionCreated({ id: 's2', header: { cwd: PREFIX } })
  await nextMacroTask()
  assert.equal(client.calls.length, 1)
})
