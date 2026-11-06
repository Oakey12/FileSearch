package cat

import (
	"image/color"
	"math"
)

// Рыжий кот в стиле 16-bit: позы собраны из прямоугольников (шаг — половина клетки),
// каждый слой обводится контуром, поверх лежат детали без контура. Клетка = fine
// пикселей холста.

const (
	fine = 2
	// Размер позы в клетках.
	spriteW = 28
	spriteH = 24
)

// spriteOX, spriteOY — где поза лежит внутри окна 64×52 пикселей, которое занимает
// кот в модели: по центру и прижата к полу.
const (
	spriteOX = (SprW - spriteW*fine) / 2
	spriteOY = SprH - spriteH*fine
)

// px — прямоугольник позы: x, y, ширина, высота (клетки), цвет.
type px struct {
	X, Y, W, H float64
	Col        color.NRGBA
}

// Sprite — поза: слои с контуром (верхние поверх нижних) и детали без контура.
type Sprite struct {
	Layers  [][]px
	Details []px
}

func nrgb(r, g, b uint8) color.NRGBA { return color.NRGBA{r, g, b, 255} }

// Палитра кота.
var (
	cOut       = nrgb(0x2b, 0x14, 0x08)
	cFur       = nrgb(0xf2, 0x8c, 0x28) // шерсть: у крупных частей сверху блик, снизу тень
	cFurLight  = nrgb(0xff, 0xb8, 0x62)
	cShade     = nrgb(0xc8, 0x64, 0x1a)
	cStripe    = nrgb(0xb4, 0x50, 0x0f)
	cCream     = nrgb(0xff, 0xf0, 0xd4)
	cCreamSh   = nrgb(0xf1, 0xc8, 0x96)
	cEye       = nrgb(0x86, 0xd6, 0x4f)
	cEyeDark   = nrgb(0x3f, 0x8f, 0x2a)
	cPupil     = nrgb(0x14, 0x11, 0x0f)
	cWhite     = nrgb(0xff, 0xff, 0xff)
	cNose      = nrgb(0xff, 0x8f, 0xa0)
	cEarIn     = nrgb(0xf7, 0xa3, 0xa8)
	cMouth     = nrgb(0x5a, 0x1a, 0x1a)
	cWhisker   = color.NRGBA{255, 246, 232, 217} // alpha 0.85
	cBall      = nrgb(0x5b, 0x8d, 0xef)
	cBallLight = nrgb(0xa9, 0xc4, 0xff)
	cBallDark  = nrgb(0x3a, 0x63, 0xc0)
	cZzz       = nrgb(0x9c, 0xc8, 0xff)
	cFist      = nrgb(0xf3, 0xd2, 0xb3)
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
	hx, hy     float64 // левый край и верх головы
	eyes       eyeKind
	front      bool    // анфас (сидит, висит), иначе профиль вправо
	mouth      float64 // насколько открыт рот (зевок), 0…1
	gaze       float64 // куда смотрит: −1 влево … 1 вправо (зрачки)
	earL, earR bool    // ухо дёрнулось
}

// pupilShift — на сколько клетки сдвигается зрачок.
func pupilShift(gaze float64) float64 {
	switch {
	case gaze > 0.33:
		return 0.5
	case gaze < -0.33:
		return -0.5
	}
	return 0
}

