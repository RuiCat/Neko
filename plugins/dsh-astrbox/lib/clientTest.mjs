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
  for (const name of ['parseBoardsWire', 'normalizeBoard', 'entryLine', 'entryRules', 'valueText', 'formatClockTime', 'formatShortTime', 'parseStateWire', 'snapshotRows', 'partitionBoards', 'sanitizeLayoutMode', 'sanitizeBoardTab', 'nextLayoutMode', 'lastPathSegment', 'truncateText']) {
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
  const stamp = api.formatShortTime('2026-11-06T16:20:00+08:00')
  assert.deepEqual(board.entries, [
    { line: '心情.当前：满足 → 开心', rules: ['r_0cb76329'], time: stamp },
    { line: '身体.体力：90 → 92', rules: [], time: stamp },
  ])
  assert.equal(parsed.boards[1].board, null)
})

test('parseBoardsWire：primary/paths 元数据（快照条）收敛非法值并截断到 8 条', () => {
  const parsed = api.parseBoardsWire({
    ok: true,
    primary: ' 猫猫 ',
    paths: ['状态.情绪', ' 身体.体力 ', '', 42, ...Array.from({ length: 12 }, (_, index) => `字段.${index}`)],
    boards: [],
  })
  assert.equal(parsed.primary, '猫猫')
  assert.deepEqual(parsed.paths.slice(0, 2), ['状态.情绪', '身体.体力'])
  assert.equal(parsed.paths.length, 8)
  const withoutMeta = api.parseBoardsWire({ ok: true, boards: [] })
  assert.equal(withoutMeta.primary, '')
  assert.deepEqual(withoutMeta.paths, [])
  const badMeta = api.parseBoardsWire({ ok: true, primary: 42, paths: 'bad', boards: [] })
  assert.equal(badMeta.primary, '')
  assert.deepEqual(badMeta.paths, [])
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
  assert.deepEqual(board.entries, [{ line: 'a：1 → 2', rules: [], time: '' }])
})

test('entryLine / valueText：字符串原样、空值 null、对象紧凑 JSON、非法路径空串', () => {
  assert.equal(api.valueText('满足'), '满足')
  assert.equal(api.valueText(90), '90')
  assert.equal(api.valueText(null), 'null')
  assert.equal(api.valueText(undefined), 'null')
  assert.equal(api.valueText({ a: 1 }), '{"a":1}')
  assert.equal(api.entryLine({ path: '心情.当前', from: '满足', to: '开心' }), '心情.当前：满足 → 开心')
  // from 缺失（新出现的字段）只显示当前值：`null → x` 对读的人没有信息量。
  assert.equal(api.entryLine({ path: 'a', from: null, to: 'x' }), 'a：x')
  assert.equal(api.entryLine({ path: 'a', to: 100 }), 'a：100')
  assert.equal(api.entryLine({ path: '  ', from: 1, to: 2 }), '')
  assert.equal(api.entryLine(null), '')
  assert.deepEqual(api.entryRules({ rule_ids: ['r1', '', 3, 'r2'] }), ['r1', 'r2'])
  assert.deepEqual(api.entryRules(null), [])
})

test('formatShortTime / lastPathSegment / truncateText：卡片时间、快照标签与长值截断', () => {
  const iso = '2026-11-06T16:20:00+08:00'
  const date = new Date(iso)
  const pad = (value) => String(value).padStart(2, '0')
  assert.equal(api.formatShortTime(iso), `${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`)
  for (const bad of [undefined, null, '', 'not-a-date', 42]) assert.equal(api.formatShortTime(bad), '')
  assert.equal(api.lastPathSegment('状态.情绪'), '情绪')
  assert.equal(api.lastPathSegment('a'), 'a')
  assert.equal(api.lastPathSegment(''), '')
  assert.equal(api.lastPathSegment(null), '')
  assert.equal(api.truncateText('abc', 5), 'abc')
  assert.equal(api.truncateText('abcdef', 3), 'abc…')
  assert.equal(api.truncateText(null, 3), '')
})

