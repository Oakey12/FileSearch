// Package cat — модель и рисование рыжего кота в стиле 16-bit без привязки к окну.
//
// Поведение считается тиками (10 в секунду). Координаты — «клетки кота» вдоль пола,
// пол на y = 0; позы рисуются с точностью до половины клетки. Рисует кота Render
// в буфер с прозрачностью, а показывает его пакет catwin.
package cat

import (
	"image/color"
	"math"
	"math/rand"
)

// Размер позы в клетках.
const (
	SprW = 28
	SprH = 24
)

// Scruff — где у кота, которого несут за шкирку, загривок (в координатах позы Carried).
var Scruff = struct{ X, Y float64 }{13.5, 3}

// Mode — чем занят кот.
type Mode int

const (
	Sit Mode = iota
	Walk
	Play
	Wash
	Yawn
	Sleep
	Stretch
	Carried
	Fall
	Land
)

type phase int

const (
	chase phase = iota
	stalk
	leap
	watch
)

// Ball — клубок для игры.
type Ball struct{ X, VX float64 }

// BallSize — размер клубка в клетках.
const BallSize = 5

const edge = 2

// Cat — состояние кота.
type Cat struct {
	Width    float64 // ширина мира (экрана) в клетках
	Mode     Mode
	X        float64 // левый край позы
	Lift     float64 // высота над полом (прыжок, падение)
	VY       float64 // скорость падения
	Sway     float64 // раскачивание, когда кота несут (−2…2)
	Facing   int     // 1 — вправо, −1 — влево
	Tick     int
	ModeTick int
	Idle     int // сколько тиков кот сидит без команды
	Yawned   bool
	Washed   bool
	Ball     *Ball
	phase    phase
	phTicks  int
	leapFrom float64
	leapTo   float64
}

// New возвращает сидящего кота посередине мира шириной width клеток.
func New(width float64) *Cat {
	return &Cat{Width: width, Facing: 1, X: (width - SprW) / 2}
}

func (c *Cat) setMode(m Mode) {
	c.Mode = m
	c.ModeTick = 0
	if m != Fall {
		c.Lift = 0
	}
	if m != Play {
		c.Ball = nil
	}
}

func (c *Cat) rest() {
	c.setMode(Sit)
	c.Idle = 0
	c.Yawned = false
	c.Washed = false
}

// Command — команда из меню: Sit, Walk или Play.
func (c *Cat) Command(m Mode, rnd func() float64) {
	if rnd == nil {
		rnd = rand.Float64
	}
	c.rest()
	c.setMode(m)
	if m == Play {
		// Клубок падает в паре шагов от кота, с той стороны, где есть место.
		right := c.X+SprW+30 < c.Width
		gap := 12 + rnd()*8
		if right {
			c.Ball = &Ball{X: c.X + SprW + gap}
		} else {
			c.Ball = &Ball{X: c.X - gap - BallSize}
		}
		c.phase = chase
		c.phTicks = 0
	}
}

// Poke — клик по коту: спящий просыпается и потягивается, умывающийся отвлекается.
func (c *Cat) Poke() {
	switch c.Mode {
	case Sleep:
		c.setMode(Stretch)
		c.Idle = 0
		c.Yawned, c.Washed = false, false
	case Wash, Yawn:
		c.rest()
	}
}

// Carry — кота взяли за шкирку.
func (c *Cat) Carry() {
	c.setMode(Carried)
	c.Sway = 0
}

// Drop — кота отпустили на высоте height клеток над его новым полом.
func (c *Cat) Drop(height float64) {
	c.setMode(Fall)
	c.Lift = math.Max(0, height)
	c.VY = 0
}

func (c *Cat) paw() float64 {
	if c.Facing > 0 {
		return c.X + 24
	}
	return c.X + 4
}

func (c *Cat) maxX() float64 { return c.Width - SprW - edge }

