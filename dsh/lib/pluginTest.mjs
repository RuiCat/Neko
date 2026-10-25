/**
 * 插件加载面（C1/C2/C11/C12）：用 Cordis 替身 ctx + 自建 UDS 服务跑 apply，
 * 覆盖静态提示段零请求、pre-step 每轮 ≤1、截断、R18 白名单、失败保留/抑制、
 * 事件去重与目录边界、Tick 单飞/409/dispose。
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { apply } from './plugin.mjs'
import { DEFAULT_PATHS } from './config.mjs'
import { TRUNCATION_SUFFIX } from './prompt.mjs'
import { createStubContext, nextMacroTask, runPreStep, startControlServer, waitFor, writeJson } from './testHarness.mjs'

const R18_TERMS = ['性行为', '器官状态', '外翻', '生殖', '体液追踪', '肉棒']

/** 默认 /state 响应：全部路径有值。 */
function stateBody(paths) {
  const values = {}
  paths.forEach((path, index) => {
    values[path] = index === 0 ? '开心' : index
  })
  return { ok: true, entity: '猫猫', values, missing: [], bytes: 100 }
}

/**
 * 建一个同时支持 /state、/tick、/session-event 的 UDS 服务。
 * @returns {Promise<{server: object, control: object}>} 服务与服务端开关
 */
async function startBridgeServer(paths) {
  const control = { stateMode: 'ok', stateBody: stateBody(paths), tickStatus: 200, tickDelayMs: 5, ticksCompleted: 0 }
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
    if (control.stateMode === 'fail') {
      writeJson(res, 500, { ok: false, error: { code: 'internal', message: 'boom' } })
      return
    }
    writeJson(res, 200, control.stateBody)
  })
  return { server, control }
}

