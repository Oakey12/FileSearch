// Шторка сверху: «Заметки» и «Холст».
// Открыть — кнопка, Ctrl+J или язычок у верхнего края окна (можно потянуть вниз).
// Нижняя ручка меняет высоту; двойной щелчок по ней или кнопка ⤢ разворачивают
// шторку на всё окно. Если стянуть шторку совсем низко — она закроется.

import { Board } from './board'
import type { FileItem } from './list'
import { Notes } from './notes'
import { toast } from './toast'

type Pane = 'notes' | 'board'

const byId = <T extends HTMLElement>(id: string) => document.getElementById(id) as T

const MIN_HEIGHT = 320
const CLOSE_BELOW = 200 // отпустили ниже — закрыть

const store = {
  get(key: string): string | null {
    try { return localStorage.getItem(key) } catch { return null }
  },
  set(key: string, value: string): void {
    try { localStorage.setItem(key, value) } catch { /* без хранилища — по умолчанию */ }
  },
}

export class Drawer {
  readonly notes = new Notes()
  readonly board = new Board()

  private el = byId<HTMLElement>('drawer')
  private scrim = byId<HTMLDivElement>('drawerScrim')
  private tab = byId<HTMLButtonElement>('drawerTab')
  private grip = byId<HTMLDivElement>('drawerGrip')
  private maxButton = byId<HTMLButtonElement>('drawerMax')
  private notesPane = byId<HTMLDivElement>('notesPane')
  private boardPane = byId<HTMLDivElement>('boardPane')
  private pane: Pane = store.get('drawerPane') === 'board' ? 'board' : 'notes'
  private maximized = store.get('drawerMax') === '1'
  private height = Number(store.get('drawerHeight')) || 0
  private returnFocus: HTMLElement | null = null
  isOpen = false

  constructor() {
    byId('openNotes').addEventListener('click', () => this.toggle())
    byId('drawerClose').addEventListener('click', () => this.close())
    this.maxButton.addEventListener('click', () => this.setMaximized(!this.maximized))
    this.scrim.addEventListener('click', () => this.close())
    this.el.querySelectorAll<HTMLButtonElement>('[data-pane]').forEach((b) => {
      b.addEventListener('click', () => this.show(b.dataset.pane as Pane))
    })
    this.pullToOpen()
    this.gripResize()
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
    window.addEventListener('resize', () => this.applySize())

    // Холст показывает заметки и открывает их; заметки отправляют себя на холст.
    this.board.notes = {
      get: (id) => this.notes.get(id),
      list: () => this.notes.all(),
      open: (id) => {
        this.show('notes')
        void this.notes.show(id)
      },
    }
    this.notes.onChange = () => this.board.refreshNoteCards()
    this.notes.onToBoard = (id) => {
      this.show('board')
      void this.board.addNote(id)
    }
    this.applySize()
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
    store.set('drawerPane', pane)
    this.el.querySelectorAll<HTMLButtonElement>('[data-pane]').forEach((b) => {
      b.setAttribute('aria-selected', String(b.dataset.pane === pane))
    })
    this.notesPane.hidden = pane !== 'notes'
    this.boardPane.hidden = pane !== 'board'
    if (!this.isOpen) return
    if (pane === 'notes') {
      void this.notes.open().then(() => this.notes.focusEditor())
    } else {
      // Заметки нужны холсту, чтобы показать текст карточек-заметок.
      void this.notes.open().then(() => this.board.open()).then(() => {
        this.board.refreshNoteCards()
        this.board.focus()
      })
    }
  }

  // --- Размер шторки ----------------------------------------------------

  private setMaximized(on: boolean): void {
    this.maximized = on
    store.set('drawerMax', on ? '1' : '0')
    this.applySize()
  }

