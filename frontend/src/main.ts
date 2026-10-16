import './style.css'
import * as Go from '../wailsjs/go/main/App'
import { EventsOn } from '../wailsjs/runtime/runtime'
import { baseName, collator, formatCount, kindOf, plural, seconds } from './format'
import { Inspector, type PreviewResult } from './inspector'
import { VirtualList, type FileItem } from './list'

// --- События из Go ------------------------------------------------------

interface SearchBatch { id: number; items: FileItem[] }
interface SearchDone {
  id: number
  stats: { found: number; dirs: number; skipped: number; limited: boolean }
  ms: number
  canceled: boolean
  error?: string
}
interface UpdateInfo { available: boolean; current: string; version?: string; notes?: string }

// --- Элементы ----------------------------------------------------------

const byId = <T extends HTMLElement>(id: string) => document.getElementById(id) as T

const app = byId<HTMLDivElement>('app')
const queryInput = byId<HTMLInputElement>('query')
const scopeButton = byId<HTMLButtonElement>('scope')
const scopeName = byId<HTMLSpanElement>('scopeName')
const listEl = byId<HTMLDivElement>('list')
const listHead = byId<HTMLDivElement>('listHead')
const emptyEl = byId<HTMLDivElement>('empty')
const statusEl = byId<HTMLSpanElement>('status')
const statusWarn = byId<HTMLSpanElement>('statusWarn')
const spinner = byId<HTMLSpanElement>('spinner')
const stopButton = byId<HTMLButtonElement>('stop')
const recentList = byId<HTMLUListElement>('recent')
const inspectorEl = byId<HTMLElement>('inspector')
const inspectorToggle = byId<HTMLButtonElement>('toggleInspector')
const toastEl = byId<HTMLDivElement>('toast')

// --- Мелочи ------------------------------------------------------------

const store = {
  get(key: string): string | null {
    try { return localStorage.getItem(key) } catch { return null }
  },
  set(key: string, value: string): void {
    try { localStorage.setItem(key, value) } catch { /* хранилище недоступно — не страшно */ }
  },
}

let toastTimer = 0
function toast(message: string): void {
  toastEl.textContent = message
  toastEl.hidden = false
  clearTimeout(toastTimer)
  toastTimer = window.setTimeout(() => { toastEl.hidden = true }, 3500)
}

const errorText = (err: unknown) => (err instanceof Error ? err.message : String(err))

// --- Состояние поиска --------------------------------------------------

let root = ''
let searchId = 0
let running = false
let lastDone: SearchDone | null = null
let items: FileItem[] = []

type SortKey = 'none' | 'name' | 'mod' | 'size' | 'kind' | 'dir'
let sortKey: SortKey = 'none'
let sortDir = 1

const list = new VirtualList(listEl, byId('spacer'), {
  onSelect: () => schedulePreview(),
  onOpen: (i) => openFile(items[i]?.path),
})

// --- Папка поиска ------------------------------------------------------

function setRoot(dir: string, runNow = true): void {
  root = dir
  scopeName.textContent = dir ? baseName(dir) : 'Выберите папку'
  scopeButton.title = dir || 'Папка для поиска'
  scopeButton.classList.toggle('scope-empty', !dir)
  store.set('root', dir)
  markActiveRecent()
  if (runNow && queryInput.value.trim()) startSearch(true)
  else updateEmpty()
}

async function pickFolder(): Promise<void> {
  try {
    const dir = await Go.PickFolder(root)
    if (dir) setRoot(dir)
  } catch (err) {
    toast(errorText(err))
  }
}

function renderRecent(folders: string[]): void {
  const nodes = folders.map((dir) => {
    const li = document.createElement('li')
    const open = document.createElement('button')
    open.type = 'button'
    open.className = 'side-item'
    open.title = dir
    open.dataset.dir = dir
    open.innerHTML = '<svg class="side-icon"><use href="#i-folder"/></svg><span class="side-label"></span>'
    ;(open.lastElementChild as HTMLElement).textContent = baseName(dir)
    open.addEventListener('click', () => setRoot(dir))

    const forget = document.createElement('button')
    forget.type = 'button'
    forget.className = 'side-forget'
    forget.title = 'Убрать из списка'
    forget.setAttribute('aria-label', `Убрать ${baseName(dir)} из списка`)
    forget.innerHTML = '<svg><use href="#i-close"/></svg>'
    forget.addEventListener('click', async () => renderRecent(await Go.ForgetFolder(dir)))

    li.append(open, forget)
    return li
  })
  recentList.replaceChildren(...nodes)
  markActiveRecent()
}

