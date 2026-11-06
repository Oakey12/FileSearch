package cat

import (
	"image/color"
	"math"
)

var (
	cOut       = nrgb(0x3a, 0x1a, 0x08)
	cWhite     = nrgb(0xff, 0xff, 0xff)
	cNose      = nrgb(0xff, 0x8f, 0xa0)
	cMouth     = nrgb(0x5a, 0x1a, 0x1a)
	cTongue    = nrgb(0xff, 0x8a, 0x9a)
	cWhisker   = nrgb(0xff, 0xf6, 0xe8)
	cPupil     = nrgb(0x14, 0x11, 0x0f)
	cFist      = nrgb(0xf3, 0xd2, 0xb3)
	cCreamBrow = nrgb(0xff, 0xf0, 0xd0)
	eyeT       = tones{nrgb(0xb4, 0xee, 0x7c), nrgb(0x86, 0xd6, 0x4f), nrgb(0x3f, 0x8f, 0x2a)}
	collarT    = tones{nrgb(0x5a, 0x9c, 0xf0), nrgb(0x2f, 0x6f, 0xd6), nrgb(0x1f, 0x4f, 0xa8)}
	bellT      = tones{nrgb(0xff, 0xf0, 0x9a), nrgb(0xff, 0xcc, 0x33), nrgb(0xd9, 0xa0, 0x1a)}
)

type eyeKind int

const (
	eyesOpen eyeKind = iota
	eyesClosed
	eyesHappy
	eyesWide
	eyesSleepy
)

// tailP — как лежит хвост: изгиб и покачивание.
type tailP struct {
	a0    float64 // направление у основания (радианы, 0 — вправо, −π/2 — вверх)
	curl  float64 // сколько он загибается к кончику
	amp   float64 // размах покачивания
	phase float64
	n     int
	len   float64
}

func (t tailP) build(bx, by float64) chain {
	n := t.n
	if n == 0 {
		n = 7
	}
	l := t.len
	if l == 0 {
		l = 4.6
	}
	var ch chain
	x, y := bx, by
	for i := 0; i < n; i++ {
		f := float64(i) / float64(n)
		a := t.a0 + t.curl*f + t.amp*math.Sin(t.phase-float64(i)*0.55)*f
		nx, ny := x+math.Cos(a)*l, y+math.Sin(a)*l
		ch = append(ch, capsule{x, y, nx, ny, 5.0 - 1.5*f, 5.0 - 1.5*(f+1/float64(n))})
		x, y = nx, ny
	}
	return ch
}

// drawTail рисует пушистый хвост с тёмными кольцами и белым кончиком.
func (p *painter) drawTail(t tailP, bx, by float64, far bool) {
	ch := t.build(bx, by)
	pal := furT
	cream := creamT
	if far {
		pal = furFarT
	}
	n := len(ch)
	p.draw(ch, pal, opts{over: func(x, y float64, tone int) (color.NRGBA, bool) {
		i := ch.idx(x, y)
		if i >= n-3 {
			return cream[tone], true
		}
		if i >= 0 && i%3 == 1 {
			return stripeT[tone], true
		}
		return color.NRGBA{}, false
	}})
}

// ---------- глаза ----------

