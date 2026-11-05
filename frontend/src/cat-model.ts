// Модель рыжего кота: поведение по тикам (10 в секунду) и позы в стиле 16-bit.
// Координаты — «клетки кота» вдоль пола, пол на y = 0. Позы рисуются с точностью
// до половины клетки (тени, блики, усы). Без DOM — рисование и окно в cat.ts.

export const SPR_W = 28
export const SPR_H = 24
// Где у кота, которого несут за шкирку, загривок (в координатах позы 'carried').
export const SCRUFF = { x: 13.5, y: 3 }

export type Mode = 'sit' | 'walk' | 'play' | 'wash' | 'yawn' | 'sleep' | 'stretch' | 'carried' | 'fall' | 'land'
export type Command = 'sit' | 'walk' | 'play'
type PlayPhase = 'chase' | 'stalk' | 'leap' | 'watch'

// Прямоугольник позы: x, y, ширина, высота, цвет. Шаг — половина клетки.
export type Px = [number, number, number, number, string]

export interface Sprite {
  layers: Px[][] // каждый слой обводится контуром, верхние — поверх нижних
  details: Px[]  // глаза, полоски, нос, усы — без контура
}

export const C = {
  out: '#2b1408',
  fur: '#f28c28',      // шерсть: у крупных частей сверху блик, снизу тень
  furLight: '#ffb862',
  shade: '#c8641a',
  stripe: '#b4500f',
  cream: '#fff0d4',
  creamShade: '#f1c896',
  eye: '#86d64f',
  eyeDark: '#3f8f2a',
  pupil: '#14110f',
  white: '#ffffff',
  nose: '#ff8fa0',
  earIn: '#f7a3a8',
  mouth: '#5a1a1a',
  whisker: 'rgba(255,246,232,0.85)',
  ball: '#5b8def',
  ballLight: '#a9c4ff',
  ballDark: '#3a63c0',
  zzz: '#9cc8ff',
}

export interface Cat {
  width: number      // ширина мира (экрана) в клетках
  mode: Mode
  x: number          // левый край позы
  lift: number       // высота над полом (прыжок, падение)
  vy: number         // скорость падения
  sway: number       // раскачивание, когда кота несут (−2…2)
  facing: 1 | -1
  tick: number
  modeTicks: number
  idle: number       // сколько тиков кот сидит без команды
  yawned: boolean
  washed: boolean
  ball: { x: number; vx: number } | null
  phase: PlayPhase
  phaseTicks: number
  leapFrom: number
  leapTo: number
}

export const BALL = 5
const EDGE = 2

export function newCat(width: number, x = (width - SPR_W) / 2): Cat {
  return {
    width, mode: 'sit', x, lift: 0, vy: 0, sway: 0, facing: 1, tick: 0, modeTicks: 0, idle: 0,
    yawned: false, washed: false, ball: null, phase: 'chase', phaseTicks: 0, leapFrom: 0, leapTo: 0,
  }
}

function setMode(cat: Cat, mode: Mode) {
  cat.mode = mode
  cat.modeTicks = 0
  if (mode !== 'fall') cat.lift = 0
  if (mode !== 'play') cat.ball = null
}

function rest(cat: Cat) {
  setMode(cat, 'sit')
  cat.idle = 0
  cat.yawned = false
  cat.washed = false
}

export function command(cat: Cat, cmd: Command, rnd: () => number = Math.random) {
  rest(cat)
  setMode(cat, cmd)
  if (cmd === 'play') {
    // Клубок падает в паре шагов от кота, с той стороны, где есть место.
    const right = cat.x + SPR_W + 30 < cat.width
    const gap = 12 + rnd() * 8
    cat.ball = { x: right ? cat.x + SPR_W + gap : cat.x - gap - BALL, vx: 0 }
    cat.phase = 'chase'
    cat.phaseTicks = 0
  }
}

// Клик по коту: спящий просыпается и потягивается, умывающийся отвлекается.
export function poke(cat: Cat) {
  if (cat.mode === 'sleep') {
    setMode(cat, 'stretch')
    cat.idle = 0
    cat.yawned = cat.washed = false
  } else if (cat.mode === 'wash' || cat.mode === 'yawn') {
    rest(cat)
  }
}

