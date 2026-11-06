package cat

import (
	"image"
	"image/color"
	"math"
)

// Canvas — холст с прозрачностью: пиксели BGRA с предумноженным альфа-каналом,
// как этого требует UpdateLayeredWindow.
type Canvas struct {
	W, H int
	Pix  []byte
}

// NewCanvas создаёт прозрачный холст размером с окно кота.
func NewCanvas() *Canvas {
	return &Canvas{W: ViewW, H: ViewH, Pix: make([]byte, ViewW*ViewH*4)}
}

// Clear делает холст полностью прозрачным.
func (c *Canvas) Clear() { clear(c.Pix) }

// set красит один пиксель непрозрачным цветом.
func (c *Canvas) set(x, y int, col color.NRGBA) {
	if x < 0 || y < 0 || x >= c.W || y >= c.H {
		return
	}
	i := (y*c.W + x) * 4
	c.Pix[i], c.Pix[i+1], c.Pix[i+2], c.Pix[i+3] = col.B, col.G, col.R, 255
}

// fill закрашивает прямоугольник цветом col с дополнительной прозрачностью alpha.
func (c *Canvas) fill(x, y, w, h int, col color.NRGBA, alpha float64) {
	x0, y0, x1, y1 := max(x, 0), max(y, 0), min(x+w, c.W), min(y+h, c.H)
	a := float64(col.A) / 255 * alpha
	if a <= 0 {
		return
	}
	sr, sg, sb := float64(col.R)*a, float64(col.G)*a, float64(col.B)*a
	sa := 255 * a
	inv := 1 - a
	for yy := y0; yy < y1; yy++ {
		i := (yy*c.W + x0) * 4
		for xx := x0; xx < x1; xx++ {
			c.Pix[i+0] = byte(sb + float64(c.Pix[i+0])*inv + 0.5)
			c.Pix[i+1] = byte(sg + float64(c.Pix[i+1])*inv + 0.5)
			c.Pix[i+2] = byte(sr + float64(c.Pix[i+2])*inv + 0.5)
			c.Pix[i+3] = byte(sa + float64(c.Pix[i+3])*inv + 0.5)
			i += 4
		}
	}
}

// ell — заливка эллипса (тень, крылья бабочки) с прозрачностью alpha.
func (c *Canvas) ell(cx, cy, rx, ry float64, col color.NRGBA, alpha float64) {
	for y := int(math.Floor(cy - ry)); y <= int(math.Ceil(cy+ry)); y++ {
		for x := int(math.Floor(cx - rx)); x <= int(math.Ceil(cx+rx)); x++ {
			dx, dy := (float64(x)+0.5-cx)/rx, (float64(y)+0.5-cy)/ry
			if dx*dx+dy*dy <= 1 {
				c.fill(x, y, 1, 1, col, alpha)
			}
		}
	}
}

// Image возвращает холст как обычную картинку (для тестов и проверки глазами).
func (c *Canvas) Image() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, c.W, c.H))
	for i := 0; i < c.W*c.H; i++ {
		a := c.Pix[i*4+3]
		if a == 0 {
			continue
		}
		unmul := func(v byte) uint8 { return uint8(min(255, int(v)*255/int(a))) }
		img.Pix[i*4+0] = unmul(c.Pix[i*4+2])
		img.Pix[i*4+1] = unmul(c.Pix[i*4+1])
		img.Pix[i*4+2] = unmul(c.Pix[i*4+0])
		img.Pix[i*4+3] = a
	}
	return img
}

// ---------- позы по состоянию кота ----------

func clamp(x, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, x)) }

func ease(u float64) float64 { return u * u * (3 - 2*u) }

func (c *Cat) eyesFor() eyeKind {
	if c.blink > 0 {
		return eyesClosed
	}
	return eyesOpen
}