function markActiveRecent(): void {
  recentList.querySelectorAll<HTMLButtonElement>('.side-item').forEach((b) => {
    b.classList.toggle('active', !!root && b.dataset.dir?.toLowerCase() === root.toLowerCase())
  })
}

// --- Поиск -------------------------------------------------------------

let debounce = 0

function startSearch(immediate = false): void {
  clearTimeout(debounce)
  const query = queryInput.value.trim()
  if (!query || !root) {
    if (running) Go.CancelSearch()
    running = false
    searchId++
    items = []
    lastDone = null
    list.setItems(items)
    inspector.empty()
    updateStatus()
    updateEmpty()
    return
  }
  debounce = window.setTimeout(() => runSearch(query), immediate ? 0 : 180)
}

function runSearch(query: string): void {
  searchId++
  running = true
  lastDone = null
  items = []
  list.root = root
  list.setItems(items)
  inspector.empty()
  updateStatus()
  updateEmpty()
  Go.Search(searchId, root, query)
}

EventsOn('search:batch', (batch: SearchBatch) => {
  if (batch.id !== searchId) return
  for (const item of batch.items) items.push(item)
  if (sortKey !== 'none') scheduleResort()
  else list.appended()
  updateStatus()
  updateEmpty()
})

EventsOn('search:done', async (done: SearchDone) => {
  if (done.id !== searchId) return
  running = false
  lastDone = done
  if (sortKey !== 'none') resort()
  updateStatus()
  updateEmpty()
  if (!done.error && !done.canceled) renderRecent(await Go.RecentFolders())
})

function updateStatus(): void {
  spinner.hidden = !running
  stopButton.hidden = !running
  statusWarn.textContent = ''
  const n = items.length
  const files = `${formatCount(n)} ${plural(n, 'файл', 'файла', 'файлов')}`

  if (running) {
    statusEl.textContent = n ? `Ищу… найдено ${files}` : 'Ищу…'
    return
  }
  const done = lastDone
  if (!done || done.error) {
    statusEl.textContent = ''
    return
  }
  const dirs = `${formatCount(done.stats.dirs)} ${plural(done.stats.dirs, 'папке', 'папках', 'папках')}`
  if (done.canceled) statusEl.textContent = `Поиск остановлен, найдено ${files}`
  else if (n === 0) statusEl.textContent = `Ничего не найдено в ${dirs} за ${seconds(done.ms)}`
  else statusEl.textContent = `Найдено ${files} в ${dirs} за ${seconds(done.ms)}`

  if (done.stats.limited) {
    statusWarn.textContent = `Показаны первые ${files}, уточните запрос`
  } else if (done.stats.skipped > 0) {
    const s = done.stats.skipped
    statusWarn.textContent = `Нет доступа к ${formatCount(s)} ${plural(s, 'папке', 'папкам', 'папкам')}`
  }
}

function updateEmpty(): void {
  let title = ''
  let text = ''
  if (!root) {
    title = 'Выберите папку'
    text = 'Нажмите «Выбрать папку…» слева или на папку в поле поиска. Сетевые диски тоже подходят.'
  } else if (!queryInput.value.trim()) {
    title = 'Начните вводить имя файла'
    text = `Поиск идёт по папке «${baseName(root)}» и всем вложенным папкам, без учёта регистра.`
  } else if (lastDone?.error) {
    title = 'Не удалось выполнить поиск'
    text = lastDone.error
  } else if (!running && lastDone && items.length === 0) {
    title = 'Ничего не найдено'
    text = 'Проверьте написание или выберите другую папку.'
  }

  emptyEl.hidden = !title
  if (title) {
    const h = document.createElement('p')
    h.className = 'empty-title'
    h.textContent = title
    const p = document.createElement('p')
    p.className = 'empty-text'
    p.textContent = text
    emptyEl.replaceChildren(h, p)
  }
}

