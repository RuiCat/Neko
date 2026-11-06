/**
 * K14 `astrbox_relay`：工具定义形状、各 target 的投递、QQ 直发、限流、非法文本与失败返回。
 * 全部用替身（bindings / agentOf / qqbot say），不依赖 DSH 宿主或真 qqbot。
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import {
  RELAY_RATE_LIMIT,
  RELAY_TEXT_MAX_CHARS,
  RELAY_TOOL_NAME,
  createRelayTool,
} from './relayTool.mjs'

/** K1 形状的两条绑定（猫猫/梦梦各一个 bot）。 */
const BINDINGS = [
  { botId: 'cat', label: 'cat', cwd: '/srv/bots/cat', entity: '梦梦', mainSessionId: 'qqbot:cat:main' },
  { botId: 'helper', label: 'helper', cwd: '/srv/bots/helper', entity: '猫猫', mainSessionId: 'qqbot:helper:main' },
]

/**
 * 造一个记录 followup 的假 agent。
 * @param {string} sessionId 会话 id
 * @param {object[]} calls 收集数组
 * @returns {object} agent
 */
function fakeAgent(sessionId, calls) {
  return {
    id: sessionId,
    session: { id: sessionId },
    followup(message) {
      calls.push({ sessionId, message })
    },
  }
}

/**
 * 建测试用 relay 与依赖。
 * @param {{bindings?: Array<object>, calls?: object[], say?: Function, qqbot?: object, now?: () => number}} [options]
 * @returns {{relay: object, calls: object[], execOf: Function}} 依赖
 */
function setup(options = {}) {
  const bindings = options.bindings ?? BINDINGS
  const calls = options.calls ?? []
  const agents = new Map()
  for (const binding of bindings) {
    if (typeof binding.mainSessionId === 'string') agents.set(binding.mainSessionId, fakeAgent(binding.mainSessionId, calls))
  }
  const qqbot = options.qqbot ?? (options.say === undefined ? undefined : { say: options.say })
  const relay = createRelayTool({
    getBindings: () => bindings,
    agentOf: (sessionId) => agents.get(sessionId),
    qqbotService: () => qqbot,
    defaultEntity: '猫猫',
    now: options.now ?? (() => 1000),
  })
  return { relay, calls, agents, execOf }
}

/**
 * 造执行上下文。
 * @param {string} sessionId 调用方会话 id
 * @param {string} cwd 调用方 cwd
 * @returns {{agent: object}} exec
 */
function execOf(sessionId, cwd) {
  return { agent: { id: sessionId, session: { id: sessionId, header: { cwd } } } }
}

test('工具定义：名称/参数/输出 schema 与 render（K14）', () => {
  const { relay } = setup()
  const definition = relay.definition
  assert.equal(definition.name, RELAY_TOOL_NAME)
  assert.equal(definition.name, 'astrbox_relay')
  assert.deepEqual(definition.parameters.required, ['target', 'text'])
  assert.equal(definition.parameters.properties.target.type, 'string')
  assert.equal(definition.parameters.properties.text.type, 'string')
  assert.equal(definition.output.schema.required[0], 'delivered')
  assert.equal(RELAY_TEXT_MAX_CHARS, 1000)
  assert.equal(RELAY_RATE_LIMIT, 6)
  const rendered = definition.output.render({}, { delivered: true, kind: 'main', entity: '梦梦' })
  assert.equal(rendered[0].type, 'text')
  assert.ok(rendered[0].text.includes('梦梦'))
  const failed = definition.output.render({}, { delivered: false, reason: 'rate_limited' })
  assert.ok(failed[0].text.includes('rate_limited'))
})

test('target=实体 id：followup 到该实体主会话，前缀带来源实体', async () => {
  const { relay, calls } = setup()
  const result = await relay.execute({ target: '梦梦', text: '在吗' }, execOf('qqbot:helper:main', '/srv/bots/helper'))
  assert.deepEqual(result, { delivered: true, kind: 'main', entity: '梦梦', sessionId: 'qqbot:cat:main' })
  assert.equal(calls.length, 1)
  assert.equal(calls[0].sessionId, 'qqbot:cat:main')
  assert.equal(calls[0].message.role, 'user')
  assert.deepEqual(calls[0].message.source, { kind: 'user' })
  assert.equal(calls[0].message.content[0].text, '[astrbox-relay] 猫猫 对你说：在吗')
})

