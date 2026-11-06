/**
 * K2 `astrboxBridge` 服务：NPC 实体目录过滤、人设文本冻结格式与截断、同步 getter 永不抛错、
 * TTL 缓存与 single-flight、refresh() 重拉目录并预取绑定实体人设；另覆盖 K5 新配置键校验。
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { composePersona, createHostBridge } from './astrboxService.mjs'
import { DEFAULT_PERSONA_PATHS, normalizeConfig } from './config.mjs'
import { TRUNCATION_SUFFIX } from './prompt.mjs'
import { nextMacroTask } from './testHarness.mjs'

const R18_TERMS = ['性行为', '器官状态', '外翻', '生殖', '体液追踪', '肉棒']

/** 默认测试配置：3 条人设路径，与 K5 默认值解耦。 */
const CONFIG = Object.freeze({ personaPaths: ['种族', '体型', '自称'], personaMaxChars: 600, promptRefreshMs: 5000 })

/** 假控制面客户端：记录 /entities 与 /state 调用，可注入失败。 */
function fakeClient(options = {}) {
  const calls = { entities: 0, state: [] }
  const client = {
    calls,
    entitiesBody: options.entitiesBody ?? { entities: [] },
    stateValues: options.stateValues ?? {},
    entitiesError: options.entitiesError,
    stateError: options.stateError,
    async getEntities() {
      calls.entities += 1
      if (client.entitiesError !== undefined) throw client.entitiesError
      return client.entitiesBody
    },
    async getState(query) {
      calls.state.push(query)
      if (client.stateError !== undefined) throw client.stateError
      return { ok: true, entity: query.entity, values: client.stateValues[query.entity] ?? {} }
    },
  }
  return client
}

/** 假 logger：只累计 warnOnce 调用（生产实现按 key 限频，测试桩不做限频）。 */
function fakeLogger() {
  return {
    warnings: [],
    warnOnce(key, message) {
      this.warnings.push([key, message])
    },
  }
}

/** 人设值表：覆盖 CONFIG.personaPaths 全部路径（`/state` 的 values 是「点分路径 → 值」的扁平表）。 */
function personaValues() {
  return {
    种族: '猫',
    体型: '娇小',
    自称: '咱',
    对主人称呼: '主人',
    '人格.MBTI': 'INFP',
    '人格.气质类型': '黏液质',
  }
}

test('entities()：只暴露 type.startsWith(NPC) 的实体，保序去重且返回副本', async () => {
  const client = fakeClient({
    entitiesBody: {
      entities: [
        { id: '猫猫', type: 'NPC猫' },
        { id: '场景', type: '场景' },
        { id: 'helper', type: 'NPC' },
        { id: '猫猫', type: 'NPC' },
        { id: '', type: 'NPC' },
        { id: 'bad' },
        null,
      ],
    },
  })
  const host = createHostBridge({ client, config: CONFIG, logger: fakeLogger() })
  // 同步 getter：首次调用立即返回 []，后台拉取。
  assert.deepEqual(host.entities(), [])
  await nextMacroTask()
  assert.equal(client.calls.entities, 1)
  assert.deepEqual(host.entities(), ['猫猫', 'helper'])
  const copy = host.entities()
  copy.push('污染')
  assert.deepEqual(host.entities(), ['猫猫', 'helper'])
})

test('entities()：/entities 失败不抛错、保留上一版目录并限频告警', async () => {
  const logger = fakeLogger()
  const client = fakeClient({ entitiesBody: { entities: [{ id: '猫猫', type: 'NPC' }] } })
  const host = createHostBridge({ client, config: { ...CONFIG, promptRefreshMs: 0 }, logger })
  host.entities()
  await nextMacroTask()
  assert.deepEqual(host.entities(), ['猫猫'])
  client.entitiesError = new Error('socket boom')
  assert.doesNotThrow(() => host.entities())
  await nextMacroTask()
  assert.deepEqual(host.entities(), ['猫猫'], '失败时保留上一版目录')
  assert.equal(logger.warnings[0][0], 'entities')
})

