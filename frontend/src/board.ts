// Холст со связями (в духе Holst и Freeform). Данные и история — в BoardModel,
// здесь — отображение и управление.
//
// Мышь:
//   двойной щелчок по полю — новая карточка; по карточке — правка текста
//   (у выделенной карточки хватает одного щелчка по тексту);
//   перетаскивание поля — сдвиг, Shift+перетаскивание — рамка выделения;
//   Shift+щелчок — добавить карточку к выделению;
//   кружок справа — потянуть к другой карточке, чтобы связать;
//   уголок справа внизу — размер. Колесо — прокрутка, Ctrl+колесо — масштаб.
// Клавиатура: Ctrl+Z / Ctrl+Shift+Z, Delete, Ctrl+D, Ctrl+A, Enter — правка,
//   стрелки — сдвиг (Shift — крупнее), Alt+стрелки — размер, Esc — отмена.

import * as Go from '../wailsjs/go/main/App'
import {
  BoardModel, CARD_WIDTH, MAX_FONT, MIN_CARD_HEIGHT, MIN_FONT, bodySizeOf, fontSizeOf, stepFont,
  type BoardData, type Card, type Link,
} from './board-model'
import { baseName, kindOf } from './format'
import type { FileItem } from './list'
import { toast } from './toast'

/** То, что холсту нужно знать о заметках. */
export interface NotesBridge {
  get(id: string): { title: string; body: string } | null
  list(): { id: string; title: string; updated: number }[]
  open(id: string): void
}

const COLORS: [string, string][] = [
  ['', 'Без цвета'], ['#0a84ff', 'Синий'], ['#30d158', 'Зелёный'], ['#ffd60a', 'Жёлтый'],
  ['#ff9f0a', 'Оранжевый'], ['#ff453a', 'Красный'], ['#bf5af2', 'Фиолетовый'],
]
// Цвета текста подобраны так, чтобы читаться на тёмной карточке.
const TEXT_COLORS: [string, string][] = [
  ['', 'Обычный'], ['#98989d', 'Серый'], ['#64d2ff', 'Голубой'], ['#409cff', 'Синий'], ['#30d158', 'Зелёный'],
  ['#ffd60a', 'Жёлтый'], ['#ff9f0a', 'Оранжевый'], ['#ff6961', 'Красный'], ['#da8fff', 'Фиолетовый'],
]
const GRID = 24
const SETTLE_MS = 150 // через столько после жеста холст перерисовывается в точном масштабе
const MIN_ZOOM = 0.25
const MAX_ZOOM = 2.5
const DOUBLE_CLICK_MS = 400
const DRAG_THRESHOLD = 3

const SVG = 'http://www.w3.org/2000/svg'
const byId = <T extends Element>(id: string) => document.getElementById(id) as unknown as T

function svgIcon(id: string, className = ''): SVGSVGElement {
  const svg = document.createElementNS(SVG, 'svg')
  if (className) svg.setAttribute('class', className)
  const use = document.createElementNS(SVG, 'use')
  use.setAttribute('href', `#${id}`)
  svg.appendChild(use)
  return svg
}

function button(icon: string, title: string, onClick: () => void, className = 'tool-btn'): HTMLButtonElement {
  const b = document.createElement('button')
  b.type = 'button'
  b.className = className
  b.title = title
  b.setAttribute('aria-label', title)
  b.appendChild(svgIcon(icon))
  b.addEventListener('click', onClick)
  return b
}

function separator(): HTMLElement {
  const s = document.createElement('span')
  s.className = 'board-zoom-sep'
  return s
}

export class Board {
  notes: NotesBridge | null = null

  private model = new BoardModel()
  private cardEls = new Map<string, HTMLElement>()
  private linkEls = new Map<string, SVGGElement>()
  private selection = new Set<string>()
  private selectedLink: string | null = null
  private editing: { id: string; field: HTMLElement } | null = null
  private connectFrom: string | null = null
  private lastDown = { id: '', time: 0, x: 0, y: 0 }
  private loaded = false
  private saveTimer = 0
  private barFrame = 0
  private settleTimer = 0
  private textPopOpen = false
  private sliderCommitted = false

  private viewport = byId<HTMLDivElement>('boardViewport')
  private world = byId<HTMLDivElement>('boardWorld')
  private svg = byId<SVGSVGElement>('boardLinks')
  private hint = byId<HTMLParagraphElement>('boardHint')
  private bar = byId<HTMLDivElement>('boardBar')
  private banner = byId<HTMLDivElement>('boardBanner')
  private zoomValue = byId<HTMLButtonElement>('boardZoomValue')
  private undoButton = byId<HTMLButtonElement>('boardUndo')
  private redoButton = byId<HTMLButtonElement>('boardRedo')
  private notePicker = byId<HTMLDivElement>('boardNotePicker')
  private marquee = document.createElement('div')
  private draft = document.createElementNS(SVG, 'path')

  private sizes = new ResizeObserver((entries) => {
    for (const entry of entries) this.drawLinksOf((entry.target as HTMLElement).dataset.id!)
    this.scheduleBar()
  })

  constructor() {
    this.draft.setAttribute('class', 'link link-draft')
    this.marquee.className = 'board-marquee'

    this.viewport.addEventListener('pointerdown', (e) => this.onPointerDown(e))
    this.viewport.addEventListener('dblclick', (e) => this.onDoubleClick(e))
    this.viewport.addEventListener('wheel', (e) => this.onWheel(e), { passive: false })
    this.viewport.addEventListener('keydown', (e) => this.onKey(e))
    this.viewport.addEventListener('paste', (e) => this.onPaste(e))
    // Кнопки панели не забирают фокус: после перестройки панели клавиши
    // (Tab, Delete, Ctrl+Z) должны по-прежнему доходить до холста.
    this.bar.addEventListener('pointerdown', (e) => {
      if ((e.target as HTMLElement).tagName !== 'INPUT') e.preventDefault() // ползунок должен тянуться
    })

    byId('boardAdd').addEventListener('click', () => this.addCardAtCenter())
    byId('boardAddNote').addEventListener('click', () => this.toggleNotePicker())
    byId('boardZoomIn').addEventListener('click', () => this.zoomBy(1.2))
    byId('boardZoomOut').addEventListener('click', () => this.zoomBy(1 / 1.2))
    byId('boardFit').addEventListener('click', () => this.fit())
    this.undoButton.addEventListener('click', () => this.undo())
    this.redoButton.addEventListener('click', () => this.redo())
    this.zoomValue.addEventListener('click', () => this.zoomTo(1))
    document.addEventListener('pointerdown', (e) => {
      if (!this.notePicker.hidden && !(e.target as Element).closest('#boardNotePicker, #boardAddNote')) this.notePicker.hidden = true
    })
  }

