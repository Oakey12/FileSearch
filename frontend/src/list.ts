// Виртуальный список: в DOM живут только видимые строки (около 40),
// поэтому 100 000 результатов прокручиваются так же быстро, как 10.

import { baseName, formatDate, formatSize, kindOf } from './format'
import { highlightInto } from './highlight'

export interface FileItem {
  path: string
  name: string
  dir: string
  ext: string
  size: number
  mod: number
  snip?: string // фрагмент текста при поиске по содержимому
  hits?: number
}

interface Handlers {
  onSelect(index: number): void
  onOpen(index: number): void
}

const ROW_HEIGHT = 28
const ROW_HEIGHT_TEXT = 48 // строка с фрагментом текста
const OVERSCAN = 6

export class VirtualList {
  items: FileItem[] = []
  selected = -1
  /** Папка поиска: в колонке «Папка» путь показывается относительно неё. */
  root = ''

  private rowHeight = ROW_HEIGHT
  private terms: string[] = []
  private textMode = false

  private pool: HTMLElement[] = []
  private frame = 0
  private viewport = 0

  constructor(
    private scroller: HTMLElement,
    private spacer: HTMLElement,
    private handlers: Handlers,
  ) {
    scroller.addEventListener('scroll', () => this.schedule(), { passive: true })
    new ResizeObserver(() => {
      this.viewport = scroller.clientHeight
      this.ensurePool()
      this.schedule()
    }).observe(scroller)

    scroller.addEventListener('mousedown', (e) => {
      const index = this.indexFromEvent(e)
      if (index >= 0) this.select(index, false)
    })
    scroller.addEventListener('dblclick', (e) => {
      const index = this.indexFromEvent(e)
      if (index >= 0) handlers.onOpen(index)
    })
    scroller.addEventListener('keydown', (e) => this.onKey(e))
  }

  /** Заменяет весь список. Выделение переносится на тот же файл, если он остался. */
  setItems(items: FileItem[], keepPath?: string): void {
    this.items = items
    this.selected = keepPath ? items.findIndex((it) => it.path === keepPath) : -1
    this.spacer.style.height = `${items.length * this.rowHeight}px`
    if (!keepPath) this.scroller.scrollTop = 0
    this.renderNow()
  }

  /** Сообщает, что в конец items добавлены элементы. */
  appended(): void {
    this.spacer.style.height = `${this.items.length * this.rowHeight}px`
    this.schedule()
  }

  /** Режим поиска по тексту: строки выше, под именем — фрагмент с подсветкой слов. */
  setTextMode(on: boolean, terms: string[]): void {
    this.textMode = on
    this.terms = terms
    this.rowHeight = on ? ROW_HEIGHT_TEXT : ROW_HEIGHT
    this.scroller.classList.toggle('text-mode', on)
    this.spacer.style.height = `${this.items.length * this.rowHeight}px`
    for (const row of this.pool) row.dataset.index = '' // перерисовать все строки
    this.ensurePool()
    this.renderNow()
  }

  get current(): FileItem | undefined {
    return this.items[this.selected]
  }

  select(index: number, scroll = true): void {
    if (index < 0 || index >= this.items.length) return
    const changed = index !== this.selected
    this.selected = index
    if (scroll) this.scrollIntoView(index)
    this.renderNow()
    if (changed) this.handlers.onSelect(index)
  }

  private scrollIntoView(index: number): void {
    const top = index * this.rowHeight
    const bottom = top + this.rowHeight
    if (top < this.scroller.scrollTop) this.scroller.scrollTop = top
    else if (bottom > this.scroller.scrollTop + this.viewport) this.scroller.scrollTop = bottom - this.viewport
  }