test('parseStateWire：控制面 /state 响应 → values；非法/失败 ok:false', () => {
  const ok = api.parseStateWire({ ok: true, entity: '猫猫', values: { '状态.情绪': '开心' }, missing: [] })
  assert.deepEqual(ok, { ok: true, values: { '状态.情绪': '开心' }, error: '' })
  const noValues = api.parseStateWire({ ok: true })
  assert.deepEqual(noValues.values, {})
  for (const wire of [undefined, null, {}, { ok: false }, { ok: 'yes' }, 42]) {
    assert.equal(api.parseStateWire(wire).ok, false, JSON.stringify(wire))
  }
})

test('snapshotRows：标签取路径末段、缺值显示 —、长值截断、非法路径跳过', () => {
  const rows = api.snapshotRows(
    ['状态.情绪', '身体.体力', '任务.当前任务', '不存在的字段', 'x'],
    { '状态.情绪': '开心', '身体.体力': 92, '任务.当前任务': '周日计划：陪主人服务端分析 BNS 反编译（winmm/themida_yukina）' },
  )
  assert.deepEqual(rows.map((row) => row.label), ['情绪', '体力', '当前任务', '不存在的字段', 'x'])
  assert.equal(rows[0].text, '开心')
  assert.equal(rows[1].text, '92')
  assert.equal(rows[2].text.endsWith('…'), true)
  assert.equal(rows[2].text.length <= 29, true)
  assert.equal(rows[3].text, '—')
  assert.equal(rows[3].missing, true)
  assert.equal(api.snapshotRows('bad', {}).length, 0)
  assert.equal(api.snapshotRows(['a.b'], null)[0].text, '—')
})

test('entityCandidates：生效实体置顶 + 黑板实体去重；非法输入空数组', () => {
  assert.deepEqual(api.entityCandidates('猫猫', [{ entity: '梦梦' }, { entity: '猫猫' }, { entity: '猫猫' }, null, { entity: '' }]), ['猫猫', '梦梦'])
  assert.deepEqual(api.entityCandidates('', [{ entity: '梦梦' }, { entity: '猫猫' }]), ['梦梦', '猫猫'])
  assert.deepEqual(api.entityCandidates(' 猫猫 ', undefined), ['猫猫'])
  assert.deepEqual(api.entityCandidates(undefined, 'bad'), [])
})

test('partitionBoards：生效实体置顶、其余按更新时间倒序、空黑板进折叠区', () => {
  const changed = { entity: '梦梦', version: 2, updatedAt: '2026-11-08T13:49:51+08:00', entries: [{ line: 'a：1 → 2', rules: [], time: '' }] }
  const older = { entity: '猫猫', version: 3, updatedAt: '2026-11-08T13:00:00+08:00', entries: [{ line: 'b：1 → 2', rules: [], time: '' }] }
  const emptyBoard = { entity: '客厅', version: 0, updatedAt: '', entries: [] }
  const { changed: hits, empty } = api.partitionBoards([
    { entity: '客厅', board: emptyBoard },
    { entity: '猫猫', board: older },
    { entity: '梦梦', board: changed },
    { entity: '没有黑板', board: null },
    null,
    { entity: '' },
  ], '猫猫')
  assert.deepEqual(hits.map((item) => item.entity), ['猫猫', '梦梦'])
  assert.deepEqual(empty.map((item) => item.entity), ['客厅', '没有黑板'])
  assert.deepEqual(empty[0].board.entries, [])
  assert.equal(empty.find((item) => item.entity === '没有黑板').board, null)
  assert.deepEqual(api.partitionBoards(undefined, '').changed, [])
})

test('K25 布局模式：收敛与互斥切换（收起 / 全宽都从分栏回来）', () => {
  assert.equal(api.sanitizeLayoutMode('monitor-full'), 'monitor-full')
  for (const bad of [undefined, null, '', 'full', 42]) assert.equal(api.sanitizeLayoutMode(bad), 'split')
  assert.equal(api.nextLayoutMode('split', 'toggleMonitor'), 'monitor-hidden')
  assert.equal(api.nextLayoutMode('monitor-hidden', 'toggleMonitor'), 'split')
  assert.equal(api.nextLayoutMode('monitor-full', 'toggleMonitor'), 'monitor-hidden')
  assert.equal(api.nextLayoutMode('split', 'toggleFull'), 'monitor-full')
  assert.equal(api.nextLayoutMode('monitor-full', 'toggleFull'), 'split')
  assert.equal(api.nextLayoutMode('monitor-hidden', 'toggleFull'), 'monitor-full')
  assert.equal(api.nextLayoutMode('bad', 'unknown'), 'split')
})