func (p *painter) eye(ex, ey float64, kind eyeKind, gx, gy float64) {
	switch kind {
	case eyesClosed:
		for i, dy := range []float64{0, 1, 1.4, 1.4, 1.4, 1, 0} {
			p.dot(ex-3+float64(i), ey+dy, cOut)
		}
		return
	case eyesHappy:
		for i, dy := range []float64{1, 0, -0.6, -0.6, -0.6, 0, 1} {
			p.dot(ex-3+float64(i), ey+dy+1, cOut)
		}
		return
	}
	rx, ry, pr := 4.0, 4.8, 2.3
	switch kind {
	case eyesWide:
		rx, ry, pr = 4.3, 5.2, 3.0
	case eyesSleepy:
		ry = 3
	}
	p.draw(ellipse{ex, ey, rx, ry, 0}, eyeT, opts{})
	p.draw(ellipse{ex + gx*1.3, ey + gy*1.1, pr, math.Min(ry-0.5, pr*1.35), 0}, tones{cPupil, cPupil, cPupil}, opts{noOutline: true})
	if kind != eyesSleepy {
		// два блика: крупный сверху слева и маленький снизу справа
		p.dot(ex-2+gx*0.6, ey-2.8, cWhite)
		p.dot(ex-1+gx*0.6, ey-2.8, cWhite)
		p.dot(ex-2+gx*0.6, ey-1.8, cWhite)
		p.dot(ex-1+gx*0.6, ey-1.8, cWhite)
		p.dot(ex+1.4+gx*0.6, ey+2, eyeT[0])
	} else {
		for dx := -rx; dx <= rx; dx++ { // тяжёлое веко
			p.dot(ex+dx, ey-ry+1, cOut)
		}
	}
}

// ---------- вид спереди: сидит, умывается, зевает, висит ----------

type frontP struct {
	breath     float64 // −1…1
	gaze       float64 // куда смотрит: −1 влево … 1 вправо
	gazeY      float64 // −1 вверх … 1 вниз
	headDown   float64 // пикселей вниз
	tilt       float64 // наклон головы вбок, пикселей смещения макушки
	eyes       eyeKind
	mouth      float64
	pawUp      float64 // 0…1 лапка поднята к мордочке
	pawWiggle  float64
	earL, earR float64 // подёргивание ушей, пикселей
	earsBack   float64
	tail       tailP
	tongue     bool
	hang       bool // кота несут за шкирку
	hangSway   float64
	hangStep   int
}

// collar — синий ошейник с золотым колокольчиком под головой.
func (p *painter) collar(cx, cy, rx float64, bell bool) {
	p.draw(ellipse{cx, cy, rx, 3.2, 0}, collarT, opts{})
	if bell {
		p.draw(ellipse{cx, cy + 3.6, 2.7, 2.7, 0}, bellT, opts{})
		p.dot(cx-0.5, cy+3.4, cOut)
		p.dot(cx-1, cy+2.4, cWhite)
	}
}

