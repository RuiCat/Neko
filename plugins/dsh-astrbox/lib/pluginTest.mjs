/**
 * 插件加载面（C1/C2/C11/C12）：用 Cordis 替身 ctx + 自建 UDS 控制面跑 apply，
 * 覆盖静态提示段零请求、pre-step 现状卡（Observation v2）、失败保留/抑制、会话事件、
 * Tick 单飞/409/dispose、实体绑定回落、工具注册与 astrboxBridge 服务。
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { apply, inject } from './plugin.mjs'
import { PERSONA_PATHS } from './astrboxService.mjs'
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { createStubContext, nextMacroTask, runPreStep, startBridgeServer, waitFor } from './testHarness.mjs'

/** 订阅间隔状态临时目录（K17）：测试不读写用户真实 ~/.dsh-astrbox。 */
const TEST_STATE_DIR = mkdtempSync(join(tmpdir(), 'astrbox-plugin-test-'))
let testStateSeq = 0
process.on('exit', () => rmSync(TEST_STATE_DIR, { recursive: true, force: true }))

/**
 * apply 包装：每次调用用独立的 subscriptionStatePath，避免测试之间/与真实状态互相影响。
 * @param {object} ctx Cordis 替身上下文
 * @param {object} [rawConfig] 行配置（显式 subscriptionStatePath 优先）
 * @returns {object} 运行时句柄
 */
function applyBridge(ctx, rawConfig = {}) {
  testStateSeq += 1
  return apply(ctx, {
    subscriptionStatePath: join(TEST_STATE_DIR, `subscription-${process.pid}-${testStateSeq}.json`),
    ...rawConfig,
  })
}

/** 默认现状夹具：一条 `状态.身体`。 */
function stateFixture(version = 5, value = '虚弱') {
  return { version, entries: [{ path: '状态.身体', value, from: '疲惫', tier: 'critical', seq: version, at: '2026-11-14T08:00:00Z' }] }
}

/** 一条绑定；可选 mainSessionId 供主会话推送。 */
function botBinding(cwd, entity, mainSessionId) {
  const binding = { botId: `bot-${entity}`, label: entity, cwd, entity }
  if (mainSessionId !== undefined) binding.mainSessionId = mainSessionId
  return binding
}

/** 假 agent：记录 followup 收到的消息。 */
function fakeAgent(sessionId, followups) {
  return {
    id: sessionId,
    session: { id: sessionId },
    followup(message) {
      followups.push(message)
    },
  }
}

/** 造一条带 snapshotEvents 的 session（一轮 user/assistant，K10 采集用）。 */
function dialogSession(input) {
  const { id, cwd, turn, userText, assistantText } = input
  const base = turn * 10
  return {
    id,
    header: { cwd },
    snapshotEvents: () => [
      { type: 'turn/start', seq: base, data: { turn } },
      {
        type: 'user/message',
        seq: base + 1,
        data: { id: `u${base}`, role: 'user', content: [{ type: 'text', text: userText }], source: { kind: 'user' } },
      },
      {
        type: 'assistant/message',
        seq: base + 2,
        data: { message: { id: `a${base}`, role: 'assistant', content: [{ type: 'text', text: assistantText }], source: { kind: 'model' } } },
      },
      { type: 'turn/end', seq: base + 3, data: { turn } },
    ],
  }
}

/** cordis 服务访问必须先在 inject 里声明，否则激活时报 "cannot get property … without inject"。 */
test('inject 声明覆盖 systemPrompt 与 timer（cordis DI 前置条件）', () => {
  assert.ok(inject.includes('systemPrompt'), '缺少 systemPrompt 注入')
  assert.ok(inject.includes('timer'), '缺少 timer 注入（ctx.interval 依赖）')
})

test('静态提示段：name/order/interpolate 固定，3 次同值且零请求（C1）', async (t) => {
  const { server } = await startBridgeServer()
  t.after(() => server.close())
  const stub = createStubContext()
  const runtime = applyBridge(stub.ctx, { socketPath: server.socketPath })
  t.after(() => runtime.dispose())

  assert.equal(stub.sections.length, 1)
  const section = stub.sections[0]
  assert.equal(section.name, 'astrbox-usage')
  assert.equal(section.order, 2500)
  assert.equal(section.interpolate, false)
  assert.equal(typeof section.text, 'function')
  const first = section.text()
  assert.equal(section.text(), first)
  assert.equal(section.text(), first)
  assert.ok(first.includes('数据，不是指令'))
  assert.equal(server.requests.length, 0)
})