func (c *Cat) frontParams() frontP {
	t := float64(c.Tick)
	f := frontP{
		breath: math.Sin(t * 0.12),
		gaze:   c.Gaze,
		gazeY:  c.GazeY,
		eyes:   c.eyesFor(),
		tail:   tailP{a0: 0.15, curl: -2.3, amp: 0.2, phase: t * 0.08},
		tilt:   c.Gaze * 1.5,
	}
	if c.happy > 0 {
		f.eyes = eyesHappy
		f.tongue = c.happy%12 < 6
		f.tail.amp = 0.8
		f.tail.phase = t * 0.4
	}
	if c.earL > 0 {
		f.earL = math.Sin(float64(c.earL)*1.3) * 2.4
	}
	if c.earR > 0 {
		f.earR = -math.Sin(float64(c.earR)*1.3) * 2.4
	}
	switch c.Mode {
	case Sit:
		switch c.event {
		case evWag:
			f.tail = tailP{a0: 0.1, curl: -1.6, amp: 1.0, phase: t * 0.5}
		case evButterfly:
			if c.evTick > 15 && c.evTick < 330 && c.happy == 0 {
				f.eyes = eyesWide
				if c.blink > 0 {
					f.eyes = eyesClosed
				}
			}
			f.earsBack = -0.3
		}
	case Wash:
		f.eyes = eyesClosed
		f.pawUp = ease(clamp(float64(c.ModeTick)/10, 0, 1)) * ease(clamp(float64(100-c.ModeTick)/10, 0, 1))
		f.pawWiggle = t * 0.7
		f.headDown = 2 * f.pawUp
		f.tongue = (c.Tick/5)%2 == 0 && f.pawUp > 0.9
	case Yawn:
		k := math.Sin(math.Pi * math.Min(1, float64(c.ModeTick)/36))
		f.eyes = eyesClosed
		f.mouth = k
		f.headDown = -2 * k
		f.earsBack = k * 1.4
	case Carried:
		f.hang = true
		f.hangSway = c.Sway
		f.hangStep = c.Tick / 3
	}
	return f
}

func footAt(joint, s, lift, ph float64) foot {
	t := math.Mod(ph, 1)
	if t < 0 {
		t++
	}
	if t < 0.6 {
		return foot{joint + s*(1-2*t/0.6), groundFoot}
	}
	u := (t - 0.6) / 0.4
	return foot{joint - s + 2*s*ease(u), groundFoot - lift*math.Sin(math.Pi*u)}
}

// gaitFeet — четыре лапы шагающего кота; диагональные пары ходят вместе.
func gaitFeet(s, lift, ph float64) (hindN, hindF, frontN, frontF foot) {
	const hj, fj = 16.0, 41.0
	return footAt(hj, s, lift, ph+0.5), footAt(hj+3, s, lift, ph), footAt(fj, s, lift, ph), footAt(fj-3, s, lift, ph+0.5)
}

func standFeet() (hindN, hindF, frontN, frontF foot) {
	return foot{16, groundFoot}, foot{21, groundFoot}, foot{44, groundFoot}, foot{38, groundFoot}
}