// frontHead — пухлая голова анфас: щёчки, уши, полоски, глаза, мордочка, усы.
func (p *painter) frontHead(hx, hy float64, f frontP, eyes eyeKind) {
	ew := f.earsBack
	// щёчки-бакенбарды
	p.draw(tri{hx - 16, hy + 1, hx - 22, hy + 9, hx - 12, hy + 10}, furT, opts{})
	p.draw(tri{hx + 16, hy + 1, hx + 22, hy + 9, hx + 12, hy + 10}, furT, opts{})
	// уши
	p.draw(tri{hx - 15.5, hy - 5, hx - 17 + f.earL - ew, hy - 21 + ew*2, hx - 3, hy - 11}, furT, opts{})
	p.draw(tri{hx + 15.5, hy - 5, hx + 17 + f.earR + ew, hy - 21 + ew*2, hx + 3, hy - 11}, furT, opts{})
	p.draw(tri{hx - 13.2, hy - 7.5, hx - 14.4 + f.earL - ew, hy - 17 + ew*2, hx - 6, hy - 11}, earT, opts{noOutline: true})
	p.draw(tri{hx + 13.2, hy - 7.5, hx + 14.4 + f.earR + ew, hy - 17 + ew*2, hx + 6, hy - 11}, earT, opts{noOutline: true})
	// голова: полоски на лбу и щеках
	head := ellipse{hx, hy, 17, 13.5, 0}
	p.draw(head, furT, opts{over: func(x, y float64, tone int) (color.NRGBA, bool) {
		u, v := head.uv(x, y)
		if v < -0.35 && v > -0.97 {
			if math.Abs(u) < 0.07 || math.Abs(math.Abs(u)-0.27) < 0.065 || (math.Abs(math.Abs(u)-0.52) < 0.06 && v > -0.8) {
				return stripeT[tone], true
			}
		}
		if math.Abs(u) > 0.84 && v > -0.2 && v < 0.5 && math.Mod(v*7+10, 1) < 0.45 { // полоски на щеках
			return stripeT[tone], true
		}
		return color.NRGBA{}, false
	}})
	// светлые «брови» над глазами и белая мордочка
	for _, sx := range []float64{-1, 1} {
		p.line(hx+sx*10, hy-5, hx+sx*7.5, hy-5, cCreamBrow)
	}
	p.draw(ellipse{hx, hy + 7.5, 8, 5.4, 0}, creamT, opts{noOutline: true})
	// глаза
	p.eye(hx-8, hy+0.5, eyes, f.gaze, f.gazeY)
	p.eye(hx+8, hy+0.5, eyes, f.gaze, f.gazeY)
	// носик и ротик «w»
	p.draw(tri{hx - 2.2, hy + 4, hx + 2.2, hy + 4, hx, hy + 6.6}, tones{cNose, cNose, cNose}, opts{noOutline: true})
	if f.mouth > 0.05 {
		o := 1 + f.mouth*4.5
		p.draw(ellipse{hx, hy + 9 + o*0.35, 3.8, o, 0}, tones{cMouth, cMouth, cMouth}, opts{noOutline: true})
		if f.mouth > 0.4 {
			p.draw(ellipse{hx, hy + 9 + o*0.7, 2.4, o * 0.45, 0}, tones{cTongue, cTongue, cTongue}, opts{noOutline: true})
		}
		p.dot(hx-2.5, hy+8, cWhite) // клычки
		p.dot(hx+1.5, hy+8, cWhite)
	} else {
		p.line(hx, hy+6.6, hx, hy+8.2, cOut)
		p.line(hx, hy+8.2, hx-3, hy+9.6, cOut)
		p.line(hx, hy+8.2, hx+3, hy+9.6, cOut)
		p.dot(hx-4, hy+9, cOut)
		p.dot(hx+3, hy+9, cOut)
		if f.tongue {
			p.dot(hx, hy+10, cTongue)
			p.dot(hx, hy+11, cTongue)
		}
	}
	// усы
	for i := 0; i < 3; i++ {
		fi := float64(i)
		p.line(hx-9, hy+7+fi*1.2, hx-19, hy+5+fi*2.7, cWhisker)
		p.line(hx+9, hy+7+fi*1.2, hx+19, hy+5+fi*2.7, cWhisker)
	}
}

func (p *painter) drawFront(f frontP) {
	br := f.breath
	if f.hang {
		p.drawHanging(f)
		return
	}
	// хвост лежит за спиной справа
	p.drawTail(f.tail, 48, 47, true)

	// задние лапы-бёдра
	p.draw(ellipse{16, 44, 9, 8, 0}, furT, opts{over: socks(47)})
	p.draw(ellipse{48, 44, 9, 8, 0}, furT, opts{over: socks(47)})
	// туловище: большая белая грудка, полоски на боках
	body := ellipse{32, 39, 13.5, 13.5 + br*0.6, 0}
	p.draw(body, furT, opts{over: func(x, y float64, tone int) (color.NRGBA, bool) {
		u, v := body.uv(x, y)
		if math.Abs(u) < 0.5 && v > -0.55 {
			return creamT[tone], true
		}
		if math.Abs(u) > 0.62 && math.Mod(v*4+8, 1) < 0.45 {
			return stripeT[tone], true
		}
		return color.NRGBA{}, false
	}})
	// передние лапы
	p.frontPaw(25, 39, 25, 49.5, creamT)
	if f.pawUp > 0 {
		k := f.pawUp
		fx := 39 - 4*k
		fy := 49.5 - 24*k + math.Sin(f.pawWiggle)*1.5*k
		p.frontPaw(39, 39, fx, fy, creamT)
	} else {
		p.frontPaw(39, 39, 39, 49.5, creamT)
	}

	hx := 32 + f.gaze*3 + f.tilt*0.5
	hy := 16 + f.headDown + f.gazeY*1.2 + br*0.3
	// ошейник под головой
	p.collar(32+f.gaze*1.5, hy+14, 10.5, true)
	p.frontHead(hx, hy, f, f.eyes)
}