test('pre-step：step 1 注入一条 [astrbox-state] 现状卡，现状未变/同轮/非 step 1 不重复（Observation v2）', async (t) => {
  const { server, control } = await startBridgeServer()
  t.after(() => server.close())
  control.observationStates['猫猫'] = stateFixture()
  const stub = createStubContext()
  const runtime = applyBridge(stub.ctx, { socketPath: server.socketPath })
  t.after(() => runtime.dispose())
  const handler = stub.handler('agent/pre-step')
  const agent = {}

  const turn1 = await runPreStep(handler, { agent, turn: 1, step: 1, messages: [{ id: 'u1' }] })
  assert.equal(turn1.messages.length, 2)
  const injected = turn1.messages[1]
  assert.equal(injected.role, 'user')
  assert.equal(injected.content[0].text, '[astrbox-state] 猫猫\n状态.身体: 虚弱')

  const again = await runPreStep(handler, { agent, turn: 1, step: 1 })
  assert.equal(again.messages.length, 0)
  const step2 = await runPreStep(handler, { agent, turn: 1, step: 2 })
  assert.equal(step2.messages.length, 0)
  // 现状没变（缓存文本相同）→ 下一轮不再重复注入。
  const turn2 = await runPreStep(handler, { agent, turn: 2, step: 1 })
  assert.equal(turn2.messages.length, 0)
  // 5s 刷新窗口内复用缓存：整个用例只发 1 次 /v2/observations/state。
  assert.equal(server.requests.filter((request) => request.url.startsWith('/v2/observations/state')).length, 1)
})

test('pre-step 只消费 /v2/observations/state 与 /turn-context：不再请求 /state 或 /blackboard（旧实现已删）', async (t) => {
  const { server, control } = await startBridgeServer()
  t.after(() => server.close())
  control.observationStates['猫猫'] = stateFixture()
  const stub = createStubContext()
  const runtime = applyBridge(stub.ctx, { socketPath: server.socketPath })
  t.after(() => runtime.dispose())

  const result = await runPreStep(stub.handler('agent/pre-step'), { agent: {}, turn: 1, step: 1 })
  assert.equal(result.messages.length, 1)
  const paths = server.requests.map((request) => new URL(request.url, 'http://localhost').pathname)
  assert.deepEqual(paths, ['/v2/observations/state', '/turn-context'])
})

test('pre-step 注入补偿包：记忆/约束/情境/专家，文本不变不重复（Observation v2）', async (t) => {
  const { server, control } = await startBridgeServer()
  t.after(() => server.close())
  control.observationStates['猫猫'] = stateFixture()
  control.turnContext = {
    ok: true,
    compensation: {
      memory: [{ type: 'episodic', source_snapshot_id: 's-1', content: '昨天在书房' }],
      persona_constraints: ['不主动离开场景'],
      situation: '傍晚，书房',
    },
    gate: { active: ['情绪专家', '记忆专家'] },
  }
  const stub = createStubContext()
  const runtime = applyBridge(stub.ctx, { socketPath: server.socketPath })
  t.after(() => runtime.dispose())
  const handler = stub.handler('agent/pre-step')

  const first = await runPreStep(handler, { agent: {}, turn: 1, step: 1, messages: [] })
  assert.equal(first.messages.length, 2)
  assert.equal(first.messages[0].content[0].text, '[astrbox-state] 猫猫\n状态.身体: 虚弱')
  const compensation = first.messages[1].content[0].text
  assert.ok(compensation.startsWith('[astrbox-compensation] 猫猫 补偿包 '), compensation)
  assert.ok(compensation.includes('记忆:\n- [episodic s-1] 昨天在书房'), compensation)
  assert.ok(compensation.includes('约束: 不主动离开场景'), compensation)
  assert.ok(compensation.includes('情境: 傍晚，书房'), compensation)
  assert.ok(compensation.includes('专家: 情绪专家、记忆专家'), compensation)

  // 补偿包文本未变 → 下一轮不再注入；现状卡也相同 → 整轮没有新消息。
  const second = await runPreStep(handler, { agent: {}, turn: 2, step: 1 })
  assert.equal(second.messages.length, 0)
})

test('现状卡超过 promptMaxChars 时截断（§4.3 字符预算）', async (t) => {
  const { server, control } = await startBridgeServer()
  t.after(() => server.close())
  control.observationStates['猫猫'] = { version: 1, entries: [{ path: '状态.情绪', value: 'x'.repeat(4000) }] }
  const stub = createStubContext()
  const runtime = applyBridge(stub.ctx, { socketPath: server.socketPath, promptMaxChars: 200 })
  t.after(() => runtime.dispose())

  const result = await runPreStep(stub.handler('agent/pre-step'), { agent: {}, turn: 1, step: 1 })
  const text = result.messages[0].content[0].text
  assert.ok(text.length <= 200, String(text.length))
  // 超预算时从尾部丢行并给未显示条数（§4.3）。
  assert.equal(text, '[astrbox-state] 猫猫\n另有 1 条未显示')
})

