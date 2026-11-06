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

// foot — куда ставится лапа (пиксели спрайта).
type foot struct{ X, Y float64 }

// footAt — где стопа в фазе шага ph: 60% времени стоит на полу и едет назад
// относительно тела, остальное время поднимается и переносится вперёд.
func footAt(joint, s, lift, ph float64) foot {
	ground := float64(boxH - 1)
	t := math.Mod(ph, 1)
	if t < 0 {
		t++
	}
	if t < 0.6 {
		return foot{joint + s*(1-2*t/0.6), ground}
	}
	u := (t - 0.6) / 0.4
	return foot{joint - s + 2*s*ease(u), ground - lift*math.Sin(math.Pi*u)}
}

// Render рисует кота на холсте. view — левый край окна в пикселях мира.
func Render(c *Canvas, cat *Cat, view float64) {
	c.Clear()
	f, mirror := spriteFrame(cat)
	if cat.Mode == Carried {
		f.blit(c, math.Round(Anchor.X-Scruff.X), math.Round(Anchor.Y-Scruff.Y), false)
		// кулак, который держит загривок
		fx, fy := int(Anchor.X)-7, int(Anchor.Y)-9
		c.fill(fx-1, fy-1, 16, 11, cOut, 1)
		c.fill(fx, fy, 14, 9, nrgb(0xf3, 0xd2, 0xb3), 1)
		c.fill(fx+1, fy+1, 12, 2, nrgb(0xff, 0xf0, 0xe0), 1)
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
	k := math.Max(0.4, 1-cat.Lift/90)
	c.ell(ox+SprW/2, float64(GroundY)+1, 24*k, 3, nrgb(0, 0, 0), 0.24*k)

	f.blit(c, ox, oy, mirror)
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

var zRows = [5]string{"#####", "...#.", "..#..", ".#...", "#####"}