func socks(from float64) func(x, y float64, tone int) (color.NRGBA, bool) {
	return func(x, y float64, tone int) (color.NRGBA, bool) {
		if y > from {
			return creamT[tone], true
		}
		return color.NRGBA{}, false
	}
}

// frontPaw — передняя лапа сверху вниз с белой «варежкой».
func (p *painter) frontPaw(ax, ay, bx, by float64, pal tones) {
	leg := capsule{ax, ay, bx, by, 4.4, 3.8}
	p.draw(leg, furT, opts{over: func(x, y float64, tone int) (color.NRGBA, bool) {
		t, _ := leg.at(x, y)
		if t > 0.58 {
			return pal[tone], true
		}
		if t > 0.25 && math.Mod(y*0.9, 3) < 1 {
			return stripeT[tone], true
		}
		return color.NRGBA{}, false
	}})
	p.draw(ellipse{bx, by + 0.8, 4.8, 2.6, 0}, pal, opts{})
	// пальчики
	p.dot(bx-1.5, by+1.6, creamT[2])
	p.dot(bx+0.5, by+1.6, creamT[2])
}

// drawHanging — кота несут за шкирку: висит, лапки и хвост качаются.
func (p *painter) drawHanging(f frontP) {
	s := f.hangSway
	sw := []float64{0, 1, 0, -1}[f.hangStep%4]
	tail := tailP{a0: math.Pi/2 - s*0.25, curl: 0.4, amp: 0.25, phase: float64(f.hangStep) * 0.8, n: 8, len: 4.6}
	p.drawTail(tail, 32+s*2, 42, true)
	p.draw(capsule{26 + s*1.5, 42, 25 + s*3 + sw*0.5, 53, 4.3, 3.6}, furT, opts{})
	p.draw(capsule{38 + s*1.5, 42, 39 + s*3 - sw*0.5, 53, 4.3, 3.6}, furT, opts{})
	p.draw(ellipse{25 + s*3 + sw*0.5, 54, 4.3, 2.4, 0}, creamT, opts{})
	p.draw(ellipse{39 + s*3 - sw*0.5, 54, 4.3, 2.4, 0}, creamT, opts{})
	body := ellipse{32 + s, 34, 11.5, 14, 0}
	p.draw(body, furT, opts{over: func(x, y float64, tone int) (color.NRGBA, bool) {
		u, v := body.uv(x, y)
		if math.Abs(u) < 0.55 && v > -0.3 {
			return creamT[tone], true
		}
		return color.NRGBA{}, false
	}})
	p.draw(capsule{24 + s*0.5, 28, 23 + s, 39, 3.8, 3.4}, furT, opts{})
	p.draw(capsule{40 + s*0.5, 28, 41 + s, 39, 3.8, 3.4}, furT, opts{})
	p.draw(ellipse{23 + s, 40, 4, 2.4, 0}, creamT, opts{})
	p.draw(ellipse{41 + s, 40, 4, 2.4, 0}, creamT, opts{})
	hx, hy := 32.0+s*0.3, 14.0
	p.collar(32+s*0.3, hy+14, 10, true)
	p.frontHead(hx, hy, frontP{}, eyesSleepy)
}

// ---------- вид сбоку ----------

// foot — куда ставится стопа (локальные координаты позы).
type foot struct{ X, Y float64 }

