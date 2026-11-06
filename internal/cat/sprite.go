package cat

import (
	"image/color"
	"math"
)

// Кот нарисован вручную по пикселям, как спрайт из игр 16-bit: части (голова,
// туловище, хвост, ошейник) заданы строками, где каждая буква — цвет, а «.» — пусто.
// Контур в один пиксель движок строит сам: сверху и слева он светлее, снизу и справа
// темнее (выборочный контур). Лапы — пиксельные линии от сустава до пола, поэтому
// походку можно анимировать без отдельного кадра на каждую фазу.
//
// Один пиксель спрайта — sp пикселей холста.
const sp = 2

func nrgb(r, g, b uint8) color.NRGBA { return color.NRGBA{r, g, b, 255} }

// Палитра со сдвигом оттенка: свет теплее и желтее, тень краснее.
var (
	cOut      = nrgb(0x4a, 0x22, 0x15) // контур снизу и справа
	cOutLight = nrgb(0x8c, 0x43, 0x20) // контур сверху и слева
	cWhite    = nrgb(0xff, 0xff, 0xff)
	cZzz      = nrgb(0x9c, 0xc8, 0xff)
)

var palette = map[byte]color.NRGBA{
	'L': nrgb(0xff, 0xbe, 0x62), // светлая шерсть
	'M': nrgb(0xf2, 0x8a, 0x30), // основная
	'D': nrgb(0xc0, 0x55, 0x1e), // тень
	'S': nrgb(0xd2, 0x66, 0x20), // полоски
	'F': nrgb(0xc8, 0x6a, 0x2a), // дальние лапы
	'W': nrgb(0xff, 0xf8, 0xec), // белая шерсть
	'C': nrgb(0xec, 0xcb, 0xa2), // белая в тени
	'P': nrgb(0xff, 0x9f, 0xb0), // нос, уши изнутри
	'E': nrgb(0x2b, 0x1a, 0x14), // глаза, рот
	'G': nrgb(0x9b, 0xd6, 0x5a), // радужка
	'B': nrgb(0x3f, 0x78, 0xe0), // ошейник
	'Y': nrgb(0xff, 0xd8, 0x4a), // колокольчик
	'R': nrgb(0xc2, 0x3a, 0x3a), // открытый рот
	'H': nrgb(0xff, 0xff, 0xff), // блик
	'K': nrgb(0xb0, 0x62, 0x55), // ротик — мягче чёрного
}

// ---------- части ----------

// Голова анфас, 15×12. Глаза рисуются отдельно (см. frontEyes).
var frontHead = []string{
	".L...........L.",
	".LL.........LM.",
	".LPL.......LPM.",
	"LLLLLLSLSLLLMMD",
	"LLLLLLLSLLLLMMD",
	"LLLLLLLLLLLLMMD",
	"LLMMMMMMMMMMMMD",
	"MMMMMMMMMMMMMMD",
	"MMMMMWWWWWMMMMD",
	"MMMMWWWWWWWMMDD",
	".DMMMWWWWWMMDD.",
	"..DDDDDDDDDDD..",
}

// Туловище сидящего кота анфас, 15×11.
var frontBody = []string{
	"....MMMMMMM....",
	"...MMWWWWWMM...",
	"..MMWWWWWWWMM..",
	"..MSWWWWWWWSD..",
	".MMSWWWWWWWSMD.",
	".MSMMWWWWWMMSD.",
	"MMSMMMWWWMMMSDD",
	"MSMMMMMMMMMMMSD",
	"MMSMMMMMMMMMSDD",
	"DMMMMMMMMMMMMDD",
	".DDDDDDDDDDDDD.",
}

// Передняя лапа анфас, 2×6: белая «варежка» внизу.
var frontLeg = []string{
	"MM",
	"MM",
	"MM",
	"MM",
	"WW",
	"WW",
}

var collar = []string{
	"BBBBYBBBB",
	"....Y....",
}

// Хвост сидящего кота, 6×10: основание внизу слева, белый кончик сверху.
var frontTail = []string{
	"...WW.",
	"...WW.",
	"...SM.",
	"...MM.",
	"...MS.",
	"..MMD.",
	"..SM..",
	".MMD..",
	"MSM...",
	"MM....",
}

