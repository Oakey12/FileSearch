// Режим «рыжий кот»: окно сжимается в маленькое окошко поверх остальных,
// а вместо поиска файлов по полу бродит пиксельный кот. Двойной клик по коту
// (или Esc) возвращает приложение как было.
import {
  WindowGetSize, WindowIsMaximised, WindowMaximise, WindowSetAlwaysOnTop, WindowSetMinSize,
  WindowSetSize, WindowSetTitle, WindowUnmaximise,
} from '../wailsjs/runtime/runtime'
import { C, GROUND, SCENE_H, SCENE_W, SPR_H, SPR_W, command, hitCat, newCat, poke, sprite, step, type Cat, type Command, type Px } from './cat-model'

const CAT_WIDTH = 440
const CAT_HEIGHT = 340
const MAIN_MIN = { w: 760, h: 480 } // как MinWidth/MinHeight в main.go
const MAIN_TITLE = 'Главная'

const LABELS: Record<Cat['mode'], string> = {
  sit: 'Сидит',
  walk: 'Гуляет',
  play: 'Играет с клубком',
  wash: 'Умывается',
  sleep: 'Спит',
}

interface Heart { x: number; y: number; life: number }

export function mountCat(button: HTMLButtonElement) {
  const root = document.getElementById('catMode') as HTMLDivElement
  const canvas = root.querySelector('canvas') as HTMLCanvasElement
  const status = document.getElementById('catStatus') as HTMLElement
  const commands = [...root.querySelectorAll<HTMLButtonElement>('[data-cat]')]
  const ctx = canvas.getContext('2d')!
  canvas.width = SCENE_W
  canvas.height = SCENE_H

  let cat = newCat()
  let hearts: Heart[] = []
  let timer = 0
  let saved: { w: number; h: number; max: boolean } | null = null

  function fit() {
    const box = canvas.parentElement!
    const scale = Math.max(1, Math.floor(Math.min(box.clientWidth / SCENE_W, box.clientHeight / SCENE_H)))
    canvas.style.width = `${SCENE_W * scale}px`
    canvas.style.height = `${SCENE_H * scale}px`
  }

  function draw() {
    ctx.clearRect(0, 0, SCENE_W, SCENE_H)
    ctx.fillStyle = 'rgba(127,127,127,0.16)'
    ctx.fillRect(0, GROUND, SCENE_W, SCENE_H - GROUND)
    ctx.fillStyle = 'rgba(127,127,127,0.4)'
    ctx.fillRect(0, GROUND, SCENE_W, 1)

    const ox = Math.round(cat.x)
    const oy = GROUND - SPR_H - cat.lift
    // Тень сжимается, когда кот в прыжке.
    ctx.fillStyle = 'rgba(0,0,0,0.22)'
    const shadow = Math.max(8, 20 - cat.lift)
    ctx.fillRect(ox + (SPR_W - shadow) / 2, GROUND, shadow, 2)

    drawSprite(ox, oy)
    if (cat.ball) drawBall(Math.round(cat.ball.x)) // клубок поверх лап, чтобы не терялся под котом
    if (cat.mode === 'sleep') drawZzz(ox, oy)
    for (const h of hearts) drawHeart(Math.round(h.x), Math.round(h.y))
  }

  function drawSprite(ox: number, oy: number) {
    const s = sprite(cat)
    const at = (p: Px) => (cat.facing > 0 ? ox + p[0] : ox + SPR_W - p[0] - p[2])
    for (const layer of s.layers) {
      // Контур без углов — пиксельные «скруглённые» края.
      ctx.fillStyle = C.out
      for (const p of layer) {
        ctx.fillRect(at(p) - 1, oy + p[1], p[2] + 2, p[3])
        ctx.fillRect(at(p), oy + p[1] - 1, p[2], p[3] + 2)
      }
      for (const p of layer) {
        ctx.fillStyle = p[4]
        ctx.fillRect(at(p), oy + p[1], p[2], p[3])
      }
    }
    for (const p of s.details) {
      ctx.fillStyle = p[4]
      ctx.fillRect(at(p), oy + p[1], p[2], p[3])
    }
  }

  function drawBall(x: number) {
    const y = GROUND - 5
    ctx.fillStyle = C.out
    ctx.fillRect(x, y - 1, 5, 7)
    ctx.fillRect(x - 1, y, 7, 5)
    ctx.fillStyle = C.ball
    ctx.fillRect(x + 1, y, 3, 5)
    ctx.fillRect(x, y + 1, 5, 3)
    ctx.fillStyle = C.ballLight
    // Нитки «катятся» вместе с клубком.
    const turn = Math.abs(Math.round(x / 2)) % 3
    ctx.fillRect(x + 1 + turn, y + 1, 1, 3)
    ctx.fillRect(x + 1, y + 2, 3, 1)
  }

  function drawZzz(ox: number, oy: number) {
    const head = cat.facing > 0 ? ox + 24 : ox + 2
    for (let i = 0; i < 3; i++) {
      const age = (cat.tick + i * 10) % 30
      const x = head + cat.facing * Math.floor(age / 6) + (cat.facing > 0 ? 0 : -3)
      const y = oy + 10 - Math.floor(age / 3)
      ctx.globalAlpha = 1 - age / 30
      ctx.fillStyle = C.zzz
      ctx.fillRect(x, y, 3, 1)
      ctx.fillRect(x + 1, y + 1, 1, 1)
      ctx.fillRect(x, y + 2, 3, 1)
    }
    ctx.globalAlpha = 1
  }

  function drawHeart(x: number, y: number) {
    ctx.fillStyle = C.heart
    ctx.fillRect(x, y, 2, 2)
    ctx.fillRect(x + 3, y, 2, 2)
    ctx.fillRect(x, y + 1, 5, 2)
    ctx.fillRect(x + 1, y + 3, 3, 1)
    ctx.fillRect(x + 2, y + 4, 1, 1)
  }

  function tick() {
    step(cat)
    hearts = hearts.filter((h) => --h.life > 0)
    for (const h of hearts) h.y -= 0.5
    status.textContent = LABELS[cat.mode]
    for (const b of commands) b.setAttribute('aria-pressed', String(b.dataset.cat === cat.mode))
    draw()
  }

  function scenePoint(e: MouseEvent) {
    const r = canvas.getBoundingClientRect()
    return { x: ((e.clientX - r.left) / r.width) * SCENE_W, y: ((e.clientY - r.top) / r.height) * SCENE_H }
  }

  canvas.addEventListener('click', (e) => {
    const p = scenePoint(e)
    if (!hitCat(cat, p.x, p.y)) return
    poke(cat)
    hearts.push({ x: cat.x + SPR_W / 2 - 2, y: GROUND - SPR_H - cat.lift - 4, life: 14 })
    draw()
  })
  canvas.addEventListener('dblclick', (e) => {
    const p = scenePoint(e)
    if (hitCat(cat, p.x, p.y)) leave()
  })
  canvas.addEventListener('mousemove', (e) => {
    const p = scenePoint(e)
    canvas.style.cursor = hitCat(cat, p.x, p.y) ? 'pointer' : ''
  })
  for (const b of commands) {
    b.addEventListener('click', () => {
      command(cat, b.dataset.cat as Command)
      tick()
    })
  }
  // Пока кот на экране, горячие клавиши поиска не срабатывают.
  document.addEventListener('keydown', (e) => {
    if (root.hidden) return
    if (e.key === 'Escape') leave()
    if (!(e.target instanceof HTMLButtonElement && root.contains(e.target))) e.preventDefault()
    e.stopImmediatePropagation()
  }, true)
  window.addEventListener('resize', () => { if (!root.hidden) fit() })

  async function enter() {
    if (!root.hidden) return
    try {
      const max = await WindowIsMaximised()
      const size = await WindowGetSize()
      saved = { w: size.w, h: size.h, max }
      if (max) WindowUnmaximise()
      WindowSetMinSize(320, 260)
      WindowSetSize(CAT_WIDTH, CAT_HEIGHT)
      WindowSetAlwaysOnTop(true)
      WindowSetTitle('Рыжик')
    } catch {
      saved = null // в браузере (vite dev без Wails) окна нет — просто показываем кота
    }
    cat = newCat()
    hearts = []
    root.hidden = false
    document.body.classList.add('cat-on')
    root.classList.remove('poof')
    void root.offsetWidth
    root.classList.add('poof')
    fit()
    tick()
    timer = window.setInterval(tick, 100)
  }

  function leave() {
    if (root.hidden) return
    window.clearInterval(timer)
    root.hidden = true
    document.body.classList.remove('cat-on')
    try {
      WindowSetAlwaysOnTop(false)
      WindowSetTitle(MAIN_TITLE)
      WindowSetMinSize(MAIN_MIN.w, MAIN_MIN.h)
      if (saved) {
        WindowSetSize(Math.max(saved.w, MAIN_MIN.w), Math.max(saved.h, MAIN_MIN.h))
        if (saved.max) WindowMaximise()
      }
    } catch { /* окна нет — нечего восстанавливать */ }
    button.focus()
  }

  button.addEventListener('click', () => void enter())
}
