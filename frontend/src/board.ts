// Холст со связями (в духе Holst): карточки на бесконечном поле, связи — кривые.
//
// Управление:
//   двойной щелчок по полю — новая карточка; по карточке — правка текста;
//   перетаскивание поля — сдвиг; колесо — прокрутка; Ctrl+колесо — масштаб;
//   кружок справа на карточке — потянуть к другой карточке, чтобы связать;
//   Delete — удалить выделенную карточку или связь.

import * as Go from '../wailsjs/go/main/App'
import { baseName, kindOf } from './format'
import type { FileItem } from './list'
import { newId } from './notes'
import { toast } from './toast'

interface Card {
  id: string
  x: number
  y: number
  w: number
  title: string
  text: string
  color: string
  path?: string // карточка-файл
  ext?: string
}

interface Link { id: string; from: string; to: string }

interface View { x: number; y: number; k: number }

interface BoardData { version: 1; cards: Card[]; links: Link[]; view: View }

const COLORS: [string, string][] = [
  ['', 'Без цвета'], ['#0a84ff', 'Синий'], ['#30d158', 'Зелёный'], ['#ffd60a', 'Жёлтый'],
  ['#ff9f0a', 'Оранжевый'], ['#ff453a', 'Красный'], ['#bf5af2', 'Фиолетовый'],
]
const CARD_WIDTH = 220
const GRID = 24
const MIN_ZOOM = 0.25
const MAX_ZOOM = 2.5

const byId = <T extends HTMLElement | SVGElement>(id: string) => document.getElementById(id) as unknown as T
const SVG = 'http://www.w3.org/2000/svg'

function editable(el: HTMLElement): void {
  el.contentEditable = 'plaintext-only'
  el.focus()
  const range = document.createRange()
  range.selectNodeContents(el)
  range.collapse(false)
  getSelection()?.removeAllRanges()
  getSelection()?.addRange(range)
}

export class Board {
  private cards = new Map<string, Card>()
  private links: Link[] = []
  private view: View = { x: 80, y: 60, k: 1 }

  private cardEls = new Map<string, HTMLElement>()
  private linkEls = new Map<string, SVGGElement>()
  private selected: { kind: 'card' | 'link'; id: string } | null = null
  private loaded = false
  private saveTimer = 0

  private viewport = byId<HTMLDivElement>('boardViewport')
  private world = byId<HTMLDivElement>('boardWorld')
  private svg = byId<SVGSVGElement>('boardLinks')
  private hint = byId<HTMLParagraphElement>('boardHint')
  private colors = byId<HTMLDivElement>('boardColors')
  private zoomValue = byId<HTMLButtonElement>('boardZoomValue')
  private draft: SVGPathElement

  constructor() {
    this.draft = document.createElementNS(SVG, 'path')
    this.draft.setAttribute('class', 'link link-draft')

    this.buildPalette()

    this.viewport.addEventListener('pointerdown', (e) => this.onPointerDown(e))
    this.viewport.addEventListener('dblclick', (e) => this.onDoubleClick(e))
    this.viewport.addEventListener('wheel', (e) => this.onWheel(e), { passive: false })
    this.viewport.addEventListener('keydown', (e) => this.onKey(e))

    byId('boardAdd').addEventListener('click', () => this.addCardAtCenter())
    byId('boardZoomIn').addEventListener('click', () => this.zoomBy(1.2))
    byId('boardZoomOut').addEventListener('click', () => this.zoomBy(1 / 1.2))
    byId('boardFit').addEventListener('click', () => this.fit())
    this.zoomValue.addEventListener('click', () => this.zoomTo(1))
  }

  // --- Загрузка и сохранение ------------------------------------------

  async open(): Promise<void> {
    if (this.loaded) return
    this.loaded = true
    try {
      const raw = await Go.LoadNotes('board')
      if (raw) {
        const data = JSON.parse(raw) as BoardData
        for (const card of data.cards ?? []) this.cards.set(card.id, card)
        this.links = data.links ?? []
        if (data.view) this.view = data.view
      }
    } catch (err) {
      toast(`Не удалось загрузить холст: ${err}`)
    }
    for (const card of this.cards.values()) this.mountCard(card)
    for (const link of this.links) this.mountLink(link)
    this.applyView()
    this.refreshHint()
  }

