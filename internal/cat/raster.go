package cat

import (
	"image/color"
	"math"
)

// Растеризатор в стиле пиксель-арта: формы (эллипсы, капсулы, треугольники, цепочки
// капсул) заливаются тремя тонами — свет падает слева сверху — и обводятся контуром
// в один пиксель. Так кота можно двигать параметрами (угол лапы, изгиб хвоста), а не
// рисовать каждый кадр вручную.

// tones — три тона одного материала: свет, основной, тень.
type tones [3]color.NRGBA

func nrgb(r, g, b uint8) color.NRGBA { return color.NRGBA{r, g, b, 255} }

var (
	furT      = tones{nrgb(0xff, 0xb8, 0x62), nrgb(0xf2, 0x8c, 0x28), nrgb(0xc8, 0x64, 0x1a)}
	furFarT   = tones{nrgb(0xd9, 0x7a, 0x22), nrgb(0xbf, 0x66, 0x1b), nrgb(0x9a, 0x4d, 0x12)}
	stripeT   = tones{nrgb(0xd8, 0x6a, 0x1a), nrgb(0xb4, 0x50, 0x0f), nrgb(0x8f, 0x3d, 0x0a)}
	creamT    = tones{nrgb(0xff, 0xfa, 0xee), nrgb(0xff, 0xf0, 0xd4), nrgb(0xe8, 0xc2, 0x92)}
	creamFarT = tones{nrgb(0xf0, 0xd8, 0xb0), nrgb(0xe0, 0xc2, 0x92), nrgb(0xc4, 0xa0, 0x70)}
	earT      = tones{nrgb(0xff, 0xc0, 0xc4), nrgb(0xf7, 0xa3, 0xa8), nrgb(0xd9, 0x82, 0x8a)}
)

// shape — фигура в локальных координатах позы.
type shape interface {
	bounds() (x0, y0, x1, y1 float64)
	in(x, y float64) bool
	// tone — 0 свет, 1 основной, 2 тень; lx — направление света по x (−1 слева, +1 справа).
	tone(x, y, lx float64) int
}

func toneByK(k float64) int {
	switch {
	case k < -0.4:
		return 0
	case k > 0.3:
		return 2
	}
	return 1
}

type ellipse struct{ cx, cy, rx, ry, rot float64 }

func (e ellipse) uv(x, y float64) (u, v float64) {
	dx, dy := x-e.cx, y-e.cy
	s, c := math.Sincos(e.rot)
	return (dx*c + dy*s) / e.rx, (-dx*s + dy*c) / e.ry
}

func (e ellipse) bounds() (float64, float64, float64, float64) {
	r := math.Max(e.rx, e.ry)
	return e.cx - r, e.cy - r, e.cx + r, e.cy + r
}

func (e ellipse) in(x, y float64) bool {
	u, v := e.uv(x, y)
	return u*u+v*v <= 1
}

func (e ellipse) tone(x, y, lx float64) int {
	dx, dy := (x-e.cx)/e.rx, (y-e.cy)/e.ry
	return toneByK(-lx*dx*1.0 + dy*0.75)
}

type capsule struct{ ax, ay, bx, by, ra, rb float64 }

func (c capsule) at(x, y float64) (t, d float64) {
	vx, vy := c.bx-c.ax, c.by-c.ay
	l2 := vx*vx + vy*vy
	if l2 > 0 {
		t = math.Max(0, math.Min(1, ((x-c.ax)*vx+(y-c.ay)*vy)/l2))
	}
	px, py := c.ax+vx*t, c.ay+vy*t
	return t, math.Hypot(x-px, y-py)
}

func (c capsule) bounds() (float64, float64, float64, float64) {
	r := math.Max(c.ra, c.rb)
	return math.Min(c.ax, c.bx) - r, math.Min(c.ay, c.by) - r, math.Max(c.ax, c.bx) + r, math.Max(c.ay, c.by) + r
}

func (c capsule) in(x, y float64) bool {
	t, d := c.at(x, y)
	return d <= c.ra+(c.rb-c.ra)*t
}