// --- Сортировка --------------------------------------------------------

const compare: Record<Exclude<SortKey, 'none'>, (a: FileItem, b: FileItem) => number> = {
  name: (a, b) => collator.compare(a.name, b.name),
  mod: (a, b) => a.mod - b.mod,
  size: (a, b) => a.size - b.size,
  kind: (a, b) => collator.compare(kindOf(a.ext).label, kindOf(b.ext).label) || collator.compare(a.name, b.name),
  dir: (a, b) => collator.compare(a.dir, b.dir) || collator.compare(a.name, b.name),
}

function resort(): void {
  if (sortKey === 'none') return
  const fn = compare[sortKey]
  const keep = list.current?.path
  items.sort((a, b) => sortDir * fn(a, b))
  list.setItems(items, keep)
}

let resortTimer = 0
function scheduleResort(): void {
  if (!resortTimer) resortTimer = window.setTimeout(() => { resortTimer = 0; resort() }, 250)
}

listHead.addEventListener('click', (e) => {
  const button = (e.target as HTMLElement).closest<HTMLButtonElement>('button[data-key]')
  if (!button) return
  const key = button.dataset.key as SortKey
  if (key === sortKey) sortDir = -sortDir
  else {
    sortKey = key
    sortDir = key === 'mod' || key === 'size' ? -1 : 1 // как в Finder: новые и крупные сверху
  }
  listHead.querySelectorAll<HTMLButtonElement>('button').forEach((b) => {
    const active = b === button
    b.classList.toggle('sorted', active)
    b.classList.toggle('desc', active && sortDir < 0)
    if (active) b.setAttribute('aria-sort', sortDir > 0 ? 'ascending' : 'descending')
    else b.removeAttribute('aria-sort')
  })
  resort()
})

// --- Предпросмотр ------------------------------------------------------

const inspector = new Inspector(byId('inspectorBody'), {
  open: (path) => openFile(path),
  reveal: (path) => revealFile(path),
})
inspector.empty()

let previewTimer = 0
let previewRequest = 0

function setInspectorOpen(open: boolean): void {
  inspectorEl.hidden = !open
  app.classList.toggle('with-inspector', open)
  inspectorToggle.setAttribute('aria-pressed', String(open))
  store.set('inspector', open ? '1' : '0')
  if (open) schedulePreview(0)
}

function schedulePreview(delay = 110): void {
  clearTimeout(previewTimer)
  if (inspectorEl.hidden) return
  const item = list.current
  if (!item) return
  previewTimer = window.setTimeout(async () => {
    const request = ++previewRequest
    inspector.loading(item)
    try {
      const result = (await Go.Preview(item.path)) as PreviewResult
      if (request === previewRequest) inspector.show(item, result)
    } catch (err) {
      if (request === previewRequest) inspector.failed(item, errorText(err))
    }
  }, delay)
}

async function openFile(path?: string): Promise<void> {
  if (!path) return
  try { await Go.OpenFile(path) } catch (err) { toast(errorText(err)) }
}

async function revealFile(path?: string): Promise<void> {
  if (!path) return
  try { await Go.ShowInExplorer(path) } catch (err) { toast(errorText(err)) }
}

// --- Обновления --------------------------------------------------------

const updateButton = byId<HTMLButtonElement>('update')
const updateDot = byId<HTMLSpanElement>('updateDot')
const updateNote = byId<HTMLParagraphElement>('updateNote')
const sheet = byId<HTMLDialogElement>('updateSheet')
const sheetConfirm = byId<HTMLButtonElement>('sheetConfirm')
const sheetCancel = byId<HTMLButtonElement>('sheetCancel')
const sheetProgress = byId<HTMLDivElement>('sheetProgress')
const sheetBar = byId<HTMLDivElement>('sheetBar')
const sheetError = byId<HTMLParagraphElement>('sheetError')

let noteTimer = 0
function updateNoteText(text: string): void {
  updateNote.textContent = text
  clearTimeout(noteTimer)
  noteTimer = window.setTimeout(() => { updateNote.textContent = '' }, 5000)
}