test('/v2/observations/state 失败保留上一版；连续 3 次失败后本轮不注入且只警告一次', async (t) => {
  const { server, control } = await startBridgeServer()
  t.after(() => server.close())
  control.observationStates['猫猫'] = stateFixture(1, '平静')
  const stub = createStubContext()
  const runtime = applyBridge(stub.ctx, { socketPath: server.socketPath, promptRefreshMs: 0 })
  t.after(() => runtime.dispose())
  const handler = stub.handler('agent/pre-step')
  const agent = {}

  const ok = await runPreStep(handler, { agent, turn: 1, step: 1 })
  assert.equal(ok.messages.length, 1)
  assert.ok(ok.messages[0].content[0].text.includes('平静'))
  control.observationMode = 'fail'

  // 拉取失败但缓存文本没变：同样默认跳过注入。
  for (const turn of [2, 3, 4]) {
    const decision = await runPreStep(handler, { agent, turn, step: 1 })
    assert.equal(decision.messages.length, 0)
  }
  const warnings = stub.logs.filter(([level, message]) => level === 'warn' && message.includes('连续失败'))
  assert.equal(warnings.length, 1)

  control.observationMode = 'ok'
  control.observationStates['猫猫'] = stateFixture(2, '恢复')
  const recovered = await runPreStep(handler, { agent, turn: 5, step: 1 })
  assert.equal(recovered.messages.length, 1)
  assert.ok(recovered.messages[0].content[0].text.includes('恢复'))
})

test('会话事件：cwd undefined / <prefix>-old 不命中，重复事件只发一次（C2）', async (t) => {
  const { server } = await startBridgeServer()
  t.after(() => server.close())
  const marker = '/tmp/astrbox-workspace'
  const stub = createStubContext()
  const runtime = applyBridge(stub.ctx, { socketPath: server.socketPath, workspacePrefix: marker })
  t.after(() => runtime.dispose())

  const onCreated = stub.handler('session/created')
  assert.doesNotThrow(() => onCreated({ id: 's0', header: {} }))
  assert.doesNotThrow(() => onCreated({ id: 's1', header: { cwd: `${marker}-old` } }))
  onCreated({ id: 's2', header: { cwd: `${marker}/sub` } })
  onCreated({ id: 's2', header: { cwd: `${marker}/sub` } })
  const onTurnStopping = stub.handler('agent/turn-stopping')
  const agent = { session: { id: 's2', header: { cwd: `${marker}/sub` } } }
  onTurnStopping({ agent, turn: 3 })
  onTurnStopping({ agent, turn: 3 })
  await waitFor(() => server.requests.filter((request) => request.url === '/session-event').length === 2)
  await nextMacroTask()

  const events = server.requests.filter((request) => request.url === '/session-event').map((request) => request.body)
  assert.equal(events.length, 2)
  assert.deepEqual(events[0], {
    kind: 'session-created',
    sessionId: 's2',
    cwd: `${marker}/sub`,
    function: 'dsh_on_event',
  })
  assert.deepEqual(events[1], {
    kind: 'turn-end',
    sessionId: 's2',
    cwd: `${marker}/sub`,
    turn: 3,
    function: 'dsh_on_event',
  })
})

test('Tick：单飞、409 记为跳过、dispose 后不再发送且中止在飞请求（K2）', async (t) => {
  const { server, control } = await startBridgeServer()
  t.after(() => server.close())
  const stub = createStubContext()
  const runtime = applyBridge(stub.ctx, { socketPath: server.socketPath, tickEveryMs: 20 })
  t.after(() => runtime.dispose())

  assert.equal(stub.intervals.length, 1)
  assert.equal(stub.intervals[0].delay, 20)
  const tick = stub.intervals[0].callback

  tick()
  tick()
  await waitFor(() => control.ticksCompleted === 1 && runtime.tickInFlight === false)
  assert.equal(server.requests.filter((request) => request.url === '/tick').length, 1)

  tick()
  await waitFor(() => control.ticksCompleted === 2 && runtime.tickInFlight === false)
  assert.equal(server.requests.filter((request) => request.url === '/tick').length, 2)

  control.tickStatus = 409
  tick()
  await waitFor(() => control.ticksCompleted === 3 && runtime.tickInFlight === false)
  await waitFor(() => stub.logs.some(([, message]) => message.includes('tick_busy')))
  assert.equal(server.requests.filter((request) => request.url === '/tick').length, 3)

  // dispose：定时器被停掉，回调变成 no-op；在飞请求被中止。
  control.tickStatus = 200
  control.tickDelayMs = 500
  const beforeDispose = server.requests.filter((request) => request.url === '/tick').length
  tick()
  await waitFor(() => server.requests.filter((request) => request.url === '/tick').length === beforeDispose + 1)
  stub.dispose()
  assert.equal(stub.intervals[0].active, false)
  await nextMacroTask()
  tick()
  await nextMacroTask()
  assert.equal(server.requests.filter((request) => request.url === '/tick').length, beforeDispose + 1)
})

