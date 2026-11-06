// Package cat — модель и рисование рыжего кота без привязки к окну.
//
// Кот живёт сам по себе в собственном окне и ни с чем не взаимодействует: ни с окнами
// других программ, ни с файлами, ни с сетью. Всё, что он делает, — рисуется внутри его
// прозрачного холста. Поведение считается тиками (20 в секунду); координаты — пиксели
// холста вдоль пола, пол на y = 0. Рисует кота Render, показывает его пакет catwin.
package cat

import (
	"math"
	"math/rand"
)

// Размер позы в пикселях холста.
const (
	SprW = 64
	SprH = 52
)

// Окно кота — ViewW × ViewH пикселей холста, пол на GroundY.
const (
	ViewW   = 200
	ViewH   = 130
	GroundY = ViewH - 4
)

// TicksPerSecond — частота тиков модели.
const TicksPerSecond = 20

// Anchor — где загривок, когда кота несут: курсор держит кота здесь.
var Anchor = struct{ X, Y float64 }{ViewW / 2, 16}

// Scruff — где у висящего кота загривок (в координатах позы).
var Scruff = struct{ X, Y float64 }{32, 0}

// DropHeight — на какой высоте над полом окажутся лапы отпущенного кота.
var DropHeight = float64(GroundY) - (Anchor.Y - Scruff.Y + 56)

// BallSize — размер клубка.
const BallSize = 10

const edge = 4

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

type event int

const (
	evNone event = iota
	evLook
	evButterfly
	evEar
	evWag
	evStretch
)

type gait int

const (
	gStride gait = iota
	gTrot
	gSniff
	gLookUp
	gHop
)

// Ball — клубок для игры.
type Ball struct{ X, VX float64 }

// Butterfly — бабочка, за которой кот следит глазами. Просто картинка в окне кота.
type Butterfly struct{ X, Y float64 }

// Print — следок лапы на полу; со временем бледнеет.
type Print struct {
	X, Y float64
	Age  int
}

// Cat — состояние кота.
type Cat struct {
	Width    float64 // ширина мира (экрана) в пикселях
	Mode     Mode
	X        float64 // левый край позы
	Lift     float64 // высота над полом (прыжок, падение)
	VY       float64
	Sway     float64 // раскачивание, когда кота несут (−2…2)
	Facing   int     // 1 — вправо, −1 — влево
	Tick     int
	ModeTick int
	Idle     int // сколько тиков кот сидит без команды
	Yawned   bool
	Washed   bool

	Ball      *Ball
	Butterfly *Butterfly
	Prints    []Print

	// покой
	event     event
	evTick    int
	nextEvent int
	evDir     float64
	Gaze      float64 // куда смотрит: −1 влево … 1 вправо
	GazeY     float64 // −1 вверх … 1 вниз
	gazeT     float64
	gazeYT    float64
	blinkIn   int
	blink     int
	happy     int
	earL      int // подёргивание ушей, тиков до конца
	earR      int

	// прогулка
	gait    gait
	gaitTk  int
	gaitDur int
	Phase   float64 // фаза шага 0…1
	hop     int

	// игра
	phase    phase
	phTicks  int
	leapFrom float64
	leapTo   float64
}

// New возвращает сидящего кота посередине мира шириной width пикселей.
func New(width float64) *Cat {
	return &Cat{Width: width, Facing: 1, X: math.Floor((width - SprW) / 2), nextEvent: 50, blinkIn: 40}
}

func (c *Cat) setMode(m Mode) {
	c.Mode = m
	c.ModeTick = 0
	c.event = evNone
	c.Butterfly = nil
	c.gazeT, c.gazeYT = 0, 0
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
	c.nextEvent = 40
}

// Command — команда из меню: Sit, Walk или Play.
func (c *Cat) Command(m Mode, rnd func() float64) {
	if rnd == nil {
		rnd = rand.Float64
	}
	c.rest()
	c.setMode(m)
	switch m {
	case Walk:
		c.gait, c.gaitTk, c.gaitDur = gStride, 0, 50+int(rnd()*40)
	case Play:
		// Клубок падает в паре шагов от кота, с той стороны, где есть место.
		right := c.X+SprW+60 < c.Width
		gap := 28 + rnd()*20
		if right {
			c.Ball = &Ball{X: c.X + SprW + gap}
		} else {
			c.Ball = &Ball{X: c.X - gap - BallSize}
		}
		c.phase = chase
		c.phTicks = 0
	}
}

// Poke — клик по коту: спящий просыпается и потягивается, умывающийся отвлекается,
// сидящий жмурится от удовольствия.
func (c *Cat) Poke() {
	switch c.Mode {
	case Sleep:
		c.setMode(Stretch)
		c.Idle = 0
		c.Yawned, c.Washed = false, false
	case Wash, Yawn:
		c.rest()
		c.happy = 30
	case Sit:
		c.happy = 36
	}
}

// Carry — кота взяли за шкирку.
func (c *Cat) Carry() {
	c.setMode(Carried)
	c.Sway = 0
}