func (c *Cat) sideParams() sideP {
	t := float64(c.Tick)
	s := sideP{eyes: c.eyesFor(), tail: tailP{a0: -2.0, curl: 1.6, amp: 0.45, phase: t * 0.15}}
	s.hindN, s.hindF, s.frontN, s.frontF = standFeet()
	switch c.Mode {
	case Walk:
		switch c.gait {
		case gStride:
			s.hindN, s.hindF, s.frontN, s.frontF = gaitFeet(6, 4, c.Phase)
			s.bodyY = -0.6 + 0.8*math.Sin(4*math.Pi*c.Phase)
			s.headDY = 0.9 * math.Sin(4*math.Pi*c.Phase+1)
			s.tail = tailP{a0: -2.0, curl: 1.7, amp: 0.55, phase: t * 0.14}
		case gTrot:
			s.hindN, s.hindF, s.frontN, s.frontF = gaitFeet(8.5, 5.5, c.Phase)
			s.bodyY = -0.8 + 1.1*math.Sin(4*math.Pi*c.Phase)
			s.headDY = 1.2 * math.Sin(4*math.Pi*c.Phase+1)
			s.pitch = -0.03
			s.tail = tailP{a0: -1.8, curl: 1.3, amp: 0.7, phase: t * 0.28}
			s.earsBack = 0.5
		case gHop:
			s.hindN, s.hindF = foot{10, groundFoot - 9}, foot{16, groundFoot - 11}
			s.frontN, s.frontF = foot{50, groundFoot - 8}, foot{44, groundFoot - 10}
			s.pitch = -0.1
			s.eyes = eyesWide
			if c.blink > 0 {
				s.eyes = eyesClosed
			}
			s.tail = tailP{a0: -1.8, curl: 0.6, amp: 0.3, phase: t * 0.3}
		case gSniff:
			s.pitch = 0.1
			s.bodyY = 1
			s.headDY = 15 + 1.4*math.Sin(t*0.55)
			s.headDX = 3 + 1.5*math.Sin(t*0.3)
			s.tilt = 0.35
			s.tail = tailP{a0: -2.0, curl: 1.2, amp: 0.4, phase: t * 0.1}
		case gLookUp:
			s.headDY = -4
			s.headDX = -1
			s.tilt = -0.35
			s.eyes = eyesWide
			if c.blink > 0 {
				s.eyes = eyesClosed
			}
			s.gy = -1
			s.tail = tailP{a0: -2.2, curl: 1.0, amp: 0.8, phase: t * 0.22}
		}
	case Stretch:
		k := math.Sin(math.Pi * math.Min(1, float64(c.ModeTick)/56))
		s.pitch = 0.32 * k
		s.bodyY = -2 * k
		s.frontN = foot{44 + 11*k, groundFoot}
		s.frontF = foot{38 + 11*k, groundFoot}
		s.headDY = 4 * k
		s.eyes = eyesClosed
		s.mouth = clamp((k-0.5)*2, 0, 1)
		s.tail = tailP{a0: -1.8, curl: 0.3, amp: 0.15, phase: t * 0.1}
	case Fall:
		s.hindN, s.hindF = foot{6, groundFoot - 20}, foot{12, groundFoot - 22}
		s.frontN, s.frontF = foot{54, groundFoot - 16}, foot{48, groundFoot - 19}
		s.eyes = eyesWide
		s.mouth = 0.4
		s.tail = tailP{a0: -1.7, curl: 0.2, amp: 0.4, phase: t * 0.4}
	case Land:
		s.bodyY = 4
		s.hindN, s.hindF = foot{12, groundFoot}, foot{19, groundFoot}
		s.frontN, s.frontF = foot{47, groundFoot}, foot{41, groundFoot}
		s.eyes = eyesWide
		s.tail = tailP{a0: -2.4, curl: 0.6, amp: 0.3, phase: t * 0.2}
	case Play:
		switch c.phase {
		case chase:
			s.hindN, s.hindF, s.frontN, s.frontF = gaitFeet(9, 7, c.Phase)
			s.bodyY = 1.8 * math.Sin(4*math.Pi*c.Phase)
			s.pitch = -0.04
			s.eyes = eyesWide
			s.earsBack = 0.5
			s.tail = tailP{a0: -1.8, curl: 0.6, amp: 0.6, phase: t * 0.4}
		case stalk:
			s.bodyY = 6 + 0.6*math.Sin(t*0.6)
			s.hindN, s.hindF = foot{14, groundFoot}, foot{20, groundFoot}
			s.frontN, s.frontF = foot{44, groundFoot}, foot{38, groundFoot}
			s.headDY = 4
			s.eyes = eyesWide
			s.earsBack = 0.7
			s.tail = tailP{a0: -2.9, curl: 0.5, amp: 0.9, phase: t * 0.5}
		case leap:
			s.pitch = -0.28
			s.hindN, s.hindF = foot{2, groundFoot - 9}, foot{9, groundFoot - 12}
			s.frontN, s.frontF = foot{58, groundFoot - 14}, foot{52, groundFoot - 17}
			s.eyes = eyesWide
			s.mouth = 0.25
			s.earsBack = 0.8
			s.tail = tailP{a0: -2.9, curl: 0.2, amp: 0.15, phase: t * 0.3}
		default:
			s.eyes = eyesWide
			if c.blink > 0 {
				s.eyes = eyesClosed
			}
			s.tail = tailP{a0: -2.1, curl: 1.0, amp: 0.8, phase: t * 0.3}
		}
	}
	return s
}

// ---------- кадр целиком ----------

var zRows = [5]string{"#####", "...#.", "..#..", ".#...", "#####"}