test('世界时钟（K12）：无活动不发 advanceMinutes；有会话活动后按 clockStepMinutes 推进', async (t) => {
  const { server, control } = await startBridgeServer()
  t.after(() => server.close())
  const stub = createStubContext()
  const runtime = applyBridge(stub.ctx, {
    socketPath: server.socketPath,
    tickEveryMs: 20,
    clockStepMinutes: 10,
    clockIdleStepMinutes: 0,
    clockActiveWindowMs: 60000,
  })
  t.after(() => runtime.dispose())
  const tick = stub.intervals[0].callback
  const tickRequests = () => server.requests.filter((request) => request.url === '/tick')

  // 从未有会话活动 → 空闲步长 0 → 请求体不带 advanceMinutes（旧协议兼容）。
  tick()
  await waitFor(() => control.ticksCompleted === 1 && runtime.tickInFlight === false)
  assert.equal(tickRequests().at(-1).body.advanceMinutes, undefined)

  // 模拟一次会话活动（turn-stopping）→ 进入活跃窗口 → 推进 10 分钟。
  stub.handler('agent/turn-stopping')({
    agent: { session: { id: 'session-x', header: { cwd: '/tmp/work' }, snapshotEvents: () => [] } },
    turn: 1,
  })
  tick()
  await waitFor(() => control.ticksCompleted === 2 && runtime.tickInFlight === false)
  assert.equal(tickRequests().at(-1).body.advanceMinutes, 10)

  // 活跃窗口过期（窗口 0ms）后回到空闲步长 0 → 不再推进。
  const expired = applyBridge(stub.ctx, {
    socketPath: server.socketPath,
    tickEveryMs: 20,
    clockStepMinutes: 10,
    clockIdleStepMinutes: 0,
    clockActiveWindowMs: 0,
  })
  t.after(() => expired.dispose())
  stub.handler('agent/pre-step')
  const tick2 = stub.intervals.at(-1).callback
  tick2()
  await waitFor(() => control.ticksCompleted === 3 && expired.tickInFlight === false)
  const bodies = tickRequests().map((request) => request.body.advanceMinutes)
  assert.equal(bodies.at(-1), undefined)
})

test('世界时钟（K12）：clockIdleStepMinutes 让空闲也按周期推进；双 0 关闭时钟', async (t) => {
  const { server, control } = await startBridgeServer()
  t.after(() => server.close())
  const stub = createStubContext()
  const runtime = applyBridge(stub.ctx, {
    socketPath: server.socketPath,
    tickEveryMs: 20,
    clockStepMinutes: 10,
    clockIdleStepMinutes: 1,
    clockActiveWindowMs: 60000,
  })
  t.after(() => runtime.dispose())
  const tick = stub.intervals[0].callback
  tick()
  await waitFor(() => control.ticksCompleted === 1 && runtime.tickInFlight === false)
  const first = server.requests.filter((request) => request.url === '/tick').at(-1)
  assert.equal(first.body.advanceMinutes, 1)

  const off = applyBridge(stub.ctx, {
    socketPath: server.socketPath,
    tickEveryMs: 20,
    clockStepMinutes: 0,
    clockIdleStepMinutes: 0,
  })
  t.after(() => off.dispose())
  const tick2 = stub.intervals.at(-1).callback
  tick2()
  await waitFor(() => control.ticksCompleted === 2 && off.tickInFlight === false)
  const second = server.requests.filter((request) => request.url === '/tick').at(-1)
  assert.equal(second.body.advanceMinutes, undefined)
})

test('promptRefreshMs 刷新窗口内不重复拉 /v2/observations/state（失败也节流）', async (t) => {
  const { server } = await startBridgeServer()
  t.after(() => server.close())
  const stub = createStubContext()
  const runtime = applyBridge(stub.ctx, { socketPath: server.socketPath, promptRefreshMs: 60000 })
  t.after(() => runtime.dispose())
  const handler = stub.handler('agent/pre-step')
  const agent = {}

  const first = await runPreStep(handler, { agent, turn: 1, step: 1 })
  assert.equal(first.messages.length, 0)
  const second = await runPreStep(handler, { agent, turn: 2, step: 1 })
  assert.equal(second.messages.length, 0)
  assert.equal(server.requests.filter((request) => request.url.startsWith('/v2/observations/state')).length, 1)
})

test('dispose 后 pre-step 不再注入、也不再请求', async (t) => {
  const { server } = await startBridgeServer()
  t.after(() => server.close())
  const stub = createStubContext()
  const runtime = applyBridge(stub.ctx, { socketPath: server.socketPath })
  t.after(() => runtime.dispose())
  stub.dispose()

  const result = await runPreStep(stub.handler('agent/pre-step'), { agent: {}, turn: 1, step: 1 })
  assert.equal(result.messages.length, 0)
  assert.equal(server.requests.length, 0)
})

test('坏配置在 apply 抛错并指名键，且不注册任何监听器（C11）', () => {
  const stub = createStubContext()
  assert.throws(
    () => applyBridge(stub.ctx, { promptMaxChar: 1200 }),
    (error) => error instanceof Error && error.message.includes('promptMaxChar'),
  )
  assert.equal(stub.sections.length, 0)
  assert.equal(stub.handlers.size, 0)
  assert.equal(stub.intervals.length, 0)
})

test('tickEveryMs=0 不注册定时器', async (t) => {
  const { server } = await startBridgeServer()
  t.after(() => server.close())
  const stub = createStubContext()
  const runtime = applyBridge(stub.ctx, { socketPath: server.socketPath, tickEveryMs: 0 })
  t.after(() => runtime.dispose())
  assert.equal(stub.intervals.length, 0)
})

