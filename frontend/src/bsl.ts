// Подсветка встроенного языка 1С (8.x и 7.7). Строит DOM без innerHTML:
// текст файла никогда не превращается в разметку.

const KEYWORDS = new Set([
  'процедура', 'конецпроцедуры', 'функция', 'конецфункции', 'перем', 'экспорт', 'знач',
  'если', 'тогда', 'иначеесли', 'иначе', 'конецесли', 'для', 'каждого', 'из', 'по', 'цикл',
  'конеццикла', 'пока', 'возврат', 'продолжить', 'прервать', 'попытка', 'исключение',
  'конецпопытки', 'вызватьисключение', 'новый', 'и', 'или', 'не', 'истина', 'ложь',
  'неопределено', 'null', 'перейти', 'асинх', 'ждать', 'выполнить', 'добавитьобработчик',
  'удалитьобработчик',
  'procedure', 'endprocedure', 'function', 'endfunction', 'var', 'export', 'val', 'if', 'then',
  'elsif', 'else', 'endif', 'for', 'each', 'in', 'to', 'do', 'enddo', 'while', 'return',
  'continue', 'break', 'try', 'except', 'endtry', 'raise', 'new', 'and', 'or', 'not', 'true',
  'false', 'undefined', 'goto', 'async', 'await', 'execute', 'addhandler', 'removehandler',
])

const WORD = /[A-Za-zА-Яа-яЁё_][A-Za-zА-Яа-яЁё_0-9]*/y
const NUMBER = /\d+(?:\.\d+)?/y

type Kind = 'kw' | 'com' | 'str' | 'num' | 'dir' | 'date'

/**
 * Заполняет el подсвеченным кодом. Многострочные строки 1С (продолжение
 * начинается с «|») и даты в одинарных кавычках учитываются.
 */
export function renderBSL(el: HTMLElement, code: string): void {
  const out = document.createDocumentFragment()
  let plain = ''
  const flush = () => {
    if (plain) {
      out.append(plain)
      plain = ''
    }
  }
  const token = (kind: Kind, text: string) => {
    flush()
    const span = document.createElement('span')
    span.className = `t-${kind}`
    span.textContent = text
    out.append(span)
  }

  let inString = false // строка не закрыта в конце прошлой строки
  const lines = code.split('\n')
  for (let li = 0; li < lines.length; li++) {
    const line = lines[li]
    let i = 0
    if (li > 0) plain += '\n'

    if (inString) {
      const end = closingQuote(line, 0)
      token('str', line.slice(0, end < 0 ? line.length : end + 1))
      if (end < 0) continue
      inString = false
      i = end + 1
    } else {
      const trimmed = line.trimStart()
      if (trimmed.startsWith('#')) {
        // Препроцессор: #Область, #Если Сервер Тогда …
        plain += line.slice(0, line.length - trimmed.length)
        token('dir', trimmed)
        continue
      }
    }

    while (i < line.length) {
      const ch = line[i]
      if (ch === '/' && line[i + 1] === '/') {
        token('com', line.slice(i))
        break
      }
      if (ch === '"') {
        const end = closingQuote(line, i + 1)
        if (end < 0) {
          token('str', line.slice(i))
          inString = true
          break
        }
        token('str', line.slice(i, end + 1))
        i = end + 1
        continue
      }
      if (ch === "'") {
        const end = line.indexOf("'", i + 1)
        const stop = end < 0 ? line.length : end + 1
        token('date', line.slice(i, stop))
        i = stop
        continue
      }
      if (ch === '&') {
        WORD.lastIndex = i + 1
        const m = WORD.exec(line)
        if (m) {
          token('dir', '&' + m[0])
          i = WORD.lastIndex
          continue
        }
      }
      WORD.lastIndex = i
      const w = WORD.exec(line)
      if (w) {
        if (KEYWORDS.has(w[0].toLowerCase())) token('kw', w[0])
        else plain += w[0]
        i = WORD.lastIndex
        continue
      }
      NUMBER.lastIndex = i
      const n = NUMBER.exec(line)
      if (n) {
        token('num', n[0])
        i = NUMBER.lastIndex
        continue
      }
      plain += ch
      i++
    }
  }
  flush()
  el.replaceChildren(out)
}

/** Позиция закрывающей кавычки (с учётом удвоенных ""), -1 — строка не закрыта. */
function closingQuote(line: string, from: number): number {
  for (let i = from; i < line.length; i++) {
    if (line[i] !== '"') continue
    if (line[i + 1] === '"') {
      i++
      continue
    }
    return i
  }
  return -1
}