test('target=主体：投到调用方自己 bot 的主会话（QQ 子会话 → 主会话）', async () => {
  const { relay, calls } = setup()
  const result = await relay.execute({ target: '主体', text: '我回来了' }, execOf('qqbot:cat:group:g1', '/srv/bots/cat'))
  assert.deepEqual(result, { delivered: true, kind: 'main', entity: '梦梦', sessionId: 'qqbot:cat:main' })
  assert.equal(calls[0].message.content[0].text, '[astrbox-relay] 梦梦 对你说：我回来了')
})

test('target=私聊/群聊：调 qqbot say 直发（带 botId/scope），映射返回值', async () => {
  const sayCalls = []
  const { relay } = setup({
    say: async (request) => {
      sayCalls.push(request)
      return { ok: true, botId: request.botId, scope: request.scope, targetId: 'openid-1' }
    },
  })
  const c2c = await relay.execute({ target: '私聊', text: '私聊一句' }, execOf('qqbot:cat:main', '/srv/bots/cat'))
  assert.deepEqual(c2c, { delivered: true, kind: 'qq', scope: 'c2c', botId: 'cat', targetId: 'openid-1' })
  const group = await relay.execute({ target: '群聊', text: '群聊一句' }, execOf('qqbot:cat:group:g1', '/srv/bots/cat'))
  assert.deepEqual(group, { delivered: true, kind: 'qq', scope: 'group', botId: 'cat', targetId: 'openid-1' })
  assert.deepEqual(sayCalls.map((request) => request.scope), ['c2c', 'group'])
  assert.deepEqual(sayCalls.map((request) => request.botId), ['cat', 'cat'])
  assert.deepEqual(sayCalls.map((request) => request.text), ['私聊一句', '群聊一句'])
})

test('target=私聊：调用方未命中绑定 → 唯一 bot 回退；qqbot 缺失/失败给 reason 不抛', async () => {
  const single = setup({ bindings: [BINDINGS[0]] })
  const sayCalls = []
  const singleRelay = createRelayTool({
    getBindings: () => [BINDINGS[0]],
    agentOf: () => undefined,
    qqbotService: () => ({ say: async (request) => { sayCalls.push(request); return { ok: true } } }),
    defaultEntity: '猫猫',
  })
  const singleResult = await singleRelay.execute({ target: '私聊', text: 'hi' }, execOf('unknown', '/elsewhere'))
  assert.equal(singleResult.delivered, true)
  assert.equal(sayCalls[0].botId, 'cat')

  const missing = await single.relay.execute({ target: '私聊', text: 'hi' }, execOf('qqbot:cat:main', '/elsewhere'))
  assert.deepEqual(missing, { delivered: false, reason: 'qqbot_unavailable' })

  const failing = setup({ qqbot: { say: async () => ({ ok: false, reason: 'no_session' }) } })
  const failed = await failing.relay.execute({ target: '群聊', text: 'hi' }, execOf('qqbot:cat:main', '/srv/bots/cat'))
  assert.deepEqual(failed, { delivered: false, reason: 'no_session' })
  const throwing = setup({ qqbot: { say: async () => { throw new Error('boom') } } })
  const thrown = await throwing.relay.execute({ target: '群聊', text: 'hi' }, execOf('qqbot:cat:main', '/srv/bots/cat'))
  assert.deepEqual(thrown, { delivered: false, reason: 'qq_send_failed' })
})