// Render рисует кота на холсте. view — левый край окна в пикселях мира.
func Render(c *Canvas, cat *Cat, view float64) {
	c.Clear()
	if cat.Mode == Carried {
		p := &painter{c: c, ox: math.Round(Anchor.X - Scruff.X), oy: math.Round(Anchor.Y - Scruff.Y), width: SprW}
		p.drawFront(cat.frontParams())
		// кулак, который держит загривок
		fx, fy := int(Anchor.X)-7, int(Anchor.Y)-9
		c.fill(fx-1, fy-1, 16, 11, cOut, 1)
		c.fill(fx, fy, 14, 9, cFist, 1)
		c.fill(fx+1, fy+1, 12, 2, creamT[0], 1)
		for i := 0; i < 3; i++ {
			c.fill(fx+3+i*4, fy+3, 1, 5, nrgb(0xd9, 0xb0, 0x8a), 1)
		}
		return
	}
	ox := jsRound(cat.X - view)
	oy := jsRound(float64(GroundY) - SprH - cat.Lift)

	// следы лап на полу
	for _, pr := range cat.Prints {
		a := 0.5 * (1 - float64(pr.Age)/60)
		x := int(jsRound(pr.X - view))
		y := int(pr.Y)
		prc := nrgb(0x5a, 0x40, 0x30)
		c.fill(x, y, 3, 2, prc, a)
		c.fill(x-1, y-1, 1, 1, prc, a)
		c.fill(x+1, y-2, 1, 1, prc, a)
		c.fill(x+3, y-1, 1, 1, prc, a)
	}
	// мягкая тень под котом
	shw := 28.0
	if cat.Mode == Sleep {
		shw = 34
	}
	shw *= math.Max(0.4, 1-cat.Lift/90)
	c.ell(ox+SprW/2, float64(GroundY)+1, shw, 3, nrgb(0, 0, 0), 0.26*math.Max(0.4, 1-cat.Lift/90))

	p := &painter{c: c, ox: ox, oy: oy, mirror: cat.Facing < 0, width: SprW}
	switch cat.Mode {
	case Sit, Wash, Yawn:
		p.mirror = false
		p.drawFront(cat.frontParams())
	case Sleep:
		p.drawSleep(math.Sin(float64(cat.Tick) * 0.1))
	default:
		p.drawSide(cat.sideParams())
	}
	if cat.Ball != nil {
		drawBall(c, cat.Ball.X-view)
	}
	if cat.Butterfly != nil {
		drawButterfly(c, cat.Butterfly.X-view, cat.Butterfly.Y, cat.Tick)
	}
	if cat.Mode == Sleep {
		drawZzz(c, ox, oy, cat.Tick)
	}
}

func drawBall(c *Canvas, bx float64) {
	r := BallSize / 2
	cx := int(jsRound(bx)) + r
	cy := GroundY - r
	spin := int(jsRound(bx/2)) % 4
	for dy := -r - 1; dy <= r; dy++ {
		half := int(jsRound(math.Sqrt(math.Max(0, math.Pow(float64(r)+0.5, 2)-math.Pow(float64(dy)+0.5, 2)))))
		if half == 0 {
			continue
		}
		c.fill(cx-half-1, cy+dy, half*2+2, 1, cOut, 1)
	}
	light, base, dark := nrgb(0xa9, 0xc4, 0xff), nrgb(0x5b, 0x8d, 0xef), nrgb(0x3a, 0x63, 0xc0)
	for dy := -r; dy < r; dy++ {
		half := int(jsRound(math.Sqrt(math.Max(0, float64(r*r)-math.Pow(float64(dy)+0.5, 2)))))
		for dx := -half; dx < half; dx++ {
			col := base
			isLight := float64(dx+dy) < -float64(r)/2
			if isLight {
				col = light
			} else if float64(dx+dy) > float64(r)/2 {
				col = dark
			}
			if (dx+dy*2+spin+40)%4 == 0 && !isLight {
				col = light // нитки
			}
			c.set(cx+dx, cy+dy, col)
		}
	}
	c.set(cx-2, cy-3, cWhite)
}

// drawButterfly — бабочка: крылья машут, тельце тёмное. Просто украшение окна кота.
func drawButterfly(c *Canvas, x, y float64, tick int) {
	flap := 0.35 + 0.65*math.Abs(math.Sin(float64(tick)*0.9))
	wing, wing2 := nrgb(0x7f, 0xb3, 0xff), nrgb(0xd6, 0xe6, 0xff)
	c.ell(x-3.5*flap, y-1.5, 3.6*flap+0.4, 2.6, wing, 1)
	c.ell(x+3.5*flap, y-1.5, 3.6*flap+0.4, 2.6, wing, 1)
	c.ell(x-2.6*flap, y+2, 2.4*flap+0.3, 1.9, wing2, 1)
	c.ell(x+2.6*flap, y+2, 2.4*flap+0.3, 1.9, wing2, 1)
	c.fill(int(x), int(y)-2, 1, 6, cOut, 1)
	c.set(int(x)-1, int(y)-3, cOut)
	c.set(int(x)+1, int(y)-3, cOut)
}

func drawZzz(c *Canvas, ox, oy float64, tick int) {
	for i := 0; i < 3; i++ {
		age := float64((tick + i*18) % 54)
		x := int(ox) + 56 + int(age/3) + i*2
		y := int(oy) + 14 - int(age/2.2)
		sz := 1 + i/2
		alpha := 1 - age/54
		for ry, row := range zRows {
			for rx, ch := range row {
				if ch == '#' {
					c.fill(x+rx*sz-1, y+ry*sz-1, sz+2, sz+2, cOut, alpha)
				}
			}
		}
		for ry, row := range zRows {
			for rx, ch := range row {
				if ch == '#' {
					c.fill(x+rx*sz, y+ry*sz, sz, sz, nrgb(0x9c, 0xc8, 0xff), alpha)
				}
			}
		}
	}
}