// Кота взяли за шкирку.
export function carry(cat: Cat) {
  setMode(cat, 'carried')
  cat.sway = 0
}

// Кота отпустили на высоте height клеток над его новым полом.
export function drop(cat: Cat, height: number) {
  setMode(cat, 'fall')
  cat.lift = Math.max(0, height)
  cat.vy = 0
}

function paw(cat: Cat) {
  return cat.facing > 0 ? cat.x + 24 : cat.x + 4
}

function maxX(cat: Cat) {
  return cat.width - SPR_W - EDGE
}

function clampX(cat: Cat, x: number) {
  return Math.max(EDGE, Math.min(maxX(cat), x))
}

export function step(cat: Cat, rnd: () => number = Math.random) {
  cat.tick++
  cat.modeTicks++
  switch (cat.mode) {
    case 'sit':
      // Сам себе развлечение: зевнул, умылся, уснул.
      cat.idle++
      if (cat.idle >= 170) setMode(cat, 'sleep')
      else if (cat.idle >= 90 && !cat.washed) { cat.washed = true; setMode(cat, 'wash') }
      else if (cat.idle >= 40 && !cat.yawned) { cat.yawned = true; setMode(cat, 'yawn') }
      break
    case 'yawn':
      cat.idle++
      if (cat.modeTicks >= 16) setMode(cat, 'sit')
      break
    case 'wash':
      cat.idle++
      if (cat.modeTicks >= 40) setMode(cat, 'sit')
      break
    case 'stretch':
      if (cat.modeTicks >= 22) rest(cat)
      break
    case 'sleep':
    case 'carried':
      break
    case 'fall':
      cat.vy += 0.7
      cat.lift -= cat.vy
      if (cat.lift <= 0) setMode(cat, 'land')
      break
    case 'land':
      if (cat.modeTicks >= 4) rest(cat)
      break
    case 'walk':
      cat.x += cat.facing
      if (cat.x >= maxX(cat)) { cat.x = maxX(cat); cat.facing = -1 }
      if (cat.x <= EDGE) { cat.x = EDGE; cat.facing = 1 }
      if (cat.modeTicks >= 300) rest(cat) // нагулялся
      break
    case 'play':
      stepPlay(cat, rnd)
      if (cat.modeTicks >= 260 && cat.phase !== 'leap') rest(cat) // наигрался
      break
  }
}

function stepPlay(cat: Cat, rnd: () => number) {
  const ball = cat.ball!
  cat.phaseTicks++
  const centre = ball.x + BALL / 2
  switch (cat.phase) {
    case 'chase': {
      cat.facing = centre >= cat.x + SPR_W / 2 ? 1 : -1
      const gap = Math.abs(paw(cat) - centre)
      if (gap <= 20) { cat.phase = 'stalk'; cat.phaseTicks = 0; break }
      cat.x = clampX(cat, cat.x + cat.facing * 2)
      if (cat.x === EDGE || cat.x === maxX(cat)) { cat.phase = 'stalk'; cat.phaseTicks = 0 }
      break
    }
    case 'stalk':
      cat.facing = centre >= cat.x + SPR_W / 2 ? 1 : -1
      if (cat.phaseTicks >= 12) {
        cat.phase = 'leap'
        cat.phaseTicks = 0
        cat.leapFrom = cat.x
        cat.leapTo = clampX(cat, cat.facing > 0 ? ball.x - 22 : ball.x - 1)
      }
      break
    case 'leap': {
      const t = Math.min(1, cat.phaseTicks / 6)
      cat.x = cat.leapFrom + (cat.leapTo - cat.leapFrom) * t
      cat.lift = Math.round(Math.sin(Math.PI * t) * 10)
      if (t >= 1) {
        cat.lift = 0
        ball.vx = cat.facing * (1.5 + rnd() * 1.5) // недалеко: клубок должен оставаться у кота в окне
        cat.phase = 'watch'
        cat.phaseTicks = 0
      }
      break
    }
    case 'watch':
      ball.x += ball.vx
      ball.vx *= 0.88
      if (ball.x < 1) { ball.x = 1; ball.vx = -ball.vx }
      if (ball.x > cat.width - BALL - 1) { ball.x = cat.width - BALL - 1; ball.vx = -ball.vx }
      cat.facing = centre >= cat.x + SPR_W / 2 ? 1 : -1
      if (Math.abs(ball.vx) < 0.2) { ball.vx = 0; cat.phase = 'chase'; cat.phaseTicks = 0 }
      break
  }
}