test('inject 不声明 qqbotBots；服务缺失仍提供 astrboxBridge 且不抛错（C6/K1/K2）', () => {
  assert.ok(!inject.includes('qqbotBots'), 'qqbotBots 不得进 inject（C6）')
  const stub = createStubContext()
  const runtime = applyBridge(stub.ctx, { socketPath: '/nonexistent/control.sock', tickEveryMs: 0 })
  try {
    assert.ok(stub.provided.has('astrboxBridge'))
    assert.doesNotThrow(() => stub.handler('session/created')({ id: 's1', header: {} }))
  } finally {
    runtime.dispose()
  }
  assert.equal(stub.provided.has('astrboxBridge'), false, 'dispose 后服务应注销')
})

test('绑定命中：pre-step 按绑定实体拉 /v2/observations/state（K6/C13）', async (t) => {
  const { server, control } = await startBridgeServer()
  t.after(() => server.close())
  control.observationStates['梦梦'] = stateFixture(2, '梦梦状态')
  control.observationStates['猫猫'] = stateFixture(1, '猫猫状态')
  const stub = createStubContext()
  stub.setService('qqbotBots', { bindings: () => [botBinding('/srv/bots/cat', '梦梦')] })
  const runtime = applyBridge(stub.ctx, { socketPath: server.socketPath })
  t.after(() => runtime.dispose())

  const result = await runPreStep(stub.handler('agent/pre-step'), {
    agent: { session: { id: 's-cat', header: { cwd: '/srv/bots/cat' } } },
    turn: 1,
    step: 1,
  })
  const text = result.messages[0].content[0].text
  assert.equal(text, '[astrbox-state] 梦梦\n状态.身体: 梦梦状态')
  const stateRequest = server.requests.find((request) => request.url.startsWith('/v2/observations/state'))
  assert.equal(new URL(stateRequest.url, 'http://localhost').searchParams.get('scope'), 'entity:梦梦')
})

test('同一刷新窗口内 cat→helper 不串实体（C4/K6）', async (t) => {
  const { server, control } = await startBridgeServer()
  t.after(() => server.close())
  control.observationStates['梦梦'] = stateFixture(1, '梦梦情绪')
  control.observationStates['猫猫'] = stateFixture(1, '猫猫情绪')
  const stub = createStubContext()
  stub.setService('qqbotBots', {
    bindings: () => [botBinding('/srv/bots/cat', '梦梦'), botBinding('/srv/bots/helper', '猫猫')],
  })
  const runtime = applyBridge(stub.ctx, { socketPath: server.socketPath, promptRefreshMs: 60000 })
  t.after(() => runtime.dispose())
  const handler = stub.handler('agent/pre-step')

  const cat = await runPreStep(handler, {
    agent: { session: { id: 's-cat', header: { cwd: '/srv/bots/cat' } } },
    turn: 1,
    step: 1,
  })
  const helper = await runPreStep(handler, {
    agent: { session: { id: 's-helper', header: { cwd: '/srv/bots/helper' } } },
    turn: 1,
    step: 1,
  })
  const catText = cat.messages[0].content[0].text
  const helperText = helper.messages[0].content[0].text
  assert.equal(catText, '[astrbox-state] 梦梦\n状态.身体: 梦梦情绪')
  assert.equal(helperText, '[astrbox-state] 猫猫\n状态.身体: 猫猫情绪')
  const entities = server.requests
    .filter((request) => request.url.startsWith('/v2/observations/state'))
    .map((request) => new URL(request.url, 'http://localhost').searchParams.get('scope'))
  assert.deepEqual(entities, ['entity:梦梦', 'entity:猫猫'])
})

test('未命中绑定回落全局 config.entity（K6）', async (t) => {
  const { server, control } = await startBridgeServer()
  t.after(() => server.close())
  control.observationStates['猫猫'] = stateFixture(1, '猫猫状态')
  control.observationStates['梦梦'] = stateFixture(1, '梦梦状态')
  const stub = createStubContext()
  stub.setService('qqbotBots', { bindings: () => [botBinding('/srv/bots/cat', '梦梦')] })
  const runtime = applyBridge(stub.ctx, { socketPath: server.socketPath })
  t.after(() => runtime.dispose())

  const result = await runPreStep(stub.handler('agent/pre-step'), {
    agent: { session: { id: 's-other', header: { cwd: '/elsewhere/project' } } },
    turn: 1,
    step: 1,
  })
  assert.equal(result.messages[0].content[0].text, '[astrbox-state] 猫猫\n状态.身体: 猫猫状态')
  const stateRequest = server.requests.find((request) => request.url.startsWith('/v2/observations/state'))
  assert.equal(new URL(stateRequest.url, 'http://localhost').searchParams.get('scope'), 'entity:猫猫')
})

