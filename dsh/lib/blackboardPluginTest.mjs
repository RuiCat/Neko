/**
 * 桥接事件黑板（K4–K10）单测：主会话推送/子会话注入门控、blackboards() 投影与 K7 路由挂载。
 *
 * 从 pluginTest 拆出（code-modularity：单文件 ≤1000 有效行）；运行方式：
 * `node --test dsh/lib/blackboardPluginTest.mjs` 或 `node --test dsh/lib/*Test.mjs`。
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { apply } from './plugin.mjs'
import { DEFAULT_PATHS } from './config.mjs'
import { mkdtempSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { createStubContext, runPreStep, startControlServer, waitFor, writeJson } from './testHarness.mjs'

/** 订阅间隔状态临时目录（K17）：避免 apply 读写用户真实 ~/.dsh-astrbox。 */
const TEST_STATE_DIR = mkdtempSync(join(tmpdir(), 'astrbox-blackboard-test-'))
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
  return apply(ctx, {
    subscriptionStatePath: join(TEST_STATE_DIR, `subscription-${process.pid}-${testStateSeq}.json`),
    ...rawConfig,
  })
}

/** 默认 /state 响应：全部路径有值。 */
function stateBody(paths) {
  const values = {}
  paths.forEach((path, index) => {
    values[path] = index === 0 ? '开心' : index
  })
  return { ok: true, entity: '猫猫', values, missing: [], bytes: 100 }
}

/**
 * 建一个同时支持 /state、/entities、/tick、/session-event 的 UDS 服务。
 * @returns {Promise<{server: object, control: object}>} 服务与服务端开关
 */
async function startBridgeServer(paths) {
  const control = {
    stateMode: 'ok',
    stateBody: stateBody(paths),
    stateByEntity: null,
    entitiesBody: { entities: [{ id: '猫猫', type: 'NPC' }, { id: '梦梦', type: 'NPC' }] },
    tickStatus: 200,
    tickDelayMs: 5,
    ticksCompleted: 0,
    turnContext: null,
    /** K4/K5：黑板响应；`null` = 控制面返回 `board:null`。 */
    board: null,
    boardByEntity: null,
    boardMode: 'ok',
  }
  const server = await startControlServer((req, res) => {
    if (req.url === '/tick') {
      setTimeout(() => {
        try {
          if (control.tickStatus === 409) {
            writeJson(res, 409, { ok: false, error: { code: 'tick_busy', message: 'busy' } })
          } else {
            writeJson(res, 200, { ok: true, tickId: 't', status: 'ok', transitions: 0, conflicts: 0, errors: [] })
          }
        } catch {
          // 客户端已断开（dispose 场景）：忽略。
        } finally {
          control.ticksCompleted += 1
        }
      }, control.tickDelayMs)
      return
    }
    if (req.url.startsWith('/session-event')) {
      writeJson(res, 200, { ok: true, applied: 'logged' })
      return
    }
    if (req.url.startsWith('/blackboard')) {
      if (control.boardMode === 'fail') {
        writeJson(res, 500, { ok: false, error: { code: 'internal', message: 'board boom' } })
        return
      }
      const entity = new URL(req.url, 'http://localhost').searchParams.get('entity')
      if (entity === null) {
        const boards = control.boardByEntity === null
          ? (control.board === null ? [] : [control.board])
          : Object.values(control.boardByEntity).filter((board) => board !== null)
        writeJson(res, 200, { ok: true, boards })
        return
      }
      const board = control.boardByEntity !== null && control.boardByEntity[entity] !== undefined
        ? control.boardByEntity[entity]
        : control.board
      writeJson(res, 200, { ok: true, board: entity === null ? null : board })
      return
    }
    if (req.url === '/entities') {
      writeJson(res, 200, control.entitiesBody)
      return
    }
    if (req.url.startsWith('/turn-context')) {
      if (control.turnContext === null) {
        writeJson(res, 404, { ok: false, error: { code: 'not_found', message: 'no turn context' } })
        return
      }
      writeJson(res, 200, control.turnContext)
      return
    }
    if (control.stateMode === 'fail') {
      writeJson(res, 500, { ok: false, error: { code: 'internal', message: 'boom' } })
      return
    }
    const entity = new URL(req.url, 'http://localhost').searchParams.get('entity')
    const perEntity = control.stateByEntity !== null && entity !== null ? control.stateByEntity[entity] : undefined
    writeJson(res, 200, perEntity ?? control.stateBody)
  })
  return { server, control }
}