function applyUpdateInfo(info: UpdateInfo): void {
  updateDot.hidden = !info.available
}

EventsOn('update:available', applyUpdateInfo)
EventsOn('update:progress', (p: number) => { sheetBar.style.width = `${Math.round(p * 100)}%` })

updateButton.addEventListener('click', async () => {
  updateButton.disabled = true
  updateButton.textContent = 'Проверяю…'
  try {
    const info = (await Go.CheckUpdate()) as UpdateInfo
    applyUpdateInfo(info)
    if (info.available) openUpdateSheet(info)
    else updateNoteText(`Установлена последняя версия`)
  } catch (err) {
    updateNoteText(errorText(err))
  } finally {
    updateButton.disabled = false
    updateButton.textContent = 'Обновить'
  }
})

function openUpdateSheet(info: UpdateInfo): void {
  byId('sheetTitle').textContent = `Доступна версия ${info.version}`
  byId('sheetText').textContent = `Сейчас установлена ${info.current}. После обновления программа перезапустится.`
  byId('sheetNotes').textContent = info.notes ?? ''
  sheetError.textContent = ''
  sheetProgress.hidden = true
  sheetBar.style.width = '0%'
  sheetConfirm.disabled = sheetCancel.disabled = false
  sheet.showModal()
}

sheetConfirm.addEventListener('click', async () => {
  sheetConfirm.disabled = sheetCancel.disabled = true
  sheetError.textContent = ''
  sheetProgress.hidden = false
  try {
    await Go.InstallUpdate()
  } catch (err) {
    sheetError.textContent = errorText(err)
    sheetProgress.hidden = true
    sheetConfirm.disabled = sheetCancel.disabled = false
  }
})

// --- Ввод и горячие клавиши --------------------------------------------

queryInput.addEventListener('input', () => startSearch())
queryInput.addEventListener('keydown', (e) => {
  if (e.key === 'Enter') startSearch(true)
  if (e.key === 'ArrowDown' && items.length) {
    e.preventDefault()
    listEl.focus()
    list.select(Math.max(0, list.selected))
  }
})
scopeButton.addEventListener('click', pickFolder)
byId('pick').addEventListener('click', pickFolder)
stopButton.addEventListener('click', () => Go.CancelSearch())
inspectorToggle.addEventListener('click', () => setInspectorOpen(inspectorEl.hidden))

listEl.addEventListener('keydown', (e) => {
  if (e.key === ' ') {
    e.preventDefault()
    setInspectorOpen(inspectorEl.hidden)
  } else if (e.key === 'Enter' && e.ctrlKey) {
    e.preventDefault()
    revealFile(list.current?.path)
  }
})

document.addEventListener('keydown', (e) => {
  const typing = document.activeElement === queryInput
  if ((e.ctrlKey && (e.key === 'f' || e.key === 'l')) || (e.key === '/' && !typing)) {
    e.preventDefault()
    queryInput.focus()
    queryInput.select()
  } else if (e.key === 'Escape' && running) {
    Go.CancelSearch()
  }
})

// --- Запуск ------------------------------------------------------------

// Повторяет вызов, если Go не ответил вовремя: в режиме разработки в браузере
// вызовы, сделанные до подключения к Go, иногда теряются.
async function patiently<T>(call: () => Promise<T>): Promise<T> {
  for (let attempt = 1; ; attempt++) {
    const timeout = new Promise<never>((_, reject) => setTimeout(() => reject(new Error('timeout')), 1500))
    try {
      return await Promise.race([call(), timeout])
    } catch (err) {
      if (attempt >= 5 || (err as Error).message !== 'timeout') throw err
    }
  }
}

async function init(): Promise<void> {
  setInspectorOpen(store.get('inspector') !== '0')
  setRoot(store.get('root') || '', false)
  updateStatus()
  queryInput.focus()

  patiently(Go.Version).then((v) => { byId('version').textContent = `Версия ${v}` })
  const recent = await patiently(Go.RecentFolders)
  renderRecent(recent)
  if (!root && recent[0]) setRoot(recent[0], false)
}

init()