test('限流：同一调用方每分钟最多 6 次，超限 rate_limited，窗口后恢复', async () => {
  let now = 1000
  const { relay, calls } = setup({ now: () => now })
  const exec = execOf('qqbot:helper:main', '/srv/bots/helper')
  for (let index = 0; index < RELAY_RATE_LIMIT; index += 1) {
    const result = await relay.execute({ target: '梦梦', text: `第${index}次` }, exec)
    assert.equal(result.delivered, true, `第 ${index + 1} 次应送达`)
  }
  assert.equal(calls.length, 6)
  const limited = await relay.execute({ target: '梦梦', text: '第7次' }, exec)
  assert.deepEqual(limited, { delivered: false, reason: 'rate_limited' })
  // 另一个调用方不受影响。
  const other = await relay.execute({ target: '梦梦', text: '别人' }, execOf('qqbot:cat:main', '/srv/bots/cat'))
  assert.equal(other.delivered, true)
  assert.equal(calls.length, 7)
  // 窗口滑过后恢复。
  now += 60000
  const resumed = await relay.execute({ target: '梦梦', text: '一分钟后' }, exec)
  assert.equal(resumed.delivered, true)
  relay.dispose()
  const afterDispose = await relay.execute({ target: '梦梦', text: 'dispose 后' }, exec)
  assert.equal(afterDispose.delivered, true, 'dispose 清空限流表后应恢复')
})

test('文本校验：空白/超 1000 字 → invalid_text；1000 字可送达', async () => {
  const { relay, calls } = setup()
  const exec = execOf('qqbot:helper:main', '/srv/bots/helper')
  assert.deepEqual(await relay.execute({ target: '梦梦', text: '   ' }, exec), { delivered: false, reason: 'invalid_text' })
  assert.deepEqual(await relay.execute({ target: '梦梦', text: 'x'.repeat(RELAY_TEXT_MAX_CHARS + 1) }, exec), { delivered: false, reason: 'invalid_text' })
  assert.deepEqual(await relay.execute({ target: '', text: 'hi' }, exec), { delivered: false, reason: 'unknown_target' })
  assert.deepEqual(await relay.execute({ target: '梦梦', text: 42 }, exec), { delivered: false, reason: 'invalid_text' })
  const edge = await relay.execute({ target: '梦梦', text: 'x'.repeat(RELAY_TEXT_MAX_CHARS) }, exec)
  assert.equal(edge.delivered, true)
  assert.equal(calls[0].message.content[0].text.length, '[astrbox-relay] 猫猫 对你说：'.length + RELAY_TEXT_MAX_CHARS)
})

test('target=对方主体：唯一另一个实体的主会话；多实体时 unknown_target', async () => {
  const { relay, calls } = setup()
  const result = await relay.execute({ target: '对方主体', text: '你好呀' }, execOf('qqbot:helper:main', '/srv/bots/helper'))
  assert.deepEqual(result, { delivered: true, kind: 'main', entity: '梦梦', sessionId: 'qqbot:cat:main' })
  assert.equal(calls[0].message.content[0].text, '[astrbox-relay] 猫猫 对你说：你好呀')

  const three = setup({
    bindings: [
      ...BINDINGS,
      { botId: 'third', label: 'third', cwd: '/srv/bots/third', entity: '第三只', mainSessionId: 'qqbot:third:main' },
    ],
  })
  const ambiguous = await three.relay.execute({ target: '对方主体', text: 'hi' }, execOf('qqbot:helper:main', '/srv/bots/helper'))
  assert.deepEqual(ambiguous, { delivered: false, reason: 'unknown_target' })
})

test('目标不可达：未知实体 / 主会话 agent 不在线 → delivered:false 不抛', async () => {
  const { relay } = setup()
  const exec = execOf('qqbot:helper:main', '/srv/bots/helper')
  assert.deepEqual(await relay.execute({ target: '不存在的猫', text: 'hi' }, exec), { delivered: false, reason: 'unknown_target' })
  const offline = createRelayTool({ getBindings: () => BINDINGS, agentOf: () => undefined, defaultEntity: '猫猫' })
  assert.deepEqual(await offline.execute({ target: '梦梦', text: 'hi' }, exec), { delivered: false, reason: 'agent_unavailable' })
})
