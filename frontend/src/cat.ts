// Окно рыжего кота (cat.html): прозрачное, без рамки, поверх всех окон. Видно
// только кота — он ходит по «полу» (сначала это низ экрана), а окно едет вместе с ним.
// Клик по коту — меню команд, перетаскивание — кота несут за шкирку куда угодно,
// двойной клик — окно поиска «разворачивается» из кота в центр экрана.
import './cat.css'
import { CatOpened, CatSetup } from '../wailsjs/go/main/App'
import { Quit, WindowSetPosition, WindowSetSize, WindowShow } from '../wailsjs/runtime/runtime'
import {
  BALL, C, SCRUFF, SPR_H, SPR_W, carry, command, drop, hitCat, newCat, poke, sprite, step,
  type Cat, type Command, type Sprite,
} from './cat-model'

// Клетка кота — UNIT точек экрана; рисуем с точностью до половины клетки (FINE = 2).
// Окно — VIEW_W × VIEW_H клеток; его размер задан в app_cat.go (catWidth/catHeight).
const UNIT = 4
const FINE = 2
const VIEW_W = 90
const VIEW_H = 55
const GROUND_Y = VIEW_H - 2
// Где загривок, когда кота несут: курсор держит кота здесь (клетки окна).
const ANCHOR = { x: VIEW_W / 2, y: 7 }
// На какой высоте над полом окажутся лапы отпущенного кота.
const DROP_HEIGHT = GROUND_Y - (ANCHOR.y - SCRUFF.y + 32)

const stage = document.getElementById('stage') as HTMLDivElement
const canvas = document.getElementById('cat') as HTMLCanvasElement
const menu = document.getElementById('catMenu') as HTMLDivElement
const commands = [...menu.querySelectorAll<HTMLButtonElement>('[data-cat]')]
const ctx = canvas.getContext('2d')!
canvas.width = VIEW_W * FINE
canvas.height = VIEW_H * FINE

let setup = { width: 0, height: 0, appWidth: 1180, appHeight: 740, maximised: false, light: false }
let cat: Cat
let dpr = 1
let view = -1          // левый край окна в клетках
let winX = 0           // положение окна, физические пиксели от края рабочей области
let winY = 0
let shown = false
let opening = false
let clickTimer = 0

// ---------- Окно ----------

function viewLeft() {
  let focus = cat.x + SPR_W / 2
  if (cat.ball) focus = (focus + cat.ball.x + BALL / 2) / 2 // в игре в окне и кот, и клубок
  return Math.round(Math.max(0, Math.min(cat.width - VIEW_W, focus - VIEW_W / 2)))
}

function moveWindow() {
  try {
    WindowSetPosition(Math.round(winX), Math.round(winY))
    if (!shown) WindowShow()
  } catch { /* в браузере окна нет */ }
  shown = true
}

function follow() {
  if (cat.mode === 'carried' || opening) return
  const left = viewLeft()
  if (left === view && shown) return
  view = left
  winX = view * UNIT * dpr
  moveWindow()
}

// ---------- Рисование (в «мелких» пикселях: клетка = FINE) ----------

function rect(x: number, y: number, w: number, h: number, color: string) {
  ctx.fillStyle = color
  ctx.fillRect(Math.round(x * FINE), Math.round(y * FINE), Math.round(w * FINE), Math.round(h * FINE))
}

function px(x: number, y: number, w: number, h: number, color: string) {
  ctx.fillStyle = color
  ctx.fillRect(x, y, w, h)
}

// Поза в клетках окна: контур в один мелкий пиксель со срезанными углами, на шерсти —
// блик сверху и тень снизу и справа (свет падает слева сверху).
function drawSprite(s: Sprite, ox: number, oy: number, facing: number) {
  const at = (p: [number, number, number, number, string]) => {
    const x = facing > 0 ? ox + p[0] : ox + SPR_W - p[0] - p[2]
    return [Math.round(x * FINE), Math.round((oy + p[1]) * FINE), Math.round(p[2] * FINE), Math.round(p[3] * FINE)]
  }
  for (const layer of s.layers) {
    for (const p of layer) {
      const [x, y, w, h] = at(p)
      px(x - 1, y, w + 2, h, C.out)
      px(x, y - 1, w, h + 2, C.out)
    }
    for (const p of layer) {
      const [x, y, w, h] = at(p)
      px(x, y, w, h, p[4])
      if (p[4] !== C.fur) continue
      if (w >= 8 && h >= 8) {
        px(x + 1, y, w - 2, 1, C.furLight)
        px(x, y + 1, 1, h - 3, C.furLight)
        px(x, y + h - 2, w, 2, C.shade)
        px(x + w - 1, y + 1, 1, h - 1, C.shade)
      } else if (w >= 3) {
        px(x + w - 1, y, 1, h, C.shade)
      }
    }
  }
  for (const p of s.details) {
    const [x, y, w, h] = at(p)
    px(x, y, w, h, p[4])
  }
}

