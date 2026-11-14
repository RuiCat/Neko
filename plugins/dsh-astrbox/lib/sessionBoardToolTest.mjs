/**
 * K7/K10 `astrbox_board` 工具单测：身份只从 exec.agent / no_workspace / 控制面不可达不抛异常 /
 * read-write-subscribe-unsubscribe 参数形状 / 异 cwd 隔离 / schema 无身份字段。
 *
 * 用 `testHarness.mjs` 的临时 UDS 服务做伪控制面，不依赖 Go 服务；运行：
 * `node --test dsh/lib/sessionBoardToolTest.mjs`。
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { mkdtempSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { createSessionBoardTool, renderBoardResult } from './sessionBoardTool.mjs'
import { createBoardSubscriptionStore } from './sessionBoardSubscription.mjs'
import { createControlClient } from './controlClient.mjs'
import { startControlServer, writeJson } from './testHarness.mjs'

/** 订阅状态临时路径（不写真实 ~/.dsh-astrbox）。 */
const TEST_DIR = mkdtempSync(join(tmpdir(), 'astrbox-board-tool-test-'))
process.on('exit', () => rmSync(TEST_DIR, { recursive: true, force: true }))
let seq = 0

/** 一条 test exec：会话 id + cwd（K7 身份来源）。 */
function makeExec(sessionId, cwd) {
  return { agent: { session: { id: sessionId, header: { cwd } } } }
}

/** 板夹具：workspace 板 version 7，global 板 version 12。 */
function boardFixture(scope, version) {
  return {
    scope,
    scope_id: scope === 'workspace' ? '/repo' : '*',
    version,
    updated_at: '2026-11-10T12:50:00+08:00',
    min_retained_version: 0,
    regions: scope === 'workspace'
      ? [{
        id: 'session-other',
        owner: { kind: 'session', id: 'session-other' },
        version,
        updated_at: '2026-11-10T12:50:00+08:00',
        entries: [{ key: '任务.当前', from: '空闲', to: '写规划', by: 'session-other', version, deleted: false }],
      }]
      : [{
        id: '/other/workspace',
        owner: { kind: 'workspace', id: '/other/workspace' },
        version,
        updated_at: '2026-11-10T12:50:00+08:00',
        entries: [{ key: '项目.状态', from: null, to: '进行中', by: 'session-other', version, deleted: false }],
      }],
  }
}

/**
 * 起伪控制面 + 工具。
 * @returns {Promise<object>} 夹具
 */
async function startFixture() {
  seq += 1
  const control = {
    boardMode: 'ok',
    writeMode: 'ok',
    writeBody: null,
  }
  const server = await startControlServer((req, res, record) => {
    if (req.url.startsWith('/boards') && req.method === 'GET') {
      if (control.boardMode === 'forbidden') {
        writeJson(res, 403, { ok: false, error: { code: 'forbidden', message: 'workspace mismatch' } })
        return
      }
      if (control.boardMode === 'fail') {
        writeJson(res, 500, { ok: false, error: { code: 'internal', message: 'board boom' } })
        return
      }
      const url = new URL(req.url, 'http://localhost')
      const scope = url.searchParams.get('scope')
      const since = url.searchParams.get('since')
      const board = boardFixture(scope, scope === 'global' ? 12 : 7)
      if (since === null) {
        writeJson(res, 200, { ok: true, board })
        return
      }
      const deltas = [{
        region_id: scope === 'global' ? '/other/workspace' : 'session-other',
        key: scope === 'global' ? '项目.状态' : '任务.当前',
        from: scope === 'global' ? null : '空闲',
        to: scope === 'global' ? '进行中' : '写规划',
        by: 'session-other',
        version: board.version,
        deleted: false,
      }]
      writeJson(res, 200, { ok: true, since: Number(since), truncated: false, board, deltas })
      return
    }
    if (req.url === '/boards' && req.method === 'POST') {
      control.writeBody = record.body
      if (control.writeMode === 'bad_request') {
        writeJson(res, 400, { ok: false, error: { code: 'bad_request', message: 'invalid_ops: ops 不能为空' } })
        return
      }
      writeJson(res, 200, { ok: true, changed: true, version: 8, updated_at: '2026-11-10T12:51:00+08:00', trimmed: [], evicted: [] })
      return
    }
    writeJson(res, 404, { ok: false, error: { code: 'not_found', message: req.url } })
  })
  const store = createBoardSubscriptionStore({ path: join(TEST_DIR, `sub-${seq}.json`) })
  const client = createControlClient({ socketPath: server.socketPath, requestTimeoutMs: 2000 })
  const bindings = [{ botId: 'bot1', cwd: '/home/ruicat/Desktop/Neko', entity: '猫猫', label: 'Neko' }]
  const tool = createSessionBoardTool({ client, store, getBindings: () => bindings })
  return { server, control, store, client, tool, requests: server.requests }
}