  private applySize(): void {
    this.el.classList.toggle('maximized', this.maximized)
    this.maxButton.setAttribute('aria-pressed', String(this.maximized))
    this.maxButton.title = this.maximized ? 'Свернуть шторку' : 'Развернуть на всё окно'
    this.maxButton.querySelector('use')?.setAttribute('href', this.maximized ? '#i-collapse' : '#i-expand')
    if (this.maximized || !this.height) {
      this.el.style.height = ''
    } else {
      this.el.style.height = `${Math.min(window.innerHeight, Math.max(MIN_HEIGHT, this.height))}px`
    }
  }

  /** Нижняя ручка: тянуть — высота, отпустить совсем низко — закрыть, двойной щелчок — на всё окно. */
  private gripResize(): void {
    this.grip.addEventListener('dblclick', () => this.setMaximized(!this.maximized))
    this.grip.addEventListener('pointerdown', (start) => {
      if (start.button !== 0) return
      start.preventDefault()
      try { this.grip.setPointerCapture(start.pointerId) } catch { /* синтетические события */ }
      const top = this.el.getBoundingClientRect().top
      let last = start.clientY
      let moved = false
      const move = (e: PointerEvent) => {
        last = e.clientY
        if (!moved && Math.abs(last - start.clientY) < 4) return
        moved = true
        this.el.classList.add('dragging')
        this.el.classList.remove('maximized')
        const h = Math.min(window.innerHeight, last - top + 9)
        this.el.style.height = `${Math.max(CLOSE_BELOW - 40, h)}px`
        this.el.style.opacity = h < CLOSE_BELOW ? '0.6' : ''
      }
      const up = () => {
        this.grip.removeEventListener('pointermove', move)
        this.grip.removeEventListener('pointerup', up)
        this.grip.removeEventListener('pointercancel', up)
        this.el.classList.remove('dragging')
        this.el.style.opacity = ''
        if (!moved) return
        const h = last - top + 9
        if (h < CLOSE_BELOW) {
          this.applySize() // вернуть прежний размер к следующему открытию
          this.close()
          return
        }
        this.height = Math.round(Math.min(window.innerHeight, h))
        store.set('drawerHeight', String(this.height))
        this.maximized = this.height >= window.innerHeight - 8
        store.set('drawerMax', this.maximized ? '1' : '0')
        this.applySize()
      }
      this.grip.addEventListener('pointermove', move)
      this.grip.addEventListener('pointerup', up)
      this.grip.addEventListener('pointercancel', up)
    })
  }

  /** Язычок у верхнего края: щелчок — открыть, потянуть вниз — шторка выезжает за курсором. */
  private pullToOpen(): void {
    this.tab.addEventListener('pointerdown', (start) => {
      if (start.button !== 0) return
      start.preventDefault()
      try { this.tab.setPointerCapture(start.pointerId) } catch { /* синтетические события */ }
      const height = this.el.offsetHeight
      let offset = -height
      let moved = false
      if (!this.isOpen) {
        this.el.inert = false
        this.scrim.hidden = false
      }
      const move = (e: PointerEvent) => {
        const dy = e.clientY - start.clientY
        if (Math.abs(dy) > 4) moved = true
        if (!moved) return
        this.el.classList.add('dragging')
        offset = Math.min(0, -height + Math.max(0, dy))
        this.el.style.transform = `translateY(${offset}px)`
        this.scrim.style.opacity = String(1 + offset / height)
      }
      const up = () => {
        this.tab.removeEventListener('pointermove', move)
        this.tab.removeEventListener('pointerup', up)
        this.tab.removeEventListener('pointercancel', up)
        this.el.classList.remove('dragging')
        this.scrim.style.opacity = ''
        if (!moved || height + offset > height * 0.25) this.open()
        else {
          this.el.style.transform = ''
          if (!this.isOpen) { this.el.inert = true; this.scrim.hidden = true }
        }
      }
      this.tab.addEventListener('pointermove', move)
      this.tab.addEventListener('pointerup', up)
      this.tab.addEventListener('pointercancel', up)
    })
  }
}