function drawShadow(cx: number, width: number) {
  const w = Math.round(width * FINE)
  const x = Math.round(cx * FINE - w / 2)
  const y = GROUND_Y * FINE
  ctx.globalAlpha = 0.28
  px(x + 2, y, w - 4, 1, '#000')
  px(x, y + 1, w, 2, '#000')
  px(x + 2, y + 3, w - 4, 1, '#000')
  ctx.globalAlpha = 1
}

// Клубок — кружок 5×5 клеток с бликом, тенью и намотанной ниткой, которая крутится.
function drawBall(bx: number) {
  const r = BALL * FINE / 2
  const cx = Math.round(bx * FINE) + r
  const cy = GROUND_Y * FINE - r
  const spin = Math.round(bx * 2) % 4
  for (let dy = -r - 1; dy <= r; dy++) {
    const half = Math.round(Math.sqrt(Math.max(0, (r + 0.5) ** 2 - (dy + 0.5) ** 2)))
    if (!half) continue
    px(cx - half - 1, cy + dy, half * 2 + 2, 1, C.out)
  }
  for (let dy = -r; dy < r; dy++) {
    const half = Math.round(Math.sqrt(Math.max(0, r * r - (dy + 0.5) ** 2)))
    for (let dx = -half; dx < half; dx++) {
      const light = dx + dy < -r / 2
      const dark = dx + dy > r / 2
      let color = light ? C.ballLight : dark ? C.ballDark : C.ball
      if ((dx + dy * 2 + spin + 40) % 4 === 0 && !light) color = C.ballLight // нитки
      px(cx + dx, cy + dy, 1, 1, color)
    }
  }
  px(cx - 2, cy - 3, 1, 1, C.white)
}

const Z = ['#####', '...#.', '..#..', '.#...', '#####']

function drawZzz(ox: number, oy: number, facing: number) {
  const head = facing > 0 ? ox + 22 : ox + 2
  for (let i = 0; i < 3; i++) {
    const age = (cat.tick + i * 10) % 30
    const x = Math.round((head + facing * age / 5 + (facing > 0 ? 0 : -3)) * FINE)
    const y = Math.round((oy + 9 - age / 3) * FINE)
    const k = i === 1 ? 1 : 0
    ctx.globalAlpha = 1 - age / 30
    Z.forEach((row, ry) => [...row].forEach((ch, rx) => {
      if (ch === '#') px(x + rx - 1 - k, y + ry - 1, 3, 3, C.out)
    }))
    Z.forEach((row, ry) => [...row].forEach((ch, rx) => {
      if (ch === '#') px(x + rx, y + ry, 1, 1, C.zzz)
    }))
  }
  ctx.globalAlpha = 1
}

function draw() {
  ctx.clearRect(0, 0, canvas.width, canvas.height)
  const s = sprite(cat)
  if (cat.mode === 'carried') {
    drawSprite(s, ANCHOR.x - SCRUFF.x, ANCHOR.y - SCRUFF.y, 1)
    // кулачок, который держит загривок
    rect(ANCHOR.x - 2, ANCHOR.y - 4, 4, 2.5, C.out)
    rect(ANCHOR.x - 1.5, ANCHOR.y - 3.5, 3, 1.5, '#f3d2b3')
    return
  }
  const ox = Math.round(cat.x) - view
  const oy = GROUND_Y - SPR_H - cat.lift
  drawShadow(ox + SPR_W / 2, Math.max(8, 20 - cat.lift))
  drawSprite(s, ox, oy, cat.facing)
  if (cat.ball) drawBall(cat.ball.x - view)
  if (cat.mode === 'sleep') drawZzz(ox, oy, cat.facing)
}

// ---------- Меню ----------