  focus(): void {
    this.viewport.focus({ preventScroll: true })
  }

  /** Есть ли что отменить по Esc внутри холста (правка или выделение). */
  handleEscape(): boolean {
    const active = document.activeElement as HTMLElement | null
    if (active?.isContentEditable && this.world.contains(active)) {
      active.blur()
      this.focus()
      return true
    }
    if (this.selected) {
      this.select(null)
      return true
    }
    return false
  }

  flush(): void {
    if (this.saveTimer) {
      clearTimeout(this.saveTimer)
      this.saveTimer = 0
      void this.save()
    }
  }

  private scheduleSave(): void {
    clearTimeout(this.saveTimer)
    this.saveTimer = window.setTimeout(() => {
      this.saveTimer = 0
      void this.save()
    }, 400)
  }

  private async save(): Promise<void> {
    const data: BoardData = { version: 1, cards: [...this.cards.values()], links: this.links, view: this.view }
    try {
      await Go.SaveNotes('board', JSON.stringify(data))
    } catch (err) {
      toast(`Не удалось сохранить холст: ${err}`)
    }
  }

  // --- Карточки --------------------------------------------------------

  /** Добавляет на холст карточку найденного файла. */
  async addFile(item: FileItem): Promise<void> {
    await this.open()
    const center = this.toWorld(this.viewport.clientWidth / 2, this.viewport.clientHeight / 2)
    const offset = (this.cards.size % 6) * 18
    this.addCard({
      id: newId(), x: center.x - CARD_WIDTH / 2 + offset, y: center.y - 40 + offset, w: CARD_WIDTH,
      // Полный путь — во всплывающей подсказке, на карточке только папка.
      title: item.name, text: baseName(item.dir), color: '', path: item.path, ext: item.ext,
    }, false)
  }

  private addCardAtCenter(): void {
    const c = this.toWorld(this.viewport.clientWidth / 2, this.viewport.clientHeight / 2)
    this.newCardAt(c.x, c.y)
  }

  private newCardAt(x: number, y: number): void {
    const card: Card = {
      id: newId(), x: Math.round(x - CARD_WIDTH / 2), y: Math.round(y - 30), w: CARD_WIDTH,
      title: '', text: '', color: '',
    }
    this.addCard(card, true)
  }

  private addCard(card: Card, edit: boolean): void {
    this.cards.set(card.id, card)
    const el = this.mountCard(card)
    el.classList.add('card-enter')
    this.select({ kind: 'card', id: card.id })
    this.refreshHint()
    this.scheduleSave()
    if (edit) editable(el.querySelector<HTMLElement>('.card-title')!)
  }

  private mountCard(card: Card): HTMLElement {
    const el = document.createElement('div')
    el.className = card.path ? 'card card-file' : 'card'
    el.dataset.id = card.id
    el.style.width = `${card.w}px`
    el.style.transform = `translate(${card.x}px, ${card.y}px)`
    el.style.setProperty('--card-color', card.color || 'transparent')

    const title = document.createElement('div')
    title.className = 'card-title'
    title.dataset.placeholder = 'Заголовок'
    title.textContent = card.title
    const text = document.createElement('div')
    text.className = card.path ? 'card-text card-path' : 'card-text'
    text.dataset.placeholder = 'Текст'
    text.textContent = card.text

    if (card.path) {
      const icon = document.createElementNS(SVG, 'svg')
      icon.setAttribute('class', 'card-icon')
      icon.dataset.cat = kindOf(card.ext ?? '').category
      const use = document.createElementNS(SVG, 'use')
      use.setAttribute('href', '#i-doc')
      icon.appendChild(use)
      el.append(icon)
      el.title = `${card.path}\nДвойной щелчок — открыть файл`
    }

    const portIn = document.createElement('span')
    portIn.className = 'card-port port-in'
    const portOut = document.createElement('span')
    portOut.className = 'card-port port-out'
    portOut.title = 'Потяните к другой карточке, чтобы связать'
    el.append(title, text, portIn, portOut)

    for (const field of [title, text]) {
      field.addEventListener('input', () => {
        const target = this.cards.get(card.id)
        if (!target) return
        if (field === title) target.title = field.innerText.replace(/\n/g, ' ')
        else target.text = field.innerText.replace(/\n$/, '')
        this.scheduleSave()
      })
      field.addEventListener('blur', () => field.removeAttribute('contenteditable'))
      field.addEventListener('keydown', (e) => {
        if (field === title && e.key === 'Enter') {
          e.preventDefault()
          if (!card.path) editable(text)
        }
      })
    }

    this.world.appendChild(el)
    this.cardEls.set(card.id, el)
    this.observeCard(el)
    return el
  }