// faceDetails — мордочка: уши изнутри, полоски на лбу, глаза с бликом, нос, щёки, усы.
func faceDetails(f face) []px {
	hx, hy := f.hx, f.hy
	var d []px
	if f.front {
		// голова 11 клеток шириной, глаза симметрично
		yl, yr := hy-1.5, hy-1.5
		if f.earL {
			yl += 0.5
		}
		if f.earR {
			yr += 0.5
		}
		d = append(d, px{hx + 1.5, yl, 1.5, 1.5, cEarIn}, px{hx + 8, yr, 1.5, 1.5, cEarIn})
		d = append(d, px{hx + 4, hy, 0.5, 2, cStripe}, px{hx + 5.5, hy, 0.5, 1.5, cStripe}, px{hx + 7, hy, 0.5, 2, cStripe})
		d = append(d, px{hx + 2.5, hy + 6, 6, 2.5, cCream}, px{hx + 3, hy + 8, 5, 0.5, cCreamSh})
		d = eyeDetails(d, f.eyes, f.gaze, [][2]float64{{hx + 2, hy + 4}, {hx + 7, hy + 4}})
		d = append(d, px{hx + 4.5, hy + 6, 2, 1, cNose}, px{hx + 5, hy + 7, 1, 0.5, cOut})
		if f.mouth > 0 {
			d = mouthDetails(d, hx+5.5, hy+7.5, f.mouth)
		}
		d = append(d, px{hx - 2, hy + 6.5, 3, 0.5, cWhisker}, px{hx - 1.5, hy + 7.5, 2.5, 0.5, cWhisker})
		d = append(d, px{hx + 10, hy + 6.5, 3, 0.5, cWhisker}, px{hx + 10, hy + 7.5, 2.5, 0.5, cWhisker})
	} else {
		// профиль вправо, голова 10 клеток
		d = append(d, px{hx + 1.5, hy - 1.5, 1.5, 1.5, cEarIn}, px{hx + 7.5, hy - 1.5, 1.5, 1.5, cEarIn})
		d = append(d, px{hx + 4, hy, 0.5, 2, cStripe}, px{hx + 5, hy, 0.5, 1.5, cStripe}, px{hx + 6, hy, 0.5, 2, cStripe})
		d = append(d, px{hx + 4, hy + 5.5, 6, 2.5, cCream}, px{hx + 4.5, hy + 7.5, 5, 0.5, cCreamSh})
		d = eyeDetails(d, f.eyes, 0, [][2]float64{{hx + 2, hy + 3}, {hx + 7, hy + 3}})
		d = append(d, px{hx + 8.5, hy + 5, 1.5, 1, cNose})
		if f.mouth > 0 {
			d = mouthDetails(d, hx+8, hy+6.5, f.mouth)
		}
		d = append(d, px{hx + 9.5, hy + 6, 3, 0.5, cWhisker}, px{hx + 9.5, hy + 7, 2.5, 0.5, cWhisker})
	}
	return d
}

func eyeDetails(d []px, kind eyeKind, gaze float64, at [][2]float64) []px {
	shift := pupilShift(gaze)
	for _, p := range at {
		x, y := p[0], p[1]
		switch kind {
		case eyesClosed:
			d = append(d, px{x, y + 1, 2, 0.5, cOut}, px{x - 0.5, y + 0.5, 0.5, 0.5, cOut}, px{x + 2, y + 0.5, 0.5, 0.5, cOut})
		case eyesSleepy:
			d = append(d, px{x, y + 0.5, 2, 1.5, cEye}, px{x, y + 1.5, 2, 0.5, cEyeDark}, px{x + 0.5, y + 0.5, 1, 1.5, cPupil}, px{x, y, 2, 0.5, cOut})
		default:
			h := 2.0
			if kind == eyesWide {
				h = 2.5
			}
			d = append(d, px{x, y, 2, h, cEye}, px{x, y + h - 0.5, 2, 0.5, cEyeDark})
			if kind == eyesWide {
				d = append(d, px{x + 0.5 + shift, y + 1, 1, 1, cPupil})
			} else {
				d = append(d, px{x + 0.5 + shift, y, 1, h, cPupil})
			}
			d = append(d, px{x + 0.5, y, 0.5, 0.5, cWhite})
		}
	}
	return d
}

func mouthDetails(d []px, cx, y, open float64) []px {
	h := 0.5 + jsRound(open*4)/2
	d = append(d, px{cx - 1.5, y, 3, h, cMouth})
	if h >= 1.5 {
		d = append(d, px{cx - 1, y + h - 0.5, 2, 0.5, cNose}) // язык
	}
	if h >= 1 {
		d = append(d, px{cx - 1.5, y, 0.5, 0.5, cWhite}, px{cx + 1, y, 0.5, 0.5, cWhite}) // клыки
	}
	return d
}

// paws — светлые подушечки на лапах, которые стоят на полу.
func paws(legs []px, bottom float64) []px {
	var out []px
	for _, l := range legs {
		if l.Y+l.H >= bottom-0.5 {
			out = append(out, px{l.X, l.Y + l.H - 0.5, l.W, 0.5, cCream})
		}
	}
	return out
}

