// Заметки в духе iPhone: список слева, редактор справа.
// Первая строка заметки — её заголовок (выделяется жирным через ::first-line).

import * as Go from '../wailsjs/go/main/App'
import { normalize } from './highlight'
import { toast } from './toast'

interface Note {
  id: string
  text: string
  created: number
  updated: number
}

const byId = <T extends HTMLElement>(id: string) => document.getElementById(id) as T

const timeFormat = new Intl.DateTimeFormat('ru-RU', { hour: '2-digit', minute: '2-digit' })
const weekdayFormat = new Intl.DateTimeFormat('ru-RU', { weekday: 'long' })
const shortDate = new Intl.DateTimeFormat('ru-RU', { day: '2-digit', month: '2-digit', year: 'numeric' })
const longDate = new Intl.DateTimeFormat('ru-RU', { day: 'numeric', month: 'long', year: 'numeric' })

const DAY = 86_400_000

function startOfDay(ms: number): number {
  const d = new Date(ms)
  d.setHours(0, 0, 0, 0)
  return d.getTime()
}

// Как в «Заметках»: сегодня — время, вчера — «Вчера», на неделе — день недели.
function listDate(ms: number): string {
  const today = startOfDay(Date.now())
  if (ms >= today) return timeFormat.format(ms)
  if (ms >= today - DAY) return 'Вчера'
  if (ms >= today - 6 * DAY) return weekdayFormat.format(ms)
  return shortDate.format(ms)
}

function lines(text: string): string[] {
  return text.split('\n').map((l) => l.trim()).filter(Boolean)
}

export const newId = () => Date.now().toString(36) + Math.random().toString(36).slice(2, 8)

export class Notes {
  /** Заметки изменились — холст обновит карточки-заметки. */
  onChange: (() => void) | null = null
  /** Нажата «На холст». */
  onToBoard: ((id: string) => void) | null = null

  private notes: Note[] = []
  private currentId: string | null = null
  private filter = ''
  private saveTimer = 0
  private loaded = false

  private list = byId<HTMLUListElement>('notesList')
  private editor = byId<HTMLDivElement>('noteText')
  private date = byId<HTMLTimeElement>('noteDate')
  private search = byId<HTMLInputElement>('notesSearch')
  private deleteButton = byId<HTMLButtonElement>('noteDelete')
  private boardButton = byId<HTMLButtonElement>('noteToBoard')

  constructor() {
    byId('notesNew').addEventListener('click', () => this.create())
    this.deleteButton.addEventListener('click', () => this.remove())
    this.boardButton.addEventListener('click', () => {
      if (this.currentId) this.onToBoard?.(this.currentId)
    })
    this.search.addEventListener('input', () => {
      this.filter = normalize(this.search.value.trim())
      this.renderList()
    })
    this.editor.addEventListener('input', () => this.onInput())
    this.list.addEventListener('click', (e) => {
      const item = (e.target as HTMLElement).closest<HTMLElement>('[data-id]')
      if (item) this.select(item.dataset.id!, true)
    })
    this.list.addEventListener('keydown', (e) => this.onListKey(e))
  }

  async open(): Promise<void> {
    if (!this.loaded) {
      this.loaded = true
      try {
        const raw = await Go.LoadNotes('notes')
        this.notes = raw ? (JSON.parse(raw).notes ?? []) : []
      } catch (err) {
        toast(`Не удалось загрузить заметки: ${err}`)
      }
      this.sort()
      this.select(this.notes[0]?.id ?? null)
    }
    this.renderList()
  }

  /** Заголовок и текст заметки для карточки на холсте. */
  get(id: string): { title: string; body: string } | null {
    const note = this.notes.find((n) => n.id === id)
    if (!note) return null
    const [title, ...rest] = note.text.split('\n')
    return { title: title.trim() || 'Новая заметка', body: rest.join('\n').trim() }
  }

  all(): { id: string; title: string; updated: number }[] {
    return this.notes.map((n) => ({ id: n.id, title: this.get(n.id)!.title, updated: n.updated }))
  }

  /** Открывает заметку в редакторе (из холста). */
  async show(id: string): Promise<void> {
    await this.open()
    this.search.value = ''
    this.filter = ''
    this.select(id, true)
  }

  focusSearch(): void {
    this.search.focus()
    this.search.select()
  }

  focusEditor(): void {
    this.editor.focus()
  }

  /** Сохраняет немедленно — при закрытии шторки или окна. */
  flush(): void {
    if (this.saveTimer) {
      clearTimeout(this.saveTimer)
      this.saveTimer = 0
      void this.save()
    }
  }

  private get current(): Note | undefined {
    return this.notes.find((n) => n.id === this.currentId)
  }

  private create(text = ''): Note {
    const now = Date.now()
    const note: Note = { id: newId(), text, created: now, updated: now }
    this.notes.unshift(note)
    this.search.value = ''
    this.filter = ''
    this.select(note.id)
    this.editor.focus()
    this.scheduleSave()
    return note
  }

