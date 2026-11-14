/**
 * K29 `astrbox_main_prompt` 工具：定义形状、主会话读写/跨实体写、子会话拒绝、清除、
 * 超长与非法参数、持久化后重建生效、render 降级。
 */
import assert from 'node:assert/strict'
import { mkdtempSync, readFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { test } from 'node:test'
import { MAIN_SESSION_PROMPT_MAX_CHARS, createMainSessionPromptStore } from './mainSessionPromptStore.mjs'
import { MAIN_PROMPT_TOOL_NAME, createMainPromptTool } from './mainPromptTool.mjs'

/** K1 形状的两条绑定（两个 bot 各一个主会话）。 */
const BINDINGS = [
  { botId: 'helper', label: 'helper', cwd: '/srv/bots/helper', entity: '梦梦', mainSessionId: 'qqbot:helper:main' },
  { botId: 'cat', label: 'cat', cwd: '/srv/bots/cat', entity: '猫猫', mainSessionId: 'qqbot:cat:main' },
]

/**
 * 建临时目录 + store + 工具。
 * @param {object} t node:test 上下文
 * @returns {{tool: object, store: object, path: string}} 依赖
 */
function setup(t) {
  const dir = mkdtempSync(join(tmpdir(), 'astrbox-main-prompt-tool-test-'))
  t.after(() => rmSync(dir, { recursive: true, force: true }))
  const path = join(dir, 'main-session-prompts.json')
  const store = createMainSessionPromptStore({ path })
  const tool = createMainPromptTool({ store, getBindings: () => BINDINGS })
  return { tool, store, path }
}

/**
 * 造执行上下文（agent.session.id 即调用方会话）。
 * @param {string} sessionId 会话 id
 * @returns {{agent: object}} exec
 */
function execOf(sessionId) {
  return { agent: { id: sessionId, session: { id: sessionId, header: { cwd: '/srv/bots/cat' } } } }
}

test('工具定义：名称/参数/输出 schema/render（K29）', () => {
  const tool = createMainPromptTool({ store: createMainSessionPromptStore({ path: join(tmpdir(), 'astrbox-none.json') }), getBindings: () => [] })
  const definition = tool.definition
  assert.equal(definition.name, 'astrbox_main_prompt')
  assert.equal(MAIN_PROMPT_TOOL_NAME, 'astrbox_main_prompt')
  assert.equal(definition.parameters.properties.entity.type, 'string')
  assert.equal(definition.parameters.properties.prompt.type, 'string')
  assert.equal(definition.parameters.properties.list.type, 'boolean')
  assert.deepEqual(definition.parameters.required ?? [], [])
  assert.equal(definition.parameters.additionalProperties, false)
  assert.equal(definition.output.schema.required[0], 'ok')
  assert.deepEqual(definition.output.schema.properties.entities.items.properties.chars.type, 'integer')
  assert.ok(definition.output.render({}, { ok: false, reason: 'not_main_session' })[0].text.includes('not_main_session'))
  assert.ok(definition.output.render({}, { ok: true, entity: '猫猫', cleared: true })[0].text.includes('已清除 猫猫'))
  assert.ok(definition.output.render({}, { ok: true, entity: '猫猫', prompt: '你好' })[0].text.includes('2 字'))
  assert.ok(definition.output.render({}, { ok: true, entities: [{ entity: '猫猫', chars: 3 }] })[0].text.includes('猫猫 3 字'))
})

test('主会话无参读取 → 自己绑定的实体；指定 entity 读取任意实体', async (t) => {
  const { tool, store } = setup(t)
  store.set('猫猫', '猫猫的提示词')
  store.set('梦梦', '梦梦的提示词')
  assert.deepEqual(await tool.execute(undefined, execOf('qqbot:cat:main')), {
    ok: true,
    entity: '猫猫',
    prompt: '猫猫的提示词',
    maxChars: MAIN_SESSION_PROMPT_MAX_CHARS,
  })
  assert.equal((await tool.execute({ entity: '梦梦' }, execOf('qqbot:cat:main'))).prompt, '梦梦的提示词')
  // 未配置的实体读回空串，不报错。
  assert.equal((await tool.execute({ entity: '不存在' }, execOf('qqbot:cat:main'))).prompt, '')
})

test('子会话可读任意实体，但写入返回 not_main_session', async (t) => {
  const { tool, store } = setup(t)
  store.set('梦梦', '梦梦的提示词')
  const child = execOf('session-child-1')
  assert.equal((await tool.execute({ entity: '梦梦' }, child)).prompt, '梦梦的提示词')
  assert.deepEqual(await tool.execute({ entity: '梦梦', prompt: '子会话想写' }, child), {
    ok: false,
    reason: 'not_main_session',
    entity: '梦梦',
  })
  assert.equal(store.get('梦梦'), '梦梦的提示词')
})

test('主会话写自己的实体 → 下一轮生效；可跨实体写（猫猫替梦梦改）', async (t) => {
  const { tool, store, path } = setup(t)
  const catMain = execOf('qqbot:cat:main')
  const wrote = await tool.execute({ prompt: '  猫猫新提示词  ' }, catMain)
  assert.equal(wrote.ok, true)
  assert.equal(wrote.entity, '猫猫')
  assert.equal(wrote.prompt, '猫猫新提示词')
  assert.equal(wrote.cleared, false)
  assert.equal(store.get('猫猫'), '猫猫新提示词')
  // 跨实体写：猫猫的主会话写梦梦的提示词，允许（真实场景）。
  const cross = await tool.execute({ entity: '梦梦', prompt: '梦梦新提示词' }, catMain)
  assert.equal(cross.ok, true)
  assert.equal(store.get('梦梦'), '梦梦新提示词')
  // 持久化：新建 store 从同一文件读回。
  const reloaded = createMainSessionPromptStore({ path })
  assert.equal(reloaded.get('猫猫'), '猫猫新提示词')
  assert.equal(reloaded.get('梦梦'), '梦梦新提示词')
  assert.equal(readFileSync(path, 'utf8').includes('梦梦新提示词'), true)
})

test('prompt 空串 = 清除该实体；空表后读回空串', async (t) => {
  const { tool, store } = setup(t)
  store.set('猫猫', '要清除的提示词')
  const cleared = await tool.execute({ entity: '猫猫', prompt: '   ' }, execOf('qqbot:cat:main'))
  assert.equal(cleared.ok, true)
  assert.equal(cleared.cleared, true)
  assert.equal(store.get('猫猫'), '')
})

test('list=true 列出全部实体与字数（任意会话可用）', async (t) => {
  const { tool, store } = setup(t)
  store.set('猫猫', 'abc')
  store.set('梦梦', '0123456789')
  const listed = await tool.execute({ list: true }, execOf('session-child-2'))
  assert.equal(listed.ok, true)
  assert.deepEqual(listed.entities, [
    { entity: '猫猫', chars: 3 },
    { entity: '梦梦', chars: 10 },
  ])
  assert.equal(listed.maxChars, MAIN_SESSION_PROMPT_MAX_CHARS)
})

test('非法参数：entity 非字符串 / 非主会话无 entity / 超长 / prompt 非字符串', async (t) => {
  const { tool } = setup(t)
  const catMain = execOf('qqbot:cat:main')
  assert.deepEqual(await tool.execute({ entity: 42 }, catMain), { ok: false, reason: 'invalid_entity' })
  assert.deepEqual(await tool.execute({}, execOf('session-child-3')), { ok: false, reason: 'need_entity' })
  const tooLong = '字'.repeat(MAIN_SESSION_PROMPT_MAX_CHARS + 1)
  assert.deepEqual(await tool.execute({ prompt: tooLong }, catMain), {
    ok: false,
    reason: 'prompt_too_long',
    entity: '猫猫',
  })
  assert.deepEqual(await tool.execute({ prompt: 42 }, catMain), { ok: false, reason: 'invalid_prompt', entity: '猫猫' })
})
