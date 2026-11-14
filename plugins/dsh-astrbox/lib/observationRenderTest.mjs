/**
 * 契约 §4：Observation v2 渲染 golden 测试——给定 payload 输出逐字节稳定。
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import {
  DEFAULT_DELTA_MAX_CHARS,
  DELTA_PREFIX,
  STATE_PREFIX,
  formatValue,
  renderDelta,
  renderState,
} from './observationRender.mjs'

/** §3.1 形状的三条变化（先 critical 后 normal）。 */
function deltaPayload() {
  return {
    ok: true,
    scope: 'entity:猫',
    version: 4,
    since: 0,
    cursor: 4,
    budget: { maxItems: 12, maxChars: 60, coalesceMs: 60000 },
    events: [
      { seq: 1, path: '状态.身体', from: '疲惫', to: '虚弱', tier: 'critical' },
      { seq: 2, path: '位置.场景', from: '书房', to: '卧室', tier: 'normal' },
      { seq: 3, path: '心情.当前', from: '满足', to: '开心', tier: 'normal' },
    ],
    truncated: false,
    dropped: 0,
    policyVersion: 2,
  }
}

test('formatValue：空值 / 字符串 / 布尔 / 数字 / 对象 / 数组（§4.1）', () => {
  assert.equal(formatValue(null), null)
  assert.equal(formatValue(undefined), null)
  assert.equal(formatValue('开心'), '开心')
  assert.equal(formatValue(true), '真')
  assert.equal(formatValue(false), '假')
  assert.equal(formatValue(1), '1')
  assert.equal(formatValue(1.5), '1.5')
  assert.equal(formatValue(1.0), '1')
  assert.equal(formatValue({ a: 1, b: 'x' }), '{"a":1,"b":"x"}')
  assert.equal(formatValue([1, 'x', null]), '[1,"x",null]')
})

test('formatValue：超长截断并追加 `…`，不切断代理对（§4.1）', () => {
  assert.equal(formatValue('x'.repeat(10), 5), 'xxxx…')
  assert.equal(formatValue('🐱'.repeat(5), 3), '🐱…')
  assert.equal(formatValue('abc', 3), 'abc')
  assert.equal(formatValue(null, 3), null)
})

test('renderDelta：头行 + 每行 path/from/to，逐字节 golden（§4.2）', () => {
  const payload = deltaPayload()
  const text = renderDelta(payload)
  assert.equal(
    text,
    '[astrbox-observation] 猫 · 3 处变化\n- 状态.身体: 疲惫 → 虚弱\n另有 2 条未显示',
  )
  assert.equal(renderDelta(payload), text, '同一 payload 必须逐字节稳定')
})

test('renderDelta：options.maxChars 覆盖 payload.budget.maxChars（§4.2）', () => {
  const text = renderDelta(deltaPayload(), { maxChars: 80 })
  assert.equal(
    text,
    '[astrbox-observation] 猫 · 3 处变化\n- 状态.身体: 疲惫 → 虚弱\n- 位置.场景: 书房 → 卧室\n另有 1 条未显示',
  )
  assert.ok(text.length <= 80)
})

test('renderDelta：极紧预算只留头行 + 未显示条数，且不超上限（§4.2）', () => {
  const text = renderDelta(deltaPayload(), { maxChars: 45 })
  assert.equal(text, '[astrbox-observation] 猫 · 3 处变化\n另有 3 条未显示')
  assert.ok(text.length <= 45)
})

test('renderDelta：from 为 null/缺失时只写当前值（§4.2）', () => {
  const payload = {
    ok: true,
    scope: 'entity:猫',
    version: 2,
    events: [
      { seq: 1, path: '任务.当前', from: null, to: '写规划' },
      { seq: 2, path: '任务.备注', to: '新增' },
    ],
  }
  assert.equal(renderDelta(payload), '[astrbox-observation] 猫 · 2 处变化\n- 任务.当前: 写规划\n- 任务.备注: 新增')
})

test('renderDelta：events 为空返回空串；非法 payload / 无实体返回空串（§4.2）', () => {
  assert.equal(renderDelta({ ok: true, scope: 'entity:猫', version: 3, events: [] }), '')
  assert.equal(renderDelta(null), '')
  assert.equal(renderDelta({ ok: true, events: [{ path: '状态.身体', to: 'x' }] }), '')
  assert.equal(renderDelta({ ok: true, scope: 'entity:猫', events: [{ path: '', to: 'x' }] }), '')
})

test('renderDelta：缺省 maxChars 回落 payload.budget.maxChars，再回落 1200（§4.2）', () => {
  const payload = deltaPayload()
  assert.ok(renderDelta(payload).length <= payload.budget.maxChars)
  const noBudget = { ...deltaPayload(), budget: undefined }
  assert.ok(renderDelta(noBudget).length <= DEFAULT_DELTA_MAX_CHARS)
  assert.equal(DELTA_PREFIX, '[astrbox-observation]')
  assert.equal(STATE_PREFIX, '[astrbox-state]')
})

test('renderState：头行 + 每 path 一行当前值，逐字节 golden（§4.3）', () => {
  const payload = {
    ok: true,
    scope: 'entity:猫',
    version: 12,
    entries: [
      { path: '状态.身体', value: '虚弱', from: '疲惫', tier: 'critical', seq: 12, at: '2026-11-14T08:00:00Z' },
      { path: '位置.场景', value: '卧室', from: '书房', tier: 'normal', seq: 11, at: '2026-11-14T07:59:00Z' },
    ],
    truncated: false,
    budget: { maxItems: 12, maxChars: 1200, coalesceMs: 60000 },
  }
  const text = renderState(payload)
  assert.equal(text, '[astrbox-state] 猫\n状态.身体: 虚弱\n位置.场景: 卧室')
  assert.equal(renderState(payload), text, '同一 payload 必须逐字节稳定')
})

test('renderState：超出 maxItems 追加 另有 N 条未显示（§4.3）', () => {
  const payload = {
    ok: true,
    scope: 'entity:猫',
    version: 12,
    entries: [
      { path: '状态.身体', value: '虚弱' },
      { path: '位置.场景', value: '卧室' },
      { path: '心情.当前', value: '开心' },
    ],
  }
  assert.equal(renderState(payload, { maxItems: 2 }), '[astrbox-state] 猫\n状态.身体: 虚弱\n位置.场景: 卧室\n另有 1 条未显示')
  // 默认 maxItems=8：三条全部显示。
  assert.equal(renderState(payload), '[astrbox-state] 猫\n状态.身体: 虚弱\n位置.场景: 卧室\n心情.当前: 开心')
})

test('renderState：字符预算超限从尾部裁行，最终不超过 maxChars（§4.3）', () => {
  const payload = {
    ok: true,
    scope: 'entity:猫',
    entries: [
      { path: '状态.身体', value: 'x'.repeat(50) },
      { path: '位置.场景', value: '卧室' },
    ],
  }
  const text = renderState(payload, { maxChars: 20 })
  assert.ok(text.length <= 20, text)
  assert.ok(text.endsWith('…'), text)
})

test('renderState：entries 为空 / 非法 payload 返回空串（§4.3）', () => {
  assert.equal(renderState({ ok: true, scope: 'entity:猫', entries: [] }), '')
  assert.equal(renderState(null), '')
  assert.equal(renderState({ ok: true, entries: [{ path: '状态.身体', value: 1 }] }), '')
})
