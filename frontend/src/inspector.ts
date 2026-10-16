// Панель предпросмотра справа (как Quick Look в macOS).
// Содержимое файлов вставляется только через textContent.

import { formatLongDate, formatSize, kindOf } from './format'
import { highlightInto, normalize } from './highlight'
import type { FileItem } from './list'

export interface PreviewSheet { name: string; rows: string[][]; truncated?: boolean }

export interface PreviewResult {
  kind: 'text' | 'image' | 'sheet' | 'pdf' | 'info'
  path: string
  name: string
  ext: string
  size: number
  mod: number
  text?: string
  encoding?: string
  truncated?: boolean
  sheets?: PreviewSheet[]
  note?: string
  url?: string
}

export interface InspectorActions {
  open(path: string): void
  reveal(path: string): void
  toBoard(item: FileItem): void
}

function el<K extends keyof HTMLElementTagNameMap>(tag: K, className?: string, text?: string): HTMLElementTagNameMap[K] {
  const node = document.createElement(tag)
  if (className) node.className = className
  if (text !== undefined) node.textContent = text
  return node
}

function fileIcon(ext: string, className: string): SVGSVGElement {
  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg')
  svg.setAttribute('class', className)
  svg.dataset.cat = kindOf(ext).category
  const use = document.createElementNS('http://www.w3.org/2000/svg', 'use')
  use.setAttribute('href', '#i-doc')
  svg.appendChild(use)
  return svg
}

export class Inspector {
  /** Слова для подсветки — заданы, когда идёт поиск по тексту. */
  terms: string[] = []

  constructor(private body: HTMLElement, private actions: InspectorActions) {}

  empty(): void {
    this.body.replaceChildren(el('p', 'qs-placeholder', 'Выберите файл в списке, чтобы посмотреть его содержимое'))
  }

  /** Заголовок и действия показываются сразу, содержимое — когда загрузится. */
  loading(item: FileItem): void {
    this.body.replaceChildren(this.header(item), el('div', 'qs-view qs-loading'), this.meta(item), this.buttons(item))
  }

  failed(item: FileItem, message: string): void {
    const view = el('div', 'qs-view qs-message')
    view.append(el('p', '', 'Не удалось открыть файл'), el('p', 'qs-dim', message))
    this.body.replaceChildren(this.header(item), view, this.meta(item), this.buttons(item))
  }

  show(item: FileItem, result: PreviewResult): void {
    this.body.replaceChildren(
      this.header(item),
      this.view(result),
      this.meta(item, result),
      this.buttons(item),
    )
  }

  private header(item: FileItem): HTMLElement {
    const head = el('header', 'qs-head')
    const titles = el('div', 'qs-titles')
    titles.append(el('h2', 'qs-name', item.name), el('p', 'qs-kind', kindOf(item.ext).label))
    head.append(fileIcon(item.ext, 'qs-icon'), titles)
    return head
  }

  private view(result: PreviewResult): HTMLElement {
    switch (result.kind) {
      case 'image': {
        const view = el('div', 'qs-view qs-image')
        const img = el('img')
        img.alt = result.name
        img.decoding = 'async'
        img.src = result.url ?? ''
        view.appendChild(img)
        return view
      }
      case 'pdf': {
        // Встроенный просмотрщик PDF из WebView2 (Edge).
        const view = el('div', 'qs-view qs-pdf')
        const frame = el('iframe')
        frame.title = result.name
        frame.src = `${result.url ?? ''}#view=FitH`
        view.appendChild(frame)
        return view
      }
      case 'text': {
        const view = el('div', 'qs-view qs-text')
        const paper = el('div', 'qs-paper paper-scroll')
        const isDoc = result.ext === 'docx' || result.ext === 'pptx'
        const pre = el('pre', isDoc ? 'paper-text paper-doc' : 'paper-text')
        paper.appendChild(pre)
        view.appendChild(paper)
        const marks = highlightInto(pre, result.text ?? '', this.terms)
        if (marks.length) view.appendChild(this.findBar(marks))
        return view
      }
      case 'sheet':
        return this.sheets(result.sheets ?? [])
      default: {
        const view = el('div', 'qs-view qs-message')
        view.append(fileIcon(result.ext, 'qs-big-icon'), el('p', 'qs-dim', result.note ?? ''))
        return view
      }
    }
  }