type standOpts struct {
	crouch, leap, gallop, walk, fall, squash bool
	sway                                     float64
	eyes                                     eyeKind
	drop                                     float64 // на сколько клеток опущена голова (принюхивается) или поднята (минус)
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
	hy += o.drop
	legTop := by + bh
	var legs []px
	switch {
	case o.leap:
		legs = []px{{0, by + 3, 5, 2, cFur}, {1, by + 5, 4, 2, cFur}, {20, by + 4, 6, 2, cFur}, {19, by + 6, 6, 2, cFur}}
	case o.gallop:
		legs = []px{{8, legTop, 2, 4, cFur}, {10, legTop, 2, 4.5, cFur}, {13, legTop, 2, 4.5, cFur}, {15, legTop, 2, 4, cFur}}
	case o.fall:
		legs = []px{{3, legTop - 1, 2, 5, cFur}, {7, legTop, 2, 4, cFur}, {15, legTop, 2, 4, cFur}, {19, legTop - 1, 2, 5, cFur}}
	default:
		dx := [4]float64{}
		var up []int
		if o.walk {
			dx = [][4]float64{{1, -1, -1, 1}, {0, 0, 0, 0}, {-1, 1, 1, -1}, {0, 0, 0, 0}}[f%4]
			up = [][]int{{}, {0, 2}, {}, {1, 3}}[f%4]
		}
		for i, x := range []float64{5, 8, 14, 17} {
			h := spriteH - legTop
			for _, u := range up {
				if u == i {
					h--
				}
			}
			legs = append(legs, px{x + dx[i], legTop, 2, h, cFur})
		}
	}
	var tail []px
	switch {
	case crouch || o.leap:
		tail = []px{{0, by + 1, 5, 2, cFur}, {math.Max(0, o.sway), by - 1 + math.Min(0, o.sway), 2, 3, cFur}}
	case o.fall:
		tail = []px{{1, 2, 2, 11, cFur}, {2, 12, 3, 2, cFur}}
	default:
		tail = []px{{2, 6, 2, 8, cFur}, {2, 12, 3, 2, cFur}, {1 + o.sway, 3, 2, 4, cFur}}
	}
	layer := append([]px{}, tail...)
	layer = append(layer, px{bx, by, bw, bh, cFur})
	layer = append(layer, legs...)
	layer = append(layer, px{17, hy, 10, 9, cFur}, px{18, hy - 2, 3, 2, cFur}, px{24, hy - 2, 3, 2, cFur})

	t0 := tail[0]
	details := []px{
		{8, by, 1, 2.5, cStripe}, {11, by, 1, 3, cStripe}, {14, by, 1, 2.5, cStripe},
		{6, by + bh - 2, 11, 1.5, cCream},
	}
	if t0.H > t0.W {
		details = append(details, px{t0.X, t0.Y + 2, 2, 0.5, cStripe}, px{t0.X, t0.Y + 4.5, 2, 0.5, cStripe})
	}
	if !o.leap {
		details = append(details, paws(legs, spriteH)...)
	}
	details = append(details, faceDetails(face{hx: 17, hy: hy, eyes: o.eyes})...)
	return Sprite{Layers: [][]px{layer}, Details: details}
}

type sitOpts struct {
	eyes       eyeKind
	wash       bool
	mouth      float64
	tilt       float64
	tip        float64
	gaze       float64 // −1…1: голова и зрачки поворачиваются
	earL, earR bool
}