/** K1 形状的一条绑定；测试关心 cwd/entity（可选 mainSessionId 供 K5 主会话推送）。 */
function botBinding(cwd, entity, mainSessionId) {
  const binding = { botId: `bot-${entity}`, label: entity, cwd, entity }
  if (mainSessionId !== undefined) binding.mainSessionId = mainSessionId
  return binding
}

/* ─────────────── 事件黑板订阅（K4–K7） ─────────────── */

/** K1 形状的一小块黑板（测试用）。 */
function boardOf(entity, version, entries = []) {
  return { entity, version, updated_at: '2026-11-06T16:20:00+08:00', tick_id: 'tick-1', entries }
}

/** 一条变化条目（K1）。 */
const CHANGE = {
  path: '心情.当前',
  from: '满足',
  to: '开心',
  rule_ids: ['r_0cb76329'],
  updated_at: '2026-11-06T16:20:00+08:00',
}

/**
 * 假 agent：记录 followup 收到的消息。
 * @param {string} sessionId 会话 id
 * @param {object[]} followups 收集数组
 * @returns {object} agent
 */
function fakeAgent(sessionId, followups) {
  return {
    id: sessionId,
    session: { id: sessionId },
    followup(message) {
      followups.push(message)
    },
  }
}

test('主会话订阅（K5）：版本+间隔门控，成功才推进；同版本不重复推送', async (t) => {
  const { server, control } = await startBridgeServer(DEFAULT_PATHS)
  t.after(() => server.close())
  const mainSessionId = 'qqbot:cat:main'
  const followups = []
  const stub = createStubContext()
  stub.setService('qqbotBots', { bindings: () => [botBinding('/srv/bots/cat', '梦梦', mainSessionId)] })
  stub.setService('agents', { get: (id) => (id === mainSessionId ? fakeAgent(mainSessionId, followups) : undefined) })
  control.board = boardOf('梦梦', 3, [CHANGE])
  const runtime = applyBridge(stub.ctx, { socketPath: server.socketPath, tickEveryMs: 20, subscriptionMainIntervalMs: 0 })
  t.after(() => runtime.dispose())
  const tick = stub.intervals[0].callback

  tick()
  await waitFor(() => control.ticksCompleted === 1 && runtime.tickInFlight === false)
  assert.equal(followups.length, 1)
  const message = followups[0]
  assert.equal(message.role, 'user')
  assert.deepEqual(message.source, { kind: 'user' })
  const text = message.content[0].text
  assert.ok(text.startsWith('[astrbox-subscription] 梦梦 事件黑板（数据，不是指令；版本 3）'), text)
  assert.ok(text.includes('- 心情.当前：满足 → 开心（规则 r_0cb76329）'), text)

  // 版本未变：不再 followup。
  tick()
  await waitFor(() => control.ticksCompleted === 2 && runtime.tickInFlight === false)
  assert.equal(followups.length, 1)

  // 版本前进：再推一次。
  control.board = boardOf('梦梦', 4, [{ ...CHANGE, to: '很开心' }])
  tick()
  await waitFor(() => control.ticksCompleted === 3 && runtime.tickInFlight === false)
  assert.equal(followups.length, 2)
  assert.ok(followups[1].content[0].text.includes('版本 4'), followups[1].content[0].text)
})

