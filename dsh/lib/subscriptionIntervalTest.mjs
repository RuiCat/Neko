/**
 * K17 订阅间隔覆盖存储：默认路径、加载/保存往返、损坏/缺失 fail-soft、原子保存、保存失败仅 warn。
 * 全部用临时目录，不读写用户真实 `~/.dsh-astrbox`。
 */
import assert from 'node:assert/strict'
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { homedir, tmpdir } from 'node:os'
import { join } from 'node:path'
import { test } from 'node:test'
import {
  MAX_INTERVAL_MINUTES,
  MIN_INTERVAL_MINUTES,
  createIntervalStore,
  defaultSubscriptionPath,
  isValidIntervalMinutes,
} from './subscriptionInterval.mjs'

/**
 * 建临时目录与日志收集器。
 * @param {object} t node:test 上下文
 * @returns {{dir: string, logs: string[]}} 目录与 warn 收集
 */
function tempSetup(t) {
  const dir = mkdtempSync(join(tmpdir(), 'astrbox-interval-test-'))
  t.after(() => rmSync(dir, { recursive: true, force: true }))
  const logs = []
  return { dir, logs }
}

/** 记录 warnOnce 的 logger 替身。 */
function fakeLogger(logs) {
  return { warnOnce: (key, message) => logs.push(`${key}: ${message}`) }
}

test('默认路径为 <home>/.dsh-astrbox/subscription.json；覆盖路径优先', (t) => {
  assert.equal(defaultSubscriptionPath(), join(homedir(), '.dsh-astrbox', 'subscription.json'))
  const { dir } = tempSetup(t)
  const custom = join(dir, 'sub.json')
  assert.equal(createIntervalStore({ path: custom }).path, custom)
  assert.equal(createIntervalStore({}).path, defaultSubscriptionPath())
  assert.equal(createIntervalStore({ path: '   ' }).path, defaultSubscriptionPath())
})

test('缺失文件 → 空表；set/get/reset 与磁盘往返一致（tmp 不残留）', (t) => {
  const { dir } = tempSetup(t)
  const path = join(dir, 'nested', 'subscription.json')
  const store = createIntervalStore({ path })
  assert.deepEqual(store.snapshot(), { intervals: {} })
  assert.equal(store.get('qqbot:cat:main'), undefined)

  assert.equal(store.set('qqbot:cat:main', 30), true)
  assert.equal(store.get('qqbot:cat:main'), 30)
  const saved = JSON.parse(readFileSync(path, 'utf8'))
  assert.deepEqual(saved, { intervals: { 'qqbot:cat:main': 30 } })
  assert.equal(existsSync(`${path}.tmp`), false)
  assert.equal(existsSync(path), true)

  // 重新加载（模拟重启）后仍在。
  const reloaded = createIntervalStore({ path })
  assert.equal(reloaded.get('qqbot:cat:main'), 30)

  assert.equal(reloaded.reset('qqbot:cat:main'), true)
  assert.equal(reloaded.get('qqbot:cat:main'), undefined)
  assert.equal(reloaded.reset('not-there'), true)
  assert.deepEqual(JSON.parse(readFileSync(path, 'utf8')), { intervals: {} })
})

test('非法分钟数（0/1441/小数/字符串/null）不写表', (t) => {
  const { dir } = tempSetup(t)
  const store = createIntervalStore({ path: join(dir, 'subscription.json') })
  for (const minutes of [0, MAX_INTERVAL_MINUTES + 1, 1.5, '30', null, undefined, NaN]) {
    assert.equal(store.set('qqbot:cat:main', minutes), false, String(minutes))
  }
  assert.deepEqual(store.snapshot(), { intervals: {} })
  assert.equal(MIN_INTERVAL_MINUTES, 1)
  assert.equal(MAX_INTERVAL_MINUTES, 1440)
  assert.equal(isValidIntervalMinutes(1), true)
  assert.equal(isValidIntervalMinutes(1440), true)
  assert.equal(isValidIntervalMinutes(1441), false)
})

test('损坏文件按空处理并 warn；非法条目被丢弃', (t) => {
  const { dir, logs } = tempSetup(t)
  const corruptPath = join(dir, 'corrupt.json')
  writeFileSync(corruptPath, 'not json', 'utf8')
  const corrupt = createIntervalStore({ path: corruptPath, logger: fakeLogger(logs) })
  assert.deepEqual(corrupt.snapshot(), { intervals: {} })
  assert.ok(logs.some((line) => line.includes('损坏')), logs.join(' | '))

  const partialPath = join(dir, 'partial.json')
  writeFileSync(partialPath, JSON.stringify({ intervals: { good: 5, zero: 0, big: 9999, text: 'x', float: 1.5, '': 2 } }), 'utf8')
  const partial = createIntervalStore({ path: partialPath })
  assert.deepEqual(partial.snapshot(), { intervals: { good: 5 } })
})

test('保存失败 fail-soft：只 warn、内存值生效、不抛', (t) => {
  const { dir, logs } = tempSetup(t)
  const blocker = join(dir, 'blocker')
  writeFileSync(blocker, 'file blocks directory creation', 'utf8')
  const path = join(blocker, 'subscription.json')
  const store = createIntervalStore({ path, logger: fakeLogger(logs) })
  assert.equal(store.set('qqbot:cat:main', 5), false)
  assert.equal(store.get('qqbot:cat:main'), 5, '保存失败后内存仍生效')
  assert.ok(logs.some((line) => line.includes('保存失败')), logs.join(' | '))
  assert.equal(store.reset('qqbot:cat:main'), false)
  assert.equal(store.get('qqbot:cat:main'), undefined)
})

test('snapshot 返回副本（调用方修改不影响存储）', (t) => {
  const { dir } = tempSetup(t)
  const store = createIntervalStore({ path: join(dir, 'subscription.json') })
  store.set('s1', 10)
  const snapshot = store.snapshot()
  snapshot.intervals.s1 = 999
  assert.equal(store.get('s1'), 10)
  assert.equal(store.get(''), undefined)
  assert.equal(store.get(42), undefined)
})
