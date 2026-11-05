package cat

import (
	"image"
	"image/color"
	"math"
)

// Окно кота — ViewW × ViewH клеток; пол на GroundY. Клетка рисуется Fine × Fine
// пикселями холста, то есть холст — ViewW*Fine × ViewH*Fine.
const (
	Fine    = 2
	ViewW   = 90
	ViewH   = 55
	GroundY = ViewH - 2
)

// Anchor — где загривок, когда кота несут: курсор держит кота здесь (клетки окна).
var Anchor = struct{ X, Y float64 }{ViewW / 2, 7}

// DropHeight — на какой высоте над полом окажутся лапы отпущенного кота.
var DropHeight = GroundY - (Anchor.Y - Scruff.Y + 32)

// Canvas — холст с прозрачностью: пиксели BGRA с предумноженным альфа-каналом,
// как этого требует UpdateLayeredWindow.
type Canvas struct {
	W, H int
	Pix  []byte
}

// NewCanvas создаёт прозрачный холст.
func NewCanvas() *Canvas {
	w, h := ViewW*Fine, ViewH*Fine
	return &Canvas{W: w, H: h, Pix: make([]byte, w*h*4)}
}

// Clear делает холст полностью прозрачным.
func (c *Canvas) Clear() { clear(c.Pix) }

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

// rect — прямоугольник в клетках.
func (c *Canvas) rect(x, y, w, h float64, col color.NRGBA) {
	c.px(ri(x*Fine), ri(y*Fine), ri(w*Fine), ri(h*Fine), col)
}

func ri(x float64) int { return int(jsRound(x)) }

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

// drawSprite рисует позу в клетках окна: контур в один мелкий пиксель со срезанными
// углами, на шерсти — блик сверху и тень снизу и справа (свет падает слева сверху).
func (c *Canvas) drawSprite(s Sprite, ox, oy float64, facing int) {
	at := func(p Px) (x, y, w, h int) {
		fx := ox + p.X
		if facing <= 0 {
			fx = ox + SprW - p.X - p.W
		}
		return ri(fx * Fine), ri((oy + p.Y) * Fine), ri(p.W * Fine), ri(p.H * Fine)
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
		c.px(x, y, w, h, p.Col)
	}
}

func (c *Canvas) drawShadow(cx, width float64) {
	w := ri(width * Fine)
	x := ri(cx*Fine - float64(w)/2)
	y := GroundY * Fine
	black := color.NRGBA{0, 0, 0, 255}
	c.fill(x+2, y, w-4, 1, black, 0.28)
	c.fill(x, y+1, w, 2, black, 0.28)
	c.fill(x+2, y+3, w-4, 1, black, 0.28)
}

// drawBall — клубок: кружок 5×5 клеток с бликом, тенью и намотанной ниткой, которая крутится.
func (c *Canvas) drawBall(bx float64) {
	r := BallSize * Fine / 2
	cx := ri(bx*Fine) + r
	cy := GroundY*Fine - r
	spin := ri(bx*2) % 4
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
			c.px(cx+dx, cy+dy, 1, 1, col)
		}
	}
	c.px(cx-2, cy-3, 1, 1, cWhite)
}

var zRows = [5]string{"#####", "...#.", "..#..", ".#...", "#####"}

func (c *Canvas) drawZzz(ox, oy float64, facing, tick int) {
	head := ox + 2
	if facing > 0 {
		head = ox + 22
	}
	for i := 0; i < 3; i++ {
		age := float64((tick + i*10) % 30)
		off := 0.0
		if facing <= 0 {
			off = -3
		}
		x := ri((head + float64(facing)*age/5 + off) * Fine)
		y := ri((oy + 9 - age/3) * Fine)
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

// Render рисует кота на холсте. view — левый край окна в клетках мира.
func Render(c *Canvas, cat *Cat, view float64) {
	c.Clear()
	s := cat.Sprite()
	if cat.Mode == Carried {
		c.drawSprite(s, Anchor.X-Scruff.X, Anchor.Y-Scruff.Y, 1)
		// кулачок, который держит загривок
		c.rect(Anchor.X-2, Anchor.Y-4, 4, 2.5, cOut)
		c.rect(Anchor.X-1.5, Anchor.Y-3.5, 3, 1.5, cFist)
		return
	}
	ox := jsRound(cat.X) - view
	oy := GroundY - SprH - cat.Lift
	c.drawShadow(ox+SprW/2, math.Max(8, 20-cat.Lift))
	c.drawSprite(s, ox, oy, cat.Facing)
	if cat.Ball != nil {
		c.drawBall(cat.Ball.X - view)
	}
	if cat.Mode == Sleep {
		c.drawZzz(ox, oy, cat.Facing, cat.Tick)
	}
}