func (c *Cat) clampX(x float64) float64 { return math.Max(edge, math.Min(c.maxX(), x)) }

// Step — один тик поведения.
func (c *Cat) Step(rnd func() float64) {
	if rnd == nil {
		rnd = rand.Float64
	}
	c.Tick++
	c.ModeTick++
	switch c.Mode {
	case Sit:
		// Сам себе развлечение: зевнул, умылся, уснул.
		c.Idle++
		switch {
		case c.Idle >= 170:
			c.setMode(Sleep)
		case c.Idle >= 90 && !c.Washed:
			c.Washed = true
			c.setMode(Wash)
		case c.Idle >= 40 && !c.Yawned:
			c.Yawned = true
			c.setMode(Yawn)
		}
	case Yawn:
		c.Idle++
		if c.ModeTick >= 16 {
			c.setMode(Sit)
		}
	case Wash:
		c.Idle++
		if c.ModeTick >= 40 {
			c.setMode(Sit)
		}
	case Stretch:
		if c.ModeTick >= 22 {
			c.rest()
		}
	case Fall:
		c.VY += 0.7
		c.Lift -= c.VY
		if c.Lift <= 0 {
			c.setMode(Land)
		}
	case Land:
		if c.ModeTick >= 4 {
			c.rest()
		}
	case Walk:
		c.X += float64(c.Facing)
		if c.X >= c.maxX() {
			c.X = c.maxX()
			c.Facing = -1
		}
		if c.X <= edge {
			c.X = edge
			c.Facing = 1
		}
		if c.ModeTick >= 300 {
			c.rest() // нагулялся
		}
	case Play:
		c.stepPlay(rnd)
		if c.ModeTick >= 260 && c.phase != leap {
			c.rest() // наигрался
		}
	}
}

func (c *Cat) faceBall(centre float64) {
	if centre >= c.X+SprW/2 {
		c.Facing = 1
	} else {
		c.Facing = -1
	}
}

func (c *Cat) stepPlay(rnd func() float64) {
	ball := c.Ball
	c.phTicks++
	centre := ball.X + BallSize/2
	switch c.phase {
	case chase:
		c.faceBall(centre)
		if math.Abs(c.paw()-centre) <= 20 {
			c.phase, c.phTicks = stalk, 0
			break
		}
		c.X = c.clampX(c.X + float64(c.Facing)*2)
		if c.X == edge || c.X == c.maxX() {
			c.phase, c.phTicks = stalk, 0
		}
	case stalk:
		c.faceBall(centre)
		if c.phTicks >= 12 {
			c.phase, c.phTicks = leap, 0
			c.leapFrom = c.X
			if c.Facing > 0 {
				c.leapTo = c.clampX(ball.X - 22)
			} else {
				c.leapTo = c.clampX(ball.X - 1)
			}
		}
	case leap:
		t := math.Min(1, float64(c.phTicks)/6)
		c.X = c.leapFrom + (c.leapTo-c.leapFrom)*t
		c.Lift = jsRound(math.Sin(math.Pi*t) * 10)
		if t >= 1 {
			c.Lift = 0
			ball.VX = float64(c.Facing) * (1.5 + rnd()*1.5) // недалеко: клубок должен оставаться у кота в окне
			c.phase, c.phTicks = watch, 0
		}
	case watch:
		ball.X += ball.VX
		ball.VX *= 0.88
		if ball.X < 1 {
			ball.X = 1
			ball.VX = -ball.VX
		}
		if ball.X > c.Width-BallSize-1 {
			ball.X = c.Width - BallSize - 1
			ball.VX = -ball.VX
		}
		c.faceBall(centre)
		if math.Abs(ball.VX) < 0.2 {
			ball.VX = 0
			c.phase, c.phTicks = chase, 0
		}
	}
}

// HitCat — попадает ли точка в стоящего на полу кота (y от пола, вверх — минус).
func (c *Cat) HitCat(x, y float64) bool {
	return x >= c.X-2 && x <= c.X+SprW+2 && y >= -SprH-c.Lift-3 && y <= 2
}

