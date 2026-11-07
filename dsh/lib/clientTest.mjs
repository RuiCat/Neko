/**
 * 仓库内验收：用最小 fake `window.__ModuleLoader__` + fake React 载入浏览器半
 * `dsh/lib/client.js`（零构建单文件插件），断言 K8 事件黑板的 wire 解析、变化行格式化、
 * 时间/版本展示、空态与不可用态文案；不依赖 jsdom，也不依赖 qqbot 构建产物。
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'

/** client.js 顶层登记的模块定义。 */
let registration
globalThis.window = {
  __ModuleLoader__: {
    load(definition) {
      registration = definition
    },
  },
}
await import(new URL('./client.js', import.meta.url).href)

/** 最小 React 桩：只满足工厂解构与返回形状，这里的用例不渲染组件。 */
const reactStub = {
  createElement: () => null,
  useCallback: (callback) => callback,
  useEffect: () => undefined,
  useMemo: (factory) => factory(),
  useRef: (value) => ({ current: value }),
  useState: (value) => [typeof value === 'function' ? value() : value, () => undefined],
}

const plugin = registration.factory((specifier) => {
  if (specifier === 'react') return reactStub
  throw new Error(`unexpected require: ${specifier}`)
})
const api = plugin.__test

/** K1 形状的一块黑板。 */
const BOARD = {
  entity: '梦梦',
  version: 12,
  updated_at: '2026-11-06T16:20:00+08:00',
  tick_id: 'f2594a43',
  entries: [
    { path: '心情.当前', from: '满足', to: '开心', rule_ids: ['r_0cb76329'], updated_at: '2026-11-06T16:20:00+08:00' },
    { path: '身体.体力', from: 90, to: 92, rule_ids: [], updated_at: '2026-11-06T16:20:00+08:00' },
  ],
}

test('__test 出口与 inject/apply 形状不变（面板不再依赖会话服务）', () => {
  assert.deepEqual(plugin.inject, ['slots'])
  assert.equal(typeof plugin.apply, 'function')
  for (const name of ['parseBoardsWire', 'normalizeBoard', 'entryLine', 'entryRules', 'valueText', 'formatClockTime']) {
    assert.equal(typeof api[name], 'function', name)
  }
})

test('parseBoardsWire：多实体 wire → 卡片模型；board:null 保留实体', () => {
  const parsed = api.parseBoardsWire({
    ok: true,
    boards: [{ entity: '梦梦', board: BOARD }, { entity: '猫猫', board: null }],
  })
  assert.equal(parsed.ok, true)
  assert.deepEqual(parsed.boards.map((item) => item.entity), ['梦梦', '猫猫'])
  const board = parsed.boards[0].board
  assert.equal(board.version, 12)
  assert.equal(board.updatedAt, '2026-11-06T16:20:00+08:00')
  assert.deepEqual(board.entries, [
    { line: '心情.当前：满足 → 开心', rules: ['r_0cb76329'] },
    { line: '身体.体力：90 → 92', rules: [] },
  ])
  assert.equal(parsed.boards[1].board, null)
})

test('parseBoardsWire：非法/失败 wire → ok:false（面板显示「黑板不可用」）', () => {
  const invalid = [undefined, null, {}, { ok: false }, { ok: true, boards: 'bad' }, { ok: 'yes', boards: [] }]
  for (const wire of invalid) {
    const parsed = api.parseBoardsWire(wire)
    assert.equal(parsed.ok, false, JSON.stringify(wire))
    assert.deepEqual(parsed.boards, [])
  }
  const filtered = api.parseBoardsWire({ ok: true, boards: [null, 42, {}, { entity: '' }, { entity: '梦梦', board: BOARD }] })
  assert.equal(filtered.ok, true)
  assert.deepEqual(filtered.boards.map((item) => item.entity), ['梦梦'])
})

test('normalizeBoard：非法实体/版本/条目 → 空安全（不抛错）', () => {
  assert.equal(api.normalizeBoard(null), null)
  assert.equal(api.normalizeBoard([]), null)
  assert.equal(api.normalizeBoard({ entity: '' }), null)
  const board = api.normalizeBoard({ entity: '猫猫', entries: [{ path: '' }, null, { path: 'a', from: 1, to: 2 }] })
  assert.equal(board.version, 0)
  assert.equal(board.updatedAt, '')
  assert.deepEqual(board.entries, [{ line: 'a：1 → 2', rules: [] }])
})

test('entryLine / valueText：字符串原样、空值 null、对象紧凑 JSON、非法路径空串', () => {
  assert.equal(api.valueText('满足'), '满足')
  assert.equal(api.valueText(90), '90')
  assert.equal(api.valueText(null), 'null')
  assert.equal(api.valueText(undefined), 'null')
  assert.equal(api.valueText({ a: 1 }), '{"a":1}')
  assert.equal(api.entryLine({ path: '心情.当前', from: '满足', to: '开心' }), '心情.当前：满足 → 开心')
  assert.equal(api.entryLine({ path: 'a', from: null, to: 'x' }), 'a：null → x')
  assert.equal(api.entryLine({ path: '  ', from: 1, to: 2 }), '')
  assert.equal(api.entryLine(null), '')
  assert.deepEqual(api.entryRules({ rule_ids: ['r1', '', 3, 'r2'] }), ['r1', 'r2'])
  assert.deepEqual(api.entryRules(null), [])
})