test('绑定服务返回空/HMR 空窗时继续用最近一次非空绑定（K6）', async (t) => {
  const { server, control } = await startBridgeServer()
  t.after(() => server.close())
  control.observationStates['梦梦'] = stateFixture(1, '初始')
  let bindings = [botBinding('/srv/bots/cat', '梦梦')]
  const stub = createStubContext()
  stub.setService('qqbotBots', { bindings: () => bindings })
  const runtime = applyBridge(stub.ctx, { socketPath: server.socketPath, promptRefreshMs: 0 })
  t.after(() => runtime.dispose())
  const handler = stub.handler('agent/pre-step')
  const agent = { session: { id: 's-cat', header: { cwd: '/srv/bots/cat' } } }

  const first = await runPreStep(handler, { agent, turn: 1, step: 1 })
  assert.ok(first.messages[0].content[0].text.startsWith('[astrbox-state] 梦梦'), first.messages[0].content[0].text)
  // 现状没变：默认跳过注入。
  bindings = []
  control.observationStates['梦梦'] = stateFixture(2, '初始')
  const second = await runPreStep(handler, { agent, turn: 2, step: 1 })
  assert.equal(second.messages.length, 0)
  // 现状变化后仍按「最近一次非空绑定」解析到梦梦（HMR 空窗回落）。
  control.observationStates['梦梦'] = stateFixture(3, '恢复')
  const third = await runPreStep(handler, { agent, turn: 3, step: 1 })
  assert.ok(third.messages[0].content[0].text.startsWith('[astrbox-state] 梦梦'), third.messages[0].content[0].text)
})

test('绑定 cwd 的会话事件带 entity 且不被 workspacePrefix 丢弃（K6/C1）', async (t) => {
  const { server } = await startBridgeServer()
  t.after(() => server.close())
  const marker = '/tmp/astrbox-workspace'
  const stub = createStubContext()
  stub.setService('qqbotBots', { bindings: () => [botBinding('/srv/bots/cat', '梦梦')] })
  const runtime = applyBridge(stub.ctx, { socketPath: server.socketPath, workspacePrefix: marker })
  t.after(() => runtime.dispose())

  const onCreated = stub.handler('session/created')
  const onTurnStopping = stub.handler('agent/turn-stopping')
  onCreated({ id: 'bot-s', header: { cwd: '/srv/bots/cat' } })
  onTurnStopping({ agent: { session: { id: 'bot-s', header: { cwd: '/srv/bots/cat' } } }, turn: 4 })
  // 未绑定且不在前缀内：旧行为不发。
  onCreated({ id: 'outside', header: { cwd: '/tmp/other' } })
  onTurnStopping({ agent: { session: { id: 'outside', header: { cwd: '/tmp/other' } } }, turn: 1 })
  // 未绑定但在前缀内：旧行为发、不带 entity。
  onCreated({ id: 'inside', header: { cwd: `${marker}/sub` } })
  await waitFor(() => server.requests.filter((request) => request.url === '/session-event').length === 3)
  await nextMacroTask()

  const events = server.requests.filter((request) => request.url === '/session-event').map((request) => request.body)
  assert.equal(events.length, 3)
  assert.deepEqual(events[0], {
    kind: 'session-created',
    sessionId: 'bot-s',
    cwd: '/srv/bots/cat',
    function: 'dsh_on_event',
    entity: '梦梦',
  })
  assert.deepEqual(events[1], {
    kind: 'turn-end',
    sessionId: 'bot-s',
    cwd: '/srv/bots/cat',
    turn: 4,
    function: 'dsh_on_event',
    entity: '梦梦',
  })
  assert.deepEqual(events[2], {
    kind: 'session-created',
    sessionId: 'inside',
    cwd: `${marker}/sub`,
    function: 'dsh_on_event',
  })
})

test('astrboxBridge 服务：entities/persona/refresh 可读，refresh 后 persona 非空（K2/C3）', async (t) => {
  const { server, control } = await startBridgeServer()
  t.after(() => server.close())
  control.entitiesBody = { entities: [{ id: '猫猫', type: '角色基础' }, { id: '梦梦', type: '角色基础' }] }
  const values = {}
  for (const path of PERSONA_PATHS) values[path] = `值-${path}`
  values['种族'] = '猫'
  values['人格.MBTI'] = 'INFP'
  control.stateBody = { ok: true, entity: '猫猫', values, missing: [], bytes: 100 }
  const stub = createStubContext()
  stub.setService('qqbotBots', { bindings: () => [botBinding('/srv/bots/cat', '猫猫')] })
  const runtime = applyBridge(stub.ctx, { socketPath: server.socketPath })
  t.after(() => runtime.dispose())

  const host = stub.provided.get('astrboxBridge')
  assert.ok(host, 'apply 后应提供 astrboxBridge 服务')
  assert.equal(typeof host.entities, 'function')
  assert.equal(typeof host.persona, 'function')
  assert.equal(typeof host.refresh, 'function')
  assert.ok(Array.isArray(host.entities()))
  await waitFor(() => host.entities().includes('梦梦'))
  assert.deepEqual(host.entities(), ['猫猫', '梦梦'])
  assert.equal(typeof host.persona('猫猫'), 'string')
  await host.refresh()
  const persona = host.persona('猫猫')
  assert.equal(typeof persona, 'string')
  assert.ok(persona.startsWith('## 你的角色（AstrBox NPC 实体：猫猫）'), persona)
  assert.ok(persona.includes('- 种族="猫"'), persona)
  assert.ok(persona.includes('- 人格.MBTI="INFP"'), persona)

  runtime.dispose()
  assert.equal(stub.provided.has('astrboxBridge'), false)
})

