// Декодер 1С: показывает модули и формы обработок, отчётов, конфигураций и
// расширений (.epf, .erf, .cf, .cfe — 1С 8; .ert, .md — 1С 7.7) человекочитаемым
// кодом, без Конфигуратора. Модули можно поправить и сохранить в том же формате.

import * as Go from '../wailsjs/go/main/App'
import { ClipboardSetText } from '../wailsjs/runtime/runtime'
import { renderBSL } from './bsl'
import { baseName } from './format'
import { normalize } from './highlight'
import { toast } from './toast'

export interface OneCSection {
  id: string
  group: string
  title: string
  kind: string // module | text
  text?: string // у больших файлов — подгружается при выборе
  lines: number
  editable: boolean
  note?: string
}

export interface OneCDoc {
  path: string
  name: string
  synonym?: string
  kind: string
  format: string
  sections: OneCSection[]
  canSave: boolean
}

// .md здесь нет: это ещё и Markdown. Конфигурацию 7.7 распознаёт предпросмотр и бэкенд.
export const ONEC_EXTENSIONS = ['epf', 'erf', 'cf', 'cfe', 'ert']
const FILTER_FROM = 20 // фильтр над списком, когда разделов больше
const RECENT_KEY = 'onecRecent'
const MAX_RECENT = 8

const byId = <T extends HTMLElement>(id: string) => document.getElementById(id) as T

function el<K extends keyof HTMLElementTagNameMap>(tag: K, className?: string, text?: string): HTMLElementTagNameMap[K] {
  const node = document.createElement(tag)
  if (className) node.className = className
  if (text !== undefined) node.textContent = text
  return node
}

export function isOneCFile(path: string): boolean {
  const ext = path.slice(path.lastIndexOf('.') + 1).toLowerCase()
  return ONEC_EXTENSIONS.includes(ext)
}

export class Decoder {
  /** Список недавних файлов изменился — обновить боковую панель. */
  onRecentChange: (() => void) | null = null

  private doc: OneCDoc | null = null
  private currentId = ''
  private texts = new Map<string, string>() // загруженные тексты разделов
  private edits = new Map<string, string>()
  private filter = ''
  private editing = false
  private finds: Range[] = []
  private findIndex = -1

  private view = byId<HTMLElement>('decoder')
  private empty = byId<HTMLDivElement>('decoderEmpty')
  private body = byId<HTMLDivElement>('decoderBody')
  private title = byId<HTMLHeadingElement>('decoderTitle')
  private meta = byId<HTMLParagraphElement>('decoderMeta')
  private nav = byId<HTMLElement>('decoderNav')
  private sectionTitle = byId<HTMLSpanElement>('decoderSection')
  private sectionInfo = byId<HTMLSpanElement>('decoderSectionInfo')
  private note = byId<HTMLParagraphElement>('decoderNote')
  private scroll = byId<HTMLDivElement>('codeScroll')
  private gutter = byId<HTMLPreElement>('codeGutter')
  private code = byId<HTMLPreElement>('codeText')
  private editor = byId<HTMLTextAreaElement>('codeEdit')
  private editButton = byId<HTMLButtonElement>('decoderEdit')
  private revertButton = byId<HTMLButtonElement>('decoderRevert')
  private saveButton = byId<HTMLButtonElement>('decoderSave')
  private saveAsButton = byId<HTMLButtonElement>('decoderSaveAs')
  private backButton = byId<HTMLButtonElement>('decoderBack')
  private actions = document.querySelector<HTMLDivElement>('.decoder-actions')!
  private recentBox = byId<HTMLDivElement>('decoderRecent')
  private recentList = byId<HTMLUListElement>('decoderRecentList')
  private findInput = byId<HTMLInputElement>('codeFind')
  private findCount = byId<HTMLSpanElement>('codeFindCount')