func (c capsule) tone(x, y, lx float64) int {
	t, d := c.at(x, y)
	r := c.ra + (c.rb-c.ra)*t
	vx, vy := c.bx-c.ax, c.by-c.ay
	l := math.Hypot(vx, vy)
	if l == 0 || r == 0 {
		return 1
	}
	nx, ny := -vy/l, vx/l // нормаль к оси
	if nx*(-lx)+ny*(-0.75) < 0 {
		nx, ny = -nx, -ny
	}
	px, py := c.ax+vx*t, c.ay+vy*t
	s := ((x-px)*nx + (y-py)*ny) / r // >0 — сторона к свету
	_ = d
	switch {
	case s > 0.35:
		return 0
	case s < -0.35:
		return 2
	}
	return 1
}

type tri struct{ x1, y1, x2, y2, x3, y3 float64 }

func (t tri) bounds() (float64, float64, float64, float64) {
	return math.Min(t.x1, math.Min(t.x2, t.x3)), math.Min(t.y1, math.Min(t.y2, t.y3)),
		math.Max(t.x1, math.Max(t.x2, t.x3)), math.Max(t.y1, math.Max(t.y2, t.y3))
}

func (t tri) in(x, y float64) bool {
	d1 := (x-t.x2)*(t.y1-t.y2) - (t.x1-t.x2)*(y-t.y2)
	d2 := (x-t.x3)*(t.y2-t.y3) - (t.x2-t.x3)*(y-t.y3)
	d3 := (x-t.x1)*(t.y3-t.y1) - (t.x3-t.x1)*(y-t.y1)
	neg := d1 < 0 || d2 < 0 || d3 < 0
	pos := d1 > 0 || d2 > 0 || d3 > 0
	return !(neg && pos)
}

func (t tri) tone(x, y, lx float64) int {
	cx, cy := (t.x1+t.x2+t.x3)/3, (t.y1+t.y2+t.y3)/3
	return toneByK((-lx*(x-cx) + (y-cy)*0.75) / 5)
}

// chain — цепочка капсул (хвост): одна фигура с общим контуром.
type chain []capsule

func (ch chain) bounds() (x0, y0, x1, y1 float64) {
	x0, y0, x1, y1 = math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, c := range ch {
		a, b, cc, d := c.bounds()
		x0, y0, x1, y1 = math.Min(x0, a), math.Min(y0, b), math.Max(x1, cc), math.Max(y1, d)
	}
	return
}

func (ch chain) idx(x, y float64) int {
	for i, c := range ch {
		if c.in(x, y) {
			return i
		}
	}
	return -1
}

func (ch chain) in(x, y float64) bool { return ch.idx(x, y) >= 0 }

func (ch chain) tone(x, y, lx float64) int {
	if i := ch.idx(x, y); i >= 0 {
		return ch[i].tone(x, y, lx)
	}
	return 1
}

// painter рисует фигуры позы на холсте: ox, oy — левый верхний угол позы на холсте,
// mirror — зеркально (кот смотрит влево), width — ширина позы для зеркала.
type painter struct {
	c      *Canvas
	ox, oy float64
	mirror bool
	width  float64
}

func (p *painter) toLocal(px, py int) (x, y float64) {
	x = float64(px) + 0.5 - p.ox
	if p.mirror {
		x = p.width - x
	}
	return x, float64(py) + 0.5 - p.oy
}

// opts — как красить фигуру.
type opts struct {
	noOutline bool
	// over может подменить цвет пикселя (полоски, светлое брюшко); tone — тон фигуры.
	over func(x, y float64, tone int) (color.NRGBA, bool)
}