  private resizeObserver = new ResizeObserver((entries) => {
    for (const entry of entries) this.updateLinksOf((entry.target as HTMLElement).dataset.id!)
  })

  private observeCard(el: HTMLElement): void {
    this.resizeObserver.observe(el)
  }

  private removeCard(id: string): void {
    const card = this.cards.get(id)
    if (!card) return
    const removedLinks = this.links.filter((l) => l.from === id || l.to === id)
    for (const link of removedLinks) this.removeLink(link.id, false)
    this.cards.delete(id)
    const el = this.cardEls.get(id)
    if (el) this.resizeObserver.unobserve(el)
    el?.remove()
    this.cardEls.delete(id)
    this.select(null)
    this.refreshHint()
    this.scheduleSave()
    toast('Карточка удалена', 'Отменить', () => {
      this.cards.set(card.id, card)
      this.mountCard(card)
      for (const link of removedLinks) {
        this.links.push(link)
        this.mountLink(link)
      }
      this.refreshHint()
      this.scheduleSave()
    })
  }

  private moveCard(id: string, x: number, y: number): void {
    const card = this.cards.get(id)
    const el = this.cardEls.get(id)
    if (!card || !el) return
    card.x = x
    card.y = y
    el.style.transform = `translate(${x}px, ${y}px)`
    this.updateLinksOf(id)
  }

  // --- Связи -----------------------------------------------------------

  private mountLink(link: Link): void {
    const g = document.createElementNS(SVG, 'g')
    g.setAttribute('class', 'link-group')
    g.dataset.id = link.id
    const hit = document.createElementNS(SVG, 'path')
    hit.setAttribute('class', 'link-hit')
    const line = document.createElementNS(SVG, 'path')
    line.setAttribute('class', 'link')
    const dot = document.createElementNS(SVG, 'circle')
    dot.setAttribute('class', 'link-end')
    dot.setAttribute('r', '3.5')
    g.append(hit, line, dot)
    this.svg.appendChild(g)
    this.linkEls.set(link.id, g)
    this.drawLink(link)
  }

  private removeLink(id: string, save = true): void {
    this.links = this.links.filter((l) => l.id !== id)
    this.linkEls.get(id)?.remove()
    this.linkEls.delete(id)
    if (save) {
      this.select(null)
      this.scheduleSave()
    }
  }

  private anchor(id: string, side: 'in' | 'out'): { x: number; y: number } | null {
    const card = this.cards.get(id)
    const el = this.cardEls.get(id)
    if (!card || !el) return null
    return { x: side === 'out' ? card.x + el.offsetWidth : card.x, y: card.y + el.offsetHeight / 2 }
  }

  private curve(a: { x: number; y: number }, b: { x: number; y: number }): string {
    const dx = Math.max(48, Math.abs(b.x - a.x) / 2)
    return `M ${a.x} ${a.y} C ${a.x + dx} ${a.y}, ${b.x - dx} ${b.y}, ${b.x} ${b.y}`
  }

