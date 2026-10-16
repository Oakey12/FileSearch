// Фон окна: обычный или одна из сцен. Интерфейс поверх сцены — матовое стекло.
//   «Частицы» — ночное небо с сиянием и размытыми светящимися кругами
//               (по мотивам codepen.io/hf666/pen/WVrpWe);
//   «Лес»     — туманный ночной лес и мигающие светлячки
//               (по мотивам codepen.io/mikegolus/pen/Jegvym).
//
// Скорость важнее эффекта: кругов немного, анимируются только transform и
// opacity, у всех один @keyframes с CSS-переменными, на свёрнутом окне
// анимация стоит, а без фона элементов нет вовсе.

export type Backdrop = 'none' | 'particles' | 'forest'

const SCENES: Backdrop[] = ['particles', 'forest']

function asBackdrop(v: string | null | undefined): Backdrop {
  return SCENES.includes(v as Backdrop) ? (v as Backdrop) : 'none'
}

/** Ключ в localStorage. Тот же ключ читает встроенный скрипт в index.html. */
export const BACKDROP_KEY = 'backdrop'

/** Сколько кругов. В оригинале 200 — для приложения, которое должно летать, много. */
export const PARTICLE_COUNT = 56

type Store = Pick<Storage, 'getItem' | 'setItem'>
const defaultStore = (): Store => localStorage

export function loadBackdrop(store: () => Store = defaultStore): Backdrop {
  try {
    return asBackdrop(store().getItem(BACKDROP_KEY))
  } catch {
    return 'none'
  }
}

export function saveBackdrop(value: Backdrop, store: () => Store = defaultStore): void {
  try { store().setItem(BACKDROP_KEY, value) } catch { /* без хранилища — на один запуск */ }
}

/** Ставит фон на <html>: от атрибута зависят прозрачность панелей и цвета. */
export function applyBackdrop(value: Backdrop, root: HTMLElement = document.documentElement): void {
  if (value === 'none') delete root.dataset.backdrop
  else root.dataset.backdrop = value
}

export function currentBackdrop(root: HTMLElement = document.documentElement): Backdrop {
  return asBackdrop(root.dataset.backdrop)
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

  return pauseWhenHidden(host, () => host.replaceChildren())
}