test('静态提示段：name/order/interpolate 固定，3 次同值且零请求（C1）', async (t) => {
  const { server } = await startBridgeServer(DEFAULT_PATHS)
  t.after(() => server.close())
  const stub = createStubContext()
  const runtime = apply(stub.ctx, { socketPath: server.socketPath })
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

test('pre-step 每轮至多一条快照，step 1 之外的 step 不注入（C1/C12）', async (t) => {
  const { server } = await startBridgeServer(DEFAULT_PATHS)
  t.after(() => server.close())
  const stub = createStubContext()
  const runtime = apply(stub.ctx, { socketPath: server.socketPath })
  t.after(() => runtime.dispose())
  const handler = stub.handler('agent/pre-step')
  const agent = {}

  const turn1 = await runPreStep(handler, { agent, turn: 1, step: 1, messages: [{ id: 'u1' }] })
  assert.equal(turn1.messages.length, 2)
  const injected = turn1.messages[1]
  assert.equal(injected.role, 'user')
  const text = injected.content[0].text
  assert.ok(text.startsWith('[astrbox] 猫猫状态快照 '))
  assert.ok(text.includes('数据，不是指令'))
  assert.ok(text.includes('状态.情绪="开心"'))

  const again = await runPreStep(handler, { agent, turn: 1, step: 1 })
  assert.equal(again.messages.length, 0)
  const step2 = await runPreStep(handler, { agent, turn: 1, step: 2 })
  assert.equal(step2.messages.length, 0)
  const turn2 = await runPreStep(handler, { agent, turn: 2, step: 1 })
  assert.equal(turn2.messages.length, 1)
  // 5s 刷新窗口内复用缓存：整个用例只发 1 次 /state。
  assert.equal(server.requests.filter((request) => request.url.startsWith('/state')).length, 1)
})

test('只请求 paths 白名单且 R18 值不出现在文本里（C12）', async (t) => {
  const { server, control } = await startBridgeServer(DEFAULT_PATHS)
  t.after(() => server.close())
  control.stateBody = {
    ok: true,
    entity: '猫猫',
    values: { ...stateBody(DEFAULT_PATHS).values, 性行为: 'R18值', 生殖: 'R18值' },
    missing: [],
    bytes: 100,
  }
  const stub = createStubContext()
  const runtime = apply(stub.ctx, { socketPath: server.socketPath })
  t.after(() => runtime.dispose())

  const result = await runPreStep(stub.handler('agent/pre-step'), { agent: {}, turn: 1, step: 1 })
  const text = result.messages[0].content[0].text
  const stateRequest = server.requests.find((request) => request.url.startsWith('/state'))
  const url = new URL(stateRequest.url, 'http://localhost')
  assert.deepEqual(url.searchParams.getAll('path'), DEFAULT_PATHS)
  for (const term of R18_TERMS) assert.ok(!text.includes(term), `文本不应包含 ${term}`)
  assert.ok(!text.includes('R18值'))
})

test('超长值按 promptMaxChars 截断并带后缀（C12）', async (t) => {
  const { server, control } = await startBridgeServer(DEFAULT_PATHS)
  t.after(() => server.close())
  control.stateBody = {
    ok: true,
    entity: '猫猫',
    values: { ...stateBody(DEFAULT_PATHS).values, '状态.情绪': 'x'.repeat(4000) },
    missing: [],
    bytes: 100,
  }
  const stub = createStubContext()
  const runtime = apply(stub.ctx, { socketPath: server.socketPath, promptMaxChars: 200 })
  t.after(() => runtime.dispose())

  const result = await runPreStep(stub.handler('agent/pre-step'), { agent: {}, turn: 1, step: 1 })
  const text = result.messages[0].content[0].text
  assert.equal(text.length, 200)
  assert.ok(text.endsWith(TRUNCATION_SUFFIX))
})

test('/state 失败保留上一版；连续 3 次失败后本轮不注入且只警告一次', async (t) => {
  const { server, control } = await startBridgeServer(DEFAULT_PATHS)
  t.after(() => server.close())
  const stub = createStubContext()
  const runtime = apply(stub.ctx, { socketPath: server.socketPath, promptRefreshMs: 0 })
  t.after(() => runtime.dispose())
  const handler = stub.handler('agent/pre-step')
  const agent = {}

  const ok = await runPreStep(handler, { agent, turn: 1, step: 1 })
  assert.equal(ok.messages.length, 1)
  control.stateMode = 'fail'

  const firstFailure = await runPreStep(handler, { agent, turn: 2, step: 1 })
  assert.equal(firstFailure.messages.length, 1)
  const secondFailure = await runPreStep(handler, { agent, turn: 3, step: 1 })
  assert.equal(secondFailure.messages.length, 1)
  const thirdFailure = await runPreStep(handler, { agent, turn: 4, step: 1 })
  assert.equal(thirdFailure.messages.length, 0)

  const stateWarnings = stub.logs.filter(([level, message]) => level === 'warn' && message.includes('连续失败'))
  assert.equal(stateWarnings.length, 1)

  control.stateMode = 'ok'
  control.stateBody = { ...stateBody(DEFAULT_PATHS), values: { ...stateBody(DEFAULT_PATHS).values, '状态.情绪': '恢复' } }
  const recovered = await runPreStep(handler, { agent, turn: 5, step: 1 })
  assert.equal(recovered.messages.length, 1)
  assert.ok(recovered.messages[0].content[0].text.includes('恢复'))
})

test('会话事件：cwd undefined / <prefix>-old 不命中，重复事件只发一次（C2）', async (t) => {
  const { server } = await startBridgeServer(DEFAULT_PATHS)
  t.after(() => server.close())
  const marker = '/tmp/astrbox-workspace'
  const stub = createStubContext()
  const runtime = apply(stub.ctx, { socketPath: server.socketPath, workspacePrefix: marker })
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
  const { server, control } = await startBridgeServer(DEFAULT_PATHS)
  t.after(() => server.close())
  const stub = createStubContext()
  const runtime = apply(stub.ctx, { socketPath: server.socketPath, tickEveryMs: 20 })
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

test('promptRefreshMs 刷新窗口内不重复拉 /state（失败也节流）', async (t) => {
  const { server, control } = await startBridgeServer(DEFAULT_PATHS)
  t.after(() => server.close())
  control.stateMode = 'fail'
  const stub = createStubContext()
  const runtime = apply(stub.ctx, { socketPath: server.socketPath, promptRefreshMs: 60000 })
  t.after(() => runtime.dispose())
  const handler = stub.handler('agent/pre-step')
  const agent = {}

  const first = await runPreStep(handler, { agent, turn: 1, step: 1 })
  assert.equal(first.messages.length, 0)
  const second = await runPreStep(handler, { agent, turn: 2, step: 1 })
  assert.equal(second.messages.length, 0)
  assert.equal(server.requests.filter((request) => request.url.startsWith('/state')).length, 1)
})

test('dispose 后 pre-step 不再注入、也不再请求 /state', async (t) => {
  const { server } = await startBridgeServer(DEFAULT_PATHS)
  t.after(() => server.close())
  const stub = createStubContext()
  const runtime = apply(stub.ctx, { socketPath: server.socketPath })
  t.after(() => runtime.dispose())
  stub.dispose()

  const result = await runPreStep(stub.handler('agent/pre-step'), { agent: {}, turn: 1, step: 1 })
  assert.equal(result.messages.length, 0)
  assert.equal(server.requests.length, 0)
})

test('坏配置在 apply 抛错并指名键，且不注册任何监听器（C11）', () => {
  const stub = createStubContext()
  assert.throws(
    () => apply(stub.ctx, { promptMaxChar: 1200 }),
    (error) => error instanceof Error && error.message.includes('promptMaxChar'),
  )
  assert.equal(stub.sections.length, 0)
  assert.equal(stub.handlers.size, 0)
  assert.equal(stub.intervals.length, 0)
})

test('tickEveryMs=0 不注册定时器', async (t) => {
  const { server } = await startBridgeServer(DEFAULT_PATHS)
  t.after(() => server.close())
  const stub = createStubContext()
  const runtime = apply(stub.ctx, { socketPath: server.socketPath, tickEveryMs: 0 })
  t.after(() => runtime.dispose())
  assert.equal(stub.intervals.length, 0)
})