  private drawLink(link: Link): void {
    const g = this.linkEls.get(link.id)
    const a = this.anchor(link.from, 'out')
    const b = this.anchor(link.to, 'in')
    if (!g || !a || !b) return
    const d = this.curve(a, b)
    for (const path of g.querySelectorAll('path')) path.setAttribute('d', d)
    const dot = g.querySelector('circle')!
    dot.setAttribute('cx', String(b.x))
    dot.setAttribute('cy', String(b.y))
  }

  private updateLinksOf(cardId: string): void {
    for (const link of this.links) if (link.from === cardId || link.to === cardId) this.drawLink(link)
  }

  // --- Выделение и палитра ---------------------------------------------

  private select(sel: { kind: 'card' | 'link'; id: string } | null): void {
    if (this.selected?.kind === 'card') this.cardEls.get(this.selected.id)?.classList.remove('selected')
    if (this.selected?.kind === 'link') this.linkEls.get(this.selected.id)?.classList.remove('selected')
    this.selected = sel
    if (sel?.kind === 'card') {
      const el = this.cardEls.get(sel.id)
      el?.classList.add('selected')
      if (el) this.world.appendChild(el) // выбранная карточка — поверх остальных
    }
    if (sel?.kind === 'link') this.linkEls.get(sel.id)?.classList.add('selected')

    const card = sel?.kind === 'card' ? this.cards.get(sel.id) : undefined
    this.colors.hidden = !card
    if (card) {
      this.colors.querySelectorAll<HTMLButtonElement>('[data-color]').forEach((b) => {
        b.setAttribute('aria-pressed', String(b.dataset.color === card.color))
      })
    }
  }

  private buildPalette(): void {
    for (const [color, name] of COLORS) {
      const b = document.createElement('button')
      b.type = 'button'
      b.className = 'swatch'
      b.dataset.color = color
      b.title = name
      b.style.setProperty('--swatch', color || '#636366')
      b.addEventListener('click', () => this.setColor(color))
      this.colors.appendChild(b)
    }
    const sep = document.createElement('span')
    sep.className = 'board-zoom-sep'
    const del = document.createElement('button')
    del.type = 'button'
    del.className = 'tool-btn'
    del.title = 'Удалить карточку (Delete)'
    del.innerHTML = '<svg><use href="#i-trash"/></svg>'
    del.addEventListener('click', () => this.selected?.kind === 'card' && this.removeCard(this.selected.id))
    this.colors.append(sep, del)
  }

  private setColor(color: string): void {
    if (this.selected?.kind !== 'card') return
    const card = this.cards.get(this.selected.id)
    if (!card) return
    card.color = color
    this.cardEls.get(card.id)?.style.setProperty('--card-color', color || 'transparent')
    this.select(this.selected)
    this.scheduleSave()
  }

  private refreshHint(): void {
    this.hint.hidden = this.cards.size > 0
  }

  // --- Вид: сдвиг и масштаб --------------------------------------------

  private applyView(): void {
    const { x, y, k } = this.view
    this.world.style.transform = `translate(${x}px, ${y}px) scale(${k})`
    const size = GRID * k
    this.viewport.style.backgroundSize = `${size}px ${size}px`
    this.viewport.style.backgroundPosition = `${x}px ${y}px`
    this.zoomValue.textContent = `${Math.round(k * 100)}%`
  }

  private toWorld(clientX: number, clientY: number): { x: number; y: number } {
    return { x: (clientX - this.view.x) / this.view.k, y: (clientY - this.view.y) / this.view.k }
  }

  private local(e: { clientX: number; clientY: number }): { x: number; y: number } {
    const rect = this.viewport.getBoundingClientRect()
    return { x: e.clientX - rect.left, y: e.clientY - rect.top }
  }