/** Окно свёрнуто или закрыто другим — анимация в host не нужна. */
function pauseWhenHidden(host: HTMLElement, cleanup: () => void): () => void {
  const onVisibility = () => host.classList.toggle('bd-paused', document.hidden)
  document.addEventListener('visibilitychange', onVisibility)
  onVisibility()
  return () => {
    document.removeEventListener('visibilitychange', onVisibility)
    cleanup()
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

// --- Лес -----------------------------------------------------------------------

/** Сколько светлячков в лесу. В оригинале 15. */
export const FIREFLY_COUNT = 24

/** Параметры светлячка в лесу. */
export interface Firefly {
  path: [number, number][] // 8 точек (vw, vh): медленный путь туда и обратно
  duration: number // мс — путь в одну сторону
  delay: number // мс, отрицательная
  orbit: number // vw — радиус петель вокруг пути
  orbitDuration: number // мс
  flash: number // мс — период вспышек
  flashDelay: number // мс
  size: number // px — свечение
}

export function makeFireflies(count = FIREFLY_COUNT, rnd: () => number = Math.random): Firefly[] {
  const between = (a: number, b: number) => a + (b - a) * rnd()
  return Array.from({ length: count }, () => {
    const duration = between(60000, 120000)
    const flash = between(5000, 11000)
    return {
      path: Array.from({ length: 8 }, (): [number, number] => [between(-2, 102), between(8, 100)]),
      duration,
      delay: -between(0, duration),
      orbit: Math.round(between(2, 7) * 10) / 10,
      orbitDuration: between(8000, 18000),
      flash,
      flashDelay: -between(0, flash),
      size: Math.round(between(34, 56)),
    }
  })
}

/** Ствол — слегка сужающийся и наклонённый многоугольник, ветки — кривыми. */
function tree(rnd: () => number, x: number, width: number, height: number): string {
  const lean = (rnd() - 0.5) * width * 3
  const top = width * (0.35 + rnd() * 0.25)
  const y0 = height + 10
  const y1 = -40
  const trunk = `M${(x - width / 2).toFixed(1)} ${y0}L${(x - top / 2 + lean).toFixed(1)} ${y1}L${(x + top / 2 + lean).toFixed(1)} ${y1}L${(x + width / 2).toFixed(1)} ${y0}Z`
  // Ветки — короткие изогнутые, с веточкой на конце; больше их вверху кроны.
  let branches = ''
  const count = 3 + Math.floor(rnd() * 4)
  for (let i = 0; i < count; i++) {
    const t = 0.08 + rnd() ** 1.6 * 0.5 // высота ветки: 0 — верх
    const bx = x + lean * (1 - t)
    const by = y1 + (y0 - y1) * t
    const dir = rnd() < 0.5 ? -1 : 1
    const len = Math.min(width * (2 + rnd() * 2.5), 90)
    const ex = dir * len
    const ey = -len * (0.45 + rnd() * 0.5)
    branches += `M${bx.toFixed(1)} ${by.toFixed(1)}q${(ex * 0.55).toFixed(1)} ${(ey * 0.15).toFixed(1)} ${ex.toFixed(1)} ${ey.toFixed(1)}`
    // веточка от середины ветки
    const mx = bx + ex * 0.55
    const my = by + ey * 0.4
    branches += `M${mx.toFixed(1)} ${my.toFixed(1)}l${(dir * len * 0.3).toFixed(1)} ${(-len * 0.45).toFixed(1)}`
  }
  return `<path d="${trunk}"/><path class="br" d="${branches}" stroke-width="${Math.max(1, width * 0.16).toFixed(1)}"/>`
}

/** Туманный лес одной картинкой SVG: дальние деревья светлее и размыты, ближние — тёмные. */
export function makeForest(rnd: () => number = Math.random): string {
  const W = 1600
  const H = 900
  const layer = (count: number, minW: number, maxW: number) => {
    let out = ''
    for (let i = 0; i < count; i++) out += tree(rnd, (W / count) * (i + rnd()), minW + (maxW - minW) * rnd(), H)
    return out
  }
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${W} ${H}" preserveAspectRatio="xMidYMax slice">
<defs>
<radialGradient id="ffGlow" cx="50%" cy="34%" r="70%"><stop offset="0" style="stop-color:var(--forest-light)"/><stop offset="1" style="stop-color:var(--forest-dark)"/></radialGradient>
<linearGradient id="ffHaze" x1="0" y1="0" x2="0" y2="1"><stop offset="0" style="stop-color:var(--forest-haze)" stop-opacity="0.15"/><stop offset="0.7" style="stop-color:var(--forest-haze)" stop-opacity="0.55"/><stop offset="1" style="stop-color:var(--forest-haze)" stop-opacity="0.2"/></linearGradient>
<linearGradient id="ffGround" x1="0" y1="0" x2="0" y2="1"><stop offset="0.55" style="stop-color:var(--forest-dark)" stop-opacity="0"/><stop offset="1" style="stop-color:var(--forest-dark)" stop-opacity="0.95"/></linearGradient>
<filter id="ffBlur" x="-5%" y="-5%" width="110%" height="110%"><feGaussianBlur stdDeviation="3"/></filter>
</defs>
<rect width="${W}" height="${H}" fill="url(#ffGlow)"/>
<g class="far" filter="url(#ffBlur)">${layer(26, 5, 12)}</g>
<rect width="${W}" height="${H}" fill="url(#ffHaze)"/>
<g class="mid">${layer(15, 10, 22)}</g>
<rect width="${W}" height="${H}" fill="url(#ffHaze)" opacity="0.45"/>
<g class="near">${layer(7, 22, 44)}</g>
<rect width="${W}" height="${H}" fill="url(#ffGround)"/>
</svg>`
}

/** Рисует лес и светлячков; возвращает функцию, которая всё убирает. */
export function mountForest(scene: HTMLElement, forest: HTMLElement, host: HTMLElement, flies: Firefly[] = makeFireflies()): () => void {
  forest.innerHTML = makeForest() // разметку собираем сами, внешних данных в ней нет
  const frag = document.createDocumentFragment()
  for (const f of flies) {
    const fly = document.createElement('span')
    fly.className = 'ff'
    const s = fly.style
    f.path.forEach(([x, y], i) => {
      s.setProperty(`--x${i}`, `${x.toFixed(1)}vw`)
      s.setProperty(`--y${i}`, `${y.toFixed(1)}vh`)
    })
    s.animationDuration = `${Math.round(f.duration)}ms`
    s.animationDelay = `${Math.round(f.delay)}ms`
    const orbit = document.createElement('span')
    orbit.className = 'ff-orbit'
    orbit.style.setProperty('--r', `${f.orbit}vw`)
    orbit.style.animationDuration = `${Math.round(f.orbitDuration)}ms`
    const light = document.createElement('span')
    light.className = 'ff-light'
    light.style.width = light.style.height = `${f.size}px`
    light.style.animationDuration = `${Math.round(f.flash)}ms`
    light.style.animationDelay = `${Math.round(f.flashDelay)}ms`
    orbit.append(light)
    fly.append(orbit)
    frag.append(fly)
  }
  host.replaceChildren(frag)
  return pauseWhenHidden(scene, () => {
    forest.replaceChildren()
    host.replaceChildren()
  })
}