// Попадает ли точка в стоящего на полу кота (y от пола, вверх — минус).
export function hitCat(cat: Cat, x: number, y: number) {
  return x >= cat.x - 2 && x <= cat.x + SPR_W + 2 && y >= -SPR_H - cat.lift - 3 && y <= 2
}

// ---------- Позы ----------

const SWAY = [0, 1, 0, -1]

interface Face {
  hx: number      // левый край головы
  hy: number      // верх головы
  eyes: 'open' | 'closed' | 'wide' | 'sleepy'
  front?: boolean // анфас (сидит, висит), иначе профиль вправо
  mouth?: number  // насколько открыт рот (зевок), 0…1
}

// Мордочка: уши изнутри, полоски на лбу, глаза с бликом, нос, щёки, усы.
function face(f: Face): Px[] {
  const { hx, hy } = f
  const d: Px[] = []
  if (f.front) {
    // голова 11 клеток шириной, глаза симметрично
    d.push([hx + 1.5, hy - 1.5, 1.5, 1.5, C.earIn], [hx + 8, hy - 1.5, 1.5, 1.5, C.earIn])
    d.push([hx + 4, hy, 0.5, 2, C.stripe], [hx + 5.5, hy, 0.5, 1.5, C.stripe], [hx + 7, hy, 0.5, 2, C.stripe])
    d.push([hx + 2.5, hy + 6, 6, 2.5, C.cream], [hx + 3, hy + 8, 5, 0.5, C.creamShade])
    eyes(d, f.eyes, [[hx + 2, hy + 4], [hx + 7, hy + 4]])
    d.push([hx + 4.5, hy + 6, 2, 1, C.nose], [hx + 5, hy + 7, 1, 0.5, C.out])
    if (f.mouth) mouth(d, hx + 5.5, hy + 7.5, f.mouth)
    d.push([hx - 2, hy + 6.5, 3, 0.5, C.whisker], [hx - 1.5, hy + 7.5, 2.5, 0.5, C.whisker])
    d.push([hx + 10, hy + 6.5, 3, 0.5, C.whisker], [hx + 10, hy + 7.5, 2.5, 0.5, C.whisker])
  } else {
    // профиль вправо, голова 10 клеток
    d.push([hx + 1.5, hy - 1.5, 1.5, 1.5, C.earIn], [hx + 7.5, hy - 1.5, 1.5, 1.5, C.earIn])
    d.push([hx + 4, hy, 0.5, 2, C.stripe], [hx + 5, hy, 0.5, 1.5, C.stripe], [hx + 6, hy, 0.5, 2, C.stripe])
    d.push([hx + 4, hy + 5.5, 6, 2.5, C.cream], [hx + 4.5, hy + 7.5, 5, 0.5, C.creamShade])
    eyes(d, f.eyes, [[hx + 2, hy + 3], [hx + 7, hy + 3]])
    d.push([hx + 8.5, hy + 5, 1.5, 1, C.nose])
    if (f.mouth) mouth(d, hx + 8, hy + 6.5, f.mouth)
    d.push([hx + 9.5, hy + 6, 3, 0.5, C.whisker], [hx + 9.5, hy + 7, 2.5, 0.5, C.whisker])
  }
  return d
}