// Туловище сбоку (кот смотрит вправо), 18×7.
var sideTorso = []string{
	"..LLLSLLLLSLLLL...",
	".LLLSLLLLSLLLLLLL.",
	"MMMMSMMMMSMMMMMMMW",
	"MMMSMMMMSMMMMMMMWW",
	"DMMMMMMMMMMMMMMMWW",
	"DDMMMMMMMMMMMMMWWW",
	".DDDCCCCCCCCCCWWW.",
}

// Голова сбоку, 12×10. Глаз рисуется отдельно (см. sideEye).
var sideHead = []string{
	"..L.....L...",
	".LPL...LPL..",
	".LLLLLLLLLL.",
	"LLLLSLSLLLLL",
	"LLLLLLLLLLLL",
	"MMMMMMMMMMMM",
	"MMMMMMMMMWWW",
	"DMMMMMMMMWWW",
	".DDMMMMMMWW.",
	"...DDDDDDD..",
}

// Хвост сбоку, 6×10: основание внизу справа, кончик сверху слева.
var sideTail = []string{
	"WW....",
	"WWW...",
	".SM...",
	".MM...",
	"..MS..",
	"..MM..",
	"..SM..",
	"...MM.",
	"...MS.",
	"....MM",
}

// Хвост, опущенный назад (крадётся, прыгает), 9×4: основание справа.
var sideTailLow = []string{
	"WW.......",
	"WWMMSMMS.",
	"..MMMSMMM",
	"......MMM",
}

// Хвост спящего кота вдоль пола перед лапами, 16×3.
var sleepTail = []string{
	"..MMSMMSMMSMMWW.",
	"MMMMMMMMMMMMMWWW",
	".DDDDDDDDDDDDCC.",
}

// Висящее туловище (кота несут за шкирку), 11×11.
var hangBody = []string{
	"..MMMMMMM..",
	".MMWWWWWMM.",
	"MMSWWWWWSMD",
	"MSMWWWWWMSD",
	"MMSWWWWWSMD",
	"MSMMWWWMMSD",
	"MMSMMMMMSMD",
	"MSMMMMMMMSD",
	".MMMMMMMMD.",
	".DMMMMMMDD.",
	"..DDDDDDD..",
}

// ---------- сборка кадра ----------

// Буфер кадра в пикселях спрайта. Поза занимает рамку boxW×boxH (= SprW×SprH
// пикселей холста), вокруг неё запас для хвоста, ушей и усов.
const (
	boxW = SprW / sp
	boxH = SprH / sp
	padX = 10
	padY = 12
	bufW = boxW + 2*padX
	bufH = boxH + 2*padY
)

type cell struct {
	col color.NRGBA
	on  bool
}

type frame struct {
	px [bufW * bufH]cell
}

func (f *frame) at(x, y int) *cell {
	x, y = x+padX, y+padY
	if x < 0 || y < 0 || x >= bufW || y >= bufH {
		return nil
	}
	return &f.px[y*bufW+x]
}

func (f *frame) set(x, y int, col color.NRGBA) {
	if c := f.at(x, y); c != nil {
		c.col, c.on = col, true
	}
}

// part рисует часть с контуром. edgeOnly — контур только по краю пустоты: так лапа
// сливается с туловищем без шва.
func (f *frame) part(rows []string, x0, y0 int, edgeOnly bool) {
	in := func(x, y int) bool {
		y -= y0
		x -= x0
		return y >= 0 && y < len(rows) && x >= 0 && x < len(rows[y]) && rows[y][x] != '.'
	}
	h := len(rows)
	w := 0
	for _, r := range rows {
		w = max(w, len(r))
	}
	for y := y0 - 1; y <= y0+h; y++ {
		for x := x0 - 1; x <= x0+w; x++ {
			if in(x, y) {
				continue
			}
			down, right := in(x, y+1), in(x+1, y)
			up, left := in(x, y-1), in(x-1, y)
			if !(down || right || up || left) {
				continue
			}
			if c := f.at(x, y); c != nil && edgeOnly && c.on {
				continue
			}
			// Контур над фигурой и слева от неё освещён — он светлее.
			if (down || right) && !up && !left {
				f.set(x, y, cOutLight)
			} else {
				f.set(x, y, cOut)
			}
		}
	}
	for y, r := range rows {
		for x := 0; x < len(r); x++ {
			if col, ok := palette[r[x]]; ok {
				f.set(x0+x, y0+y, col)
			}
		}
	}
}

