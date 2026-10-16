// Фон окна: обычный или «Частицы» — ночное небо с медленно всплывающими
// светящимися кругами (по мотивам codepen.io/hf666/pen/WVrpWe). Интерфейс
// при этом становится чуть прозрачным.
//
// Скорость важнее эффекта: кругов немного, анимируются только transform и
// opacity, у всех один @keyframes с CSS-переменными, на свёрнутом окне
// анимация стоит, а без фона элементов нет вовсе.

export type Backdrop = 'none' | 'particles'

/** Ключ в localStorage. Тот же ключ читает встроенный скрипт в index.html. */
export const BACKDROP_KEY = 'backdrop'

/** Сколько кругов. В оригинале 200 — для приложения, которое должно летать, много. */
export const PARTICLE_COUNT = 56

type Store = Pick<Storage, 'getItem' | 'setItem'>
const defaultStore = (): Store => localStorage

export function loadBackdrop(store: () => Store = defaultStore): Backdrop {
  try {
    return store().getItem(BACKDROP_KEY) === 'particles' ? 'particles' : 'none'
  } catch {
    return 'none'
  }
}

export function saveBackdrop(value: Backdrop, store: () => Store = defaultStore): void {
  try { store().setItem(BACKDROP_KEY, value) } catch { /* без хранилища — на один запуск */ }
}

/** Ставит фон на <html>: от атрибута зависят прозрачность панелей и цвета. */
export function applyBackdrop(value: Backdrop, root: HTMLElement = document.documentElement): void {
  if (value === 'particles') root.dataset.backdrop = 'particles'
  else delete root.dataset.backdrop
}

export function currentBackdrop(root: HTMLElement = document.documentElement): Backdrop {
  return root.dataset.backdrop === 'particles' ? 'particles' : 'none'
}

/** Параметры одного круга: всё случайное, чтобы небо не повторялось. */
export interface Particle {
  size: number // px, 8–34
  x0: number // vw — откуда всплывает
  x1: number // vw — куда уплывает
  y0: number // vh — старт ниже окна
  y1: number // vh — финиш выше окна
  duration: number // мс — путь снизу вверх
  delay: number // мс, отрицательная: круг уже на полпути, а не все стартуют снизу
  pulse: number // мс — период «дыхания»
  pulseDelay: number // мс
}

/** Случайные круги. rnd — для тестов; по умолчанию Math.random. */
export function makeParticles(count = PARTICLE_COUNT, rnd: () => number = Math.random): Particle[] {
  const between = (a: number, b: number) => a + (b - a) * rnd()
  return Array.from({ length: count }, () => {
    const duration = between(26000, 48000)
    const y0 = between(102, 115)
    return {
      // Крупнее, чем в оригинале, и чаще мелкие: квадрат случайного числа.
      size: Math.round(8 + 26 * rnd() ** 2),
      x0: between(-5, 105),
      x1: between(-5, 105),
      y0,
      y1: -y0 - between(0, 30),
      duration,
      delay: -between(0, duration),
      pulse: between(2400, 6000),
      pulseDelay: -between(0, 6000),
    }
  })
}

/** Рисует круги в host; возвращает функцию, которая всё убирает. */
export function mountParticles(host: HTMLElement, particles: Particle[] = makeParticles()): () => void {
  const reduced = matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false
  const frag = document.createDocumentFragment()
  for (const p of particles) {
    const wrap = document.createElement('span')
    wrap.className = 'bd-particle'
    const s = wrap.style
    s.width = s.height = `${p.size}px`
    if (reduced) {
      // Без движения: круги просто стоят в случайных местах.
      s.transform = `translate3d(${p.x0}vw, ${(p.y0 - 102) * 7}vh, 0)`
    } else {
      s.setProperty('--x0', `${p.x0}vw`)
      s.setProperty('--x1', `${p.x1}vw`)
      s.setProperty('--y0', `${p.y0}vh`)
      s.setProperty('--y1', `${p.y1}vh`)
      s.animationDuration = `${Math.round(p.duration)}ms`
      s.animationDelay = `${Math.round(p.delay)}ms`
    }
    const dot = document.createElement('span')
    dot.className = 'bd-dot'
    dot.style.animationDuration = `${Math.round(p.pulse)}ms`
    dot.style.animationDelay = `${Math.round(p.pulseDelay)}ms`
    wrap.append(dot)
    frag.append(wrap)
  }
  host.replaceChildren(frag)

  // Окно свёрнуто или закрыто другим — анимация не нужна.
  const onVisibility = () => host.classList.toggle('bd-paused', document.hidden)
  document.addEventListener('visibilitychange', onVisibility)
  onVisibility()
  return () => {
    document.removeEventListener('visibilitychange', onVisibility)
    host.replaceChildren()
    host.classList.remove('bd-paused')
  }
}