test('astrboxBridge.mainSessionPrompt：按实体读提示词，未配置/空实体回空串（K28）', async (t) => {
  const { server } = await startBridgeServer()
  t.after(() => server.close())
  const promptPath = join(TEST_STATE_DIR, `main-session-prompt-${process.pid}.json`)
  writeFileSync(promptPath, JSON.stringify({ prompts: { 猫猫: '  你是猫猫，这里是你自己的主会话。  ' } }), 'utf8')
  const stub = createStubContext()
  const runtime = applyBridge(stub.ctx, { socketPath: server.socketPath, mainSessionPromptStatePath: promptPath })
  t.after(() => runtime.dispose())

  const host = stub.provided.get('astrboxBridge')
  assert.equal(typeof host.mainSessionPrompt, 'function')
  assert.equal(host.mainSessionPrompt('猫猫'), '你是猫猫，这里是你自己的主会话。')
  assert.equal(host.mainSessionPrompt(' 猫猫 '), '你是猫猫，这里是你自己的主会话。')
  assert.equal(host.mainSessionPrompt('梦梦'), '')
  assert.equal(host.mainSessionPrompt(''), '')
  assert.equal(host.mainSessionPrompt(undefined), '')
  runtime.dispose()
})

test('astrbox_relay（K14）：tools 服务可用时注册，execute 转达到实体主会话', async (t) => {
  const { server } = await startBridgeServer()
  t.after(() => server.close())
  const registered = []
  const followups = []
  const catMain = 'qqbot:cat:main'
  const helperMain = 'qqbot:helper:main'
  const stub = createStubContext()
  stub.setService('tools', {
    register(definition) {
      registered.push(definition)
      return () => {
        definition.disposed = true
      }
    },
  })
  stub.setService('qqbotBots', {
    bindings: () => [botBinding('/srv/bots/cat', '梦梦', catMain), botBinding('/srv/bots/helper', '猫猫', helperMain)],
  })
  stub.setService('agents', { get: (id) => (id === catMain ? fakeAgent(catMain, followups) : undefined) })
  // K18：来源标签读宿主 sessionTitle（主体 / 私聊(…) / 群聊(…)）。
  stub.setService('sessionTitle', { get: () => ({ title: '主体' }) })
  const runtime = applyBridge(stub.ctx, { socketPath: server.socketPath, tickEveryMs: 0 })
  t.after(() => runtime.dispose())

  const relayDefinition = registered.find((definition) => definition.name === 'astrbox_relay')
  const subscriptionDefinition = registered.find((definition) => definition.name === 'astrbox_subscription')
  const mainPromptDefinition = registered.find((definition) => definition.name === 'astrbox_main_prompt')
  assert.ok(relayDefinition, '应注册 astrbox_relay')
  assert.ok(subscriptionDefinition, '应注册 astrbox_subscription（K17）')
  assert.ok(mainPromptDefinition, '应注册 astrbox_main_prompt（K29）')
  assert.equal(typeof subscriptionDefinition.execute, 'function')
  const result = await relayDefinition.execute(
    { target: '梦梦', text: '转达一句' },
    { agent: { session: { id: helperMain, header: { cwd: '/srv/bots/helper' } } } },
  )
  assert.equal(result.delivered, true)
  assert.equal(result.kind, 'main')
  assert.equal(result.entity, '梦梦')
  assert.equal(result.sessionId, catMain)
  // K18：成功结果带来源会话信息，正文前缀含「实体·标题（会话 id 前 8）」。
  assert.deepEqual(result.source, { sessionId: helperMain, entity: '猫猫', label: '猫猫·主体（qqbot:he）' })
  assert.equal(followups.length, 1)
  assert.equal(followups[0].content[0].text, '[astrbox-relay] 猫猫·主体（qqbot:he） 对你说：转达一句')

  // dispose 时注销工具（tools.register 返回的释放函数被调用）。
  runtime.dispose()
  assert.equal(relayDefinition.disposed, true)
  assert.equal(subscriptionDefinition.disposed, true)
})

test('astrbox_relay（K14）：无 tools 服务只 warn 一次且插件照常工作', async (t) => {
  const { server, control } = await startBridgeServer()
  t.after(() => server.close())
  control.observationStates['梦梦'] = stateFixture(1, '平静')
  const stub = createStubContext()
  stub.setService('qqbotBots', { bindings: () => [botBinding('/srv/bots/cat', '梦梦', 'qqbot:cat:main')] })
  const runtime = applyBridge(stub.ctx, { socketPath: server.socketPath, tickEveryMs: 0 })
  t.after(() => runtime.dispose())
  const warnings = stub.logs.filter(([level, message]) => level === 'warn' && message.includes('tools 服务不可用'))
  assert.equal(warnings.length, 1)
  const result = await runPreStep(stub.handler('agent/pre-step'), {
    agent: { session: { id: 'qqbot:cat:main', header: { cwd: '/srv/bots/cat' } } },
    turn: 1,
    step: 1,
  })
  assert.equal(result.messages.length, 1, '插件照常注入现状卡')
})