test('工具 schema 无身份字段（K7/C8）', () => {
  const tool = createSessionBoardTool({ client: {}, store: {} })
  const properties = tool.definition.parameters.properties
  assert.equal(properties.session, undefined)
  assert.equal(properties.workspace, undefined)
  assert.deepEqual(tool.definition.parameters.required, ['op'])
})

test('身份只从 exec.agent：read 的 workspace/session 参数来自 cwd 与会话 id（K7）', async (t) => {
  const fixture = await startFixture()
  t.after(() => fixture.server.close())
  const result = await fixture.tool.execute(
    { op: 'read', scope: 'workspace', since: 6 },
    makeExec('session-caller', '/home/ruicat/Desktop/Neko'),
  )
  assert.equal(result.ok, true)
  assert.equal(result.version, 7)
  assert.equal(result.count, 1)
  assert.equal(result.entries[0].key, '任务.当前')
  // K4 契约字段 region_id：差异条目必须能带出区域（真机实测曾读到空串）。
  assert.equal(result.entries[0].region, 'session-other')
  const request = fixture.requests.find((item) => item.method === 'GET')
  assert.match(request.url, /scope=workspace/)
  assert.match(request.url, /workspace=%2Fhome%2Fruicat%2FDesktop%2FNeko/)
  assert.match(request.url, /since=6/)
})

test('异 cwd：两个会话请求各自工作区，互不串（C11 桥接侧）', async (t) => {
  const fixture = await startFixture()
  t.after(() => fixture.server.close())
  await fixture.tool.execute({ op: 'read', scope: 'workspace' }, makeExec('session-a', '/repo-a/nested/'))
  await fixture.tool.execute({ op: 'read', scope: 'workspace' }, makeExec('session-b', '/repo-b'))
  const workspaceParams = fixture.requests
    .filter((item) => item.method === 'GET')
    .map((item) => new URL(item.url, 'http://localhost').searchParams.get('workspace'))
  assert.deepEqual(workspaceParams, ['/repo-a/nested', '/repo-b'])
})

test('no_workspace：缺 sessionId 或 cwd → {ok:false, reason:"no_workspace"} 且不发请求', async (t) => {
  const fixture = await startFixture()
  t.after(() => fixture.server.close())
  const before = fixture.requests.length
  const noCwd = await fixture.tool.execute({ op: 'read', scope: 'workspace' }, { agent: { session: { id: 'session-x' } } })
  assert.deepEqual(noCwd, { ok: false, reason: 'no_workspace' })
  const noSession = await fixture.tool.execute({ op: 'read', scope: 'workspace' }, { agent: { session: { header: { cwd: '/repo' } } } })
  assert.deepEqual(noSession, { ok: false, reason: 'no_workspace' })
  assert.equal(fixture.requests.length, before)
})