// jsRound — округление как в JavaScript (половинки вверх), чтобы позы совпали с прежними.
func jsRound(x float64) float64 { return math.Floor(x + 0.5) }

// ---------- Позы ----------

// Px — прямоугольник позы: x, y, ширина, высота, цвет. Шаг — половина клетки.
type Px struct {
	X, Y, W, H float64
	Col        color.NRGBA
}

// Sprite — поза: слои с контуром (верхние поверх нижних) и детали без контура.
type Sprite struct {
	Layers  [][]Px
	Details []Px
}

func hex(r, g, b uint8) color.NRGBA { return color.NRGBA{r, g, b, 255} }

// Палитра кота.
var (
	cOut       = hex(0x2b, 0x14, 0x08)
	cFur       = hex(0xf2, 0x8c, 0x28) // шерсть: у крупных частей сверху блик, снизу тень
	cFurLight  = hex(0xff, 0xb8, 0x62)
	cShade     = hex(0xc8, 0x64, 0x1a)
	cStripe    = hex(0xb4, 0x50, 0x0f)
	cCream     = hex(0xff, 0xf0, 0xd4)
	cCreamSh   = hex(0xf1, 0xc8, 0x96)
	cEye       = hex(0x86, 0xd6, 0x4f)
	cEyeDark   = hex(0x3f, 0x8f, 0x2a)
	cPupil     = hex(0x14, 0x11, 0x0f)
	cWhite     = hex(0xff, 0xff, 0xff)
	cNose      = hex(0xff, 0x8f, 0xa0)
	cEarIn     = hex(0xf7, 0xa3, 0xa8)
	cMouth     = hex(0x5a, 0x1a, 0x1a)
	cWhisker   = color.NRGBA{255, 246, 232, 217} // alpha 0.85
	cBall      = hex(0x5b, 0x8d, 0xef)
	cBallLight = hex(0xa9, 0xc4, 0xff)
	cBallDark  = hex(0x3a, 0x63, 0xc0)
	cZzz       = hex(0x9c, 0xc8, 0xff)
	cFist      = hex(0xf3, 0xd2, 0xb3)
)

type eyeKind int

const (
	eyesOpen eyeKind = iota
	eyesClosed
	eyesWide
	eyesSleepy
)

var swayTable = [4]float64{0, 1, 0, -1}

type face struct {
	hx, hy float64 // левый край и верх головы
	eyes   eyeKind
	front  bool    // анфас (сидит, висит), иначе профиль вправо
	mouth  float64 // насколько открыт рот (зевок), 0…1
}