  private select(id: string | null, focusEditor = false): void {
    this.currentId = id
    const note = this.current
    this.editor.textContent = note?.text ?? ''
    this.date.textContent = note ? `${longDate.format(note.updated)} в ${timeFormat.format(note.updated)}` : ''
    this.deleteButton.disabled = !note
    this.boardButton.disabled = !note
    this.renderList()
    if (focusEditor) {
      this.editor.focus()
      // Курсор в конец текста.
      const range = document.createRange()
      range.selectNodeContents(this.editor)
      range.collapse(false)
      const sel = getSelection()
      sel?.removeAllRanges()
      sel?.addRange(range)
    }
  }

  private onInput(): void {
    const text = this.editor.innerText.replace(/\n$/, '')
    let note = this.current
    if (!note) {
      // Начали печатать, когда заметок нет, — создаём заметку с этим текстом.
      if (!text.trim()) return
      const now = Date.now()
      note = { id: newId(), text, created: now, updated: now }
      this.notes.unshift(note)
      this.currentId = note.id
      this.deleteButton.disabled = false
      this.boardButton.disabled = false
    }
    note.text = text
    note.updated = Date.now()
    this.date.textContent = `${longDate.format(note.updated)} в ${timeFormat.format(note.updated)}`
    this.sort()
    this.renderList()
    this.scheduleSave()
    this.onChange?.()
  }

  private remove(): void {
    const note = this.current
    if (!note) return
    const index = this.notes.indexOf(note)
    this.notes.splice(index, 1)
    const next = this.notes[Math.min(index, this.notes.length - 1)]
    this.select(next?.id ?? null)
    this.scheduleSave()
    this.onChange?.()
    toast('Заметка удалена', 'Отменить', () => {
      this.notes.splice(index, 0, note)
      this.select(note.id)
      this.scheduleSave()
      this.onChange?.()
    })
  }

  private onListKey(e: KeyboardEvent): void {
    const visible = this.visible()
    const i = visible.findIndex((n) => n.id === this.currentId)
    if (e.key === 'ArrowDown' && i < visible.length - 1) this.select(visible[i + 1].id)
    else if (e.key === 'ArrowUp' && i > 0) this.select(visible[i - 1].id)
    else if (e.key === 'Enter') this.select(this.currentId, true)
    else if (e.key === 'Delete') this.remove()
    else return
    e.preventDefault()
    this.list.querySelector<HTMLElement>('.selected')?.focus()
  }

  private sort(): void {
    this.notes.sort((a, b) => b.updated - a.updated)
  }

  private visible(): Note[] {
    if (!this.filter) return this.notes
    return this.notes.filter((n) => normalize(n.text).includes(this.filter))
  }

  private renderList(): void {
    const today = startOfDay(Date.now())
    const groups: [string, Note[]][] = [['Сегодня', []], ['Предыдущие 7 дней', []], ['Ранее', []]]
    for (const note of this.visible()) {
      const group = note.updated >= today ? 0 : note.updated >= today - 7 * DAY ? 1 : 2
      groups[group][1].push(note)
    }

    const nodes: HTMLElement[] = []
    for (const [title, notes] of groups) {
      if (!notes.length) continue
      const head = document.createElement('li')
      head.className = 'notes-group'
      head.textContent = title
      nodes.push(head)
      for (const note of notes) nodes.push(this.item(note))
    }
    if (!nodes.length) {
      const empty = document.createElement('li')
      empty.className = 'notes-empty'
      empty.textContent = this.filter ? 'Ничего не найдено' : 'Заметок пока нет'
      nodes.push(empty)
    }
    this.list.replaceChildren(...nodes)
  }

  private item(note: Note): HTMLElement {
    const [title, second] = lines(note.text)
    const li = document.createElement('li')
    li.className = note.id === this.currentId ? 'note-item selected' : 'note-item'
    li.dataset.id = note.id
    li.tabIndex = note.id === this.currentId ? 0 : -1
    li.setAttribute('role', 'option')
    li.setAttribute('aria-selected', String(note.id === this.currentId))

    const t = document.createElement('p')
    t.className = 'note-item-title'
    t.textContent = title ?? 'Новая заметка'
    const meta = document.createElement('p')
    meta.className = 'note-item-meta'
    const when = document.createElement('span')
    when.className = 'note-item-date'
    when.textContent = listDate(note.updated)
    const preview = document.createElement('span')
    preview.className = 'note-item-preview'
    preview.textContent = second ?? 'Нет дополнительного текста'
    meta.append(when, preview)
    li.append(t, meta)
    return li
  }

  private scheduleSave(): void {
    clearTimeout(this.saveTimer)
    this.saveTimer = window.setTimeout(() => {
      this.saveTimer = 0
      void this.save()
    }, 400)
  }

  private async save(): Promise<void> {
    try {
      await Go.SaveNotes('notes', JSON.stringify({ version: 1, notes: this.notes }))
    } catch (err) {
      toast(`Не удалось сохранить заметки: ${err}`)
    }
  }
}
