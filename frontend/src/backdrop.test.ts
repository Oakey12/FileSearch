// @vitest-environment happy-dom
import { describe, expect, it } from 'vitest'
import { BACKDROP_KEY, PARTICLE_COUNT, applyBackdrop, currentBackdrop, loadBackdrop, makeParticles, mountParticles, saveBackdrop } from './backdrop'

function memoryStore() {
  const data = new Map<string, string>()
  return {
    getItem: (k: string) => data.get(k) ?? null,
    setItem: (k: string, v: string) => { data.set(k, v) },
    data,
  }
}

// Предсказуемый генератор для проверок.
function seeded(seed: number): () => number {
  let s = seed
  return () => {
    s = (s * 16807) % 2147483647
    return (s - 1) / 2147483646
  }
}

describe('выбор фона', () => {
  it('по умолчанию — без фона, выбор сохраняется', () => {
    const store = memoryStore()
    expect(loadBackdrop(() => store)).toBe('none')
    saveBackdrop('particles', () => store)
    expect(store.data.get(BACKDROP_KEY)).toBe('particles')
    expect(loadBackdrop(() => store)).toBe('particles')
  })

  it('недоступное хранилище не ломает программу', () => {
    const broken = () => { throw new Error('нет доступа') }
    expect(loadBackdrop(broken)).toBe('none')
    expect(() => saveBackdrop('particles', broken)).not.toThrow()
  })

  it('атрибут на <html> ставится и снимается', () => {
    const root = document.createElement('html')
    applyBackdrop('particles', root)
    expect(root.dataset.backdrop).toBe('particles')
    expect(currentBackdrop(root)).toBe('particles')
    applyBackdrop('none', root)
    expect(root.dataset.backdrop).toBeUndefined()
    expect(currentBackdrop(root)).toBe('none')
  })
})

describe('частицы', () => {
  it('случайные, крупные и сразу разбросаны по небу', () => {
    const ps = makeParticles(400, seeded(7))
    expect(ps).toHaveLength(400)
    for (const p of ps) {
      expect(p.size).toBeGreaterThanOrEqual(8)
      expect(p.size).toBeLessThanOrEqual(34)
      expect(p.y0).toBeGreaterThan(100) // старт ниже окна
      expect(p.y1).toBeLessThan(-100) // финиш выше окна
      // Отрицательная задержка: круг уже в пути, а не ждёт старта внизу.
      expect(p.delay).toBeLessThanOrEqual(0)
      expect(-p.delay).toBeLessThanOrEqual(p.duration)
    }
    // Разброс по горизонтали — по всему окну, а не кучкой.
    const xs = ps.map((p) => p.x0)
    expect(Math.min(...xs)).toBeLessThan(10)
    expect(Math.max(...xs)).toBeGreaterThan(90)
    // Пути не вертикальные: начало и конец по горизонтали различаются.
    expect(ps.filter((p) => Math.abs(p.x1 - p.x0) > 10).length).toBeGreaterThan(ps.length / 2)
  })

  it('рисуются, ставятся на паузу на скрытом окне и убираются без следа', () => {
    const host = document.createElement('div')
    const stop = mountParticles(host, makeParticles(PARTICLE_COUNT, seeded(3)))
    expect(host.querySelectorAll('.bd-particle')).toHaveLength(PARTICLE_COUNT)
    expect(host.querySelectorAll('.bd-dot')).toHaveLength(PARTICLE_COUNT)
    const first = host.querySelector<HTMLElement>('.bd-particle')!
    expect(first.style.getPropertyValue('--x0')).toMatch(/vw$/)
    expect(first.style.animationDelay.startsWith('-')).toBe(true)

    Object.defineProperty(document, 'hidden', { configurable: true, get: () => true })
    document.dispatchEvent(new Event('visibilitychange'))
    expect(host.classList.contains('bd-paused')).toBe(true)

    stop()
    expect(host.childElementCount).toBe(0)
    expect(host.classList.contains('bd-paused')).toBe(false)
    // После остановки обработчик снят: класс больше не появляется.
    document.dispatchEvent(new Event('visibilitychange'))
    expect(host.classList.contains('bd-paused')).toBe(false)
    Object.defineProperty(document, 'hidden', { configurable: true, get: () => false })
  })
})
