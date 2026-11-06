/**
 * K6：绑定规范化（非法条目丢弃、同 cwd first-wins）与按 cwd 解析
 * （目录边界安全、最长前缀优先、未命中返回 undefined）。
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { normalizeBindings, normalizeCwd, resolveEntityForCwd } from './entityBindings.mjs'

/** 合法条目工厂：只覆盖需要改的字段。 */
function binding(overrides = {}) {
  return { botId: 'bot-cat', cwd: '/srv/bots/cat', entity: '梦梦', label: 'cat', mainSessionId: 's-cat', ...overrides }
}

test('normalizeBindings：保留合法条目与全部非空可选字段', () => {
  const bindings = normalizeBindings([binding()])
  assert.deepEqual(bindings, [
    { botId: 'bot-cat', cwd: '/srv/bots/cat', entity: '梦梦', label: 'cat', mainSessionId: 's-cat' },
  ])
})

test('normalizeBindings：可选字段缺省或为空串时不写出该字段', () => {
  const bindings = normalizeBindings([
    { botId: 'b1', cwd: '/a' },
    { botId: 'b2', cwd: '/b', entity: '', label: '   ', mainSessionId: 42 },
  ])
  assert.deepEqual(bindings, [
    { botId: 'b1', cwd: '/a' },
    { botId: 'b2', cwd: '/b' },
  ])
})

test('normalizeBindings：非法条目逐个丢弃，不影响合法条目', () => {
  const raw = [
    null,
    undefined,
    'not-an-object',
    [],
    {},
    { botId: 'b1' },
    { cwd: '/x' },
    { botId: '', cwd: '/x' },
    { botId: 'b1', cwd: '   ' },
    { botId: 7, cwd: '/x' },
    { botId: 'b1', cwd: 42 },
    { botId: 'ok', cwd: '/ok', entity: '猫猫' },
  ]
  const bindings = normalizeBindings(raw)
  assert.deepEqual(bindings, [{ botId: 'ok', cwd: '/ok', entity: '猫猫' }])
})

test('normalizeBindings：同 cwd 只保留数组序第一条（结尾分隔符视为同一 cwd）', () => {
  const bindings = normalizeBindings([
    binding({ botId: 'first', cwd: '/srv/bots/cat', entity: '梦梦' }),
    binding({ botId: 'second', cwd: '/srv/bots/cat/', entity: '猫猫' }),
    binding({ botId: 'third', cwd: '/srv/bots/cat', entity: '助助' }),
    binding({ botId: 'nested', cwd: '/srv/bots/cat/sub', entity: '小猫' }),
  ])
  assert.deepEqual(
    bindings.map((item) => [item.botId, item.entity]),
    [
      ['first', '梦梦'],
      ['nested', '小猫'],
    ],
  )
})

test('normalizeBindings：非数组输入返回空数组（不抛错）', () => {
  for (const raw of [undefined, null, {}, 'bindings', 42, true]) {
    assert.deepEqual(normalizeBindings(raw), [], String(raw))
  }
})

test('normalizeCwd：去尾分隔符与空白，根目录记为 /', () => {
  assert.equal(normalizeCwd('/srv/bots/cat'), '/srv/bots/cat')
  assert.equal(normalizeCwd('/srv/bots/cat/'), '/srv/bots/cat')
  assert.equal(normalizeCwd('  /srv/bots/cat//  '), '/srv/bots/cat')
  assert.equal(normalizeCwd('/'), '/')
  assert.equal(normalizeCwd('//'), '/')
  assert.equal(normalizeCwd(''), undefined)
  assert.equal(normalizeCwd('   '), undefined)
  assert.equal(normalizeCwd(undefined), undefined)
  assert.equal(normalizeCwd(7), undefined)
})

test('resolveEntityForCwd：精确命中返回完整绑定', () => {
  const source = binding()
  const bindings = normalizeBindings([source])
  assert.deepEqual(resolveEntityForCwd(bindings, '/srv/bots/cat'), source)
  // 会话 cwd 在绑定目录之下同样命中。
  assert.deepEqual(resolveEntityForCwd(bindings, '/srv/bots/cat/sub/dir'), source)
})

test('resolveEntityForCwd：嵌套绑定取最长前缀（不取外层）', () => {
  const bindings = normalizeBindings([
    binding({ botId: 'outer', cwd: '/srv/bots', entity: '猫猫' }),
    binding({ botId: 'inner', cwd: '/srv/bots/cat', entity: '梦梦' }),
  ])
  assert.equal(resolveEntityForCwd(bindings, '/srv/bots/cat/sub').botId, 'inner')
  assert.equal(resolveEntityForCwd(bindings, '/srv/bots/dog').botId, 'outer')
  assert.equal(resolveEntityForCwd(bindings, '/srv/bots').botId, 'outer')
})

test('resolveEntityForCwd：目录边界安全（<cwd>-old / <cwd>extra 不命中）', () => {
  const bindings = normalizeBindings([binding()])
  assert.equal(resolveEntityForCwd(bindings, '/srv/bots/cat-old'), undefined)
  assert.equal(resolveEntityForCwd(bindings, '/srv/bots/catextra'), undefined)
  assert.equal(resolveEntityForCwd(bindings, '/srv/bots'), undefined)
  assert.equal(resolveEntityForCwd(bindings, '/srv/bots/cat/sub'), bindings[0])
})

test('resolveEntityForCwd：同 cwd 重复条目按数组序第一条（规范化后的入参也一样）', () => {
  const bindings = [
    { botId: 'first', cwd: '/srv/bots/cat', entity: '梦梦' },
    { botId: 'second', cwd: '/srv/bots/cat', entity: '猫猫' },
  ]
  assert.equal(resolveEntityForCwd(bindings, '/srv/bots/cat').botId, 'first')
})

test('resolveEntityForCwd：未命中与非法 cwd 返回 undefined，且跳过非法条目', () => {
  const bindings = normalizeBindings([binding()])
  assert.equal(resolveEntityForCwd(bindings, '/elsewhere'), undefined)
  assert.equal(resolveEntityForCwd(bindings, undefined), undefined)
  assert.equal(resolveEntityForCwd(bindings, ''), undefined)
  assert.equal(resolveEntityForCwd(undefined, '/srv/bots/cat'), undefined)
  // 未经 normalizeBindings 的数组里，非法条目不参与解析。
  const raw = [null, { cwd: '/srv/bots/cat' }, { botId: 'x' }, { botId: 'ok', cwd: '/srv/bots/cat', entity: '猫猫' }]
  assert.equal(resolveEntityForCwd(raw, '/srv/bots/cat').botId, 'ok')
})

test('resolveEntityForCwd：绑定在根目录时命中任意 cwd', () => {
  const bindings = normalizeBindings([binding({ cwd: '/' })])
  assert.equal(resolveEntityForCwd(bindings, '/anything/here'), bindings[0])
})
