import { describe, expect, it } from 'vitest'
import { BALL, SPR_H, SPR_W, carry, command, drop, hitCat, newCat, poke, sprite, step, type Cat } from './cat-model'

const SCENE_W = 400

const run = (cat: Cat, n: number) => { for (let i = 0; i < n; i++) step(cat, () => 0.5) }

describe('рыжий кот', () => {
  it('сидит, зевает, умывается, потом засыпает сам', () => {
    const cat = newCat(SCENE_W)
    expect(cat.mode).toBe('sit')
    run(cat, 40)
    expect(cat.mode).toBe('yawn')
    run(cat, 16)
    expect(cat.mode).toBe('sit')
    run(cat, 34)
    expect(cat.mode).toBe('wash')
    run(cat, 40)
    expect(cat.mode).toBe('sit')
    run(cat, 40)
    expect(cat.mode).toBe('sleep')
    run(cat, 500)
    expect(cat.mode).toBe('sleep')
  })

  it('клик будит спящего кота: он потягивается и садится', () => {
    const cat = newCat(SCENE_W)
    run(cat, 200)
    poke(cat)
    expect(cat.mode).toBe('stretch')
    run(cat, 22)
    expect(cat.mode).toBe('sit')
  })

  it('кота несут за шкирку, отпускают — он падает, приземляется и садится', () => {
    const cat = newCat(SCENE_W)
    command(cat, 'play')
    carry(cat)
    expect(cat.mode).toBe('carried')
    expect(cat.ball).toBeNull()
    run(cat, 50)
    expect(cat.mode).toBe('carried')
    drop(cat, 17)
    const seen = new Set<string>()
    for (let i = 0; i < 20; i++) { step(cat); seen.add(cat.mode); expect(cat.lift).toBeGreaterThanOrEqual(0) }
    expect([...seen]).toEqual(['fall', 'land', 'sit'])
  })

  it('гуляет и разворачивается у края, не выходя со сцены', () => {
    const cat = newCat(SCENE_W)
    command(cat, 'walk')
    const facings = new Set<number>()
    for (let i = 0; i < 290; i++) {
      step(cat)
      facings.add(cat.facing)
      expect(cat.x).toBeGreaterThanOrEqual(0)
      expect(cat.x + SPR_W).toBeLessThanOrEqual(SCENE_W)
    }
    expect(facings.size).toBe(2)
  })

  it('играет: подкрадывается, прыгает и пинает клубок', () => {
    const cat = newCat(SCENE_W)
    command(cat, 'play', () => 0.5)
    expect(cat.ball).not.toBeNull()
    const phases = new Set<string>()
    let jumped = false
    for (let i = 0; i < 120; i++) {
      step(cat, () => 0.5)
      phases.add(cat.phase)
      if (cat.lift > 0) jumped = true
      expect(cat.ball!.x).toBeGreaterThanOrEqual(0)
      expect(cat.ball!.x).toBeLessThanOrEqual(SCENE_W - BALL)
      // клубок не укатывается далеко: кот и клубок помещаются в окно кота (100 пикселей)
      expect(Math.abs(cat.ball!.x - cat.x)).toBeLessThan(100 - BALL)
    }
    expect(jumped).toBe(true)
    expect([...phases]).toEqual(expect.arrayContaining(['chase', 'stalk', 'leap', 'watch']))
  })

  it('команда сбрасывает игру и убирает клубок', () => {
    const cat = newCat(SCENE_W)
    command(cat, 'play')
    command(cat, 'sit')
    expect(cat.ball).toBeNull()
    expect(cat.mode).toBe('sit')
  })

  it('попадание по коту', () => {
    const cat = newCat(SCENE_W)
    expect(hitCat(cat, cat.x + SPR_W / 2, -5)).toBe(true)
    expect(hitCat(cat, cat.x - 20, -5)).toBe(false)
  })

  it('у каждой анимации есть поза, стоящие позы — в пределах спрайта, шаг — полклетки', () => {
    const cat = newCat(SCENE_W)
    const seen = new Set<string>()
    const check = () => {
      const s = sprite(cat)
      for (const p of [...s.layers.flat(), ...s.details]) {
        for (const v of p.slice(0, 4) as number[]) expect(Number.isInteger(v * 2)).toBe(true)
        if (cat.mode === 'carried') continue // висит под курсором, ниже пола позы
        expect(p[0]).toBeGreaterThanOrEqual(-3) // усы могут торчать за контур
        expect(p[0] + p[2]).toBeLessThanOrEqual(SPR_W + 3)
        expect(p[1] + p[3]).toBeLessThanOrEqual(SPR_H)
      }
      seen.add(cat.mode)
    }
    for (let i = 0; i < 200; i++) { step(cat); check() }
    poke(cat); for (let i = 0; i < 25; i++) { step(cat); check() }
    command(cat, 'walk'); for (let i = 0; i < 20; i++) { step(cat); check() }
    command(cat, 'play'); for (let i = 0; i < 60; i++) { step(cat, () => 0.5); check() }
    carry(cat); for (let i = 0; i < 8; i++) { cat.sway = (i % 5) - 2; step(cat); check() }
    drop(cat, 17); for (let i = 0; i < 12; i++) { step(cat); check() }
    expect(seen).toEqual(new Set(['sit', 'yawn', 'wash', 'sleep', 'stretch', 'walk', 'play', 'carried', 'fall', 'land']))
  })
})