type sideP struct {
	bodyY, pitch   float64
	hindN, hindF   foot // задние: ближняя и дальняя к зрителю
	frontN, frontF foot
	headDX, headDY float64
	tilt           float64
	eyes           eyeKind
	gx, gy         float64
	mouth          float64
	tail           tailP
	earsBack       float64
	tongue         bool
}

const groundFoot = SprH - 2.2 // центр стопы, когда она стоит на полу

func rot(cx, cy, x, y, a float64) (float64, float64) {
	s, c := math.Sincos(a)
	dx, dy := x-cx, y-cy
	return cx + dx*c - dy*s, cy + dx*s + dy*c
}

func (p *painter) leg(jx, jy float64, f foot, l1, l2, bend float64, far, hind bool) {
	fur, cream := furT, creamT
	if far {
		fur, cream = furFarT, creamFarT
	}
	kx, ky, fx, fy := ik(jx, jy, f.X, f.Y, l1, l2, bend)
	if hind {
		p.draw(capsule{jx, jy, kx, ky, 5.8, 4.2}, fur, opts{})
	} else {
		p.draw(capsule{jx, jy, kx, ky, 4.6, 3.7}, fur, opts{})
	}
	shin := capsule{kx, ky, fx, fy, 3.7, 3.1}
	p.draw(shin, fur, opts{over: func(x, y float64, tone int) (color.NRGBA, bool) {
		t, _ := shin.at(x, y)
		if t > 0.62 {
			return cream[tone], true
		}
		return color.NRGBA{}, false
	}})
	p.draw(ellipse{fx + 1.2, fy + 0.9, 4, 2.3, 0}, cream, opts{})
}