// shift сдвигает верхние строки части на dx: так хвост качается.
func shift(rows []string, top, dx int) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		k := 0
		if i < top {
			k = dx
		}
		switch {
		case k > 0:
			out[i] = repeat('.', k) + r[:len(r)-k]
		case k < 0:
			out[i] = r[-k:] + repeat('.', -k)
		default:
			out[i] = r
		}
	}
	return out
}

func repeat(b byte, n int) string {
	s := make([]byte, n)
	for i := range s {
		s[i] = b
	}
	return string(s)
}

// leg — пиксельная лапа шириной 2 от сустава (jx, jy) до стопы (fx, fy): шерсть,
// внизу белый «носочек» и лапка, вытянутая вперёд.
func (f *frame) leg(jx, jy, fx, fy int, far bool) {
	fur, sock := byte('M'), byte('W')
	if far {
		fur, sock = 'F', 'C'
	}
	n := max(1, fy-jy)
	rows := map[[2]int]byte{}
	for i := 0; i <= n; i++ {
		y := jy + i
		x := jx + int(math.Round(float64(fx-jx)*float64(i)/float64(n)))
		c := fur
		if fy-y <= 1 {
			c = sock
		}
		rows[[2]int{x, y}] = c
		rows[[2]int{x + 1, y}] = c
	}
	rows[[2]int{fx + 2, fy}] = sock // пальчики вперёд
	// в строки, чтобы получить общий контур
	minX, minY, maxX, maxY := 1<<30, 1<<30, -1<<30, -1<<30
	for k := range rows {
		minX, minY = min(minX, k[0]), min(minY, k[1])
		maxX, maxY = max(maxX, k[0]), max(maxY, k[1])
	}
	grid := make([]string, maxY-minY+1)
	for y := minY; y <= maxY; y++ {
		b := []byte(repeat('.', maxX-minX+1))
		for x := minX; x <= maxX; x++ {
			if c, ok := rows[[2]int{x, y}]; ok {
				b[x-minX] = c
			}
		}
		grid[y-minY] = string(b)
	}
	f.part(grid, minX, minY, true)
}

// ---------- лица ----------

// frontEyes рисует глаза анфас: x — левый край левого глаза, второй глаз на 7 правее.
func (f *frame) frontEyes(x, y int, kind eyeKind, gaze float64) {
	e, g, h := palette['E'], palette['G'], palette['H']
	for _, ex := range []int{x, x + 7} {
		switch kind {
		case eyesClosed:
			f.set(ex, y+1, e)
			f.set(ex+1, y+1, e)
		case eyesHappy: // ^ ^
			if ex == x {
				f.set(ex, y+1, e)
				f.set(ex+1, y, e)
			} else {
				f.set(ex, y, e)
				f.set(ex+1, y+1, e)
			}
		case eyesSleepy:
			f.set(ex, y+1, e)
			f.set(ex+1, y+1, e)
			f.set(ex, y, palette['D'])
			f.set(ex+1, y, palette['D'])
		default:
			// зелёная радужка и зрачок, который смотрит туда же, куда кот
			f.set(ex, y, g)
			f.set(ex+1, y, g)
			f.set(ex, y+1, g)
			f.set(ex+1, y+1, g)
			px := ex + 1
			if gaze < -0.3 {
				px = ex
			}
			f.set(px, y, e)
			f.set(px, y+1, e)
			if kind == eyesWide {
				f.set(ex, y-1, g)
				f.set(ex+1, y-1, g)
				f.set(px, y-1, e)
			}
			hx := ex
			if px == ex {
				hx = ex + 1
			}
			f.set(hx, y, h)
		}
	}
}