func (p *painter) draw(s shape, t tones, o opts) {
	lx := -0.55
	if p.mirror {
		lx = 0.55
	}
	bx0, by0, bx1, by1 := s.bounds()
	// границы в пикселях холста
	var cx0, cx1 int
	if p.mirror {
		cx0, cx1 = int(math.Floor(p.ox+p.width-bx1))-2, int(math.Ceil(p.ox+p.width-bx0))+2
	} else {
		cx0, cx1 = int(math.Floor(p.ox+bx0))-2, int(math.Ceil(p.ox+bx1))+2
	}
	cy0, cy1 := int(math.Floor(p.oy+by0))-2, int(math.Ceil(p.oy+by1))+2
	w, h := cx1-cx0+1, cy1-cy0+1
	mask := make([]bool, w*h)
	for y := cy0; y <= cy1; y++ {
		for x := cx0; x <= cx1; x++ {
			lxp, lyp := p.toLocal(x, y)
			mask[(y-cy0)*w+(x-cx0)] = s.in(lxp, lyp)
		}
	}
	at := func(x, y int) bool {
		if x < cx0 || x > cx1 || y < cy0 || y > cy1 {
			return false
		}
		return mask[(y-cy0)*w+(x-cx0)]
	}
	if !o.noOutline {
		for y := cy0; y <= cy1; y++ {
			for x := cx0; x <= cx1; x++ {
				if !at(x, y) && (at(x-1, y) || at(x+1, y) || at(x, y-1) || at(x, y+1)) {
					p.c.set(x, y, cOut)
				}
			}
		}
	}
	for y := cy0; y <= cy1; y++ {
		for x := cx0; x <= cx1; x++ {
			if !at(x, y) {
				continue
			}
			lxp, lyp := p.toLocal(x, y)
			tone := s.tone(lxp, lyp, lx)
			col := t[tone]
			if o.over != nil {
				if c2, ok := o.over(lxp, lyp, tone); ok {
					col = c2
				}
			}
			p.c.set(x, y, col)
		}
	}
}

// dot — один пиксель в локальных координатах.
func (p *painter) dot(x, y float64, col color.NRGBA) {
	px := x
	if p.mirror {
		px = p.width - x - 1
	}
	p.c.set(int(math.Floor(p.ox+px)), int(math.Floor(p.oy+y)), col)
}

// line — линия в локальных координатах (алгоритм Брезенхэма).
func (p *painter) line(x0, y0, x1, y1 float64, col color.NRGBA) {
	ix0, iy0, ix1, iy1 := int(math.Floor(x0)), int(math.Floor(y0)), int(math.Floor(x1)), int(math.Floor(y1))
	dx, dy := absInt(ix1-ix0), -absInt(iy1-iy0)
	sx, sy := 1, 1
	if ix0 > ix1 {
		sx = -1
	}
	if iy0 > iy1 {
		sy = -1
	}
	err := dx + dy
	for {
		p.dot(float64(ix0), float64(iy0), col)
		if ix0 == ix1 && iy0 == iy1 {
			return
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			ix0 += sx
		}
		if e2 <= dx {
			err += dx
			iy0 += sy
		}
	}
}

func absInt(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

// ik — двухзвенная нога: от сустава (hx, hy) до стопы (fx, fy) через колено; bend —
// в какую сторону оно смотрит (+1 — влево от направления «сустав → стопа»).
func ik(hx, hy, fx, fy, l1, l2, bend float64) (kx, ky, ffx, ffy float64) {
	dx, dy := fx-hx, fy-hy
	d := math.Hypot(dx, dy)
	if d < 0.001 {
		return hx, hy, fx, fy
	}
	maxD := l1 + l2 - 0.05
	if d > maxD {
		fx, fy = hx+dx/d*maxD, hy+dy/d*maxD
		dx, dy, d = fx-hx, fy-hy, maxD
	}
	minD := math.Abs(l1-l2) + 0.05
	if d < minD {
		d = minD
	}
	a := (l1*l1 - l2*l2 + d*d) / (2 * d)
	h := math.Sqrt(math.Max(0, l1*l1-a*a))
	ux, uy := dx/math.Hypot(dx, dy), dy/math.Hypot(dx, dy)
	mx, my := hx+ux*a, hy+uy*a
	return mx + bend*(-uy)*h, my + bend*ux*h, fx, fy
}
