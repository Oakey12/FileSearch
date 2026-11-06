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

func (c *Canvas) px(x, y, w, h int, col color.NRGBA) { c.fill(x, y, w, h, col, 1) }

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

func ri(x float64) int { return int(jsRound(x)) }

// rect — прямоугольник, заданный в клетках, от точки (ox, oy) в пикселях.
func (c *Canvas) rect(ox, oy, x, y, w, h float64, col color.NRGBA) {
	c.px(ri(ox+x*fine), ri(oy+y*fine), ri(w*fine), ri(h*fine), col)
}

// drawSprite рисует позу от точки (ox, oy) в пикселях холста: контур в один пиксель
// со срезанными углами, на шерсти — блик сверху и тень снизу и справа (свет падает
// слева сверху).
func (c *Canvas) drawSprite(s Sprite, ox, oy float64, facing int) {
	at := func(p px) (x, y, w, h int) {
		fx := p.X
		if facing <= 0 {
			fx = spriteW - p.X - p.W
		}
		return ri(ox + fx*fine), ri(oy + p.Y*fine), ri(p.W * fine), ri(p.H * fine)
	}
	for _, layer := range s.Layers {
		for _, p := range layer {
			x, y, w, h := at(p)
			c.px(x-1, y, w+2, h, cOut)
			c.px(x, y-1, w, h+2, cOut)
		}
		for _, p := range layer {
			x, y, w, h := at(p)
			c.px(x, y, w, h, p.Col)
			if p.Col != cFur {
				continue
			}
			if w >= 8 && h >= 8 {
				c.px(x+1, y, w-2, 1, cFurLight)
				c.px(x, y+1, 1, h-3, cFurLight)
				c.px(x, y+h-2, w, 2, cShade)
				c.px(x+w-1, y+1, 1, h-1, cShade)
			} else if w >= 3 {
				c.px(x+w-1, y, 1, h, cShade)
			}
		}
	}
	for _, p := range s.Details {
		x, y, w, h := at(p)
		if p.Col.A < 255 {
			c.fill(x, y, w, h, p.Col, 1)
		} else {
			c.px(x, y, w, h, p.Col)
		}
	}
}

func (c *Canvas) drawShadow(cx, width float64) {
	w := ri(width)
	x := ri(cx - float64(w)/2)
	y := GroundY
	black := color.NRGBA{0, 0, 0, 255}
	c.fill(x+2, y, w-4, 1, black, 0.28)
	c.fill(x, y+1, w, 2, black, 0.28)
	c.fill(x+2, y+3, w-4, 1, black, 0.28)
}

func drawBall(c *Canvas, bx float64) {
	r := BallSize / 2
	cx := ri(bx) + r
	cy := GroundY - r
	spin := ri(bx/2) % 4
	for dy := -r - 1; dy <= r; dy++ {
		half := ri(math.Sqrt(math.Max(0, math.Pow(float64(r)+0.5, 2)-math.Pow(float64(dy)+0.5, 2))))
		if half == 0 {
			continue
		}
		c.px(cx-half-1, cy+dy, half*2+2, 1, cOut)
	}
	for dy := -r; dy < r; dy++ {
		half := ri(math.Sqrt(math.Max(0, float64(r*r)-math.Pow(float64(dy)+0.5, 2))))
		for dx := -half; dx < half; dx++ {
			light := float64(dx+dy) < -float64(r)/2
			dark := float64(dx+dy) > float64(r)/2
			col := cBall
			if light {
				col = cBallLight
			} else if dark {
				col = cBallDark
			}
			if (dx+dy*2+spin+40)%4 == 0 && !light {
				col = cBallLight // нитки
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
	c.px(int(x), int(y)-2, 1, 6, cOut)
	c.set(int(x)-1, int(y)-3, cOut)
	c.set(int(x)+1, int(y)-3, cOut)
}

var zRows = [5]string{"#####", "...#.", "..#..", ".#...", "#####"}

func drawZzz(c *Canvas, ox, oy float64, facing, tick int) {
	head := ox + 2*fine
	if facing > 0 {
		head = ox + 22*fine
	}
	off := 0.0
	if facing <= 0 {
		off = -3 * fine
	}
	for i := 0; i < 3; i++ {
		age := float64((tick/2 + i*10) % 30)
		x := ri(head + float64(facing)*age/5*fine + off)
		y := ri(oy + 9*fine - age/3*fine)
		k := 0
		if i == 1 {
			k = 1
		}
		alpha := 1 - age/30
		for ry, row := range zRows {
			for rx, ch := range row {
				if ch == '#' {
					c.fill(x+rx-1-k, y+ry-1, 3, 3, cOut, alpha)
				}
			}
		}
		for ry, row := range zRows {
			for rx, ch := range row {
				if ch == '#' {
					c.fill(x+rx, y+ry, 1, 1, cZzz, alpha)
				}
			}
		}
	}
}

// Render рисует кота на холсте. view — левый край окна в пикселях мира.
func Render(c *Canvas, cat *Cat, view float64) {
	c.Clear()
	s := cat.Sprite()
	if cat.Mode == Carried {
		ox, oy := math.Round(Anchor.X-Scruff.X), math.Round(Anchor.Y-Scruff.Y)
		c.drawSprite(s, ox, oy, 1)
		// кулачок, который держит загривок
		c.rect(Anchor.X, Anchor.Y, -2, -4, 4, 2.5, cOut)
		c.rect(Anchor.X, Anchor.Y, -1.5, -3.5, 3, 1.5, cFist)
		return
	}
	ox := jsRound(cat.X-view) + spriteOX
	oy := jsRound(float64(GroundY) - spriteH*fine - cat.Lift)

	// следы лап на полу
	for _, pr := range cat.Prints {
		a := 0.5 * (1 - float64(pr.Age)/60)
		x := ri(pr.X - view)
		y := int(pr.Y)
		prc := nrgb(0x5a, 0x40, 0x30)
		c.fill(x, y, 3, 2, prc, a)
		c.fill(x-1, y-1, 1, 1, prc, a)
		c.fill(x+1, y-2, 1, 1, prc, a)
		c.fill(x+3, y-1, 1, 1, prc, a)
	}
	c.drawShadow(ox+spriteW*fine/2, math.Max(16, 40-cat.Lift))
	c.drawSprite(s, ox, oy, cat.Facing)
	if cat.Ball != nil {
		drawBall(c, cat.Ball.X-view)
	}
	if cat.Butterfly != nil {
		drawButterfly(c, cat.Butterfly.X-view, cat.Butterfly.Y, cat.Tick)
	}
	if cat.Mode == Sleep {
		drawZzz(c, ox, oy, cat.Facing, cat.Tick)
	}
}