// frontMouth — нос и рот; open — зевок.
func (f *frame) frontMouth(cx, y int, open bool) {
	f.set(cx, y, palette['P'])
	if open {
		for dy := 1; dy <= 2; dy++ {
			for dx := -1; dx <= 1; dx++ {
				f.set(cx+dx, y+dy, palette['R'])
			}
		}
		f.set(cx, y+2, palette['P']) // язычок
		return
	}
	f.set(cx, y+1, palette['K'])
	f.set(cx-1, y+2, palette['K'])
	f.set(cx+1, y+2, palette['K'])
}

// sideEye — глаз в профиль у головы с левым краем x0.
func (f *frame) sideEye(x0, y0 int, kind eyeKind, up bool) {
	e, g := palette['E'], palette['G']
	x, y := x0+8, y0+4
	switch kind {
	case eyesClosed, eyesSleepy, eyesHappy:
		f.set(x-1, y+1, e)
		f.set(x, y+1, e)
		return
	}
	f.set(x-1, y, g)
	f.set(x-1, y+1, g)
	f.set(x, y, e)
	f.set(x, y+1, e)
	if kind == eyesWide {
		f.set(x-1, y-1, g)
		f.set(x, y-1, e)
	}
	if up {
		f.set(x, y+1, g)
	}
	f.set(x-1, y, palette['H'])
}

func (f *frame) sideFace(x0, y0 int, kind eyeKind, up, open bool) {
	f.sideEye(x0, y0, kind, up)
	f.set(x0+11, y0+5, palette['P']) // нос
	if open {
		f.set(x0+9, y0+7, palette['R'])
		f.set(x0+10, y0+7, palette['R'])
		f.set(x0+10, y0+8, palette['P'])
	} else {
		f.set(x0+10, y0+7, palette['K'])
	}
	w := palette['C']
	f.set(x0+12, y0+6, w)
	f.set(x0+13, y0+6, w)
	f.set(x0+12, y0+7, w)
	f.set(x0+13, y0+8, w)
}

// ---------- позы ----------

func (c *Cat) eyes() eyeKind {
	if c.blink > 0 {
		return eyesClosed
	}
	if c.happy > 0 {
		return eyesHappy
	}
	return eyesOpen
}

type eyeKind int

const (
	eyesOpen eyeKind = iota
	eyesClosed
	eyesHappy
	eyesWide
	eyesSleepy
)

// tailSway — куда качнулся кончик хвоста: −1, 0 или 1.
func tailSway(t int, fast bool) int {
	k := t / 12
	if fast {
		k = t / 4
	}
	return []int{0, 1, 0, -1}[k%4]
}

// drawFront — сидит анфас (а также умывается и зевает).
func (f *frame) drawFront(c *Cat) {
	t := c.Tick
	sway := tailSway(t, c.event == evWag || c.happy > 0)
	f.part(shift(frontTail, 5, sway), 22, 15, false)
	f.part(frontBody, 9, 15, false)
	f.legsFront(13, 20)
	pawUp := c.Mode == Wash
	if !pawUp {
		f.legsFront(17, 20)
	}

	hx := 9
	switch {
	case c.Gaze > 0.4:
		hx++
	case c.Gaze < -0.4:
		hx--
	}
	hy := 3
	if c.GazeY < -0.5 || c.Mode == Yawn {
		hy-- // смотрит вверх или тянется в зевке
	}
	if c.Mode == Wash {
		hy++
	}
	head := frontHead
	if c.earL > 0 && (c.earL/3)%2 == 0 {
		head = twitchEar(head, false)
	}
	if c.earR > 0 && (c.earR/3)%2 == 0 {
		head = twitchEar(head, true)
	}
	f.part(head, hx, hy, false)
	f.part(collar, hx+3, hy+12, true) // ошейник под подбородком

	kind := c.eyes()
	open := false
	switch c.Mode {
	case Wash:
		kind = eyesClosed
	case Yawn:
		kind = eyesClosed
		open = c.ModeTick > 6 && c.ModeTick < 30
	case Sit:
		if c.event == evButterfly && c.evTick > 15 && c.evTick < 330 && kind == eyesOpen {
			kind = eyesWide
		}
	}
	f.frontEyes(hx+3, hy+6, kind, c.Gaze)
	f.frontMouth(hx+7, hy+8, open)

	if pawUp { // лапка у мордочки, двигается вверх-вниз
		dy := (t / 5) % 2
		f.leg(18, 20, hx+9, hy+10+dy, false)
		if (t/5)%2 == 0 {
			f.set(hx+8, hy+10, palette['P']) // язычок
		}
	}
}