  // --- Загрузка и сохранение ----------------------------------------------

  async open(): Promise<void> {
    if (this.loaded) return
    this.loaded = true
    try {
      const raw = await Go.LoadNotes('board')
      this.model.load(raw ? (JSON.parse(raw) as BoardData) : null)
    } catch (err) {
      toast(`Не удалось загрузить холст: ${err}`)
    }
    this.renderAll()
  }

  focus(): void {
    this.viewport.focus({ preventScroll: true })
  }

  flush(): void {
    if (this.editing) this.finishEdit()
    if (this.saveTimer) {
      clearTimeout(this.saveTimer)
      this.saveTimer = 0
      void this.save()
    }
  }

  /** Esc внутри холста: выйти из правки, режима связи или снять выделение. */
  handleEscape(): boolean {
    if (this.textPopOpen) {
      this.textPopOpen = false
      this.buildBar()
      return true
    }
    if (!this.notePicker.hidden) {
      this.notePicker.hidden = true
      return true
    }
    if (this.editing) {
      this.finishEdit()
      this.focus()
      return true
    }
    if (this.connectFrom) {
      this.setConnectMode(null)
      return true
    }
    if (this.selection.size || this.selectedLink) {
      this.select([])
      return true
    }
    return false
  }

  private changed(): void {
    this.updateHistoryButtons()
    this.scheduleSave()
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
      await Go.SaveNotes('board', JSON.stringify(this.model.toJSON()))
    } catch (err) {
      toast(`Не удалось сохранить холст: ${err}`)
    }
  }

  // --- Отрисовка ------------------------------------------------------------

  private renderAll(): void {
    for (const el of this.cardEls.values()) {
      this.sizes.unobserve(el)
      el.remove()
    }
    for (const g of this.linkEls.values()) g.remove()
    this.cardEls.clear()
    this.linkEls.clear()
    for (const card of this.model.cards.values()) this.mountCard(card)
    for (const link of this.model.links) this.mountLink(link)
    // Выделение — только из того, что осталось после отмены/повтора.
    this.selection = new Set([...this.selection].filter((id) => this.model.cards.has(id)))
    if (this.selectedLink && !this.model.links.some((l) => l.id === this.selectedLink)) this.selectedLink = null
    this.applySelection()
    this.applyView()
    this.hint.hidden = this.model.cards.size > 0
    this.updateHistoryButtons()
  }

  private mountCard(card: Card): HTMLElement {
    const el = document.createElement('div')
    el.dataset.id = card.id

    const title = document.createElement('div')
    title.className = 'card-title'
    title.dataset.placeholder = 'Заголовок'
    const text = document.createElement('div')
    text.className = 'card-text'
    text.dataset.placeholder = 'Текст'
    for (const field of [title, text]) {
      field.addEventListener('input', () => this.onFieldInput(card.id, field))
      field.addEventListener('blur', () => {
        if (this.editing?.field === field) this.finishEdit()
      })
      field.addEventListener('keydown', (e) => this.onFieldKey(e, card.id, field))
    }

    if (card.path) el.appendChild(svgIcon('i-doc', 'card-icon'))
    if (card.noteId) el.appendChild(svgIcon('i-note', 'card-icon card-note-icon'))

    const portIn = document.createElement('span')
    portIn.className = 'card-port port-in'
    const portOut = document.createElement('span')
    portOut.className = 'card-port port-out'
    portOut.title = 'Потяните к другой карточке, чтобы связать'
    const resize = document.createElement('span')
    resize.className = 'card-resize'
    resize.title = 'Потяните, чтобы изменить размер (Alt+стрелки)'
    el.append(title, text, portIn, portOut, resize)

    this.world.appendChild(el)
    this.cardEls.set(card.id, el)
    this.sizes.observe(el)
    this.updateCardEl(card.id)
    return el
  }

  /** Переносит данные карточки в её элемент. Текст не трогаем, пока его правят. */
  private updateCardEl(id: string): void {
    const card = this.model.cards.get(id)
    const el = this.cardEls.get(id)
    if (!card || !el) return
    const kind = card.path ? 'card-file' : card.noteId ? 'card-noteref' : ''
    el.className = ['card', kind, card.fill && card.color ? 'card-filled' : '',
      this.selection.has(id) ? 'selected' : '', this.connectFrom === id ? 'connect-source' : ''].filter(Boolean).join(' ')
    el.style.setProperty('--title-size', `${fontSizeOf(card)}px`)
    el.style.setProperty('--text-size', `${bodySizeOf(card)}px`)
    if (card.textColor) {
      el.style.setProperty('--title-color', card.textColor)
      el.style.setProperty('--body-color', `color-mix(in srgb, ${card.textColor} 78%, transparent)`)
    } else {
      el.style.removeProperty('--title-color')
      el.style.removeProperty('--body-color')
    }
    el.style.transform = `translate(${card.x}px, ${card.y}px)`
    el.style.width = `${card.w}px`
    el.style.minHeight = `${card.h ?? MIN_CARD_HEIGHT}px`
    el.style.setProperty('--card-color', card.color || 'transparent')

    const [title, text] = el.querySelectorAll<HTMLElement>('.card-title, .card-text')
    let titleText = card.title
    let bodyText = card.text
    if (card.path) {
      el.title = `${card.path}\nДвойной щелчок — открыть файл`
      el.querySelector('.card-icon')?.setAttribute('data-cat', kindOf(card.ext ?? '').category)
    } else if (card.noteId) {
      const note = this.notes?.get(card.noteId)
      titleText = note ? note.title : 'Заметка удалена'
      bodyText = note ? note.body : ''
      el.title = 'Заметка. Двойной щелчок — открыть'
    }
    if (this.editing?.field !== title && title.textContent !== titleText) title.textContent = titleText
    if (this.editing?.field !== text && text.textContent !== bodyText) text.textContent = bodyText
  }

  /** Заметки поменялись — обновить карточки-заметки. */
  refreshNoteCards(): void {
    for (const card of this.model.cards.values()) if (card.noteId) this.updateCardEl(card.id)
  }

  private mountLink(link: Link): void {
    const g = document.createElementNS(SVG, 'g')
    g.dataset.id = link.id
    const hit = document.createElementNS(SVG, 'path')
    hit.setAttribute('class', 'link-hit')
    const line = document.createElementNS(SVG, 'path')
    line.setAttribute('class', 'link')
    const end = document.createElementNS(SVG, 'circle')
    end.setAttribute('class', 'link-end')
    end.setAttribute('r', '3.5')
    g.append(hit, line, end)
    this.svg.appendChild(g)
    this.linkEls.set(link.id, g)
    this.drawLink(link)
  }

  private anchor(id: string, side: 'in' | 'out'): { x: number; y: number } | null {
    const card = this.model.cards.get(id)
    const el = this.cardEls.get(id)
    if (!card || !el) return null
    return { x: side === 'out' ? card.x + el.offsetWidth : card.x, y: card.y + el.offsetHeight / 2 }
  }

  private curve(a: { x: number; y: number }, b: { x: number; y: number }): string {
    const dx = Math.max(48, Math.abs(b.x - a.x) / 2)
    return `M ${a.x} ${a.y} C ${a.x + dx} ${a.y}, ${b.x - dx} ${b.y}, ${b.x} ${b.y}`
  }

  /**
   * Точки крепления связи по взаимному расположению карточек: если цель
   * правее или левее — связь идёт сбоку, если ниже или выше — снизу/сверху.
   * Так кривые не делают петель вокруг карточек.
   */
  private linkPath(fromId: string, toId: string): { d: string; end: { x: number; y: number } } | null {
    const a = this.model.cards.get(fromId)
    const b = this.model.cards.get(toId)
    const ae = this.cardEls.get(fromId)
    const be = this.cardEls.get(toId)
    if (!a || !b || !ae || !be) return null
    const A = { l: a.x, r: a.x + ae.offsetWidth, t: a.y, b: a.y + ae.offsetHeight }
    const B = { l: b.x, r: b.x + be.offsetWidth, t: b.y, b: b.y + be.offsetHeight }
    const midY = (r: typeof A) => (r.t + r.b) / 2
    const midX = (r: typeof A) => (r.l + r.r) / 2

    let p1, p2, c1, c2
    if (B.l >= A.r - 16 || B.r <= A.l + 16) {
      const right = B.l >= A.r - 16
      p1 = { x: right ? A.r : A.l, y: midY(A) }
      p2 = { x: right ? B.l : B.r, y: midY(B) }
      const dx = Math.max(40, Math.abs(p2.x - p1.x) / 2) * (right ? 1 : -1)
      c1 = { x: p1.x + dx, y: p1.y }
      c2 = { x: p2.x - dx, y: p2.y }
    } else {
      const down = midY(B) >= midY(A)
      p1 = { x: midX(A), y: down ? A.b : A.t }
      p2 = { x: midX(B), y: down ? B.t : B.b }
      const dy = Math.max(40, Math.abs(p2.y - p1.y) / 2) * (down ? 1 : -1)
      c1 = { x: p1.x, y: p1.y + dy }
      c2 = { x: p2.x, y: p2.y - dy }
    }
    return { d: `M ${p1.x} ${p1.y} C ${c1.x} ${c1.y}, ${c2.x} ${c2.y}, ${p2.x} ${p2.y}`, end: p2 }
  }

  private drawLink(link: Link): void {
    const g = this.linkEls.get(link.id)
    const path = this.linkPath(link.from, link.to)
    if (!g || !path) return
    g.setAttribute('class', ['link-group', link.dashed ? 'dashed' : '', this.selectedLink === link.id ? 'selected' : ''].filter(Boolean).join(' '))
    const { d, end: b } = path
    for (const path of g.querySelectorAll('path')) path.setAttribute('d', d)
    const end = g.querySelector('circle')!
    end.setAttribute('cx', String(b.x))
    end.setAttribute('cy', String(b.y))
  }

  private drawLinksOf(cardId: string): void {
    for (const link of this.model.linksOf(cardId)) this.drawLink(link)
  }

  private sizeOf = (card: Card) => {
    const el = this.cardEls.get(card.id)
    return { w: el?.offsetWidth ?? card.w, h: el?.offsetHeight ?? MIN_CARD_HEIGHT }
  }

  // --- Выделение и панель инструментов ------------------------------------

  private select(ids: Iterable<string>, link: string | null = null): void {
    this.textPopOpen = false
    this.selection = new Set(ids)
    this.selectedLink = link
    this.applySelection()
  }

  private applySelection(): void {
    for (const [id, el] of this.cardEls) el.classList.toggle('selected', this.selection.has(id))
    for (const link of this.model.links) this.drawLink(link)
    // Выделенные карточки — поверх остальных.
    for (const id of this.selection) {
      const el = this.cardEls.get(id)
      if (el) this.world.appendChild(el)
    }
    this.buildBar()
  }

  /** Контекстная панель над выделением: инструменты рядом с объектом. */
  private buildBar(): void {
    this.bar.replaceChildren()
    const cards = [...this.selection].map((id) => this.model.cards.get(id)!).filter(Boolean)
    if (this.selectedLink) {
      const link = this.model.links.find((l) => l.id === this.selectedLink)!
      const dashed = button('i-dash', link.dashed ? 'Сплошная линия' : 'Пунктирная линия', () => this.edit(() => this.model.updateLink(link.id, { dashed: !link.dashed })))
      dashed.setAttribute('aria-pressed', String(!!link.dashed))
      this.bar.append(dashed, separator(), button('i-trash', 'Удалить связь (Delete)', () => this.deleteSelection()))
    } else if (cards.length) {
      const first = cards[0]
      for (const [color, name] of COLORS) {
        const swatch = document.createElement('button')
        swatch.type = 'button'
        swatch.className = 'swatch'
        swatch.title = name
        swatch.style.setProperty('--swatch', color || '#636366')
        swatch.setAttribute('aria-pressed', String(cards.every((c) => c.color === color)))
        swatch.addEventListener('click', () => this.edit(() => cards.forEach((c) => this.model.update(c.id, { color }))))
        this.bar.appendChild(swatch)
      }
      const fill = button('i-fill', 'Залить карточку цветом', () => this.edit(() => cards.forEach((c) => this.model.update(c.id, { fill: !first.fill }))))
      fill.setAttribute('aria-pressed', String(!!first.fill))
      fill.disabled = cards.every((c) => !c.color)
      this.bar.append(separator(), fill)

      const textButton = document.createElement('button')
      textButton.type = 'button'
      textButton.className = 'tool-btn bar-textbtn'
      textButton.title = 'Текст: размер и цвет (Ctrl+Shift+> / Ctrl+Shift+<)'
      textButton.setAttribute('aria-expanded', String(this.textPopOpen))
      textButton.setAttribute('aria-pressed', String(this.textPopOpen))
      const aa = document.createElement('span')
      aa.className = 'bar-aa'
      aa.textContent = 'Aa'
      aa.style.color = first.textColor || ''
      const sizeLabel = document.createElement('span')
      sizeLabel.className = 'bar-size-num'
      sizeLabel.textContent = String(fontSizeOf(first))
      textButton.append(aa, sizeLabel)
      textButton.addEventListener('click', () => {
        this.textPopOpen = !this.textPopOpen
        this.buildBar()
      })
      this.bar.appendChild(textButton)
      if (this.textPopOpen) this.bar.appendChild(this.textPopover(cards))

      this.bar.appendChild(separator())
      if (cards.length === 1) {
        if (first.noteId) this.bar.appendChild(button('i-note', 'Открыть заметку', () => this.notes?.open(first.noteId!), 'tool-btn bar-text'))
        if (first.path) {
          this.bar.appendChild(button('i-open', 'Открыть файл', () => void Go.OpenFile(first.path!).catch((e) => toast(String(e)))))
          this.bar.appendChild(button('i-folder', 'Показать в Проводнике', () => void Go.ShowInExplorer(first.path!).catch((e) => toast(String(e)))))
        }
        const connect = button('i-link', 'Связать с другой карточкой', () => this.setConnectMode(this.connectFrom ? null : first.id))
        connect.setAttribute('aria-pressed', String(this.connectFrom === first.id))
        this.bar.append(connect, button('i-branch', 'Новая связанная карточка (Tab)', () => this.addLinkedCard(first.id)))
      }
      this.bar.append(
        button('i-duplicate', 'Дублировать (Ctrl+D)', () => this.duplicateSelection()),
        button('i-trash', 'Удалить (Delete)', () => this.deleteSelection()),
      )
    }
    this.bar.hidden = !this.bar.childElementCount
    this.positionBar()
  }

  /** Окошко «Текст»: размер (−/+, ползунок) и цвет. */
  private textPopover(cards: Card[]): HTMLElement {
    const ids = cards.map((c) => c.id)
    const size = fontSizeOf(cards[0])
    const pop = document.createElement('div')
    pop.className = 'bar-pop'
    pop.setAttribute('role', 'dialog')
    pop.setAttribute('aria-label', 'Текст карточки')

    const sizeRow = document.createElement('div')
    sizeRow.className = 'pop-row'
    const sizeTitle = document.createElement('span')
    sizeTitle.className = 'pop-label'
    sizeTitle.textContent = 'Размер'
    const value = document.createElement('span')
    value.className = 'pop-value'
    value.textContent = `${size}`
    const minus = button('i-minus', 'Мельче (Ctrl+Shift+<)', () => this.edit(() => this.model.setFontSize(ids, stepFont(fontSizeOf(this.model.cards.get(ids[0])!), -1))))
    const plus = button('i-plus', 'Крупнее (Ctrl+Shift+>)', () => this.edit(() => this.model.setFontSize(ids, stepFont(fontSizeOf(this.model.cards.get(ids[0])!), 1))))
    minus.disabled = size <= MIN_FONT
    plus.disabled = size >= MAX_FONT
    sizeRow.append(sizeTitle, minus, value, plus)

    const slider = document.createElement('input')
    slider.type = 'range'
    slider.className = 'pop-slider'
    slider.min = String(MIN_FONT)
    slider.max = String(MAX_FONT)
    slider.step = '1'
    slider.value = String(size)
    slider.setAttribute('aria-label', 'Размер текста')
    slider.style.setProperty('--fill', `${((size - MIN_FONT) / (MAX_FONT - MIN_FONT)) * 100}%`)
    slider.addEventListener('input', () => {
      // Одна запись в истории на всё движение ползунка.
      if (!this.sliderCommitted) {
        this.model.commit()
        this.sliderCommitted = true
      }
      const v = Number(slider.value)
      this.model.setFontSize(ids, v)
      value.textContent = `${v}`
      slider.style.setProperty('--fill', `${((v - MIN_FONT) / (MAX_FONT - MIN_FONT)) * 100}%`)
      for (const id of ids) this.updateCardEl(id)
    })
    slider.addEventListener('change', () => {
      this.sliderCommitted = false
      this.buildBar()
      this.changed()
    })

    const colorRow = document.createElement('div')
    colorRow.className = 'pop-row pop-colors'
    const colorTitle = document.createElement('span')
    colorTitle.className = 'pop-label'
    colorTitle.textContent = 'Цвет'
    colorRow.appendChild(colorTitle)
    for (const [color, name] of TEXT_COLORS) {
      const swatch = document.createElement('button')
      swatch.type = 'button'
      swatch.className = color ? 'swatch' : 'swatch swatch-auto'
      swatch.title = name
      swatch.style.setProperty('--swatch', color || '#f2f2f4')
      swatch.setAttribute('aria-pressed', String(cards.every((c) => (c.textColor ?? '') === color)))
      swatch.addEventListener('click', () => this.edit(() => ids.forEach((id) => this.model.update(id, { textColor: color || undefined }))))
      colorRow.appendChild(swatch)
    }

    pop.append(sizeRow, slider, colorRow)
    return pop
  }

  private scheduleBar(): void {
    if (!this.barFrame) this.barFrame = requestAnimationFrame(() => { this.barFrame = 0; this.positionBar() })
  }

  /** Ставит панель над выделением, а если сверху нет места — под ним. */
  private positionBar(): void {
    if (this.bar.hidden) return
    let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity
    if (this.selectedLink) {
      const link = this.model.links.find((l) => l.id === this.selectedLink)
      const a = link && this.anchor(link.from, 'out')
      const b = link && this.linkPath(link.from, link.to)?.end
      if (!a || !b) return
      minX = Math.min(a.x, b.x); maxX = Math.max(a.x, b.x)
      minY = Math.min(a.y, b.y); maxY = Math.max(a.y, b.y)
    } else {
      for (const id of this.selection) {
        const card = this.model.cards.get(id)
        if (!card) continue
        const s = this.sizeOf(card)
        minX = Math.min(minX, card.x); minY = Math.min(minY, card.y)
        maxX = Math.max(maxX, card.x + s.w); maxY = Math.max(maxY, card.y + s.h)
      }
    }
    if (minX === Infinity) return
    const { x, y, k } = this.model.view
    const barW = this.bar.offsetWidth
    const barH = this.bar.offsetHeight
    const vw = this.viewport.clientWidth
    const left = Math.min(vw - barW - 8, Math.max(8, x + ((minX + maxX) / 2) * k - barW / 2))
    let top = y + minY * k - barH - 12
    const barAbove = top >= 8
    if (!barAbove) top = y + maxY * k + 12
    top = Math.min(this.viewport.clientHeight - barH - 64, Math.max(8, top))
    this.bar.style.transform = `translate(${Math.round(left)}px, ${Math.round(top)}px)`
    // Окошко «Текст» раскрывается в сторону от карточки, чтобы изменения были видны.
    // Если над панелью нет места — вниз.
    const popH = this.bar.querySelector<HTMLElement>('.bar-pop')?.offsetHeight ?? 150
    this.bar.classList.toggle('pop-above', barAbove ? top > popH + 16 : top > this.viewport.clientHeight - top - barH)
  }

  private updateHistoryButtons(): void {
    this.undoButton.disabled = !this.model.canUndo
    this.redoButton.disabled = !this.model.canRedo
  }

  // --- Изменения ------------------------------------------------------------

  /** Выполняет отменяемое изменение и перерисовывает затронутое. */
  private edit(change: () => void): void {
    this.model.commit()
    change()
    for (const id of this.model.cards.keys()) {
      if (this.cardEls.has(id)) this.updateCardEl(id)
    }
    for (const link of this.model.links) this.drawLink(link)
    this.buildBar()
    this.changed()
  }

  undo(): void {
    if (this.editing) this.finishEdit()
    if (this.model.undo()) {
      this.renderAll()
      this.scheduleSave()
    }
  }

  redo(): void {
    if (this.model.redo()) {
      this.renderAll()
      this.scheduleSave()
    }
  }

  /** Свободное место ближе всего к центру видимой области. */
  private spotAtCenter(h = 90): { x: number; y: number } {
    const c = this.toWorld(this.viewport.clientWidth / 2, this.viewport.clientHeight / 2)
    return this.model.freeSpotNear(Math.round(c.x - CARD_WIDTH / 2), Math.round(c.y - h / 2), CARD_WIDTH, h, this.sizeOf)
  }

  private addCardAtCenter(): void {
    this.createCard(this.spotAtCenter(), true)
  }

  private createCard(partial: Partial<Card> & { x: number; y: number }, edit: boolean): string {
    this.model.commit()
    const card = this.model.addCard({ ...partial, x: Math.round(partial.x), y: Math.round(partial.y) })
    const el = this.mountCard(card)
    el.classList.add('card-enter')
    this.hint.hidden = true
    this.select([card.id])
    this.changed()
    if (edit) this.startEdit(card.id, el.querySelector('.card-title')!)
    return card.id
  }

  private addLinkedCard(fromId: string): void {
    const spot = this.model.placeBeside(fromId, this.sizeOf)
    const id = this.createCard(spot, false)
    // Связь — в ту же запись истории, что и карточка.
    const link = this.model.link(fromId, id)
    if (link) this.mountLink(link)
    this.startEdit(id, this.cardEls.get(id)!.querySelector('.card-title')!)
  }

  /** Добавляет на холст карточку найденного файла. */
  async addFile(item: FileItem): Promise<void> {
    await this.open()
    this.createCard({
      ...this.spotAtCenter(70),
      // Полный путь — во всплывающей подсказке, на карточке только папка.
      title: item.name, text: baseName(item.dir), path: item.path, ext: item.ext,
    }, false)
  }

  /** Добавляет на холст заметку (или выделяет уже добавленную). */
  async addNote(noteId: string): Promise<void> {
    await this.open()
    const existing = [...this.model.cards.values()].find((c) => c.noteId === noteId)
    if (existing) {
      this.select([existing.id])
      this.centerOn(existing.id)
      return
    }
    this.createCard({ ...this.spotAtCenter(110), noteId, color: '#ffd60a' }, false)
  }

  private duplicateSelection(): void {
    if (!this.selection.size) return
    let copies: string[] = []
    this.model.commit()
    copies = this.model.duplicate(this.selection)
    this.renderAll()
    this.select(copies)
    this.changed()
  }

  private deleteSelection(): void {
    if (this.selectedLink) {
      this.edit(() => this.model.unlink(this.selectedLink!))
      this.select([])
      return
    }
    if (!this.selection.size) return
    const count = this.selection.size
    this.model.commit()
    this.model.remove(this.selection)
    this.selection.clear()
    this.renderAll()
    this.changed()
    toast(count > 1 ? `Удалено карточек: ${count}` : 'Карточка удалена', 'Отменить', () => this.undo())
  }

  // --- Правка текста --------------------------------------------------------

  /** Включает правку поля и ставит курсор туда, куда щёлкнули (или в конец). */
  private startEdit(id: string, field: HTMLElement, point?: { x: number; y: number }): void {
    const card = this.model.cards.get(id)
    if (!card) return
    if (card.path) {
      void Go.OpenFile(card.path).catch((e) => toast(String(e)))
      return
    }
    if (card.noteId) {
      this.notes?.open(card.noteId)
      return
    }
    if (this.editing) this.finishEdit()
    this.select([id])
    this.model.commit()
    this.editing = { id, field }
    field.contentEditable = 'plaintext-only'
    field.focus({ preventScroll: true })

    const range = (point && document.caretRangeFromPoint?.(point.x, point.y)) || null
    const sel = getSelection()
    if (range && field.contains(range.startContainer)) {
      sel?.removeAllRanges()
      sel?.addRange(range)
    } else {
      const end = document.createRange()
      end.selectNodeContents(field)
      end.collapse(false)
      sel?.removeAllRanges()
      sel?.addRange(end)
    }
    this.bar.hidden = true
  }

  private finishEdit(): void {
    const editing = this.editing
    if (!editing) return
    this.editing = null
    editing.field.removeAttribute('contenteditable')
    this.model.discardCommitIfUnchanged()
    this.updateCardEl(editing.id)
    this.buildBar()
    this.changed()
  }

  private onFieldInput(id: string, field: HTMLElement): void {
    const isTitle = field.classList.contains('card-title')
    const value = isTitle ? field.innerText.replace(/\n/g, ' ') : field.innerText.replace(/\n$/, '')
    this.model.update(id, isTitle ? { title: value } : { text: value })
    this.scheduleSave()
  }

  private onFieldKey(e: KeyboardEvent, id: string, field: HTMLElement): void {
    e.stopPropagation() // клавиши правки не должны доходить до холста
    if (e.key === 'Escape' || (e.key === 'Enter' && e.ctrlKey)) {
      e.preventDefault()
      this.finishEdit()
      this.focus()
    } else if (field.classList.contains('card-title') && (e.key === 'Enter' || e.key === 'Tab')) {
      e.preventDefault()
      const text = this.cardEls.get(id)?.querySelector<HTMLElement>('.card-text')
      if (text) {
        this.finishEdit()
        this.startEdit(id, text)
      }
    }
  }

  // --- Связывание кнопкой (без перетаскивания) -------------------------------

  private setConnectMode(fromId: string | null): void {
    const prev = this.connectFrom
    this.connectFrom = fromId
    if (prev) this.updateCardEl(prev)
    if (fromId) this.updateCardEl(fromId)
    this.viewport.classList.toggle('connecting', !!fromId)
    this.banner.hidden = !fromId
    this.banner.textContent = fromId ? 'Щёлкните карточку, с которой нужно связать. Esc — отмена' : ''
    this.buildBar()
  }

  private finishConnect(toId: string): void {
    const from = this.connectFrom!
    this.setConnectMode(null)
    if (from === toId) return
    if (this.model.hasLink(from, toId)) {
      toast('Эти карточки уже связаны')
      return
    }
    this.model.commit()
    const link = this.model.link(from, toId)
    if (link) this.mountLink(link)
    this.select([toId])
    this.changed()
  }

  // --- Вид: сдвиг и масштаб ----------------------------------------------------

  /**
   * Во время жеста слой холста двигается как готовая картинка (will-change) —
   * это плавно. Через SETTLE_MS после остановки will-change снимается, и браузер
   * перерисовывает текст и рамки в точном масштабе — без размытия при приближении.
   * Сдвиг округляется до экранного пикселя: дробные пиксели тоже размывают текст.
   */
  private applyView(): void {
    const { k } = this.model.view
    const dpr = window.devicePixelRatio || 1
    const x = Math.round(this.model.view.x * dpr) / dpr
    const y = Math.round(this.model.view.y * dpr) / dpr
    this.world.classList.add('moving')
    clearTimeout(this.settleTimer)
    this.settleTimer = window.setTimeout(() => this.world.classList.remove('moving'), SETTLE_MS)
    this.world.style.transform = `translate(${x}px, ${y}px) scale(${k})`
    const size = GRID * k
    this.viewport.style.backgroundSize = `${size}px ${size}px`
    this.viewport.style.backgroundPosition = `${x}px ${y}px`
    this.zoomValue.textContent = `${Math.round(k * 100)}%`
    this.scheduleBar()
  }

  private toWorld(px: number, py: number): { x: number; y: number } {
    const { x, y, k } = this.model.view
    return { x: (px - x) / k, y: (py - y) / k }
  }

  private local(e: { clientX: number; clientY: number }): { x: number; y: number } {
    const rect = this.viewport.getBoundingClientRect()
    return { x: e.clientX - rect.left, y: e.clientY - rect.top }
  }

  private zoomAt(px: number, py: number, k: number): void {
    k = Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, k))
    const w = this.toWorld(px, py)
    this.model.view = { k, x: px - w.x * k, y: py - w.y * k }
    this.applyView()
    this.scheduleSave()
  }

  private zoomBy(f: number): void { this.zoomAt(this.viewport.clientWidth / 2, this.viewport.clientHeight / 2, this.model.view.k * f) }
  private zoomTo(k: number): void { this.zoomAt(this.viewport.clientWidth / 2, this.viewport.clientHeight / 2, k) }

  private centerOn(id: string): void {
    const card = this.model.cards.get(id)
    if (!card) return
    const s = this.sizeOf(card)
    const { k } = this.model.view
    this.model.view = { k, x: this.viewport.clientWidth / 2 - (card.x + s.w / 2) * k, y: this.viewport.clientHeight / 2 - (card.y + s.h / 2) * k }
    this.applyView()
  }

  private fit(): void {
    if (!this.model.cards.size) return this.zoomTo(1)
    let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity
    for (const card of this.model.cards.values()) {
      const s = this.sizeOf(card)
      minX = Math.min(minX, card.x); minY = Math.min(minY, card.y)
      maxX = Math.max(maxX, card.x + s.w); maxY = Math.max(maxY, card.y + s.h)
    }
    const pad = 80
    const vw = this.viewport.clientWidth
    const vh = this.viewport.clientHeight
    const k = Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, Math.min((vw - pad * 2) / (maxX - minX), (vh - pad * 2) / (maxY - minY), 1.2)))
    this.model.view = { k, x: (vw - (maxX - minX) * k) / 2 - minX * k, y: (vh - (maxY - minY) * k) / 2 - minY * k }
    this.applyView()
    this.scheduleSave()
  }

  private onWheel(e: WheelEvent): void {
    if ((e.target as Element).closest('[contenteditable]')) return // прокрутка длинного текста в карточке
    e.preventDefault()
    const p = this.local(e)
    if (e.ctrlKey) {
      this.zoomAt(p.x, p.y, this.model.view.k * Math.exp(-e.deltaY * 0.0025))
    } else {
      this.model.view.x -= e.shiftKey ? e.deltaY : e.deltaX
      this.model.view.y -= e.shiftKey ? 0 : e.deltaY
      this.applyView()
      this.scheduleSave()
    }
  }

  // --- Мышь ----------------------------------------------------------------------

  private onPointerDown(e: PointerEvent): void {
    const target = e.target as Element
    if (target.closest('#boardBar')) return
    if (this.editing?.field.contains(target)) return // внутри правки — обычное выделение текста

    const cardEl = target.closest<HTMLElement>('.card')
    const linkEl = target.closest<SVGGElement>('.link-group')

    if (e.button === 1 || (e.button === 0 && !cardEl && !linkEl)) {
      if (this.editing) this.finishEdit()
      this.focus()
      if (e.button === 0 && e.shiftKey) return this.marqueeSelect(e)
      if (this.connectFrom) this.setConnectMode(null)
      else if (e.button === 0) this.select([])
      return this.pan(e)
    }
    if (e.button !== 0) return
    e.preventDefault()
    if (this.editing) this.finishEdit()
    this.focus()

    if (linkEl) {
      this.select([], linkEl.dataset.id!)
      return
    }
    const id = cardEl!.dataset.id!
    if (this.connectFrom) return this.finishConnect(id)
    if (target.classList.contains('port-out')) return this.dragConnect(id, e)
    if (target.classList.contains('card-resize')) return this.dragResize(id, e)

    // Двойной щелчок распознаём сами: родной dblclick при захвате указателя
    // приходит полю, а не карточке.
    const now = performance.now()
    const isDouble = this.lastDown.id === id && now - this.lastDown.time < DOUBLE_CLICK_MS &&
      Math.hypot(e.clientX - this.lastDown.x, e.clientY - this.lastDown.y) < 6
    this.lastDown = { id, time: isDouble ? 0 : now, x: e.clientX, y: e.clientY }
    if (isDouble) return this.startEdit(id, this.fieldAt(cardEl!, e), { x: e.clientX, y: e.clientY })

    if (e.shiftKey) {
      const next = new Set(this.selection)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      this.select(next)
      return
    }
    const wasOnlySelected = this.selection.size === 1 && this.selection.has(id)
    if (!this.selection.has(id)) this.select([id])
    this.dragCards(e, () => {
      // Щелчок без сдвига по тексту уже выделенной карточки — сразу правка (как во Freeform).
      if (wasOnlySelected && target.closest('.card-title, .card-text')) {
        this.startEdit(id, this.fieldAt(cardEl!, e), { x: e.clientX, y: e.clientY })
      }
    })
  }

  private fieldAt(cardEl: HTMLElement, e: { clientX: number; clientY: number }): HTMLElement {
    const under = document.elementFromPoint(e.clientX, e.clientY)?.closest<HTMLElement>('.card-title, .card-text')
    return under && cardEl.contains(under) ? under : cardEl.querySelector<HTMLElement>('.card-title')!
  }

  /** Цикл перетаскивания: onMove получает смещение от начала; onClick — если сдвига не было. */
  private track(
    start: PointerEvent,
    onMove: (dx: number, dy: number, e: PointerEvent) => void,
    onEnd: (moved: boolean) => void,
    bodyClass: string,
  ): void {
    const el = this.viewport
    try { el.setPointerCapture(start.pointerId) } catch { /* синтетические события в тестах */ }
    let moved = false
    let frame = 0
    let last = start
    const move = (e: PointerEvent) => {
      last = e
      if (!moved && Math.hypot(e.clientX - start.clientX, e.clientY - start.clientY) < DRAG_THRESHOLD) return
      if (!moved) {
        moved = true
        document.body.classList.add(bodyClass)
      }
      if (!frame) frame = requestAnimationFrame(() => {
        frame = 0
        onMove(last.clientX - start.clientX, last.clientY - start.clientY, last)
      })
    }
    const up = () => {
      cancelAnimationFrame(frame)
      if (moved) onMove(last.clientX - start.clientX, last.clientY - start.clientY, last)
      el.removeEventListener('pointermove', move)
      el.removeEventListener('pointerup', up)
      el.removeEventListener('pointercancel', up)
      document.body.classList.remove(bodyClass)
      onEnd(moved)
    }
    el.addEventListener('pointermove', move)
    el.addEventListener('pointerup', up)
    el.addEventListener('pointercancel', up)
  }

  private pan(e: PointerEvent): void {
    const start = { ...this.model.view }
    this.track(e, (dx, dy) => {
      this.model.view.x = start.x + dx
      this.model.view.y = start.y + dy
      this.applyView()
    }, (moved) => moved && this.scheduleSave(), 'panning')
  }

  private dragCards(e: PointerEvent, onClick: () => void): void {
    const ids = [...this.selection]
    const starts = new Map(ids.map((id) => {
      const c = this.model.cards.get(id)!
      return [id, { x: c.x, y: c.y }]
    }))
    let committed = false
    this.track(e, (dx, dy) => {
      if (!committed) {
        this.model.commit()
        committed = true
        this.bar.hidden = true
      }
      const k = this.model.view.k
      for (const id of ids) {
        const s = starts.get(id)!
        this.model.update(id, { x: s.x + dx / k, y: s.y + dy / k })
        this.updateCardEl(id)
        this.drawLinksOf(id)
      }
    }, (moved) => {
      if (!moved) return onClick()
      for (const id of ids) {
        const c = this.model.cards.get(id)!
        this.model.update(id, { x: Math.round(c.x / 4) * 4, y: Math.round(c.y / 4) * 4 })
        this.updateCardEl(id)
        this.drawLinksOf(id)
      }
      this.buildBar()
      this.changed()
    }, 'dragging')
  }

  private dragResize(id: string, e: PointerEvent): void {
    const card = this.model.cards.get(id)!
    const el = this.cardEls.get(id)!
    const startW = card.w
    const startH = el.offsetHeight
    let committed = false
    this.track(e, (dx, dy) => {
      if (!committed) {
        this.model.commit()
        committed = true
        this.bar.hidden = true
      }
      const k = this.model.view.k
      this.model.resize(id, startW + dx / k, startH + dy / k)
      this.updateCardEl(id)
      this.drawLinksOf(id)
    }, (moved) => {
      if (moved) {
        this.buildBar()
        this.changed()
      }
    }, 'resizing-card')
  }

  private dragConnect(fromId: string, e: PointerEvent): void {
    const a = this.anchor(fromId, 'out')
    if (!a) return
    this.svg.appendChild(this.draft)
    let target: HTMLElement | null = null
    const update = (clientX: number, clientY: number) => {
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
    this.track(e, (_dx, _dy, ev) => update(ev.clientX, ev.clientY), () => {
      this.draft.remove()
      target?.classList.remove('drop-target')
      const toId = target?.dataset.id
      if (!toId) return
      if (this.model.hasLink(fromId, toId)) return
      this.model.commit()
      const link = this.model.link(fromId, toId)
      if (link) this.mountLink(link)
      this.changed()
    }, 'connecting')
  }

  private marqueeSelect(e: PointerEvent): void {
    const start = this.local(e)
    const base = new Set(this.selection)
    this.viewport.appendChild(this.marquee)
    this.track(e, (_dx, _dy, ev) => {
      const p = this.local(ev)
      const left = Math.min(start.x, p.x), top = Math.min(start.y, p.y)
      const w = Math.abs(p.x - start.x), h = Math.abs(p.y - start.y)
      Object.assign(this.marquee.style, { transform: `translate(${left}px, ${top}px)`, width: `${w}px`, height: `${h}px` })
      const a = this.toWorld(left, top)
      const k = this.model.view.k
      this.select([...base, ...this.model.cardsInRect({ x: a.x, y: a.y, w: w / k, h: h / k }, this.sizeOf)])
    }, () => this.marquee.remove(), 'selecting')
  }

  private onDoubleClick(e: MouseEvent): void {
    // Цель берём по координатам: при захвате указателя e.target — само поле.
    const under = document.elementFromPoint(e.clientX, e.clientY)
    if (!under || under.closest('.card, .link-group, #boardBar, .board-zoom, #boardNotePicker')) return
    const p = this.local(e)
    const w = this.toWorld(p.x, p.y)
    this.createCard({ x: w.x - CARD_WIDTH / 2, y: w.y - 30 }, true)
  }

  // --- Клавиатура --------------------------------------------------------------

  private onKey(e: KeyboardEvent): void {
    if (this.editing || (e.target as HTMLElement).tagName === 'INPUT') return // стрелки ползунка — его собственные
    const ctrl = e.ctrlKey || e.metaKey
    const code = e.code
    const ids = [...this.selection]

    if (ctrl && code === 'KeyZ') {
      e.preventDefault()
      if (e.shiftKey) this.redo()
      else this.undo()
    } else if (ctrl && code === 'KeyY') {
      e.preventDefault()
      this.redo()
    } else if (ctrl && code === 'KeyA') {
      e.preventDefault()
      this.select(this.model.cards.keys())
    } else if (ctrl && e.shiftKey && (code === 'Period' || code === 'Comma') && ids.length) {
      e.preventDefault()
      const dir = code === 'Period' ? 1 : -1
      this.edit(() => this.model.setFontSize(ids, stepFont(fontSizeOf(this.model.cards.get(ids[0])!), dir)))
    } else if (ctrl && code === 'KeyD') {
      e.preventDefault()
      this.duplicateSelection()
    } else if (e.key === 'Delete' || e.key === 'Backspace') {
      e.preventDefault()
      this.deleteSelection()
    } else if (e.key === 'Enter' && ids.length === 1) {
      e.preventDefault()
      this.startEdit(ids[0], this.cardEls.get(ids[0])!.querySelector('.card-title')!)
    } else if (e.key === 'Tab' && ids.length === 1) {
      e.preventDefault()
      this.addLinkedCard(ids[0])
    } else if (e.key.startsWith('Arrow') && ids.length) {
      e.preventDefault()
      const step = e.shiftKey ? 24 : 4
      const dx = e.key === 'ArrowLeft' ? -step : e.key === 'ArrowRight' ? step : 0
      const dy = e.key === 'ArrowUp' ? -step : e.key === 'ArrowDown' ? step : 0
      if (e.altKey && ids.length === 1) {
        // Alt+стрелки — размер: альтернатива перетаскиванию уголка.
        const card = this.model.cards.get(ids[0])!
        const h = this.cardEls.get(ids[0])!.offsetHeight
        this.edit(() => this.model.resize(ids[0], card.w + dx * 5, h + dy * 5))
      } else {
        this.edit(() => this.model.moveBy(ids, dx, dy))
      }
    } else if (ctrl && (e.key === '=' || e.key === '+')) {
      e.preventDefault()
      this.zoomBy(1.2)
    } else if (ctrl && e.key === '-') {
      e.preventDefault()
      this.zoomBy(1 / 1.2)
    } else if (ctrl && e.key === '0') {
      e.preventDefault()
      this.zoomTo(1)
    }
  }

  /** Вставка текста из буфера на пустое поле — новая карточка с этим текстом. */
  private onPaste(e: ClipboardEvent): void {
    if (this.editing) return
    const text = e.clipboardData?.getData('text/plain')?.trim()
    if (!text) return
    e.preventDefault()
    const [first, ...rest] = text.split('\n')
    this.createCard({ ...this.spotAtCenter(), title: first.slice(0, 200), text: rest.join('\n').trim() }, false)
  }

  // --- Вставка заметки ------------------------------------------------------------

  private toggleNotePicker(): void {
    if (!this.notePicker.hidden) {
      this.notePicker.hidden = true
      return
    }
    const notes = this.notes?.list() ?? []
    this.notePicker.replaceChildren()
    const head = document.createElement('p')
    head.className = 'picker-head'
    head.textContent = notes.length ? 'Добавить заметку на холст' : 'Заметок пока нет — создайте их во вкладке «Заметки»'
    this.notePicker.appendChild(head)
    for (const note of notes.slice(0, 50)) {
      const b = document.createElement('button')
      b.type = 'button'
      b.className = 'picker-item'
      b.appendChild(svgIcon('i-note'))
      const span = document.createElement('span')
      span.textContent = note.title
      b.appendChild(span)
      b.addEventListener('click', () => {
        this.notePicker.hidden = true
        void this.addNote(note.id)
      })
      this.notePicker.appendChild(b)
    }
    this.notePicker.hidden = false
  }
}