  private zoomAt(px: number, py: number, k: number): void {
    k = Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, k))
    const w = this.toWorld(px, py)
    this.view = { k, x: px - w.x * k, y: py - w.y * k }
    this.applyView()
    this.scheduleSave()
  }

  private zoomBy(factor: number): void {
    this.zoomAt(this.viewport.clientWidth / 2, this.viewport.clientHeight / 2, this.view.k * factor)
  }

  private zoomTo(k: number): void {
    this.zoomAt(this.viewport.clientWidth / 2, this.viewport.clientHeight / 2, k)
  }

  private fit(): void {
    if (!this.cards.size) return this.zoomTo(1)
    let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity
    for (const card of this.cards.values()) {
      const el = this.cardEls.get(card.id)!
      minX = Math.min(minX, card.x)
      minY = Math.min(minY, card.y)
      maxX = Math.max(maxX, card.x + el.offsetWidth)
      maxY = Math.max(maxY, card.y + el.offsetHeight)
    }
    const pad = 60
    const vw = this.viewport.clientWidth
    const vh = this.viewport.clientHeight
    const k = Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, Math.min((vw - pad * 2) / (maxX - minX), (vh - pad * 2) / (maxY - minY), 1.2)))
    this.view = { k, x: (vw - (maxX - minX) * k) / 2 - minX * k, y: (vh - (maxY - minY) * k) / 2 - minY * k }
    this.applyView()
    this.scheduleSave()
  }

  private onWheel(e: WheelEvent): void {
    e.preventDefault()
    const p = this.local(e)
    if (e.ctrlKey) {
      // Ctrl+колесо и щипок на тачпаде — масштаб под курсором.
      this.zoomAt(p.x, p.y, this.view.k * Math.exp(-e.deltaY * 0.0025))
    } else {
      this.view.x -= e.shiftKey ? e.deltaY : e.deltaX
      this.view.y -= e.shiftKey ? 0 : e.deltaY
      this.applyView()
      this.scheduleSave()
    }
  }

  // --- Мышь и клавиатура -----------------------------------------------

  private onPointerDown(e: PointerEvent): void {
    const target = e.target as Element
    const cardEl = target.closest<HTMLElement>('.card')
    const linkEl = target.closest<SVGGElement>('.link-group')

    if (e.button === 1 || (!cardEl && !linkEl && e.button === 0)) {
      // Сдвиг поля: средняя кнопка где угодно или левая по пустому месту.
      if (!cardEl && !linkEl) this.select(null)
      ;(document.activeElement as HTMLElement | null)?.blur?.()
      this.focus()
      this.drag(e, (dx, dy) => {
        this.view.x += dx
        this.view.y += dy
        this.applyView()
      }, () => this.scheduleSave(), 'panning')
      return
    }
    if (e.button !== 0) return

    if (linkEl) {
      this.select({ kind: 'link', id: linkEl.dataset.id! })
      this.focus()
      return
    }
    if (!cardEl) return
    const id = cardEl.dataset.id!

    if (target.classList.contains('port-out')) {
      e.preventDefault()
      this.connectFrom(id, e)
      return
    }
    // Во время правки текста карточку не таскаем — там выделяют текст.
    if ((target as HTMLElement).isContentEditable) return

    e.preventDefault()
    this.select({ kind: 'card', id })
    this.focus()
    const card = this.cards.get(id)!
    const startX = card.x
    const startY = card.y
    let moved = false
    this.drag(e, (_dx, _dy, totalX, totalY) => {
      moved = true
      this.moveCard(id, startX + totalX / this.view.k, startY + totalY / this.view.k)
    }, () => {
      if (!moved) return
      // Лёгкая привязка к сетке после отпускания.
      this.moveCard(id, Math.round(card.x / 4) * 4, Math.round(card.y / 4) * 4)
      this.scheduleSave()
    }, 'dragging')
  }

  /** Общий цикл перетаскивания с захватом указателя. */
  private drag(
    start: PointerEvent,
    onMove: (dx: number, dy: number, totalX: number, totalY: number) => void,
    onEnd: () => void,
    bodyClass: string,
  ): void {
    const el = this.viewport
    el.setPointerCapture(start.pointerId)
    document.body.classList.add(bodyClass)
    let lastX = start.clientX
    let lastY = start.clientY
    let frame = 0
    let pending: PointerEvent | null = null

    const move = (e: PointerEvent) => {
      pending = e
      if (frame) return
      frame = requestAnimationFrame(() => {
        frame = 0
        if (!pending) return
        onMove(pending.clientX - lastX, pending.clientY - lastY, pending.clientX - start.clientX, pending.clientY - start.clientY)
        lastX = pending.clientX
        lastY = pending.clientY
      })
    }
    const up = () => {
      cancelAnimationFrame(frame)
      if (pending) onMove(pending.clientX - lastX, pending.clientY - lastY, pending.clientX - start.clientX, pending.clientY - start.clientY)
      el.removeEventListener('pointermove', move)
      el.removeEventListener('pointerup', up)
      el.removeEventListener('pointercancel', up)
      document.body.classList.remove(bodyClass)
      onEnd()
    }
    el.addEventListener('pointermove', move)
    el.addEventListener('pointerup', up)
    el.addEventListener('pointercancel', up)
  }

  private connectFrom(fromId: string, e: PointerEvent): void {
    const a = this.anchor(fromId, 'out')
    if (!a) return
    this.svg.appendChild(this.draft)
    let target: HTMLElement | null = null
    let lastX = e.clientX
    let lastY = e.clientY

    const update = (clientX: number, clientY: number) => {
      lastX = clientX
      lastY = clientY
      const p = this.local({ clientX, clientY })
      this.draft.setAttribute('d', this.curve(a, this.toWorld(p.x, p.y)))
      const under = document.elementFromPoint(clientX, clientY)?.closest<HTMLElement>('.card') ?? null
      const next = under && under.dataset.id !== fromId ? under : null
      if (next !== target) {
        target?.classList.remove('drop-target')
        next?.classList.add('drop-target')
        target = next
      }
    }
    update(e.clientX, e.clientY)

    this.drag(e, (_dx, _dy, totalX, totalY) => update(e.clientX + totalX, e.clientY + totalY), () => {
      update(lastX, lastY)
      this.draft.remove()
      target?.classList.remove('drop-target')
      const toId = target?.dataset.id
      if (!toId) return
      if (this.links.some((l) => (l.from === fromId && l.to === toId) || (l.from === toId && l.to === fromId))) return
      const link: Link = { id: newId(), from: fromId, to: toId }
      this.links.push(link)
      this.mountLink(link)
      this.scheduleSave()
    }, 'connecting')
  }

  private onDoubleClick(e: MouseEvent): void {
    const target = e.target as HTMLElement
    const cardEl = target.closest<HTMLElement>('.card')
    if (!cardEl) {
      if (target.closest('.link-group')) return
      const p = this.local(e)
      const w = this.toWorld(p.x, p.y)
      this.newCardAt(w.x, w.y)
      return
    }
    const card = this.cards.get(cardEl.dataset.id!)
    if (!card) return
    if (card.path) {
      // У карточки-файла имя и путь не правятся — двойной щелчок открывает файл.
      void Go.OpenFile(card.path).catch((err) => toast(String(err)))
      return
    }
    const field = target.closest<HTMLElement>('.card-title, .card-text') ?? cardEl.querySelector<HTMLElement>('.card-title')!
    editable(field)
  }

  private onKey(e: KeyboardEvent): void {
    const active = document.activeElement as HTMLElement | null
    if (active?.isContentEditable) return
    if ((e.key === 'Delete' || e.key === 'Backspace') && this.selected) {
      e.preventDefault()
      if (this.selected.kind === 'card') this.removeCard(this.selected.id)
      else this.removeLink(this.selected.id)
    } else if (e.key === 'Enter' && this.selected?.kind === 'card') {
      e.preventDefault()
      const el = this.cardEls.get(this.selected.id)
      const card = this.cards.get(this.selected.id)
      if (el && card && !card.path) editable(el.querySelector<HTMLElement>('.card-title')!)
    } else if ((e.key === '=' || e.key === '+') && e.ctrlKey) {
      e.preventDefault()
      this.zoomBy(1.2)
    } else if (e.key === '-' && e.ctrlKey) {
      e.preventDefault()
      this.zoomBy(1 / 1.2)
    } else if (e.key === '0' && e.ctrlKey) {
      e.preventDefault()
      this.zoomTo(1)
    }
  }
}
