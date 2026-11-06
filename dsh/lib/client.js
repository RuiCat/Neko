/**
 * Neko 面板 · dsh-astrbox 的浏览器半（客户端插件）。
 *
 * code-modularity: no-split —— 交付契约要求本插件是**单文件浏览器插件**（宿主按
 * `/plugins/dsh-astrbox/client.js` 整体加载本文件，`window.__ModuleLoader__.load` 登记
 * 工厂，不支持 import、无构建步骤），拆成多文件会破坏加载契约；纯函数已通过
 * `__test` 出口独立单测（`dsh/lib/clientTest.mjs`）。技术债：面板继续增长时应引入
 * 打包步骤把纯逻辑拆出（需改 `dsh/package.json` 的 dsh.client 与构建入口，超出本文件写域）。
 *
 * 左侧栏全局面板入口（`sidebar.panellist`）+ 主列页面（`main`）：左栏 iframe 内嵌
 * Neko Monitor 状态界面（`127.0.0.1:8765/monitor/`），右栏是事件黑板（每个实体一块卡片，
 * 轮询同源 `GET /astrbox/blackboard`，10s 一次、页面隐藏暂停）。代码只用注入服务与 React，
 * 不 require 其他 Harness Client 包。
 *
 * 生效方式：DSH 启动时扫描 bundle 行的 `dsh.client` 声明，从
 * `/plugins/dsh-astrbox/client.js` 加载本工厂；新增/改动本文件后需重启 DSH。
 */
