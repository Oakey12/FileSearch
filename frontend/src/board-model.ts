// Данные холста и операции над ними — без DOM, чтобы их можно было проверять тестами.
// Каждое изменение, которое пользователь может отменить, идёт через commit().

export type TextSize = 's' | 'm' | 'l'

export interface Card {
  id: string
  x: number
  y: number
  w: number
  h?: number // минимальная высота, если пользователь растянул карточку
  title: string
  text: string
  color: string
  fill?: boolean // залить карточку цветом, а не только полосой сверху
  size?: TextSize // старый формат размера (до 2.3) — читается через fontSizeOf
  fontSize?: number // размер заголовка в пикселях; текст — чуть мельче
  textColor?: string // '' — обычный цвет
  path?: string // карточка-файл
  ext?: string
  noteId?: string // карточка-заметка
}

export interface Link {
  id: string
  from: string
  to: string
  dashed?: boolean
}

export interface View { x: number; y: number; k: number }

export interface BoardData {
  version: 1
  cards: Card[]
  links: Link[]
  view: View
}

export const CARD_WIDTH = 220
export const MIN_CARD_WIDTH = 140
export const MAX_CARD_WIDTH = 720
export const MIN_CARD_HEIGHT = 64
const HISTORY_LIMIT = 100

export const MIN_FONT = 10
export const MAX_FONT = 72
/** Ступени для кнопок «−» и «+» и Ctrl+Shift+</>. */
export const FONT_STEPS = [10, 12, 14, 16, 18, 20, 24, 28, 32, 40, 48, 56, 64, 72]
const LEGACY_FONT: Record<TextSize, number> = { s: 12, m: 14, l: 19 }

/** Размер заголовка карточки с учётом старого формата (s/m/l). */
export function fontSizeOf(card: Card): number {
  return card.fontSize ?? LEGACY_FONT[card.size ?? 'm']
}

/** Размер основного текста — чуть мельче заголовка. */
export function bodySizeOf(card: Card): number {
  return Math.max(MIN_FONT, Math.round(fontSizeOf(card) * 0.86 * 2) / 2)
}

/** Следующая ступень размера вверх (dir = 1) или вниз (dir = -1). */
export function stepFont(size: number, dir: 1 | -1): number {
  if (dir > 0) return FONT_STEPS.find((s) => s > size) ?? MAX_FONT
  return [...FONT_STEPS].reverse().find((s) => s < size) ?? MIN_FONT
}

export const newId = () => Date.now().toString(36) + Math.random().toString(36).slice(2, 8)

interface Snapshot { cards: Card[]; links: Link[] }

export class BoardModel {
  cards = new Map<string, Card>()
  links: Link[] = []
  view: View = { x: 80, y: 60, k: 1 }

  private undoStack: string[] = []
  private redoStack: string[] = []

  // --- Загрузка и сохранение ----------------------------------------------

  load(data: Partial<BoardData> | null): void {
    this.cards.clear()
    for (const card of data?.cards ?? []) this.cards.set(card.id, { ...card })
    // Связи на несуществующие карточки (например, после ручной правки файла) отбрасываем.
    this.links = (data?.links ?? []).filter((l) => this.cards.has(l.from) && this.cards.has(l.to) && l.from !== l.to)
    if (data?.view) this.view = { ...data.view }
    this.undoStack = []
    this.redoStack = []
  }

  toJSON(): BoardData {
    return { version: 1, cards: [...this.cards.values()], links: this.links, view: this.view }
  }

  // --- История --------------------------------------------------------------

  /** Запоминает состояние до изменения. Вызывать перед каждой отменяемой правкой. */
  commit(): void {
    this.undoStack.push(this.snapshot())
    if (this.undoStack.length > HISTORY_LIMIT) this.undoStack.shift()
    this.redoStack = []
  }

  /** Убирает последнюю запись истории, если изменение так и не случилось
   *  (начали править текст и ничего не поменяли). */
  discardCommitIfUnchanged(): void {
    if (this.undoStack.at(-1) === this.snapshot()) this.undoStack.pop()
  }

  get canUndo(): boolean { return this.undoStack.length > 0 }
  get canRedo(): boolean { return this.redoStack.length > 0 }

  undo(): boolean {
    const prev = this.undoStack.pop()
    if (prev === undefined) return false
    this.redoStack.push(this.snapshot())
    this.restore(prev)
    return true
  }

  redo(): boolean {
    const next = this.redoStack.pop()
    if (next === undefined) return false
    this.undoStack.push(this.snapshot())
    this.restore(next)
    return true
  }

  private snapshot(): string {
    const s: Snapshot = { cards: [...this.cards.values()], links: this.links }
    return JSON.stringify(s)
  }

  private restore(json: string): void {
    const s = JSON.parse(json) as Snapshot
    this.cards = new Map(s.cards.map((c) => [c.id, c]))
    this.links = s.links
  }

  // --- Карточки ---------------------------------------------------------------

  addCard(partial: Partial<Card> & { x: number; y: number }): Card {
    const card: Card = { id: newId(), w: CARD_WIDTH, title: '', text: '', color: '', ...partial }
    this.cards.set(card.id, card)
    return card
  }