test('K31b 黑板 Tab：合法值 event/session，其它收敛为 event', () => {
  assert.equal(api.sanitizeBoardTab('session'), 'session')
  assert.equal(api.sanitizeBoardTab('event'), 'event')
  for (const bad of [undefined, null, '', 'board', 42, {}]) assert.equal(api.sanitizeBoardTab(bad), 'event')
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
    'board.changed',
    'board.more',
    'board.expand',
    'board.collapse',
    'status.collapse',
    'status.expand',
    'status.full',
    'status.unfold',
    'snapshot.loading',
    'snapshot.unavailable',
    'snapshot.empty',
    'snapshot.noPrimary',
    'snapshot.switch',
    'snapshot.world',
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
  assert.ok(zh['board.rules'].includes('{count}'))
  assert.ok(zh['board.changed'].includes('{count}'))
  assert.ok(zh['board.more'].includes('{count}'))
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

test('K28 parseMainPromptsWire：过滤空实体/非字符串/空文本并保留上限', () => {
  const parsed = api.parseMainPromptsWire({
    ok: true,
    maxChars: 20000,
    prompts: { ' 猫猫 ': '  你是猫猫  ', 梦梦: '', 多余: 42, '': 'skip' },
  })
  assert.equal(parsed.ok, true)
  assert.equal(parsed.maxChars, 20000)
  assert.deepEqual(parsed.prompts, { 猫猫: '你是猫猫' })
  assert.equal(parsed.error, '')
  const bad = api.parseMainPromptsWire({ ok: false, error: { message: 'store_failed' } })
  assert.equal(bad.ok, false)
  assert.equal(bad.error, 'store_failed')
  assert.deepEqual(bad.prompts, {})
  assert.equal(api.parseMainPromptsWire(null).ok, false)
})

test('K28 mainPromptPayload：实体必填、文本 trim（空文本 = 清除）', () => {
  assert.deepEqual(api.mainPromptPayload(' 猫猫 ', '  你好  '), { entity: '猫猫', prompt: '你好' })
  assert.deepEqual(api.mainPromptPayload('猫猫', '   '), { entity: '猫猫', prompt: '' })
  assert.deepEqual(api.mainPromptPayload('猫猫', undefined), { entity: '猫猫', prompt: '' })
  for (const value of ['', '  ', null, 42]) assert.equal(api.mainPromptPayload(value, 'x'), null)
})

test('K28 promptValueOf：命中返回文本，未配置/非法形状回空串', () => {
  const prompts = { 猫猫: 'A' }
  assert.equal(api.promptValueOf(prompts, ' 猫猫 '), 'A')
  assert.equal(api.promptValueOf(prompts, '梦梦'), '')
  assert.equal(api.promptValueOf(prompts, ''), '')
  assert.equal(api.promptValueOf(null, '猫猫'), '')
  assert.equal(api.promptValueOf({ 猫猫: 42 }, '猫猫'), '')
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

/** K31 会话黑板 wire：两个区域（含死条目与非法区域项）。 */
const SESSION_BOARD = {
  scope: 'workspace',
  scope_id: '/home/neko',
  version: 5,
  updated_at: '2026-11-10T15:00:00+08:00',
  min_retained_version: 1,
  regions: [
    {
      id: 'session-abc',
      version: 4,
      updated_at: '2026-11-10T14:00:00+08:00',
      entries: [
        { key: '任务.当前', from: null, to: '写规划', by: 'session-abc', version: 4, updated_at: '2026-11-10T14:00:00+08:00', deleted: false },
        { key: '旧键', from: 1, to: null, by: 'session-abc', version: 4, updated_at: '2026-11-10T14:00:00+08:00', deleted: true },
      ],
    },
    {
      id: 'session-def',
      version: 5,
      updated_at: '2026-11-10T15:00:00+08:00',
      entries: [
        { key: '项目.状态', from: null, to: '进行中', by: 'session-def', version: 5, updated_at: '2026-11-10T15:00:00+08:00', deleted: false },
      ],
    },
    { id: '', entries: [] },
    null,
  ],
}

test('K31 会话黑板 __test 出口', () => {
  for (const name of ['parseSessionBoardWire', 'parseBoardWorkspacesWire', 'sessionBoardEntryLine', 'parseSessionBoardValue', 'sessionBoardTarget', 'sessionBoardOpsPayload', 'sessionBoardSetPayload', 'pickBoardWorkspace']) {
    assert.equal(typeof api[name], 'function', name)
  }
})

test('K31 parseSessionBoardWire：解析/过滤死条目/按区更新时间倒序', () => {
  const parsed = api.parseSessionBoardWire({ ok: true, board: SESSION_BOARD })
  assert.equal(parsed.ok, true)
  assert.equal(parsed.board.scope, 'workspace')
  assert.equal(parsed.board.version, 5)
  assert.deepEqual(parsed.board.regions.map((region) => region.id), ['session-def', 'session-abc'])
  assert.deepEqual(parsed.board.regions[0].entries, [{
    key: '项目.状态',
    line: '项目.状态：进行中（session-def）',
    time: api.formatShortTime('2026-11-10T15:00:00+08:00'),
  }])
  assert.equal(parsed.board.regions[1].entries.length, 1)
  for (const wire of [undefined, null, {}, { ok: false }, { ok: true, board: [] }, { ok: true, board: 'bad' }]) {
    const bad = api.parseSessionBoardWire(wire)
    assert.equal(bad.ok, false, JSON.stringify(wire))
    assert.equal(bad.board, null)
  }
  assert.equal(api.parseSessionBoardWire({ ok: false, error: { message: 'board_failed' } }).error, 'board_failed')
})

test('K31 parseBoardWorkspacesWire：只取工作区键（regions 是数量 int）并去重', () => {
  const parsed = api.parseBoardWorkspacesWire({
    ok: true,
    workspaces: [
      { workspace: '/home/neko', version: 3, regions: 2, updated_at: '2026-11-10T15:00:00+08:00' },
      { workspace: ' /home/neko ' },
      { workspace: '' },
      { workspace: 42 },
      null,
    ],
  })
  assert.equal(parsed.ok, true)
  assert.deepEqual(parsed.items, ['/home/neko'])
  assert.equal(parsed.error, '')
  const bad = api.parseBoardWorkspacesWire({ ok: false, error: { message: 'nope' } })
  assert.equal(bad.ok, false)
  assert.deepEqual(bad.items, [])
  assert.equal(bad.error, 'nope')
})

test('K31 sessionBoardEntryLine：`key：value（by）`；死条目/空 key 空串', () => {
  assert.equal(api.sessionBoardEntryLine({ key: '任务.当前', to: '写规划', by: 'session-abc' }), '任务.当前：写规划（session-abc）')
  assert.equal(api.sessionBoardEntryLine({ key: 'a', to: 1 }), 'a：1')
  assert.equal(api.sessionBoardEntryLine({ key: 'a', to: null, by: '' }), 'a：null')
  assert.equal(api.sessionBoardEntryLine({ key: 'a', to: 1, deleted: true }), '')
  assert.equal(api.sessionBoardEntryLine({ key: ' ', to: 1 }), '')
  assert.equal(api.sessionBoardEntryLine(null), '')
})

test('K31 parseSessionBoardValue：先 JSON，失败按字符串', () => {
  assert.deepEqual(api.parseSessionBoardValue('92'), 92)
  assert.deepEqual(api.parseSessionBoardValue('"开心"'), '开心')
  assert.deepEqual(api.parseSessionBoardValue('{"a":1}'), { a: 1 })
  assert.equal(api.parseSessionBoardValue('开心'), '开心')
  assert.equal(api.parseSessionBoardValue(''), '')
  assert.equal(api.parseSessionBoardValue('  '), '  ')
  assert.equal(api.parseSessionBoardValue(null), '')
})

test('K31 sessionBoardTarget / payload：global 区域=工作区键、workspace 分开、字段不全 null', () => {
  assert.deepEqual(api.sessionBoardTarget('global', '', '/w'), { scope: 'global', workspace: '/w', region: '/w' })
  assert.deepEqual(api.sessionBoardTarget('global', '/w', ''), { scope: 'global', workspace: '/w', region: '/w' })
  assert.deepEqual(api.sessionBoardTarget('workspace', '/w', 'session-1'), { scope: 'workspace', workspace: '/w', region: 'session-1' })
  assert.equal(api.sessionBoardTarget('workspace', '', 'session-1'), null)
  assert.equal(api.sessionBoardTarget('workspace', '/w', ''), null)
  assert.equal(api.sessionBoardTarget('global', '', ''), null)
  assert.equal(api.sessionBoardTarget('bad', '/w', 'r'), null)
  assert.deepEqual(api.sessionBoardSetPayload('global', '', '/w', ' 项目.状态 ', '进行中'), {
    scope: 'global',
    workspace: '/w',
    region: '/w',
    ops: [{ op: 'set', key: '项目.状态', value: '进行中' }],
  })
  assert.equal(api.sessionBoardSetPayload('global', '', '/w', '', 'x'), null)
  assert.deepEqual(api.sessionBoardOpsPayload('workspace', '/w', 'session-1', [{ op: 'clear' }]), {
    scope: 'workspace',
    workspace: '/w',
    region: 'session-1',
    ops: [{ op: 'clear' }],
  })
  assert.equal(api.sessionBoardOpsPayload('workspace', '/w', 'session-1', []), null)
})

test('K31 pickBoardWorkspace：存值优先、否则第一个、空列表空串', () => {
  assert.equal(api.pickBoardWorkspace('/stored', ['/a', '/b']), '/stored')
  assert.equal(api.pickBoardWorkspace('  ', ['/a', '/b']), '/a')
  assert.equal(api.pickBoardWorkspace('', []), '')
  assert.equal(api.pickBoardWorkspace(null, 'bad'), '')
})

test('K31 会话黑板文案：ZH/EN 同键且齐全', () => {
  const zh = api.dictionaries.zh
  const en = api.dictionaries.en
  assert.deepEqual(Object.keys(zh).sort(), Object.keys(en).sort())
  for (const key of [
    'sessionBoard.title',
    'sessionBoard.global',
    'sessionBoard.workspace',
    'sessionBoard.empty',
    'sessionBoard.emptyRegion',
    'sessionBoard.delete',
    'sessionBoard.clear',
    'sessionBoard.clearConfirm',
    'sessionBoard.cancel',
    'sessionBoard.region',
    'sessionBoard.key',
    'sessionBoard.value',
    'sessionBoard.write',
    'sessionBoard.needFields',
    'sessionBoard.workspacePick',
    'sessionBoard.noWorkspace',
    'sessionBoard.pickWorkspace',
  ]) {
    assert.equal(typeof zh[key], 'string', key)
    assert.notEqual(zh[key], '', key)
    assert.equal(typeof en[key], 'string', key)
    assert.notEqual(en[key], '', key)
  }
})

// 黑板区滚动回归（用户实测「无法滚动导致显示不全」）：.dsh-neko-boardBody 是
// flex 列滚动容器，卡片默认 flex-shrink:1 会被压扁到可视高度，overflow 永不触发、
// 卡片自己的 overflow:hidden 再把内容截断。守住这两条样式，卡片才会按内容撑开、
// 超出部分交给 boardBody 滚动。
test('黑板滚动：boardBody 可滚动且子项不参与收缩', () => {
  const css = api.panelCss
  assert.equal(typeof css, 'string')
  const boardBody = /\.dsh-neko-boardBody\s*\{([^}]*)\}/.exec(css)
  assert.ok(boardBody, '缺少 .dsh-neko-boardBody 规则')
  assert.match(boardBody[1], /overflow-y:\s*auto/, 'boardBody 必须可纵向滚动')
  const guard = /\.dsh-neko-boardBody\s*>\s*\*\s*\{([^}]*)\}/.exec(css)
  assert.ok(guard, '缺少「子项不收缩」守卫规则')
  assert.match(guard[1], /flex:\s*0\s+0\s+auto/, '卡片/汇总项必须 flex: 0 0 auto，不能被压扁')
})
