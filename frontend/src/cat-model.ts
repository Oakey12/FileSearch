// Модель рыжего кота: поведение по тикам (10 в секунду) и пиксельные позы.
// Без DOM — рисование и окно в cat.ts.

export const SCENE_W = 128
export const SCENE_H = 72
export const GROUND = 62
export const SPR_W = 28
export const SPR_H = 24

export type Mode = 'sit' | 'walk' | 'play' | 'wash' | 'sleep'
export type Command = 'sit' | 'walk' | 'play'
type PlayPhase = 'chase' | 'stalk' | 'leap' | 'watch'

// Прямоугольник позы в координатах спрайта: x, y, ширина, высота, цвет.
export type Px = [number, number, number, number, string]

export interface Sprite {
  layers: Px[][] // каждый слой обводится контуром, верхние — поверх нижних
  details: Px[]  // глаза, полоски, нос — без контура
}

export const C = {
  out: '#3a1d0c',
  fur: '#ef8a2e',
  dark: '#c2601a',
  cream: '#ffd9a8',
  eye: '#86d44f',
  pupil: '#1b1b1b',
  nose: '#ff8fa0',
  ball: '#5b8def',
  ballLight: '#a9c4ff',
  heart: '#ff4d6d',
  zzz: '#9cc8ff',
}

export interface Cat {
  mode: Mode
  x: number          // левый край спрайта
  lift: number       // высота прыжка над полом
  facing: 1 | -1
  tick: number       // общий счётчик тиков
  modeTicks: number  // сколько тиков в текущем режиме
  idle: number       // сколько тиков кот сидит без команды
  ball: { x: number; vx: number } | null
  phase: PlayPhase
  phaseTicks: number
  leapFrom: number
  leapTo: number
}

const MIN_X = 2
const MAX_X = SCENE_W - SPR_W - 2
const BALL = 5

export function newCat(): Cat {
  return {
    mode: 'sit', x: (SCENE_W - SPR_W) / 2, lift: 0, facing: 1, tick: 0, modeTicks: 0, idle: 0,
    ball: null, phase: 'chase', phaseTicks: 0, leapFrom: 0, leapTo: 0,
  }
}

function setMode(cat: Cat, mode: Mode) {
  cat.mode = mode
  cat.modeTicks = 0
  cat.lift = 0
  if (mode !== 'play') cat.ball = null
}

export function command(cat: Cat, cmd: Command, rnd: () => number = Math.random) {
  setMode(cat, cmd)
  cat.idle = 0
  if (cmd === 'play') {
    // Клубок появляется с той стороны, где больше места.
    const right = cat.x < SCENE_W / 2
    cat.ball = { x: right ? SCENE_W - 20 - rnd() * 10 : 14 + rnd() * 10, vx: 0 }
    cat.phase = 'chase'
    cat.phaseTicks = 0
  }
}

// Кот проснулся или отвлёкся от умывания — садится.
export function poke(cat: Cat) {
  if (cat.mode === 'sleep' || cat.mode === 'wash') setMode(cat, 'sit')
  cat.idle = 0
}

// Где у кота передние лапы (координата сцены).
function paw(cat: Cat) {
  return cat.facing > 0 ? cat.x + 24 : cat.x + 4
}

function clampX(x: number) {
  return Math.max(MIN_X, Math.min(MAX_X, x))
}

