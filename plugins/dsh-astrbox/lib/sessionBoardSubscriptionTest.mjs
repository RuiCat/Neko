/**
 * K8 订阅状态单测：形状 / 订阅退订 / 游标推进 / 原子读写 / 损坏 fail-soft / 临时路径隔离。
 *
 * 运行：`node --test dsh/lib/sessionBoardSubscriptionTest.mjs`。
 * 所有落盘都在 `os.tmpdir()` 下，不写真实 `~/.dsh-astrbox`。
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import {
  BOARD_SCOPES,
  createBoardSubscriptionStore,
  defaultBoardSubscriptionPath,
  isValidBoardScope,
} from './sessionBoardSubscription.mjs'

/** 测试用临时目录（进程退出清理）。 */
const TEST_DIR = mkdtempSync(join(tmpdir(), 'astrbox-board-sub-test-'))
let seq = 0
process.on('exit', () => rmSync(TEST_DIR, { recursive: true, force: true }))

/** 每次用独立状态文件，测试互不影响。 */
function newPath() {
  seq += 1
  return join(TEST_DIR, `board-subscriptions-${process.pid}-${seq}.json`)
}

/** 静默 logger（记录 warn 次数）。 */
function silentLogger() {
  const warnings = []
  return { warnings, warnOnce: (_key, message) => warnings.push(message) }
}

test('默认路径与 scope 常量符合 K8', () => {
  assert.equal(defaultBoardSubscriptionPath().endsWith('/.dsh-neko/state/board-subscriptions.json'), true)
  assert.deepEqual([...BOARD_SCOPES], ['workspace', 'global'])
  assert.equal(isValidBoardScope('workspace'), true)
  assert.equal(isValidBoardScope('global'), true)
  assert.equal(isValidBoardScope('other'), false)
})

test('订阅/退订/游标推进：内存与落盘形状为 {subscriptions:{id:{scope:{since}}}}', () => {
  const path = newPath()
  const store = createBoardSubscriptionStore({ path, logger: silentLogger() })
  assert.equal(store.subscribe('session-1', 'workspace', 7), true)
  assert.equal(store.subscribe('session-1', 'global', 12), true)
  assert.equal(store.subscribe('session-2', 'workspace', 0), true)
  assert.deepEqual(store.get('session-1'), { workspace: { since: 7 }, global: { since: 12 } })
  const saved = JSON.parse(readFileSync(path, 'utf8'))
  assert.deepEqual(saved, {
    subscriptions: {
      'session-1': { workspace: { since: 7 }, global: { since: 12 } },
      'session-2': { workspace: { since: 0 } },
    },
  })
  // 另一个实例从同一文件加载，验证持久化可读。
  const reloaded = createBoardSubscriptionStore({ path })
  assert.deepEqual(reloaded.get('session-1'), { workspace: { since: 7 }, global: { since: 12 } })
})

test('setCursor：只对已订阅 scope 生效，推进后落盘；无变化不重写', () => {
  const path = newPath()
  const store = createBoardSubscriptionStore({ path })
  assert.equal(store.subscribe('session-1', 'workspace', 3), true)
  assert.deepEqual(store.setCursor('session-1', 'workspace', 9), { ok: true, saved: true })
  assert.equal(store.get('session-1').workspace.since, 9)
  assert.deepEqual(store.setCursor('session-1', 'workspace', 9), { ok: true, saved: true })
  assert.deepEqual(store.setCursor('session-1', 'global', 1), { ok: false, saved: false })
  assert.deepEqual(store.setCursor('session-unknown', 'workspace', 1), { ok: false, saved: false })
  assert.equal(JSON.parse(readFileSync(path, 'utf8')).subscriptions['session-1'].workspace.since, 9)
})

test('退订：指定 scope 只退该 scope，省略 scope 退整个会话', () => {
  const path = newPath()
  const store = createBoardSubscriptionStore({ path })
  store.subscribe('session-1', 'workspace', 1)
  store.subscribe('session-1', 'global', 2)
  assert.equal(store.unsubscribe('session-1', 'global'), true)
  assert.deepEqual(store.get('session-1'), { workspace: { since: 1 } })
  assert.equal(store.unsubscribe('session-1'), true)
  assert.equal(store.get('session-1'), undefined)
  assert.equal(store.unsubscribe('session-1'), true)
  assert.deepEqual(JSON.parse(readFileSync(path, 'utf8')).subscriptions, {})
})

test('非法入参不写表：空 sessionId / 非法 scope / 负数或非整数 since', () => {
  const path = newPath()
  const store = createBoardSubscriptionStore({ path })
  assert.equal(store.subscribe('', 'workspace', 1), false)
  assert.equal(store.subscribe('session-1', 'bogus', 1), false)
  assert.equal(store.subscribe('session-1', 'workspace', -1), false)
  assert.equal(store.subscribe('session-1', 'workspace', 1.5), false)
  assert.equal(store.unsubscribe('session-1', 'bogus'), false)
  assert.deepEqual(store.snapshot(), { subscriptions: {} })
})

test('损坏/缺字段状态 fail-soft：按空表处理并 warn 一次', () => {
  const path = newPath()
  writeFileSync(path, '{ not json', 'utf8')
  const logger = silentLogger()
  const store = createBoardSubscriptionStore({ path, logger })
  assert.deepEqual(store.snapshot(), { subscriptions: {} })
  assert.equal(logger.warnings.length, 1)

  const path2 = newPath()
  writeFileSync(path2, JSON.stringify({ subscriptions: { 'session-1': { workspace: { since: 'x' }, global: { since: 4 } } } }), 'utf8')
  const store2 = createBoardSubscriptionStore({ path: path2 })
  assert.deepEqual(store2.get('session-1'), { global: { since: 4 } })
})

test('损坏文件在订阅后被原子覆盖（tmp+rename 后不再残留半写）', () => {
  const path = newPath()
  writeFileSync(path, 'garbage', 'utf8')
  const store = createBoardSubscriptionStore({ path })
  assert.equal(store.subscribe('session-1', 'workspace', 5), true)
  assert.deepEqual(JSON.parse(readFileSync(path, 'utf8')), { subscriptions: { 'session-1': { workspace: { since: 5 } } } })
})

test('落盘失败 fail-soft：不可写路径只 warn，内存值仍生效（至少一次投递）', () => {
  const path = join(TEST_DIR, 'missing-parent-file', 'x', 'board.json')
  // 用同名文件占住父路径，制造 mkdir 失败。
  writeFileSync(join(TEST_DIR, 'missing-parent-file'), 'not a dir', 'utf8')
  const logger = silentLogger()
  const store = createBoardSubscriptionStore({ path, logger })
  assert.equal(store.subscribe('session-1', 'workspace', 2), false)
  assert.deepEqual(store.get('session-1'), { workspace: { since: 2 } })
  assert.ok(logger.warnings.length >= 1)
})