// faceDetails — мордочка: уши изнутри, полоски на лбу, глаза с бликом, нос, щёки, усы.
func faceDetails(f face) []Px {
	hx, hy := f.hx, f.hy
	var d []Px
	if f.front {
		// голова 11 клеток шириной, глаза симметрично
		d = append(d, Px{hx + 1.5, hy - 1.5, 1.5, 1.5, cEarIn}, Px{hx + 8, hy - 1.5, 1.5, 1.5, cEarIn})
		d = append(d, Px{hx + 4, hy, 0.5, 2, cStripe}, Px{hx + 5.5, hy, 0.5, 1.5, cStripe}, Px{hx + 7, hy, 0.5, 2, cStripe})
		d = append(d, Px{hx + 2.5, hy + 6, 6, 2.5, cCream}, Px{hx + 3, hy + 8, 5, 0.5, cCreamSh})
		d = eyeDetails(d, f.eyes, [][2]float64{{hx + 2, hy + 4}, {hx + 7, hy + 4}})
		d = append(d, Px{hx + 4.5, hy + 6, 2, 1, cNose}, Px{hx + 5, hy + 7, 1, 0.5, cOut})
		if f.mouth > 0 {
			d = mouthDetails(d, hx+5.5, hy+7.5, f.mouth)
		}
		d = append(d, Px{hx - 2, hy + 6.5, 3, 0.5, cWhisker}, Px{hx - 1.5, hy + 7.5, 2.5, 0.5, cWhisker})
		d = append(d, Px{hx + 10, hy + 6.5, 3, 0.5, cWhisker}, Px{hx + 10, hy + 7.5, 2.5, 0.5, cWhisker})
	} else {
		// профиль вправо, голова 10 клеток
		d = append(d, Px{hx + 1.5, hy - 1.5, 1.5, 1.5, cEarIn}, Px{hx + 7.5, hy - 1.5, 1.5, 1.5, cEarIn})
		d = append(d, Px{hx + 4, hy, 0.5, 2, cStripe}, Px{hx + 5, hy, 0.5, 1.5, cStripe}, Px{hx + 6, hy, 0.5, 2, cStripe})
		d = append(d, Px{hx + 4, hy + 5.5, 6, 2.5, cCream}, Px{hx + 4.5, hy + 7.5, 5, 0.5, cCreamSh})
		d = eyeDetails(d, f.eyes, [][2]float64{{hx + 2, hy + 3}, {hx + 7, hy + 3}})
		d = append(d, Px{hx + 8.5, hy + 5, 1.5, 1, cNose})
		if f.mouth > 0 {
			d = mouthDetails(d, hx+8, hy+6.5, f.mouth)
		}
		d = append(d, Px{hx + 9.5, hy + 6, 3, 0.5, cWhisker}, Px{hx + 9.5, hy + 7, 2.5, 0.5, cWhisker})
	}
	return d
}

func eyeDetails(d []Px, kind eyeKind, at [][2]float64) []Px {
	for _, p := range at {
		x, y := p[0], p[1]
		switch kind {
		case eyesClosed:
			d = append(d, Px{x, y + 1, 2, 0.5, cOut}, Px{x - 0.5, y + 0.5, 0.5, 0.5, cOut}, Px{x + 2, y + 0.5, 0.5, 0.5, cOut})
		case eyesSleepy:
			d = append(d, Px{x, y + 0.5, 2, 1.5, cEye}, Px{x, y + 1.5, 2, 0.5, cEyeDark}, Px{x + 0.5, y + 0.5, 1, 1.5, cPupil}, Px{x, y, 2, 0.5, cOut})
		default:
			h := 2.0
			if kind == eyesWide {
				h = 2.5
			}
			d = append(d, Px{x, y, 2, h, cEye}, Px{x, y + h - 0.5, 2, 0.5, cEyeDark})
			if kind == eyesWide {
				d = append(d, Px{x + 0.5, y + 1, 1, 1, cPupil})
			} else {
				d = append(d, Px{x + 0.5, y, 1, h, cPupil})
			}
			d = append(d, Px{x + 0.5, y, 0.5, 0.5, cWhite})
		}
	}
	return d
}

func mouthDetails(d []Px, cx, y, open float64) []Px {
	h := 0.5 + jsRound(open*4)/2
	d = append(d, Px{cx - 1.5, y, 3, h, cMouth})
	if h >= 1.5 {
		d = append(d, Px{cx - 1, y + h - 0.5, 2, 0.5, cNose}) // язык
	}
	if h >= 1 {
		d = append(d, Px{cx - 1.5, y, 0.5, 0.5, cWhite}, Px{cx + 1, y, 0.5, 0.5, cWhite}) // клыки
	}
	return d
}

// paws — светлые подушечки на лапах, которые стоят на полу.
func paws(legs []Px, bottom float64) []Px {
	var out []Px
	for _, l := range legs {
		if l.Y+l.H >= bottom-0.5 {
			out = append(out, Px{l.X, l.Y + l.H - 0.5, l.W, 0.5, cCream})
		}
	}
	return out
}

type standOpts struct {
	crouch, leap, gallop, walk, fall, squash bool
	sway                                     float64
	eyes                                     eyeKind
}