func (p *painter) drawSide(s sideP) {
	mid := [2]float64{27, 30 + s.bodyY}
	hip := func(dx, dy float64) (float64, float64) {
		x, y := rot(mid[0], mid[1], mid[0]-13+dx, mid[1]+dy, s.pitch)
		return x, y
	}
	sh := func(dx, dy float64) (float64, float64) {
		x, y := rot(mid[0], mid[1], mid[0]+13+dx, mid[1]+dy, s.pitch)
		return x, y
	}
	hjx, hjy := hip(1, 3.5)
	sjx, sjy := sh(0, 3.5)

	// дальние лапы и хвост — за туловищем
	tx, ty := hip(-4, -4)
	p.drawTail(s.tail, tx, ty, true)
	p.leg(hjx+1, hjy, s.hindF, 10, 9.5, -1, true, true)
	p.leg(sjx, sjy, s.frontF, 10, 9.5, 1, true, false)

	// туловище: широкие полоски по спине, белое брюшко и грудка
	torso := ellipse{mid[0], mid[1], 18, 10.5, s.pitch}
	p.draw(torso, furT, opts{over: func(x, y float64, tone int) (color.NRGBA, bool) {
		u, v := torso.uv(x, y)
		if v > 0.55 || (u > 0.5 && v > 0.0) {
			return creamT[tone], true
		}
		if v < 0.4 && math.Mod(u*18/5+20, 1) < 0.5 {
			return stripeT[tone], true
		}
		return color.NRGBA{}, false
	}})

	// ближние лапы
	p.leg(hjx, hjy, s.hindN, 10, 9.5, -1, false, true)
	p.leg(sjx, sjy, s.frontN, 10, 9.5, 1, false, false)

	// голова
	nx, ny := sh(10+s.headDX, -8+s.headDY)
	hp := func(dx, dy float64) (float64, float64) { x, y := rot(0, 0, dx, dy, s.tilt); return nx + x, ny + y }
	// ошейник с колокольчиком — на шее, под головой
	c1x, c1y := hp(-7.5, 7)
	c2x, c2y := hp(0, 10.6)
	p.draw(capsule{c1x, c1y, c2x, c2y, 3.3, 3.3}, collarT, opts{})
	bx, by := hp(-3.5, 13.6)
	p.draw(ellipse{bx, by, 2.9, 2.9, 0}, bellT, opts{})
	p.dot(bx-1, by-1, cWhite)

	ew := s.earsBack
	// уши
	ax, ay := hp(-8, -6.5)
	bx2, by2 := hp(-8.5-ew*2, -18+ew*3)
	cx, cy := hp(1.5, -9.5)
	p.draw(tri{ax, ay, bx2, by2, cx, cy}, furT, opts{})
	dx, dy := hp(2.5, -9.5)
	ex, ey := hp(8.5+ew, -17.5+ew*3)
	fx, fy := hp(11.5, -4.5)
	p.draw(tri{dx, dy, ex, ey, fx, fy}, furT, opts{})
	ix, iy := hp(-6.6, -8)
	jx, jy := hp(-7-ew*2, -14.6+ew*2.4)
	kx, ky := hp(0.2, -9.6)
	p.draw(tri{ix, iy, jx, jy, kx, ky}, earT, opts{noOutline: true})
	ix, iy = hp(4, -9.2)
	jx, jy = hp(8+ew, -14.4+ew*2.4)
	kx, ky = hp(10, -5.6)
	p.draw(tri{ix, iy, jx, jy, kx, ky}, earT, opts{noOutline: true})
	// череп и мордочка
	head := ellipse{nx, ny, 12.5, 10.5, s.tilt}
	p.draw(head, furT, opts{over: func(x, y float64, tone int) (color.NRGBA, bool) {
		u, v := head.uv(x, y)
		if v < -0.3 && v > -0.92 && (math.Abs(u+0.25) < 0.08 || math.Abs(u-0.1) < 0.08 || math.Abs(u+0.55) < 0.07) {
			return stripeT[tone], true
		}
		if u < -0.4 && v > -0.2 && v < 0.5 && math.Mod(v*7+10, 1) < 0.45 { // полоски на щеке
			return stripeT[tone], true
		}
		return color.NRGBA{}, false
	}})
	mx, my := hp(10, 4)
	p.draw(ellipse{mx, my, 6.6, 5, s.tilt}, creamT, opts{})
	// глаз, нос, рот, усы
	exx, eyy := hp(5.6, -1)
	p.eye(exx, eyy, s.eyes, s.gx, s.gy)
	bx3, by3 := hp(4, -6.4)
	p.dot(bx3, by3, cCreamBrow)
	p.dot(bx3+1, by3, cCreamBrow)
	nxp, nyp := hp(15.8, 2.4)
	p.draw(ellipse{nxp, nyp, 2, 1.5, 0}, tones{cNose, cNose, cNose}, opts{noOutline: true})
	if s.mouth > 0.05 {
		mox, moy := hp(12.5, 7.4)
		o := 0.9 + s.mouth*3.4
		p.draw(ellipse{mox, moy + o*0.3, 3.6, o, 0}, tones{cMouth, cMouth, cMouth}, opts{noOutline: true})
		if s.mouth > 0.4 {
			tx2, ty2 := hp(11.5, 8+o*0.5)
			p.draw(ellipse{tx2, ty2, 2.1, o * 0.45, 0}, tones{cTongue, cTongue, cTongue}, opts{noOutline: true})
		}
	} else {
		a1x, a1y := hp(15, 4.6)
		a2x, a2y := hp(13.4, 7)
		a3x, a3y := hp(10, 7.2)
		p.line(a1x, a1y, a2x, a2y, cOut)
		p.line(a2x, a2y, a3x, a3y, cOut)
		if s.tongue {
			p.dot(a3x, a3y+1, cTongue)
		}
	}
	for i := 0; i < 2; i++ {
		wx, wy := hp(12.4, 5.2+float64(i)*0.8)
		w2x, w2y := hp(22, 3+float64(i)*3.6)
		p.line(wx, wy, w2x, w2y, cWhisker)
	}
}