  private onKey(e: KeyboardEvent): void {
    const page = Math.max(1, Math.floor(this.viewport / this.rowHeight) - 1)
    const last = this.items.length - 1
    const from = this.selected < 0 ? -1 : this.selected
    let next: number | null = null
    switch (e.key) {
      case 'ArrowDown': next = Math.min(last, from + 1); break
      case 'ArrowUp': next = Math.max(0, from - 1); break
      case 'PageDown': next = Math.min(last, from + page); break
      case 'PageUp': next = Math.max(0, from - page); break
      case 'Home': next = 0; break
      case 'End': next = last; break
      case 'Enter':
        if (this.selected >= 0 && !e.ctrlKey) {
          e.preventDefault()
          this.handlers.onOpen(this.selected)
        }
        return
      default:
        return
    }
    e.preventDefault()
    if (last >= 0) this.select(next)
  }

  // «C:\Docs\Отчёты\2026» при поиске в «C:\Docs» → «Отчёты\2026».
  // Файлы прямо в папке поиска показывают её имя.
  private relativeDir(dir: string): string {
    const root = this.root.replace(/[\\/]+$/, '')
    if (!root || dir.length < root.length || dir.slice(0, root.length).toLowerCase() !== root.toLowerCase()) return dir
    return dir.slice(root.length).replace(/^[\\/]+/, '') || baseName(root)
  }

  private indexFromEvent(e: Event): number {
    const row = (e.target as HTMLElement).closest<HTMLElement>('.row')
    return row ? Number(row.dataset.index) : -1
  }

  private ensurePool(): void {
    const needed = Math.ceil(this.viewport / this.rowHeight) + OVERSCAN * 2
    while (this.pool.length < needed) {
      const row = document.createElement('div')
      row.className = 'row'
      row.setAttribute('role', 'option')
      row.innerHTML =
        '<span class="cell c-name"><svg class="ficon"><use href="#i-doc"/></svg><span class="name"></span></span>' +
        '<span class="cell c-mod"></span><span class="cell c-size"></span>' +
        '<span class="cell c-kind"></span><span class="cell c-dir"></span>' +
        '<span class="cell c-snip"></span>'
      row.hidden = true
      this.spacer.appendChild(row)
      this.pool.push(row)
    }
  }

  private schedule(): void {
    if (!this.frame) this.frame = requestAnimationFrame(() => this.renderNow())
  }

  private renderNow(): void {
    if (this.frame) cancelAnimationFrame(this.frame)
    this.frame = 0
    const first = Math.max(0, Math.floor(this.scroller.scrollTop / this.rowHeight) - OVERSCAN)

    for (let slot = 0; slot < this.pool.length; slot++) {
      const row = this.pool[slot]
      const index = first + slot
      const item = this.items[index]
      if (!item) {
        row.hidden = true
        continue
      }
      row.hidden = false
      const isSelected = index === this.selected
      row.classList.toggle('selected', isSelected)
      row.id = isSelected ? 'row-active' : ''
      row.setAttribute('aria-selected', String(isSelected))
      if (row.dataset.index === String(index) && row.dataset.path === item.path) continue

      row.dataset.index = String(index)
      row.dataset.path = item.path
      row.style.transform = `translateY(${index * this.rowHeight}px)`
      row.classList.toggle('odd', index % 2 === 1)
      const kind = kindOf(item.ext)
      const cells = row.children
      const nameCell = cells[0] as HTMLElement
      nameCell.dataset.cat = kind.category
      ;(nameCell.lastElementChild as HTMLElement).textContent = item.name
      cells[1].textContent = formatDate(item.mod)
      cells[2].textContent = formatSize(item.size)
      cells[3].textContent = kind.label
      cells[4].textContent = this.relativeDir(item.dir)
      const snip = cells[5] as HTMLElement
      if (this.textMode && item.snip) highlightInto(snip, item.snip, this.terms, 20)
      else snip.textContent = ''
      row.title = item.path
    }

    if (this.selected >= 0) this.scroller.setAttribute('aria-activedescendant', 'row-active')
    else this.scroller.removeAttribute('aria-activedescendant')
  }
}
