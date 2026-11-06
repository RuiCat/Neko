/**
 * K17 `astrbox_subscription` 工具：定义形状、读取/设置/重置、1..1440 边界、非主会话、
 * 持久化后重建生效、保存失败 fail-soft。
 */
import assert from 'node:assert/strict'
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { test } from 'node:test'
import { createIntervalStore } from './subscriptionInterval.mjs'
import { SUBSCRIPTION_TOOL_NAME, createSubscriptionTool } from './subscriptionTool.mjs'

/** K1 形状的两条绑定（两个 bot 各一个主会话）。 */
const BINDINGS = [
  { botId: 'cat', label: 'cat', cwd: '/srv/bots/cat', entity: '梦梦', mainSessionId: 'qqbot:cat:main' },
  { botId: 'helper', label: 'helper', cwd: '/srv/bots/helper', entity: '猫猫', mainSessionId: 'qqbot:helper:main' },
]

/**
 * 建临时目录 + store + 工具。
 * @param {object} t node:test 上下文
 * @param {{path?: string, logger?: object, defaultIntervalMs?: number}} [options]
 * @returns {{tool: object, store: object, path: string, execOf: Function}} 依赖
 */
function setup(t, options = {}) {
  const dir = mkdtempSync(join(tmpdir(), 'astrbox-sub-tool-test-'))
  t.after(() => rmSync(dir, { recursive: true, force: true }))
  const path = options.path ?? join(dir, 'subscription.json')
  const store = createIntervalStore({ path, logger: options.logger })
  const tool = createSubscriptionTool({
    store,
    defaultIntervalMs: options.defaultIntervalMs ?? 600000,
    getBindings: () => BINDINGS,
    logger: options.logger,
  })
  return { tool, store, path, execOf }
}

/**
 * 造执行上下文（agent.session.id 即调用方会话）。
 * @param {string} sessionId 会话 id
 * @returns {{agent: object}} exec
 */
function execOf(sessionId) {
  return { agent: { id: sessionId, session: { id: sessionId, header: { cwd: '/srv/bots/cat' } } } }
}

test('工具定义：名称/参数/输出 schema/render（K17）', () => {
  const { tool } = setup({ after: () => {} })
  const definition = tool.definition
  assert.equal(definition.name, 'astrbox_subscription')
  assert.equal(SUBSCRIPTION_TOOL_NAME, 'astrbox_subscription')
  assert.equal(definition.parameters.properties.minutes.type, 'integer')
  assert.equal(definition.parameters.properties.minutes.minimum, 1)
  assert.equal(definition.parameters.properties.minutes.maximum, 1440)
  assert.equal(definition.parameters.properties.reset.type, 'boolean')
  assert.deepEqual(definition.parameters.required ?? [], [])
  assert.equal(definition.output.schema.required[0], 'ok')
  const rendered = definition.output.render({}, { ok: true, minutes: 5, intervalMs: 300000, source: 'override' })
  assert.ok(rendered[0].text.includes('5 分钟') && rendered[0].text.includes('自定义'), rendered[0].text)
  assert.ok(definition.output.render({}, { ok: false, reason: 'not_main_session' })[0].text.includes('not_main_session'))
})

test('主会话无参读取 → 全局默认（10 分钟）；子会话 → not_main_session', async (t) => {
  const { tool } = setup(t)
  assert.deepEqual(await tool.execute(undefined, execOf('qqbot:cat:main')), {
    ok: true,
    minutes: 10,
    intervalMs: 600000,
    sessionId: 'qqbot:cat:main',
    source: 'default',
  })
  assert.deepEqual(await tool.execute({}, execOf('qqbot:helper:main')), {
    ok: true,
    minutes: 10,
    intervalMs: 600000,
    sessionId: 'qqbot:helper:main',
    source: 'default',
  })
  // 子会话读/写都拒绝（简单语义）。
  assert.deepEqual(await tool.execute({}, execOf('qqbot:cat:group:g1')), { ok: false, reason: 'not_main_session' })
  assert.deepEqual(await tool.execute({ minutes: 5 }, execOf('unknown-session')), { ok: false, reason: 'not_main_session' })
})

test('设置/读取/重置：1..1440 边界、source 与 intervalMs', async (t) => {
  const { tool, store } = setup(t)
  const main = execOf('qqbot:cat:main')

  const set = await tool.execute({ minutes: 1 }, main)
  assert.deepEqual(set, { ok: true, minutes: 1, intervalMs: 60000, sessionId: 'qqbot:cat:main', source: 'override' })
  assert.equal(store.get('qqbot:cat:main'), 1)

  const read = await tool.execute({}, main)
  assert.deepEqual(read, { ok: true, minutes: 1, intervalMs: 60000, sessionId: 'qqbot:cat:main', source: 'override' })

  const max = await tool.execute({ minutes: 1440 }, main)
  assert.equal(max.minutes, 1440)
  assert.equal(max.intervalMs, 1440 * 60000)

  const reset = await tool.execute({ reset: true }, main)
  assert.deepEqual(reset, { ok: true, minutes: 10, intervalMs: 600000, sessionId: 'qqbot:cat:main', source: 'default' })
  assert.equal(store.get('qqbot:cat:main'), undefined)

  // 无覆盖时 reset 仍 ok。
  assert.equal((await tool.execute({ reset: true }, main)).source, 'default')
})

test('非法 minutes → invalid_minutes 且不改表', async (t) => {
  const { tool, store } = setup(t)
  const main = execOf('qqbot:cat:main')
  for (const minutes of [0, 1441, 1.5, '30', null, NaN]) {
    assert.deepEqual(await tool.execute({ minutes }, main), { ok: false, reason: 'invalid_minutes' }, String(minutes))
  }
  assert.equal(store.get('qqbot:cat:main'), undefined)
})

test('持久化：写入磁盘后新 store/新工具仍生效（重启往返）', async (t) => {
  const { tool, path } = setup(t)
  await tool.execute({ minutes: 7 }, execOf('qqbot:cat:main'))
  const saved = JSON.parse(readFileSync(path, 'utf8'))
  assert.deepEqual(saved, { intervals: { 'qqbot:cat:main': 7 } })

  const rebuiltStore = createIntervalStore({ path })
  const rebuilt = createSubscriptionTool({ store: rebuiltStore, defaultIntervalMs: 600000, getBindings: () => BINDINGS })
  assert.deepEqual(await rebuilt.execute({}, execOf('qqbot:cat:main')), {
    ok: true,
    minutes: 7,
    intervalMs: 420000,
    sessionId: 'qqbot:cat:main',
    source: 'override',
  })
})

test('保存失败 fail-soft：工具仍返回 ok 且内存覆盖生效', async (t) => {
  const dir = mkdtempSync(join(tmpdir(), 'astrbox-sub-tool-fail-'))
  t.after(() => rmSync(dir, { recursive: true, force: true }))
  const blocker = join(dir, 'blocker')
  writeFileSync(blocker, 'x', 'utf8')
  const logs = []
  const { tool, store } = setup(t, {
    path: join(blocker, 'subscription.json'),
    logger: { warnOnce: (key, message) => logs.push(`${key}: ${message}`) },
  })
  const result = await tool.execute({ minutes: 5 }, execOf('qqbot:cat:main'))
  assert.equal(result.ok, true)
  assert.equal(result.source, 'override')
  assert.equal(store.get('qqbot:cat:main'), 5)
  assert.ok(logs.some((line) => line.includes('保存失败')), logs.join(' | '))
})
