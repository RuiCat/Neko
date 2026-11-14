/**
 * Neko 面板 · dsh-astrbox 的浏览器半（客户端插件）。
 *
 * code-modularity: no-split —— 交付契约要求本插件是**单文件浏览器插件**（宿主按
 * `/plugins/dsh-astrbox/client.js` 整体加载本文件，`window.__ModuleLoader__.load` 登记
 * 工厂，不支持 import、无构建步骤），拆成多文件会破坏加载契约；纯函数已通过
 * `__test` 出口独立单测（`dsh/lib/clientTest.mjs`）。技术债：面板继续增长（K28 又加了
 * 主会话提示词区）时应引入打包步骤把纯逻辑拆出（需改 `dsh/package.json` 的 dsh.client
 * 与构建入口，超出本文件写域）。
 *
 * 左侧栏全局面板入口（`sidebar.panellist`）+ 主列页面（`main`），自上而下三层：
 *  ① 状态快照条：生效实体（`/astrbox/blackboard` 的 `primary`）+ 白名单字段值 + 世界时钟；
 *  ② 主体：左 Neko Monitor iframe（可收起 / 可切全宽，选择持久化到 localStorage），
 *     右侧事件黑板（只有变化的实体出卡片，其余折叠成一行汇总）+ 控制区 + 主会话提示词（K28）；
 *  ③ 失败一律降级成可读提示，不空白、不崩溃。
 *
 * 视觉值只有一个来源：`.dsh-neko-panel` 上的 `--neko-*` 令牌块（颜色一律走宿主的
 * `--dsw-alias-*` 语义令牌，深色/浅色跟着宿主主题走，不做第二套配色）。
 *
 * 生效方式：DSH 启动时扫描 bundle 行的 `dsh.client` 声明，从
 * `/plugins/dsh-astrbox/client.js` 加载本工厂；新增/改动本文件后需重启 DSH。
 */