export function step(cat: Cat, rnd: () => number = Math.random) {
  cat.tick++
  cat.modeTicks++
  switch (cat.mode) {
    case 'sit':
      // Сам себе развлечение: посидел — умылся, ещё посидел — уснул.
      cat.idle++
      if (cat.idle >= 160) setMode(cat, 'sleep')
      else if (cat.modeTicks >= 60) setMode(cat, 'wash')
      break
    case 'wash':
      cat.idle++
      if (cat.modeTicks >= 40) setMode(cat, 'sit')
      break
    case 'sleep':
      break
    case 'walk':
      cat.x += cat.facing
      if (cat.x >= MAX_X) { cat.x = MAX_X; cat.facing = -1 }
      if (cat.x <= MIN_X) { cat.x = MIN_X; cat.facing = 1 }
      if (cat.modeTicks >= 200) setMode(cat, 'sit') // нагулялся
      break
    case 'play':
      stepPlay(cat, rnd)
      if (cat.modeTicks >= 260 && cat.phase !== 'leap') { setMode(cat, 'sit'); cat.idle = 0 } // наигрался
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
      cat.x = clampX(cat.x + cat.facing * 1.5)
      if (cat.x === MIN_X || cat.x === MAX_X) { cat.phase = 'stalk'; cat.phaseTicks = 0 }
      break
    }
    case 'stalk':
      cat.facing = centre >= cat.x + SPR_W / 2 ? 1 : -1
      if (cat.phaseTicks >= 12) {
        cat.phase = 'leap'
        cat.phaseTicks = 0
        cat.leapFrom = cat.x
        cat.leapTo = clampX(cat.facing > 0 ? ball.x - 22 : ball.x - 1)
      }
      break
    case 'leap': {
      const t = Math.min(1, cat.phaseTicks / 6)
      cat.x = cat.leapFrom + (cat.leapTo - cat.leapFrom) * t
      cat.lift = Math.round(Math.sin(Math.PI * t) * 10)
      if (t >= 1) {
        cat.lift = 0
        ball.vx = cat.facing * (2.5 + rnd() * 2.5)
        cat.phase = 'watch'
        cat.phaseTicks = 0
      }
      break
    }
    case 'watch':
      ball.x += ball.vx
      ball.vx *= 0.88
      if (ball.x < 1) { ball.x = 1; ball.vx = -ball.vx }
      if (ball.x > SCENE_W - BALL - 1) { ball.x = SCENE_W - BALL - 1; ball.vx = -ball.vx }
      cat.facing = centre >= cat.x + SPR_W / 2 ? 1 : -1
      if (Math.abs(ball.vx) < 0.2) { ball.vx = 0; cat.phase = 'chase'; cat.phaseTicks = 0 }
      break
  }
}

// Попадает ли точка сцены в кота (с запасом в пару пикселей — по коту легко промахнуться).
export function hitCat(cat: Cat, x: number, y: number) {
  const top = GROUND - SPR_H - cat.lift
  return x >= cat.x - 2 && x <= cat.x + SPR_W + 2 && y >= top - 3 && y <= GROUND + 2
}

// ---------- Позы ----------

const SWAY = [0, 1, 0, -1]

function standing(f: number, opts: { crouch?: boolean; leap?: boolean; walk?: boolean; sway: number; blink: boolean }): Sprite {
  const by = opts.crouch ? 14 : 12
  const bh = opts.crouch ? 6 : 7
  const hy = opts.crouch ? 9 : 5
  const legTop = by + bh
  let legs: Px[]
  if (opts.leap) {
    legs = [[0, by + 3, 5, 2, C.fur], [1, by + 5, 4, 2, C.fur], [20, by + 4, 6, 2, C.fur], [19, by + 6, 6, 2, C.fur]]
  } else {
    const dx = opts.walk ? [[1, -1, -1, 1], [0, 0, 0, 0], [-1, 1, 1, -1], [0, 0, 0, 0]][f % 4] : [0, 0, 0, 0]
    const up = opts.walk ? [[], [0, 2], [], [1, 3]][f % 4] : []
    legs = [5, 8, 14, 17].map((x, i): Px => {
      const h = SPR_H - legTop - (up.includes(i) ? 1 : 0)
      return [x + dx[i], legTop, 2, h, C.fur]
    })
  }
  const tail: Px[] = opts.crouch || opts.leap
    ? [[0, by + 1, 5, 2, C.fur], [0 + Math.max(0, opts.sway), by - 1 + Math.min(0, opts.sway), 2, 3, C.fur]]
    : [[2, 6, 2, 8, C.fur], [2, 12, 3, 2, C.fur], [1 + opts.sway, 3, 2, 4, C.fur]]
  const layers: Px[][] = [[
    ...tail,
    [4, by, 16, bh, C.fur],
    ...legs,
    [17, hy, 10, 9, C.fur],
    [18, hy - 2, 3, 2, C.fur],
    [24, hy - 2, 3, 2, C.fur],
  ]]
  const eyes: Px[] = opts.blink
    ? [[19, hy + 4, 2, 1, C.out], [24, hy + 4, 2, 1, C.out]]
    : [[19, hy + 3, 2, 2, C.eye], [20, hy + 3, 1, 2, C.pupil], [24, hy + 3, 2, 2, C.eye], [25, hy + 3, 1, 2, C.pupil]]
  const details: Px[] = [
    [8, by, 1, 3, C.dark], [11, by, 1, 3, C.dark], [14, by, 1, 3, C.dark],
    [6, by + bh - 2, 11, 2, C.cream],
    [21, hy, 1, 2, C.dark], [23, hy, 1, 2, C.dark],
    [19, hy - 1, 1, 1, C.nose], [25, hy - 1, 1, 1, C.nose],
    [21, hy + 6, 5, 2, C.cream],
    [23, hy + 5, 2, 1, C.nose],
    ...eyes,
  ]
  return { layers, details }
}