func sitting(f int, o sitOpts) Sprite {
	hy := 3.0 - o.tilt
	if o.wash && f%2 == 1 {
		hy++
	}
	hdx := jsRound(o.gaze*2) / 2 // голова поворачивается на пол-клетки в каждую сторону
	tipY := 19 + o.tip
	legs := []px{{12, 18, 2, 6, cFur}, {16, 18, 2, 6, cFur}}
	earLY, earRY, earLH, earRH := hy-2, hy-2, 2.0, 2.0
	if o.earL {
		earLY, earLH = hy-1.5, 1.5
	}
	if o.earR {
		earRY, earRH = hy-1.5, 1.5
	}
	layer := []px{
		{1, 22, 7, 2, cFur},
		{0, tipY, 2, 24 - tipY, cFur},
		{6, 14, 12, 10, cFur},
		{11, 10, 8, 10, cFur},
	}
	layer = append(layer, legs...)
	layer = append(layer, px{10 + hdx, hy, 11, 9, cFur}, px{11 + hdx, earLY, 3, earLH, cFur}, px{18 + hdx, earRY, 3, earRH, cFur})
	layers := [][]px{layer}
	pawY := 8.0 + float64(f%2)
	if o.wash {
		layers = append(layers, []px{{16, pawY, 3, 8, cFur}}) // лапка у мордочки
	}
	details := []px{
		{13, 12, 4, 6, cCream}, {13.5, 17.5, 3, 0.5, cCreamSh},
		{14, 19, 0.5, 5, cShade},
	}
	details = append(details, paws(legs, spriteH)...)
	details = append(details,
		px{7, 16, 1, 3, cStripe}, px{9, 15, 1, 3, cStripe}, px{3, 22, 0.5, 2, cStripe}, px{5, 22, 0.5, 2, cStripe})
	details = append(details, faceDetails(face{hx: 10 + hdx, hy: hy, eyes: o.eyes, front: true, mouth: o.mouth, gaze: o.gaze, earL: o.earL, earR: o.earR})...)
	if o.wash {
		details = append(details, px{16, pawY, 3, 1, cCream})
		if f%2 == 0 {
			details = append(details, px{15, hy + 8.5, 1, 0.5, cNose}) // язычок
		}
	}
	return Sprite{Layers: layers, Details: details}
}

