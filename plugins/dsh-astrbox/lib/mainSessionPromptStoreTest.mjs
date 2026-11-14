/**
 * K28 主会话提示词存储：默认路径、加载/保存往返、空文本清除、损坏降级、入参 fail-fast 与超长收敛。
 * 用临时目录，不碰真实 `~/.dsh-astrbox`。
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import {
  MAIN_SESSION_PROMPT_MAX_CHARS,
  PROMPT_STATE_DIR,
  PROMPT_STATE_FILE,
  createMainSessionPromptStore,
  defaultMainSessionPromptPath,
  normalizePromptEntity,
  normalizePromptText,
} from './mainSessionPromptStore.mjs'

/** 建一个隔离的临时状态文件路径。 */
function tempPath() {
  const dir = mkdtempSync(join(tmpdir(), 'neko-msp-'))
  return { dir, path: join(dir, PROMPT_STATE_FILE) }
}

test('默认路径 ~/.dsh-neko/state/main-session-prompts.json（K28）', () => {
  assert.ok(defaultMainSessionPromptPath().endsWith(join(PROMPT_STATE_DIR, PROMPT_STATE_FILE)))
  assert.equal(PROMPT_STATE_DIR, join('.dsh-neko', 'state'))
  assert.equal(PROMPT_STATE_FILE, 'main-session-prompts.json')
})

test('缺失文件：空表、get 空串、list 空对象，且不抛错', () => {
  const { dir, path } = tempPath()
  try {
    const store = createMainSessionPromptStore({ path })
    assert.equal(store.path, path)
    assert.equal(store.get('猫猫'), '')
    assert.deepEqual(store.list(), {})
    assert.equal(store.get(''), '')
    assert.equal(store.get(undefined), '')
  } finally {
    rmSync(dir, { recursive: true, force: true })
  }
})

test('set 落盘后可被新实例读回；空文本 = 清除', () => {
  const { dir, path } = tempPath()
  try {
    const store = createMainSessionPromptStore({ path })
    assert.equal(store.set('猫猫', '  你是猫猫，这里是你自己的主会话。  '), true)
    assert.equal(store.get('猫猫'), '你是猫猫，这里是你自己的主会话。')
    const reloaded = createMainSessionPromptStore({ path })
    assert.equal(reloaded.get('猫猫'), '你是猫猫，这里是你自己的主会话。')
    const raw = JSON.parse(readFileSync(path, 'utf8'))
    assert.deepEqual(raw, { prompts: { 猫猫: '你是猫猫，这里是你自己的主会话。' } })
    assert.equal(reloaded.set('猫猫', '   '), true)
    assert.equal(reloaded.get('猫猫'), '')
    assert.deepEqual(createMainSessionPromptStore({ path }).list(), {})
  } finally {
    rmSync(dir, { recursive: true, force: true })
  }
})

test('remove：有/无条目都成功，并同步落盘', () => {
  const { dir, path } = tempPath()
  try {
    const store = createMainSessionPromptStore({ path })
    store.set('梦梦', 'A')
    assert.equal(store.remove('梦梦'), true)
    assert.equal(store.remove('梦梦'), true)
    assert.deepEqual(createMainSessionPromptStore({ path }).list(), {})
  } finally {
    rmSync(dir, { recursive: true, force: true })
  }
})

test('损坏 JSON / 缺 prompts 对象：降级为空表并只告警', () => {
  const { dir, path } = tempPath()
  try {
    const warns = []
    const logger = { warnOnce: (key, message) => warns.push(`${key}:${message}`) }
    writeFileSync(path, '{ not json', 'utf8')
    const broken = createMainSessionPromptStore({ path, logger })
    assert.deepEqual(broken.list(), {})
    assert.equal(warns.length, 1)

    writeFileSync(path, JSON.stringify({ prompts: 'nope' }), 'utf8')
    const wrongShape = createMainSessionPromptStore({ path, logger })
    assert.deepEqual(wrongShape.list(), {})
    assert.equal(warns.length, 2)
  } finally {
    rmSync(dir, { recursive: true, force: true })
  }
})

test('加载时过滤非法条目并把超长文本收敛到上限', () => {
  const { dir, path } = tempPath()
  try {
    const long = 'x'.repeat(MAIN_SESSION_PROMPT_MAX_CHARS + 50)
    writeFileSync(path, JSON.stringify({
      prompts: {
        猫猫: '  OK  ',
        '': 'skip',
        梦梦: 42,
        多余: '',
        超长: long,
      },
    }), 'utf8')
    const store = createMainSessionPromptStore({ path })
    assert.equal(store.get('猫猫'), 'OK')
    assert.equal(store.get('多余'), '')
    assert.equal(store.get('超长').length, MAIN_SESSION_PROMPT_MAX_CHARS)
    assert.deepEqual(Object.keys(store.list()).sort(), ['猫猫', '超长'])
  } finally {
    rmSync(dir, { recursive: true, force: true })
  }
})

test('入参 fail-fast：空实体 / 非字符串 / 超长', () => {
  assert.throws(() => normalizePromptEntity('  '), /entity/)
  assert.throws(() => normalizePromptEntity(1), /entity/)
  assert.throws(() => normalizePromptText(1), /prompt/)
  assert.throws(() => normalizePromptText('x'.repeat(MAIN_SESSION_PROMPT_MAX_CHARS + 1)), /超长/)
  assert.equal(normalizePromptText(''), '')
  const { dir, path } = tempPath()
  try {
    const store = createMainSessionPromptStore({ path })
    assert.throws(() => store.set('', 'x'), /entity/)
    assert.throws(() => store.remove(''), /entity/)
  } finally {
    rmSync(dir, { recursive: true, force: true })
  }
})

test('保存失败只告警，内存值仍可读（fail-soft）', () => {
  const dir = mkdtempSync(join(tmpdir(), 'neko-msp-'))
  try {
    const warns = []
    const logger = { warnOnce: (key, message) => warns.push(`${key}:${message}`) }
    // 状态路径指向目录本身：rename 必然失败。
    const store = createMainSessionPromptStore({ path: dir, logger })
    assert.equal(store.set('猫猫', 'MEM'), false)
    assert.equal(store.get('猫猫'), 'MEM')
    // 目录当状态文件：加载（EISDIR）与保存各告警一次，且都不得抛错。
    assert.ok(warns.some((message) => message.includes('保存失败')), warns.join('|'))
  } finally {
    rmSync(dir, { recursive: true, force: true })
  }
})
