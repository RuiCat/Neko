/**
 * C11：配置校验（未知键 / 类型错 / 负数 / 路径无分隔符 → 抛错并指名键）
 * 与 K2 默认值（8 条非 R18 路径）。
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { DEFAULT_PATHS, REPO_ROOT, normalizeConfig } from './config.mjs'

const R18_TERMS = ['性行为', '器官状态', '外翻', '生殖', '体液追踪', '肉棒']

test('默认配置与 K2 冻结值一致', () => {
  const config = normalizeConfig()
  assert.equal(config.socketPath, `${REPO_ROOT}/data/dsh-control.sock`)
  assert.equal(config.entity, '猫猫')
  assert.deepEqual([...config.paths], [...DEFAULT_PATHS])
  assert.equal(config.promptMaxChars, 1200)
  assert.equal(config.promptRefreshMs, 5000)
  assert.equal(config.tickEveryMs, 60000)
  assert.equal(config.tickScope, '')
  assert.equal(config.workspacePrefix, REPO_ROOT)
  assert.equal(config.functionName, 'dsh_on_event')
  assert.equal(config.requestTimeoutMs, 2000)
  assert.equal(config.tickTimeoutMs, 15000)
  assert.equal(config.warningAfterMs, 30000)
  assert.ok(Object.isFrozen(config))
})

test('默认 paths 为 8 条且不含 R18 字段', () => {
  assert.equal(DEFAULT_PATHS.length, 8)
  assert.equal(normalizeConfig().paths.length, 8)
  for (const path of DEFAULT_PATHS) {
    for (const term of R18_TERMS) assert.ok(!path.includes(term), `${path} 不应含 ${term}`)
  }
})

test('未知键抛错并指名键', () => {
  assert.throws(() => normalizeConfig({ promptMaxChar: 1200 }), /invalid config key promptMaxChar: unknown key/)
})

test('类型错逐键抛错并指名键', () => {
  const cases = [
    [{ socketPath: 42 }, 'socketPath'],
    [{ entity: '' }, 'entity'],
    [{ entity: 3 }, 'entity'],
    [{ paths: '状态.情绪' }, 'paths'],
    [{ paths: ['状态.情绪', 7] }, 'paths[1]'],
    [{ promptMaxChars: 1.5 }, 'promptMaxChars'],
    [{ promptRefreshMs: -1 }, 'promptRefreshMs'],
    [{ tickEveryMs: -5 }, 'tickEveryMs'],
    [{ tickScope: null }, 'tickScope'],
    [{ functionName: 3 }, 'functionName'],
    [{ requestTimeoutMs: 0 }, 'requestTimeoutMs'],
    [{ tickTimeoutMs: 0 }, 'tickTimeoutMs'],
    [{ warningAfterMs: -1 }, 'warningAfterMs'],
    [{ workspacePrefix: 'no-separator' }, 'workspacePrefix'],
    [{ socketPath: 'dsh-control.sock' }, 'socketPath'],
  ]
  for (const [raw, key] of cases) {
    assert.throws(
      () => normalizeConfig(raw),
      (error) => error instanceof Error && error.message.includes(`invalid config key ${key}:`),
      JSON.stringify(raw),
    )
  }
})

test('tickEveryMs=0 与 promptRefreshMs=0 合法', () => {
  const config = normalizeConfig({ tickEveryMs: 0, promptRefreshMs: 0, tickScope: '实体.猫猫' })
  assert.equal(config.tickEveryMs, 0)
  assert.equal(config.promptRefreshMs, 0)
  assert.equal(config.tickScope, '实体.猫猫')
})

test('自定义合法配置被接受且 paths 被复制', () => {
  const paths = ['状态.情绪']
  const config = normalizeConfig({ paths, promptMaxChars: 300, entity: '测试猫' })
  assert.deepEqual([...config.paths], paths)
  assert.equal(config.promptMaxChars, 300)
  assert.equal(config.entity, '测试猫')
  // 必须复制数组：调用方后续改动入参不得影响已规范化的配置。
  assert.notEqual(config.paths, paths)
  assert.ok(Object.isFrozen(config.paths))
})

test('世界时钟（K12）：新配置键的默认值与校验', () => {
  const defaults = normalizeConfig({})
  assert.equal(defaults.clockStepMinutes, 10)
  assert.equal(defaults.clockIdleStepMinutes, 1)
  assert.equal(defaults.clockActiveWindowMs, 600000)

  const custom = normalizeConfig({ clockStepMinutes: 0, clockIdleStepMinutes: 2, clockActiveWindowMs: 0 })
  assert.equal(custom.clockStepMinutes, 0)
  assert.equal(custom.clockIdleStepMinutes, 2)
  assert.equal(custom.clockActiveWindowMs, 0)

  for (const key of ['clockStepMinutes', 'clockIdleStepMinutes', 'clockActiveWindowMs']) {
    assert.throws(
      () => normalizeConfig({ [key]: -1 }),
      (error) => error instanceof Error && error.message.includes(`invalid config key ${key}:`),
      key,
    )
  }
})

test('事件黑板订阅（K4）：3 个新键的默认值、合法覆盖与非法值', () => {
  const defaults = normalizeConfig({})
  assert.equal(defaults.subscriptionEnabled, true)
  assert.equal(defaults.subscriptionMainIntervalMs, 600000)
  assert.equal(defaults.subscriptionMaxChars, 2000)

  const custom = normalizeConfig({ subscriptionEnabled: false, subscriptionMainIntervalMs: 0, subscriptionMaxChars: 1 })
  assert.equal(custom.subscriptionEnabled, false)
  assert.equal(custom.subscriptionMainIntervalMs, 0)
  assert.equal(custom.subscriptionMaxChars, 1)

  const invalid = [
    [{ subscriptionEnabled: 'yes' }, 'subscriptionEnabled'],
    [{ subscriptionEnabled: 1 }, 'subscriptionEnabled'],
    [{ subscriptionMainIntervalMs: -1 }, 'subscriptionMainIntervalMs'],
    [{ subscriptionMainIntervalMs: 1.5 }, 'subscriptionMainIntervalMs'],
    [{ subscriptionMainIntervalMs: '600000' }, 'subscriptionMainIntervalMs'],
    [{ subscriptionMaxChars: 0 }, 'subscriptionMaxChars'],
    [{ subscriptionMaxChars: 2000.5 }, 'subscriptionMaxChars'],
  ]
  for (const [raw, key] of invalid) {
    assert.throws(
      () => normalizeConfig(raw),
      (error) => error instanceof Error && error.message.includes(`invalid config key ${key}:`),
      JSON.stringify(raw),
    )
  }
})
test('对话摘要 Feed（K10）：dialogExcerptChars / dialogFeedSize 默认值、覆盖与非法值', () => {
  const defaults = normalizeConfig({})
  assert.equal(defaults.dialogExcerptChars, 160)
  assert.equal(defaults.dialogFeedSize, 20)

  const custom = normalizeConfig({ dialogExcerptChars: 40, dialogFeedSize: 1 })
  assert.equal(custom.dialogExcerptChars, 40)
  assert.equal(custom.dialogFeedSize, 1)

  const invalid = [
    [{ dialogExcerptChars: 0 }, 'dialogExcerptChars'],
    [{ dialogExcerptChars: -3 }, 'dialogExcerptChars'],
    [{ dialogExcerptChars: 1.5 }, 'dialogExcerptChars'],
    [{ dialogExcerptChars: '160' }, 'dialogExcerptChars'],
    [{ dialogFeedSize: 0 }, 'dialogFeedSize'],
    [{ dialogFeedSize: -1 }, 'dialogFeedSize'],
  ]
  for (const [raw, key] of invalid) {
    assert.throws(
      () => normalizeConfig(raw),
      (error) => error instanceof Error && error.message.includes(`invalid config key ${key}:`),
      JSON.stringify(raw),
    )
  }
})
test('K17 订阅间隔状态路径：默认空串、自定义字符串、非法类型', () => {
  assert.equal(normalizeConfig({}).subscriptionStatePath, '')
  assert.equal(normalizeConfig({ subscriptionStatePath: '/tmp/astrbox/sub.json' }).subscriptionStatePath, '/tmp/astrbox/sub.json')
  assert.equal(normalizeConfig({ subscriptionStatePath: 'relative-sub.json' }).subscriptionStatePath, 'relative-sub.json')
  for (const value of [3, null, true, {}]) {
    assert.throws(
      () => normalizeConfig({ subscriptionStatePath: value }),
      (error) => error instanceof Error && error.message.includes('invalid config key subscriptionStatePath:'),
      JSON.stringify(value),
    )
  }
})
