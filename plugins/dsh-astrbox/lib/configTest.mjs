/**
 * C11：配置校验（未知键 / 类型错 / 负数 / 路径无分隔符 → 抛错并指名键）
 * 与 K2 默认值；Observation v2 退役的配置键必须按未知键拒绝。
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { NEKO_HOME, REPO_ROOT, normalizeConfig } from './config.mjs'

test('默认配置与 K2 冻结值一致', () => {
  const config = normalizeConfig()
  assert.equal(config.socketPath, `${NEKO_HOME}/Neko/data/dsh-control.sock`)
  assert.equal(config.entity, '猫猫')
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

test('退役的推送配置键被忽略而不是报错（旧 bridge.config.json 仍能启动）', () => {
  const retired = [
    { paths: ['状态.情绪'] },
    { personaPaths: ['种族'] },
    { scenePaths: ['名称'] },
    { subscriptionPaths: ['状态.情绪'] },
    { subscriptionIgnore: ['代谢.心率'] },
    { subscriptionMaxEntries: 3 },
  ]
  for (const raw of retired) {
    const key = Object.keys(raw)[0]
    const config = normalizeConfig({ ...raw, entity: '猫猫' })
    assert.equal(config.entity, '猫猫', key)
    assert.equal(Object.hasOwn(config, key), false, key)
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

test('自定义合法配置被接受', () => {
  const config = normalizeConfig({ promptMaxChars: 300, entity: '测试猫' })
  assert.equal(config.promptMaxChars, 300)
  assert.equal(config.entity, '测试猫')
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

test('变化推送（Observation v2）：开关 / 间隔 / 文本上限的默认值、合法覆盖与非法值', () => {
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

test('K28 主会话提示词状态路径：默认空串、自定义字符串、非法类型', () => {
  assert.equal(normalizeConfig({}).mainSessionPromptStatePath, '')
  assert.equal(normalizeConfig({ mainSessionPromptStatePath: '/tmp/astrbox/msp.json' }).mainSessionPromptStatePath, '/tmp/astrbox/msp.json')
  for (const value of [3, null, true, {}]) {
    assert.throws(
      () => normalizeConfig({ mainSessionPromptStatePath: value }),
      (error) => error instanceof Error && error.message.includes('invalid config key mainSessionPromptStatePath:'),
      JSON.stringify(value),
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

test('K11 会话黑板：boardEnabled / boardSubscriptionMaxChars / boardSubscriptionStatePath 默认与非法值', () => {
  const defaults = normalizeConfig({})
  assert.equal(defaults.boardEnabled, true)
  assert.equal(defaults.boardSubscriptionMaxChars, 1200)
  assert.equal(defaults.boardSubscriptionStatePath, '')

  const custom = normalizeConfig({
    boardEnabled: false,
    boardSubscriptionMaxChars: 1,
    boardSubscriptionStatePath: '/tmp/astrbox/board-subscriptions.json',
  })
  assert.equal(custom.boardEnabled, false)
  assert.equal(custom.boardSubscriptionMaxChars, 1)
  assert.equal(custom.boardSubscriptionStatePath, '/tmp/astrbox/board-subscriptions.json')

  const invalid = [
    [{ boardEnabled: 'yes' }, 'boardEnabled'],
    [{ boardEnabled: 1 }, 'boardEnabled'],
    [{ boardSubscriptionMaxChars: 0 }, 'boardSubscriptionMaxChars'],
    [{ boardSubscriptionMaxChars: -1 }, 'boardSubscriptionMaxChars'],
    [{ boardSubscriptionMaxChars: 1200.5 }, 'boardSubscriptionMaxChars'],
    [{ boardSubscriptionMaxChars: '1200' }, 'boardSubscriptionMaxChars'],
    [{ boardSubscriptionStatePath: 3 }, 'boardSubscriptionStatePath'],
    [{ boardSubscriptionStatePath: null }, 'boardSubscriptionStatePath'],
  ]
  for (const [raw, key] of invalid) {
    assert.throws(
      () => normalizeConfig(raw),
      (error) => error instanceof Error && error.message.includes(`invalid config key ${key}:`),
      JSON.stringify(raw),
    )
  }
})