func standing(f int, o standOpts) Sprite {
	crouch := o.crouch || o.squash
	by, bh := 12.0, 7.0
	if crouch {
		by, bh = 14, 6
	}
	bx, bw := 4.0, 16.0
	if o.squash {
		bx, bw = 3, 18
	}
	hy := 5.0
	if crouch {
		hy = 9
	}
	legTop := by + bh
	var legs []Px
	switch {
	case o.leap:
		legs = []Px{{0, by + 3, 5, 2, cFur}, {1, by + 5, 4, 2, cFur}, {20, by + 4, 6, 2, cFur}, {19, by + 6, 6, 2, cFur}}
	case o.gallop:
		legs = []Px{{8, legTop, 2, 4, cFur}, {10, legTop, 2, 4.5, cFur}, {13, legTop, 2, 4.5, cFur}, {15, legTop, 2, 4, cFur}}
	case o.fall:
		legs = []Px{{3, legTop - 1, 2, 5, cFur}, {7, legTop, 2, 4, cFur}, {15, legTop, 2, 4, cFur}, {19, legTop - 1, 2, 5, cFur}}
	default:
		dx := [4]float64{}
		var up []int
		if o.walk {
			dx = [][4]float64{{1, -1, -1, 1}, {0, 0, 0, 0}, {-1, 1, 1, -1}, {0, 0, 0, 0}}[f%4]
			up = [][]int{{}, {0, 2}, {}, {1, 3}}[f%4]
		}
		for i, x := range []float64{5, 8, 14, 17} {
			h := SprH - legTop
			for _, u := range up {
				if u == i {
					h--
				}
			}
			legs = append(legs, Px{x + dx[i], legTop, 2, h, cFur})
		}
	}
	var tail []Px
	switch {
	case crouch || o.leap:
		tail = []Px{{0, by + 1, 5, 2, cFur}, {math.Max(0, o.sway), by - 1 + math.Min(0, o.sway), 2, 3, cFur}}
	case o.fall:
		tail = []Px{{1, 2, 2, 11, cFur}, {2, 12, 3, 2, cFur}}
	default:
		tail = []Px{{2, 6, 2, 8, cFur}, {2, 12, 3, 2, cFur}, {1 + o.sway, 3, 2, 4, cFur}}
	}
	layer := append([]Px{}, tail...)
	layer = append(layer, Px{bx, by, bw, bh, cFur})
	layer = append(layer, legs...)
	layer = append(layer, Px{17, hy, 10, 9, cFur}, Px{18, hy - 2, 3, 2, cFur}, Px{24, hy - 2, 3, 2, cFur})

	t0 := tail[0]
	details := []Px{
		{8, by, 1, 2.5, cStripe}, {11, by, 1, 3, cStripe}, {14, by, 1, 2.5, cStripe},
		{6, by + bh - 2, 11, 1.5, cCream},
	}
	if t0.H > t0.W {
		details = append(details, Px{t0.X, t0.Y + 2, 2, 0.5, cStripe}, Px{t0.X, t0.Y + 4.5, 2, 0.5, cStripe})
	}
	if !o.leap {
		details = append(details, paws(legs, SprH)...)
	}
	details = append(details, faceDetails(face{hx: 17, hy: hy, eyes: o.eyes})...)
	return Sprite{Layers: [][]Px{layer}, Details: details}
}

type sitOpts struct {
	eyes  eyeKind
	wash  bool
	mouth float64
	tilt  float64
	tip   float64
}