test('formatClockTime：RFC3339 → 本地 HH:MM:SS；非法输入空串', () => {
  const iso = '2026-11-06T16:20:00+08:00'
  const date = new Date(iso)
  const pad = (value) => String(value).padStart(2, '0')
  assert.equal(api.formatClockTime(iso), `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`)
  for (const bad of [undefined, null, '', 'not-a-date', 42]) assert.equal(api.formatClockTime(bad), '')
})

test('字典：ZH/EN 同键集，黑板文案与空态/不可用态齐全', () => {
  const zh = api.dictionaries.zh
  const en = api.dictionaries.en
  assert.deepEqual(Object.keys(zh).sort(), Object.keys(en).sort())
  for (const key of [
    'board.title',
    'board.refresh',
    'board.ready',
    'board.loading',
    'board.unavailable',
    'board.error',
    'board.empty',
    'board.noChanges',
    'board.version',
    'board.updated',
    'board.rules',
  ]) {
    assert.equal(typeof zh[key], 'string', key)
    assert.notEqual(zh[key], '', key)
    assert.notEqual(en[key], '', key)
  }
  assert.equal(zh['board.unavailable'], '黑板不可用')
  assert.ok(zh['board.noChanges'].includes('暂无变化'))
  assert.ok(zh['board.empty'].includes('暂无实体黑板'))
  assert.ok(zh['board.version'].includes('{version}'))
  assert.ok(zh['board.updated'].includes('{time}'))
  assert.ok(zh['board.rules'].includes('{rules}'))
  // 面板不再有聊天文案（K8：删除会话聊天）。
  for (const key of Object.keys(zh)) assert.ok(!key.startsWith('chat.'), key)
})
test('K22 toRfc3339：datetime-local → 本地时区 RFC3339；非法返回空串', () => {
  const date = new Date(2026, 10, 7, 18, 0, 0)
  const pad = (value) => String(value).padStart(2, '0')
  const offset = -date.getTimezoneOffset()
  const suffix = `${offset >= 0 ? '+' : '-'}${pad(Math.floor(Math.abs(offset) / 60))}:${pad(Math.abs(offset) % 60)}`
  assert.equal(api.toRfc3339('2026-11-07T18:00'), `2026-11-07T18:00:00${suffix}`)
  assert.equal(api.toRfc3339('2026-11-07T18:00:30'), `2026-11-07T18:00:30${suffix}`)
  for (const bad of [undefined, null, '', 'not-a-time', '2026-13-01T00:00', 42]) {
    assert.equal(api.toRfc3339(bad), '', String(bad))
  }
})

test('K22 parseJsonValue：JSON 解析与可读错误', () => {
  assert.deepEqual(api.parseJsonValue('"开心"'), { ok: true, value: '开心', error: '' })
  assert.deepEqual(api.parseJsonValue('92'), { ok: true, value: 92, error: '' })
  assert.deepEqual(api.parseJsonValue('{"a":1}'), { ok: true, value: { a: 1 }, error: '' })
  assert.equal(api.parseJsonValue('').ok, false)
  assert.equal(api.parseJsonValue('  ').ok, false)
  const broken = api.parseJsonValue('{bad json')
  assert.equal(broken.ok, false)
  assert.notEqual(broken.error, '')
})

test('K22 statePayload：组装 POST /astrbox/control/state 请求体', () => {
  assert.deepEqual(api.statePayload(' 猫猫 ', ' 状态.情绪 ', '开心'), { entity: '猫猫', path: '状态.情绪', value: '开心' })
  assert.deepEqual(api.statePayload('猫猫', '身体.体力', 92), { entity: '猫猫', path: '身体.体力', value: 92 })
  for (const [entity, path] of [['', 'a'], ['猫猫', ''], [null, 'a'], ['猫猫', null]]) {
    assert.equal(api.statePayload(entity, path, 1), null)
  }
})

test('K22 控制区文案：ZH/EN 同键且齐全（含错误提示）', () => {
  const zh = api.dictionaries.zh
  const en = api.dictionaries.en
  assert.deepEqual(Object.keys(zh).sort(), Object.keys(en).sort())
  for (const key of ['control.title', 'control.tick', 'control.add15', 'control.add60', 'control.time', 'control.setTime', 'control.entity', 'control.path', 'control.value', 'control.write', 'control.done', 'control.badJson', 'control.badTime', 'control.needFields']) {
    assert.equal(typeof zh[key], 'string', key)
    assert.notEqual(zh[key], '', key)
    assert.equal(typeof en[key], 'string', key)
  }
  assert.ok(zh['control.badJson'].includes('{error}'))
})