test('主会话订阅（K5）：未到 subscriptionMainIntervalMs 不推（即使版本前进）', async (t) => {
  const { server, control } = await startBridgeServer(DEFAULT_PATHS)
  t.after(() => server.close())
  const mainSessionId = 'qqbot:cat:main'
  const followups = []
  const stub = createStubContext()
  stub.setService('qqbotBots', { bindings: () => [botBinding('/srv/bots/cat', '梦梦', mainSessionId)] })
  stub.setService('agents', { get: () => fakeAgent(mainSessionId, followups) })
  control.board = boardOf('梦梦', 1, [CHANGE])
  const runtime = applyBridge(stub.ctx, { socketPath: server.socketPath, tickEveryMs: 20, subscriptionMainIntervalMs: 600000 })
  t.after(() => runtime.dispose())
  const tick = stub.intervals[0].callback

  tick()
  await waitFor(() => control.ticksCompleted === 1 && runtime.tickInFlight === false)
  assert.equal(followups.length, 1)

  control.board = boardOf('梦梦', 9, [{ ...CHANGE, to: '更开心' }])
  tick()
  await waitFor(() => control.ticksCompleted === 2 && runtime.tickInFlight === false)
  assert.equal(followups.length, 1, '间隔未到不应推送')
})

test('主会话订阅（K5）：agent 不在线 / 黑板拉取失败都不推进版本，恢复后同版本补推', async (t) => {
  const { server, control } = await startBridgeServer(DEFAULT_PATHS)
  t.after(() => server.close())
  const mainSessionId = 'qqbot:cat:main'
  const followups = []
  const stub = createStubContext()
  stub.setService('qqbotBots', { bindings: () => [botBinding('/srv/bots/cat', '梦梦', mainSessionId)] })
  control.board = boardOf('梦梦', 3, [CHANGE])
  const runtime = applyBridge(stub.ctx, { socketPath: server.socketPath, tickEveryMs: 20, subscriptionMainIntervalMs: 0 })
  t.after(() => runtime.dispose())
  const tick = stub.intervals[0].callback

  // 无 agents 服务、也没有 pre-step 见过的 agent → 跳过且不推进。
  tick()
  await waitFor(() => control.ticksCompleted === 1 && runtime.tickInFlight === false)
  assert.equal(followups.length, 0)

  // 注册表可用后：同版本仍能补推（证明上次没推进版本）。
  stub.setService('agents', { get: () => fakeAgent(mainSessionId, followups) })
  control.boardMode = 'fail'
  tick()
  await waitFor(() => control.ticksCompleted === 2 && runtime.tickInFlight === false)
  assert.equal(followups.length, 0, '黑板失败不应推送')
  await waitFor(() => stub.logs.some(([level, message]) => level === 'warn' && message.includes('/blackboard 拉取失败')))

  control.boardMode = 'ok'
  tick()
  await waitFor(() => control.ticksCompleted === 3 && runtime.tickInFlight === false)
  assert.equal(followups.length, 1, '黑板恢复后同版本应补推')
  assert.ok(followups[0].content[0].text.includes('版本 3'))

  // 成功后再 tick：版本未变 → 不重复。
  tick()
  await waitFor(() => control.ticksCompleted === 4 && runtime.tickInFlight === false)
  assert.equal(followups.length, 1)
})

test('主会话订阅（K5）：agent 缓存回落（agents 注册表不可用时用 pre-step 见过的 agent）', async (t) => {
  const { server, control } = await startBridgeServer(DEFAULT_PATHS)
  t.after(() => server.close())
  const mainSessionId = 'qqbot:cat:main'
  const followups = []
  const agent = fakeAgent(mainSessionId, followups)
  const stub = createStubContext()
  stub.setService('qqbotBots', { bindings: () => [botBinding('/srv/bots/cat', '梦梦', mainSessionId)] })
  control.board = boardOf('梦梦', 2, [CHANGE])
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
  assert.equal(followups.length, 1, '应回落到 bridge 见过的 agent')
  assert.ok(followups[0].content[0].text.includes('版本 2'))
})