test('控制面不可达：返回 control_unavailable，不抛异常', async () => {
  seq += 1
  const client = createControlClient({ socketPath: join(TEST_DIR, `no-such-${process.pid}-${seq}.sock`), requestTimeoutMs: 200 })
  const store = createBoardSubscriptionStore({ path: join(TEST_DIR, `sub-unreachable-${seq}.json`) })
  const tool = createSessionBoardTool({ client, store })
  const result = await tool.execute({ op: 'read', scope: 'workspace' }, makeExec('session-x', '/repo'))
  assert.equal(result.ok, false)
  assert.equal(result.reason, 'control_unavailable')
})

test('控制面 403 → forbidden（词表 K9/K10）', async (t) => {
  const fixture = await startFixture()
  t.after(() => fixture.server.close())
  fixture.control.boardMode = 'forbidden'
  const result = await fixture.tool.execute({ op: 'read', scope: 'global' }, makeExec('session-x', '/repo'))
  assert.deepEqual(result, { ok: false, reason: 'forbidden' })
})

test('write：session/workspace 由桥接填写、ops 来自 set/delete/clear（K5/K7）', async (t) => {
  const fixture = await startFixture()
  t.after(() => fixture.server.close())
  const result = await fixture.tool.execute(
    { op: 'write', scope: 'workspace', set: { '任务.当前': '写规划' }, delete: ['旧键'] },
    makeExec('session-caller', '/repo'),
  )
  assert.equal(result.ok, true)
  assert.equal(result.changed, true)
  assert.equal(result.version, 8)
  assert.deepEqual(fixture.control.writeBody, {
    scope: 'workspace',
    workspace: '/repo',
    session: 'session-caller',
    ops: [{ op: 'set', key: '任务.当前', value: '写规划' }, { op: 'delete', key: '旧键' }],
  })
  const rendered = fixture.tool.definition.output.render({}, result)
  assert.match(rendered[0].text, /写入工作区黑板：已更新，版本 8/)
})

test('write global：workspace 必须随请求发送（K5；Go global 板据此推导本工作区的区）', async (t) => {
  const fixture = await startFixture()
  t.after(() => fixture.server.close())
  const result = await fixture.tool.execute(
    { op: 'write', scope: 'global', set: { '项目.状态': '进行中' } },
    makeExec('session-caller', '/repo'),
  )
  assert.equal(result.ok, true)
  assert.deepEqual(fixture.control.writeBody, {
    scope: 'global',
    workspace: '/repo',
    session: 'session-caller',
    ops: [{ op: 'set', key: '项目.状态', value: '进行中' }],
  })
})

test('控制面 400 bad_request → invalid_args（K10 词表；不再误报 bad_response）', async (t) => {
  const fixture = await startFixture()
  t.after(() => fixture.server.close())
  fixture.control.writeMode = 'bad_request'
  const result = await fixture.tool.execute(
    { op: 'write', scope: 'global', set: { a: 1 } },
    makeExec('session-x', '/repo'),
  )
  assert.deepEqual(result, { ok: false, reason: 'invalid_args' })
})

test('write 参数校验：空批 / clear 混用 / 非法 key / 超量 → invalid_args，不发请求', async (t) => {
  const fixture = await startFixture()
  t.after(() => fixture.server.close())
  const cases = [
    { op: 'write', scope: 'workspace' },
    { op: 'write', scope: 'workspace', clear: true, set: { a: 1 } },
    { op: 'write', scope: 'workspace', set: { 'bad\nkey': 1 } },
    { op: 'write', scope: 'workspace', delete: [7] },
    { op: 'write', scope: 'workspace', set: Object.fromEntries(Array.from({ length: 17 }, (_, index) => [`k${index}`, index])) },
    { op: 'write', scope: 'workspace', set: { big: 'x'.repeat(3000) } },
  ]
  for (const args of cases) {
    const result = await fixture.tool.execute(args, makeExec('session-caller', '/repo'))
    assert.deepEqual(result, { ok: false, reason: 'invalid_args' }, JSON.stringify(args))
  }
  assert.equal(fixture.requests.filter((item) => item.method === 'POST').length, 0)
})