// Drop — кота отпустили на высоте height пикселей над его новым полом.
func (c *Cat) Drop(height float64) {
	c.setMode(Fall)
	c.Lift = math.Max(0, height)
	c.VY = 0
}

func (c *Cat) paw() float64 {
	if c.Facing > 0 {
		return c.X + 54
	}
	return c.X + 10
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
	c.ageProps()
	c.blinking(rnd)
	switch c.Mode {
	case Sit:
		c.stepSit(rnd)
	case Yawn:
		c.Idle++
		if c.ModeTick >= 36 {
			c.setMode(Sit)
			c.nextEvent = 30
		}
	case Wash:
		c.Idle++
		if c.ModeTick >= 100 {
			c.setMode(Sit)
			c.nextEvent = 30
		}
	case Stretch:
		if c.ModeTick >= 56 {
			c.rest()
		}
	case Fall:
		c.VY += 0.35
		c.Lift -= c.VY
		if c.Lift <= 0 {
			c.setMode(Land)
		}
	case Land:
		if c.ModeTick >= 8 {
			c.rest()
		}
	case Walk:
		c.stepWalk(rnd)
	case Play:
		c.stepPlay(rnd)
		if c.ModeTick >= 520 && c.phase != leap {
			c.rest() // наигрался
		}
	}
	c.easeGaze()
}

func (c *Cat) ageProps() {
	for i := 0; i < len(c.Prints); {
		c.Prints[i].Age++
		if c.Prints[i].Age > 60 {
			c.Prints = append(c.Prints[:i], c.Prints[i+1:]...)
			continue
		}
		i++
	}
	if c.happy > 0 {
		c.happy--
	}
	if c.earL > 0 {
		c.earL--
	}
	if c.earR > 0 {
		c.earR--
	}
}

func (c *Cat) blinking(rnd func() float64) {
	if c.blink > 0 {
		c.blink--
		return
	}
	c.blinkIn--
	if c.blinkIn <= 0 {
		c.blink = 4
		c.blinkIn = 50 + int(rnd()*90)
		if rnd() < 0.2 { // иногда моргает дважды
			c.blinkIn = 8
		}
	}
}

func (c *Cat) easeGaze() {
	c.Gaze += (c.gazeT - c.Gaze) * 0.18
	c.GazeY += (c.gazeYT - c.GazeY) * 0.18
}

func (c *Cat) stepSit(rnd func() float64) {
	c.Idle++
	switch {
	case c.Idle >= 900:
		c.setMode(Sleep)
		return
	case c.Idle >= 420 && !c.Washed:
		c.Washed = true
		c.setMode(Wash)
		return
	case c.Idle >= 260 && !c.Yawned:
		c.Yawned = true
		c.setMode(Yawn)
		return
	}
	if c.event == evNone {
		c.nextEvent--
		if c.nextEvent <= 0 {
			c.startEvent(rnd)
		}
		return
	}
	c.evTick++
	switch c.event {
	case evLook:
		// влево, вправо, снова вперёд
		switch {
		case c.evTick < 40:
			c.gazeT, c.gazeYT = c.evDir, 0
		case c.evTick < 90:
			c.gazeT, c.gazeYT = -c.evDir, 0
		default:
			c.gazeT, c.gazeYT = 0, 0
		}
		if c.evTick >= 130 {
			c.endEvent(rnd)
		}
	case evButterfly:
		t := float64(c.evTick)
		cx := c.X + SprW/2
		b := c.Butterfly
		b.X = cx + c.evDir*(50+10*math.Sin(t*0.05)) + 22*math.Sin(t*0.11)
		b.Y = float64(GroundY) - 78 - 12*math.Sin(t*0.07) + 10*math.Sin(t*0.19)
		headY := float64(GroundY) - SprH + 17
		c.gazeT = math.Max(-1, math.Min(1, (b.X-cx)/40))
		c.gazeYT = math.Max(-1, math.Min(1, (b.Y-headY)/22))
		if c.evTick > 320 { // улетает
			b.Y -= float64(c.evTick-320) * 0.6
			b.X += c.evDir * float64(c.evTick-320) * 0.5
		}
		if c.evTick >= 380 {
			c.endEvent(rnd)
		}
	case evEar:
		if c.evTick >= 36 {
			c.endEvent(rnd)
		}
	case evWag:
		if c.evTick >= 100 {
			c.endEvent(rnd)
		}
	case evStretch:
		c.Mode = Stretch
		c.ModeTick = 0
		c.event = evNone
	}
}

func (c *Cat) startEvent(rnd func() float64) {
	c.evTick = 0
	c.evDir = 1
	if rnd() < 0.5 {
		c.evDir = -1
	}
	r := rnd()
	switch {
	case r < 0.30:
		c.event = evLook
	case r < 0.52:
		c.event = evButterfly
		c.Butterfly = &Butterfly{X: c.X + SprW/2 + c.evDir*60, Y: float64(GroundY) - 80}
	case r < 0.70:
		c.event = evEar
		if rnd() < 0.5 {
			c.earL = 12
		} else {
			c.earR = 12
		}
	case r < 0.88:
		c.event = evWag
	default:
		c.event = evStretch
	}
}

