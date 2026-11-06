package cat

import (
	"image/color"
	"math"
)

var (
	cOut     = nrgb(0x2b, 0x14, 0x08)
	cWhite   = nrgb(0xff, 0xff, 0xff)
	cNose    = nrgb(0xff, 0x8f, 0xa0)
	cMouth   = nrgb(0x5a, 0x1a, 0x1a)
	cTongue  = nrgb(0xff, 0x8a, 0x9a)
	cWhisker = nrgb(0xff, 0xf6, 0xe8)
	cPupil   = nrgb(0x14, 0x11, 0x0f)
	cFist    = nrgb(0xf3, 0xd2, 0xb3)
	eyeT     = tones{nrgb(0xb4, 0xee, 0x7c), nrgb(0x86, 0xd6, 0x4f), nrgb(0x3f, 0x8f, 0x2a)}
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
		n = 8
	}
	l := t.len
	if l == 0 {
		l = 4.4
	}
	var ch chain
	x, y := bx, by
	for i := 0; i < n; i++ {
		f := float64(i) / float64(n)
		a := t.a0 + t.curl*f + t.amp*math.Sin(t.phase-float64(i)*0.55)*f
		nx, ny := x+math.Cos(a)*l, y+math.Sin(a)*l
		ch = append(ch, capsule{x, y, nx, ny, 3.9 - 1.5*f, 3.9 - 1.5*(f+1/float64(n))})
		x, y = nx, ny
	}
	return ch
}

