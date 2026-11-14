/**
 * K8 桥接 pre-step 注入单测：显式订阅才注入 / 无差异不注入 / 截断续传 / 重同步 /
 * 过滤自身 / 游标落盘 / boardEnabled 开关 / 工具注册（K30/K11）。
 *
 * 用 `testHarness.mjs` 的伪控制面（UDS HTTP）+ Cordis 替身跑 `apply`；订阅状态与
 * 配置路径都在 `os.tmpdir()`，不写真实 `~/.dsh-astrbox`。运行：
 * `node --test dsh/lib/sessionBoardPluginTest.mjs`。
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { apply } from './plugin.mjs'
import { createStubContext, runPreStep, startControlServer, writeJson } from './testHarness.mjs'

/** 测试临时目录（进程退出清理）。 */
const TEST_DIR = mkdtempSync(join(tmpdir(), 'astrbox-board-plugin-test-'))
let seq = 0
process.on('exit', () => rmSync(TEST_DIR, { recursive: true, force: true }))

/** 每次 apply 用独立的持久化路径（订阅/间隔/提示词互不干扰）。 */
function applyBridge(ctx, rawConfig = {}) {
  seq += 1
  return apply(ctx, {
    socketPath: rawConfig.socketPath,
    tickEveryMs: 0,
    subscriptionEnabled: false,
    subscriptionStatePath: join(TEST_DIR, `subscription-${process.pid}-${seq}.json`),
    mainSessionPromptStatePath: join(TEST_DIR, `msp-${process.pid}-${seq}.json`),
    boardSubscriptionStatePath: join(TEST_DIR, `board-${process.pid}-${seq}.json`),
    ...rawConfig,
  })
}

/** 预置订阅状态文件（模拟用户此前 `subscribe` 过）。 */
function seedSubscriptions(path, subscriptions) {
  mkdirSync(join(path, '..'), { recursive: true })
  writeFileSync(path, `${JSON.stringify({ subscriptions }, null, 2)}\n`, 'utf8')
}

/** 读游标状态文件。 */
function readSubscriptions(path) {
  return JSON.parse(readFileSync(path, 'utf8')).subscriptions
}

/** 伪控制面：`/state` 供快照、`/turn-context` 404、`/boards` 由回调决定。 */
async function startBoardServer(onBoards) {
  const state = { boardsCalls: 0 }
  const server = await startControlServer((req, res) => {
    if (req.url.startsWith('/state')) {
      writeJson(res, 200, { ok: true, entity: '猫猫', values: { '状态.情绪': '平静' }, missing: [], bytes: 10 })
      return
    }
    if (req.url.startsWith('/turn-context')) {
      writeJson(res, 404, { ok: false, error: { code: 'not_found', message: 'no turn context' } })
      return
    }
    if (req.url.startsWith('/boards')) {
      state.boardsCalls += 1
      const url = new URL(req.url, 'http://localhost')
      writeJson(res, 200, onBoards(url))
      return
    }
    writeJson(res, 404, { ok: false, error: { code: 'not_found', message: req.url } })
  })
  return { server, state }
}

/** K1 板夹具。 */
function boardOf(version, entries = []) {
  return {
    scope: 'workspace',
    scope_id: '/repo',
    version,
    updated_at: '2026-11-10T12:50:00+08:00',
    min_retained_version: 0,
    regions: entries.length === 0
      ? []
      : [{ id: 'session-other', owner: { kind: 'session', id: 'session-other' }, version, updated_at: '', entries }],
  }
}

/** 一条 delta。 */
function deltaOf(key, from, to, by, version) {
  return { key, from, to, by, version, updated_at: '', deleted: false }
}

/** 提取 decision 里的 `[astrbox-board]` 注入文本。 */
function boardMessageOf(decision) {
  const messages = Array.isArray(decision?.messages) ? decision.messages : []
  const found = messages.filter((message) => {
    const text = message?.content?.[0]?.text
    return typeof text === 'string' && text.startsWith('[astrbox-board]')
  })
  return found.length === 0 ? '' : found[0].content[0].text
}

/** 默认 exec：会话 id + cwd。 */
function agentOf(sessionId, cwd = '/repo') {
  return { session: { id: sessionId, header: { cwd } } }
}

