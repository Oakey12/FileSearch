package cat

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math/rand"
	"os"
	"strings"
	"testing"
)

func steps(c *Cat, n int) {
	for i := 0; i < n; i++ {
		c.Step(func() float64 { return 0.5 })
	}
}

func TestIdleSequence(t *testing.T) {
	c := New(400)
	steps(c, 260)
	if c.Mode != Yawn {
		t.Fatalf("после 260 тиков ждали зевок, а режим %v", c.Mode)
	}
	steps(c, 36+160)
	if c.Mode != Wash {
		t.Fatalf("ждали умывание, а режим %v", c.Mode)
	}
	steps(c, 100+500)
	if c.Mode != Sleep {
		t.Fatalf("ждали сон, а режим %v", c.Mode)
	}
	c.Poke()
	if c.Mode != Stretch {
		t.Fatalf("клик по спящему должен будить, а режим %v", c.Mode)
	}
}

func TestIdleEventsHappen(t *testing.T) {
	c := New(400)
	seen := map[event]bool{}
	rnds := []float64{0.1, 0.4, 0.6, 0.8, 0.95}
	for _, r := range rnds {
		c = New(400)
		for i := 0; i < 200 && c.Mode == Sit; i++ {
			c.Step(func() float64 { return r })
			seen[c.event] = true
		}
	}
	for _, e := range []event{evLook, evButterfly, evEar, evWag} {
		if !seen[e] {
			t.Errorf("событие %v ни разу не случилось", e)
		}
	}
}

func TestButterflyMakesCatLook(t *testing.T) {
	c := New(400)
	c.event, c.evTick, c.evDir = evButterfly, 0, 1
	c.Butterfly = &Butterfly{}
	steps(c, 120)
	if c.Gaze == 0 && c.GazeY == 0 {
		t.Fatal("кот не следит за бабочкой")
	}
}

func TestWalkBouncesAndVariesGait(t *testing.T) {
	c := New(300)
	c.Command(Walk, nil)
	gaits := map[gait]bool{}
	sawPrints := false
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 790; i++ {
		c.Step(rng.Float64)
		if c.X < edge || c.X > c.maxX() {
			t.Fatalf("кот вышел за пол: x=%v", c.X)
		}
		gaits[c.gait] = true
		sawPrints = sawPrints || len(c.Prints) > 0
	}
	if len(gaits) < 3 {
		t.Fatalf("прогулка однообразна: походок %d", len(gaits))
	}
	if !sawPrints {
		t.Fatal("после ходьбы нет следов лап")
	}
}

func TestPlayPutsBallOnFreeSide(t *testing.T) {
	c := New(500)
	c.Command(Play, func() float64 { return 0 })
	if c.Ball == nil || c.Ball.X <= c.X {
		t.Fatalf("клубок должен лечь справа: %+v", c.Ball)
	}
	steps(c, 900)
	if c.Mode == Play && c.phase != leap {
		t.Fatalf("игра должна закончиться, режим %v", c.Mode)
	}
}

func TestCarryDropLand(t *testing.T) {
	c := New(400)
	c.Carry()
	c.Drop(DropHeight)
	for i := 0; i < 80 && c.Mode == Fall; i++ {
		c.Step(nil)
	}
	if c.Mode != Land {
		t.Fatalf("кот должен приземлиться, режим %v", c.Mode)
	}
	steps(c, 10)
	if c.Mode != Sit {
		t.Fatalf("после приземления кот садится, режим %v", c.Mode)
	}
}

func TestHitCat(t *testing.T) {
	c := New(400)
	if !c.HitCat(c.X+SprW/2, -SprH/2) || c.HitCat(c.X-30, -5) || c.HitCat(c.X+5, -SprH-30) {
		t.Fatal("попадание по коту определяется неверно")
	}
}

// Вне кота холст прозрачен: в этом смысл окна без фона. Каждая поза должна рисоваться.
func TestRenderIsTransparentAroundCat(t *testing.T) {
	canvas := NewCanvas()
	for _, mode := range []Mode{Sit, Walk, Play, Wash, Yawn, Sleep, Stretch, Carried, Fall, Land} {
		c := New(ViewW)
		if mode == Walk || mode == Play {
			c.Command(mode, nil)
		} else {
			c.Mode = mode
		}
		steps(c, 5)
		Render(canvas, c, 0)
		if canvas.Pix[3] != 0 || canvas.Pix[(canvas.W-1)*4+3] != 0 {
			t.Errorf("режим %v: верхние углы должны быть прозрачны", mode)
		}
		opaque := 0
		for i := 3; i < len(canvas.Pix); i += 4 {
			if canvas.Pix[i] == 255 {
				opaque++
			}
		}
		if opaque < 800 {
			t.Errorf("режим %v: кот не нарисован (непрозрачных пикселей %d)", mode, opaque)
		}
	}
}