function placeMenu() {
  if (menu.hidden) return
  const centre = (cat.x + SPR_W / 2 - view) * UNIT
  const w = menu.offsetWidth
  menu.style.left = `${Math.round(Math.max(4, Math.min(VIEW_W * UNIT - w - 4, centre - w / 2)))}px`
  menu.style.top = `${Math.max(2, (GROUND_Y - SPR_H - cat.lift) * UNIT - menu.offsetHeight - 8)}px`
}

function toggleMenu(open = menu.hidden) {
  menu.hidden = !open
  placeMenu()
}

function tick() {
  if (opening) return
  step(cat)
  if (cat.mode === 'carried') cat.sway *= 0.75
  follow()
  draw()
  for (const b of commands) b.setAttribute('aria-pressed', String(b.dataset.cat === cat.mode))
  placeMenu()
}

// ---------- Мышь: клик, перетаскивание, двойной клик ----------

function onCat(e: MouseEvent) {
  return hitCat(cat, view + e.clientX / UNIT, e.clientY / UNIT - GROUND_Y)
}

let press: { sx: number; sy: number } | null = null
let drag: { winX: number; winY: number; sx: number; sy: number; lastX: number } | null = null
let justDragged = false
let frame = 0

canvas.addEventListener('pointerdown', (e) => {
  if (e.button !== 0 || opening || !onCat(e)) return
  press = { sx: e.screenX, sy: e.screenY }
  try { canvas.setPointerCapture(e.pointerId) } catch { /* указатель уже отпущен */ }
})

canvas.addEventListener('pointermove', (e) => {
  if (!press) {
    document.body.style.cursor = !opening && onCat(e) ? 'grab' : ''
    return
  }
  if (!drag) {
    if (Math.hypot(e.screenX - press.sx, e.screenY - press.sy) < 5) return
    // Взяли за шкирку: окно прыгает так, чтобы загривок оказался под курсором.
    window.clearTimeout(clickTimer)
    menu.hidden = true
    carry(cat)
    winX += (e.clientX - ANCHOR.x * UNIT) * dpr
    winY += (e.clientY - ANCHOR.y * UNIT) * dpr
    drag = { winX, winY, sx: e.screenX, sy: e.screenY, lastX: e.screenX }
    document.body.style.cursor = 'grabbing'
    moveWindow()
    draw()
    return
  }
  winX = drag.winX + (e.screenX - drag.sx) * dpr
  winY = drag.winY + (e.screenY - drag.sy) * dpr
  // Кот раскачивается против движения: чем резче рывок, тем сильнее.
  cat.sway = Math.max(-2, Math.min(2, cat.sway * 0.6 + (e.screenX - drag.lastX) / 8))
  drag.lastX = e.screenX
  if (!frame) frame = requestAnimationFrame(() => { frame = 0; moveWindow(); draw() })
})

function release() {
  press = null
  if (!drag) return
  drag = null
  justDragged = true
  document.body.style.cursor = ''
  // Где отпустили, там теперь и пол. Окно не уходит за края рабочей области.
  const maxY = setup.height - VIEW_H * UNIT * dpr
  winY = Math.max(0, Math.min(maxY, winY))
  view = Math.round(winX / (UNIT * dpr))
  cat.x = view + ANCHOR.x - SPR_W / 2
  view = viewLeft()
  winX = view * UNIT * dpr
  moveWindow()
  drop(cat, DROP_HEIGHT)
  draw()
}
canvas.addEventListener('pointerup', release)
canvas.addEventListener('pointercancel', release)

document.addEventListener('click', (e) => {
  if (justDragged) { justDragged = false; return }
  if (opening || menu.contains(e.target as Node)) return
  if (!onCat(e)) { toggleMenu(false); return }
  // Ждём: вдруг это первый клик двойного.
  window.clearTimeout(clickTimer)
  clickTimer = window.setTimeout(() => {
    poke(cat)
    toggleMenu()
  }, 250)
})
document.addEventListener('dblclick', (e) => {
  if (opening || cat.mode === 'carried' || !onCat(e)) return
  window.clearTimeout(clickTimer)
  void openApp()
})
document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape') toggleMenu(false)
})
for (const b of commands) {
  b.addEventListener('click', () => {
    command(cat, b.dataset.cat as Command)
    toggleMenu(false)
    tick()
  })
}

// ---------- Открытие окна поиска ----------

const wait = (ms: number) => new Promise((r) => setTimeout(r, ms))