func sleeping(f int) Sprite {
	breathe := 0.0
	if f%2 == 1 {
		breathe = 0.5
	}
	layers := [][]px{
		{
			{3, 15 - breathe, 20, 9 + breathe, cFur},
			{16, 14, 9, 8, cFur},
			{17, 12, 2, 2, cFur},
			{22, 12, 2, 2, cFur},
		},
		{{4, 21, 15, 2, cFur}, {17, 20, 2, 2, cFur}}, // хвост обвивает лапы
	}
	details := []px{
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
	legs := []px{{2, 19, 2, 5, cFur}, {7, 19, 2, 5, cFur}}
	layer := []px{
		{0, 2, 2, 9, cFur}, {1, 1, 2, 2, cFur}, // хвост трубой
		{1, 10, 10, 9, cFur},
	}
	layer = append(layer, legs...)
	layer = append(layer,
		px{8, 14, 10, 6, cFur},
		px{14, 21, 11, 3, cFur},
		px{18, 11, 9, 8, cFur},
		px{19, 9, 3, 2, cFur},
		px{24, 9, 3, 2, cFur})
	details := []px{
		{3, 11, 1, 3, cStripe}, {6, 10.5, 1, 3, cStripe}, {11, 14.5, 1, 2.5, cStripe},
		{0, 4, 2, 0.5, cStripe}, {0, 7, 2, 0.5, cStripe},
		{10, 18.5, 8, 1.5, cCream},
	}
	details = append(details, paws(legs, spriteH)...)
	details = append(details,
		px{23.5, 23, 1.5, 1, cCream}, px{24, 21.5, 0.5, 1, cShade}, px{25, 21.5, 0.5, 1, cShade})
	mouth := 0.0
	if f%6 < 3 {
		mouth = 0.6
	}
	details = append(details, faceDetails(face{hx: 17, hy: 11, eyes: eyesClosed, mouth: mouth})...)
	return Sprite{Layers: [][]px{layer}, Details: details}
}

// hanging — кота несут за шкирку: висит анфас, лапки и хвост качаются против движения.
func hanging(sway float64, f int) Sprite {
	s := jsRound(-sway*2) / 2
	mid := jsRound(s) / 2
	layer := []px{
		{13 + s, 24, 2, 6, cFur}, {13 + s + swayTable[f%4]*0.5, 29, 2, 2, cFur}, // хвост
		{10 + mid, 11, 8, 12, cFur},
		{9 + s, 18, 10, 6, cFur},
		{10 + s, 23, 2, 4, cFur}, {16 + s, 23, 2, 4, cFur}, // задние лапы
		{9 + mid, 12, 2, 6, cFur}, {17 + mid, 12, 2, 6, cFur}, // передние
		{8, 3, 11, 9, cFur},
		{8, 1, 3, 2, cFur}, {16, 1, 3, 2, cFur},
	}
	details := []px{
		{12 + mid, 13, 4, 7, cCream}, {12.5 + mid, 19.5, 3, 0.5, cCreamSh},
		{10 + s, 26.5, 2, 0.5, cCream}, {16 + s, 26.5, 2, 0.5, cCream},
		{9 + mid, 17.5, 2, 0.5, cCream}, {17 + mid, 17.5, 2, 0.5, cCream},
		{13 + s, 26, 2, 0.5, cStripe}, {13 + s, 28, 2, 0.5, cStripe},
		{10 + s, 19.5, 1, 2, cStripe}, {17 + s, 19.5, 1, 2, cStripe},
	}
	details = append(details, faceDetails(face{hx: 8, hy: 3, eyes: eyesSleepy, front: true})...)
	return Sprite{Layers: [][]px{layer}, Details: details}
}

// Sprite возвращает позу кота в его нынешнем состоянии. Тики модели идут 20 раз в
// секунду, а кадры поз подобраны под 10, поэтому t — половина тика.
func (c *Cat) Sprite() Sprite {
	t := c.Tick / 2
	open := eyesOpen
	if c.blink > 0 {
		open = eyesClosed
	}
	switch c.Mode {
	case Walk:
		f := int(c.Phase * 8)
		sw := swayTable[(t/3)%4]
		switch c.gait {
		case gTrot:
			return standing(f, standOpts{walk: true, sway: swayTable[t%4], eyes: open})
		case gHop:
			return standing(0, standOpts{leap: true, sway: 0, eyes: eyesWide})
		case gSniff:
			return standing(0, standOpts{sway: sw, eyes: open, drop: 4 + 0.5*math.Round(math.Sin(float64(c.Tick)*0.5))})
		case gLookUp:
			return standing(0, standOpts{sway: swayTable[t%4], eyes: eyesWide, drop: -2})
		}
		return standing(f, standOpts{walk: true, sway: sw, eyes: open})
	case Sleep:
		return sleeping(t / 8)
	case Wash:
		return sitting(t/3, sitOpts{eyes: eyesClosed, wash: true, tip: swayTable[(t/4)%4]})
	case Yawn:
		k := math.Sin(math.Pi * math.Min(1, float64(c.ModeTick)/36))
		tilt := 0.0
		if k > 0.5 {
			tilt = 1
		}
		return sitting(0, sitOpts{eyes: eyesClosed, mouth: k, tilt: tilt, tip: swayTable[(t/4)%4]})
	case Stretch:
		return stretching(c.ModeTick / 2)
	case Carried:
		return hanging(c.Sway, t/2)
	case Fall:
		return standing(0, standOpts{fall: true, sway: 0, eyes: eyesWide})
	case Land:
		return standing(0, standOpts{squash: true, sway: 0, eyes: eyesWide})
	case Play:
		switch c.phase {
		case leap:
			return standing(0, standOpts{leap: true, sway: 0, eyes: eyesWide})
		case stalk:
			return standing(0, standOpts{crouch: true, sway: swayTable[t%4], eyes: eyesWide})
		case chase:
			if int(c.Phase*4)%2 == 1 {
				return standing(0, standOpts{gallop: true, sway: 1, eyes: eyesOpen})
			}
			return standing(0, standOpts{leap: true, sway: -1, eyes: eyesOpen})
		}
		return standing(0, standOpts{sway: swayTable[(t/2)%4], eyes: open})
	}
	// сидит: следит глазами, жмурится, виляет хвостом
	o := sitOpts{eyes: open, tip: swayTable[(t/4)%4], gaze: c.Gaze, earL: c.earL > 0 && (c.earL/3)%2 == 0, earR: c.earR > 0 && (c.earR/3)%2 == 0}
	if c.GazeY < -0.5 {
		o.tilt = 1 // смотрит вверх — голова чуть приподнята
	}
	if c.happy > 0 {
		o.eyes = eyesClosed
		o.tip = swayTable[(c.Tick/2)%4]
	}
	switch c.event {
	case evWag:
		o.tip = swayTable[(c.Tick/2)%4]
	case evButterfly:
		if c.evTick > 15 && c.evTick < 330 && c.blink == 0 && c.happy == 0 {
			o.eyes = eyesWide
		}
	}
	return sitting(0, o)
}