func sitting(f int, o sitOpts) Sprite {
	hy := 3.0 - o.tilt
	if o.wash && f%2 == 1 {
		hy++
	}
	tipY := 19 + o.tip
	legs := []Px{{12, 18, 2, 6, cFur}, {16, 18, 2, 6, cFur}}
	layer := []Px{
		{1, 22, 7, 2, cFur},
		{0, tipY, 2, 24 - tipY, cFur},
		{6, 14, 12, 10, cFur},
		{11, 10, 8, 10, cFur},
	}
	layer = append(layer, legs...)
	layer = append(layer, Px{10, hy, 11, 9, cFur}, Px{11, hy - 2, 3, 2, cFur}, Px{18, hy - 2, 3, 2, cFur})
	layers := [][]Px{layer}
	pawY := 8.0 + float64(f%2)
	if o.wash {
		layers = append(layers, []Px{{16, pawY, 3, 8, cFur}}) // лапка у мордочки
	}
	details := []Px{
		{13, 12, 4, 6, cCream}, {13.5, 17.5, 3, 0.5, cCreamSh},
		{14, 19, 0.5, 5, cShade},
	}
	details = append(details, paws(legs, SprH)...)
	details = append(details,
		Px{7, 16, 1, 3, cStripe}, Px{9, 15, 1, 3, cStripe}, Px{3, 22, 0.5, 2, cStripe}, Px{5, 22, 0.5, 2, cStripe})
	details = append(details, faceDetails(face{hx: 10, hy: hy, eyes: o.eyes, front: true, mouth: o.mouth})...)
	if o.wash {
		details = append(details, Px{16, pawY, 3, 1, cCream})
		if f%2 == 0 {
			details = append(details, Px{15, hy + 8.5, 1, 0.5, cNose}) // язычок
		}
	}
	return Sprite{Layers: layers, Details: details}
}

func sleeping(f int) Sprite {
	breathe := 0.0
	if f%2 == 1 {
		breathe = 0.5
	}
	layers := [][]Px{
		{
			{3, 15 - breathe, 20, 9 + breathe, cFur},
			{16, 14, 9, 8, cFur},
			{17, 12, 2, 2, cFur},
			{22, 12, 2, 2, cFur},
		},
		{{4, 21, 15, 2, cFur}, {17, 20, 2, 2, cFur}}, // хвост обвивает лапы
	}
	details := []Px{
		{7, 16 - breathe, 1, 2.5, cStripe}, {10, 15 - breathe, 1, 3, cStripe}, {13, 16 - breathe, 1, 2.5, cStripe},
		{17.5, 12.5, 1, 1, cEarIn}, {22.5, 12.5, 1, 1, cEarIn},
		{18, 18, 2, 0.5, cOut}, {22, 18, 2, 0.5, cOut},
		{19.5, 19, 4, 2, cCream},
		{21, 19, 1, 0.5, cNose},
		{6, 21, 0.5, 2, cStripe}, {9, 21, 0.5, 2, cStripe}, {12, 21, 0.5, 2, cStripe},
	}
	return Sprite{Layers: layers, Details: details}
}

func stretching(f int) Sprite {
	legs := []Px{{2, 19, 2, 5, cFur}, {7, 19, 2, 5, cFur}}
	layer := []Px{
		{0, 2, 2, 9, cFur}, {1, 1, 2, 2, cFur}, // хвост трубой
		{1, 10, 10, 9, cFur},
	}
	layer = append(layer, legs...)
	layer = append(layer,
		Px{8, 14, 10, 6, cFur},
		Px{14, 21, 11, 3, cFur},
		Px{18, 11, 9, 8, cFur},
		Px{19, 9, 3, 2, cFur},
		Px{24, 9, 3, 2, cFur})
	details := []Px{
		{3, 11, 1, 3, cStripe}, {6, 10.5, 1, 3, cStripe}, {11, 14.5, 1, 2.5, cStripe},
		{0, 4, 2, 0.5, cStripe}, {0, 7, 2, 0.5, cStripe},
		{10, 18.5, 8, 1.5, cCream},
	}
	details = append(details, paws(legs, SprH)...)
	details = append(details,
		Px{23.5, 23, 1.5, 1, cCream}, Px{24, 21.5, 0.5, 1, cShade}, Px{25, 21.5, 0.5, 1, cShade})
	mouth := 0.0
	if f%6 < 3 {
		mouth = 0.6
	}
	details = append(details, faceDetails(face{hx: 17, hy: 11, eyes: eyesClosed, mouth: mouth})...)
	return Sprite{Layers: [][]Px{layer}, Details: details}
}