// twitchEar прижимает кончик уха.
func twitchEar(rows []string, right bool) []string {
	out := append([]string(nil), rows...)
	b0, b1 := []byte(out[0]), []byte(out[1])
	if right {
		b0[13] = '.'
		b1[13] = 'L'
		b1[14] = 'L'
	} else {
		b0[1] = '.'
		b1[0] = 'L'
	}
	out[0], out[1] = string(b0), string(b1)
	return out
}

// sidePose — что делают части кота сбоку.
type sidePose struct {
	bodyDY   int       // туловище ниже обычного (крадётся)
	headDX   int       // голова вперёд
	headDY   int       // голова ниже (нюхает) или выше (минус)
	feet     [4][2]int // ближняя задняя, дальняя задняя, ближняя передняя, дальняя передняя
	lowTail  bool
	sway     int
	eyes     eyeKind
	up, open bool
	noLegs   bool
}

func (c *Cat) sidePose() sidePose {
	t := c.Tick
	p := sidePose{eyes: c.eyes(), sway: tailSway(t, false)}
	const ground = boxH - 1
	stand := [4][2]int{{8, ground}, {11, ground}, {20, ground}, {17, ground}}
	p.feet = stand
	walk := func(s, lift float64) {
		at := func(joint float64, ph float64) [2]int {
			ft := footAt(joint, s, lift, ph)
			return [2]int{int(math.Round(ft.X)), int(math.Round(ft.Y))}
		}
		p.feet = [4][2]int{at(8, c.Phase+0.5), at(11, c.Phase), at(20, c.Phase), at(17, c.Phase+0.5)}
	}
	switch c.Mode {
	case Walk:
		switch c.gait {
		case gStride:
			walk(2.5, 1.5)
		case gTrot:
			walk(3.5, 2)
			p.sway = tailSway(t, true)
		case gHop:
			p.feet = [4][2]int{{5, ground - 2}, {8, ground - 3}, {22, ground - 2}, {20, ground - 3}}
			p.lowTail = true
		case gSniff:
			p.headDY, p.headDX = 4+(t/6)%2, 1
		case gLookUp:
			p.headDY, p.up = -1, true
			p.eyes = eyesWide
			if c.blink > 0 {
				p.eyes = eyesClosed
			}
		}
	case Play:
		switch c.phase {
		case chase:
			walk(4, 2.5)
			p.eyes = eyesWide
			p.sway = tailSway(t, true)
		case stalk:
			p.bodyDY, p.headDY, p.lowTail = 3, 3, true
			p.eyes = eyesWide
			p.sway = tailSway(t, true)
		case leap:
			p.feet = [4][2]int{{4, ground - 2}, {6, ground - 3}, {24, ground - 3}, {22, ground - 4}}
			p.lowTail, p.eyes = true, eyesWide
		default:
			p.sway = tailSway(t, true)
		}
	case Stretch:
		k := math.Sin(math.Pi * math.Min(1, float64(c.ModeTick)/56))
		ext := int(math.Round(5 * k))
		p.feet[2] = [2]int{20 + ext, ground}
		p.feet[3] = [2]int{17 + ext, ground}
		p.bodyDY = int(math.Round(2 * k))
		p.headDY = int(math.Round(3 * k))
		p.headDX = int(math.Round(2 * k))
		p.eyes = eyesClosed
		p.open = k > 0.6
	case Fall:
		p.feet = [4][2]int{{5, ground - 5}, {8, ground - 6}, {22, ground - 5}, {19, ground - 6}}
		p.eyes = eyesWide
	case Land:
		p.bodyDY, p.headDY = 2, 2
		p.feet = [4][2]int{{6, ground}, {9, ground}, {22, ground}, {19, ground}}
		p.eyes = eyesWide
	}
	return p
}

