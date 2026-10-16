// Панель предпросмотра справа (как Quick Look в macOS).
// Содержимое файлов вставляется только через textContent.

import { renderBSL } from './bsl'
import { isOneCFile } from './decoder'
import { formatCount, formatLongDate, formatSize, kindOf, plural } from './format'
import { highlightInto, normalize } from './highlight'
import type { FileItem } from './list'

export interface PreviewSheet { name: string; rows: string[][]; truncated?: boolean }

/** Сводка по файлу 1С: что это, из чего состоит, начало главного модуля. */
export interface OneCSummary {
  kind: string
  format: string
  name: string
  synonym?: string
  sections: number
  modules: number
  groups: { name: string; count: number }[]
  main?: { title: string; text: string; lines: number; truncated?: boolean }
}

/** Сколько групп показывать сразу; остальные — «ещё N». */
const ONEC_GROUPS_SHOWN = 8

export interface PreviewResult {
  kind: 'text' | 'image' | 'sheet' | 'pdf' | 'info' | 'onec'
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
  onec?: OneCSummary
}

export interface InspectorActions {
  open(path: string): void
  reveal(path: string): void
  toBoard(item: FileItem): void
  openOneC(path: string): void
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
      this.header(item, result),
      this.view(result),
      this.meta(item, result),
      this.buttons(item, result),
    )
  }

  private header(item: FileItem, result?: PreviewResult): HTMLElement {
    const head = el('header', 'qs-head')
    const titles = el('div', 'qs-titles')
    // .md бывает конфигурацией 1С 7.7 — тогда это не «Текст».
    const onec = result?.kind === 'onec'
    titles.append(el('h2', 'qs-name', item.name), el('p', 'qs-kind', onec ? 'Файл 1С' : kindOf(item.ext).label))
    head.append(fileIcon(onec ? 'epf' : item.ext, 'qs-icon'), titles)
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
      case 'onec':
        return this.oneC(result.onec!)
      default: {
        const view = el('div', 'qs-view qs-message')
        view.append(fileIcon(result.ext, 'qs-big-icon'), el('p', 'qs-dim', result.note ?? ''))
        return view
      }
    }
  }

  /** Файл 1С: карточка (вид, имя, состав по группам) и начало главного модуля. */
  private oneC(s: OneCSummary): HTMLElement {
    const view = el('div', 'qs-view qs-onec')
    const card = el('section', 'onec-card')
    card.append(el('p', 'onec-kind', `${s.kind} · ${s.format}`), el('h3', 'onec-title', s.synonym || s.name))
    if (s.synonym && s.synonym !== s.name) card.append(el('p', 'onec-name', s.name))
    const stats = [`${formatCount(s.modules)} ${plural(s.modules, 'модуль', 'модуля', 'модулей')}`]
    if (s.sections > s.modules) {
      const other = s.sections - s.modules
      stats.push(`${formatCount(other)} ${plural(other, 'макет или справка', 'макета или справки', 'макетов или справок')}`)
    }
    card.append(el('p', 'onec-stats', stats.join(', ')))

    if (s.groups.length > 1) {
      const list = el('ul', 'onec-groups')
      list.setAttribute('aria-label', 'Состав')
      for (const g of s.groups.slice(0, ONEC_GROUPS_SHOWN)) {
        const item = el('li', 'onec-group')
        item.append(el('span', 'onec-group-name', g.name), el('span', 'onec-group-count', formatCount(g.count)))
        list.append(item)
      }
      const rest = s.groups.length - ONEC_GROUPS_SHOWN
      if (rest > 0) list.append(el('li', 'onec-group onec-group-more', `ещё ${rest} ${plural(rest, 'группа', 'группы', 'групп')}`))
      card.append(list)
    }
    view.append(card)

    if (!s.main) {
      view.append(el('p', 'qs-dim onec-empty', 'Модули сохранены без исходного текста — показать нечего.'))
      return view
    }
    const code = el('section', 'onec-code')
    const head = el('header', 'onec-code-head')
    head.append(el('span', 'onec-code-title', s.main.title), el('span', 'onec-code-meta', `${formatCount(s.main.lines)} стр.`))
    const paper = el('div', 'qs-paper paper-scroll onec-paper')
    const pre = el('pre', 'paper-text onec-pre')
    renderBSL(pre, s.main.text)
    paper.append(pre)
    code.append(head, paper)
    if (s.main.truncated) code.append(el('p', 'onec-more', 'Показано начало модуля. Целиком — в декодере.'))
    view.append(code)
    return view
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
    if (result?.encoding && result.kind !== 'onec') add('Кодировка', result.encoding)
    if (result?.truncated || result?.sheets?.some((s) => s.truncated)) add('Показано', result.kind === 'sheet'
      ? 'Первые 500 строк и 50 столбцов'
      : 'Первый 1 МБ текста')
    return list
  }

  private buttons(item: FileItem, result?: PreviewResult): HTMLElement {
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
    // .md — конфигурация 1С 7.7, только если предпросмотр её узнал (иначе это Markdown).
    if (isOneCFile(item.path) || result?.encoding === '1С') {
      // Для файлов 1С главное действие — посмотреть код в декодере.
      const decode = el('button', 'btn btn-primary', 'Открыть в декодере 1С')
      decode.type = 'button'
      decode.addEventListener('click', () => this.actions.openOneC(item.path))
      open.className = 'btn'
      row.append(decode, open, reveal, board)
      return row
    }
    row.append(open, reveal, board)
    return row
  }
}
