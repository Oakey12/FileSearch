// Фон окна: обычный или «Частицы» — ночное небо со светлячками: размытые
// светящиеся круги мерцают и кружат по всему окну (по мотивам
// codepen.io/hf666/pen/WVrpWe). Интерфейс при этом становится чуть прозрачным.
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

/** Параметры одного светлячка: всё случайное, чтобы небо не повторялось. */
export interface Particle {
  size: number // px, 10–40
  path: [number, number][] // 4 точки (vw, vh): светлячок кружит между ними и возвращается
  duration: number // мс — полный круг по точкам
  delay: number // мс, отрицательная: с первого кадра светлячки уже в разных местах пути
  pulse: number // мс — период мерцания
  pulseDelay: number // мс
  blur: number // px — глубина: дальние мягче, ближние чётче
}

/** Случайные светлячки. rnd — для тестов; по умолчанию Math.random. */
export function makeParticles(count = PARTICLE_COUNT, rnd: () => number = Math.random): Particle[] {
  const between = (a: number, b: number) => a + (b - a) * rnd()
  return Array.from({ length: count }, () => {
    const duration = between(14000, 30000)
    // Точки — по всему окну, в разные стороны, а не только снизу вверх.
    const path = Array.from({ length: 4 }, (): [number, number] => [between(-4, 104), between(-4, 104)])
    const near = rnd() // 0 — далеко, 1 — близко
    return {
      size: Math.round(10 + 30 * near ** 2),
      path,
      duration,
      delay: -between(0, duration),
      pulse: between(1600, 4000),
      pulseDelay: -between(0, 4000),
      blur: Math.round((1.5 + 5.5 * (1 - near)) * 10) / 10,
    }
  })
}

/** Рисует светлячков в host; возвращает функцию, которая всё убирает. */
export function mountParticles(host: HTMLElement, particles: Particle[] = makeParticles()): () => void {
  // «Уменьшение движения» в Windows здесь не учитываем: фон человек включил сам,
  // а в удалённом рабочем столе и на офисных ПК анимации Windows часто выключены.
  const frag = document.createDocumentFragment()
  for (const p of particles) {
    const wrap = document.createElement('span')
    wrap.className = 'bd-particle'
    const s = wrap.style
    s.width = s.height = `${p.size}px`
    p.path.forEach(([x, y], i) => {
      s.setProperty(`--x${i}`, `${x.toFixed(1)}vw`)
      s.setProperty(`--y${i}`, `${y.toFixed(1)}vh`)
    })
    s.animationDuration = `${Math.round(p.duration)}ms`
    s.animationDelay = `${Math.round(p.delay)}ms`
    const dot = document.createElement('span')
    dot.className = 'bd-dot'
    dot.style.filter = `blur(${p.blur}px)`
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

/**
 * Следит за плавностью первые две секунды после включения фона: если кадры
 * медленнее 45 в секунду (слабая видеокарта, удалённый рабочий стол), вызывает
 * onSlow — стекло без размытия. Возвращает отмену.
 */
export function watchSmoothness(onSlow: () => void, sampleMs = 2000, minFps = 45): () => void {
  let frames = 0
  let start = 0
  let raf = requestAnimationFrame(function step(t) {
    if (!start) start = t
    frames++
    if (t - start < sampleMs) {
      raf = requestAnimationFrame(step)
    } else if (!document.hidden && (frames * 1000) / (t - start) < minFps) {
      onSlow()
    }
  })
  return () => cancelAnimationFrame(raf)
}