// drawSide — кот в профиль, смотрит вправо.
func (f *frame) drawSide(p sidePose) {
	tx, ty := 7, 14+p.bodyDY
	// хвост и дальние лапы за туловищем
	if p.lowTail {
		f.part(shift(sideTailLow, 2, p.sway), tx-8, ty, false)
	} else {
		f.part(shift(sideTail, 4, p.sway), tx-4, ty-9, false)
	}
	jy := ty + 6
	if !p.noLegs {
		f.leg(tx+4, jy, p.feet[1][0], p.feet[1][1], true)
		f.leg(tx+10, jy, p.feet[3][0], p.feet[3][1], true)
	}
	f.part(sideTorso, tx-1, ty, false)
	if !p.noLegs {
		f.leg(tx+1, jy, p.feet[0][0], p.feet[0][1], false)
		f.leg(tx+13, jy, p.feet[2][0], p.feet[2][1], false)
	}
	hx, hy := 20+p.headDX, 7+p.bodyDY+p.headDY
	f.part(sideCollar, hx-1, hy+8, false)
	f.part(sideHead, hx, hy, false)
	f.sideFace(hx, hy, p.eyes, p.up, p.open)
}

var sideCollar = []string{
	"BBB",
	"BYB",
}

// drawSleep — свернулся клубком: туловище лежит на полу, голова на лапах, хвост
// обвивает лапы.
func (f *frame) drawSleep(c *Cat) {
	breathe := (c.Tick / 24) % 2
	tx, ty := 7, 18+breathe
	f.part(sideTorso, tx-1, ty, false)
	f.part(sleepTail, 9, boxH-3, false)
	hx, hy := 19, 14+breathe
	f.part(sideHead, hx, hy, false)
	f.sideEye(hx, hy, eyesClosed, false)
	f.set(hx+11, hy+5, palette['P'])
}

// drawHanging — кота несут за шкирку: висит анфас, лапки и хвост болтаются.
func (f *frame) drawHanging(c *Cat) {
	s := int(math.Round(-c.Sway))
	sw := []int{0, 1, 0, -1}[(c.Tick/6)%4]
	hx, hy := 9, 0
	f.part(shift(frontTail, 4, sw), 14+s, 18, false)
	f.leg(12+s, 20, 11+s+sw, 25, false)
	f.leg(18+s, 20, 19+s-sw, 25, false)
	f.part(hangBody, 11+s, 11, false)
	f.part(collar, 12, 11, false)
	f.part(frontHead, hx, hy, false)
	f.frontEyes(hx+3, hy+6, eyesSleepy, 0)
	f.frontMouth(hx+7, hy+8, false)
}

// spriteFrame собирает кадр для текущего состояния кота.
func spriteFrame(c *Cat) (f *frame, mirror bool) {
	f = &frame{}
	switch c.Mode {
	case Sit, Wash, Yawn:
		f.drawFront(c)
	case Sleep:
		f.drawSleep(c)
		mirror = c.Facing < 0
	case Carried:
		f.drawHanging(c)
	default:
		f.drawSide(c.sidePose())
		mirror = c.Facing < 0
	}
	return f, mirror
}

// blit переносит кадр на холст: (ox, oy) — левый верхний угол рамки позы.
func (f *frame) blit(c *Canvas, ox, oy float64, mirror bool) {
	for y := 0; y < bufH; y++ {
		for x := 0; x < bufW; x++ {
			cl := f.px[y*bufW+x]
			if !cl.on {
				continue
			}
			bx := x - padX
			if mirror {
				bx = boxW - 1 - bx
			}
			cx := int(ox) + bx*sp
			cy := int(oy) + (y-padY)*sp
			c.fill(cx, cy, sp, sp, cl.col, 1)
		}
	}
}

// legsFront — передняя лапа анфас: контур только по бокам и снизу, чтобы лапа
// выходила из белой грудки без шва.
func (f *frame) legsFront(x, y int) {
	for i, r := range frontLeg {
		for j := 0; j < len(r); j++ {
			f.set(x+j, y+i, palette[r[j]])
		}
	}
	h := len(frontLeg)
	for i := 1; i < h; i++ {
		f.set(x-1, y+i, cOut)
		f.set(x+2, y+i, cOut)
	}
	for j := -1; j <= 2; j++ {
		f.set(x+j, y+h, cOut)
	}
}
