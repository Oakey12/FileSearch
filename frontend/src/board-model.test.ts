import { describe, expect, it } from 'vitest'
import {
  BoardModel, MAX_CARD_WIDTH, MAX_FONT, MIN_CARD_HEIGHT, MIN_CARD_WIDTH, MIN_FONT, bodySizeOf, fontSizeOf, stepFont,
} from './board-model'

const size = () => ({ w: 200, h: 80 })

function board() {
  const m = new BoardModel()
  const a = m.addCard({ x: 0, y: 0, title: 'A' })
  const b = m.addCard({ x: 300, y: 0, title: 'B' })
  const c = m.addCard({ x: 600, y: 0, title: 'C' })
  return { m, a, b, c }
}

describe('связи', () => {
  it('не дублируются и не замыкаются на себя', () => {
    const { m, a, b } = board()
    expect(m.link(a.id, b.id)).not.toBeNull()
    expect(m.link(a.id, b.id)).toBeNull()
    expect(m.link(b.id, a.id)).toBeNull() // обратная — тоже дубль
    expect(m.link(a.id, a.id)).toBeNull()
    expect(m.link(a.id, 'нет такой')).toBeNull()
    expect(m.links).toHaveLength(1)
  })

  it('удаляются вместе с карточкой', () => {
    const { m, a, b, c } = board()
    m.link(a.id, b.id)
    m.link(b.id, c.id)
    m.link(a.id, c.id)
    m.remove([b.id])
    expect(m.cards.has(b.id)).toBe(false)
    expect(m.links.map((l) => [l.from, l.to])).toEqual([[a.id, c.id]])
  })

  it('стиль связи меняется', () => {
    const { m, a, b } = board()
    const link = m.link(a.id, b.id)!
    m.updateLink(link.id, { dashed: true })
    expect(m.links[0].dashed).toBe(true)
    m.unlink(link.id)
    expect(m.links).toHaveLength(0)
  })
})

describe('карточки', () => {
  it('размер ограничен разумными пределами', () => {
    const { m, a } = board()
    m.resize(a.id, 10, 5)
    expect(m.cards.get(a.id)!.w).toBe(MIN_CARD_WIDTH)
    expect(m.cards.get(a.id)!.h).toBe(MIN_CARD_HEIGHT)
    m.resize(a.id, 5000)
    expect(m.cards.get(a.id)!.w).toBe(MAX_CARD_WIDTH)
  })

  it('сдвигаются группой', () => {
    const { m, a, b } = board()
    m.moveBy([a.id, b.id], 10, -5)
    expect([m.cards.get(a.id)!.x, m.cards.get(a.id)!.y]).toEqual([10, -5])
    expect([m.cards.get(b.id)!.x, m.cards.get(b.id)!.y]).toEqual([310, -5])
  })

  it('дублируются вместе со связями внутри группы', () => {
    const { m, a, b, c } = board()
    m.link(a.id, b.id)
    m.link(b.id, c.id) // c не копируется — эта связь не должна появиться у копий
    const copies = m.duplicate([a.id, b.id])
    expect(copies).toHaveLength(2)
    expect(m.cards.size).toBe(5)
    const copiedLinks = m.links.filter((l) => copies.includes(l.from) || copies.includes(l.to))
    expect(copiedLinks).toHaveLength(1)
    expect(copies).toContain(copiedLinks[0].from)
    expect(copies).toContain(copiedLinks[0].to)
    expect(m.cards.get(copies[0])!.title).toBe('A')
  })

  it('выделяются рамкой', () => {
    const { m, a, b } = board()
    expect(m.cardsInRect({ x: -10, y: -10, w: 350, h: 50 }, size).sort()).toEqual([a.id, b.id].sort())
    expect(m.cardsInRect({ x: 900, y: 0, w: 10, h: 10 }, size)).toEqual([])
  })

  it('новая карточка ищет свободное место рядом', () => {
    const { m } = board()
    // Точка (0,0) занята карточкой A — место должно найтись рядом и не пересекаться.
    const spot = m.freeSpotNear(0, 0, 200, 80, size)
    expect(m.cardsInRect({ x: spot.x, y: spot.y, w: 200, h: 80 }, size)).toEqual([])
    expect(Math.hypot(spot.x, spot.y)).toBeLessThan(400)
    // Свободная точка остаётся как есть.
    expect(m.freeSpotNear(0, 500, 200, 80, size)).toEqual({ x: 0, y: 500 })
  })

  it('связанная карточка ставится справа и не поверх соседа', () => {
    const { m, a } = board()
    // Справа от A на месте новой карточки уже стоит B (x=300).
    const spot = m.placeBeside(a.id, size)
    expect(spot.x).toBe(280)
    expect(m.cardsInRect({ x: spot.x, y: spot.y, w: 220, h: MIN_CARD_HEIGHT }, size)).toEqual([])
  })
})

