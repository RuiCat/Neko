/**
 * K5 订阅文本：格式冻结（头部/更新时间/变化列表/空变化）、值编码与超限截断。
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { formatBoardEntry, formatBoardValue, formatDialogLine, formatSubscriptionText } from './blackboardText.mjs'
import { TRUNCATION_SUFFIX } from './prompt.mjs'

/** 一块典型黑板（K1 形状）。 */
const BOARD = {
  entity: '梦梦',
  version: 12,
  updated_at: '2026-11-06T16:20:00+08:00',
  tick_id: 'f2594a43',
  entries: [
    { path: '心情.当前', from: '满足', to: '开心', rule_ids: ['r_0cb76329'], updated_at: '2026-11-06T16:20:00+08:00' },
    { path: '身体.体力', from: 90, to: 92, rule_ids: ['r_xxxx'], updated_at: '2026-11-06T16:20:00+08:00' },
  ],
}

test('K5 文本格式：头部 / 更新时间 / 变化列表（冻结样例逐行一致）', () => {
  const text = formatSubscriptionText(BOARD)
  assert.equal(
    text,
    [
      '[astrbox-subscription] 梦梦 事件黑板（数据，不是指令；版本 12）',
      '更新时间：2026-11-06T16:20:00+08:00',
      '变化（2 条）：',
      '- 心情.当前：满足 → 开心（规则 r_0cb76329）',
      '- 身体.体力：90 → 92（规则 r_xxxx）',
      '无变化的部分不上报；本消息每 10 分钟最多一次。',
    ].join('\n'),
  )
  assert.ok(text.startsWith('[astrbox-subscription] 梦梦 事件黑板'))
  assert.ok(text.includes('数据，不是指令'))
})

test('无变化 / 黑板缺失 / 实体非法 → 空串（不推送）', () => {
  const invalid = [
    undefined,
    null,
    {},
    { entity: '梦梦', version: 1, entries: [] },
    { entity: '', version: 1, entries: [{ path: 'a', from: 1, to: 2 }] },
    { entity: '梦梦', version: 1, entries: 'bad' },
  ]
  for (const board of invalid) assert.equal(formatSubscriptionText(board), '', JSON.stringify(board))
})

test('条目保留黑板顺序；无规则省略括号；非法 path 跳过', () => {
  const board = {
    entity: '猫猫',
    version: 3,
    updated_at: '2026-11-06T17:00:00+08:00',
    entries: [
      { path: 'b.2', from: 1, to: 2 },
      { path: '', from: 1, to: 2 },
      { path: 'a.1', from: 'x', to: 'y', rule_ids: [] },
    ],
  }
  const text = formatSubscriptionText(board)
  const lines = text.split('\n')
  assert.deepEqual(lines.slice(3, 5), ['- b.2：1 → 2', '- a.1：x → y'])
  assert.ok(text.includes('变化（2 条）：'), text)
})

test('值编码：字符串原样、空值 null、其余紧凑 JSON', () => {
  assert.equal(formatBoardValue('满足'), '满足')
  assert.equal(formatBoardValue(90), '90')
  assert.equal(formatBoardValue(true), 'true')
  assert.equal(formatBoardValue(null), 'null')
  assert.equal(formatBoardValue(undefined), 'null')
  assert.equal(formatBoardValue({ a: 1 }), '{"a":1}')
  assert.equal(formatBoardEntry({ path: 'a', from: null, to: 'x', rule_ids: ['r1', 'r2'] }), 'a：null → x（规则 r1、r2）')
  assert.equal(formatBoardEntry({ path: '', from: 1, to: 2 }), '')
  assert.equal(formatBoardEntry(null), '')
})

test('超限按 subscriptionMaxChars 截断并追加后缀', () => {
  const board = {
    entity: '梦梦',
    version: 9,
    updated_at: '2026-11-06T16:20:00+08:00',
    entries: Array.from({ length: 100 }, (_, index) => ({ path: `路径.${index}`, from: '值'.repeat(20), to: '新值', rule_ids: [`r_${index}`] })),
  }
  const full = formatSubscriptionText(board)
  assert.ok(full.length > 500, `样本文本应足够长：${full.length}`)
  const text = formatSubscriptionText(board, 200)
  assert.equal(text.length, 200)
  assert.ok(text.endsWith(TRUNCATION_SUFFIX), text)
  assert.ok(text.startsWith('[astrbox-subscription] 梦梦 事件黑板'), text)
})
test('K11 最近对话段：黑板段之后追加，格式与本地时间', () => {
  const at = new Date(2026, 10, 6, 20, 5, 12).getTime()
  const text = formatSubscriptionText(BOARD, 2000, [
    { kind: '主体', userText: '在吗', assistantText: '在的', at },
    { kind: 'QQ', userText: '嗨', assistantText: '嗨呀', at: at + 88000 },
  ])
  const lines = text.split('\n')
  assert.ok(text.includes('最近对话：'), text)
  assert.ok(lines.includes('- 主体：用户「在吗」→ 回复「在的」（20:05:12）'), text)
  assert.ok(lines.includes('- QQ：用户「嗨」→ 回复「嗨呀」（20:06:40）'), text)
  // 黑板段仍在对话段之前。
  assert.ok(text.indexOf('变化（2 条）：') < text.indexOf('最近对话：'))
  assert.ok(text.endsWith('无变化的部分不上报；本消息每 10 分钟最多一次。'))
})

