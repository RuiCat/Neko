/**
 * Observation v2 接入面（契约 §4/§5）单测：主会话按实体游标拉 delta 推送、
 * 首次基线不回放、子会话 pre-step 只注入现状卡、面板路由由 `/v2/observations/state` 供数。
 *
 * 从既有黑板订阅链路改写；运行方式 `node --test plugins/dsh-astrbox/lib/*Test.mjs`。
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { apply } from './plugin.mjs'
import { mkdtempSync, readFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { createStubContext, runPreStep, startBridgeServer, waitFor } from './testHarness.mjs'

/** 订阅间隔状态临时目录（K17）：避免 apply 读写用户真实 ~/.dsh-astrbox。 */
const TEST_STATE_DIR = mkdtempSync(join(tmpdir(), 'astrbox-observation-test-'))
let testStateSeq = 0
process.on('exit', () => rmSync(TEST_STATE_DIR, { recursive: true, force: true }))

/**
 * apply 包装：每次调用用独立的 subscriptionStatePath。
 * @param {object} ctx Cordis 替身上下文
 * @param {object} [rawConfig] 行配置
 * @returns {object} 运行时句柄
 */
function applyBridge(ctx, rawConfig = {}) {
  testStateSeq += 1
  // entity 固定：apply() 默认会读本机 bridge.config.json，测试不能跟着本机配置漂。
  return apply(ctx, {
    subscriptionStatePath: join(TEST_STATE_DIR, `subscription-${process.pid}-${testStateSeq}.json`),
    entity: '猫猫',
    ...rawConfig,
  })
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

/** 现状夹具：单条 `心情.当前`。 */
function stateFixture(version, value) {
  return {
    version,
    entries: [{ path: '心情.当前', value, from: '满足', tier: 'normal', seq: version, at: '2026-11-06T16:20:00Z' }],
  }
}

/** 一条变化事件。 */
function change(seq, from, to) {
  return { seq, path: '心情.当前', from, to, tier: 'normal', at: '2026-11-06T16:20:00Z' }
}

/** 最近一次 `/v2/observations` 请求的 since 参数（无则 null）。 */
function lastSince(server) {
  const requests = server.requests.filter((request) => new URL(request.url, 'http://localhost').pathname === '/v2/observations')
  if (requests.length === 0) return null
  return new URL(requests.at(-1).url, 'http://localhost').searchParams.get('since')
}

test('主会话推送（v2）：首次以现状 version 为基线不回放；之后只推 delta；无变化不重复', async (t) => {
  const { server, control } = await startBridgeServer()
  t.after(() => server.close())
  const mainSessionId = 'qqbot:cat:main'
  const followups = []
  const stub = createStubContext()
  stub.setService('qqbotBots', { bindings: () => [botBinding('/srv/bots/cat', '梦梦', mainSessionId)] })
  stub.setService('agents', { get: () => fakeAgent(mainSessionId, followups) })
  // 历史事件（seq 1..2）在基线之前：不得回放。
  control.observationStates['梦梦'] = stateFixture(2, '开心')
  control.observations['梦梦'] = { version: 2, events: [change(1, '平静', '满足'), change(2, '满足', '开心')] }
  const runtime = applyBridge(stub.ctx, { socketPath: server.socketPath, tickEveryMs: 20, subscriptionMainIntervalMs: 0 })
  t.after(() => runtime.dispose())
  const tick = stub.intervals[0].callback
  const runTickOnce = async (count) => {
    tick()
    await waitFor(() => control.ticksCompleted === count && runtime.tickInFlight === false)
  }

  // 1) 首次：只拉现状建立游标（version=2），不推送、不回放历史事件。
  await runTickOnce(1)
  assert.equal(followups.length, 0, '首次基线不得回放历史')
  assert.equal(server.requests.filter((request) => new URL(request.url, 'http://localhost').pathname === '/v2/observations/state').length, 1)
  assert.equal(lastSince(server), null, '基线不得用 since=0 拉事件流')

  // 2) 有新事件 → 按游标推 delta。
  control.observations['梦梦'] = { version: 4, events: [change(3, '开心', '超开心'), change(4, '超开心', '兴奋')] }
  await runTickOnce(2)
  assert.equal(followups.length, 1)
  const text = followups[0].content[0].text
  assert.equal(
    text,
    '[astrbox-observation] 梦梦 · 2 处变化\n- 心情.当前: 开心 → 超开心\n- 心情.当前: 超开心 → 兴奋',
  )
  assert.equal(lastSince(server), '2', '第二次请求应带基线游标')
  assert.deepEqual(followups[0].source, { kind: 'user' })

  // 3) 版本未前进 → 不再推送，但游标推进到 cursor。
  await runTickOnce(3)
  assert.equal(followups.length, 1)
  assert.equal(lastSince(server), '4')
})

test('主会话推送（v2）：事件流只有 noise（events 空但 version 前进）时只推进游标不推送', async (t) => {
  const { server, control } = await startBridgeServer()
  t.after(() => server.close())
  const mainSessionId = 'qqbot:cat:main'
  const followups = []
  const stub = createStubContext()
  stub.setService('qqbotBots', { bindings: () => [botBinding('/srv/bots/cat', '梦梦', mainSessionId)] })
  stub.setService('agents', { get: () => fakeAgent(mainSessionId, followups) })
  control.observationStates['梦梦'] = stateFixture(1, '平静')
  control.observations['梦梦'] = { version: 1, events: [] }
  const runtime = applyBridge(stub.ctx, { socketPath: server.socketPath, tickEveryMs: 20, subscriptionMainIntervalMs: 0 })
  t.after(() => runtime.dispose())
  const tick = stub.intervals[0].callback
  const runTickOnce = async (count) => {
    tick()
    await waitFor(() => control.ticksCompleted === count && runtime.tickInFlight === false)
  }

  await runTickOnce(1)
  assert.equal(followups.length, 0)
  // 服务端跳过噪声：version=9、events 空、cursor=9。
  control.observations['梦梦'] = { version: 9, events: [] }
  await runTickOnce(2)
  assert.equal(followups.length, 0)
  // 下一次请求 since=9（已跳过噪声，不重扫）。
  control.observations['梦梦'] = { version: 10, events: [change(10, '平静', '开心')] }
  await runTickOnce(3)
  assert.equal(followups.length, 1)
  assert.ok(followups[0].content[0].text.includes('心情.当前: 平静 → 开心'), followups[0].content[0].text)
})

test('主会话推送（v2）：未到 subscriptionMainIntervalMs 不推（即使有新事件）', async (t) => {
  const { server, control } = await startBridgeServer()
  t.after(() => server.close())
  const mainSessionId = 'qqbot:cat:main'
  const followups = []
  const stub = createStubContext()
  stub.setService('qqbotBots', { bindings: () => [botBinding('/srv/bots/cat', '梦梦', mainSessionId)] })
  stub.setService('agents', { get: () => fakeAgent(mainSessionId, followups) })
  control.observationStates['梦梦'] = stateFixture(1, '平静')
  control.observations['梦梦'] = { version: 2, events: [change(2, '平静', '开心')] }
  const runtime = applyBridge(stub.ctx, { socketPath: server.socketPath, tickEveryMs: 20, subscriptionMainIntervalMs: 600000 })
  t.after(() => runtime.dispose())
  const tick = stub.intervals[0].callback
  const runTickOnce = async (count) => {
    tick()
    await waitFor(() => control.ticksCompleted === count && runtime.tickInFlight === false)
  }

  await runTickOnce(1)
  assert.equal(followups.length, 0, '首次仅基线')
  await runTickOnce(2)
  assert.equal(followups.length, 1, '基线后新事件可立即推一次')
  control.observations['梦梦'] = { version: 3, events: [change(3, '开心', '更开心')] }
  await runTickOnce(3)
  assert.equal(followups.length, 1, '间隔未到不应推送')
  assert.equal(lastSince(server), '1', '间隔门控在拉取前生效，不消耗游标')
})

test('主会话推送（v2）：拉取失败不推进游标，恢复后同批事件补推', async (t) => {
  const { server, control } = await startBridgeServer()
  t.after(() => server.close())
  const mainSessionId = 'qqbot:cat:main'
  const followups = []
  const stub = createStubContext()
  stub.setService('qqbotBots', { bindings: () => [botBinding('/srv/bots/cat', '梦梦', mainSessionId)] })
  stub.setService('agents', { get: () => fakeAgent(mainSessionId, followups) })
  control.observationStates['梦梦'] = stateFixture(2, '平静')
  control.observations['梦梦'] = { version: 2, events: [] }
  const runtime = applyBridge(stub.ctx, { socketPath: server.socketPath, tickEveryMs: 20, subscriptionMainIntervalMs: 0 })
  t.after(() => runtime.dispose())
  const tick = stub.intervals[0].callback
  const runTickOnce = async (count) => {
    tick()
    await waitFor(() => control.ticksCompleted === count && runtime.tickInFlight === false)
  }

  await runTickOnce(1)
  control.observations['梦梦'] = { version: 3, events: [change(3, '平静', '开心')] }
  control.observationMode = 'fail'
  await runTickOnce(2)
  assert.equal(followups.length, 0, '拉取失败不应推送')
  control.observationMode = 'ok'
  await runTickOnce(3)
  assert.equal(followups.length, 1, '恢复后同批事件应补推')
  assert.ok(followups[0].content[0].text.includes('心情.当前: 平静 → 开心'), followups[0].content[0].text)
})

test('主会话推送（v2）：agent 不在线不建立基线；恢复后才建立，仍不回放', async (t) => {
  const { server, control } = await startBridgeServer()
  t.after(() => server.close())
  const mainSessionId = 'qqbot:cat:main'
  const followups = []
  const stub = createStubContext()
  stub.setService('qqbotBots', { bindings: () => [botBinding('/srv/bots/cat', '梦梦', mainSessionId)] })
  control.observationStates['梦梦'] = stateFixture(5, '开心')
  control.observations['梦梦'] = { version: 5, events: [change(5, '满足', '开心')] }
  const runtime = applyBridge(stub.ctx, { socketPath: server.socketPath, tickEveryMs: 20, subscriptionMainIntervalMs: 0 })
  t.after(() => runtime.dispose())
  const tick = stub.intervals[0].callback
  const runTickOnce = async (count) => {
    tick()
    await waitFor(() => control.ticksCompleted === count && runtime.tickInFlight === false)
  }

  // 无 agents 服务、也没有 pre-step 见过的 agent → 跳过，且不建立基线。
  await runTickOnce(1)
  assert.equal(followups.length, 0)
  assert.equal(server.requests.filter((request) => new URL(request.url, 'http://localhost').pathname === '/v2/observations/state').length, 0)

  stub.setService('agents', { get: () => fakeAgent(mainSessionId, followups) })
  await runTickOnce(2)
  assert.equal(followups.length, 0, '恢复后的首轮只建立基线，不回放历史')
  control.observations['梦梦'] = { version: 6, events: [change(6, '开心', '超开心')] }
  await runTickOnce(3)
  assert.equal(followups.length, 1)
  assert.ok(followups[0].content[0].text.includes('超开心'), followups[0].content[0].text)
})

test('主会话推送（v2）：agent 缓存回落（agents 注册表不可用时用 pre-step 见过的 agent）', async (t) => {
  const { server, control } = await startBridgeServer()
  t.after(() => server.close())
  const mainSessionId = 'qqbot:cat:main'
  const followups = []
  const agent = fakeAgent(mainSessionId, followups)
  const stub = createStubContext()
  stub.setService('qqbotBots', { bindings: () => [botBinding('/srv/bots/cat', '梦梦', mainSessionId)] })
  control.observationStates['梦梦'] = stateFixture(1, '开心')
  control.observations['梦梦'] = { version: 2, events: [change(2, '开心', '超开心')] }
  const runtime = applyBridge(stub.ctx, { socketPath: server.socketPath, tickEveryMs: 20, subscriptionMainIntervalMs: 0 })
  t.after(() => runtime.dispose())

  // 主会话先经过一次 pre-step（桥接记住 agent），之后注册表仍不可用。
  await runPreStep(stub.handler('agent/pre-step'), {
    agent: { ...agent, session: { id: mainSessionId, header: { cwd: '/srv/bots/cat' } } },
    turn: 1,
    step: 1,
  })
  stub.intervals[0].callback()
  await waitFor(() => control.ticksCompleted === 1 && runtime.tickInFlight === false)
  assert.equal(followups.length, 0, '首轮建立基线')
  stub.intervals[0].callback()
  await waitFor(() => control.ticksCompleted === 2 && runtime.tickInFlight === false)
  assert.equal(followups.length, 1, '应回落到 bridge 见过的 agent')
  assert.ok(followups[0].content[0].text.includes('超开心'), followups[0].content[0].text)
})

test('子会话 pre-step 只注入现状卡：不再有变化推送链路（v2 单一出口）', async (t) => {
  const { server, control } = await startBridgeServer()
  t.after(() => server.close())
  const stub = createStubContext()
  stub.setService('qqbotBots', { bindings: () => [botBinding('/srv/bots/cat', '梦梦', 'qqbot:cat:main')] })
  control.observationStates['梦梦'] = stateFixture(2, '开心')
  control.observations['梦梦'] = { version: 2, events: [change(2, '满足', '开心')] }
  const runtime = applyBridge(stub.ctx, { socketPath: server.socketPath, promptRefreshMs: 0 })
  t.after(() => runtime.dispose())
  const agent = { session: { id: 'qqbot:cat:group:g1', header: { cwd: '/srv/bots/cat' } } }

  const first = await runPreStep(stub.handler('agent/pre-step'), { agent, turn: 1, step: 1 })
  assert.equal(first.messages.length, 1, '子会话只收现状卡')
  const text = first.messages[0].content[0].text
  assert.ok(text.startsWith('[astrbox-state] 梦梦'), text)
  assert.ok(!text.includes('[astrbox-observation]'), text)
  // 主会话同样只收现状卡（主推送走 tick，不在 pre-step）。
  const mainAgent = { session: { id: 'qqbot:cat:main', header: { cwd: '/srv/bots/cat' } } }
  const main = await runPreStep(stub.handler('agent/pre-step'), { agent: mainAgent, turn: 1, step: 1 })
  assert.equal(main.messages.length, 1)
  assert.ok(main.messages[0].content[0].text.startsWith('[astrbox-state]'))
})

test('astrboxBridge.blackboards()：绑定实体 + 全局实体去重升序，返回缓存的 v2 投影', async (t) => {
  const { server, control } = await startBridgeServer()
  t.after(() => server.close())
  control.observationStates['梦梦'] = stateFixture(7, '开心')
  control.observationStates['猫猫'] = { version: 0, entries: [] }
  const stub = createStubContext()
  stub.setService('qqbotBots', {
    bindings: () => [botBinding('/srv/bots/cat', '梦梦', 'qqbot:cat:main'), botBinding('/srv/bots/helper', '猫猫', 'qqbot:helper:main')],
  })
  const runtime = applyBridge(stub.ctx, { socketPath: server.socketPath, promptRefreshMs: 0 })
  t.after(() => runtime.dispose())

  const host = stub.provided.get('astrboxBridge')
  assert.equal(typeof host.blackboards, 'function')
  assert.deepEqual(host.blackboards().map((item) => item.entity), ['梦梦', '猫猫'])
  await waitFor(() => host.blackboards().some((item) => item.entity === '梦梦' && item.board !== null))
  const board = host.blackboards().find((item) => item.entity === '梦梦').board
  assert.equal(board.entity, '梦梦')
  assert.equal(board.version, 7)
  assert.deepEqual(board.entries, [{ path: '心情.当前', from: '满足', to: '开心', updated_at: '2026-11-06T16:20:00Z' }])
  assert.equal(host.blackboards().find((item) => item.entity === '猫猫').board, null)
})

test('K7 路由挂载：GET /astrbox/blackboard 保留路径与响应形状，由 /v2/observations/state 供数', async (t) => {
  const { server, control } = await startBridgeServer()
  t.after(() => server.close())
  control.observationStates['梦梦'] = stateFixture(5, '开心')
  control.observationStates['猫猫'] = stateFixture(3, '平静')
  const routes = []
  const webServer = {
    register(route) {
      routes.push(route)
      return () => {}
    },
  }
  const stub = createStubContext()
  stub.setService('webServer', webServer)
  stub.setService('qqbotBots', { bindings: () => [botBinding('/srv/bots/cat', '梦梦', 'qqbot:cat:main')] })
  const runtime = applyBridge(stub.ctx, { socketPath: server.socketPath, promptRefreshMs: 0 })
  t.after(() => runtime.dispose())

  const blackboardRoute = routes.find((route) => route.path === '/astrbox/blackboard')
  assert.ok(blackboardRoute, '应注册 GET /astrbox/blackboard')
  assert.equal(blackboardRoute.kind, 'exact')
  assert.ok(routes.some((route) => route.path === '/astrbox/control/tick'), '应注册控制路由（K21）')

  const response = () => ({
    statusCode: 0,
    headers: {},
    body: undefined,
    writeHead(status, headers) {
      this.statusCode = status
      this.headers = headers
    },
    end(text) {
      this.body = JSON.parse(text)
    },
  })

  const one = response()
  await blackboardRoute.handler({ method: 'GET', url: '/astrbox/blackboard?entity=%E6%A2%A6%E6%A2%A6' }, one)
  assert.equal(one.statusCode, 200)
  assert.equal(one.body.ok, true)
  assert.equal(one.body.board.entity, '梦梦')
  assert.equal(one.body.board.version, 5)
  assert.deepEqual(one.body.board.entries, [{ path: '心情.当前', from: '满足', to: '开心', updated_at: '2026-11-06T16:20:00Z' }])

  const all = response()
  await blackboardRoute.handler({ method: 'GET', url: '/astrbox/blackboard' }, all)
  assert.equal(all.body.ok, true)
  assert.equal(all.body.primary, '猫猫')
  assert.deepEqual(all.body.paths, ['心情.当前'])
  assert.deepEqual(
    all.body.boards.map((item) => [item.entity, item.board === null ? null : item.board.version]),
    [['梦梦', 5], ['猫猫', 3]],
  )

  const missing = response()
  await blackboardRoute.handler({ method: 'GET', url: '/astrbox/blackboard?entity=unknown' }, missing)
  assert.deepEqual(missing.body, { ok: true, entity: 'unknown', board: null })

  // 无 webServer 的上下文：不注册、不抛错。
  const plainStub = createStubContext()
  const plain = applyBridge(plainStub.ctx, { socketPath: server.socketPath, tickEveryMs: 0 })
  plain.dispose()
})

test('只有对话 feed 变化不触发主会话推送；有变化时对话随同一条消息附带', async (t) => {
  const { server, control } = await startBridgeServer()
  t.after(() => server.close())
  const mainSessionId = 'qqbot:cat:main'
  const followups = []
  const stub = createStubContext()
  stub.setService('qqbotBots', { bindings: () => [botBinding('/srv/bots/cat', '梦梦', mainSessionId)] })
  stub.setService('agents', { get: () => fakeAgent(mainSessionId, followups) })
  stub.setService('sessionTitle', { get: () => ({ title: '主体' }) })
  control.observationStates['梦梦'] = stateFixture(3, '开心')
  control.observations['梦梦'] = { version: 3, events: [] }
  const runtime = applyBridge(stub.ctx, { socketPath: server.socketPath, tickEveryMs: 20, subscriptionMainIntervalMs: 0, promptRefreshMs: 0 })
  t.after(() => runtime.dispose())
  const tick = stub.intervals[0].callback
  const runTickOnce = async (count) => {
    tick()
    await waitFor(() => control.ticksCompleted === count && runtime.tickInFlight === false)
  }
  const turnStopping = stub.handler('agent/turn-stopping')

  await runTickOnce(1)
  // 只记对话、无变化事件 → 不推送。
  turnStopping({
    agent: {
      session: {
        id: mainSessionId,
        header: { cwd: '/srv/bots/cat' },
        snapshotEvents: () => [
          { type: 'turn/start', data: { turn: 1 } },
          { type: 'user/message', data: { source: { kind: 'user' }, content: [{ type: 'text', text: '主人问' }] } },
          { type: 'assistant/message', data: { message: { content: [{ type: 'text', text: '猫答' }] } } },
          { type: 'turn/end' },
        ],
      },
    },
    turn: 1,
  })
  await runTickOnce(2)
  assert.equal(followups.length, 0, '只有对话变化不推送')

  // 变化出现 → 推送并附带「最近对话」（只含新增条目）。
  control.observations['梦梦'] = { version: 4, events: [change(4, '开心', '超开心')] }
  await runTickOnce(3)
  assert.equal(followups.length, 1)
  const text = followups[0].content[0].text
  assert.ok(text.includes('最近对话：'), text)
  assert.ok(text.includes('- 主体：用户「主人问」（'), text)

  // 再次推送：已推过的对话不重复附带。
  control.observations['梦梦'] = { version: 5, events: [change(5, '超开心', '平静')] }
  await runTickOnce(4)
  assert.equal(followups.length, 2)
  assert.ok(!followups[1].content[0].text.includes('最近对话'), followups[1].content[0].text)
})

test('K17 订阅间隔覆盖：覆盖值门控推送，reset 后恢复全局间隔', async (t) => {
  const { server, control } = await startBridgeServer()
  t.after(() => server.close())
  const mainSessionId = 'qqbot:cat:main'
  const followups = []
  const registered = []
  const statePath = join(TEST_STATE_DIR, `k17-${process.pid}.json`)
  control.observationStates['梦梦'] = stateFixture(1, '开心')
  control.observations['梦梦'] = { version: 2, events: [change(2, '开心', '超开心')] }
  const stub = createStubContext()
  stub.setService('tools', {
    register(definition) {
      registered.push(definition)
      return () => {
        definition.disposed = true
      }
    },
  })
  stub.setService('qqbotBots', { bindings: () => [botBinding('/srv/bots/cat', '梦梦', mainSessionId)] })
  stub.setService('agents', { get: () => fakeAgent(mainSessionId, followups) })
  const runtime = applyBridge(stub.ctx, {
    socketPath: server.socketPath,
    tickEveryMs: 20,
    subscriptionMainIntervalMs: 0,
    subscriptionStatePath: statePath,
  })
  t.after(() => runtime.dispose())
  const tick = stub.intervals[0].callback
  const runTickOnce = async (count) => {
    tick()
    await waitFor(() => control.ticksCompleted === count && runtime.tickInFlight === false)
  }
  const definition = registered.find((item) => item.name === 'astrbox_subscription')
  const mainExec = { agent: { session: { id: mainSessionId, header: { cwd: '/srv/bots/cat' } } } }

  // 全局间隔 0：首轮基线、次轮推送。
  await runTickOnce(1)
  await runTickOnce(2)
  assert.equal(followups.length, 1)

  // 主会话把间隔覆盖为 1 分钟 → 立即再推会被覆盖值挡住。
  const set = await definition.execute({ minutes: 1 }, mainExec)
  assert.deepEqual(set, { ok: true, minutes: 1, intervalMs: 60000, sessionId: mainSessionId, source: 'override' })
  assert.deepEqual(JSON.parse(readFileSync(statePath, 'utf8')), { intervals: { [mainSessionId]: 1 } })
  control.observations['梦梦'] = { version: 3, events: [change(3, '超开心', '兴奋')] }
  await runTickOnce(3)
  assert.equal(followups.length, 1, '覆盖间隔未到不应推送')

  // reset → 回到全局 0，下一 tick 立即满足间隔。
  const reset = await definition.execute({ reset: true }, mainExec)
  assert.equal(reset.ok, true)
  assert.equal(reset.source, 'default')
  control.observations['梦梦'] = { version: 4, events: [change(4, '兴奋', '平静')] }
  await runTickOnce(4)
  assert.equal(followups.length, 2, 'reset 后应恢复全局间隔并推送')
})