describe('текст карточки', () => {
  it('старые размеры s/m/l читаются как пиксели', () => {
    const { m, a, b, c } = board()
    m.update(a.id, { size: 's' })
    m.update(c.id, { size: 'l' })
    expect([fontSizeOf(m.cards.get(a.id)!), fontSizeOf(m.cards.get(b.id)!), fontSizeOf(m.cards.get(c.id)!)]).toEqual([12, 14, 19])
  })

  it('размер ограничен и заменяет старый формат', () => {
    const { m, a, b } = board()
    m.update(a.id, { size: 'l' })
    m.setFontSize([a.id, b.id], 200)
    expect(fontSizeOf(m.cards.get(a.id)!)).toBe(MAX_FONT)
    expect(m.cards.get(a.id)!.size).toBeUndefined()
    m.setFontSize([a.id], 3)
    expect(fontSizeOf(m.cards.get(a.id)!)).toBe(MIN_FONT)
  })

  it('ступени размера и текст мельче заголовка', () => {
    expect(stepFont(14, 1)).toBe(16)
    expect(stepFont(15, 1)).toBe(16)
    expect(stepFont(14, -1)).toBe(12)
    expect(stepFont(MAX_FONT, 1)).toBe(MAX_FONT)
    expect(stepFont(MIN_FONT, -1)).toBe(MIN_FONT)
    const { m, a } = board()
    m.setFontSize([a.id], 40)
    expect(bodySizeOf(m.cards.get(a.id)!)).toBeLessThan(40)
  })

  it('цвет текста отменяется', () => {
    const { m, a } = board()
    m.commit()
    m.update(a.id, { textColor: '#30d158' })
    m.undo()
    expect(m.cards.get(a.id)!.textColor).toBeUndefined()
  })
})

describe('отмена и повтор', () => {
  it('возвращают состояние по шагам', () => {
    const { m, a, b } = board()
    m.commit()
    m.link(a.id, b.id)
    m.commit()
    m.update(a.id, { title: 'Новый' })
    m.commit()
    m.remove([b.id])

    expect(m.undo()).toBe(true) // вернулась B и связь
    expect(m.cards.has(b.id)).toBe(true)
    expect(m.links).toHaveLength(1)
    expect(m.undo()).toBe(true) // старый заголовок
    expect(m.cards.get(a.id)!.title).toBe('A')
    expect(m.undo()).toBe(true) // без связи
    expect(m.links).toHaveLength(0)
    expect(m.undo()).toBe(false)

    expect(m.redo()).toBe(true)
    expect(m.redo()).toBe(true)
    expect(m.cards.get(a.id)!.title).toBe('Новый')
  })

  it('пустая правка не засоряет историю', () => {
    const { m, a } = board()
    m.commit()
    m.update(a.id, { title: 'A' }) // текст тот же
    m.discardCommitIfUnchanged()
    expect(m.canUndo).toBe(false)
    m.commit()
    m.update(a.id, { title: 'Б' })
    m.discardCommitIfUnchanged()
    expect(m.canUndo).toBe(true)
  })

  it('новое изменение сбрасывает повтор', () => {
    const { m, a } = board()
    m.commit()
    m.update(a.id, { color: '#0a84ff' })
    m.undo()
    m.commit()
    m.update(a.id, { title: 'другое' })
    expect(m.canRedo).toBe(false)
  })
})

describe('сохранение', () => {
  it('переживает сериализацию и отбрасывает битые связи', () => {
    const { m, a, b } = board()
    m.link(a.id, b.id)
    m.view = { x: 5, y: 6, k: 1.5 }
    const data = JSON.parse(JSON.stringify(m.toJSON()))
    data.links.push({ id: 'x', from: a.id, to: 'удалённая' })

    const copy = new BoardModel()
    copy.load(data)
    expect(copy.cards.size).toBe(3)
    expect(copy.links).toHaveLength(1)
    expect(copy.view).toEqual({ x: 5, y: 6, k: 1.5 })
    expect(copy.canUndo).toBe(false)
  })

  it('загружает пустой холст', () => {
    const m = new BoardModel()
    m.load(null)
    expect(m.cards.size).toBe(0)
  })
})