function eyes(d: Px[], kind: Face['eyes'], at: [number, number][]) {
  for (const [x, y] of at) {
    if (kind === 'closed') {
      d.push([x, y + 1, 2, 0.5, C.out], [x - 0.5, y + 0.5, 0.5, 0.5, C.out], [x + 2, y + 0.5, 0.5, 0.5, C.out])
    } else if (kind === 'sleepy') {
      d.push([x, y + 0.5, 2, 1.5, C.eye], [x, y + 1.5, 2, 0.5, C.eyeDark], [x + 0.5, y + 0.5, 1, 1.5, C.pupil], [x, y, 2, 0.5, C.out])
    } else {
      const h = kind === 'wide' ? 2.5 : 2
      d.push([x, y, 2, h, C.eye], [x, y + h - 0.5, 2, 0.5, C.eyeDark])
      d.push(kind === 'wide' ? [x + 0.5, y + 1, 1, 1, C.pupil] : [x + 0.5, y, 1, h, C.pupil])
      d.push([x + 0.5, y, 0.5, 0.5, C.white])
    }
  }
}

function mouth(d: Px[], cx: number, y: number, open: number) {
  const h = 0.5 + Math.round(open * 4) / 2
  d.push([cx - 1.5, y, 3, h, C.mouth])
  if (h >= 1.5) d.push([cx - 1, y + h - 0.5, 2, 0.5, C.nose])                      // язык
  if (h >= 1) d.push([cx - 1.5, y, 0.5, 0.5, C.white], [cx + 1, y, 0.5, 0.5, C.white]) // клыки
}

// Светлые подушечки на лапах, которые стоят на полу.
function paws(legs: Px[], bottom: number): Px[] {
  return legs.filter((l) => l[1] + l[3] >= bottom - 0.5).map((l): Px => [l[0], l[1] + l[3] - 0.5, l[2], 0.5, C.cream])
}

interface StandOpts { crouch?: boolean; leap?: boolean; gallop?: boolean; walk?: boolean; fall?: boolean; squash?: boolean; sway: number; eyes: Face['eyes'] }

function standing(f: number, o: StandOpts): Sprite {
  const crouch = o.crouch || o.squash
  const by = crouch ? 14 : 12
  const bh = crouch ? 6 : 7
  const bx = o.squash ? 3 : 4
  const bw = o.squash ? 18 : 16
  const hy = crouch ? 9 : 5
  const legTop = by + bh
  let legs: Px[]
  if (o.leap) {
    legs = [[0, by + 3, 5, 2, C.fur], [1, by + 5, 4, 2, C.fur], [20, by + 4, 6, 2, C.fur], [19, by + 6, 6, 2, C.fur]]
  } else if (o.gallop) {
    legs = [[8, legTop, 2, 4, C.fur], [10, legTop, 2, 4.5, C.fur], [13, legTop, 2, 4.5, C.fur], [15, legTop, 2, 4, C.fur]]
  } else if (o.fall) {
    legs = [[3, legTop - 1, 2, 5, C.fur], [7, legTop, 2, 4, C.fur], [15, legTop, 2, 4, C.fur], [19, legTop - 1, 2, 5, C.fur]]
  } else {
    const dx = o.walk ? [[1, -1, -1, 1], [0, 0, 0, 0], [-1, 1, 1, -1], [0, 0, 0, 0]][f % 4] : [0, 0, 0, 0]
    const up = o.walk ? [[], [0, 2], [], [1, 3]][f % 4] : []
    legs = [5, 8, 14, 17].map((x, i): Px => [x + dx[i], legTop, 2, SPR_H - legTop - (up.includes(i) ? 1 : 0), C.fur])
  }
  const tail: Px[] = crouch || o.leap
    ? [[0, by + 1, 5, 2, C.fur], [Math.max(0, o.sway), by - 1 + Math.min(0, o.sway), 2, 3, C.fur]]
    : o.fall
      ? [[1, 2, 2, 11, C.fur], [2, 12, 3, 2, C.fur]]
      : [[2, 6, 2, 8, C.fur], [2, 12, 3, 2, C.fur], [1 + o.sway, 3, 2, 4, C.fur]]
  const layers: Px[][] = [[
    ...tail,
    [bx, by, bw, bh, C.fur],
    ...legs,
    [17, hy, 10, 9, C.fur],
    [18, hy - 2, 3, 2, C.fur],
    [24, hy - 2, 3, 2, C.fur],
  ]]
  const t0 = tail[0]
  const details: Px[] = [
    [8, by, 1, 2.5, C.stripe], [11, by, 1, 3, C.stripe], [14, by, 1, 2.5, C.stripe],
    [6, by + bh - 2, 11, 1.5, C.cream],
    ...(t0[3] > t0[2] ? [[t0[0], t0[1] + 2, 2, 0.5, C.stripe], [t0[0], t0[1] + 4.5, 2, 0.5, C.stripe]] as Px[] : []),
    ...(o.leap ? [] : paws(legs, SPR_H)),
    ...face({ hx: 17, hy, eyes: o.eyes }),
  ]
  return { layers, details }
}

