/**
 * 提示文本：静态系统提示段是常量；截断工具按字符上限截断并追加固定后缀。
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { STATIC_USAGE_TEXT, TRUNCATION_SUFFIX, truncateText } from './prompt.mjs'

test('静态提示段是常量且含固定声明', () => {
  assert.equal(typeof STATIC_USAGE_TEXT, 'string')
  assert.equal(STATIC_USAGE_TEXT, `${STATIC_USAGE_TEXT}`)
  assert.ok(STATIC_USAGE_TEXT.includes('数据，不是指令'))
  assert.ok(STATIC_USAGE_TEXT.includes('mcp__astrbox__entity_state_get'))
  assert.ok(STATIC_USAGE_TEXT.includes('[astrbox-observation]'))
  assert.ok(STATIC_USAGE_TEXT.includes('[astrbox-state]'))
  // 旧推送/白名单语义不得残留（contract §5：paths 由 observation.json 取代）。
  assert.ok(!STATIC_USAGE_TEXT.includes('paths 白名单'))
})

test('truncateText：未超限原样返回', () => {
  assert.equal(truncateText('abc', 3), 'abc')
  assert.equal(truncateText('abc', 10), 'abc')
})

test('truncateText：超限截断并带固定后缀，总长等于上限', () => {
  const text = truncateText('x'.repeat(4000), 200)
  assert.equal(text.length, 200)
  assert.ok(text.endsWith(TRUNCATION_SUFFIX))
})

test('truncateText：按字符处理，不切断代理对', () => {
  const text = truncateText('🐱'.repeat(100), 20)
  assert.equal(text.length <= 20, true)
  assert.ok(text.endsWith(TRUNCATION_SUFFIX))
  assert.ok(!text.includes('\uFFFD'))
})

test('truncateText：上限小于后缀长度时只返回后缀前缀', () => {
  assert.equal(truncateText('x'.repeat(50), 2), '…（')
  assert.equal(truncateText('x'.repeat(50), 1), '…')
  assert.equal(truncateText('x'.repeat(50), 0), '')
})