// hanging — кота несут за шкирку: висит анфас, лапки и хвост качаются против движения.
func hanging(sway float64, f int) Sprite {
	s := jsRound(-sway*2) / 2
	mid := jsRound(s) / 2
	layer := []Px{
		{13 + s, 24, 2, 6, cFur}, {13 + s + swayTable[f%4]*0.5, 29, 2, 2, cFur}, // хвост
		{10 + mid, 11, 8, 12, cFur},
		{9 + s, 18, 10, 6, cFur},
		{10 + s, 23, 2, 4, cFur}, {16 + s, 23, 2, 4, cFur}, // задние лапы
		{9 + mid, 12, 2, 6, cFur}, {17 + mid, 12, 2, 6, cFur}, // передние
		{8, 3, 11, 9, cFur},
		{8, 1, 3, 2, cFur}, {16, 1, 3, 2, cFur},
	}
	details := []Px{
		{12 + mid, 13, 4, 7, cCream}, {12.5 + mid, 19.5, 3, 0.5, cCreamSh},
		{10 + s, 26.5, 2, 0.5, cCream}, {16 + s, 26.5, 2, 0.5, cCream},
		{9 + mid, 17.5, 2, 0.5, cCream}, {17 + mid, 17.5, 2, 0.5, cCream},
		{13 + s, 26, 2, 0.5, cStripe}, {13 + s, 28, 2, 0.5, cStripe},
		{10 + s, 19.5, 1, 2, cStripe}, {17 + s, 19.5, 1, 2, cStripe},
	}
	details = append(details, faceDetails(face{hx: 8, hy: 3, eyes: eyesSleepy, front: true})...)
	return Sprite{Layers: [][]Px{layer}, Details: details}
}

// Sprite возвращает позу кота в его нынешнем состоянии.
func (c *Cat) Sprite() Sprite {
	t := c.Tick
	open := eyesOpen
	if t%40 < 2 {
		open = eyesClosed // моргнул
	}
	switch c.Mode {
	case Walk:
		return standing(t/2, standOpts{walk: true, sway: swayTable[(t/3)%4], eyes: open})
	case Sleep:
		return sleeping(t / 8)
	case Wash:
		return sitting(t/3, sitOpts{eyes: eyesClosed, wash: true, tip: swayTable[(t/4)%4]})
	case Yawn:
		k := math.Sin(math.Pi * math.Min(1, float64(c.ModeTick)/16))
		tilt := 0.0
		if k > 0.5 {
			tilt = 1
		}
		return sitting(0, sitOpts{eyes: eyesClosed, mouth: k, tilt: tilt, tip: swayTable[(t/4)%4]})
	case Stretch:
		return stretching(c.ModeTick)
	case Carried:
		return hanging(c.Sway, t/2)
	case Fall:
		return standing(0, standOpts{fall: true, eyes: eyesWide})
	case Land:
		return standing(0, standOpts{squash: true, eyes: eyesWide})
	case Play:
		switch c.phase {
		case leap:
			return standing(0, standOpts{leap: true, eyes: eyesWide})
		case stalk:
			return standing(0, standOpts{crouch: true, sway: swayTable[t%4], eyes: eyesWide})
		case chase:
			if t%2 == 1 {
				return standing(0, standOpts{gallop: true, sway: 1, eyes: eyesOpen})
			}
			return standing(0, standOpts{leap: true, sway: -1, eyes: eyesOpen})
		}
		return standing(0, standOpts{sway: swayTable[(t/2)%4], eyes: open})
	}
	return sitting(0, sitOpts{eyes: open, tip: swayTable[(t/4)%4]})
}