  update(id: string, patch: Partial<Card>): void {
    const card = this.cards.get(id)
    if (card) Object.assign(card, patch)
  }

  /** Размер текста в пикселях, в пределах MIN_FONT…MAX_FONT. */
  setFontSize(ids: Iterable<string>, size: number): void {
    const value = Math.round(Math.min(MAX_FONT, Math.max(MIN_FONT, size)))
    for (const id of ids) {
      const card = this.cards.get(id)
      if (card) {
        card.fontSize = value
        delete card.size
      }
    }
  }

  moveBy(ids: Iterable<string>, dx: number, dy: number): void {
    for (const id of ids) {
      const card = this.cards.get(id)
      if (card) {
        card.x += dx
        card.y += dy
      }
    }
  }

  resize(id: string, w: number, h?: number): void {
    const card = this.cards.get(id)
    if (!card) return
    card.w = Math.round(Math.min(MAX_CARD_WIDTH, Math.max(MIN_CARD_WIDTH, w)))
    if (h !== undefined) card.h = Math.round(Math.max(MIN_CARD_HEIGHT, h))
  }

  /** Удаляет карточки вместе со связями. */
  remove(ids: Iterable<string>): void {
    const gone = new Set(ids)
    for (const id of gone) this.cards.delete(id)
    this.links = this.links.filter((l) => !gone.has(l.from) && !gone.has(l.to))
  }

  /** Копирует карточки со сдвигом и связи между ними. Возвращает новые id. */
  duplicate(ids: Iterable<string>, offset = 24): string[] {
    const map = new Map<string, string>()
    for (const id of ids) {
      const card = this.cards.get(id)
      if (!card) continue
      const copy = { ...card, id: newId(), x: card.x + offset, y: card.y + offset }
      this.cards.set(copy.id, copy)
      map.set(id, copy.id)
    }
    for (const link of [...this.links]) {
      const from = map.get(link.from)
      const to = map.get(link.to)
      if (from && to) this.links.push({ ...link, id: newId(), from, to })
    }
    return [...map.values()]
  }

  // --- Связи ------------------------------------------------------------------

  hasLink(a: string, b: string): boolean {
    return this.links.some((l) => (l.from === a && l.to === b) || (l.from === b && l.to === a))
  }

  /** Связывает две карточки. null — если связь уже есть или карточки те же. */
  link(from: string, to: string): Link | null {
    if (from === to || !this.cards.has(from) || !this.cards.has(to) || this.hasLink(from, to)) return null
    const link: Link = { id: newId(), from, to }
    this.links.push(link)
    return link
  }

  updateLink(id: string, patch: Partial<Link>): void {
    const link = this.links.find((l) => l.id === id)
    if (link) Object.assign(link, patch)
  }

  unlink(id: string): void {
    this.links = this.links.filter((l) => l.id !== id)
  }

  linksOf(cardId: string): Link[] {
    return this.links.filter((l) => l.from === cardId || l.to === cardId)
  }

  // --- Геометрия -----------------------------------------------------------------

  /** Карточки, попавшие в прямоугольник (в координатах холста). size — размеры карточки. */
  cardsInRect(
    rect: { x: number; y: number; w: number; h: number },
    size: (card: Card) => { w: number; h: number },
  ): string[] {
    const right = rect.x + rect.w
    const bottom = rect.y + rect.h
    const out: string[] = []
    for (const card of this.cards.values()) {
      const s = size(card)
      if (card.x < right && card.x + s.w > rect.x && card.y < bottom && card.y + s.h > rect.y) out.push(card.id)
    }
    return out
  }

  /**
   * Ближайшее к точке место, где карточка w×h не накроет другие.
   * Ищет по расширяющимся кольцам — новая карточка остаётся рядом с центром.
   */
  freeSpotNear(x: number, y: number, w: number, h: number, size: (card: Card) => { w: number; h: number }): { x: number; y: number } {
    const gap = 16
    const free = (px: number, py: number) => this.cardsInRect({ x: px - gap, y: py - gap, w: w + gap * 2, h: h + gap * 2 }, size).length === 0
    if (free(x, y)) return { x, y }
    const step = 40
    for (let ring = 1; ring <= 30; ring++) {
      for (let i = -ring; i <= ring; i++) {
        for (const [dx, dy] of [[i, -ring], [i, ring], [-ring, i], [ring, i]]) {
          const px = x + dx * step
          const py = y + dy * step
          if (free(px, py)) return { x: px, y: py }
        }
      }
    }
    return { x, y }
  }

  /** Место справа от карточки для новой связанной карточки, не поверх соседей. */
  placeBeside(id: string, size: (card: Card) => { w: number; h: number }): { x: number; y: number } {
    const card = this.cards.get(id)!
    const s = size(card)
    const x = card.x + s.w + 80
    let y = card.y
    const taken = (yy: number) => [...this.cards.values()].some((c) => {
      const cs = size(c)
      return c.id !== id && c.x < x + CARD_WIDTH && c.x + cs.w > x && c.y < yy + MIN_CARD_HEIGHT && c.y + cs.h > yy
    })
    for (let i = 0; i < 20 && taken(y); i++) y += MIN_CARD_HEIGHT + 24
    return { x, y }
  }
}
