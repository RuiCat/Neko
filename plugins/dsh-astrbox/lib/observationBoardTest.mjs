/**
 * 面板投影：`/v2/observations/state` → `GET /astrbox/blackboard` 的 board 形状。
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { DEFAULT_PANEL_PATH_LIMIT, boardFromObservationState, statePathsOf } from './observationBoard.mjs'

/** §3.2 形状的两条现状。 */
function statePayload() {
  return {
    ok: true,
    scope: 'entity:猫',
    version: 12,
    entries: [
      { path: '状态.身体', value: '虚弱', from: '疲惫', tier: 'critical', seq: 12, at: '2026-11-14T07:00:00Z' },
      { path: '位置.场景', value: '卧室', from: '书房', tier: 'normal', seq: 11, at: '2026-11-14T08:00:00Z' },
    ],
    truncated: false,
    budget: { maxItems: 12, maxChars: 1200, coalesceMs: 60000 },
  }
}

test('board：entity/version/updated_at + entries 每条 path/from/to/updated_at', () => {
  const board = boardFromObservationState('猫', statePayload())
  assert.deepEqual(board, {
    entity: '猫',
    version: 12,
    updated_at: '2026-11-14T08:00:00Z',
    entries: [
      { path: '状态.身体', from: '疲惫', to: '虚弱', updated_at: '2026-11-14T07:00:00Z' },
      { path: '位置.场景', from: '书房', to: '卧室', updated_at: '2026-11-14T08:00:00Z' },
    ],
  })
})

test('board：无数据 / 非法响应 / 空实体 → null（面板显示无黑板）', () => {
  assert.equal(boardFromObservationState('猫', { ok: true, entries: [] }), null)
  assert.equal(boardFromObservationState('猫', { ok: false }), null)
  assert.equal(boardFromObservationState('猫', null), null)
  assert.equal(boardFromObservationState('', statePayload()), null)
  assert.equal(
    boardFromObservationState('猫', { ok: true, entries: [{ path: '', value: 1 }, { value: 2 }] }),
    null,
  )
})

test('board：from 缺失记为 null，非整数 version 回落 0', () => {
  const board = boardFromObservationState('猫', {
    ok: true,
    version: 'bad',
    entries: [{ path: '任务.当前', value: '写规划', at: '' }],
  })
  assert.equal(board.version, 0)
  assert.equal(board.updated_at, '')
  assert.deepEqual(board.entries, [{ path: '任务.当前', from: null, to: '写规划', updated_at: '' }])
})

test('statePathsOf：按服务端顺序去重、截断到上限', () => {
  const payload = statePayload()
  assert.deepEqual(statePathsOf(payload), ['状态.身体', '位置.场景'])
  const many = { ok: true, entries: Array.from({ length: 40 }, (_, index) => ({ path: `字段.${index}` })) }
  assert.equal(statePathsOf(many).length, DEFAULT_PANEL_PATH_LIMIT)
  assert.equal(statePathsOf(many, 3).length, 3)
  assert.deepEqual(statePathsOf(null), [])
  assert.deepEqual(statePathsOf({ ok: true, entries: [{ path: 'a' }, { path: ' a ' }, { path: '' }] }), ['a'])
})