// CAT_PNG=путь.png go test ./internal/cat -run TestDumpPNG — лист с позами, чтобы посмотреть глазами.
func TestDumpPNG(t *testing.T) {
	path := os.Getenv("CAT_PNG")
	if path == "" {
		t.Skip("CAT_PNG не задан")
	}
	type shot struct {
		name  string
		setup func(c *Cat)
	}
	shots := []shot{
		{"sit", func(c *Cat) {}},
		{"look", func(c *Cat) { c.Gaze, c.GazeY = 0.8, -0.3 }},
		{"butterfly", func(c *Cat) {
			c.event, c.evTick = evButterfly, 50
			c.Butterfly = &Butterfly{X: c.X + 90, Y: 45}
			c.Gaze, c.GazeY = 1, -0.8
		}},
		{"happy", func(c *Cat) { c.happy = 20 }},
		{"wash", func(c *Cat) { c.setMode(Wash); c.ModeTick = 50 }},
		{"yawn", func(c *Cat) { c.setMode(Yawn); c.ModeTick = 18 }},
		{"sleep", func(c *Cat) { c.setMode(Sleep) }},
		{"walk0", func(c *Cat) { c.Command(Walk, nil); c.Phase = 0.05 }},
		{"walk1", func(c *Cat) { c.Command(Walk, nil); c.Phase = 0.35 }},
		{"trot", func(c *Cat) { c.Command(Walk, nil); c.gait = gTrot; c.Phase = 0.2 }},
		{"sniff", func(c *Cat) { c.Command(Walk, nil); c.gait = gSniff }},
		{"lookup", func(c *Cat) { c.Command(Walk, nil); c.gait = gLookUp }},
		{"hop", func(c *Cat) { c.Command(Walk, nil); c.gait = gHop; c.Lift = 12 }},
		{"stretch", func(c *Cat) { c.setMode(Stretch); c.ModeTick = 28 }},
		{"stalk", func(c *Cat) { c.Command(Play, nil); c.phase = stalk }},
		{"leap", func(c *Cat) { c.Command(Play, nil); c.phase = leap; c.Lift = 14 }},
		{"carried", func(c *Cat) { c.setMode(Carried) }},
		{"fall", func(c *Cat) { c.setMode(Fall); c.Lift = 30 }},
	}
	// CAT_ONLY=sit,walk0 — только эти позы (удобно смотреть вблизи)
	if only := os.Getenv("CAT_ONLY"); only != "" {
		var keep []shot
		for _, s := range shots {
			if strings.Contains(","+only+",", ","+s.name+",") {
				keep = append(keep, s)
			}
		}
		shots = keep
	}
	cols := min(3, len(shots))
	rows := (len(shots) + cols - 1) / cols
	sheet := image.NewNRGBA(image.Rect(0, 0, ViewW*cols, ViewH*rows))
	draw.Draw(sheet, sheet.Bounds(), image.NewUniform(nrgbaGray), image.Point{}, draw.Src)
	canvas := NewCanvas()
	for i, s := range shots {
		c := New(ViewW)
		c.X = 70
		s.setup(c)
		if c.Mode == Walk || c.Mode == Play {
			c.Facing = 1
		}
		Render(canvas, c, 0)
		img := canvas.Image()
		r := image.Rect((i%cols)*ViewW, (i/cols)*ViewH, (i%cols+1)*ViewW, (i/cols+1)*ViewH)
		draw.Draw(sheet, r, img, image.Point{}, draw.Over)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, sheet); err != nil {
		t.Fatal(err)
	}
}

var nrgbaGray = color.NRGBA{0x5a, 0x62, 0x70, 255}

// CAT_PNG + CAT_SCALE=4 — тот же лист, увеличенный в несколько раз «ближайшим соседом».
func TestMain(m *testing.M) {
	code := m.Run()
	if p, s := os.Getenv("CAT_PNG"), os.Getenv("CAT_SCALE"); p != "" && s != "" {
		k := 0
		fmt.Sscanf(s, "%d", &k)
		if f, err := os.Open(p); err == nil && k > 1 {
			img, _ := png.Decode(f)
			f.Close()
			b := img.Bounds()
			out := image.NewNRGBA(image.Rect(0, 0, b.Dx()*k, b.Dy()*k))
			for y := 0; y < out.Bounds().Dy(); y++ {
				for x := 0; x < out.Bounds().Dx(); x++ {
					out.Set(x, y, img.At(b.Min.X+x/k, b.Min.Y+y/k))
				}
			}
			if o, err := os.Create(p[:len(p)-4] + "-x.png"); err == nil {
				png.Encode(o, out)
				o.Close()
			}
		}
	}
	os.Exit(code)
}