// Окно кота растягивается на весь экран (оно прозрачное), и из кота «разворачивается»
// макет окна поиска — ровно туда и того размера, где появится настоящее окно.
async function openApp() {
  opening = true
  menu.hidden = true
  const sw = setup.width / dpr
  const sh = setup.height / dpr
  const at = { x: winX / dpr, y: winY / dpr }
  const from = {
    x: at.x + (Math.round(cat.x) - view) * UNIT,
    y: at.y + (GROUND_Y - SPR_H - cat.lift) * UNIT,
    w: SPR_W * UNIT,
    h: SPR_H * UNIT,
  }
  const to = setup.maximised
    ? { x: 0, y: 0, w: sw, h: sh }
    : { x: Math.round((sw - setup.appWidth) / 2), y: Math.round((sh - setup.appHeight) / 2), w: setup.appWidth, h: setup.appHeight }

  stage.style.left = `${at.x}px`
  stage.style.top = `${at.y}px`
  try {
    WindowSetPosition(0, 0)
    WindowSetSize(Math.ceil(sw), Math.ceil(sh))
  } catch { /* браузер */ }
  for (let i = 0; i < 20 && window.innerWidth < sw - 2; i++) await wait(16)

  const shell = buildShell(to)
  document.body.append(shell)
  sparks(from)

  const dx = from.x - to.x
  const dy = from.y - to.y
  const grow = shell.animate([
    { transform: `translate(${dx}px, ${dy}px) scale(${from.w / to.w}, ${from.h / to.h})`, borderRadius: '45%', opacity: 0 },
    { opacity: 1, offset: 0.2 },
    { transform: 'none', borderRadius: '9px', opacity: 1 },
  ], { duration: 620, easing: 'cubic-bezier(0.16, 1, 0.3, 1)', fill: 'forwards' })
  canvas.animate([
    { transform: 'none', opacity: 1 },
    { transform: 'scale(1.25)', opacity: 0 },
  ], { duration: 260, easing: 'ease-in', fill: 'forwards' })
  await grow.finished

  try { await CatOpened() } catch { /* браузер */ }
  await wait(220) // настоящее окно появляется под макетом, потом кот уходит
  try { Quit() } catch { /* браузер */ }
}

// Упрощённый макет окна поиска: заголовок, боковая панель, строка поиска.
function buildShell(to: { x: number; y: number; w: number; h: number }) {
  const el = document.createElement('div')
  el.className = `shell${setup.light ? ' shell-light' : ''}`
  Object.assign(el.style, { left: `${to.x}px`, top: `${to.y}px`, width: `${to.w}px`, height: `${to.h}px` })
  el.innerHTML = `
    <div class="shell-title"><i></i>Главная</div>
    <div class="shell-body">
      <div class="shell-side"><b></b><b></b><b></b><b></b></div>
      <div class="shell-main"><div class="shell-search"></div><div class="shell-row"></div><div class="shell-row"></div><div class="shell-row"></div></div>
    </div>`
  return el
}

// Рыжие пиксельные искры разлетаются от кота.
function sparks(from: { x: number; y: number; w: number; h: number }) {
  const colors = [C.fur, C.furLight, C.cream, C.shade]
  for (let i = 0; i < 18; i++) {
    const s = document.createElement('i')
    s.className = 'spark'
    const size = (1 + (i % 3)) * FINE * 2
    Object.assign(s.style, {
      left: `${from.x + from.w / 2}px`, top: `${from.y + from.h / 2}px`,
      width: `${size}px`, height: `${size}px`, background: colors[i % colors.length],
    })
    document.body.append(s)
    const angle = (i / 18) * Math.PI * 2
    const dist = 40 + (i % 5) * 18
    s.animate([
      { transform: 'translate(-50%, -50%)', opacity: 1 },
      { transform: `translate(calc(-50% + ${Math.cos(angle) * dist}px), calc(-50% + ${Math.sin(angle) * dist}px))`, opacity: 0 },
    ], { duration: 520, easing: 'cubic-bezier(0.2, 0.8, 0.3, 1)', fill: 'forwards' })
  }
}

// ---------- Старт ----------

async function start() {
  dpr = window.devicePixelRatio || 1
  try { setup = { ...setup, ...(await CatSetup()) } } catch { /* браузер */ }
  if (!setup.width) setup = { ...setup, width: Math.round(screen.availWidth * dpr), height: Math.round(screen.availHeight * dpr) }
  cat = newCat(Math.max(VIEW_W, Math.floor(setup.width / dpr / UNIT)))
  winY = setup.height - VIEW_H * UNIT * dpr
  tick()
  window.setInterval(tick, 100)
}

void start()