test('subscribe：游标初值 = 当前板版本（不灌历史），subscriptions 可读，unsubscribe 清除', async (t) => {
  const fixture = await startFixture()
  t.after(() => fixture.server.close())
  const exec = makeExec('session-caller', '/repo')
  const subscribed = await fixture.tool.execute({ op: 'subscribe', scope: 'workspace' }, exec)
  assert.equal(subscribed.ok, true)
  assert.equal(subscribed.since, 7)
  assert.deepEqual(fixture.store.get('session-caller'), { workspace: { since: 7 } })

  const listed = await fixture.tool.execute({ op: 'subscriptions' }, exec)
  assert.deepEqual(listed.subscriptions, [{ scope: 'workspace', since: 7 }])
  assert.match(fixture.tool.definition.output.render({}, listed)[0].text, /工作区\(since 7\)/)

  const removed = await fixture.tool.execute({ op: 'unsubscribe', scope: 'workspace' }, exec)
  assert.equal(removed.ok, true)
  assert.equal(fixture.store.get('session-caller'), undefined)
})

test('read 不带 since：返回 live 视图条目；带 since：返回差异条目', async (t) => {
  const fixture = await startFixture()
  t.after(() => fixture.server.close())
  const exec = makeExec('session-caller', '/repo')
  const live = await fixture.tool.execute({ op: 'read', scope: 'global' }, exec)
  assert.equal(live.since, undefined)
  assert.equal(live.count, 1)
  assert.equal(live.entries[0].to, '进行中')
  const delta = await fixture.tool.execute({ op: 'read', scope: 'global', since: 11 }, exec)
  assert.equal(delta.since, 11)
  assert.equal(delta.count, 1)
  assert.equal(delta.version, 12)
})

// 读板/差异的渲染必须带内容，而不只是条数——用户实际读到的就是这段文本。
test('read 渲染：live 视图与差异都带 key/value 内容', async (t) => {
  const fixture = await startFixture()
  t.after(() => fixture.server.close())
  const exec = makeExec('session-caller', '/repo')

  const live = await fixture.tool.execute({ op: 'read', scope: 'global' }, exec)
  const liveText = fixture.tool.definition.output.render({}, live)[0].text
  assert.match(liveText, /读取跨工作区黑板：版本 12，条目 1 条/)
  assert.match(liveText, /- \[\/other\/workspace\] 项目\.状态：null → 进行中/)

  const delta = await fixture.tool.execute({ op: 'read', scope: 'global', since: 11 }, exec)
  const deltaText = fixture.tool.definition.output.render({}, delta)[0].text
  assert.match(deltaText, /读取跨工作区黑板：版本 12，差异 1 条/)
  assert.match(deltaText, /- \[\/other\/workspace\] 项目\.状态：null → 进行中/)
})

test('read 渲染：超过行数上限只报剩余条数', () => {
  const entries = Array.from({ length: 35 }, (_, index) => ({
    region: '', key: `键${index}`, from: null, to: `值${index}`, by: 's', version: 1,
  }))
  const text = renderBoardResult({ ok: true, op: 'read', scope: 'workspace', version: 9, count: 35, entries })
  const lines = text.split('\n')
  assert.equal(lines[0], '读取工作区黑板：版本 9，条目 35 条')
  assert.equal(lines.length, 1 + 30 + 1)
  assert.equal(lines[31], '…（还有 5 条）')
})

test('write 渲染：带上本次写入的键', async (t) => {
  const fixture = await startFixture()
  t.after(() => fixture.server.close())
  const result = await fixture.tool.execute(
    { op: 'write', scope: 'workspace', set: { '任务.当前': '写规划' }, delete: ['旧键'] },
    makeExec('session-caller', '/repo'),
  )
  const text = fixture.tool.definition.output.render({}, result)[0].text
  assert.match(text, /键 任务\.当前、旧键/)
})