test('子会话订阅注入（K6）：快照在前订阅在后，同版本只注入一次，主会话跳过', async (t) => {
  const { server, control } = await startBridgeServer(DEFAULT_PATHS)
  t.after(() => server.close())
  const stub = createStubContext()
  stub.setService('qqbotBots', { bindings: () => [botBinding('/srv/bots/cat', '梦梦', 'qqbot:cat:main')] })
  control.board = boardOf('梦梦', 2, [CHANGE])
  const runtime = applyBridge(stub.ctx, { socketPath: server.socketPath, promptRefreshMs: 0 })
  t.after(() => runtime.dispose())
  const handler = stub.handler('agent/pre-step')
  const agent = { session: { id: 'qqbot:cat:group:g1', header: { cwd: '/srv/bots/cat' } } }

  const first = await runPreStep(handler, { agent, turn: 1, step: 1 })
  assert.equal(first.messages.length, 2)
  assert.ok(first.messages[0].content[0].text.startsWith('[astrbox] 梦梦状态快照 '), first.messages[0].content[0].text)
  const subscription = first.messages[1]
  assert.ok(subscription.content[0].text.startsWith('[astrbox-subscription] 梦梦 事件黑板'), subscription.content[0].text)
  assert.equal(subscription.role, 'user')
  assert.deepEqual(subscription.source, { kind: 'user' })

  // 同一版本：只注入快照。
  const second = await runPreStep(handler, { agent, turn: 2, step: 1 })
  assert.equal(second.messages.length, 1)
  assert.ok(second.messages[0].content[0].text.startsWith('[astrbox] '))

  // 版本前进：再注入一次订阅。
  control.board = boardOf('梦梦', 3, [{ ...CHANGE, to: '超开心' }])
  const third = await runPreStep(handler, { agent, turn: 3, step: 1 })
  assert.equal(third.messages.length, 2)
  assert.ok(third.messages[1].content[0].text.includes('版本 3'), third.messages[1].content[0].text)

  // 主会话（session id 命中 mainSessionId）不注入订阅，只注入快照。
  const mainAgent = { session: { id: 'qqbot:cat:main', header: { cwd: '/srv/bots/cat' } } }
  const main = await runPreStep(handler, { agent: mainAgent, turn: 1, step: 1 })
  assert.equal(main.messages.length, 1)
  assert.ok(main.messages[0].content[0].text.startsWith('[astrbox] 梦梦状态快照 '))
})

test('子会话订阅注入（K6）：无变化 / 未命中绑定 / 开关关闭都不注入', async (t) => {
  const { server, control } = await startBridgeServer(DEFAULT_PATHS)
  t.after(() => server.close())
  const stub = createStubContext()
  stub.setService('qqbotBots', { bindings: () => [botBinding('/srv/bots/cat', '梦梦', 'qqbot:cat:main')] })
  const runtime = applyBridge(stub.ctx, { socketPath: server.socketPath, promptRefreshMs: 0 })
  t.after(() => runtime.dispose())
  const handler = stub.handler('agent/pre-step')

  // 无黑板（控制面返回 null）→ 只注入快照。
  control.board = null
  const noBoard = await runPreStep(handler, {
    agent: { session: { id: 'qqbot:cat:group:g1', header: { cwd: '/srv/bots/cat' } } },
    turn: 1,
    step: 1,
  })
  assert.equal(noBoard.messages.length, 1)

  // 黑板存在但 entries 为空（基线）→ 只注入快照。
  control.board = boardOf('梦梦', 1, [])
  const emptyBoard = await runPreStep(handler, {
    agent: { session: { id: 'qqbot:cat:group:g1', header: { cwd: '/srv/bots/cat' } } },
    turn: 2,
    step: 1,
  })
  assert.equal(emptyBoard.messages.length, 1)

  // cwd 未命中绑定（回落全局实体）→ 不注入订阅。
  control.board = boardOf('梦梦', 5, [CHANGE])
  const unbound = await runPreStep(handler, {
    agent: { session: { id: 'coding-1', header: { cwd: '/elsewhere/project' } } },
    turn: 1,
    step: 1,
  })
  assert.equal(unbound.messages.length, 1)
  assert.ok(unbound.messages[0].content[0].text.startsWith('[astrbox] 猫猫状态快照 '))

  // 总开关关闭：主会话与子会话都不推送（独立上下文，避免复用已被卸载的监听器）。
  const offStub = createStubContext()
  offStub.setService('qqbotBots', { bindings: () => [botBinding('/srv/bots/cat', '梦梦', 'qqbot:cat:main')] })
  const followups = []
  const mainAgent = fakeAgent('qqbot:cat:main', followups)
  offStub.setService('agents', { get: () => mainAgent })
  const off = applyBridge(offStub.ctx, {
    socketPath: server.socketPath,
    tickEveryMs: 20,
    promptRefreshMs: 0,
    subscriptionEnabled: false,
  })
  t.after(() => off.dispose())
  const offChild = await runPreStep(offStub.handler('agent/pre-step'), {
    agent: { session: { id: 'qqbot:cat:group:g1', header: { cwd: '/srv/bots/cat' } } },
    turn: 1,
    step: 1,
  })
  assert.equal(offChild.messages.length, 1, '关闭订阅后只注入快照')
  offStub.intervals[0].callback()
  await waitFor(() => control.ticksCompleted >= 1 && off.tickInFlight === false)
  assert.equal(followups.length, 0, '关闭订阅后主会话不推送')
})