  private sheets(sheets: PreviewSheet[]): HTMLElement {
    const view = el('div', 'qs-view qs-sheet')
    const tabs = el('div', 'segmented')
    tabs.setAttribute('role', 'tablist')
    const table = el('div', 'qs-paper sheet-scroll')

    const terms = this.terms
    const cellMatches = (cell: string) => {
      if (!terms.length || !cell) return false
      const norm = normalize(cell)
      return terms.some((t) => norm.includes(t))
    }

    const showSheet = (index: number) => {
      tabs.querySelectorAll('button').forEach((b, i) => b.setAttribute('aria-selected', String(i === index)))
      const sheet = sheets[index]
      const t = el('table', 'sheet-table')
      const body = el('tbody')
      let first: HTMLElement | null = null
      for (const row of sheet.rows) {
        const tr = el('tr')
        for (const cell of row) {
          const td = el('td', cellMatches(cell) ? 'hit' : '', cell)
          if (!first && td.className) first = td
          tr.appendChild(td)
        }
        body.appendChild(tr)
      }
      t.appendChild(body)
      table.replaceChildren(t)
      if (sheet.rows.length === 0) table.replaceChildren(el('p', 'paper-empty', 'Лист пуст'))
      first?.scrollIntoView({ block: 'center', inline: 'center' })
    }

    // Если идёт поиск по тексту, открываем первый лист, где есть совпадение.
    const firstHit = Math.max(0, sheets.findIndex((s) => s.rows.some((r) => r.some(cellMatches))))

    sheets.forEach((sheet, i) => {
      const b = el('button', 'segment', sheet.name)
      b.type = 'button'
      b.setAttribute('role', 'tab')
      b.addEventListener('click', () => showSheet(i))
      tabs.appendChild(b)
    })
    if (sheets.length > 1) view.appendChild(tabs)
    view.appendChild(table)
    if (sheets.length) requestAnimationFrame(() => showSheet(firstHit))
    return view
  }

  /** Панель «3 из 12 ˄ ˅» для перехода между найденными словами в тексте. */
  private findBar(marks: HTMLElement[]): HTMLElement {
    const bar = el('div', 'find-bar')
    const label = el('span', 'find-count')
    const prev = el('button', 'find-btn')
    const next = el('button', 'find-btn')
    prev.type = next.type = 'button'
    prev.title = 'Предыдущее совпадение'
    next.title = 'Следующее совпадение'
    prev.innerHTML = '<svg><use href="#i-up"/></svg>'
    next.innerHTML = '<svg><use href="#i-chevron"/></svg>'
    bar.append(label, prev, next)

    let current = -1
    const go = (i: number) => {
      marks[current]?.classList.remove('current')
      current = (i + marks.length) % marks.length
      const mark = marks[current]
      mark.classList.add('current')
      mark.scrollIntoView({ block: 'center', inline: 'nearest' })
      label.textContent = `${current + 1} из ${marks.length}${marks.length >= 1000 ? '+' : ''}`
    }
    prev.addEventListener('click', () => go(current - 1))
    next.addEventListener('click', () => go(current + 1))
    requestAnimationFrame(() => go(0))
    return bar
  }

  private meta(item: FileItem, result?: PreviewResult): HTMLElement {
    const list = el('dl', 'qs-meta')
    const add = (term: string, value: string) => list.append(el('dt', '', term), el('dd', '', value))
    add('Размер', formatSize(item.size))
    add('Изменён', formatLongDate(item.mod))
    add('Папка', item.dir)
    if (result?.encoding) add('Кодировка', result.encoding)
    if (result?.truncated || result?.sheets?.some((s) => s.truncated)) add('Показано', result.kind === 'sheet'
      ? 'Первые 500 строк и 50 столбцов'
      : 'Первый 1 МБ текста')
    return list
  }

  private buttons(item: FileItem): HTMLElement {
    const row = el('div', 'qs-actions')
    const open = el('button', 'btn btn-primary', 'Открыть')
    const reveal = el('button', 'btn', 'Показать в Проводнике')
    const board = el('button', 'btn btn-icon')
    open.type = reveal.type = board.type = 'button'
    board.title = 'Добавить на холст'
    board.innerHTML = '<svg><use href="#i-board"/></svg>'
    open.addEventListener('click', () => this.actions.open(item.path))
    reveal.addEventListener('click', () => this.actions.reveal(item.path))
    board.addEventListener('click', () => this.actions.toBoard(item))
    row.append(open, reveal, board)
    return row
  }
}