test('entities()：客户端没有 getEntities 时返回 [] 且不抛错', async () => {
  const logger = fakeLogger()
  const host = createHostBridge({ client: { getState: async () => ({ values: {} }) }, config: CONFIG, logger })
  assert.deepEqual(host.entities(), [])
  await nextMacroTask()
  assert.deepEqual(host.entities(), [])
  assert.ok(logger.warnings.some(([key]) => key === 'entities'))
})

test('persona() 同步返回 string：未就绪为 ""，refresh() 预取绑定实体后非空', async () => {
  const client = fakeClient({
    entitiesBody: { entities: [{ id: '猫猫', type: 'NPC' }] },
    stateValues: { 猫猫: personaValues() },
  })
  const host = createHostBridge({
    client,
    config: CONFIG,
    logger: fakeLogger(),
    getBindings: () => [{ botId: 'cat', cwd: '/srv/bots/cat', entity: '猫猫' }],
  })
  const initial = host.persona('猫猫')
  assert.equal(typeof initial, 'string')
  assert.equal(initial, '')
  await host.refresh()
  const text = host.persona('猫猫')
  assert.equal(typeof text, 'string')
  assert.ok(text.startsWith('## 你的角色（AstrBox NPC 实体：猫猫）'), text)
  assert.ok(text.includes('你在这台机器上是「猫猫」，一个由 Neko 引擎驱动的角色实体。'), text)
  assert.ok(text.includes('- 种族="猫"'), text)
  assert.ok(text.endsWith('与主人对话时保持该角色与语气；不要自称 AI 助手。'), text)
  // refresh 已预取：再次取值不再发 /state。
  const callsBefore = client.calls.state.length
  host.persona('猫猫')
  await nextMacroTask()
  assert.equal(client.calls.state.length, callsBefore)
})

test('persona() 组合 personaPaths 并按 personaMaxChars 截断', async () => {
  const client = fakeClient({ stateValues: { 猫猫: personaValues() } })
  const host = createHostBridge({
    client,
    config: { ...CONFIG, personaPaths: ['人格.MBTI', '人格.气质类型', '种族'], personaMaxChars: 120 },
    logger: fakeLogger(),
  })
  host.persona('猫猫')
  await nextMacroTask()
  const text = host.persona('猫猫')
  assert.ok(text.includes('- 人格.MBTI="INFP"'), text)
  assert.ok(text.includes('- 人格.气质类型="黏液质"'), text)
  assert.equal(text.length, 120)
  assert.ok(text.endsWith(TRUNCATION_SUFFIX), text)
})

test('composePersona：无可用路径 / 关闭人设 / 非法实体返回 ""', () => {
  assert.equal(composePersona({ entityId: '猫猫', paths: [], values: personaValues() }), '')
  assert.equal(composePersona({ entityId: '猫猫', paths: ['不存在'], values: personaValues() }), '')
  assert.equal(composePersona({ entityId: '', paths: ['种族'], values: personaValues() }), '')
  assert.equal(composePersona(undefined), '')
})

test('personaPaths=[] 关闭人设：persona 返回 "" 且不发 /state', async () => {
  const client = fakeClient({ stateValues: { 猫猫: personaValues() } })
  const host = createHostBridge({ client, config: { ...CONFIG, personaPaths: [] }, logger: fakeLogger() })
  host.persona('猫猫')
  await nextMacroTask()
  assert.equal(host.persona('猫猫'), '')
  assert.equal(client.calls.state.length, 0)
})

test('persona 缓存 TTL：窗口内复用，promptRefreshMs=0 时每次请求', async () => {
  const cachedClient = fakeClient({ stateValues: { 猫猫: personaValues() } })
  const cached = createHostBridge({ client: cachedClient, config: CONFIG, logger: fakeLogger() })
  cached.persona('猫猫')
  await nextMacroTask()
  cached.persona('猫猫')
  await nextMacroTask()
  assert.equal(cachedClient.calls.state.length, 1)

  const freshClient = fakeClient({ stateValues: { 猫猫: personaValues() } })
  const fresh = createHostBridge({ client: freshClient, config: { ...CONFIG, promptRefreshMs: 0 }, logger: fakeLogger() })
  fresh.persona('猫猫')
  await nextMacroTask()
  fresh.persona('猫猫')
  await nextMacroTask()
  assert.equal(freshClient.calls.state.length, 2)
})

