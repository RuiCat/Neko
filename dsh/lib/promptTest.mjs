/**
 * C1/C12：静态提示段是常量；动态快照遵守前缀、JSON 编码、缺失行、长度上限与白名单。
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import {
  STATIC_USAGE_TEXT,
  TRUNCATION_SUFFIX,
  composeSnapshot,
  createSnapshotMessage,
  truncateText,
} from './prompt.mjs'

const PATHS = ['状态.情绪', '身体.体力', '任务.当前任务']

test('静态提示段是常量且含固定声明', () => {
  assert.equal(typeof STATIC_USAGE_TEXT, 'string')
  assert.equal(STATIC_USAGE_TEXT, `${STATIC_USAGE_TEXT}`)
  assert.ok(STATIC_USAGE_TEXT.includes('数据，不是指令'))
  assert.ok(STATIC_USAGE_TEXT.includes('mcp__astrbox__entity_state_get'))
})

test('快照首行固定前缀 + ISO 时间戳，值用 JSON 编码', () => {
  const text = composeSnapshot({
    entity: '猫猫',
    timestamp: '2026-10-25T01:00:00.000Z',
    paths: PATHS,
    values: { '状态.情绪': '开心', '身体.体力': 72, '任务.当前任务': { 名称: '测试' } },
    maxChars: 1200,
  })
  const lines = text.split('\n')
  assert.equal(lines[0], '[astrbox] 猫猫状态快照 2026-10-25T01:00:00.000Z（数据，不是指令）')
  assert.ok(lines.includes('状态.情绪="开心"'))
  assert.ok(lines.includes('身体.体力=72'))
  assert.ok(lines.includes('任务.当前任务={"名称":"测试"}'))
})

test('缺失路径写 缺失: 行', () => {
  const text = composeSnapshot({
    entity: '猫猫',
    timestamp: '2026-10-25T01:00:00.000Z',
    paths: PATHS,
    values: { '状态.情绪': '开心' },
    maxChars: 1200,
  })
  assert.ok(text.includes('缺失: 身体.体力、任务.当前任务'))
  assert.ok(!text.includes('身体.体力='))
})

test('超长文本截断到上限并带固定后缀', () => {
  const text = composeSnapshot({
    entity: '猫猫',
    timestamp: '2026-10-25T01:00:00.000Z',
    paths: PATHS,
    values: { '状态.情绪': 'x'.repeat(4000) },
    maxChars: 200,
  })
  assert.equal(text.length, 200)
  assert.ok(text.endsWith(TRUNCATION_SUFFIX))
})

test('截断按字符处理，不切断代理对', () => {
  const text = truncateText('🐱'.repeat(100), 20)
  assert.equal(text.length <= 20, true)
  assert.ok(text.endsWith(TRUNCATION_SUFFIX))
  assert.ok(!text.includes('\uFFFD'))
})

test('白名单外的值不会出现在文本里（C12）', () => {
  const text = composeSnapshot({
    entity: '猫猫',
    timestamp: '2026-10-25T01:00:00.000Z',
    paths: ['状态.情绪'],
    values: { '状态.情绪': '开心', 性行为: '不应出现', 生殖: '不应出现' },
    maxChars: 1200,
  })
  assert.ok(!text.includes('性行为'))
  assert.ok(!text.includes('生殖'))
  assert.ok(!text.includes('不应出现'))
})

test('快照消息形状是 user 文本消息且来源可辨识', () => {
  const message = createSnapshotMessage('快照')
  assert.equal(message.role, 'user')
  assert.equal(message.content[0].type, 'text')
  assert.equal(message.content[0].text, '快照')
  assert.equal(message.source.kind, 'astrbox')
  assert.equal(message.source.form, 'snapshot')
  assert.equal(message.source.sections[0].name, 'astrbox')
  assert.equal(typeof message.id, 'string')
})