// ---------- сон ----------

// drawSleep — свернулся клубком, голова на лапах, хвост обвивает.
func (p *painter) drawSleep(breath float64) {
	b := breath * 0.6
	tail := chain{
		{12, 47, 22, 49.5, 4, 3.8}, {22, 49.5, 34, 50, 3.8, 3.7}, {34, 50, 46, 49.4, 3.7, 3.4}, {46, 49.4, 54, 47.2, 3.4, 3.0},
	}
	p.draw(tail, furT, opts{over: func(x, y float64, tone int) (color.NRGBA, bool) {
		if tail.idx(x, y) >= 0 {
			if x > 47 {
				return creamT[tone], true
			}
			if math.Mod(x/4.5, 1) < 0.45 {
				return stripeT[tone], true
			}
		}
		return color.NRGBA{}, false
	}})
	body := ellipse{29, 40 - b, 25, 11.5 + b, 0}
	p.draw(body, furT, opts{over: func(x, y float64, tone int) (color.NRGBA, bool) {
		u, v := body.uv(x, y)
		if v > 0.62 {
			return creamT[tone], true
		}
		if v < 0.45 && math.Mod(u*25/5+30, 1) < 0.5 {
			return stripeT[tone], true
		}
		return color.NRGBA{}, false
	}})
	p.draw(ellipse{17, 41 - b, 11, 9.5 + b, 0}, furT, opts{over: func(x, y float64, tone int) (color.NRGBA, bool) {
		if math.Mod(y/4, 1) < 0.4 && y < 44 {
			return stripeT[tone], true
		}
		return color.NRGBA{}, false
	}})
	p.draw(ellipse{50, 47.5, 8.5, 3.4, 0}, furT, opts{over: func(x, y float64, tone int) (color.NRGBA, bool) {
		if x > 51 {
			return creamT[tone], true
		}
		return color.NRGBA{}, false
	}})
	hx, hy := 48.0, 38.0-b*0.4
	// ошейник
	p.draw(capsule{hx - 9, hy + 3, hx - 7, hy + 9, 2.6, 2.6}, collarT, opts{})
	p.draw(ellipse{hx - 6, hy + 11, 2.3, 2.3, 0}, bellT, opts{})
	p.draw(tri{hx - 8, hy - 6, hx - 8.5, hy - 17, hx + 0.5, hy - 9}, furT, opts{})
	p.draw(tri{hx + 1.5, hy - 9, hx + 7.5, hy - 16.5, hx + 11, hy - 5}, furT, opts{})
	p.draw(tri{hx - 6.5, hy - 7.5, hx - 7, hy - 13.5, hx - 0.5, hy - 9}, earT, opts{noOutline: true})
	p.draw(tri{hx + 2.5, hy - 9, hx + 7, hy - 13.5, hx + 9.4, hy - 6}, earT, opts{noOutline: true})
	head := ellipse{hx, hy, 12, 10, 0}
	p.draw(head, furT, opts{over: func(x, y float64, tone int) (color.NRGBA, bool) {
		u, v := head.uv(x, y)
		if v < -0.3 && v > -0.9 && (math.Abs(u+0.2) < 0.08 || math.Abs(u-0.15) < 0.08) {
			return stripeT[tone], true
		}
		return color.NRGBA{}, false
	}})
	p.draw(ellipse{hx + 5, hy + 4.6, 7, 4.8, 0}, creamT, opts{noOutline: true})
	p.eye(hx-3, hy+0.5, eyesClosed, 0, 0)
	p.eye(hx+6, hy+0.5, eyesClosed, 0, 0)
	p.draw(ellipse{hx + 9, hy + 3.6, 1.9, 1.4, 0}, tones{cNose, cNose, cNose}, opts{noOutline: true})
	p.line(hx+7.2, hy+6.6, hx+10, hy+6.6, cOut)
}