  constructor() {
    byId('decoderOpen').addEventListener('click', () => void this.pick())
    byId('decoderOpenEmpty').addEventListener('click', () => void this.pick())
    byId('decoderCopy').addEventListener('click', () => this.copy())
    byId('decoderSaveText').addEventListener('click', () => void this.saveText())
    byId('decoderReveal').addEventListener('click', () => this.doc && void Go.ShowInExplorer(this.doc.path).catch((e) => toast(String(e))))
    this.editButton.addEventListener('click', () => this.setEditing(!this.editing))
    this.revertButton.addEventListener('click', () => this.revert())
    this.saveButton.addEventListener('click', () => void this.save())
    this.saveAsButton.addEventListener('click', () => void this.saveAs())
    this.backButton.addEventListener('click', () => this.close())
    this.nav.addEventListener('click', (e) => {
      const item = (e.target as HTMLElement).closest<HTMLElement>('[data-id]')
      if (item) void this.show(item.dataset.id!)
    })
    this.editor.addEventListener('input', () => this.onEdit())
    this.editor.addEventListener('keydown', (e) => {
      if (e.key === 'Tab' && !e.ctrlKey) {
        // Tab вставляет табуляцию, как в Конфигураторе, а не уводит фокус.
        e.preventDefault()
        document.execCommand('insertText', false, '\t')
      }
    })
    this.findInput.addEventListener('input', () => this.find())
    this.findInput.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') {
        e.preventDefault()
        this.step(e.shiftKey ? -1 : 1)
      } else if (e.key === 'Escape') {
        this.findInput.value = ''
        this.find()
      }
    })
    byId('codeFindPrev').addEventListener('click', () => this.step(-1))
    byId('codeFindNext').addEventListener('click', () => this.step(1))
    this.view.addEventListener('keydown', (e) => {
      if (e.ctrlKey && e.code === 'KeyF') {
        e.preventDefault()
        e.stopPropagation()
        this.findInput.focus()
        this.findInput.select()
      } else if (e.ctrlKey && e.code === 'KeyS') {
        e.preventDefault()
        void (e.shiftKey ? this.saveAs() : this.save())
      }
    })
    this.renderEmpty()
  }

  get hasChanges(): boolean {
    return this.edits.size > 0
  }

  // --- Недавние файлы ---------------------------------------------------

  recent(): string[] {
    try {
      return JSON.parse(localStorage.getItem(RECENT_KEY) ?? '[]') as string[]
    } catch {
      return []
    }
  }

  forget(path: string): void {
    this.storeRecent(this.recent().filter((p) => p.toLowerCase() !== path.toLowerCase()))
  }

  private remember(path: string): void {
    this.storeRecent([path, ...this.recent().filter((p) => p.toLowerCase() !== path.toLowerCase())].slice(0, MAX_RECENT))
  }

  private storeRecent(list: string[]): void {
    try { localStorage.setItem(RECENT_KEY, JSON.stringify(list)) } catch { /* без хранилища — не страшно */ }
    this.onRecentChange?.()
    if (!this.doc) this.renderEmpty()
  }

  get currentPath(): string {
    return this.doc?.path ?? ''
  }

  // --- Открытие ---------------------------------------------------------

  async pick(): Promise<void> {
    try {
      const path = await Go.PickOneCFile()
      if (path) await this.open(path)
    } catch (err) {
      toast(String(err))
    }
  }

  /** Можно ли уйти от текущего файла: несохранённые правки — только с согласия. */
  private confirmDiscard(action: string): boolean {
    return !this.hasChanges || !this.doc ||
      confirm(`В «${this.doc.name}» есть несохранённые правки. ${action} и потерять их?`)
  }

  /** Вернуться на стартовый экран декодера. */
  close(): void {
    if (!this.confirmDiscard('Закрыть файл')) return
    this.doc = null
    this.edits.clear()
    this.editing = false
    this.renderEmpty()
    this.onRecentChange?.()
  }

  async open(path: string, force = false): Promise<boolean> {
    if (!force && this.doc?.path !== path && !this.confirmDiscard('Открыть другой файл')) {
      return false
    }
    try {
      const doc = (await Go.OpenOneC(path)) as OneCDoc
      this.doc = doc
      this.texts.clear()
      for (const sec of doc.sections) if (sec.text !== undefined && sec.text !== '') this.texts.set(sec.id, sec.text)
      this.edits.clear()
      this.filter = ''
      this.editing = false
      this.findInput.value = '' // поиск прошлого файла здесь не нужен
      this.remember(path)
      this.render()
      return true
    } catch (err) {
      toast(`Не удалось открыть «${baseName(path)}»: ${err}`)
      this.forget(path)
      return false
    }
  }

  // --- Отрисовка --------------------------------------------------------

  private renderEmpty(): void {
    this.empty.hidden = false
    this.body.hidden = true
    this.backButton.hidden = true
    this.actions.hidden = true // кнопкам сохранения нечего делать без открытого файла
    this.title.textContent = 'Декодер 1С'
    this.meta.textContent = 'Внешние обработки и отчёты 1С:Предприятия 8 и 7.7'
    this.saveButton.disabled = this.saveAsButton.disabled = true
    byId<HTMLButtonElement>('decoderSaveText').disabled = true
    byId<HTMLButtonElement>('decoderReveal').disabled = true

    // Стартовый экран: недавние файлы — открыть одним щелчком.
    const recent = this.recent()
    this.recentBox.hidden = !recent.length
    this.recentList.replaceChildren(...recent.map((path) => {
      const li = el('li')
      const b = el('button', 'decoder-recent-item')
      b.type = 'button'
      b.title = path
      b.innerHTML = '<svg><use href="#i-onec"/></svg>'
      b.append(el('span', 'decoder-recent-name', baseName(path)), el('span', 'decoder-recent-path', path))
      b.addEventListener('click', () => void this.open(path).then(() => this.onRecentChange?.()))
      li.append(b)
      return li
    }))
  }

  private render(): void {
    const doc = this.doc
    if (!doc) return this.renderEmpty()
    this.empty.hidden = true
    this.body.hidden = false
    this.backButton.hidden = false
    this.actions.hidden = false
    byId<HTMLButtonElement>('decoderSaveText').disabled = false
    byId<HTMLButtonElement>('decoderReveal').disabled = false
    this.title.textContent = doc.synonym && doc.synonym !== doc.name ? `${doc.name} — ${doc.synonym}` : doc.name
    this.meta.textContent = `${doc.kind}, ${doc.format}. ${baseName(doc.path)}`
    this.renderNav()
    const first = doc.sections.find((s) => s.kind === 'module' && s.lines > 1) ?? doc.sections[0]
    void this.show(first.id)
  }

  private renderNav(): void {
    const doc = this.doc!
    const nodes: HTMLElement[] = []
    if (doc.sections.length > FILTER_FROM) nodes.push(this.filterBox())
    const query = normalize(this.filter.trim())
    const visible = query ? doc.sections.filter((s) => normalize(`${s.group} ${s.title}`).includes(query)) : doc.sections
    // Группы — в порядке, в котором их отдал разбор (как в Конфигураторе).
    const groups = [...new Set(visible.map((s) => s.group))]
    for (const group of groups) {
      const items = visible.filter((x) => x.group === group)
      nodes.push(el('p', 'dnav-group', items.length > 1 ? `${group} · ${items.length}` : group))
      for (const s of items) {
        const item = el('button', 'dnav-item')
        item.type = 'button'
        item.dataset.id = s.id
        item.title = s.title
        item.setAttribute('aria-current', String(s.id === this.currentId))
        item.append(el('span', 'dnav-title', s.title), el('span', 'dnav-meta', s.lines ? `${s.lines} стр.` : '—'))
        if (this.edits.has(s.id)) item.append(el('span', 'dnav-dot'))
        nodes.push(item)
      }
    }
    if (!visible.length) nodes.push(el('p', 'dnav-empty', 'Ничего не найдено'))
    // Поле фильтра пересоздаётся — сохраняем фокус и курсор.
    const focused = document.activeElement?.classList.contains('dnav-filter-input')
    this.nav.replaceChildren(...nodes)
    if (focused) {
      const input = this.nav.querySelector<HTMLInputElement>('.dnav-filter-input')
      input?.focus()
      input?.setSelectionRange(input.value.length, input.value.length)
    }
  }

  private filterBox(): HTMLElement {
    const box = el('label', 'dnav-filter')
    box.innerHTML = '<svg><use href="#i-search"/></svg>'
    const input = el('input', 'dnav-filter-input')
    input.type = 'search'
    input.placeholder = 'Фильтр модулей'
    input.spellcheck = false
    input.value = this.filter
    input.setAttribute('aria-label', 'Фильтр модулей и форм')
    input.addEventListener('input', () => {
      this.filter = input.value
      this.renderNav()
    })
    box.append(input)
    return box
  }

  private get section(): OneCSection | undefined {
    return this.doc?.sections.find((s) => s.id === this.currentId)
  }

  private textOf(s: OneCSection): string {
    return this.edits.get(s.id) ?? this.texts.get(s.id) ?? ''
  }

  private original(s: OneCSection): string {
    return this.texts.get(s.id) ?? ''
  }

  private async show(id: string): Promise<void> {
    if (this.editing) this.setEditing(false)
    this.currentId = id
    const s = this.section
    if (!s || !this.doc) return
    if (!this.texts.has(id) && s.lines) {
      // Текст модуля большого файла — по запросу.
      this.sectionTitle.textContent = s.title
      this.code.textContent = 'Загрузка…'
      try {
        const text = await Go.OneCSectionText(this.doc.path, id)
        this.texts.set(id, text)
      } catch (err) {
        toast(String(err))
        return
      }
      if (this.currentId !== id) return // пока грузилось, выбрали другое
    }
    this.nav.querySelectorAll<HTMLElement>('[data-id]').forEach((b) => b.setAttribute('aria-current', String(b.dataset.id === id)))
    this.sectionTitle.textContent = s.title
    this.note.textContent = s.note ?? ''
    this.note.hidden = !s.note
    this.renderCode()
    this.scroll.scrollTop = 0
    this.updateButtons()
    this.find()
  }

  private renderCode(): void {
    const s = this.section!
    const text = this.textOf(s)
    const lines = text ? text.split('\n').length : 0
    this.sectionInfo.textContent = lines ? `${lines} стр.` : ''
    this.gutter.textContent = Array.from({ length: Math.max(1, lines) }, (_, i) => i + 1).join('\n')
    if (s.kind === 'module') renderBSL(this.code, text)
    else this.code.textContent = text
    this.code.classList.toggle('code-plain', s.kind !== 'module')
  }

  private updateButtons(): void {
    const s = this.section
    const doc = this.doc
    this.editButton.disabled = !s?.editable
    this.editButton.setAttribute('aria-pressed', String(this.editing))
    this.editButton.textContent = this.editing ? 'Готово' : 'Изменить'
    this.revertButton.hidden = !s || !this.edits.has(s.id)
    const ext = doc ? doc.path.slice(doc.path.lastIndexOf('.')) : ''
    this.saveButton.disabled = this.saveAsButton.disabled = !doc?.canSave || !this.hasChanges
    this.saveButton.title = doc && !doc.canSave
      ? 'Этот файл нельзя записать: модуль не прочитан (возможно, защищён паролем)'
      : `Сохранить правки в этот же файл ${ext} (Ctrl+S). Копия исходника — в резервных копиях`
    this.saveAsButton.title = `Сохранить правки в новый файл ${ext} (Ctrl+Shift+S)`
  }

  // --- Правка -----------------------------------------------------------

  private setEditing(on: boolean): void {
    const s = this.section
    if (on && !s?.editable) return
    this.editing = on
    this.scroll.classList.toggle('editing', on)
    this.editor.hidden = !on
    this.code.hidden = on
    this.gutter.hidden = on
    if (on && s) {
      this.editor.value = this.textOf(s)
      this.editor.focus()
      this.editor.setSelectionRange(0, 0)
      this.editor.scrollTop = 0
    } else {
      this.renderCode()
      this.find()
    }
    this.updateButtons()
  }

  private onEdit(): void {
    const s = this.section
    if (!s) return
    if (this.editor.value === this.original(s)) this.edits.delete(s.id)
    else this.edits.set(s.id, this.editor.value)
    this.renderNav()
    this.updateButtons()
  }

  private revert(): void {
    const s = this.section
    if (!s) return
    this.edits.delete(s.id)
    if (this.editing) this.editor.value = this.original(s)
    this.renderCode()
    this.renderNav()
    this.updateButtons()
    toast('Правки модуля отменены')
  }

  /** «Сохранить»: правки — в этот же файл и в том же формате. */
  private async save(): Promise<void> {
    const doc = this.doc
    if (!doc || !doc.canSave || !this.hasChanges) return
    const keep = this.currentId
    try {
      const backup = await Go.SaveOneCInPlace(doc.path, Object.fromEntries(this.edits))
      this.edits.clear()
      await this.open(doc.path, true) // перечитать файл с диска — показываем то, что записано
      await this.show(keep)
      toast(`Сохранено в «${baseName(doc.path)}». Исходник — в резервных копиях`, 'Показать копию', () => void Go.ShowInExplorer(backup))
    } catch (err) {
      toast(String(err))
    }
  }

  /** «Сохранить как…»: правки — в новый файл того же формата. */
  private async saveAs(): Promise<void> {
    const doc = this.doc
    if (!doc || !doc.canSave || !this.hasChanges) return
    try {
      const target = await Go.SaveOneC(doc.path, Object.fromEntries(this.edits))
      if (!target) return
      toast(`Сохранено: ${baseName(target)}`, 'Открыть', () => {
        this.edits.clear()
        void this.open(target, true)
      })
    } catch (err) {
      toast(String(err))
    }
  }

  private async saveText(): Promise<void> {
    const s = this.section
    if (!s || !this.doc) return
    try {
      const ext = s.kind === 'module' ? 'bsl' : 'txt'
      const target = await Go.SaveOneCText(`${this.doc.name} — ${s.title}.${ext}`, this.textOf(s))
      if (target) toast(`Сохранено: ${baseName(target)}`, 'Показать в Проводнике', () => void Go.ShowInExplorer(target))
    } catch (err) {
      toast(String(err))
    }
  }

  private copy(): void {
    const s = this.section
    if (!s) return
    void ClipboardSetText(this.editing ? this.editor.value : this.textOf(s)).then(() => toast('Текст модуля скопирован'))
  }

  // --- Поиск в модуле ---------------------------------------------------
  // CSS Custom Highlight API: подсветка поверх готового кода, без перестройки DOM.

  private find(): void {
    const query = normalize(this.findInput.value)
    this.finds = []
    this.findIndex = -1
    const highlights = (CSS as unknown as { highlights?: Map<string, unknown> }).highlights
    highlights?.delete('code-find')
    highlights?.delete('code-find-current')
    if (!query || this.editing) {
      this.findCount.textContent = ''
      return
    }
    const walker = document.createTreeWalker(this.code, NodeFilter.SHOW_TEXT)
    for (let node = walker.nextNode(); node && this.finds.length < 5000; node = walker.nextNode()) {
      const text = normalize(node.nodeValue ?? '')
      for (let at = text.indexOf(query); at >= 0; at = text.indexOf(query, at + query.length)) {
        const range = new Range()
        range.setStart(node, at)
        range.setEnd(node, at + query.length)
        this.finds.push(range)
      }
    }
    if (highlights && typeof Highlight !== 'undefined') highlights.set('code-find', new Highlight(...this.finds))
    this.findCount.textContent = this.finds.length ? '' : 'Нет совпадений'
    if (this.finds.length) this.step(1)
  }

  private step(dir: 1 | -1): void {
    if (!this.finds.length) return
    this.findIndex = (this.findIndex + dir + this.finds.length) % this.finds.length
    const range = this.finds[this.findIndex]
    const highlights = (CSS as unknown as { highlights?: Map<string, unknown> }).highlights
    if (highlights && typeof Highlight !== 'undefined') highlights.set('code-find-current', new Highlight(range))
    this.findCount.textContent = `${this.findIndex + 1} из ${this.finds.length}`
    const rect = range.getBoundingClientRect()
    const box = this.scroll.getBoundingClientRect()
    if (rect.top < box.top + 40 || rect.bottom > box.bottom - 40) {
      this.scroll.scrollTop += rect.top - box.top - box.height / 3
    }
  }
}