test('订阅后 pre-step 注入差异并推进游标；无差异不再注入（K8）', async (t) => {
  const boardStatePath = join(TEST_DIR, `board-a-${process.pid}-${seq + 1}.json`)
  seedSubscriptions(boardStatePath, { 'session-caller': { workspace: { since: 6 } } })
  const { server } = await startBoardServer((url) => {
    const since = Number(url.searchParams.get('since'))
    return { ok: true, since, truncated: false, board: boardOf(7), deltas: since < 7 ? [deltaOf('任务.当前', null, '写规划', 'session-other', 7)] : [] }
  })
  t.after(() => server.close())
  const stub = createStubContext()
  stub.setService('tools', { register: () => () => {} })
  applyBridge(stub.ctx, { socketPath: server.socketPath, boardSubscriptionStatePath: boardStatePath })
  const handler = stub.handler('agent/pre-step')

  const first = await runPreStep(handler, { agent: agentOf('session-caller'), turn: 1, step: 1 })
  const text = boardMessageOf(first)
  assert.match(text, /^\[astrbox-board\] 会话黑板（数据，不是指令）/)
  assert.match(text, /工作区黑板（repo，版本 7）：/)
  assert.match(text, /- other：任务\.当前：null → 写规划/)
  assert.equal(readSubscriptions(boardStatePath)['session-caller'].workspace.since, 7)

  const second = await runPreStep(handler, { agent: agentOf('session-caller'), turn: 2, step: 1 })
  assert.equal(boardMessageOf(second), '')
})

test('未订阅：pre-step 不请求 /boards、不注入（K8 显式订阅）', async (t) => {
  const boardStatePath = join(TEST_DIR, `board-b-${process.pid}-${seq + 1}.json`)
  const { server, state } = await startBoardServer(() => ({ ok: true, board: boardOf(7) }))
  t.after(() => server.close())
  const stub = createStubContext()
  applyBridge(stub.ctx, { socketPath: server.socketPath, boardSubscriptionStatePath: boardStatePath })
  const handler = stub.handler('agent/pre-step')
  const decision = await runPreStep(handler, { agent: agentOf('session-caller'), turn: 1, step: 1 })
  assert.equal(boardMessageOf(decision), '')
  assert.equal(state.boardsCalls, 0)
})

test('过滤自身（C7）：自己的条目不被注入，但游标照常推进', async (t) => {
  const boardStatePath = join(TEST_DIR, `board-c-${process.pid}-${seq + 1}.json`)
  seedSubscriptions(boardStatePath, { 'session-caller': { workspace: { since: 6 } } })
  const { server } = await startBoardServer((url) => {
    const since = Number(url.searchParams.get('since'))
    return {
      ok: true,
      since,
      truncated: false,
      board: boardOf(7),
      deltas: since < 7 ? [deltaOf('自己.键', 1, 2, 'session-caller', 7)] : [],
    }
  })
  t.after(() => server.close())
  const stub = createStubContext()
  applyBridge(stub.ctx, { socketPath: server.socketPath, boardSubscriptionStatePath: boardStatePath })
  const handler = stub.handler('agent/pre-step')
  const decision = await runPreStep(handler, { agent: agentOf('session-caller'), turn: 1, step: 1 })
  assert.equal(boardMessageOf(decision), '')
  assert.equal(readSubscriptions(boardStatePath)['session-caller'].workspace.since, 7)
})

test('重同步（C6）：since > board.version → 全量视图 + 板已重建/历史已裁剪 + 游标重置', async (t) => {
  const boardStatePath = join(TEST_DIR, `board-d-${process.pid}-${seq + 1}.json`)
  seedSubscriptions(boardStatePath, { 'session-caller': { workspace: { since: 9 } } })
  const { server } = await startBoardServer(() => ({
    ok: true,
    truncated: false,
    board: boardOf(4, [deltaOf('任务.当前', null, '写规划', 'session-other', 4)]),
  }))
  t.after(() => server.close())
  const stub = createStubContext()
  applyBridge(stub.ctx, { socketPath: server.socketPath, boardSubscriptionStatePath: boardStatePath })
  const handler = stub.handler('agent/pre-step')
  const decision = await runPreStep(handler, { agent: agentOf('session-caller'), turn: 1, step: 1 })
  const text = boardMessageOf(decision)
  assert.match(text, /板已重建\/历史已裁剪/)
  assert.match(text, /- other：任务\.当前：写规划/)
  assert.equal(readSubscriptions(boardStatePath)['session-caller'].workspace.since, 4)
})