function sitting(f: number, opts: { blink: boolean; wash: boolean; tip: number }): Sprite {
  const hy = 3 + (opts.wash && f % 2 ? 1 : 0)
  const tipY = 19 + opts.tip
  const layers: Px[][] = [[
    [1, 22, 7, 2, C.fur],
    [0, tipY, 2, 24 - tipY, C.fur],
    [6, 14, 12, 10, C.fur],
    [11, 10, 8, 10, C.fur],
    [12, 18, 2, 6, C.fur],
    [16, 18, 2, 6, C.fur],
    [10, hy, 11, 9, C.fur],
    [11, hy - 2, 3, 2, C.fur],
    [18, hy - 2, 3, 2, C.fur],
  ]]
  if (opts.wash) layers.push([[16, 8 + (f % 2), 3, 8, C.fur]]) // лапка у мордочки
  const closed = opts.blink || opts.wash
  const eyes: Px[] = closed
    ? [[12, hy + 5, 2, 1, C.out], [17, hy + 5, 2, 1, C.out]]
    : [[12, hy + 4, 2, 2, C.eye], [13, hy + 4, 1, 2, C.pupil], [17, hy + 4, 2, 2, C.eye], [18, hy + 4, 1, 2, C.pupil]]
  const details: Px[] = [
    [13, 12, 4, 6, C.cream],
    [14, 19, 1, 5, C.dark],
    [12, 23, 2, 1, C.cream], [16, 23, 2, 1, C.cream],
    [7, 16, 1, 3, C.dark], [9, 15, 1, 3, C.dark],
    [14, hy, 1, 2, C.dark], [16, hy, 1, 2, C.dark],
    [12, hy - 1, 1, 1, C.nose], [19, hy - 1, 1, 1, C.nose],
    [13, hy + 6, 5, 2, C.cream],
    [15, hy + 6, 2, 1, C.nose],
    ...eyes,
  ]
  if (opts.wash) {
    details.push([16, 8 + (f % 2), 3, 1, C.cream])
    if (f % 2 === 0) details.push([15, hy + 8, 1, 1, C.nose]) // язычок
  }
  return { layers, details }
}

function sleeping(f: number): Sprite {
  const breathe = f % 2
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
    [7, 16 - breathe, 1, 3, C.dark], [10, 15 - breathe, 1, 3, C.dark], [13, 16 - breathe, 1, 3, C.dark],
    [18, 18, 2, 1, C.out], [22, 18, 2, 1, C.out],
    [20, 19, 3, 2, C.cream],
    [21, 19, 1, 1, C.nose],
    [5, 21, 1, 2, C.dark], [9, 21, 1, 2, C.dark], [13, 21, 1, 2, C.dark],
  ]
  return { layers, details }
}

export function sprite(cat: Cat): Sprite {
  const t = cat.tick
  const blink = t % 40 < 2
  switch (cat.mode) {
    case 'walk':
      return standing(Math.floor(t / 2), { walk: true, sway: SWAY[Math.floor(t / 3) % 4], blink })
    case 'sleep':
      return sleeping(Math.floor(t / 8))
    case 'wash':
      return sitting(Math.floor(t / 3), { blink, wash: true, tip: SWAY[Math.floor(t / 4) % 4] })
    case 'play':
      if (cat.phase === 'leap') return standing(0, { leap: true, sway: 0, blink: false })
      if (cat.phase === 'stalk') return standing(0, { crouch: true, sway: SWAY[t % 4], blink: false })
      if (cat.phase === 'chase') return standing(Math.floor(t / 2), { walk: true, sway: SWAY[t % 4], blink: false })
      return standing(0, { sway: SWAY[Math.floor(t / 2) % 4], blink })
    default:
      return sitting(0, { blink, wash: false, tip: SWAY[Math.floor(t / 4) % 4] })
  }
}
