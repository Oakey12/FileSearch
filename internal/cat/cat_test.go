package cat

import (
	"image/png"
	"os"
	"testing"
)

func steps(c *Cat, n int) {
	for i := 0; i < n; i++ {
		c.Step(func() float64 { return 0.5 })
	}
}

func TestIdleSequence(t *testing.T) {
	c := New(200)
	steps(c, 40)
	if c.Mode != Yawn {
		t.Fatalf("после 40 тиков ждали зевок, а режим %v", c.Mode)
	}
	steps(c, 16+50)
	if c.Mode != Wash {
		t.Fatalf("ждали умывание, а режим %v", c.Mode)
	}
	steps(c, 40+80)
	if c.Mode != Sleep {
		t.Fatalf("ждали сон, а режим %v", c.Mode)
	}
	c.Poke()
	if c.Mode != Stretch {
		t.Fatalf("клик по спящему должен будить, а режим %v", c.Mode)
	}
}

func TestWalkBouncesAtEdges(t *testing.T) {
	c := New(120)
	c.Command(Walk, nil)
	for i := 0; i < 299; i++ {
		c.Step(nil)
		if c.X < edge || c.X > c.maxX() {
			t.Fatalf("кот вышел за пол: x=%v", c.X)
		}
	}
}

func TestPlayPutsBallOnFreeSide(t *testing.T) {
	c := New(200)
	c.Command(Play, func() float64 { return 0 })
	if c.Ball == nil || c.Ball.X <= c.X {
		t.Fatalf("клубок должен лечь справа: %+v", c.Ball)
	}
	steps(c, 400)
	if c.Mode == Play && c.phase != leap {
		t.Fatalf("игра должна закончиться, режим %v", c.Mode)
	}
}

func TestCarryDropLand(t *testing.T) {
	c := New(200)
	c.Carry()
	c.Drop(DropHeight)
	for i := 0; i < 30 && c.Mode == Fall; i++ {
		c.Step(nil)
	}
	if c.Mode != Land {
		t.Fatalf("кот должен приземлиться, режим %v", c.Mode)
	}
	steps(c, 5)
	if c.Mode != Sit {
		t.Fatalf("после приземления кот садится, режим %v", c.Mode)
	}
}

func TestHitCat(t *testing.T) {
	c := New(200)
	if !c.HitCat(c.X+SprW/2, -SprH/2) || c.HitCat(c.X-20, -5) || c.HitCat(c.X+5, -SprH-10) {
		t.Fatal("попадание по коту определяется неверно")
	}
}

// Вне кота холст прозрачен: в этом смысл окна без фона.
func TestRenderIsTransparentAroundCat(t *testing.T) {
	canvas := NewCanvas()
	c := New(ViewW)
	view := 0.0
	for _, mode := range []Mode{Sit, Walk, Sleep, Carried} {
		c.Mode = mode
		Render(canvas, c, view)
		if canvas.Pix[3] != 0 || canvas.Pix[(canvas.W-1)*4+3] != 0 {
			t.Errorf("режим %v: левый верхний и правый верхний углы должны быть прозрачны", mode)
		}
		opaque := 0
		for i := 3; i < len(canvas.Pix); i += 4 {
			if canvas.Pix[i] == 255 {
				opaque++
			}
		}
		if opaque < 200 {
			t.Errorf("режим %v: кот не нарисован (непрозрачных пикселей %d)", mode, opaque)
		}
	}
}

// CAT_PNG=путь.png go test ./internal/cat -run TestDumpPNG — сохранить кадр, чтобы посмотреть глазами.
func TestDumpPNG(t *testing.T) {
	path := os.Getenv("CAT_PNG")
	if path == "" {
		t.Skip("CAT_PNG не задан")
	}
	canvas := NewCanvas()
	c := New(ViewW)
	Render(canvas, c, 0)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, canvas.Image()); err != nil {
		t.Fatal(err)
	}
}