window.__ModuleLoader__.load({
  id: 'dsh-astrbox',
  factory(require) {
    const React = require('react')
    const h = React.createElement
    const { useCallback, useEffect, useState } = React

    /** 面板 id：同时作为 sidebar.panellist 的行 id 与 main 槽位的 key。 */
    const PANEL_ID = 'neko-panel'
    /** 左侧栏全局面板行的顺序（任务看板 20，技能 30，SSH 40）。 */
    const PANEL_ORDER = 50
    /** 本插件文案命名空间。 */
    const NS = 'neko-panel'
    /** Neko Monitor 页面默认地址；面板里可改并持久化到 localStorage。 */
    const DEFAULT_MONITOR_URL = 'http://127.0.0.1:8765/monitor/'
    const STORAGE_MONITOR = 'dsh-neko-panel.monitor-url'
    /** 事件黑板轮询间隔（K8：10s）。 */
    const BOARDS_REFRESH_MS = 10000
    /** 同源黑板数据路由（桥接注册，K7）。 */
    const BOARDS_PATH = '/astrbox/blackboard'

    /** 中文文案（键集真源）。 */
    const ZH = {
      'entry.label': 'Neko 面板',
      'status.title': 'Neko 状态',
      'status.refresh': '刷新',
      'status.open': '新标签打开',
      'status.url': '状态界面地址',
      'status.hint': '状态界面来自 Neko Monitor；若空白，先在仓库里运行 make monitor。',
      'board.title': '事件黑板',
      'board.refresh': '刷新',
      'board.ready': '就绪',
      'board.loading': '加载中…',
      'board.unavailable': '黑板不可用',
      'board.error': '读取失败：{error}',
      'board.empty': '暂无实体黑板（等待状态机事件）',
      'board.noChanges': '暂无变化（等待状态机事件）',
      'board.version': '版本 {version}',
      'board.updated': '更新 {time}',
      'board.rules': '规则 {rules}',
    }

    /** 英文文案：与 ZH 同键集。 */
    const EN = {
      'entry.label': 'Neko panel',
      'status.title': 'Neko status',
      'status.refresh': 'Refresh',
      'status.open': 'Open in new tab',
      'status.url': 'Status view URL',
      'status.hint': 'The status view comes from Neko Monitor; run make monitor if it stays blank.',
      'board.title': 'Event blackboard',
      'board.refresh': 'Refresh',
      'board.ready': 'Ready',
      'board.loading': 'Loading…',
      'board.unavailable': 'Blackboard unavailable',
      'board.error': 'Could not load: {error}',
      'board.empty': 'No entity blackboards yet (waiting for a state-machine event).',
      'board.noChanges': 'No changes yet (waiting for a state-machine event).',
      'board.version': 'Version {version}',
      'board.updated': 'Updated {time}',
      'board.rules': 'Rules {rules}',
    }

    /**
     * 当前文档语言对应的内置字典（zh 前缀用中文，其余回落英文）。
     * @returns {Record<string, string>} 字典。
     */
    function activeDictionary() {
      const language = typeof document === 'undefined' ? '' : String(document.documentElement.lang || '')
      return language.toLowerCase().startsWith('zh') ? ZH : EN
    }

    /**
     * 占位符替换：`{name}` 形式。
     * @param {string} template 文本模板。
     * @param {Record<string, string>} [params] 占位符值。
     * @returns {string} 替换结果。
     */
    function fillTemplate(template, params) {
      if (params === undefined) return template
      return template.replace(/\{(\w+)\}/g, (match, key) => (key in params ? String(params[key]) : match))
    }

    /**
     * 内置字典查词（locale 服务缺失时的回落）。
     * @param {string} key 文案键。
     * @param {Record<string, string>} [params] 占位符值。
     * @returns {string} 文案。
     */
    function localTranslate(key, params) {
      const dictionary = activeDictionary()
      return fillTemplate(dictionary[key] ?? EN[key] ?? key, params)
    }

    /**
     * 取异常的可读文本。
     * @param {unknown} error 异常或业务失败。
     * @returns {string} 文本。
     */
    function errorText(error) {
      if (error === null || error === undefined) return 'unknown error'
      if (typeof error === 'string') return error
      if (typeof error === 'object' && 'message' in error && typeof error.message === 'string') return error.message
      return String(error)
    }

    /**
     * 读取 localStorage（不可用时回落默认值）。
     * @param {string} key 存储键。
     * @param {string} fallback 默认值。
     * @returns {string} 存储值。
     */
    function readStored(key, fallback) {
      try {
        const value = window.localStorage.getItem(key)
        return value === null || value === '' ? fallback : value
      } catch (error) {
        return fallback
      }
    }

    /**
     * 写入 localStorage（失败静默）。
     * @param {string} key 存储键。
     * @param {string} value 值。
     * @returns {void}
     */
    function writeStored(key, value) {
      try {
        window.localStorage.setItem(key, value)
      } catch (error) {
        // 存储不可用只影响持久化，不影响面板本身。
      }
    }

    /**
     * 黑板值 → 文本（与桥接 K5 文本同口径；面板单文件不能 import）。
     * @param {unknown} value 黑板条目的 from/to。
     * @returns {string} 文本。
     */
    function valueText(value) {
      if (value === undefined || value === null) return 'null'
      if (typeof value === 'string') return value
      const encoded = JSON.stringify(value)
      return encoded === undefined ? 'null' : encoded
    }

    /**
     * 一条黑板变化 → `path：from → to`；非法条目返回 `''`。
     * @param {unknown} entry K1 黑板条目。
     * @returns {string} 行文本。
     */
    function entryLine(entry) {
      if (entry === null || typeof entry !== 'object') return ''
      const path = typeof entry.path === 'string' && entry.path.trim() !== '' ? entry.path : ''
      if (path === '') return ''
      return `${path}：${valueText(entry.from)} → ${valueText(entry.to)}`
    }

    /**
     * 黑板条目的规则 id 列表（非字符串/空串丢弃）。
     * @param {unknown} entry K1 黑板条目。
     * @returns {string[]} 规则 id。
     */
    function entryRules(entry) {
      const ids = entry !== null && typeof entry === 'object' && Array.isArray(entry.rule_ids) ? entry.rule_ids : []
      return ids.filter((id) => typeof id === 'string' && id !== '')
    }

    /**
     * 黑板 → 展示模型；非法返回 null。
     * @param {unknown} raw K1 黑板。
     * @returns {{entity: string, version: number, updatedAt: string, entries: Array<{line: string, rules: string[]}>} | null} 展示模型。
     */
    function normalizeBoard(raw) {
      if (raw === null || typeof raw !== 'object' || Array.isArray(raw)) return null
      const entity = typeof raw.entity === 'string' && raw.entity.trim() !== '' ? raw.entity : ''
      if (entity === '') return null
      const rows = Array.isArray(raw.entries) ? raw.entries : []
      const entries = []
      for (const entry of rows) {
        const line = entryLine(entry)
        if (line !== '') entries.push({ line, rules: entryRules(entry) })
      }
      return {
        entity,
        version: Number.isInteger(raw.version) ? raw.version : 0,
        updatedAt: typeof raw.updated_at === 'string' && raw.updated_at !== '' ? raw.updated_at : '',
        entries,
      }
    }

    /**
     * `GET /astrbox/blackboard` 响应 → `{ok, boards, error}`；非法/失败按不可用处理（K8）。
     * @param {unknown} wire 响应体。
     * @returns {{ok: boolean, boards: Array<{entity: string, board: object|null}>, error: string}} 解析结果。
     */
    function parseBoardsWire(wire) {
      if (wire === null || typeof wire !== 'object' || wire.ok !== true) return { ok: false, boards: [], error: '' }
      if (wire.boards !== undefined && !Array.isArray(wire.boards)) return { ok: false, boards: [], error: '' }
      const list = Array.isArray(wire.boards) ? wire.boards : []
      const boards = []
      for (const item of list) {
        if (item === null || typeof item !== 'object') continue
        const entity = typeof item.entity === 'string' && item.entity !== '' ? item.entity : ''
        if (entity === '') continue
        boards.push({ entity, board: normalizeBoard(item.board) })
      }
      return { ok: true, boards, error: '' }
    }

    /**
     * RFC3339 → 本地 `HH:MM:SS`；不可解析返回 `''`。
     * @param {unknown} iso 时间字符串。
     * @returns {string} 时间文本。
     */
    function formatClockTime(iso) {
      if (typeof iso !== 'string' || iso === '') return ''
      const date = new Date(iso)
      if (Number.isNaN(date.getTime())) return ''
      const pad = (value) => String(value).padStart(2, '0')
      return `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
    }

    /**
     * 同源拉取全部实体黑板；任何失败都回成结果对象，不抛错。
     * @returns {Promise<{ok: boolean, boards: Array<object>, error: string}>} 拉取结果。
     */
    async function fetchBoardsWire() {
      if (typeof fetch !== 'function') return { ok: false, boards: [], error: 'fetch unavailable' }
      try {
        const response = await fetch(BOARDS_PATH, { cache: 'no-store' })
        if (response === null || typeof response !== 'object') return { ok: false, boards: [], error: 'empty response' }
        const status = typeof response.status === 'number' ? response.status : 0
        if (status !== 200) return { ok: false, boards: [], error: `HTTP ${status}` }
        return parseBoardsWire(await response.json())
      } catch (error) {
        return { ok: false, boards: [], error: errorText(error) }
      }
    }

    /**
     * 黑板轮询 hook：立即拉一次，之后每 10s 一次；页面隐藏时暂停，恢复可见时立刻补拉。
     * 接口不可用只置为 unavailable（面板显示「黑板不可用」），不抛错、不崩溃。
     * @returns {{status: string, boards: Array<object>, error: string, refresh: Function}} 黑板状态。
     */
    function useBoards() {
      const [state, setState] = useState({ status: 'loading', boards: [], error: '' })
      const [reload, setReload] = useState(0)
      useEffect(() => {
        let alive = true
        let timer
        const load = async () => {
          const result = await fetchBoardsWire()
          if (!alive) return
          setState(
            result.ok
              ? { status: 'ready', boards: result.boards, error: '' }
              : { status: 'unavailable', boards: [], error: result.error },
          )
        }
        const stop = () => {
          if (timer !== undefined) {
            window.clearInterval(timer)
            timer = undefined
          }
        }
        const start = () => {
          stop()
          void load()
          timer = window.setInterval(load, BOARDS_REFRESH_MS)
        }
        const hidden = () => typeof document !== 'undefined' && document.hidden
        const onVisibility = () => {
          if (hidden()) stop()
          else start()
        }
        if (hidden()) stop()
        else start()
        if (typeof document !== 'undefined') document.addEventListener('visibilitychange', onVisibility)
        return () => {
          alive = false
          stop()
          if (typeof document !== 'undefined') document.removeEventListener('visibilitychange', onVisibility)
        }
      }, [reload])
      const refresh = useCallback(() => setReload((value) => value + 1), [])
      return { ...state, refresh }
    }

    /**
     * 左侧栏面板图标：shell 负责按钮、标签与几何，这里只画猫脸轮廓。
     * @param {{size: number, active: boolean}} props shell 给的图标份额。
     * @returns {React.ReactElement} 图标。
     */
    function NekoPanelIcon({ size }) {
      return h('svg', {
        'data-dsh-panel-entry': PANEL_ID,
        viewBox: '0 0 16 16',
        width: size,
        height: size,
        fill: 'none',
        stroke: 'currentColor',
        strokeWidth: 1.3,
        strokeLinecap: 'round',
        strokeLinejoin: 'round',
        'aria-hidden': true,
      },
        h('path', { d: 'M3 6.2 2.2 2.8l3.1 1.7' }),
        h('path', { d: 'M13 6.2l.8-3.4-3.1 1.7' }),
        h('path', { d: 'M8 4.4c2.9 0 5 1.9 5 4.4S10.9 13 8 13 3 11.3 3 8.8s2.1-4.4 5-4.4Z' }),
        h('path', { d: 'M6.2 8.4h.01M9.8 8.4h.01' }),
        h('path', { d: 'M6.9 10.4c.7.5 1.5.5 2.2 0' }),
      )
    }

    /**
     * 状态区：内嵌 Neko Monitor 页面 + 地址编辑/刷新/外链。
     * @param {{t: Function}} props 组件份额。
     * @returns {React.ReactElement} 状态区。
     */
    function StatusPane({ t }) {
      const [url, setUrl] = useState(() => readStored(STORAGE_MONITOR, DEFAULT_MONITOR_URL))
      const [attempt, setAttempt] = useState(0)
      const [loaded, setLoaded] = useState(false)
      const applyUrl = useCallback((next) => {
        setUrl(next)
        writeStored(STORAGE_MONITOR, next)
        setLoaded(false)
        setAttempt(value => value + 1)
      }, [])
      const reload = useCallback(() => {
        setLoaded(false)
        setAttempt(value => value + 1)
      }, [])
      const openExternal = useCallback(() => {
        window.open(url, '_blank', 'noopener,noreferrer')
      }, [url])
      return h('section', { className: 'dsh-neko-status', 'data-dsh-neko-pane': 'status' },
        h('header', { className: 'dsh-neko-paneHead' },
          h('strong', { className: 'dsh-neko-paneTitle' }, t('status.title')),
          h('input', {
            className: 'dsh-neko-input',
            value: url,
            spellCheck: false,
            'aria-label': t('status.url'),
            onChange: event => setUrl(event.target.value),
            onKeyDown: event => {
              if (event.key === 'Enter') applyUrl(event.currentTarget.value)
            },
            onBlur: event => {
              if (event.currentTarget.value !== readStored(STORAGE_MONITOR, DEFAULT_MONITOR_URL)) applyUrl(event.currentTarget.value)
            },
          }),
          h('button', { type: 'button', className: 'dsh-neko-button', onClick: reload }, t('status.refresh')),
          h('button', { type: 'button', className: 'dsh-neko-button', onClick: openExternal }, t('status.open')),
        ),
        h('iframe', {
          key: attempt,
          className: 'dsh-neko-frame',
          src: url,
          title: t('status.title'),
          sandbox: 'allow-same-origin allow-scripts allow-forms allow-popups',
          onLoad: () => setLoaded(true),
        }),
        loaded ? null : h('p', { className: 'dsh-neko-hint' }, t('status.hint')),
      )
    }

    /**
     * 一块实体黑板卡片：实体名、版本、更新时间、变化行（规则小字）。
     * @param {{item: {entity: string, board: object|null}, t: Function}} props 组件份额。
     * @returns {React.ReactElement} 卡片。
     */
    function BoardCard({ item, t }) {
      const { entity, board } = item
      const head = h('header', { className: 'dsh-neko-boardHead' },
        h('strong', { className: 'dsh-neko-boardEntity' }, entity),
        board === null ? null : h('span', { className: 'dsh-neko-badge' }, t('board.version', { version: String(board.version) })),
        board === null || board.updatedAt === '' ? null
          : h('span', { className: 'dsh-neko-boardTime' }, t('board.updated', { time: formatClockTime(board.updatedAt) || board.updatedAt })),
      )
      let body
      if (board === null || board.entries.length === 0) {
        body = h('p', { className: 'dsh-neko-hint' }, t('board.noChanges'))
      } else {
        body = h('ul', { className: 'dsh-neko-boardList' }, board.entries.map((entry, index) => h('li', {
          key: `${entity}:${index}`,
          className: 'dsh-neko-boardRow',
        },
          h('span', { className: 'dsh-neko-boardChange' }, entry.line),
          entry.rules.length === 0 ? null
            : h('span', { className: 'dsh-neko-boardRule' }, t('board.rules', { rules: entry.rules.join('、') })),
        )))
      }
      return h('section', { className: 'dsh-neko-boardCard', 'data-dsh-neko-board': entity }, head, body)
    }

    /**
     * 事件黑板区：标题/状态/刷新 + 每实体一张卡片；接口失败显示「黑板不可用」。
     * @param {{t: Function}} props 组件份额。
     * @returns {React.ReactElement} 黑板区。
     */
    function BoardPane({ t }) {
      const state = useBoards()
      const head = h('header', { className: 'dsh-neko-paneHead' },
        h('strong', { className: 'dsh-neko-paneTitle' }, t('board.title')),
        h('span', { className: 'dsh-neko-badge', 'data-dsh-neko-boards': state.status },
          state.status === 'ready' ? t('board.ready') : state.status === 'loading' ? t('board.loading') : t('board.unavailable')),
        h('button', { type: 'button', className: 'dsh-neko-button', onClick: state.refresh }, t('board.refresh')),
      )
      let body
      if (state.status === 'loading') {
        body = h('p', { className: 'dsh-neko-hint' }, t('board.loading'))
      } else if (state.status === 'unavailable') {
        body = h('div', { className: 'dsh-neko-boardBody' },
          h('p', { className: 'dsh-neko-hint', 'data-dsh-neko-boards': 'unavailable' }, t('board.unavailable')),
          state.error === '' ? null : h('p', { className: 'dsh-neko-error' }, t('board.error', { error: state.error })),
        )
      } else if (state.boards.length === 0) {
        body = h('p', { className: 'dsh-neko-hint' }, t('board.empty'))
      } else {
        body = h('div', { className: 'dsh-neko-boardBody' },
          state.boards.map(item => h(BoardCard, { key: item.entity, item, t })),
        )
      }
      return h('section', { className: 'dsh-neko-board', 'data-dsh-neko-pane': 'boards' }, head, body)
    }

    /**
     * 主列页面：状态区与事件黑板区左右分栏。
     * @param {{t: Function}} props 注册时注入的份额。
     * @returns {React.ReactElement} 面板。
     */
    function NekoPanel({ t }) {
      return h('div', { className: 'dsh-neko-panel', 'data-dsh-plugin': 'astrbox' },
        h(StatusPane, { t }),
        h(BoardPane, { t }),
      )
    }

    /** 面板样式：颜色只引用 DSH 主题令牌，其余按主列既有页面的密度取值。 */
    const PANEL_CSS = `
.dsh-neko-panel { display:flex; height:100%; min-height:0; background:var(--dsw-alias-bg-base); color:var(--dsw-alias-label-primary); }
.dsh-neko-status, .dsh-neko-board { display:flex; flex-direction:column; min-width:0; min-height:0; }
.dsh-neko-status { flex:1 1 58%; }
.dsh-neko-board { flex:1 1 42%; min-width:300px; border-left:1px solid var(--dsw-alias-border-l1); background:var(--dsw-alias-bg-layer-1); }
.dsh-neko-paneHead { display:flex; align-items:center; gap:8px; padding:8px 10px; border-bottom:1px solid var(--dsw-alias-border-l1); }
.dsh-neko-paneTitle { font-size:13px; font-weight:600; white-space:nowrap; }
.dsh-neko-input { flex:1 1 auto; min-width:0; border:1px solid var(--dsw-alias-border-l1); border-radius:6px; background:var(--dsw-alias-bg-base); color:var(--dsw-alias-label-primary); font:inherit; font-size:12px; padding:4px 8px; }
.dsh-neko-button { flex:0 0 auto; border:1px solid var(--dsw-alias-border-l1); border-radius:6px; background:transparent; color:var(--dsw-alias-label-primary); font:inherit; font-size:12px; padding:4px 10px; cursor:pointer; white-space:nowrap; }
.dsh-neko-button:hover:not(:disabled) { background:var(--dsw-alias-bg-layer-2); }
.dsh-neko-frame { flex:1 1 auto; width:100%; min-height:0; border:0; background:var(--dsw-alias-bg-base); }
.dsh-neko-hint { margin:0; padding:8px 10px; font-size:12px; line-height:1.5; color:var(--dsw-alias-label-secondary); }
.dsh-neko-error { margin:0; padding:0 10px 6px; font-size:12px; color:var(--dsw-alias-state-error-primary); }
.dsh-neko-badge { flex:0 0 auto; font-size:11px; padding:1px 6px; border-radius:999px; border:1px solid var(--dsw-alias-border-l1); color:var(--dsw-alias-label-secondary); }
.dsh-neko-badge[data-dsh-neko-boards="ready"] { color:var(--dsw-alias-state-success-primary); border-color:var(--dsw-alias-state-success-primary); }
.dsh-neko-badge[data-dsh-neko-boards="unavailable"] { color:var(--dsw-alias-state-error-primary); border-color:var(--dsw-alias-state-error-primary); }
.dsh-neko-boardBody { flex:1 1 auto; min-height:0; overflow-y:auto; padding:10px; display:flex; flex-direction:column; gap:8px; }
.dsh-neko-boardCard { border:1px solid var(--dsw-alias-border-l1); border-radius:8px; background:var(--dsw-alias-bg-base); overflow:hidden; }
.dsh-neko-boardHead { display:flex; align-items:center; gap:8px; padding:6px 10px; border-bottom:1px solid var(--dsw-alias-border-l1); background:var(--dsw-alias-bg-layer-2); }
.dsh-neko-boardEntity { font-size:13px; font-weight:600; }
.dsh-neko-boardTime { margin-left:auto; font-size:11px; color:var(--dsw-alias-label-secondary); white-space:nowrap; }
.dsh-neko-boardList { margin:0; padding:6px 10px 8px; list-style:none; display:flex; flex-direction:column; gap:4px; }
.dsh-neko-boardRow { display:flex; flex-direction:column; gap:1px; font-size:12px; line-height:1.5; overflow-wrap:anywhere; }
.dsh-neko-boardChange { color:var(--dsw-alias-label-primary); }
.dsh-neko-boardRule { font-size:11px; color:var(--dsw-alias-label-secondary); }
@media (max-width:880px) { .dsh-neko-panel { flex-direction:column; } .dsh-neko-board { border-left:0; border-top:1px solid var(--dsw-alias-border-l1); min-width:0; } }
`

    /**
     * 注入面板样式表；卸载时移除。
     * @param {import('@deepseek-ai/cordis').Context} ctx 客户端根上下文。
     * @returns {void}
     */
    function installStyles(ctx) {
      ctx.effect(() => {
        const style = document.createElement('style')
        style.setAttribute('data-dsh-neko-panel', '')
        style.textContent = PANEL_CSS
        document.head.append(style)
        return () => {
          style.remove()
        }
      }, 'neko-panel: styles')
    }

    /**
     * 注册文案字典并返回翻译函数；没有 locale 服务时用内置字典。
     * @param {import('@deepseek-ai/cordis').Context} ctx 客户端根上下文。
     * @returns {(key: string, params?: Record<string, string>) => string} 翻译函数。
     */
    function installLocale(ctx) {
      const locale = ctx.get('locale')
      if (locale === undefined) return localTranslate
      try {
        ctx.effect(() => locale.register(NS, { zh: ZH, en: EN }), 'neko-panel: dictionaries')
        const bound = locale.bind(NS)
        return (key, params) => bound(key, params)
      } catch (error) {
        return localTranslate
      }
    }

    /**
     * 插件入口：注册左侧栏行与主列页面，两者都等宿主声明槽位后再挂。
     * @param {import('@deepseek-ai/cordis').Context} ctx 客户端根上下文。
     * @returns {() => void} 释放函数。
     */
    function apply(ctx) {
      installStyles(ctx)
      const translate = installLocale(ctx)
      const face = { t: translate }
      const disposers = []
      disposers.push(ctx.slots.inject('sidebar.panellist', () => ctx.slots.register({
        name: 'sidebar.panellist',
        id: PANEL_ID,
        order: PANEL_ORDER,
        label: () => translate('entry.label'),
      }, NekoPanelIcon)))
      disposers.push(ctx.slots.inject('main', () => ctx.slots.register({
        name: 'main',
        key: PANEL_ID,
        inject: () => face,
      }, NekoPanel)))
      return () => {
        for (const dispose of disposers.splice(0)) {
          try {
            dispose()
          } catch (error) {
            // 槽位已随宿主卸载时，释放是幂等的。
          }
        }
      }
    }

    return {
      inject: ['slots'],
      apply,
      /** 仅单测用：浏览器半纯函数与内置字典出口；不改 apply/inject 形状。 */
      __test: {
        parseBoardsWire,
        normalizeBoard,
        entryLine,
        entryRules,
        valueText,
        formatClockTime,
        dictionaries: { zh: ZH, en: EN },
      },
    }
  },
})