func (c *Cat) endEvent(rnd func() float64) {
	c.event = evNone
	c.Butterfly = nil
	c.gazeT, c.gazeYT = 0, 0
	c.nextEvent = 70 + int(rnd()*130)
}

func (c *Cat) stepWalk(rnd func() float64) {
	c.gaitTk++
	speed, dphase := 0.0, 0.0
	switch c.gait {
	case gStride:
		speed, dphase = 1.0, 1.0/27
	case gTrot:
		speed, dphase = 2.0, 1.0/18
	case gHop:
		c.hop++
		speed = 1.6
		t := float64(c.hop) / 22
		c.Lift = math.Round(math.Sin(math.Pi*math.Min(1, t)) * 13)
		if t >= 1 {
			c.Lift = 0
		}
	case gSniff:
		c.gazeYT = 1
	case gLookUp:
		c.gazeYT = -1
	}
	if c.gait != gSniff && c.gait != gLookUp {
		c.gazeYT = 0
	}
	if speed > 0 {
		c.X += float64(c.Facing) * speed
		if c.X >= c.maxX() {
			c.X = c.maxX()
			c.Facing = -1
		}
		if c.X <= edge {
			c.X = edge
			c.Facing = 1
		}
	}
	if dphase > 0 {
		prev := c.Phase
		c.Phase = math.Mod(c.Phase+dphase, 1)
		// лапа коснулась пола — остаётся следок
		for _, mark := range []float64{0.0, 0.5} {
			if (prev < mark && c.Phase >= mark) || (mark == 0 && c.Phase < prev) {
				c.addPrint(mark == 0)
			}
		}
	}
	if c.gaitTk >= c.gaitDur {
		c.nextGait(rnd)
	}
	if c.ModeTick >= 800 {
		c.rest() // нагулялся
	}
}

func (c *Cat) addPrint(near bool) {
	x := c.X + 12
	if c.Facing > 0 {
		x = c.X + SprW - 14
	}
	y := float64(GroundY) + 1
	if !near {
		y -= 1
	}
	c.Prints = append(c.Prints, Print{X: x, Y: y})
	if len(c.Prints) > 14 {
		c.Prints = c.Prints[1:]
	}
}

func (c *Cat) nextGait(rnd func() float64) {
	c.gaitTk, c.hop = 0, 0
	c.Lift = 0
	r := rnd()
	switch {
	case r < 0.40:
		c.gait, c.gaitDur = gStride, 70+int(rnd()*90)
	case r < 0.58:
		c.gait, c.gaitDur = gTrot, 40+int(rnd()*40)
	case r < 0.74:
		c.gait, c.gaitDur = gSniff, 60+int(rnd()*40)
	case r < 0.88:
		c.gait, c.gaitDur = gLookUp, 50+int(rnd()*30)
	default:
		c.gait, c.gaitDur = gHop, 22
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
		c.Phase = math.Mod(c.Phase+1.0/14, 1)
		if math.Abs(c.paw()-centre) <= 40 {
			c.phase, c.phTicks = stalk, 0
			break
		}
		c.X = c.clampX(c.X + float64(c.Facing)*2)
		if c.X == edge || c.X == c.maxX() {
			c.phase, c.phTicks = stalk, 0
		}
	case stalk:
		c.faceBall(centre)
		if c.phTicks >= 26 {
			c.phase, c.phTicks = leap, 0
			c.leapFrom = c.X
			if c.Facing > 0 {
				c.leapTo = c.clampX(ball.X - 46)
			} else {
				c.leapTo = c.clampX(ball.X - 2)
			}
		}
	case leap:
		t := math.Min(1, float64(c.phTicks)/13)
		c.X = c.leapFrom + (c.leapTo-c.leapFrom)*t
		c.Lift = math.Round(math.Sin(math.Pi*t) * 22)
		if t >= 1 {
			c.Lift = 0
			ball.VX = float64(c.Facing) * (1.6 + rnd()*1.6) // недалеко: клубок должен оставаться у кота в окне
			c.phase, c.phTicks = watch, 0
		}
	case watch:
		ball.X += ball.VX
		ball.VX *= 0.938
		if ball.X < 2 {
			ball.X = 2
			ball.VX = -ball.VX
		}
		if ball.X > c.Width-BallSize-2 {
			ball.X = c.Width - BallSize - 2
			ball.VX = -ball.VX
		}
		c.faceBall(centre)
		if math.Abs(ball.VX) < 0.2 {
			ball.VX = 0
			c.phase, c.phTicks = chase, 0
		}
	}
}

// HitCat — попадает ли точка в кота на полу (y от пола, вверх — минус).
func (c *Cat) HitCat(x, y float64) bool {
	return x >= c.X-4 && x <= c.X+SprW+4 && y >= -SprH-c.Lift-6 && y <= 4
}

// jsRound — округление «половинки вверх».
func jsRound(x float64) float64 { return math.Floor(x + 0.5) }