function sitting(f: number, o: { eyes: Face['eyes']; wash?: boolean; mouth?: number; tilt?: number; tip: number }): Sprite {
  const hy = 3 + (o.wash && f % 2 ? 1 : 0) - (o.tilt ?? 0)
  const tipY = 19 + o.tip
  const legs: Px[] = [[12, 18, 2, 6, C.fur], [16, 18, 2, 6, C.fur]]
  const layers: Px[][] = [[
    [1, 22, 7, 2, C.fur],
    [0, tipY, 2, 24 - tipY, C.fur],
    [6, 14, 12, 10, C.fur],
    [11, 10, 8, 10, C.fur],
    ...legs,
    [10, hy, 11, 9, C.fur],
    [11, hy - 2, 3, 2, C.fur],
    [18, hy - 2, 3, 2, C.fur],
  ]]
  if (o.wash) layers.push([[16, 8 + (f % 2), 3, 8, C.fur]]) // лапка у мордочки
  const details: Px[] = [
    [13, 12, 4, 6, C.cream], [13.5, 17.5, 3, 0.5, C.creamShade],
    [14, 19, 0.5, 5, C.shade],
    ...paws(legs, SPR_H),
    [7, 16, 1, 3, C.stripe], [9, 15, 1, 3, C.stripe], [3, 22, 0.5, 2, C.stripe], [5, 22, 0.5, 2, C.stripe],
    ...face({ hx: 10, hy, eyes: o.eyes, front: true, mouth: o.mouth }),
  ]
  if (o.wash) {
    details.push([16, 8 + (f % 2), 3, 1, C.cream])
    if (f % 2 === 0) details.push([15, hy + 8.5, 1, 0.5, C.nose]) // язычок
  }
  return { layers, details }
}

function sleeping(f: number): Sprite {
  const breathe = f % 2 ? 0.5 : 0
  const layers: Px[][] = [
    [
      [3, 15 - breathe, 20, 9 + breathe, C.fur],
      [16, 14, 9, 8, C.fur],
      [17, 12, 2, 2, C.fur],
      [22, 12, 2, 2, C.fur],
    ],
    [[4, 21, 15, 2, C.fur], [17, 20, 2, 2, C.fur]], // хвост обвивает лапы
  ]
  const details: Px[] = [
    [7, 16 - breathe, 1, 2.5, C.stripe], [10, 15 - breathe, 1, 3, C.stripe], [13, 16 - breathe, 1, 2.5, C.stripe],
    [17.5, 12.5, 1, 1, C.earIn], [22.5, 12.5, 1, 1, C.earIn],
    [18, 18, 2, 0.5, C.out], [22, 18, 2, 0.5, C.out],
    [19.5, 19, 4, 2, C.cream],
    [21, 19, 1, 0.5, C.nose],
    [6, 21, 0.5, 2, C.stripe], [9, 21, 0.5, 2, C.stripe], [12, 21, 0.5, 2, C.stripe],
  ]
  return { layers, details }
}

function stretching(f: number): Sprite {
  const legs: Px[] = [[2, 19, 2, 5, C.fur], [7, 19, 2, 5, C.fur]]
  const layers: Px[][] = [[
    [0, 2, 2, 9, C.fur], [1, 1, 2, 2, C.fur], // хвост трубой
    [1, 10, 10, 9, C.fur],
    ...legs,
    [8, 14, 10, 6, C.fur],
    [14, 21, 11, 3, C.fur],
    [18, 11, 9, 8, C.fur],
    [19, 9, 3, 2, C.fur],
    [24, 9, 3, 2, C.fur],
  ]]
  const details: Px[] = [
    [3, 11, 1, 3, C.stripe], [6, 10.5, 1, 3, C.stripe], [11, 14.5, 1, 2.5, C.stripe],
    [0, 4, 2, 0.5, C.stripe], [0, 7, 2, 0.5, C.stripe],
    [10, 18.5, 8, 1.5, C.cream],
    ...paws(legs, SPR_H),
    [23.5, 23, 1.5, 1, C.cream], [24, 21.5, 0.5, 1, C.shade], [25, 21.5, 0.5, 1, C.shade],
    ...face({ hx: 17, hy: 11, eyes: 'closed', mouth: f % 6 < 3 ? 0.6 : 0 }),
  ]
  return { layers, details }
}

