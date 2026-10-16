// Шторка сверху: «Заметки» и «Холст». Открывается кнопкой, Ctrl+J или
// если потянуть язычок у верхнего края окна; закрывается Esc, щелчком мимо
// или если потянуть ручку внизу шторки вверх.

import { Board } from './board'
import type { FileItem } from './list'
import { Notes } from './notes'
import { toast } from './toast'

type Pane = 'notes' | 'board'

const byId = <T extends HTMLElement>(id: string) => document.getElementById(id) as T

export class Drawer {
  readonly notes = new Notes()
  readonly board = new Board()

  private el = byId<HTMLElement>('drawer')
  private scrim = byId<HTMLDivElement>('drawerScrim')
  private tab = byId<HTMLButtonElement>('drawerTab')
  private grip = byId<HTMLDivElement>('drawerGrip')
  private notesPane = byId<HTMLDivElement>('notesPane')
  private boardPane = byId<HTMLDivElement>('boardPane')
  private pane: Pane = 'notes'
  private returnFocus: HTMLElement | null = null
  isOpen = false

  constructor() {
    byId('openNotes').addEventListener('click', () => this.toggle())
    byId('drawerClose').addEventListener('click', () => this.close())
    this.scrim.addEventListener('click', () => this.close())
    this.el.querySelectorAll<HTMLButtonElement>('[data-pane]').forEach((b) => {
      b.addEventListener('click', () => this.show(b.dataset.pane as Pane))
    })
    this.pullGesture(this.tab, 1)
    this.pullGesture(this.grip, -1)
    this.el.addEventListener('keydown', (e) => {
      if (e.key !== 'Escape') return
      e.stopPropagation()
      if (this.pane === 'board' && this.board.handleEscape()) return
      this.close()
    })
    window.addEventListener('beforeunload', () => {
      this.notes.flush()
      this.board.flush()
    })
    try {
      if (localStorage.getItem('drawerPane') === 'board') this.pane = 'board'
    } catch { /* без хранилища — по умолчанию заметки */ }
  }

  toggle(): void {
    if (this.isOpen) this.close()
    else this.open()
  }

  open(pane?: Pane): void {
    if (pane) this.pane = pane
    if (!this.isOpen) {
      this.returnFocus = document.activeElement as HTMLElement | null
      this.isOpen = true
      this.el.inert = false
      this.el.setAttribute('aria-hidden', 'false')
      this.scrim.hidden = false
      document.body.classList.add('drawer-open')
      this.el.style.transform = ''
    }
    this.show(this.pane)
  }

  close(): void {
    if (!this.isOpen) return
    this.isOpen = false
    this.notes.flush()
    this.board.flush()
    document.body.classList.remove('drawer-open')
    this.el.style.transform = ''
    this.el.setAttribute('aria-hidden', 'true')
    this.el.inert = true
    // Подложку убираем после анимации, чтобы она плавно погасла.
    window.setTimeout(() => { if (!this.isOpen) this.scrim.hidden = true }, 320)
    this.returnFocus?.focus?.()
  }

  /** Добавляет файл на холст и предлагает его открыть. */
  async addToBoard(item: FileItem): Promise<void> {
    await this.board.addFile(item)
    if (this.isOpen && this.pane === 'board') return
    toast('Файл добавлен на холст', 'Открыть холст', () => this.open('board'))
  }

  private show(pane: Pane): void {
    this.pane = pane
    try { localStorage.setItem('drawerPane', pane) } catch { /* не страшно */ }
    this.el.querySelectorAll<HTMLButtonElement>('[data-pane]').forEach((b) => {
      b.setAttribute('aria-selected', String(b.dataset.pane === pane))
    })
    this.notesPane.hidden = pane !== 'notes'
    this.boardPane.hidden = pane !== 'board'
    if (!this.isOpen) return
    if (pane === 'notes') void this.notes.open().then(() => this.notes.focusEditor())
    else void this.board.open().then(() => this.board.focus())
  }

  /**
   * Шторка следует за указателем, пока её тянут. direction: 1 — тянут вниз
   * (открыть), -1 — тянут вверх (закрыть). Короткий щелчок без движения
   * по язычку просто открывает шторку.
   */
  private pullGesture(handle: HTMLElement, direction: 1 | -1): void {
    handle.addEventListener('pointerdown', (start) => {
      if (start.button !== 0) return
      start.preventDefault()
      handle.setPointerCapture(start.pointerId)
      const height = this.el.offsetHeight
      let offset = 0
      let moved = false

      if (direction === 1 && !this.isOpen) {
        // Готовим шторку к выезду: видна, но пока за верхним краем.
        this.el.inert = false
        this.scrim.hidden = false
      }
      const move = (e: PointerEvent) => {
        const dy = e.clientY - start.clientY
        if (Math.abs(dy) > 4) moved = true
        if (!moved) return
        this.el.classList.add('dragging')
        offset = direction === 1 ? Math.min(0, -height + Math.max(0, dy)) : Math.min(0, dy)
        this.el.style.transform = `translateY(${offset}px)`
        this.scrim.style.opacity = String(1 - Math.abs(offset) / height)
      }
      const up = () => {
        handle.removeEventListener('pointermove', move)
        handle.removeEventListener('pointerup', up)
        handle.removeEventListener('pointercancel', up)
        this.el.classList.remove('dragging')
        this.scrim.style.opacity = ''
        const shown = height + offset // сколько шторки видно
        if (direction === 1) {
          if (!moved || shown > height * 0.25) this.open()
          else {
            this.el.style.transform = ''
            if (!this.isOpen) { this.el.inert = true; this.scrim.hidden = true }
          }
        } else if (moved && -offset > Math.min(120, height * 0.2)) this.close()
        else this.el.style.transform = ''
      }
      handle.addEventListener('pointermove', move)
      handle.addEventListener('pointerup', up)
      handle.addEventListener('pointercancel', up)
    })
  }
}