func (p *painter) drawTail(t tailP, bx, by float64, far bool) {
	ch := t.build(bx, by)
	pal := furT
	if far {
		pal = furFarT
	}
	n := len(ch)
	p.draw(ch, pal, opts{over: func(x, y float64, tone int) (color.NRGBA, bool) {
		i := ch.idx(x, y)
		if i >= 0 && i%3 == 1 && i < n-1 {
			return stripeT[tone], true
		}
		if i == n-1 { // светлый кончик не нужен: рыжий хвост с тёмными кольцами
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
	rx, ry, pr := 3.3, 4.0, 1.4
	switch kind {
	case eyesWide:
		rx, ry, pr = 3.6, 4.6, 2.3
	case eyesSleepy:
		ry = 2.6
	}
	p.draw(ellipse{ex, ey, rx, ry, 0}, eyeT, opts{})
	p.draw(ellipse{ex + gx*1.4, ey + gy*1.2, pr, math.Min(ry-0.4, pr*2.2), 0}, tones{cPupil, cPupil, cPupil}, opts{noOutline: true})
	if kind != eyesSleepy {
		p.dot(ex-1.5+gx, ey-2.2+gy*0.6, cWhite)
		p.dot(ex-0.5+gx, ey-2.2+gy*0.6, cWhite)
	}
	if kind == eyesSleepy { // тяжёлое веко
		for dx := -rx; dx <= rx; dx++ {
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

func (p *painter) drawFront(f frontP) {
	br := f.breath
	if f.hang {
		p.drawHanging(f)
		return
	}
	// хвост лежит за спиной справа
	p.drawTail(f.tail, 47, 47, true)

	// задние лапы-бёдра
	p.draw(ellipse{15, 43, 9.5, 8.5, 0}, furT, opts{over: socks(46)})
	p.draw(ellipse{49, 43, 9.5, 8.5, 0}, furT, opts{over: socks(46)})
	// туловище: светлая грудка, полоски на боках
	body := ellipse{32, 35, 14.5, 16.5 + br*0.6, 0}
	p.draw(body, furT, opts{over: func(x, y float64, tone int) (color.NRGBA, bool) {
		u, v := body.uv(x, y)
		if math.Abs(u) < 0.5 && v > -0.35 {
			return creamT[tone], true
		}
		if math.Abs(u) > 0.62 && math.Mod(v*4+8, 1) < 0.4 {
			return stripeT[tone], true
		}
		return color.NRGBA{}, false
	}})
	// передние лапы
	p.frontPaw(26, 34, 26, 49.5, creamT)
	if f.pawUp > 0 {
		k := f.pawUp
		fx := 38 - 3*k
		fy := 49.5 - 23*k + math.Sin(f.pawWiggle)*1.5*k
		p.frontPaw(38, 34, fx, fy, creamT)
	} else {
		p.frontPaw(38, 34, 38, 49.5, creamT)
	}

	// голова
	hx := 32 + f.gaze*3 + f.tilt*0.5
	hy := 17 + f.headDown + f.gazeY*1.2 + br*0.3
	ew := f.earsBack
	// щёчки-бакенбарды
	p.draw(tri{hx - 13, hy + 1, hx - 19, hy + 8, hx - 10, hy + 8}, furT, opts{})
	p.draw(tri{hx + 13, hy + 1, hx + 19, hy + 8, hx + 10, hy + 8}, furT, opts{})
	// уши
	p.draw(tri{hx - 12.5, hy - 3, hx - 14 + f.earL - ew, hy - 17 + ew*2, hx - 2, hy - 9}, furT, opts{})
	p.draw(tri{hx + 12.5, hy - 3, hx + 14 + f.earR + ew, hy - 17 + ew*2, hx + 2, hy - 9}, furT, opts{})
	p.draw(tri{hx - 10.5, hy - 5.5, hx - 11.6 + f.earL - ew, hy - 13.5 + ew*2, hx - 4.5, hy - 8.5}, earT, opts{noOutline: true})
	p.draw(tri{hx + 10.5, hy - 5.5, hx + 11.6 + f.earR + ew, hy - 13.5 + ew*2, hx + 4.5, hy - 8.5}, earT, opts{noOutline: true})
	// голова с полосками на лбу
	head := ellipse{hx, hy, 14, 11.5, 0}
	p.draw(head, furT, opts{over: func(x, y float64, tone int) (color.NRGBA, bool) {
		u, v := head.uv(x, y)
		if v < -0.35 && v > -0.95 {
			if math.Abs(u) < 0.07 || math.Abs(math.Abs(u)-0.3) < 0.07 || (math.Abs(math.Abs(u)-0.6) < 0.06 && v > -0.75) {
				return stripeT[tone], true
			}
		}
		return color.NRGBA{}, false
	}})
	// светлая мордочка
	p.draw(ellipse{hx, hy + 6.5, 6.8, 4.8, 0}, creamT, opts{noOutline: true})
	// глаза
	gx, gy := f.gaze, f.gazeY
	p.eye(hx-6.5, hy+0.5, f.eyes, gx, gy)
	p.eye(hx+6.5, hy+0.5, f.eyes, gx, gy)
	// носик и ротик
	p.draw(tri{hx - 2, hy + 3, hx + 2, hy + 3, hx, hy + 5.5}, tones{cNose, cNose, cNose}, opts{noOutline: true})
	if f.mouth > 0.05 {
		o := 1 + f.mouth*4.5
		p.draw(ellipse{hx, hy + 8 + o*0.35, 3.6, o, 0}, tones{cMouth, cMouth, cMouth}, opts{noOutline: true})
		if f.mouth > 0.4 {
			p.draw(ellipse{hx, hy + 8 + o*0.7, 2.3, o * 0.45, 0}, tones{cTongue, cTongue, cTongue}, opts{noOutline: true})
		}
		p.dot(hx-2.5, hy+7, cWhite) // клычки
		p.dot(hx+1.5, hy+7, cWhite)
	} else {
		p.line(hx, hy+5.5, hx, hy+7, cOut)
		p.line(hx, hy+7, hx-2.5, hy+8.5, cOut)
		p.line(hx, hy+7, hx+2.5, hy+8.5, cOut)
		if f.tongue {
			p.dot(hx, hy+9, cTongue)
			p.dot(hx, hy+10, cTongue)
		}
	}
	// усы
	for i := 0; i < 3; i++ {
		fi := float64(i)
		p.line(hx-8, hy+6.5+fi*1.2, hx-17, hy+4.5+fi*2.6, cWhisker)
		p.line(hx+8, hy+6.5+fi*1.2, hx+17, hy+4.5+fi*2.6, cWhisker)
	}
}

func socks(from float64) func(x, y float64, tone int) (color.NRGBA, bool) {
	return func(x, y float64, tone int) (color.NRGBA, bool) {
		if y > from {
			return creamT[tone], true
		}
		return color.NRGBA{}, false
	}
}

// frontPaw — передняя лапа сверху вниз с светлой «варежкой».
func (p *painter) frontPaw(ax, ay, bx, by float64, pal tones) {
	leg := capsule{ax, ay, bx, by, 4.3, 3.7}
	p.draw(leg, furT, opts{over: func(x, y float64, tone int) (color.NRGBA, bool) {
		t, _ := leg.at(x, y)
		if t > 0.62 {
			return pal[tone], true
		}
		if t > 0.3 && math.Mod(y*0.9, 3) < 1 {
			return stripeT[tone], true
		}
		return color.NRGBA{}, false
	}})
	p.draw(ellipse{bx, by + 0.8, 4.6, 2.5, 0}, pal, opts{})
	// пальчики
	p.dot(bx-1.5, by+1.6, creamT[2])
	p.dot(bx+0.5, by+1.6, creamT[2])
}

// drawHanging — кота несут за шкирку: висит, лапки и хвост качаются.
func (p *painter) drawHanging(f frontP) {
	s := f.hangSway
	sw := []float64{0, 1, 0, -1}[f.hangStep%4]
	// хвост свисает
	tail := tailP{a0: math.Pi/2 - s*0.25, curl: 0.4, amp: 0.25, phase: float64(f.hangStep) * 0.8, n: 8, len: 5}
	p.drawTail(tail, 32+s*2, 40, true)
	// задние лапки
	p.draw(capsule{26 + s*1.5, 40, 25 + s*3 + sw*0.5, 53, 4.2, 3.4}, furT, opts{})
	p.draw(capsule{38 + s*1.5, 40, 39 + s*3 - sw*0.5, 53, 4.2, 3.4}, furT, opts{})
	p.draw(ellipse{25 + s*3 + sw*0.5, 54, 4, 2.2, 0}, creamT, opts{})
	p.draw(ellipse{39 + s*3 - sw*0.5, 54, 4, 2.2, 0}, creamT, opts{})
	// туловище вытянуто
	body := ellipse{32 + s, 32, 11, 15, 0}
	p.draw(body, furT, opts{over: func(x, y float64, tone int) (color.NRGBA, bool) {
		u, v := body.uv(x, y)
		if math.Abs(u) < 0.5 && v > -0.2 {
			return creamT[tone], true
		}
		return color.NRGBA{}, false
	}})
	// передние лапки поджаты
	p.draw(capsule{24 + s*0.5, 26, 23 + s, 37, 3.6, 3.2}, furT, opts{})
	p.draw(capsule{40 + s*0.5, 26, 41 + s, 37, 3.6, 3.2}, furT, opts{})
	p.draw(ellipse{23 + s, 38, 3.8, 2.2, 0}, creamT, opts{})
	p.draw(ellipse{41 + s, 38, 3.8, 2.2, 0}, creamT, opts{})
	// голова
	hx, hy := 32.0+s*0.3, 13.0
	p.draw(tri{hx - 12.5, hy - 3, hx - 14, hy - 17, hx - 2, hy - 9}, furT, opts{})
	p.draw(tri{hx + 12.5, hy - 3, hx + 14, hy - 17, hx + 2, hy - 9}, furT, opts{})
	p.draw(tri{hx - 10.5, hy - 5.5, hx - 11.6, hy - 13.5, hx - 4.5, hy - 8.5}, earT, opts{noOutline: true})
	p.draw(tri{hx + 10.5, hy - 5.5, hx + 11.6, hy - 13.5, hx + 4.5, hy - 8.5}, earT, opts{noOutline: true})
	head := ellipse{hx, hy, 14, 11.5, 0}
	p.draw(head, furT, opts{over: func(x, y float64, tone int) (color.NRGBA, bool) {
		u, v := head.uv(x, y)
		if v < -0.35 && v > -0.95 && (math.Abs(u) < 0.07 || math.Abs(math.Abs(u)-0.3) < 0.07) {
			return stripeT[tone], true
		}
		return color.NRGBA{}, false
	}})
	p.draw(ellipse{hx, hy + 6.5, 6.8, 4.8, 0}, creamT, opts{noOutline: true})
	p.eye(hx-6.5, hy+0.5, eyesSleepy, 0, 0)
	p.eye(hx+6.5, hy+0.5, eyesSleepy, 0, 0)
	p.draw(tri{hx - 2, hy + 3, hx + 2, hy + 3, hx, hy + 5.5}, tones{cNose, cNose, cNose}, opts{noOutline: true})
	p.line(hx, hy+5.5, hx, hy+7, cOut)
	p.line(hx, hy+7, hx-2.5, hy+8.5, cOut)
	p.line(hx, hy+7, hx+2.5, hy+8.5, cOut)
	for i := 0; i < 3; i++ {
		fi := float64(i)
		p.line(hx-8, hy+6.5+fi*1.2, hx-17, hy+4.5+fi*2.6, cWhisker)
		p.line(hx+8, hy+6.5+fi*1.2, hx+17, hy+4.5+fi*2.6, cWhisker)
	}
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
		p.draw(capsule{jx, jy, kx, ky, 5.6, 4.0}, fur, opts{})
	} else {
		p.draw(capsule{jx, jy, kx, ky, 4.4, 3.5}, fur, opts{})
	}
	shin := capsule{kx, ky, fx, fy, 3.5, 2.9}
	p.draw(shin, fur, opts{over: func(x, y float64, tone int) (color.NRGBA, bool) {
		t, _ := shin.at(x, y)
		if t > 0.7 {
			return cream[tone], true
		}
		return color.NRGBA{}, false
	}})
	p.draw(ellipse{fx + 1.2, fy + 0.9, 3.7, 2.1, 0}, cream, opts{})
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

	// туловище: полоски по спине, светлое брюшко и грудка
	torso := ellipse{mid[0], mid[1], 18.5, 10, s.pitch}
	p.draw(torso, furT, opts{over: func(x, y float64, tone int) (color.NRGBA, bool) {
		u, v := torso.uv(x, y)
		if v > 0.55 || (u > 0.45 && v > 0.1) {
			return creamT[tone], true
		}
		if v < 0.3 && math.Mod(u*18.5/4.6+20, 1) < 0.4 {
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
	ew := s.earsBack
	// уши
	ax, ay := hp(-6, -6)
	bx, by := hp(-6.5-ew*2, -16.5+ew*3)
	cx, cy := hp(1.5, -8.5)
	p.draw(tri{ax, ay, bx, by, cx, cy}, furT, opts{})
	dx, dy := hp(2, -8.5)
	ex, ey := hp(7+ew, -16+ew*3)
	fx, fy := hp(9.5, -4.5)
	p.draw(tri{dx, dy, ex, ey, fx, fy}, furT, opts{})
	ix, iy := hp(-4.8, -7)
	jx, jy := hp(-5.2-ew*2, -13.4+ew*2.4)
	kx, ky := hp(0.5, -8.5)
	p.draw(tri{ix, iy, jx, jy, kx, ky}, earT, opts{noOutline: true})
	ix, iy = hp(3.4, -8)
	jx, jy = hp(6.6+ew, -13+ew*2.4)
	kx, ky = hp(8, -5.5)
	p.draw(tri{ix, iy, jx, jy, kx, ky}, earT, opts{noOutline: true})
	// череп и мордочка
	head := ellipse{nx, ny, 10.5, 9, s.tilt}
	p.draw(head, furT, opts{over: func(x, y float64, tone int) (color.NRGBA, bool) {
		u, v := head.uv(x, y)
		if v < -0.3 && v > -0.9 && (math.Abs(u+0.25) < 0.08 || math.Abs(u-0.1) < 0.08 || math.Abs(u+0.55) < 0.07) {
			return stripeT[tone], true
		}
		return color.NRGBA{}, false
	}})
	mx, my := hp(8.5, 3.4)
	p.draw(ellipse{mx, my, 6, 4.6, s.tilt}, creamT, opts{})
	// глаз, нос, рот, усы
	exx, eyy := hp(4.6, -0.8)
	p.eye(exx, eyy, s.eyes, s.gx, s.gy)
	nxp, nyp := hp(14, 2)
	p.draw(ellipse{nxp, nyp, 1.9, 1.4, 0}, tones{cNose, cNose, cNose}, opts{noOutline: true})
	if s.mouth > 0.05 {
		mox, moy := hp(11.2, 6.5)
		o := 0.9 + s.mouth*3.4
		p.draw(ellipse{mox, moy + o*0.3, 3.4, o, 0}, tones{cMouth, cMouth, cMouth}, opts{noOutline: true})
		if s.mouth > 0.4 {
			tx, ty := hp(10.5, 7+o*0.5)
			p.draw(ellipse{tx, ty, 2, o * 0.45, 0}, tones{cTongue, cTongue, cTongue}, opts{noOutline: true})
		}
	} else {
		a1x, a1y := hp(13.4, 4)
		a2x, a2y := hp(12, 6.2)
		a3x, a3y := hp(9.2, 6.4)
		p.line(a1x, a1y, a2x, a2y, cOut)
		p.line(a2x, a2y, a3x, a3y, cOut)
		if s.tongue {
			p.dot(a3x, a3y+1, cTongue)
		}
	}
	for i := 0; i < 2; i++ {
		wx, wy := hp(11, 4.6+float64(i)*0.8)
		w2x, w2y := hp(20, 2.6+float64(i)*3.6)
		p.line(wx, wy, w2x, w2y, cWhisker)
	}
}

// ---------- сон ----------

// drawSleep — свернулся клубком, голова на лапах, хвост обвивает.
func (p *painter) drawSleep(breath float64) {
	b := breath * 0.6
	// хвост по полу перед котом
	tail := chain{
		{12, 47, 22, 49.5, 3.6, 3.4}, {22, 49.5, 34, 50, 3.4, 3.3}, {34, 50, 46, 49.4, 3.3, 3.0}, {46, 49.4, 54, 47.2, 3.0, 2.6},
	}
	p.draw(tail, furT, opts{over: func(x, y float64, tone int) (color.NRGBA, bool) {
		if i := tail.idx(x, y); i >= 0 && math.Mod(x/4.5, 1) < 0.4 {
			return stripeT[tone], true
		}
		return color.NRGBA{}, false
	}})
	// спина
	body := ellipse{29, 40 - b, 25, 11 + b, 0}
	p.draw(body, furT, opts{over: func(x, y float64, tone int) (color.NRGBA, bool) {
		u, v := body.uv(x, y)
		if v > 0.6 {
			return creamT[tone], true
		}
		if v < 0.45 && math.Mod(u*25/4.8+30, 1) < 0.4 {
			return stripeT[tone], true
		}
		return color.NRGBA{}, false
	}})
	p.draw(ellipse{17, 41 - b, 11, 9 + b, 0}, furT, opts{over: func(x, y float64, tone int) (color.NRGBA, bool) {
		if math.Mod(y/4, 1) < 0.35 && y < 44 {
			return stripeT[tone], true
		}
		return color.NRGBA{}, false
	}})
	// передние лапы под мордочкой
	p.draw(ellipse{50, 47.5, 8, 3.2, 0}, furT, opts{over: func(x, y float64, tone int) (color.NRGBA, bool) {
		if x > 52 {
			return creamT[tone], true
		}
		return color.NRGBA{}, false
	}})
	// голова лежит набок
	hx, hy := 48.0, 38.5-b*0.4
	p.draw(tri{hx - 7, hy - 5, hx - 7.5, hy - 14.5, hx + 0.5, hy - 8}, furT, opts{})
	p.draw(tri{hx + 1.5, hy - 8, hx + 6.5, hy - 14.5, hx + 9.5, hy - 4}, furT, opts{})
	p.draw(tri{hx - 5.5, hy - 6.5, hx - 6, hy - 12, hx - 0.5, hy - 8}, earT, opts{noOutline: true})
	p.draw(tri{hx + 2.5, hy - 8, hx + 6, hy - 12, hx + 8, hy - 5.5}, earT, opts{noOutline: true})
	head := ellipse{hx, hy, 11, 9, 0}
	p.draw(head, furT, opts{over: func(x, y float64, tone int) (color.NRGBA, bool) {
		u, v := head.uv(x, y)
		if v < -0.3 && v > -0.9 && (math.Abs(u+0.2) < 0.08 || math.Abs(u-0.15) < 0.08) {
			return stripeT[tone], true
		}
		return color.NRGBA{}, false
	}})
	p.draw(ellipse{hx + 4.5, hy + 4.2, 6.5, 4.4, 0}, creamT, opts{noOutline: true})
	p.eye(hx-3, hy+0.5, eyesClosed, 0, 0)
	p.eye(hx+5.5, hy+0.5, eyesClosed, 0, 0)
	p.draw(ellipse{hx + 8, hy + 3.4, 1.8, 1.3, 0}, tones{cNose, cNose, cNose}, opts{noOutline: true})
	p.line(hx+6.5, hy+6.2, hx+9, hy+6.2, cOut)
}