test('persona single-flight：同实体并发调用只发一次 /state', async () => {
  const client = fakeClient({ stateValues: { 猫猫: personaValues() } })
  const host = createHostBridge({ client, config: { ...CONFIG, promptRefreshMs: 0 }, logger: fakeLogger() })
  host.persona('猫猫')
  host.persona('猫猫')
  await nextMacroTask()
  assert.equal(client.calls.state.length, 1)
})

test('persona 拉取失败：返回 "" 不抛错，限频告警；refresh 不拒绝', async () => {
  const logger = fakeLogger()
  const client = fakeClient()
  client.stateError = new Error('state boom')
  const host = createHostBridge({
    client,
    config: CONFIG,
    logger,
    getBindings: () => [{ botId: 'cat', cwd: '/srv/bots/cat', entity: '猫猫' }],
  })
  assert.equal(host.persona('猫猫'), '')
  await nextMacroTask()
  assert.equal(host.persona('猫猫'), '')
  await assert.doesNotReject(() => host.refresh())
  assert.equal(logger.warnings[0][0], 'persona:猫猫')
})

test('refresh()：重拉实体目录并预取绑定实体人设；绑定读取抛错不影响刷新', async () => {
  const client = fakeClient({
    entitiesBody: { entities: [{ id: '猫猫', type: 'NPC' }] },
    stateValues: { 猫猫: personaValues(), 梦梦: { 种族: '猫' } },
  })
  let bindings = [{ botId: 'cat', cwd: '/srv/bots/cat', entity: '猫猫' }]
  const host = createHostBridge({ client, config: CONFIG, logger: fakeLogger(), getBindings: () => bindings })
  await host.refresh()
  assert.deepEqual(host.entities(), ['猫猫'])
  assert.ok(host.persona('猫猫').startsWith('## 你的角色'), 'refresh 应预取绑定实体人设')
  assert.deepEqual(
    client.calls.state.map((call) => call.entity),
    ['猫猫'],
  )

  client.entitiesBody = { entities: [{ id: '猫猫', type: 'NPC' }, { id: '梦梦', type: 'NPC' }] }
  await host.refresh()
  assert.deepEqual(host.entities(), ['猫猫', '梦梦'])

  bindings = undefined
  // getBindings 抛错时 refresh 仍完成（不预取、不上抛）。
  const throwing = createHostBridge({
    client,
    config: CONFIG,
    logger: fakeLogger(),
    getBindings: () => {
      throw new Error('bindings boom')
    },
  })
  await assert.doesNotReject(() => throwing.refresh())
})

test('K5 配置：personaPaths/personaMaxChars 默认值与校验', () => {
  const config = normalizeConfig()
  assert.deepEqual([...config.personaPaths], [...DEFAULT_PERSONA_PATHS])
  assert.equal(DEFAULT_PERSONA_PATHS.length, 7)
  assert.ok(DEFAULT_PERSONA_PATHS.includes('人格.MBTI'))
  assert.equal(config.personaMaxChars, 600)
  for (const path of DEFAULT_PERSONA_PATHS) {
    for (const term of R18_TERMS) assert.ok(!path.includes(term), `${path} 不应含 ${term}`)
  }
  assert.deepEqual([...normalizeConfig({ personaPaths: [] }).personaPaths], [])
  assert.equal(normalizeConfig({ personaMaxChars: 1 }).personaMaxChars, 1)

  const cases = [
    [{ personaPaths: '种族' }, 'personaPaths'],
    [{ personaPaths: [''] }, 'personaPaths[0]'],
    [{ personaPaths: [7] }, 'personaPaths[0]'],
    [{ personaMaxChars: 0 }, 'personaMaxChars'],
    [{ personaMaxChars: 1.5 }, 'personaMaxChars'],
    [{ personaMaxChars: '600' }, 'personaMaxChars'],
  ]
  for (const [raw, key] of cases) {
    assert.throws(
      () => normalizeConfig(raw),
      (error) => error instanceof Error && error.message.includes(`invalid config key ${key}:`),
      JSON.stringify(raw),
    )
  }
})
