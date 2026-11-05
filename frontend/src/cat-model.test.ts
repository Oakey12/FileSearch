import { describe, expect, it } from 'vitest'
import { GROUND, SCENE_W, SPR_W, command, hitCat, newCat, poke, sprite, step, type Cat } from './cat-model'

const run = (cat: Cat, n: number) => { for (let i = 0; i < n; i++) step(cat, () => 0.5) }

describe('рыжий кот', () => {
  it('сидит, потом умывается, потом засыпает сам', () => {
    const cat = newCat()
    expect(cat.mode).toBe('sit')
    run(cat, 60)
    expect(cat.mode).toBe('wash')
    run(cat, 40)
    expect(cat.mode).toBe('sit')
    run(cat, 60)
    expect(cat.mode).toBe('sleep')
    run(cat, 500)
    expect(cat.mode).toBe('sleep')
  })

  it('клик будит спящего кота', () => {
    const cat = newCat()
    run(cat, 200)
    poke(cat)
    expect(cat.mode).toBe('sit')
  })

  it('гуляет и разворачивается у края, не выходя со сцены', () => {
    const cat = newCat()
    command(cat, 'walk')
    const facings = new Set<number>()
    for (let i = 0; i < 150; i++) {
      step(cat)
      facings.add(cat.facing)
      expect(cat.x).toBeGreaterThanOrEqual(0)
      expect(cat.x + SPR_W).toBeLessThanOrEqual(SCENE_W)
    }
    expect(facings.size).toBe(2)
  })

  it('играет: подкрадывается, прыгает и пинает клубок', () => {
    const cat = newCat()
    command(cat, 'play', () => 0.5)
    expect(cat.ball).not.toBeNull()
    const phases = new Set<string>()
    let jumped = false
    for (let i = 0; i < 120; i++) {
      step(cat, () => 0.5)
      phases.add(cat.phase)
      if (cat.lift > 0) jumped = true
      expect(cat.ball!.x).toBeGreaterThanOrEqual(0)
      expect(cat.ball!.x).toBeLessThanOrEqual(SCENE_W)
    }
    expect(jumped).toBe(true)
    expect([...phases]).toEqual(expect.arrayContaining(['chase', 'stalk', 'leap', 'watch']))
  })

  it('команда сбрасывает игру и убирает клубок', () => {
    const cat = newCat()
    command(cat, 'play')
    command(cat, 'sit')
    expect(cat.ball).toBeNull()
    expect(cat.mode).toBe('sit')
  })

  it('попадание по коту', () => {
    const cat = newCat()
    expect(hitCat(cat, cat.x + SPR_W / 2, GROUND - 5)).toBe(true)
    expect(hitCat(cat, cat.x - 20, GROUND - 5)).toBe(false)
  })

  it('у каждой анимации есть поза в пределах спрайта', () => {
    const cat = newCat()
    const seen = new Set<string>()
    const check = () => {
      const s = sprite(cat)
      for (const p of [...s.layers.flat(), ...s.details]) {
        expect(p[0]).toBeGreaterThanOrEqual(-1)
        expect(p[0] + p[2]).toBeLessThanOrEqual(SPR_W)
        expect(p[1] + p[3]).toBeLessThanOrEqual(24)
      }
      seen.add(cat.mode)
    }
    for (let i = 0; i < 200; i++) { step(cat); check() }
    command(cat, 'walk'); for (let i = 0; i < 20; i++) { step(cat); check() }
    command(cat, 'play'); for (let i = 0; i < 60; i++) { step(cat, () => 0.5); check() }
    expect(seen).toEqual(new Set(['sit', 'wash', 'sleep', 'walk', 'play']))
  })
})