test('截断续传（C5）：第一次注入部分条目 + 尾注且只推进到整组边界，第二次续传剩余', async (t) => {
  const boardStatePath = join(TEST_DIR, `board-e-${process.pid}-${seq + 1}.json`)
  seedSubscriptions(boardStatePath, { 'session-caller': { workspace: { since: 0 } } })
  const deltas = []
  for (let version = 1; version <= 4; version += 1) {
    deltas.push(deltaOf(`键.${version}${'k'.repeat(55)}`, null, 'x'.repeat(68), 'session-other', version))
  }
  const { server } = await startBoardServer((url) => {
    const since = Number(url.searchParams.get('since'))
    return { ok: true, since, truncated: false, board: boardOf(4), deltas: deltas.filter((entry) => entry.version > since) }
  })
  t.after(() => server.close())
  const stub = createStubContext()
  applyBridge(stub.ctx, {
    socketPath: server.socketPath,
    boardSubscriptionStatePath: boardStatePath,
    boardSubscriptionMaxChars: 520,
  })
  const handler = stub.handler('agent/pre-step')
  const first = await runPreStep(handler, { agent: agentOf('session-caller'), turn: 1, step: 1 })
  const firstText = boardMessageOf(first)
  assert.match(firstText, /还有 1 条未显示/)
  assert.equal(readSubscriptions(boardStatePath)['session-caller'].workspace.since, 3)
  const second = await runPreStep(handler, { agent: agentOf('session-caller'), turn: 2, step: 1 })
  const secondText = boardMessageOf(second)
  assert.equal(readSubscriptions(boardStatePath)['session-caller'].workspace.since, 4)
  const merged = `${firstText}\n${secondText}`
  for (let version = 1; version <= 4; version += 1) assert.ok(merged.includes(`键.${version}`), `缺少 键.${version}`)
})

test('boardEnabled=false（K11）：不注册 astrbox_board、不注入', async (t) => {
  const { server, state } = await startBoardServer(() => ({ ok: true, board: boardOf(7) }))
  t.after(() => server.close())
  const registered = []
  const stub = createStubContext()
  stub.setService('tools', { register: (definition) => { registered.push(definition.name); return () => {} } })
  const boardStatePath = join(TEST_DIR, `board-f-${process.pid}-${seq + 1}.json`)
  seedSubscriptions(boardStatePath, { 'session-caller': { workspace: { since: 6 } } })
  applyBridge(stub.ctx, { socketPath: server.socketPath, boardEnabled: false, boardSubscriptionStatePath: boardStatePath })
  assert.ok(!registered.includes('astrbox_board'))
  const decision = await runPreStep(stub.handler('agent/pre-step'), { agent: agentOf('session-caller'), turn: 1, step: 1 })
  assert.equal(boardMessageOf(decision), '')
  assert.equal(state.boardsCalls, 0)
})

test('boardEnabled=true：注册 astrbox_board 工具（与既有三个工具并列）', async (t) => {
  const { server } = await startBoardServer(() => ({ ok: true, board: boardOf(0) }))
  t.after(() => server.close())
  const registered = []
  const stub = createStubContext()
  stub.setService('tools', { register: (definition) => { registered.push(definition.name); return () => {} } })
  applyBridge(stub.ctx, { socketPath: server.socketPath })
  assert.ok(registered.includes('astrbox_board'), registered.join(','))
})

test('控制面不可达：pre-step 不注入、不抛异常、游标不推进（下轮重试）', async (t) => {
  const boardStatePath = join(TEST_DIR, `board-h-${process.pid}-${seq + 1}.json`)
  seedSubscriptions(boardStatePath, { 'session-caller': { workspace: { since: 6 } } })
  const stub = createStubContext()
  applyBridge(stub.ctx, {
    socketPath: join(TEST_DIR, `no-such-${process.pid}-${seq}.sock`),
    boardSubscriptionStatePath: boardStatePath,
  })
  const decision = await runPreStep(stub.handler('agent/pre-step'), { agent: agentOf('session-caller'), turn: 1, step: 1 })
  assert.equal(boardMessageOf(decision), '')
  assert.equal(readSubscriptions(boardStatePath)['session-caller'].workspace.since, 6)
})

test('注入顺序（K8）：快照在前、会话黑板在后，每轮最多一条 [astrbox-board]', async (t) => {
  const boardStatePath = join(TEST_DIR, `board-i-${process.pid}-${seq + 1}.json`)
  seedSubscriptions(boardStatePath, { 'session-caller': { workspace: { since: 6 } } })
  const { server } = await startBoardServer((url) => {
    const since = Number(url.searchParams.get('since'))
    return { ok: true, since, truncated: false, board: boardOf(7), deltas: since < 7 ? [deltaOf('任务.当前', null, '写规划', 'session-other', 7)] : [] }
  })
  t.after(() => server.close())
  const stub = createStubContext()
  applyBridge(stub.ctx, { socketPath: server.socketPath, boardSubscriptionStatePath: boardStatePath })
  const decision = await runPreStep(stub.handler('agent/pre-step'), { agent: agentOf('session-caller'), turn: 1, step: 1 })
  const texts = decision.messages.map((message) => message.content[0].text)
  assert.equal(texts.filter((text) => text.startsWith('[astrbox-board]')).length, 1)
  const snapshotIndex = texts.findIndex((text) => text.startsWith('[astrbox]'))
  const boardIndex = texts.findIndex((text) => text.startsWith('[astrbox-board]'))
  assert.ok(snapshotIndex >= 0, texts.join('\n---\n'))
  assert.ok(boardIndex > snapshotIndex, texts.join('\n---\n'))
})
