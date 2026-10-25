/**
 * K1 客户端：node:http + socketPath（UDS）；/state 查询、/tick 409 跳过、超时、dispose 中止在飞请求。
 * 全部用临时目录里的自建 UDS 服务，不依赖真实 Go 服务。
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { ControlError, createControlClient } from './controlClient.mjs'
import { nextMacroTask, startControlServer, waitFor, writeJson } from './testHarness.mjs'

test('/state 使用 socketPath 发出实体与重复 path 参数', async (t) => {
  const server = await startControlServer((req, res) => {
    writeJson(res, 200, { ok: true, entity: '猫猫', values: { '状态.情绪': '开心' }, missing: [], bytes: 10 })
  })
  t.after(() => server.close())
  const client = createControlClient({ socketPath: server.socketPath, requestTimeoutMs: 2000 })
  t.after(() => client.dispose())

  const state = await client.getState({ entity: '猫猫', paths: ['状态.情绪', '任务.当前任务'] })
  assert.equal(state.entity, '猫猫')
  assert.equal(server.requests.length, 1)
  const record = server.requests[0]
  assert.equal(record.method, 'GET')
  const url = new URL(record.url, 'http://localhost')
  assert.equal(url.pathname, '/state')
  assert.equal(url.searchParams.get('entity'), '猫猫')
  assert.deepEqual(url.searchParams.getAll('path'), ['状态.情绪', '任务.当前任务'])
})

test('/tick 200 与 409 跳过语义', async (t) => {
  let status = 200
  const server = await startControlServer((req, res) => {
    if (status === 409) {
      writeJson(res, 409, { ok: false, error: { code: 'tick_busy', message: 'busy' } })
      return
    }
    writeJson(res, 200, { ok: true, tickId: 't1', status: 'ok', transitions: 0, conflicts: 0, errors: [] })
  })
  t.after(() => server.close())
  const client = createControlClient({ socketPath: server.socketPath, requestTimeoutMs: 2000, tickTimeoutMs: 15000 })
  t.after(() => client.dispose())

  const ok = await client.postTick({ scope: '' })
  assert.equal(ok.status, 200)
  assert.equal(ok.busy, false)
  assert.deepEqual(server.requests[0].body, { scope: '' })

  status = 409
  const busy = await client.postTick({ scope: '实体.猫猫' })
  assert.equal(busy.status, 409)
  assert.equal(busy.busy, true)
  assert.deepEqual(server.requests[1].body, { scope: '实体.猫猫' })
})

test('非 200 响应映射为带 K1 错误码的 ControlError', async (t) => {
  const server = await startControlServer((req, res) => {
    writeJson(res, 500, { ok: false, error: { code: 'internal', message: '保存失败' } })
  })
  t.after(() => server.close())
  const client = createControlClient({ socketPath: server.socketPath, requestTimeoutMs: 2000 })
  t.after(() => client.dispose())

  await assert.rejects(
    () => client.postSessionEvent({ kind: 'session-created', sessionId: 's1', cwd: '/tmp/x' }),
    (error) => error instanceof ControlError && error.code === 'internal' && error.message.includes('保存失败'),
  )
})

test('/state 超时使用 requestTimeoutMs', async (t) => {
  const server = await startControlServer(() => {
    // 故意不响应：等待客户端超时
  })
  t.after(() => server.close())
  const client = createControlClient({ socketPath: server.socketPath, requestTimeoutMs: 40 })
  t.after(() => client.dispose())

  await assert.rejects(
    () => client.getState({ entity: '猫猫', paths: [] }),
    (error) => error instanceof ControlError && error.code === 'timeout',
  )
})

test('dispose 中止在飞请求并拒绝后续请求', async (t) => {
  const server = await startControlServer(() => {
    // 故意不响应：等待 dispose
  })
  t.after(() => server.close())
  const client = createControlClient({ socketPath: server.socketPath, requestTimeoutMs: 5000 })

  const pending = client.getState({ entity: '猫猫', paths: [] })
  await waitFor(() => server.requests.length === 1)
  client.dispose()
  await assert.rejects(pending, (error) => error instanceof ControlError && error.code === 'disposed')
  await assert.rejects(
    () => client.getState({ entity: '猫猫', paths: [] }),
    (error) => error instanceof ControlError && error.code === 'disposed',
  )
  assert.equal(client.disposed, true)
  await nextMacroTask()
})

test('非 JSON 响应报 bad_response', async (t) => {
  const server = await startControlServer((req, res) => {
    res.statusCode = 200
    res.end('not json')
  })
  t.after(() => server.close())
  const client = createControlClient({ socketPath: server.socketPath, requestTimeoutMs: 2000 })
  t.after(() => client.dispose())

  await assert.rejects(
    () => client.getState({ entity: '猫猫', paths: [] }),
    (error) => error instanceof ControlError && error.code === 'bad_response',
  )
})