// Кота несут за шкирку: висит анфас, лапки и хвост качаются против движения.
function hanging(sway: number, f: number): Sprite {
  const s = Math.round(-sway * 2) / 2
  const mid = Math.round(s) / 2
  const layers: Px[][] = [[
    [13 + s, 24, 2, 6, C.fur], [13 + s + SWAY[f % 4] * 0.5, 29, 2, 2, C.fur], // хвост
    [10 + mid, 11, 8, 12, C.fur],
    [9 + s, 18, 10, 6, C.fur],
    [10 + s, 23, 2, 4, C.fur], [16 + s, 23, 2, 4, C.fur], // задние лапы
    [9 + mid, 12, 2, 6, C.fur], [17 + mid, 12, 2, 6, C.fur], // передние
    [8, 3, 11, 9, C.fur],
    [8, 1, 3, 2, C.fur], [16, 1, 3, 2, C.fur],
  ]]
  const details: Px[] = [
    [12 + mid, 13, 4, 7, C.cream], [12.5 + mid, 19.5, 3, 0.5, C.creamShade],
    [10 + s, 26.5, 2, 0.5, C.cream], [16 + s, 26.5, 2, 0.5, C.cream],
    [9 + mid, 17.5, 2, 0.5, C.cream], [17 + mid, 17.5, 2, 0.5, C.cream],
    [13 + s, 26, 2, 0.5, C.stripe], [13 + s, 28, 2, 0.5, C.stripe],
    [10 + s, 19.5, 1, 2, C.stripe], [17 + s, 19.5, 1, 2, C.stripe],
    ...face({ hx: 8, hy: 3, eyes: 'sleepy', front: true }),
  ]
  return { layers, details }
}

export function sprite(cat: Cat): Sprite {
  const t = cat.tick
  const blink = t % 40 < 2
  const open: Face['eyes'] = blink ? 'closed' : 'open'
  switch (cat.mode) {
    case 'walk':
      return standing(Math.floor(t / 2), { walk: true, sway: SWAY[Math.floor(t / 3) % 4], eyes: open })
    case 'sleep':
      return sleeping(Math.floor(t / 8))
    case 'wash':
      return sitting(Math.floor(t / 3), { eyes: 'closed', wash: true, tip: SWAY[Math.floor(t / 4) % 4] })
    case 'yawn': {
      const k = Math.sin(Math.PI * Math.min(1, cat.modeTicks / 16))
      return sitting(0, { eyes: 'closed', mouth: k, tilt: k > 0.5 ? 1 : 0, tip: SWAY[Math.floor(t / 4) % 4] })
    }
    case 'stretch':
      return stretching(cat.modeTicks)
    case 'carried':
      return hanging(cat.sway, Math.floor(t / 2))
    case 'fall':
      return standing(0, { fall: true, sway: 0, eyes: 'wide' })
    case 'land':
      return standing(0, { squash: true, sway: 0, eyes: 'wide' })
    case 'play':
      if (cat.phase === 'leap') return standing(0, { leap: true, sway: 0, eyes: 'wide' })
      if (cat.phase === 'stalk') return standing(0, { crouch: true, sway: SWAY[t % 4], eyes: 'wide' })
      if (cat.phase === 'chase') return t % 2 ? standing(0, { gallop: true, sway: 1, eyes: 'open' }) : standing(0, { leap: true, sway: -1, eyes: 'open' })
      return standing(0, { sway: SWAY[Math.floor(t / 2) % 4], eyes: open })
    default:
      return sitting(0, { eyes: open, tip: SWAY[Math.floor(t / 4) % 4] })
  }
}