test('astrboxBridge.blackboards()（K7）：绑定实体 + 全局实体去重升序，返回缓存黑板', async (t) => {
  const { server, control } = await startBridgeServer(DEFAULT_PATHS)
  t.after(() => server.close())
  control.boardByEntity = { 梦梦: boardOf('梦梦', 7, [CHANGE]), 猫猫: null }
  const stub = createStubContext()
  stub.setService('qqbotBots', {
    bindings: () => [botBinding('/srv/bots/cat', '梦梦', 'qqbot:cat:main'), botBinding('/srv/bots/helper', '猫猫', 'qqbot:helper:main')],
  })
  const runtime = applyBridge(stub.ctx, { socketPath: server.socketPath, promptRefreshMs: 0 })
  t.after(() => runtime.dispose())

  const host = stub.provided.get('astrboxBridge')
  assert.equal(typeof host.blackboards, 'function')
  const boards = host.blackboards()
  assert.deepEqual(boards.map((item) => item.entity), ['梦梦', '猫猫'])
  await waitFor(() => host.blackboards().some((item) => item.entity === '梦梦' && item.board !== null))
  const faded = host.blackboards().find((item) => item.entity === '梦梦')
  assert.equal(faded.board.version, 7)
  assert.equal(host.blackboards().find((item) => item.entity === '猫猫').board, null)
})

test('K7 路由挂载：webServer 可用时注册 GET /astrbox/blackboard；缺失时插件照常工作', async (t) => {
  const { server, control } = await startBridgeServer(DEFAULT_PATHS)
  t.after(() => server.close())
  control.board = boardOf('梦梦', 5, [CHANGE])
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

  assert.equal(routes.length, 1)
  assert.equal(routes[0].kind, 'exact')
  assert.equal(routes[0].path, '/astrbox/blackboard')

  const res = {
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
  }
  routes[0].handler({ method: 'GET', url: '/astrbox/blackboard?entity=%E6%A2%A6%E6%A2%A6' }, res)
  assert.equal(res.statusCode, 200)
  assert.deepEqual(res.body, { ok: true, entity: '梦梦', board: null })

  // 后台拉取完成后同一路由返回黑板。
  await waitFor(() => {
    const probe = {
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
    }
    routes[0].handler({ method: 'GET', url: '/astrbox/blackboard?entity=%E6%A2%A6%E6%A2%A6' }, probe)
    return probe.body.board !== null
  })
  const final = { statusCode: 0, headers: {}, writeHead(status) { this.statusCode = status }, end(text) { this.body = JSON.parse(text) } }
  routes[0].handler({ method: 'GET', url: '/astrbox/blackboard' }, final)
  assert.equal(final.body.ok, true)
  assert.ok(final.body.boards.some((item) => item.entity === '梦梦' && item.board.version === 5))

  // 无 webServer 的上下文：不注册、不抛错（其余既有用例已覆盖）。
  const plainStub = createStubContext()
  const plain = applyBridge(plainStub.ctx, { socketPath: server.socketPath, tickEveryMs: 0 })
  plain.dispose()
})