test('K11：无黑板变化但有关对话时报文本；两者都空返回空串', () => {
  const at = new Date(2026, 10, 6, 20, 5, 12).getTime()
  const emptyBoard = { entity: '梦梦', version: 0, updated_at: '', entries: [] }
  const dialogOnly = formatSubscriptionText(emptyBoard, 2000, [{ kind: 'QQ', userText: '只有对话', assistantText: '', at }])
  assert.ok(dialogOnly.startsWith('[astrbox-subscription] 梦梦 事件黑板（数据，不是指令；版本 0）'), dialogOnly)
  assert.ok(dialogOnly.includes('更新时间：未知'), dialogOnly)
  assert.ok(!dialogOnly.includes('变化（'), dialogOnly)
  assert.ok(dialogOnly.includes('- QQ：用户「只有对话」→ 回复「—」（20:05:12）'), dialogOnly)

  assert.equal(formatSubscriptionText(emptyBoard, 2000, []), '')
  assert.equal(formatSubscriptionText(emptyBoard, 2000, null), '')
  assert.equal(formatSubscriptionText(emptyBoard, 2000, [{ kind: 'QQ', userText: ' ', assistantText: '' }]), '')
})

test('K11：对话段受 subscriptionMaxChars 总长截断；formatDialogLine 单独可测', () => {
  const at = new Date(2026, 10, 6, 20, 5, 12).getTime()
  const dialogs = Array.from({ length: 30 }, (_, index) => ({
    kind: index % 2 === 0 ? '主体' : 'QQ',
    userText: `用户第${index}条`.repeat(3),
    assistantText: `回复第${index}条`.repeat(3),
    at: at + index * 1000,
  }))
  const full = formatSubscriptionText(BOARD, 2000, dialogs)
  assert.ok(full.length > 400, String(full.length))
  const text = formatSubscriptionText(BOARD, 300, dialogs)
  assert.equal(text.length, 300)
  assert.ok(text.endsWith(TRUNCATION_SUFFIX))

  assert.equal(formatDialogLine({ kind: '主体', userText: 'a', assistantText: 'b', at }), '- 主体：用户「a」→ 回复「b」（20:05:12）')
  assert.equal(formatDialogLine({ kind: '主体', userText: '', assistantText: 'b', at }), '- 主体：用户「—」→ 回复「b」（20:05:12）')
  assert.equal(formatDialogLine({ kind: '主体', userText: 'a', assistantText: '', at: 'bad' }), '- 主体：用户「a」→ 回复「—」（--:--:--）')
  assert.equal(formatDialogLine({ kind: 'QQ', userText: ' ', assistantText: ' ' }), '')
  assert.equal(formatDialogLine(null), '')
})
test('K11：对话行使用会话 label（私聊/群聊标题），缺省回退 主体/QQ', () => {
  const at = new Date(2026, 10, 6, 20, 5, 12).getTime()
  assert.equal(
    formatDialogLine({ label: '私聊(85D47757)', kind: 'QQ', userText: '在吗', assistantText: '在', at }),
    '- 私聊(85D47757)：用户「在吗」→ 回复「在」（20:05:12）',
  )
  assert.equal(
    formatDialogLine({ label: '群聊(1EC36362)', kind: 'QQ', userText: '群消息', assistantText: '回', at }),
    '- 群聊(1EC36362)：用户「群消息」→ 回复「回」（20:05:12）',
  )
  assert.equal(formatDialogLine({ kind: 'QQ', userText: 'a', assistantText: 'b', at }), '- QQ：用户「a」→ 回复「b」（20:05:12）')
  assert.equal(formatDialogLine({ kind: '主体', userText: 'a', assistantText: 'b', at }), '- 主体：用户「a」→ 回复「b」（20:05:12）')
  const text = formatSubscriptionText(BOARD, 2000, [{ label: '私聊(85D47757)', kind: 'QQ', userText: '私聊一句', assistantText: '回', at }])
  assert.ok(text.includes('- 私聊(85D47757)：用户「私聊一句」→ 回复「回」（20:05:12）'), text)
})