window.__ModuleLoader__.load({
  id: 'dsh-astrbox',
  factory(require) {
    const React = require('react')
    const h = React.createElement
    const { useCallback, useEffect, useMemo, useRef, useState } = React

    /** 面板 id：同时作为 sidebar.panellist 的行 id 与 main 槽位的 key。 */
    const PANEL_ID = 'neko-panel'
    /** 左侧栏全局面板行的顺序（任务看板 20，技能 30，SSH 40）。 */
    const PANEL_ORDER = 50
    /** 本插件文案命名空间。 */
    const NS = 'neko-panel'
    /** Neko Monitor 页面默认地址；面板里可改并持久化到 localStorage。 */
    const DEFAULT_MONITOR_URL = 'http://127.0.0.1:8765/monitor/'
    const STORAGE_MONITOR = 'dsh-neko-panel.monitor-url'
    /** 布局模式：左右分栏 / 收起监控页 / 监控页全宽；持久化到 localStorage。 */
    const STORAGE_LAYOUT = 'dsh-neko-panel.layout'
    /** 快照条查看的实体（空串 = 跟随桥接上报的生效实体）；持久化到 localStorage。 */
    const STORAGE_ENTITY = 'dsh-neko-panel.entity'
    /** 会话黑板「工作区板」选择的工作区键（K16）；持久化到 localStorage。 */
    const STORAGE_BOARD_WORKSPACE = 'dsh-neko-panel.boardWorkspace'
    /** 黑板区当前 Tab：事件黑板 / 会话黑板（K31b）。 */
    const STORAGE_BOARD_TAB = 'dsh-neko-panel.boardTab'
    const LAYOUT_MODES = Object.freeze(['split', 'monitor-hidden', 'monitor-full'])
    /** 黑板轮询间隔（K8：10s）；页面隐藏时暂停，恢复可见时立刻补拉。 */
    const BOARDS_REFRESH_MS = 10000
    /** 同源黑板数据路由（桥接注册，K7）；响应里同时带快照条元数据 primary/paths。 */
    const BOARDS_PATH = '/astrbox/blackboard'
    /** 同源控制面代理路由（桥接注册，K21）。 */
    const CONTROL_BASE = '/astrbox/control'
    /** 主会话提示词路由（桥接注册，K28）：按实体读写机器人主会话的补充提示词。 */
    const MAIN_PROMPT_PATH = '/astrbox/main-session-prompts'
    /** 会话黑板面板路由前缀（桥接注册，K15）：读板 / 工作区列表 / 面板写。 */
    const BOARD_BASE = '/astrbox/boards'
    /** 世界时钟：实体与其相对路径（K22）。 */
    const WORLD_ENTITY = '世界'
    const CLOCK_PATHS = Object.freeze(['环境.当前时间', '状态.阶段'])
    /** 快照条最多展示的字段数与单值截断长度（长任务名不该把状态条撑爆）。 */
    const MAX_SNAPSHOT_PATHS = 8
    const MAX_VALUE_CHARS = 28

    /** 中文文案（键集真源）。 */
    const ZH = {
      'entry.label': 'Neko 面板',
      'status.title': 'Neko 状态',
      'status.refresh': '刷新',
      'status.reload': '重新载入',
      'status.open': '新标签打开',
      'status.url': '状态界面地址',
      'status.hint': '状态界面来自 Neko Monitor；若空白，先在仓库里运行 make monitor。',
      'status.collapse': '收起监控',
      'status.expand': '展开监控',
      'status.full': '监控全宽',
      'status.unfold': '还原分栏',
      'snapshot.loading': '读取中…',
      'snapshot.unavailable': '状态不可用',
      'snapshot.empty': '暂无状态字段',
      'snapshot.noPrimary': '桥接未上报生效实体，快照不可用（重启 DSH 后恢复）',
      'snapshot.world': '世界',
      'snapshot.switch': '切换实体',
      'board.title': '事件黑板',
      'board.refresh': '刷新',
      'board.ready': '就绪',
      'board.loading': '加载中…',
      'board.unavailable': '黑板不可用',
      'board.error': '读取失败：{error}',
      'board.empty': '暂无实体黑板（等待状态机事件）',
      'board.noChanges': '暂无变化（等待状态机事件）',
      'board.primary': '生效实体',
      'board.version': '版本 {version}',
      'board.updated': '更新 {time}',
      'board.rules': '规则 ×{count}',
      'board.changed': '{count} 个有变化',
      'board.more': '另外 {count} 个实体暂无变化',
      'board.expand': '展开',
      'board.collapse': '收起',
      'control.title': '控制',
      'control.tick': '手动 tick',
      'control.add15': '+15 分钟',
      'control.add60': '+60 分钟',
      'control.time': '世界时间',
      'control.setTime': '设定',
      'control.unknown': '时间未知',
      'control.entity': '实体',
      'control.path': '路径',
      'control.value': '值（JSON）',
      'control.write': '写入',
      'control.pick': '选择实体',
      'control.done': '已执行',
      'control.badJson': '值不是合法 JSON：{error}',
      'control.badTime': '时间格式无效',
      'control.needFields': '请选择实体并填写路径',
      'mainPrompt.title': '主会话提示词',
      'mainPrompt.save': '保存',
      'mainPrompt.saved': '已保存，下一轮生效',
      'mainPrompt.hint': '按实体保存：生效于该实体对应机器人的主会话（主体），不影响 QQ 私聊/群聊。',
      'mainPrompt.placeholder': '给主会话的补充提示词（留空 = 不注入）',
      'mainPrompt.ready': '可编辑',
      'mainPrompt.unavailable': '不可用',
      'sessionBoard.title': '会话黑板',
      'sessionBoard.global': '跨工作区黑板',
      'sessionBoard.workspace': '工作区黑板',
      'sessionBoard.empty': '暂无区域',
      'sessionBoard.emptyRegion': '暂无条目',
      'sessionBoard.delete': '删除',
      'sessionBoard.clear': '清空',
      'sessionBoard.clearConfirm': '确认清空？',
      'sessionBoard.cancel': '取消',
      'sessionBoard.region': '区域',
      'sessionBoard.key': '键',
      'sessionBoard.value': '值',
      'sessionBoard.write': '写入',
      'sessionBoard.needFields': '请填写区域与键',
      'sessionBoard.workspacePick': '工作区',
      'sessionBoard.noWorkspace': '（未选择工作区）',
      'sessionBoard.pickWorkspace': '先选择工作区再读写',
    }

    /** 英文文案：与 ZH 同键集。 */
    const EN = {
      'entry.label': 'Neko panel',
      'status.title': 'Neko status',
      'status.refresh': 'Refresh',
      'status.reload': 'Reload',
      'status.open': 'Open in new tab',
      'status.url': 'Status view URL',
      'status.hint': 'The status view comes from Neko Monitor; run make monitor if it stays blank.',
      'status.collapse': 'Hide monitor',
      'status.expand': 'Show monitor',
      'status.full': 'Full-width monitor',
      'status.unfold': 'Back to split',
      'snapshot.loading': 'Loading…',
      'snapshot.unavailable': 'Status unavailable',
      'snapshot.empty': 'No state fields',
      'snapshot.noPrimary': 'The bridge reported no bound entity, so the snapshot is unavailable (restart DSH).',
      'snapshot.world': 'World',
      'snapshot.switch': 'Switch entity',
      'board.title': 'Event blackboard',
      'board.refresh': 'Refresh',
      'board.ready': 'Ready',
      'board.loading': 'Loading…',
      'board.unavailable': 'Blackboard unavailable',
      'board.error': 'Could not load: {error}',
      'board.empty': 'No entity blackboards yet (waiting for a state-machine event).',
      'board.noChanges': 'No changes yet (waiting for a state-machine event).',
      'board.primary': 'Primary',
      'board.version': 'Version {version}',
      'board.updated': 'Updated {time}',
      'board.rules': 'Rules ×{count}',
      'board.changed': '{count} changed',
      'board.more': '{count} more entities have no changes',
      'board.expand': 'Expand',
      'board.collapse': 'Collapse',
      'control.title': 'Control',
      'control.tick': 'Manual tick',
      'control.add15': '+15 min',
      'control.add60': '+60 min',
      'control.time': 'World time',
      'control.setTime': 'Set',
      'control.unknown': 'Unknown time',
      'control.entity': 'Entity',
      'control.path': 'Path',
      'control.value': 'Value (JSON)',
      'control.write': 'Write',
      'control.pick': 'Pick entity',
      'control.done': 'Done',
      'control.badJson': 'Not valid JSON: {error}',
      'control.badTime': 'Invalid time',
      'control.needFields': 'Pick an entity and fill the path',
      'mainPrompt.title': 'Main-session prompt',
      'mainPrompt.save': 'Save',
      'mainPrompt.saved': 'Saved; applies from the next turn',
      'mainPrompt.hint': 'Saved per entity: applies to the main session (主体) of the bot bound to it, not to QQ chats.',
      'mainPrompt.placeholder': 'Extra prompt for the main session (empty = not injected)',
      'mainPrompt.ready': 'Editable',
      'mainPrompt.unavailable': 'Unavailable',
      'sessionBoard.title': 'Session blackboard',
      'sessionBoard.global': 'Cross-workspace blackboard',
      'sessionBoard.workspace': 'Workspace blackboard',
      'sessionBoard.empty': 'No regions yet',
      'sessionBoard.emptyRegion': 'No entries yet',
      'sessionBoard.delete': 'Delete',
      'sessionBoard.clear': 'Clear',
      'sessionBoard.clearConfirm': 'Confirm clear?',
      'sessionBoard.cancel': 'Cancel',
      'sessionBoard.region': 'Region',
      'sessionBoard.key': 'Key',
      'sessionBoard.value': 'Value',
      'sessionBoard.write': 'Write',
      'sessionBoard.needFields': 'Fill in the region and key',
      'sessionBoard.workspacePick': 'Workspace',
      'sessionBoard.noWorkspace': '(no workspace selected)',
      'sessionBoard.pickWorkspace': 'Pick a workspace before reading or writing',
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
     * 黑板值 / 状态值 → 文本（与桥接 K5 文本同口径；面板单文件不能 import）。
     * @param {unknown} value 值。
     * @returns {string} 文本。
     */
    function valueText(value) {
      if (value === undefined || value === null) return 'null'
      if (typeof value === 'string') return value
      const encoded = JSON.stringify(value)
      return encoded === undefined ? 'null' : encoded
    }

    /**
     * 点分路径取末段（快照条标签）：`状态.情绪` → `情绪`；非法返回 `''`。
     * @param {unknown} path 点分路径。
     * @returns {string} 末段。
     */
    function lastPathSegment(path) {
      if (typeof path !== 'string') return ''
      const parts = path.split('.')
      const last = parts[parts.length - 1]
      return typeof last === 'string' ? last.trim() : ''
    }

    /**
     * 截断长文本（保留可辨识前缀）；非法输入返回 `''`。
     * @param {unknown} text 原文。
     * @param {number} max 字符上限。
     * @returns {string} 截断结果。
     */
    function truncateText(text, max) {
      const source = typeof text === 'string' ? text : ''
      if (!Number.isFinite(max) || max <= 0 || source.length <= max) return source
      return `${source.slice(0, max)}…`
    }

    /**
     * 一条黑板变化 → `path：from → to`；`from` 缺失（新字段）时只显示当前值。
     * 非法条目返回 `''`。
     * @param {unknown} entry K1 黑板条目。
     * @returns {string} 行文本。
     */
    function entryLine(entry) {
      if (entry === null || typeof entry !== 'object') return ''
      const path = typeof entry.path === 'string' && entry.path.trim() !== '' ? entry.path : ''
      if (path === '') return ''
      if (entry.from === null || entry.from === undefined) return `${path}：${valueText(entry.to)}`
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
     * RFC3339 → 本地 `MM-DD HH:MM`（卡片内逐条变化的时间戳）；不可解析返回 `''`。
     * @param {unknown} iso 时间字符串。
     * @returns {string} 时间文本。
     */
    function formatShortTime(iso) {
      if (typeof iso !== 'string' || iso === '') return ''
      const date = new Date(iso)
      if (Number.isNaN(date.getTime())) return ''
      const pad = (value) => String(value).padStart(2, '0')
      return `${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`
    }

    /**
     * 一条黑板变化的时间戳文本（相对字段 `updated_at`）。
     * @param {unknown} entry K1 黑板条目。
     * @returns {string} 时间文本。
     */
    function entryTime(entry) {
      const updatedAt = entry !== null && typeof entry === 'object' ? entry.updated_at : undefined
      return formatShortTime(updatedAt)
    }

    /**
     * 黑板 → 展示模型；非法返回 null。
     * @param {unknown} raw K1 黑板。
     * @returns {{entity: string, version: number, updatedAt: string, entries: Array<{line: string, rules: string[], time: string}>} | null} 展示模型。
     */
    function normalizeBoard(raw) {
      if (raw === null || typeof raw !== 'object' || Array.isArray(raw)) return null
      const entity = typeof raw.entity === 'string' && raw.entity.trim() !== '' ? raw.entity : ''
      if (entity === '') return null
      const rows = Array.isArray(raw.entries) ? raw.entries : []
      const entries = []
      for (const entry of rows) {
        const line = entryLine(entry)
        if (line === '') continue
        entries.push({ line, rules: entryRules(entry), time: entryTime(entry) })
      }
      return {
        entity,
        version: Number.isInteger(raw.version) ? raw.version : 0,
        updatedAt: typeof raw.updated_at === 'string' && raw.updated_at !== '' ? raw.updated_at : '',
        entries,
      }
    }

    /**
     * `GET /astrbox/blackboard` 响应 → `{ok, boards, primary, paths, error}`；
     * 非法/失败按不可用处理（K8），快照条元数据非法时退化为空（只少显示快照条）。
     * @param {unknown} wire 响应体。
     * @returns {{ok: boolean, boards: Array<{entity: string, board: object|null}>, primary: string, paths: string[], error: string}} 解析结果。
     */
    function parseBoardsWire(wire) {
      const unavailable = { ok: false, boards: [], primary: '', paths: [], error: '' }
      if (wire === null || typeof wire !== 'object' || wire.ok !== true) return unavailable
      if (wire.boards !== undefined && !Array.isArray(wire.boards)) return unavailable
      const list = Array.isArray(wire.boards) ? wire.boards : []
      const boards = []
      for (const item of list) {
        if (item === null || typeof item !== 'object') continue
        const entity = typeof item.entity === 'string' && item.entity !== '' ? item.entity : ''
        if (entity === '') continue
        boards.push({ entity, board: normalizeBoard(item.board) })
      }
      const primary = typeof wire.primary === 'string' && wire.primary.trim() !== '' ? wire.primary.trim() : ''
      const paths = []
      if (Array.isArray(wire.paths)) {
        for (const path of wire.paths) {
          if (typeof path !== 'string' || path.trim() === '') continue
          paths.push(path.trim())
          if (paths.length >= MAX_SNAPSHOT_PATHS) break
        }
      }
      return { ok: true, boards, primary, paths, error: '' }
    }

    /**
     * 控制面 `GET /state` 响应 → `{ok, values, error}`；`values` 只接受普通对象。
     * @param {unknown} wire 响应体。
     * @returns {{ok: boolean, values: Record<string, unknown>, error: string}} 解析结果。
     */
    function parseStateWire(wire) {
      if (wire === null || typeof wire !== 'object' || wire.ok !== true) return { ok: false, values: {}, error: '' }
      const values = wire.values !== null && typeof wire.values === 'object' && !Array.isArray(wire.values) ? wire.values : {}
      return { ok: true, values, error: '' }
    }

    /**
     * 快照条可切换的实体：生效实体置顶，其余取黑板里的实体（去重、保持桥接顺序）。
     * 候选只来自板桥接已暴露的实体，不去读全量实体目录（避免把 53 个场景塞进切换器）。
     * @param {unknown} primary 生效实体 id。
     * @param {unknown} boards `{entity, board}` 列表。
     * @returns {string[]} 候选实体 id。
     */
    function entityCandidates(primary, boards) {
      const seen = new Set()
      const list = []
      const push = (value) => {
        const entity = typeof value === 'string' ? value.trim() : ''
        if (entity === '' || seen.has(entity)) return
        seen.add(entity)
        list.push(entity)
      }
      push(primary)
      for (const item of Array.isArray(boards) ? boards : []) {
        if (item !== null && typeof item === 'object') push(item.entity)
      }
      return list
    }

    /**
     * 快照条展示模型：白名单路径 → `{label, text}`（缺值显示 `—`，长值截断）。
     * @param {unknown} paths 白名单路径。
     * @param {unknown} values 控制面 values。
     * @returns {Array<{path: string, label: string, text: string, title: string, missing: boolean}>} 展示行。
     */
    function snapshotRows(paths, values) {
      const rows = []
      const source = Array.isArray(paths) ? paths : []
      const map = values !== null && typeof values === 'object' && !Array.isArray(values) ? values : {}
      for (const path of source) {
        if (rows.length >= MAX_SNAPSHOT_PATHS) break
        const label = lastPathSegment(path)
        if (label === '') continue
        const has = Object.prototype.hasOwnProperty.call(map, path)
        const full = has ? valueText(map[path]) : '—'
        rows.push({ path, label, text: truncateText(full, MAX_VALUE_CHARS), title: `${path}：${full}`, missing: !has })
      }
      return rows
    }

    /**
     * 黑板分区：有变化的实体出卡片（生效实体置顶，其余按更新时间倒序），空黑板折叠成汇总。
     * @param {unknown} items `{entity, board}` 列表。
     * @param {unknown} primary 生效实体 id。
     * @returns {{changed: Array<{entity: string, board: object}>, empty: Array<{entity: string, board: object|null}>}} 分区。
     */
    function partitionBoards(items, primary) {
      const changed = []
      const empty = []
      const list = Array.isArray(items) ? items : []
      for (const item of list) {
        if (item === null || typeof item !== 'object') continue
        const entity = typeof item.entity === 'string' ? item.entity : ''
        if (entity === '') continue
        const board = item.board !== null && typeof item.board === 'object' && Array.isArray(item.board.entries) ? item.board : null
        if (board !== null && board.entries.length > 0) changed.push({ entity, board })
        else empty.push({ entity, board: item.board === undefined ? null : item.board })
      }
      const updatedAt = (item) => {
        const value = item.board !== null && typeof item.board === 'object' ? item.board.updatedAt : ''
        const time = Date.parse(typeof value === 'string' ? value : '')
        return Number.isNaN(time) ? 0 : time
      }
      const rank = (item) => (item.entity === primary ? 0 : 1)
      changed.sort((left, right) => {
        const byPrimary = rank(left) - rank(right)
        if (byPrimary !== 0) return byPrimary
        const byTime = updatedAt(right) - updatedAt(left)
        if (byTime !== 0) return byTime
        return left.entity < right.entity ? -1 : left.entity > right.entity ? 1 : 0
      })
      empty.sort((left, right) => (left.entity < right.entity ? -1 : left.entity > right.entity ? 1 : 0))
      return { changed, empty }
    }

    /**
     * 布局模式收敛：非法值回落 `split`。
     * @param {unknown} value 存储值。
     * @returns {string} 合法布局模式。
     */
    function sanitizeLayoutMode(value) {
      return LAYOUT_MODES.includes(value) ? value : 'split'
    }

    /**
     * 黑板 Tab 的合法值：`event`（事件黑板）或 `session`（会话黑板）；其它一律回 `event`。
     * @param {unknown} value 原始存储值
     * @returns {string} 合法 Tab
     */
    function sanitizeBoardTab(value) {
      return value === 'session' ? 'session' : 'event'
    }

    /**
     * 布局按钮的下一状态：监控页在「分栏 / 收起」与「分栏 / 全宽」之间互斥切换。
     * @param {unknown} mode 当前模式。
     * @param {string} action `toggleMonitor` 或 `toggleFull`。
     * @returns {string} 下一模式。
     */
    function nextLayoutMode(mode, action) {
      const current = sanitizeLayoutMode(mode)
      if (action === 'toggleMonitor') return current === 'monitor-hidden' ? 'split' : 'monitor-hidden'
      if (action === 'toggleFull') return current === 'monitor-full' ? 'split' : 'monitor-full'
      return current
    }

    /** datetime-local → 本地时区 RFC3339；非法输入返回 `''`（K22）。 */
    function toRfc3339(localValue) {
      const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})(?::(\d{2}))?$/.exec(String(localValue ?? '').trim())
      if (match === null) return ''
      const [, year, month, day, hour, minute, second = '00'] = match
      const date = new Date(`${year}-${month}-${day}T${hour}:${minute}:${second}`)
      if (Number.isNaN(date.getTime())) return ''
      const offset = -date.getTimezoneOffset()
      const pad = (value) => String(value).padStart(2, '0')
      const abs = Math.abs(offset)
      return `${year}-${month}-${day}T${hour}:${minute}:${second}${offset >= 0 ? '+' : '-'}${pad(Math.floor(abs / 60))}:${pad(abs % 60)}`
    }

    /** 文本框 → JSON 值；空文本/非法 JSON 返回 `{ok:false,error}`（K22）。 */
    function parseJsonValue(text) {
      const source = typeof text === 'string' ? text.trim() : ''
      if (source === '') return { ok: false, value: undefined, error: 'empty' }
      try { return { ok: true, value: JSON.parse(source), error: '' } }
      catch (error) { return { ok: false, value: undefined, error: errorText(error) } }
    }

    /** 组装 `POST /astrbox/control/state` 请求体；字段缺失返回 null（K19/K22）。 */
    function statePayload(entity, path, value) {
      if (typeof entity !== 'string' || entity.trim() === '' || typeof path !== 'string' || path.trim() === '') return null
      return { entity: entity.trim(), path: path.trim(), value }
    }

    /**
     * 提示词响应体 → 归一化结果（K28）：过滤空实体/非字符串/空文本，保留 `maxChars`。
     * @param {unknown} body 响应体
     * @returns {{ok: boolean, prompts: Record<string, string>, maxChars: number, error: string}} 解析结果
     */
    function parseMainPromptsWire(body) {
      if (body === null || typeof body !== 'object' || body.ok !== true) {
        const message = body !== null && typeof body === 'object' && body.error !== null && typeof body.error === 'object' ? body.error.message : 'invalid response'
        return { ok: false, prompts: {}, maxChars: 0, error: errorText(message) }
      }
      const prompts = {}
      const source = body.prompts
      if (source !== null && typeof source === 'object' && !Array.isArray(source)) {
        for (const [entity, text] of Object.entries(source)) {
          if (typeof entity !== 'string' || entity.trim() === '') continue
          if (typeof text !== 'string') continue
          const trimmed = text.trim()
          if (trimmed !== '') prompts[entity.trim()] = trimmed
        }
      }
      const maxChars = Number.isInteger(body.maxChars) && body.maxChars > 0 ? body.maxChars : 0
      return { ok: true, prompts, maxChars, error: '' }
    }

    /**
     * 组装 `POST /astrbox/main-session-prompts` 请求体（K28）；未选实体返回 null。
     * @param {string} entity 实体 id
     * @param {string} text 提示词文本（空文本 = 清除该实体）
     * @returns {{entity: string, prompt: string} | null} 请求体
     */
    function mainPromptPayload(entity, text) {
      if (typeof entity !== 'string' || entity.trim() === '') return null
      return { entity: entity.trim(), prompt: typeof text === 'string' ? text.trim() : '' }
    }

    /**
     * 取某实体的提示词文本（K28）；未配置/形状不对返回空串。
     * @param {Record<string, string>} prompts 提示词表
     * @param {string} entity 实体 id
     * @returns {string} 提示词文本
     */
    function promptValueOf(prompts, entity) {
      if (prompts === null || typeof prompts !== 'object') return ''
      if (typeof entity !== 'string' || entity.trim() === '') return ''
      const text = prompts[entity.trim()]
      return typeof text === 'string' ? text : ''
    }

    /**
     * 会话黑板值文本 → 写入值（K16）：先按 JSON 解析，失败按原始字符串。
     * @param {unknown} text 文本域内容
     * @returns {unknown} 写入值（空文本 = 空字符串）
     */
    function parseSessionBoardValue(text) {
      const source = typeof text === 'string' ? text : ''
      const trimmed = source.trim()
      if (trimmed === '') return source
      try { return JSON.parse(trimmed) } catch { return source }
    }

    /**
     * 会话黑板条目文本 `key：value（by）`（K16）；非法/已删除条目返回空串。
     * @param {unknown} entry K1 条目
     * @returns {string} 行文本
     */
    function sessionBoardEntryLine(entry) {
      if (entry === null || typeof entry !== 'object' || entry.deleted === true) return ''
      const key = typeof entry.key === 'string' ? entry.key.trim() : ''
      if (key === '') return ''
      return `${key}：${valueText(entry.to)}${typeof entry.by === 'string' && entry.by !== '' ? `（${entry.by}）` : ''}`
    }

    /**
     * 目标区解析（K13/K16）：global 板区域=工作区键；workspace 板区域 id 与工作区键分开。
     * @param {unknown} scope 板 scope
     * @param {unknown} workspace 工作区键
     * @param {unknown} region 区域 id
     * @returns {{scope: string, workspace: string, region: string} | null} 目标或 null
     */
    function sessionBoardTarget(scope, workspace, region) {
      if (scope !== 'workspace' && scope !== 'global') return null
      const key = typeof workspace === 'string' ? workspace.trim() : ''
      const id = typeof region === 'string' ? region.trim() : ''
      if (scope === 'workspace') return key === '' || id === '' ? null : { scope, workspace: key, region: id }
      const target = id !== '' ? id : key
      return target === '' ? null : { scope, workspace: target, region: target }
    }

    /**
     * 组装 `POST /astrbox/boards/panel` 请求体（K13）；目标或 ops 非法返回 null。
     * @param {string} scope 板 scope
     * @param {string} workspace 工作区键
     * @param {string} region 区域 id
     * @param {object[]} ops K13 操作
     * @returns {object | null} 请求体
     */
    function sessionBoardOpsPayload(scope, workspace, region, ops) {
      const target = sessionBoardTarget(scope, workspace, region)
      if (target === null || !Array.isArray(ops) || ops.length === 0) return null
      return { ...target, ops }
    }

    /**
     * 组装一条 `set` 写请求（K16）；key 为空返回 null。
     * @param {string} scope 板 scope
     * @param {string} workspace 工作区键
     * @param {string} region 区域 id
     * @param {unknown} key 键
     * @param {unknown} text 值文本（先 JSON 后字符串）
     * @returns {object | null} 请求体
     */
    function sessionBoardSetPayload(scope, workspace, region, key, text) {
      const name = typeof key === 'string' ? key.trim() : ''
      if (name === '') return null
      return sessionBoardOpsPayload(scope, workspace, region, [{ op: 'set', key: name, value: parseSessionBoardValue(text) }])
    }

    /**
     * `GET /astrbox/boards` 响应 → `{ok, board, error}`（K1）：过滤死条目/空区域，
     * 区域按更新时间倒序（最近写入的区在前）。
     * @param {unknown} wire 响应体
     * @returns {{ok: boolean, board: object|null, error: string}} 解析结果
     */
    function parseSessionBoardWire(wire) {
      if (wire === null || typeof wire !== 'object' || wire.ok !== true) {
        const detail = wire !== null && typeof wire === 'object' && wire.error !== null && typeof wire.error === 'object' ? wire.error.message : 'invalid response'
        return { ok: false, board: null, error: errorText(detail) }
      }
      const raw = wire.board
      if (raw === null || typeof raw !== 'object' || Array.isArray(raw)) return { ok: false, board: null, error: 'invalid board' }
      const regions = []
      for (const region of Array.isArray(raw.regions) ? raw.regions : []) {
        if (region === null || typeof region !== 'object') continue
        const id = typeof region.id === 'string' && region.id.trim() !== '' ? region.id.trim() : ''
        if (id === '') continue
        const entries = []
        for (const entry of Array.isArray(region.entries) ? region.entries : []) {
          const line = sessionBoardEntryLine(entry)
          if (line === '') continue
          entries.push({ key: entry.key, line, time: formatShortTime(entry.updated_at) })
        }
        regions.push({
          id,
          version: Number.isInteger(region.version) ? region.version : 0,
          updatedAt: typeof region.updated_at === 'string' ? region.updated_at : '',
          entries,
        })
      }
      regions.sort((left, right) => (Date.parse(right.updatedAt) || 0) - (Date.parse(left.updatedAt) || 0))
      return {
        ok: true,
        board: {
          scope: typeof raw.scope === 'string' ? raw.scope : '',
          version: Number.isInteger(raw.version) ? raw.version : 0,
          updatedAt: typeof raw.updated_at === 'string' ? raw.updated_at : '',
          regions,
        },
        error: '',
      }
    }

    /**
     * `GET /astrbox/boards/workspaces` 响应 → `{ok, items, error}`（K14）：只取工作区键
     * （`regions` 是区域数量 int；区域明细走板读取，不在此展开）。
     * @param {unknown} wire 响应体
     * @returns {{ok: boolean, items: string[], error: string}} 解析结果
     */
    function parseBoardWorkspacesWire(wire) {
      if (wire === null || typeof wire !== 'object' || wire.ok !== true) {
        const detail = wire !== null && typeof wire === 'object' && wire.error !== null && typeof wire.error === 'object' ? wire.error.message : 'invalid response'
        return { ok: false, items: [], error: errorText(detail) }
      }
      const items = []
      for (const item of Array.isArray(wire.workspaces) ? wire.workspaces : []) {
        const workspace = item !== null && typeof item === 'object' && typeof item.workspace === 'string' ? item.workspace.trim() : ''
        if (workspace !== '' && !items.includes(workspace)) items.push(workspace)
      }
      return { ok: true, items, error: '' }
    }

    /**
     * 工作区选择（K16）：存过且非空就用它，否则取列表第一个；空列表 = 未选择。
     * @param {unknown} stored 持久化的选择
     * @param {unknown} keys 工作区键列表
     * @returns {string} 生效工作区键
     */
    function pickBoardWorkspace(stored, keys) {
      const wanted = typeof stored === 'string' ? stored.trim() : ''
      const list = Array.isArray(keys) ? keys.filter((key) => typeof key === 'string' && key.trim() !== '') : []
      return wanted !== '' ? wanted : (list.length === 0 ? '' : list[0])
    }

    /** 读主会话提示词表（K28）；失败回成结果对象，不抛错。 */
    async function fetchMainPrompts() {
      if (typeof fetch !== 'function') return { ok: false, prompts: {}, maxChars: 0, error: 'fetch unavailable' }
      try {
        const response = await fetch(MAIN_PROMPT_PATH, { cache: 'no-store' })
        const body = await response.json().catch(() => ({}))
        if (response.status !== 200) return parseMainPromptsWire({ ok: false, error: { message: body?.error?.message ?? `HTTP ${response.status}` } })
        return parseMainPromptsWire(body)
      } catch (error) {
        return { ok: false, prompts: {}, maxChars: 0, error: errorText(error) }
      }
    }

    /** 保存一条主会话提示词（空文本 = 清除）；失败回成结果对象，不抛错（K28）。 */
    async function saveMainPrompt(entity, text) {
      const payload = mainPromptPayload(entity, text)
      if (payload === null) return { ok: false, prompts: {}, maxChars: 0, error: 'needFields' }
      try {
        const response = await fetch(MAIN_PROMPT_PATH, {
          method: 'POST',
          headers: { 'content-type': 'application/json' },
          body: JSON.stringify(payload),
        })
        const body = await response.json().catch(() => ({}))
        if (response.status < 200 || response.status >= 300) {
          return parseMainPromptsWire({ ok: false, error: { message: body?.error?.message ?? `HTTP ${response.status}` } })
        }
        return parseMainPromptsWire(body)
      } catch (error) {
        return { ok: false, prompts: {}, maxChars: 0, error: errorText(error) }
      }
    }

    /**
     * 同源 JSON 代理调用；返回 `{ok, body, error}`，不抛错（K15/K21）。
     * @param {string} base 代理前缀（如 `/astrbox/control`）
     * @param {string} path 路由路径
     * @param {object} [init] fetch 选项
     * @returns {Promise<{ok: boolean, body: unknown, error: string}>} 结果
     */
    async function requestJson(base, path, init) {
      if (typeof fetch !== 'function') return { ok: false, body: undefined, error: 'fetch unavailable' }
      try {
        const response = await fetch(`${base}${path}`, { cache: 'no-store', ...init })
        const body = await response.json().catch(() => ({}))
        if (response.status >= 200 && response.status < 300) return { ok: true, body, error: '' }
        return { ok: false, body, error: errorText(body?.error?.message ?? `HTTP ${response.status}`) }
      } catch (error) { return { ok: false, body: undefined, error: errorText(error) } }
    }

    /** 调一次控制代理；返回 `{ok, body, error}`，不抛错（K21/K22）。 */
    const controlCall = (path, init) => requestJson(CONTROL_BASE, path, init)

    /** 调一次会话黑板代理；返回 `{ok, body, error}`，不抛错（K15）。 */
    const boardCall = (path, init) => requestJson(BOARD_BASE, path, init)

    /** POST JSON 到控制代理。 */
    const postJson = (path, payload) => controlCall(path, { method: 'POST', headers: { 'content-type': 'application/json' }, body: JSON.stringify(payload) })

    /**
     * 读一块会话黑板（K15/K4）；失败回成结果对象，不抛错。
     * @param {'workspace'|'global'} scope 板 scope
     * @param {string} [workspace] 工作区键（workspace 板必填）
     * @returns {Promise<{ok: boolean, board: object|null, error: string}>} 解析结果
     */
    async function fetchSessionBoard(scope, workspace) {
      const params = new URLSearchParams({ scope })
      if (scope === 'workspace' && typeof workspace === 'string' && workspace !== '') params.set('workspace', workspace)
      const result = await boardCall(`?${params.toString()}`, { method: 'GET' })
      return result.ok ? parseSessionBoardWire(result.body) : { ok: false, board: null, error: result.error }
    }

    /**
     * 读工作区板摘要列表（K14）；失败回成结果对象，不抛错。
     * @returns {Promise<{ok: boolean, items: object[], error: string}>} 解析结果
     */
    async function fetchBoardWorkspaces() {
      const result = await boardCall('/workspaces', { method: 'GET' })
      return result.ok ? parseBoardWorkspacesWire(result.body) : { ok: false, items: [], error: result.error }
    }

    /**
     * 面板写会话黑板（K13）；字段不全或失败回成 `{ok:false, error}`。
     * @param {object|null} payload 请求体（null = 字段不全）
     * @returns {Promise<{ok: boolean, body: unknown, error: string}>} 结果
     */
    async function writeSessionBoard(payload) {
      if (payload === null) return { ok: false, body: undefined, error: 'incomplete' }
      return boardCall('/panel', { method: 'POST', headers: { 'content-type': 'application/json' }, body: JSON.stringify(payload) })
    }

    /**
     * 同源拉取全部实体黑板；任何失败都回成结果对象，不抛错。
     * @returns {Promise<{ok: boolean, boards: Array<object>, primary: string, paths: string[], error: string}>} 拉取结果。
     */
    async function fetchBoardsWire() {
      if (typeof fetch !== 'function') return { ok: false, boards: [], primary: '', paths: [], error: 'fetch unavailable' }
      try {
        const response = await fetch(BOARDS_PATH, { cache: 'no-store' })
        if (response === null || typeof response !== 'object') return { ok: false, boards: [], primary: '', paths: [], error: 'empty response' }
        const status = typeof response.status === 'number' ? response.status : 0
        if (status !== 200) return { ok: false, boards: [], primary: '', paths: [], error: `HTTP ${status}` }
        return parseBoardsWire(await response.json())
      } catch (error) {
        return { ok: false, boards: [], primary: '', paths: [], error: errorText(error) }
      }
    }

    /**
     * 读一个实体的若干路径值（`GET /astrbox/control/state`）；失败回成结果对象，不抛错。
     * @param {string} entity 实体 id。
     * @param {string[]} paths 相对路径。
     * @returns {Promise<{ok: boolean, values: Record<string, unknown>, error: string}>} 拉取结果。
     */
    async function fetchStateWire(entity, paths) {
      if (typeof entity !== 'string' || entity === '') return { ok: false, values: {}, error: 'empty entity' }
      const query = new URLSearchParams({ entity })
      for (const path of Array.isArray(paths) ? paths : []) query.append('path', path)
      const result = await controlCall(`/state?${query.toString()}`, { method: 'GET' })
      if (!result.ok) return { ok: false, values: {}, error: result.error }
      const parsed = parseStateWire(result.body)
      if (!parsed.ok) return { ok: false, values: {}, error: result.error !== '' ? result.error : 'invalid state response' }
      return parsed
    }

    /** 面板数据的初始态（首次渲染就显示骨架，不出现 undefined）。 */
    function initialPanelData() {
      return {
        boards: { status: 'loading', items: [], primary: '', entities: [], paths: [], error: '' },
        snapshot: { status: 'loading', entity: '', values: {}, error: '' },
        clock: { status: 'loading', time: '', phase: '', error: '' },
      }
    }

    /**
     * 面板数据轮询：一次拉「黑板 + 快照 + 世界时钟」，10s 一轮，页面隐藏时暂停。
     * 快照实体 = 用户选中的候选实体（持久化）否则桥接上报的生效实体。
     * 黑板不可用会把快照条一起置为不可用（同一数据源）；单实体读失败只影响快照条。
     * @param {string} preferredEntity 用户选中的实体 id（空串 = 跟随生效实体）。
     * @returns {{boards: object, snapshot: object, clock: object, refresh: Function}} 面板数据。
     */
    function usePanelData(preferredEntity) {
      const [state, setState] = useState(initialPanelData)
      const [reload, setReload] = useState(0)
      useEffect(() => {
        let alive = true
        let timer
        const load = async () => {
          const boardsWire = await fetchBoardsWire()
          if (!alive) return
          if (!boardsWire.ok) {
            setState({
              boards: { status: 'unavailable', items: [], primary: '', entities: [], paths: [], error: boardsWire.error },
              snapshot: { status: 'unavailable', entity: '', values: {}, error: boardsWire.error },
              clock: { status: 'unavailable', time: '', phase: '', error: boardsWire.error },
            })
            return
          }
          const candidates = entityCandidates(boardsWire.primary, boardsWire.boards)
          const wanted = typeof preferredEntity === 'string' ? preferredEntity.trim() : ''
          const entity = wanted !== '' && candidates.includes(wanted) ? wanted : boardsWire.primary
          const paths = boardsWire.paths
          const snapshotRequest = entity !== '' && paths.length > 0
            ? fetchStateWire(entity, paths)
            : Promise.resolve({ ok: false, values: {}, error: '' })
          const [snapshot, clock] = await Promise.all([snapshotRequest, fetchStateWire(WORLD_ENTITY, CLOCK_PATHS)])
          if (!alive) return
          setState({
            boards: { status: 'ready', items: boardsWire.boards, primary: boardsWire.primary, entities: candidates, paths, error: '' },
            snapshot: snapshot.ok
              ? { status: 'ready', entity, values: snapshot.values, error: '' }
              : { status: entity === '' ? 'empty' : 'unavailable', entity, values: {}, error: snapshot.error },
            clock: clock.ok
              ? { status: 'ready', time: String(clock.values['环境.当前时间'] ?? ''), phase: String(clock.values['状态.阶段'] ?? ''), error: '' }
              : { status: 'unavailable', time: '', phase: '', error: clock.error },
          })
        }
        const hidden = () => typeof document !== 'undefined' && document.hidden
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
      }, [reload, preferredEntity])
      const refresh = useCallback(() => setReload((value) => value + 1), [])
      return { ...state, refresh }
    }

    /** 会话黑板初始态（首次渲染就显示加载态）。 */
    function initialSessionBoardData() {
      const pending = { status: 'loading', board: null, items: [], error: '' }
      return { workspaces: pending, global: pending, workspace: pending }
    }

    /** 拉取结果 → 区块状态（loading 由初始态负责）。 */
    function boardSectionState(result) {
      return result.ok
        ? { status: 'ready', board: result.board ?? null, items: Array.isArray(result.items) ? result.items : [], error: '' }
        : { status: 'unavailable', board: null, items: [], error: result.error }
    }

    /**
     * 会话黑板数据轮询（K16）：工作区列表 + 跨工作区板 + 选中工作区板，10s 一轮，
     * 页面隐藏时暂停、恢复可见立刻补拉。
     * @param {string} workspaceKey 选中的工作区键（空串 = 未选择）
     * @returns {{workspaces: object, global: object, workspace: object, refresh: Function}} 数据与刷新
     */
    function useSessionBoardData(workspaceKey) {
      const [state, setState] = useState(initialSessionBoardData)
      const [reload, setReload] = useState(0)
      useEffect(() => {
        let alive = true
        let timer
        const load = async () => {
          const [workspaces, global, workspace] = await Promise.all([
            fetchBoardWorkspaces(),
            fetchSessionBoard('global'),
            workspaceKey === '' ? Promise.resolve({ ok: false, board: null, error: '' }) : fetchSessionBoard('workspace', workspaceKey),
          ])
          if (!alive) return
          setState({
            workspaces: boardSectionState(workspaces),
            global: boardSectionState(global),
            workspace: workspaceKey === '' ? { status: 'empty', board: null, items: [], error: '' } : boardSectionState(workspace),
          })
        }
        const hidden = () => typeof document !== 'undefined' && document.hidden
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
      }, [reload, workspaceKey])
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
     * 状态快照条：实体切换 + 白名单字段 chips + 世界时钟 + 面板动作（刷新 / 收起或全宽）。
     * @param {{data: object, layoutMode: string, onRefresh: Function, onToggleMonitor: Function, onToggleFull: Function, onSelectEntity: Function, t: Function}} props 组件份额
     * @returns {React.ReactElement} 状态条。
     */
    function StatusBar({ data, layoutMode, onRefresh, onToggleMonitor, onToggleFull, onSelectEntity, t }) {
      const { boards, snapshot, clock } = data
      const rows = useMemo(() => snapshotRows(boards.paths, snapshot.values), [boards.paths, snapshot.values])
      const entity = snapshot.entity !== '' ? snapshot.entity : boards.primary
      let hint = null
      if (boards.status === 'loading') hint = t('snapshot.loading')
      else if (boards.status === 'unavailable') hint = t('snapshot.unavailable')
      else if (entity === '') hint = t('snapshot.noPrimary')
      else if (snapshot.status === 'loading') hint = t('snapshot.loading')
      else if (snapshot.status === 'unavailable') hint = t('snapshot.unavailable')
      else if (rows.length === 0) hint = t('snapshot.empty')
      const clockText = clock.status === 'ready' && formatShortTime(clock.time) !== ''
        ? `${formatShortTime(clock.time)}${clock.phase === '' ? '' : ` · ${clock.phase}`}`
        : t('control.unknown')
      const monitorHidden = layoutMode === 'monitor-hidden'
      const monitorFull = layoutMode === 'monitor-full'
      const candidates = Array.isArray(boards.entities) ? boards.entities : []
      const canSwitch = candidates.length > 1 && boards.paths.length > 0
      let entityNode = null
      if (canSwitch && candidates.length <= 3) {
        entityNode = h('div', { className: 'dsh-neko-entitySwitch', role: 'group', 'aria-label': t('snapshot.switch') },
          candidates.map((name) => h('button', {
            key: name,
            type: 'button',
            className: 'dsh-neko-entity',
            'aria-pressed': name === entity,
            title: name === boards.primary ? `${name} · ${t('board.primary')}` : name,
            onClick: () => onSelectEntity(name),
          }, name)))
      } else if (canSwitch) {
        entityNode = h('select', {
          className: 'dsh-neko-select dsh-neko-entitySelect',
          value: entity,
          'aria-label': t('snapshot.switch'),
          title: t('snapshot.switch'),
          onChange: (event) => onSelectEntity(event.target.value),
        }, ...candidates.map((name) => h('option', { key: name, value: name }, name)))
      } else if (entity !== '') {
        entityNode = h('span', { className: 'dsh-neko-primary', title: entity }, entity)
      }
      return h('section', { className: 'dsh-neko-statusbar', 'data-dsh-neko-pane': 'snapshot' },
        entityNode,
        h('div', { className: 'dsh-neko-chips' },
          hint === null ? null : h('span', { className: 'dsh-neko-hint dsh-neko-hintInline' }, hint),
          ...rows.map((row) => h('span', {
            key: row.path,
            className: 'dsh-neko-chip',
            title: row.title,
            'data-missing': row.missing ? 'true' : 'false',
          },
            h('span', { className: 'dsh-neko-chipLabel' }, row.label),
            h('span', { className: 'dsh-neko-chipValue' }, row.text),
          )),
        ),
        h('span', { className: 'dsh-neko-clock', title: t('control.time') },
          h('span', { className: 'dsh-neko-clockLabel' }, t('snapshot.world')),
          h('span', { className: 'dsh-neko-clockValue' }, clockText),
        ),
        h('div', { className: 'dsh-neko-statusActions' },
          h('button', { type: 'button', className: 'dsh-neko-button', onClick: onRefresh }, t('status.refresh')),
          h('button', {
            type: 'button',
            className: 'dsh-neko-button',
            'aria-pressed': monitorHidden,
            onClick: onToggleMonitor,
          }, monitorHidden ? t('status.expand') : t('status.collapse')),
          h('button', {
            type: 'button',
            className: 'dsh-neko-button',
            'aria-pressed': monitorFull,
            onClick: onToggleFull,
          }, monitorFull ? t('status.unfold') : t('status.full')),
        ),
      )
    }

    /**
     * 监控区：iframe 内嵌 Neko Monitor 页面 + 地址编辑/刷新/外链。
     * @param {{t: Function}} props 组件份额。
     * @returns {React.ReactElement} 监控区。
     */
    function MonitorPane({ t }) {
      const [url, setUrl] = useState(() => readStored(STORAGE_MONITOR, DEFAULT_MONITOR_URL))
      const [attempt, setAttempt] = useState(0)
      const [loaded, setLoaded] = useState(false)
      const applyUrl = useCallback((next) => {
        const value = typeof next === 'string' && next.trim() !== '' ? next.trim() : DEFAULT_MONITOR_URL
        setUrl(value)
        writeStored(STORAGE_MONITOR, value)
        setLoaded(false)
        setAttempt((value) => value + 1)
      }, [])
      const reload = useCallback(() => {
        setLoaded(false)
        setAttempt(value => value + 1)
      }, [])
      const openExternal = useCallback(() => {
        if (typeof window !== 'undefined' && typeof window.open === 'function') window.open(url, '_blank', 'noopener,noreferrer')
      }, [url])
      return h('section', { className: 'dsh-neko-monitor', 'data-dsh-neko-pane': 'monitor' },
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
          h('button', { type: 'button', className: 'dsh-neko-button', onClick: reload }, t('status.reload')),
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
     * 一块有变化的实体黑板卡片：实体名、版本、更新时间、逐条变化（时间 + 规则数）。
     * 规则 id 列表只在 title 里给出（12 个 id 摊在行内是纯噪声）。
     * @param {{item: {entity: string, board: object}, t: Function}} props 组件份额。
     * @returns {React.ReactElement} 卡片。
     */
    function BoardCard({ item, t, primary }) {
      const { entity, board } = item
      return h('section', { className: 'dsh-neko-card', 'data-dsh-neko-board': entity, 'data-primary': primary ? 'true' : 'false' },
        h('header', { className: 'dsh-neko-cardHead' },
          h('strong', { className: 'dsh-neko-cardEntity' }, entity),
          primary ? h('span', { className: 'dsh-neko-badge dsh-neko-badgePrimary' }, t('board.primary')) : null,
          board === null ? null : h('span', { className: 'dsh-neko-badge' }, t('board.version', { version: String(board.version) })),
          board === null || board.updatedAt === '' ? null
            : h('span', { className: 'dsh-neko-cardTime' }, t('board.updated', { time: formatClockTime(board.updatedAt) || board.updatedAt })),
        ),
        board === null ? null : h('ul', { className: 'dsh-neko-boardList' }, board.entries.map((entry, index) => h('li', {
          key: `${entity}:${index}`,
          className: 'dsh-neko-boardRow',
        },
          h('span', { className: 'dsh-neko-boardChange', title: entry.line }, entry.line),
          h('span', { className: 'dsh-neko-boardMeta' },
            entry.time === '' ? null : h('span', { className: 'dsh-neko-boardTime' }, entry.time),
            entry.rules.length === 0 ? null
              : h('span', { className: 'dsh-neko-rule', title: entry.rules.join('、') }, t('board.rules', { count: String(entry.rules.length) })),
          ),
        ))),
      )
    }

    /**
     * 事件黑板区：标题/状态/计数/刷新 + 有变化的卡片 + 空黑板汇总（默认折叠）。
     * @param {{data: object, t: Function}} props 组件份额。
     * @returns {React.ReactElement} 黑板区。
     */
    function BoardPane({ data, t, embedded = false }) {
      const { boards } = data
      const [expanded, setExpanded] = useState(false)
      const { changed, empty } = useMemo(() => partitionBoards(boards.items, boards.primary), [boards.items, boards.primary])
      const statusText = boards.status === 'ready' ? t('board.ready') : boards.status === 'loading' ? t('board.loading') : t('board.unavailable')
      let body
      if (boards.status === 'loading') {
        body = h('p', { className: 'dsh-neko-hint' }, t('board.loading'))
      } else if (boards.status === 'unavailable') {
        body = h('div', { className: 'dsh-neko-boardBody' },
          h('p', { className: 'dsh-neko-hint', 'data-dsh-neko-boards': 'unavailable' }, t('board.unavailable')),
          boards.error === '' ? null : h('p', { className: 'dsh-neko-error' }, t('board.error', { error: boards.error })),
        )
      } else {
        const children = []
        if (changed.length === 0) children.push(h('p', { key: 'none', className: 'dsh-neko-hint' }, t('board.noChanges')))
        else children.push(...changed.map(item => h(BoardCard, { key: item.entity, item, t, primary: item.entity === boards.primary })))
        if (empty.length > 0) {
          children.push(h('button', {
            key: 'more',
            type: 'button',
            className: 'dsh-neko-more',
            'aria-expanded': expanded,
            onClick: () => setExpanded((value) => !value),
          },
            h('span', null, t('board.more', { count: String(empty.length) })),
            h('span', { className: 'dsh-neko-moreAction' }, expanded ? t('board.collapse') : t('board.expand')),
          ))
          if (expanded) {
            children.push(h('div', { key: 'empty', className: 'dsh-neko-emptyList' },
              empty.map(item => h('span', { key: item.entity, className: 'dsh-neko-emptyChip', title: item.entity }, item.entity))))
          }
        }
        body = h('div', { className: 'dsh-neko-boardBody' }, ...children)
      }
      if (embedded) {
        return h('div', { className: 'dsh-neko-tabBody', 'data-dsh-neko-pane': 'boards' }, body)
      }
      return h('section', { className: 'dsh-neko-board', 'data-dsh-neko-pane': 'boards' },
        h('header', { className: 'dsh-neko-paneHead' },
          h('strong', { className: 'dsh-neko-paneTitle' }, t('board.title')),
          h('span', { className: 'dsh-neko-badge', 'data-dsh-neko-boards': boards.status }, statusText),
          boards.status === 'ready' && changed.length > 0
            ? h('span', { className: 'dsh-neko-count' }, t('board.changed', { count: String(changed.length) }))
            : null,
          h('span', { className: 'dsh-neko-spacer' }),
          h('button', { type: 'button', className: 'dsh-neko-button', onClick: () => data.refresh() }, t('board.refresh')),
        ),
        body,
      )
    }

    /**
     * 控制区：世界时钟（来自面板数据）、手动 tick、时钟推进/设定、实体状态写入。
     * 失败可读、不崩；写成功后通知外层刷新面板数据。
     * @param {{t: Function, clock: object, onChanged: Function}} props 组件份额
     * @returns {React.ReactElement} 控制区。
     */
    function ControlPane({ t, clock, onChanged }) {
      const [entities, setEntities] = useState([])
      const [form, setForm] = useState({ entity: '', path: '', value: '', when: '' })
      const [ui, setUi] = useState({ busy: false, error: '', note: '' })
      const field = (name) => (event) => setForm((previous) => ({ ...previous, [name]: event.target.value }))
      const setUiField = (name, value) => setUi((previous) => ({ ...previous, [name]: value }))
      const loadEntities = useCallback(async () => {
        const result = await controlCall('/entities', { method: 'GET' })
        if (result.ok && Array.isArray(result.body?.entities)) setEntities(result.body.entities)
      }, [])
      useEffect(() => { void loadEntities() }, [loadEntities])
      const run = useCallback(async (work) => {
        setUi({ busy: true, error: '', note: '' })
        try {
          const result = await work()
          if (!result.ok) setUiField('error', result.error)
          else {
            setUi({ busy: true, error: '', note: t('control.done') })
            await loadEntities()
            if (typeof onChanged === 'function') onChanged()
          }
        } finally {
          setUi((previous) => ({ ...previous, busy: false }))
        }
      }, [loadEntities, onChanged, t])
      const act = (route, payload) => { if (!ui.busy) void run(() => postJson(route, payload)) }
      const submit = (kind) => {
        if (kind === 'time') {
          const time = toRfc3339(form.when)
          return time === '' ? setUiField('error', t('control.badTime')) : act('/clock', { time })
        }
        const parsed = parseJsonValue(form.value)
        if (!parsed.ok) return setUiField('error', t('control.badJson', { error: parsed.error }))
        const payload = statePayload(form.entity, form.path, parsed.value)
        return payload === null ? setUiField('error', t('control.needFields')) : act('/state', payload)
      }
      const row = (...children) => h('div', { className: 'dsh-neko-controlRow' }, ...children)
      const input = (props) => h('input', { className: 'dsh-neko-input', spellCheck: false, ...props })
      const button = (label, onClick) => h('button', { type: 'button', className: 'dsh-neko-button', disabled: ui.busy, onClick }, label)
      const options = [h('option', { key: '', value: '' }, t('control.pick'))]
        .concat(entities.map((item) => h('option', { key: String(item?.id ?? ''), value: String(item?.id ?? '') }, String(item?.id ?? ''))))
      const clockText = clock !== null && typeof clock === 'object' && clock.status === 'ready' && formatShortTime(clock.time) !== ''
        ? `${formatShortTime(clock.time)}${clock.phase === '' ? '' : ` · ${clock.phase}`}`
        : t('control.unknown')
      return h('section', { className: 'dsh-neko-control', 'data-dsh-neko-pane': 'control' },
        h('header', { className: 'dsh-neko-paneHead' },
          h('strong', { className: 'dsh-neko-paneTitle' }, t('control.title')),
          h('span', { className: 'dsh-neko-badge', title: t('control.time') }, clockText),
          h('span', { className: 'dsh-neko-spacer' }),
          button(t('control.tick'), () => act('/tick', {})),
        ),
        row(button(t('control.add15'), () => act('/clock', { minutes: 15 })), button(t('control.add60'), () => act('/clock', { minutes: 60 })), input({ type: 'datetime-local', value: form.when, 'aria-label': t('control.time'), onChange: field('when') }), button(t('control.setTime'), () => submit('time'))),
        row(h('select', { className: 'dsh-neko-select', value: form.entity, 'aria-label': t('control.entity'), onChange: field('entity') }, ...options),
          input({ value: form.path, placeholder: '状态.情绪', 'aria-label': t('control.path'), onChange: field('path') }), input({ value: form.value, placeholder: '"开心"', 'aria-label': t('control.value'), onChange: field('value') }), button(t('control.write'), () => submit('state'))),
        ui.note === '' ? null : h('p', { className: 'dsh-neko-hint' }, ui.note),
        ui.error === '' ? null : h('p', { className: 'dsh-neko-error' }, ui.error),
      )
    }


    /**
     * 主会话提示词区（K28）：按实体编辑机器人主会话（`主体`）的补充提示词。
     *
     * 数据面：`GET/POST /astrbox/main-session-prompts`（桥接注册，按实体存 ~/.dsh-astrbox）；
     * 保存后下一轮由 qqbot 主会话作为 system prompt 段注入；空文本 = 清除该实体的提示词。
     * 实体列表复用控制面的 `/astrbox/control/entities`；失败一律降级成可读提示，不崩溃。
     * @param {{t: Function}} props 组件份额
     * @returns {React.ReactElement} 主会话提示词区。
     */
    function MainPromptPane({ t }) {
      const [entities, setEntities] = useState([])
      const [entity, setEntity] = useState('')
      const [text, setText] = useState('')
      const [ui, setUi] = useState({ busy: false, status: 'loading', error: '', note: '' })
      const setUiField = (name, value) => setUi((previous) => ({ ...previous, [name]: value }))
      useEffect(() => {
        let stopped = false
        void (async () => {
          const [catalog, stored] = await Promise.all([controlCall('/entities', { method: 'GET' }), fetchMainPrompts()])
          if (stopped) return
          if (catalog.ok && Array.isArray(catalog.body?.entities)) setEntities(catalog.body.entities)
          setUi((previous) => ({
            ...previous,
            status: stored.ok ? 'ready' : 'unavailable',
            error: stored.ok ? '' : t('mainPrompt.unavailable'),
          }))
        })()
        return () => { stopped = true }
      }, [t])
      const selectEntity = useCallback(async (next) => {
        const value = typeof next === 'string' ? next.trim() : ''
        setEntity(value)
        setUiField('note', '')
        if (value === '') { setText(''); return }
        const stored = await fetchMainPrompts()
        if (!stored.ok) { setUiField('error', stored.error); setText(''); return }
        setUiField('error', '')
        setText(promptValueOf(stored.prompts, value))
      }, [])
      const save = useCallback(async () => {
        if (ui.busy) return
        setUi({ busy: true, status: ui.status, error: '', note: '' })
        const result = await saveMainPrompt(entity, text)
        if (!result.ok) { setUi((previous) => ({ ...previous, busy: false, error: result.error })); return }
        setText(promptValueOf(result.prompts, entity))
        setUi((previous) => ({ ...previous, busy: false, note: t('mainPrompt.saved') }))
      }, [entity, text, ui.busy, ui.status, t])
      const options = [h('option', { key: '', value: '' }, t('control.pick'))]
        .concat(entities.map((item) => h('option', { key: String(item?.id ?? ''), value: String(item?.id ?? '') }, String(item?.id ?? ''))))
      const selected = entity !== ''
      return h('section', { className: 'dsh-neko-control', 'data-dsh-neko-pane': 'main-prompt' },
        h('header', { className: 'dsh-neko-paneHead' },
          h('strong', { className: 'dsh-neko-paneTitle' }, t('mainPrompt.title')),
          h('span', { className: 'dsh-neko-badge', 'data-dsh-neko-main-prompt': ui.status }, ui.status === 'ready' ? t('mainPrompt.ready') : t('mainPrompt.unavailable')),
          h('span', { className: 'dsh-neko-spacer' }),
          h('button', {
            type: 'button',
            className: 'dsh-neko-button',
            disabled: ui.busy || !selected,
            onClick: () => { void save() },
          }, t('mainPrompt.save')),
        ),
        h('div', { className: 'dsh-neko-controlRow' },
          h('select', {
            className: 'dsh-neko-select',
            value: entity,
            'aria-label': t('control.entity'),
            onChange: (event) => { void selectEntity(event.target.value) },
          }, ...options),
        ),
        h('div', { className: 'dsh-neko-controlRow' },
          h('textarea', {
            className: 'dsh-neko-input dsh-neko-textarea',
            value: text,
            'aria-label': t('mainPrompt.title'),
            placeholder: t('mainPrompt.placeholder'),
            spellCheck: false,
            disabled: !selected || ui.busy,
            onChange: (event) => setText(event.target.value),
          }),
        ),
        h('p', { className: 'dsh-neko-hint' }, t('mainPrompt.hint')),
        ui.note === '' ? null : h('p', { className: 'dsh-neko-hint' }, ui.note),
        ui.error === '' ? null : h('p', { className: 'dsh-neko-error' }, ui.error),
      )
    }

    /**
     * 会话黑板区（K16）：跨工作区板 + 工作区板（区域卡片、条目删除、两步确认清空、写表单）。
     * 数据来自 K15 路由；写失败显示 error.message，失败不崩。
     * @param {{t: Function}} props 组件份额
     * @returns {React.ReactElement} 会话黑板区
     */
    function SessionBoardPane({ t, onRefreshReady }) {
      const [workspace, setWorkspace] = useState(() => readStored(STORAGE_BOARD_WORKSPACE, ''))
      const data = useSessionBoardData(workspace)
      const keys = data.workspaces.items
      const effective = pickBoardWorkspace(workspace, keys)
      useEffect(() => {
        if (effective === workspace) return
        setWorkspace(effective)
        writeStored(STORAGE_BOARD_WORKSPACE, effective)
      }, [effective, workspace])
      const selectWorkspace = useCallback((next) => {
        const value = typeof next === 'string' ? next.trim() : ''
        setWorkspace(value)
        writeStored(STORAGE_BOARD_WORKSPACE, value)
      }, [])
      const panel = useCallback((payload) => writeSessionBoard(payload), [])
      useEffect(() => {
        if (typeof onRefreshReady === 'function') onRefreshReady(data.refresh)
      }, [data.refresh, onRefreshReady])
      return h('div', { className: 'dsh-neko-tabBody', 'data-dsh-neko-pane': 'session-board' },
        h('div', { className: 'dsh-neko-sessionBoardBody' },
          h(SessionBoardScope, { scope: 'global', title: t('sessionBoard.global'), data: data.global, workspaces: keys, workspace: '', onPanel: panel, onChanged: data.refresh, t }),
          h(SessionBoardScope, { scope: 'workspace', title: t('sessionBoard.workspace'), data: data.workspace, workspaces: keys, workspace: effective, onSelectWorkspace: selectWorkspace, onPanel: panel, onChanged: data.refresh, t }),
        ),
      )
    }

    /**
     * 一块会话黑板的区域列表 + 写表单（每个 scope 一张卡片）。
     * @param {{scope: string, title: string, data: object, workspaces: string[], workspace: string, onSelectWorkspace?: Function, onPanel: Function, onChanged: Function, t: Function}} props 组件份额
     * @returns {React.ReactElement} 一块板
     */
    function SessionBoardScope({ scope, title, data, workspaces, workspace, onSelectWorkspace, onPanel, onChanged, t }) {
      const board = data.board
      const regions = board === null ? [] : board.regions
      let body
      if (data.status === 'loading') body = h('p', { className: 'dsh-neko-hint' }, t('board.loading'))
      else if (data.status === 'unavailable') body = h('div', null,
        h('p', { className: 'dsh-neko-hint' }, t('board.unavailable')),
        data.error === '' ? null : h('p', { className: 'dsh-neko-error' }, t('board.error', { error: data.error })))
      else if (scope === 'workspace' && workspace === '') body = h('p', { className: 'dsh-neko-hint' }, t('sessionBoard.pickWorkspace'))
      else body = h('div', { className: 'dsh-neko-boardBody' },
        regions.length === 0 ? h('p', { className: 'dsh-neko-hint' }, t('sessionBoard.empty')) : null,
        regions.map((region) => h(SessionBoardRegionCard, { key: region.id, region, scope, workspace: scope === 'global' ? region.id : workspace, onPanel, onChanged, t })),
        h(SessionBoardWriteForm, { scope, workspace, regions: regions.map((region) => region.id), workspaces, onPanel, onChanged, t }),
      )
      return h('div', { 'data-dsh-neko-scope': scope },
        h('div', { className: 'dsh-neko-controlRow' },
          h('strong', { className: 'dsh-neko-paneTitle' }, title),
          scope === 'workspace'
            ? h('select', { className: 'dsh-neko-select', value: workspace, 'aria-label': t('sessionBoard.workspacePick'), onChange: (event) => onSelectWorkspace(event.target.value) },
                h('option', { value: '' }, t('sessionBoard.noWorkspace')),
                ...workspaces.map((key) => h('option', { key, value: key }, key)))
            : null,
          board === null ? null : h('span', { className: 'dsh-neko-badge' }, t('board.version', { version: String(board.version) })),
          board === null || board.updatedAt === '' ? null : h('span', { className: 'dsh-neko-cardTime' }, formatShortTime(board.updatedAt)),
        ),
        body,
      )
    }

    /**
     * 一个区域卡片：版本/更新时间、条目（可删）与两步确认清空（K16）。
     * @param {{region: object, scope: string, workspace: string, onPanel: Function, onChanged: Function, t: Function}} props 组件份额
     * @returns {React.ReactElement} 区域卡片
     */
    function SessionBoardRegionCard({ region, scope, workspace, onPanel, onChanged, t }) {
      const [confirming, setConfirming] = useState(false)
      const [ui, setUi] = useState({ busy: false, error: '' })
      const send = useCallback(async (payload) => {
        if (ui.busy) return
        setUi({ busy: true, error: '' })
        const result = await onPanel(payload)
        setUi({ busy: false, error: result.ok ? '' : result.error })
        if (result.ok) onChanged()
      }, [onChanged, onPanel, ui.busy])
      const clear = () => { setConfirming(false); void send(sessionBoardOpsPayload(scope, workspace, region.id, [{ op: 'clear' }])) }
      return h('section', { className: 'dsh-neko-card', 'data-dsh-neko-region': region.id },
        h('header', { className: 'dsh-neko-cardHead' },
          h('strong', { className: 'dsh-neko-cardEntity', title: region.id }, region.id),
          h('span', { className: 'dsh-neko-badge' }, t('board.version', { version: String(region.version) })),
          h('span', { className: 'dsh-neko-cardTime' }, formatShortTime(region.updatedAt)),
          h('button', { type: 'button', className: 'dsh-neko-button', disabled: ui.busy, onClick: () => (confirming ? clear() : setConfirming(true)) }, confirming ? t('sessionBoard.clearConfirm') : t('sessionBoard.clear')),
          confirming ? h('button', { type: 'button', className: 'dsh-neko-button', onClick: () => setConfirming(false) }, t('sessionBoard.cancel')) : null,
        ),
        region.entries.length === 0
          ? h('p', { className: 'dsh-neko-hint' }, t('sessionBoard.emptyRegion'))
          : h('ul', { className: 'dsh-neko-boardList' }, region.entries.map((entry) => h('li', { key: entry.key, className: 'dsh-neko-boardRow' },
            h('span', { className: 'dsh-neko-boardChange', title: entry.line }, entry.line),
            h('span', { className: 'dsh-neko-boardMeta' },
              entry.time === '' ? null : h('span', { className: 'dsh-neko-boardTime' }, entry.time),
              h('span', { className: 'dsh-neko-spacer' }),
              h('button', { type: 'button', className: 'dsh-neko-button', disabled: ui.busy, onClick: () => { void send(sessionBoardOpsPayload(scope, workspace, region.id, [{ op: 'delete', key: entry.key }])) } }, t('sessionBoard.delete')),
            )))),
        ui.error === '' ? null : h('p', { className: 'dsh-neko-error' }, ui.error),
      )
    }

    /**
     * 一个写表单（K16）：区域（global=工作区键；workspace=已有区域或新区域 id）+ key + value。
     * value 文本先按 JSON 解析，失败按字符串写入；写失败显示 error.message。
     * @param {{scope: string, workspace: string, regions: string[], workspaces: string[], onPanel: Function, onChanged: Function, t: Function}} props 组件份额
     * @returns {React.ReactElement} 写表单
     */
    function SessionBoardWriteForm({ scope, workspace, regions, workspaces, onPanel, onChanged, t }) {
      const [form, setForm] = useState({ region: '', key: '', value: '' })
      const [ui, setUi] = useState({ busy: false, error: '', note: '' })
      const field = (name) => (event) => setForm((previous) => ({ ...previous, [name]: event.target.value }))
      const submit = useCallback(async () => {
        if (ui.busy) return
        const payload = sessionBoardSetPayload(scope, scope === 'global' ? '' : workspace, form.region, form.key, form.value)
        if (payload === null) {
          setUi({ busy: false, error: t('sessionBoard.needFields'), note: '' })
          return
        }
        setUi({ busy: true, error: '', note: '' })
        const result = await onPanel(payload)
        setUi({ busy: false, error: result.ok ? '' : result.error, note: result.ok ? t('control.done') : '' })
        if (result.ok) onChanged()
      }, [form, onChanged, onPanel, scope, t, ui.busy, workspace])
      const suggestions = scope === 'global' ? workspaces : regions
      return h('div', { className: 'dsh-neko-controlRow', 'data-dsh-neko-write': scope },
        h('input', { className: 'dsh-neko-input', value: form.region, list: `dsh-neko-regions-${scope}`, placeholder: t('sessionBoard.region'), 'aria-label': t('sessionBoard.region'), onChange: field('region') }),
        h('datalist', { id: `dsh-neko-regions-${scope}` }, suggestions.map((value) => h('option', { key: value, value }))),
        h('input', { className: 'dsh-neko-input', value: form.key, placeholder: '任务.当前', 'aria-label': t('sessionBoard.key'), onChange: field('key') }),
        h('input', { className: 'dsh-neko-input', value: form.value, placeholder: 'JSON / 文本', 'aria-label': t('sessionBoard.value'), onChange: field('value') }),
        h('button', { type: 'button', className: 'dsh-neko-button', disabled: ui.busy, onClick: () => { void submit() } }, t('sessionBoard.write')),
        ui.note === '' ? null : h('span', { className: 'dsh-neko-hint dsh-neko-hintInline' }, ui.note),
        ui.error === '' ? null : h('span', { className: 'dsh-neko-error' }, ui.error),
      )
    }

    /**
     * 黑板 Tab 面板（K31b）：事件黑板 / 会话黑板共用一个 pane，Tab 头含刷新；
     * 激活 Tab 持久化到 `dsh-neko-panel.boardTab`。刷新同时刷新两个数据源。
     * @param {{data: object, t: Function}} props 组件份额
     * @returns {React.ReactElement} Tab 面板
     */
    function BoardTabs({ data, t }) {
      const [tab, setTab] = useState(() => sanitizeBoardTab(readStored(STORAGE_BOARD_TAB, 'event')))
      const sessionRefresh = useRef(null)
      const selectTab = useCallback((next) => {
        const value = sanitizeBoardTab(next)
        setTab(value)
        writeStored(STORAGE_BOARD_TAB, value)
      }, [])
      const registerSessionRefresh = useCallback((fn) => {
        sessionRefresh.current = typeof fn === 'function' ? fn : null
      }, [])
      const refresh = useCallback(() => {
        data.refresh()
        if (sessionRefresh.current !== null) sessionRefresh.current()
      }, [data])
      const items = [['event', t('board.title')], ['session', t('sessionBoard.title')]]
      return h('section', { className: 'dsh-neko-board dsh-neko-boardTabs', 'data-dsh-neko-pane': 'blackboards' },
        h('header', { className: 'dsh-neko-paneHead' },
          h('div', { className: 'dsh-neko-tabs', role: 'tablist' },
            items.map(([id, label]) => h('button', {
              key: id,
              type: 'button',
              role: 'tab',
              className: 'dsh-neko-tab',
              'aria-selected': tab === id,
              onClick: () => selectTab(id),
            }, label))),
          h('span', { className: 'dsh-neko-spacer' }),
          h('button', { type: 'button', className: 'dsh-neko-button', onClick: refresh }, t('board.refresh')),
        ),
        tab === 'event'
          ? h(BoardPane, { data, t, embedded: true })
          : h(SessionBoardPane, { t, onRefreshReady: registerSessionRefresh }),
      )
    }

    /**
     * 主列页面：快照条 + （监控区 / 黑板区 + 控制区）按布局模式装配。
     * @param {{t: Function}} props 注册时注入的份额。
     * @returns {React.ReactElement} 面板。
     */
    function NekoPanel({ t }) {
      const [layout, setLayout] = useState(() => sanitizeLayoutMode(readStored(STORAGE_LAYOUT, 'split')))
      const [selectedEntity, setSelectedEntity] = useState(() => readStored(STORAGE_ENTITY, ''))
      const data = usePanelData(selectedEntity)
      const selectEntity = useCallback((next) => {
        const value = typeof next === 'string' ? next.trim() : ''
        setSelectedEntity(value)
        writeStored(STORAGE_ENTITY, value)
      }, [])
      const applyLayout = useCallback((next) => {
        const value = sanitizeLayoutMode(next)
        setLayout(value)
        writeStored(STORAGE_LAYOUT, value)
      }, [])
      const toggleMonitor = useCallback(() => applyLayout(nextLayoutMode(layout, 'toggleMonitor')), [applyLayout, layout])
      const toggleFull = useCallback(() => applyLayout(nextLayoutMode(layout, 'toggleFull')), [applyLayout, layout])
      const side = h('div', { className: 'dsh-neko-side', key: 'side' },
        h(BoardTabs, { data, t }),
        h(ControlPane, { t, clock: data.clock, onChanged: data.refresh }),
        h(MainPromptPane, { key: 'main-prompt', t }),
      )
      const monitor = h(MonitorPane, { key: 'monitor', t })
      let body
      if (layout === 'monitor-hidden') body = h('div', { className: 'dsh-neko-body' }, side)
      else if (layout === 'monitor-full') body = h('div', { className: 'dsh-neko-body' }, monitor)
      else body = h('div', { className: 'dsh-neko-body' }, monitor, side)
      return h('div', { className: 'dsh-neko-panel', 'data-dsh-plugin': 'astrbox', 'data-layout': layout },
        h(StatusBar, {
          data,
          layoutMode: layout,
          onRefresh: data.refresh,
          onToggleMonitor: toggleMonitor,
          onToggleFull: toggleFull,
          onSelectEntity: selectEntity,
          t,
        }),
        body,
      )
    }

    /**
     * 面板样式。
     *
     * 令牌块（`--neko-*`）是本文件里**唯一**出现字面量视觉值的位置，下面所有规则只引用它；
     * 颜色一律引用宿主语义令牌 `--dsw-alias-*`（深色/浅色由宿主主题决定），不放原始色值。
     * 例外：`@media` 断点必须写字面量（CSS 不支持媒体查询引用自定义属性）。
     */
    const PANEL_CSS = `
.dsh-neko-panel {
  --neko-space-1: 2px;
  --neko-space-2: 4px;
  --neko-space-3: 6px;
  --neko-space-4: 8px;
  --neko-space-5: 12px;
  --neko-radius-sm: 6px;
  --neko-radius-md: 8px;
  --neko-radius-pill: 999px;
  --neko-font-xs: 11px;
  --neko-font-sm: 12px;
  --neko-font-md: 13px;
  --neko-line-height: 1.5;
  --neko-border-width: 1px;
  --neko-side-min: 340px;
  --neko-control-max: 42%;
  --neko-chip-max: 220px;
  --neko-duration: 120ms;
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  background: var(--dsw-alias-bg-base);
  color: var(--dsw-alias-label-primary);
  font-size: var(--neko-font-md);
  line-height: var(--neko-line-height);
}
.dsh-neko-muted { color: var(--dsw-alias-label-secondary); }
.dsh-neko-spacer { flex: 1 1 auto; }

/* 状态快照条 */
.dsh-neko-statusbar {
  flex: 0 0 auto;
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--neko-space-2) var(--neko-space-3);
  padding: var(--neko-space-3) var(--neko-space-5);
  border-bottom: var(--neko-border-width) solid var(--dsw-alias-border-l1);
  background: var(--dsw-alias-bg-layer-1);
}
.dsh-neko-primary { flex: 0 0 auto; font-size: var(--neko-font-md); font-weight: 600; white-space: nowrap; }
.dsh-neko-entitySwitch {
  flex: 0 0 auto; display: inline-flex; align-items: center; gap: var(--neko-space-1);
  padding: var(--neko-space-1);
  border: var(--neko-border-width) solid var(--dsw-alias-border-l1);
  border-radius: var(--neko-radius-pill);
  background: var(--dsw-alias-bg-layer-1);
}
.dsh-neko-entity {
  border: 0; border-radius: var(--neko-radius-pill); background: transparent;
  color: var(--dsw-alias-label-secondary);
  font: inherit; font-size: var(--neko-font-xs);
  padding: var(--neko-space-1) var(--neko-space-3);
  cursor: pointer; white-space: nowrap;
  transition: background-color var(--neko-duration) ease, color var(--neko-duration) ease;
}
.dsh-neko-entity:hover { color: var(--dsw-alias-label-primary); }
.dsh-neko-entity[aria-pressed="true"] { background: var(--dsw-alias-bg-overlay); color: var(--dsw-alias-label-primary); font-weight: 600; }
.dsh-neko-entitySelect { flex: 0 0 auto; max-width: 140px; }
.dsh-neko-chips { flex: 1 1 auto; display: flex; align-items: center; flex-wrap: wrap; gap: var(--neko-space-2); min-width: 0; }
.dsh-neko-chip {
  display: inline-flex; align-items: baseline; gap: var(--neko-space-1);
  max-width: var(--neko-chip-max);
  padding: var(--neko-space-1) var(--neko-space-3);
  border: var(--neko-border-width) solid var(--dsw-alias-border-l1);
  border-radius: var(--neko-radius-pill);
  background: var(--dsw-alias-bg-base);
  font-size: var(--neko-font-xs);
}
.dsh-neko-chip[data-missing="true"] { border-style: dashed; }
.dsh-neko-chipLabel { color: var(--dsw-alias-label-secondary); white-space: nowrap; }
.dsh-neko-chipValue { color: var(--dsw-alias-label-primary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.dsh-neko-clock {
  flex: 0 0 auto; display: inline-flex; align-items: baseline; gap: var(--neko-space-2);
  padding: var(--neko-space-1) var(--neko-space-3);
  border: var(--neko-border-width) solid var(--dsw-alias-border-l1);
  border-radius: var(--neko-radius-pill);
  background: var(--dsw-alias-bg-layer-2);
  font-size: var(--neko-font-xs);
  white-space: nowrap;
}
.dsh-neko-clockLabel { color: var(--dsw-alias-label-secondary); }
.dsh-neko-clockValue { color: var(--dsw-alias-label-primary); font-variant-numeric: tabular-nums; }
.dsh-neko-statusActions { flex: 0 0 auto; display: flex; align-items: center; gap: var(--neko-space-2); margin-left: auto; }

/* 主体：监控区 + 侧栏 */
.dsh-neko-body { flex: 1 1 auto; display: flex; min-height: 0; }
.dsh-neko-monitor { flex: 1 1 55%; display: flex; flex-direction: column; min-width: 0; min-height: 0; }
.dsh-neko-side {
  flex: 1 1 45%; display: flex; flex-direction: column;
  min-width: var(--neko-side-min); min-height: 0; overflow: hidden;
  border-left: var(--neko-border-width) solid var(--dsw-alias-border-l1);
}

/* 分区头（监控 / 黑板 / 控制共用） */
.dsh-neko-paneHead {
  flex: 0 0 auto;
  display: flex; align-items: center; gap: var(--neko-space-3);
  padding: var(--neko-space-3) var(--neko-space-4);
  border-bottom: var(--neko-border-width) solid var(--dsw-alias-border-l1);
  background: var(--dsw-alias-bg-layer-1);
}
.dsh-neko-paneTitle { font-size: var(--neko-font-md); font-weight: 600; white-space: nowrap; }
.dsh-neko-input, .dsh-neko-select {
  flex: 1 1 auto; min-width: 0;
  border: var(--neko-border-width) solid var(--dsw-alias-border-l1);
  border-radius: var(--neko-radius-sm);
  background: var(--dsw-alias-bg-base);
  color: var(--dsw-alias-label-primary);
  font: inherit; font-size: var(--neko-font-sm);
  padding: var(--neko-space-1) var(--neko-space-3);
  transition: border-color var(--neko-duration) ease;
}
.dsh-neko-input:focus-visible, .dsh-neko-select:focus-visible { outline: none; border-color: var(--dsw-alias-brand-primary); }
.dsh-neko-select { flex: 0 0 auto; max-width: 160px; }
.dsh-neko-button {
  flex: 0 0 auto;
  border: var(--neko-border-width) solid var(--dsw-alias-border-l1);
  border-radius: var(--neko-radius-sm);
  background: transparent;
  color: var(--dsw-alias-label-primary);
  font: inherit; font-size: var(--neko-font-sm);
  padding: var(--neko-space-1) var(--neko-space-4);
  cursor: pointer; white-space: nowrap;
  transition: background-color var(--neko-duration) ease, border-color var(--neko-duration) ease;
}
.dsh-neko-button:hover:not(:disabled) { background: var(--dsw-alias-bg-layer-2); }
.dsh-neko-button[aria-pressed="true"] { border-color: var(--dsw-alias-brand-primary); color: var(--dsw-alias-brand-primary); }
.dsh-neko-button:disabled { opacity: 0.55; cursor: default; }
.dsh-neko-frame { flex: 1 1 auto; width: 100%; min-height: 0; border: 0; background: var(--dsw-alias-bg-base); }
.dsh-neko-hint { margin: 0; padding: var(--neko-space-3) var(--neko-space-5); font-size: var(--neko-font-sm); color: var(--dsw-alias-label-secondary); }
.dsh-neko-hintInline { padding: 0; font-size: var(--neko-font-xs); }
.dsh-neko-error { margin: 0; padding: 0 var(--neko-space-5) var(--neko-space-3); font-size: var(--neko-font-sm); color: var(--dsw-alias-state-error-primary); }
.dsh-neko-badge {
  flex: 0 0 auto;
  font-size: var(--neko-font-xs);
  padding: 0 var(--neko-space-3);
  border-radius: var(--neko-radius-pill);
  border: var(--neko-border-width) solid var(--dsw-alias-border-l1);
  color: var(--dsw-alias-label-secondary);
  font-variant-numeric: tabular-nums;
}
.dsh-neko-badge[data-dsh-neko-boards="ready"] { color: var(--dsw-alias-state-success-primary); border-color: var(--dsw-alias-state-success-primary); }
.dsh-neko-badge[data-dsh-neko-boards="unavailable"] { color: var(--dsw-alias-state-error-primary); border-color: var(--dsw-alias-state-error-primary); }
.dsh-neko-count { font-size: var(--neko-font-xs); color: var(--dsw-alias-label-secondary); white-space: nowrap; }

/* 事件黑板 */
.dsh-neko-board { flex: 1 1 auto; display: flex; flex-direction: column; min-height: 0; background: var(--dsw-alias-bg-layer-1); }
.dsh-neko-boardBody {
  flex: 1 1 auto; min-height: 0; overflow-y: auto;
  padding: var(--neko-space-4) var(--neko-space-4) var(--neko-space-5);
  display: flex; flex-direction: column; gap: var(--neko-space-3);
}
/* 滚动条目的子项不参与收缩：默认 flex-shrink:1 会把卡片/汇总按钮压扁到可视高度，
   卡片自己的 overflow:hidden 再把内容截断——于是 overflow 永远不触发、黑板无法滚动。 */
.dsh-neko-boardBody > * { flex: 0 0 auto; }
.dsh-neko-card {
  border: var(--neko-border-width) solid var(--dsw-alias-border-l1);
  border-radius: var(--neko-radius-md);
  background: var(--dsw-alias-bg-base);
  overflow: hidden;
}
.dsh-neko-cardHead {
  display: flex; align-items: center; gap: var(--neko-space-2);
  padding: var(--neko-space-2) var(--neko-space-4);
  border-bottom: var(--neko-border-width) solid var(--dsw-alias-border-l1);
  background: var(--dsw-alias-bg-layer-2);
}
.dsh-neko-cardEntity { font-size: var(--neko-font-md); font-weight: 600; }
.dsh-neko-card[data-primary="true"] { border-color: var(--dsw-alias-border-l2); }
.dsh-neko-badgePrimary { color: var(--dsw-alias-brand-primary); border-color: var(--dsw-alias-brand-primary); }
.dsh-neko-cardTime { margin-left: auto; font-size: var(--neko-font-xs); color: var(--dsw-alias-label-secondary); white-space: nowrap; }
.dsh-neko-boardList { margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; }
.dsh-neko-boardRow {
  display: flex; flex-direction: column; gap: var(--neko-space-1);
  padding: var(--neko-space-2) var(--neko-space-4);
  border-top: var(--neko-border-width) solid var(--dsw-alias-border-l1);
}
.dsh-neko-boardRow:first-child { border-top: 0; }
.dsh-neko-boardChange {
  font-size: var(--neko-font-sm);
  color: var(--dsw-alias-label-primary);
  overflow-wrap: anywhere;
  display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden;
}
.dsh-neko-boardMeta { display: flex; align-items: center; gap: var(--neko-space-3); font-size: var(--neko-font-xs); color: var(--dsw-alias-label-secondary); }
.dsh-neko-boardTime { font-variant-numeric: tabular-nums; }
.dsh-neko-rule {
  padding: 0 var(--neko-space-3);
  border-radius: var(--neko-radius-pill);
  border: var(--neko-border-width) solid var(--dsw-alias-border-l1);
  color: var(--dsw-alias-label-secondary);
  font-variant-numeric: tabular-nums;
  cursor: help;
}
.dsh-neko-more {
  display: flex; align-items: center; justify-content: space-between; gap: var(--neko-space-3);
  width: 100%;
  padding: var(--neko-space-2) var(--neko-space-4);
  border: var(--neko-border-width) dashed var(--dsw-alias-border-l1);
  border-radius: var(--neko-radius-md);
  background: transparent;
  color: var(--dsw-alias-label-secondary);
  font: inherit; font-size: var(--neko-font-xs);
  text-align: left; cursor: pointer;
}
.dsh-neko-more:hover { background: var(--dsw-alias-bg-layer-2); }
.dsh-neko-moreAction { color: var(--dsw-alias-brand-primary); white-space: nowrap; }
.dsh-neko-emptyList { display: flex; flex-wrap: wrap; gap: var(--neko-space-2); padding: 0 var(--neko-space-1); }
.dsh-neko-emptyChip {
  padding: 0 var(--neko-space-3);
  border: var(--neko-border-width) solid var(--dsw-alias-border-l1);
  border-radius: var(--neko-radius-pill);
  font-size: var(--neko-font-xs);
  color: var(--dsw-alias-label-secondary);
  background: var(--dsw-alias-bg-base);
}

/* 控制区 */
.dsh-neko-control {
  flex: 0 0 auto; max-height: var(--neko-control-max); overflow-y: auto;
  border-top: var(--neko-border-width) solid var(--dsw-alias-border-l1);
  background: var(--dsw-alias-bg-base);
}
.dsh-neko-tabs { display: flex; align-items: center; gap: var(--neko-space-2); }
.dsh-neko-tab {
  border: var(--neko-border-width) solid var(--dsw-alias-border-l1);
  border-radius: var(--neko-radius-pill);
  background: transparent;
  color: var(--dsw-alias-label-secondary);
  font: inherit; font-size: var(--neko-font-sm);
  padding: 0 var(--neko-space-4);
  cursor: pointer; white-space: nowrap;
  transition: background-color var(--neko-duration) ease, border-color var(--neko-duration) ease;
}
.dsh-neko-tab:hover { background: var(--dsw-alias-bg-layer-2); }
.dsh-neko-tab[aria-selected="true"] {
  color: var(--dsw-alias-label-primary);
  border-color: var(--dsw-alias-brand-primary);
  background: var(--dsw-alias-bg-layer-2);
}
.dsh-neko-tabBody { flex: 1 1 auto; min-height: 0; display: flex; flex-direction: column; }
.dsh-neko-sessionBoardBody { flex: 1 1 auto; min-height: 0; overflow-y: auto; }
.dsh-neko-controlRow { display: flex; align-items: center; flex-wrap: wrap; gap: var(--neko-space-2); padding: var(--neko-space-2) var(--neko-space-4); }
.dsh-neko-textarea {
  flex: 1 1 100%; min-height: 72px; resize: vertical;
  font-family: inherit; line-height: 1.5;
  padding: var(--neko-space-2) var(--neko-space-3);
}

/* 窄屏：上下堆叠，监控区给一个可用高度 */
@media (max-width: 920px) {
  .dsh-neko-body { flex-direction: column; overflow-y: auto; }
  .dsh-neko-monitor { flex: 0 0 auto; height: 42vh; }
  .dsh-neko-side { flex: 0 0 auto; min-width: 0; overflow: visible; border-left: 0; border-top: var(--neko-border-width) solid var(--dsw-alias-border-l1); }
  .dsh-neko-board { flex: 0 0 auto; }
  .dsh-neko-boardBody { overflow: visible; }
  .dsh-neko-control { max-height: none; }
  .dsh-neko-statusActions { margin-left: 0; }
}
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
        formatShortTime,
        parseStateWire,
        entityCandidates,
        snapshotRows,
        partitionBoards,
        sanitizeLayoutMode,
        sanitizeBoardTab,
        nextLayoutMode,
        panelCss: PANEL_CSS,
        lastPathSegment,
        truncateText,
        toRfc3339,
        parseJsonValue,
        statePayload,
        parseMainPromptsWire,
        mainPromptPayload,
        promptValueOf,
        parseSessionBoardWire,
        parseBoardWorkspacesWire,
        sessionBoardEntryLine,
        parseSessionBoardValue,
        sessionBoardTarget,
        sessionBoardOpsPayload,
        sessionBoardSetPayload,
        pickBoardWorkspace,
        dictionaries: { zh: ZH, en: EN },
      },
    }
  },
})