test('工具注册：冷启动 tools 尚未就绪时经 ctx.inject 延迟注册（不重复、dispose 注销）', async (t) => {
  const { server } = await startBridgeServer()
  t.after(() => server.close())
  const stub = createStubContext()
  stub.setService('qqbotBots', { bindings: () => [] })
  let deferred
  let scopeDisposed = false
  stub.ctx.inject = (deps, callback) => {
    deferred = callback
    return () => { scopeDisposed = true }
  }
  const runtime = applyBridge(stub.ctx, { socketPath: server.socketPath, tickEveryMs: 0 })
  t.after(() => runtime.dispose())

  const registered = []
  const tools = { register: (definition) => { registered.push(definition); return () => {} } }
  assert.equal(registered.length, 0, 'tools 未就绪时不应立即注册')
  deferred({ tools })
  assert.deepEqual(registered.map((definition) => definition.name).sort(), ['astrbox_board', 'astrbox_main_prompt', 'astrbox_relay', 'astrbox_subscription'])
  deferred({ tools })
  assert.equal(registered.length, 4, '重复回调不得重复注册')

  runtime.dispose()
  assert.equal(scopeDisposed, true, 'dispose 应释放 inject 作用域')
})

test('K10：assistant-only / 注入文本轮次不记 feed；真实对话行带 sessionTitle label', async (t) => {
  const { server, control } = await startBridgeServer()
  t.after(() => server.close())
  const mainSessionId = 'qqbot:cat:main'
  const followups = []
  const stub = createStubContext()
  stub.setService('qqbotBots', { bindings: () => [botBinding('/srv/bots/cat', '梦梦', mainSessionId)] })
  stub.setService('agents', { get: () => fakeAgent(mainSessionId, followups) })
  stub.setService('sessionTitle', { get: () => ({ title: '私聊(85D47757)' }) })
  control.observationStates['梦梦'] = stateFixture(2, '初始')
  control.observations['梦梦'] = { version: 3, events: [{ seq: 3, path: '心情.当前', from: '满足', to: '开心', tier: 'normal' }] }
  const runtime = applyBridge(stub.ctx, {
    socketPath: server.socketPath,
    tickEveryMs: 20,
    subscriptionMainIntervalMs: 0,
    promptRefreshMs: 0,
    subscriptionMaxChars: 2000,
  })
  t.after(() => runtime.dispose())
  const tick = stub.intervals[0].callback
  const onTurnStopping = stub.handler('agent/turn-stopping')

  // 1) 基线：只建立游标，不推送、不回放。
  tick()
  await waitFor(() => control.ticksCompleted === 1 && runtime.tickInFlight === false)
  assert.equal(followups.length, 0)

  // 2) 注入文本触发的 NPC 回复（user 文本是桥接自己注入的前缀）→ 不产生 feed entry。
  onTurnStopping({
    agent: {
      session: dialogSession({
        id: 'qqbot:cat:c2c:u1',
        cwd: '/srv/bots/cat',
        turn: 1,
        userText: '[astrbox-observation] 梦梦 · 1 处变化\n- 心情.当前: 满足 → 开心',
        assistantText: '好的喵',
      }),
    },
    turn: 1,
  })

  // 3) 变化流有事件 → 推送；没有对话条目时不含「最近对话」。
  tick()
  await waitFor(() => control.ticksCompleted === 2 && runtime.tickInFlight === false)
  assert.equal(followups.length, 1)
  assert.ok(followups[0].content[0].text.startsWith('[astrbox-observation] 梦梦 · 1 处变化'), followups[0].content[0].text)
  assert.ok(!followups[0].content[0].text.includes('最近对话'), followups[0].content[0].text)

  // 4) 真实用户对话 → 记录并带 sessionTitle label；下一次变化推送附带该行。
  onTurnStopping({
    agent: {
      session: dialogSession({ id: 'qqbot:cat:c2c:u1', cwd: '/srv/bots/cat', turn: 2, userText: '主人真的在吗', assistantText: '在的喵' }),
    },
    turn: 2,
  })
  control.observations['梦梦'] = {
    version: 4,
    events: [{ seq: 4, path: '心情.当前', from: '开心', to: '兴奋', tier: 'normal' }],
  }
  tick()
  await waitFor(() => control.ticksCompleted === 3 && runtime.tickInFlight === false)
  assert.equal(followups.length, 2)
  const text = followups[1].content[0].text
  assert.ok(text.includes('最近对话：'), text)
  assert.ok(text.includes('- 私聊(85D47757)：用户「主人真的在吗」（'), text)
})
