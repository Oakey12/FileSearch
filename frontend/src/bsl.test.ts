// @vitest-environment happy-dom
import { describe, expect, it } from 'vitest'
import { renderBSL } from './bsl'

function tokens(code: string) {
  const el = document.createElement('pre')
  renderBSL(el, code)
  return {
    el,
    of: (kind: string) => [...el.querySelectorAll(`.t-${kind}`)].map((s) => s.textContent),
  }
}

describe('подсветка 1С', () => {
  it('ключевые слова, комментарии, числа, директивы', () => {
    const t = tokens('&НаКлиенте\nПроцедура Тест() Экспорт // пояснение\n\tА = 42;\nКонецПроцедуры')
    expect(t.of('kw')).toEqual(['Процедура', 'Экспорт', 'КонецПроцедуры'])
    expect(t.of('com')).toEqual(['// пояснение'])
    expect(t.of('num')).toEqual(['42'])
    expect(t.of('dir')).toEqual(['&НаКлиенте'])
  })

  it('текст не меняется и не превращается в разметку', () => {
    const code = 'Сообщить("<b>жирный</b> ""в кавычках""");\n#Область Тест\nА = \'20240101\';'
    const t = tokens(code)
    expect(t.el.textContent).toBe(code)
    expect(t.el.querySelector('b')).toBeNull()
    expect(t.of('str')).toEqual(['"<b>жирный</b> ""в кавычках"""'])
    expect(t.of('dir')).toEqual(['#Область Тест'])
    expect(t.of('date')).toEqual(["'20240101'"])
  })

  it('многострочная строка с | не ломает подсветку дальше', () => {
    const t = tokens('Текст = "ВЫБРАТЬ\n|  Поле // не комментарий\n|ИЗ Таблица";\nЕсли А Тогда')
    expect(t.of('com')).toEqual([])
    expect(t.of('str').join('\n')).toContain('|  Поле // не комментарий')
    expect(t.of('kw')).toEqual(['Если', 'Тогда'])
  })

  it('слова из букв ключевых слов не подсвечиваются', () => {
    expect(tokens('Процедурный = Если1 + Иначее;').of('kw')).toEqual([])
  })
})
