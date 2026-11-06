//go:build windows

// Package catwin показывает рыжего кота в окне без фона и рамки поверх всех окон.
//
// Это обычное окно Win32 с попиксельной прозрачностью (UpdateLayeredWindow), без
// WebView: оно работает на любой Windows начиная с 2000/XP, в том числе на Windows 7
// и Windows Server 2008–2012, где настоящей прозрачности у веб-окон нет. Прозрачные
// пиксели пропускают клики, поэтому кот «кликабелен» только там, где он нарисован.
package catwin

import (
	"errors"
	"fmt"
	"math"
	"runtime"
	"syscall"
	"unsafe"

	"file-search/internal/cat"
)

// Hooks — что делает окно кота для вызывающего.
type Hooks struct {
	// Ready вызывается, когда кот нарисован и показан. quit можно вызвать из любого
	// потока: окно закроется, а Run вернётся.
	Ready func(quit func())
	// Open вызывается по двойному клику: пользователь хочет вернуться к окну поиска.
	// После этого окно кота закрывается само.
	Open func()
}

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	pRegisterClassEx    = user32.NewProc("RegisterClassExW")
	pCreateWindowEx     = user32.NewProc("CreateWindowExW")
	pDefWindowProc      = user32.NewProc("DefWindowProcW")
	pShowWindow         = user32.NewProc("ShowWindow")
	pGetMessage         = user32.NewProc("GetMessageW")
	pTranslateMessage   = user32.NewProc("TranslateMessage")
	pDispatchMessage    = user32.NewProc("DispatchMessageW")
	pPostQuitMessage    = user32.NewProc("PostQuitMessage")
	pPostMessage        = user32.NewProc("PostMessageW")
	pSetTimer           = user32.NewProc("SetTimer")
	pKillTimer          = user32.NewProc("KillTimer")
	pSetCapture         = user32.NewProc("SetCapture")
	pReleaseCapture     = user32.NewProc("ReleaseCapture")
	pGetCursorPos       = user32.NewProc("GetCursorPos")
	pLoadCursor         = user32.NewProc("LoadCursorW")
	pGetDC              = user32.NewProc("GetDC")
	pReleaseDC          = user32.NewProc("ReleaseDC")
	pUpdateLayered      = user32.NewProc("UpdateLayeredWindow")
	pSystemParamInfo    = user32.NewProc("SystemParametersInfoW")
	pSetProcessDPIAware = user32.NewProc("SetProcessDPIAware")
	pCreatePopupMenu    = user32.NewProc("CreatePopupMenu")
	pAppendMenu         = user32.NewProc("AppendMenuW")
	pTrackPopupMenu     = user32.NewProc("TrackPopupMenu")
	pDestroyMenu        = user32.NewProc("DestroyMenu")
	pSetForegroundWnd   = user32.NewProc("SetForegroundWindow")
	pGetModuleHandle    = kernel32.NewProc("GetModuleHandleW")

	pCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	pCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	pSelectObject       = gdi32.NewProc("SelectObject")
	pDeleteObject       = gdi32.NewProc("DeleteObject")
	pDeleteDC           = gdi32.NewProc("DeleteDC")
	pGetDeviceCaps      = gdi32.NewProc("GetDeviceCaps")
)

const (
	wsPopup        = 0x80000000
	wsExTopmost    = 0x00000008
	wsExToolWindow = 0x00000080
	wsExLayered    = 0x00080000
	csDblClks      = 0x0008
	swShowNoActive = 4
	idcHand        = 32649
	spiGetWorkArea = 0x0030
	logPixelsX     = 88
	ulwAlpha       = 0x2

	wmDestroy     = 0x0002
	wmClose       = 0x0010
	wmNull        = 0x0000
	wmTimer       = 0x0113
	wmMouseMove   = 0x0200
	wmLButtonDown = 0x0201
	wmLButtonUp   = 0x0202
	wmLButtonDbl  = 0x0203
	wmRButtonUp   = 0x0205

	tpmCenterAlign = 0x0004
	tpmBottomAlign = 0x0020
	tpmReturnCmd   = 0x0100
	mfString       = 0x0000
	mfChecked      = 0x0008

	timerTick  = 1
	timerClick = 2
	timerQuit  = 3
	tickMs     = 1000 / cat.TicksPerSecond // 20 тиков в секунду, как в модели
	clickWait  = 250                       // ждём: вдруг это первый клик двойного
	dragSlop   = 5                         // на сколько пикселей надо сдвинуть мышь, чтобы это стало «несением»
)

type point struct{ X, Y int32 }
type size struct{ CX, CY int32 }
type rect struct{ Left, Top, Right, Bottom int32 }

type msg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type bitmapInfoHeader struct {
	Size          uint32
	Width, Height int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPels, YPels  int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type blendFunction struct{ Op, Flags, SrcAlpha, AlphaFormat byte }

// window — всё состояние окна кота; окно одно на процесс, поэтому оно глобальное.
type window struct {
	hwnd   uintptr
	memDC  uintptr
	bitmap uintptr
	bits   []byte
	hooks  Hooks

	cat    *cat.Cat
	canvas *cat.Canvas
	k      int // во сколько раз растянут холст
	winW   int
	winH   int
	wa     rect // рабочая область экрана (без панели задач)

	view float64 // левый край окна в клетках мира
	winX float64 // положение окна от левого верхнего угла рабочей области, пиксели
	winY float64

	shown, opening bool
	press          *point
	drag           *dragState
}

type dragState struct {
	winX, winY float64
	sx, sy     int32
	lastX      int32
}

var w = &window{}

// Run показывает кота и крутит цикл сообщений, пока окно не закроют.
func Run(h Hooks) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	pSetProcessDPIAware.Call()
	w.hooks = h
	if err := w.setup(); err != nil {
		return err
	}
	defer w.release()

	var m msg
	for {
		r, _, _ := pGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return nil
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func utf16(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

func (w *window) setup() error {
	// Масштаб экрана: пиксель кота — 2 «обычных» экранных пикселя, на 150% экране — 3.
	dc, _, _ := pGetDC.Call(0)
	dpi, _, _ := pGetDeviceCaps.Call(dc, logPixelsX)
	pReleaseDC.Call(0, dc)
	if dpi == 0 {
		dpi = 96
	}
	w.k = max(1, int(math.Round(2*float64(dpi)/96)))

	w.canvas = cat.NewCanvas()
	w.winW, w.winH = w.canvas.W*w.k, w.canvas.H*w.k
	pSystemParamInfo.Call(spiGetWorkArea, 0, uintptr(unsafe.Pointer(&w.wa)), 0)
	workW, workH := int(w.wa.Right-w.wa.Left), int(w.wa.Bottom-w.wa.Top)
	if workW <= 0 || workH <= 0 {
		workW, workH = 1024, 768
		w.wa = rect{0, 0, int32(workW), int32(workH)}
	}
	w.cat = cat.New(math.Max(cat.ViewW, math.Floor(float64(workW)/float64(w.k))))
	w.winY = float64(workH - w.winH)
	w.view = -1

	if err := w.createBitmap(); err != nil {
		return err
	}

	inst, _, _ := pGetModuleHandle.Call(0)
	cursor, _, _ := pLoadCursor.Call(0, idcHand)
	className := utf16("FileSearchCat")
	wc := wndClassEx{
		Style:     csDblClks,
		WndProc:   syscall.NewCallback(wndProc),
		Instance:  inst,
		Cursor:    cursor,
		ClassName: className,
	}
	wc.Size = uint32(unsafe.Sizeof(wc))
	if r, _, err := pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return fmt.Errorf("не удалось создать окно кота: %w", err)
	}
	hwnd, _, err := pCreateWindowEx.Call(
		wsExLayered|wsExTopmost|wsExToolWindow, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(utf16("Рыжик"))),
		wsPopup, uintptr(w.wa.Left), uintptr(w.wa.Top), uintptr(w.winW), uintptr(w.winH), 0, 0, inst, 0)
	if hwnd == 0 {
		return fmt.Errorf("не удалось создать окно кота: %w", err)
	}
	w.hwnd = hwnd

	w.follow()
	pShowWindow.Call(hwnd, swShowNoActive)
	pSetTimer.Call(hwnd, timerTick, tickMs, 0)
	if w.hooks.Ready != nil {
		w.hooks.Ready(func() { pPostMessage.Call(hwnd, wmClose, 0, 0) })
	}
	return nil
}

// createBitmap делает буфер 32 бита на пиксель, в который рисуется кот.
func (w *window) createBitmap() error {
	screen, _, _ := pGetDC.Call(0)
	defer pReleaseDC.Call(0, screen)
	w.memDC, _, _ = pCreateCompatibleDC.Call(screen)
	bmi := bitmapInfoHeader{Width: int32(w.winW), Height: -int32(w.winH), Planes: 1, BitCount: 32}
	bmi.Size = uint32(unsafe.Sizeof(bmi))
	var bits unsafe.Pointer
	bmp, _, _ := pCreateDIBSection.Call(w.memDC, uintptr(unsafe.Pointer(&bmi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bmp == 0 || bits == nil {
		return errors.New("не удалось выделить буфер для кота")
	}
	w.bitmap = bmp
	w.bits = unsafe.Slice((*byte)(bits), w.winW*w.winH*4)
	pSelectObject.Call(w.memDC, bmp)
	return nil
}

func (w *window) release() {
	if w.memDC != 0 {
		pDeleteDC.Call(w.memDC)
	}
	if w.bitmap != 0 {
		pDeleteObject.Call(w.bitmap)
	}
}

// present рисует кота и выставляет окно на место одним вызовом UpdateLayeredWindow.
func (w *window) present() {
	cat.Render(w.canvas, w.cat, w.view)

	// Растягиваем холст в k раз «ближайшим соседом»: пиксели остаются квадратными.
	row := make([]byte, w.winW*4)
	for y := 0; y < w.canvas.H; y++ {
		src := w.canvas.Pix[y*w.canvas.W*4 : (y+1)*w.canvas.W*4]
		for x := 0; x < w.canvas.W; x++ {
			for i := 0; i < w.k; i++ {
				copy(row[(x*w.k+i)*4:], src[x*4:x*4+4])
			}
		}
		for i := 0; i < w.k; i++ {
			copy(w.bits[(y*w.k+i)*w.winW*4:], row)
		}
	}

	pos := point{w.wa.Left + int32(math.Round(w.winX)), w.wa.Top + int32(math.Round(w.winY))}
	sz := size{int32(w.winW), int32(w.winH)}
	src := point{}
	blend := blendFunction{Op: 0 /* AC_SRC_OVER */, SrcAlpha: 255, AlphaFormat: 1 /* AC_SRC_ALPHA */}
	pUpdateLayered.Call(w.hwnd, 0, uintptr(unsafe.Pointer(&pos)), uintptr(unsafe.Pointer(&sz)),
		w.memDC, uintptr(unsafe.Pointer(&src)), 0, uintptr(unsafe.Pointer(&blend)), ulwAlpha)
	w.shown = true
}

// viewLeft — левый край окна в клетках: окно едет за котом (а в игре — за котом и клубком).
func (w *window) viewLeft() float64 {
	focus := w.cat.X + cat.SprW/2
	if w.cat.Ball != nil {
		focus = (focus + w.cat.Ball.X + cat.BallSize/2) / 2
	}
	return math.Floor(math.Max(0, math.Min(w.cat.Width-cat.ViewW, focus-cat.ViewW/2)) + 0.5)
}

func (w *window) follow() {
	if w.cat.Mode == cat.Carried || w.opening {
		return
	}
	left := w.viewLeft()
	if left == w.view && w.shown {
		return
	}
	w.view = left
	w.winX = w.view * float64(w.k)
}

func (w *window) tick() {
	if w.opening {
		return
	}
	w.cat.Step(nil)
	if w.cat.Mode == cat.Carried {
		w.cat.Sway *= 0.75
	}
	w.follow()
	w.present()
}

func (w *window) onCat(cx, cy int32) bool {
	return w.cat.HitCat(w.view+float64(cx)/float64(w.k), float64(cy)/float64(w.k)-cat.GroundY)
}

func cursor() point {
	var p point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	return p
}

func (w *window) startDrag(cx, cy int32, cur point) {
	pKillTimer.Call(w.hwnd, timerClick)
	w.cat.Carry()
	// Взяли за шкирку: окно прыгает так, чтобы загривок оказался под курсором.
	w.winX += float64(cx) - cat.Anchor.X*float64(w.k)
	w.winY += float64(cy) - cat.Anchor.Y*float64(w.k)
	w.drag = &dragState{winX: w.winX, winY: w.winY, sx: cur.X, sy: cur.Y, lastX: cur.X}
	w.present()
}

func (w *window) moveDrag(cur point) {
	d := w.drag
	w.winX = d.winX + float64(cur.X-d.sx)
	w.winY = d.winY + float64(cur.Y-d.sy)
	// Кот раскачивается против движения: чем резче рывок, тем сильнее.
	w.cat.Sway = math.Max(-2, math.Min(2, w.cat.Sway*0.6+float64(cur.X-d.lastX)/8))
	d.lastX = cur.X
	w.present()
}

// endDrag: где отпустили, там теперь и пол. Окно не уходит за края рабочей области.
func (w *window) endDrag() {
	w.drag = nil
	maxY := float64(int(w.wa.Bottom-w.wa.Top) - w.winH)
	w.winY = math.Max(0, math.Min(maxY, w.winY))
	w.view = math.Floor(w.winX/float64(w.k) + 0.5)
	w.cat.X = math.Max(0, math.Min(w.cat.Width-cat.SprW, w.view+cat.Anchor.X-cat.SprW/2))
	w.view = w.viewLeft()
	w.winX = w.view * float64(w.k)
	w.cat.Drop(cat.DropHeight)
	w.present()
}

func (w *window) showMenu() {
	if w.opening {
		return
	}
	menu, _, _ := pCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer pDestroyMenu.Call(menu)
	for i, it := range []struct {
		text string
		mode cat.Mode
	}{{"Сидеть", cat.Sit}, {"Играть", cat.Play}, {"Гулять", cat.Walk}} {
		flags := uintptr(mfString)
		if w.cat.Mode == it.mode {
			flags |= mfChecked
		}
		pAppendMenu.Call(menu, flags, uintptr(i+1), uintptr(unsafe.Pointer(utf16(it.text))))
	}
	// Меню открывается над котом.
	x := float64(w.wa.Left) + w.winX + (w.cat.X+cat.SprW/2-w.view)*float64(w.k)
	y := float64(w.wa.Top) + w.winY + (cat.GroundY-cat.SprH-w.cat.Lift)*float64(w.k) - 8
	pSetForegroundWnd.Call(w.hwnd) // иначе меню не закроется кликом мимо
	id, _, _ := pTrackPopupMenu.Call(menu, tpmCenterAlign|tpmBottomAlign|tpmReturnCmd, uintptr(int32(x)), uintptr(int32(y)), 0, w.hwnd, 0)
	pPostMessage.Call(w.hwnd, wmNull, 0, 0)
	switch id {
	case 1:
		w.cat.Command(cat.Sit, nil)
	case 2:
		w.cat.Command(cat.Play, nil)
	case 3:
		w.cat.Command(cat.Walk, nil)
	}
	if id != 0 {
		w.tick()
	}
}

func (w *window) openApp() {
	if w.opening {
		return
	}
	w.opening = true
	pKillTimer.Call(w.hwnd, timerClick)
	if w.hooks.Open != nil {
		w.hooks.Open()
	}
	pSetTimer.Call(w.hwnd, timerQuit, 250, 0) // даём окну поиска появиться, потом кот уходит
}

func wndProc(hwnd, m, wp, lp uintptr) uintptr {
	cx, cy := int32(int16(lp&0xffff)), int32(int16((lp>>16)&0xffff))
	switch uint32(m) {
	case wmTimer:
		switch wp {
		case timerTick:
			w.tick()
		case timerClick:
			pKillTimer.Call(hwnd, timerClick)
			w.cat.Poke()
			w.showMenu()
		case timerQuit:
			pKillTimer.Call(hwnd, timerQuit)
			pPostMessage.Call(hwnd, wmClose, 0, 0)
		}
		return 0
	case wmLButtonDown:
		if w.opening || !w.onCat(cx, cy) {
			return 0
		}
		cur := cursor()
		w.press = &cur
		pSetCapture.Call(hwnd)
		return 0
	case wmMouseMove:
		if w.press == nil {
			return 0
		}
		cur := cursor()
		if w.drag == nil {
			if math.Hypot(float64(cur.X-w.press.X), float64(cur.Y-w.press.Y)) < dragSlop {
				return 0
			}
			w.startDrag(cx, cy, cur)
			return 0
		}
		w.moveDrag(cur)
		return 0
	case wmLButtonUp:
		if w.press == nil {
			return 0
		}
		w.press = nil
		pReleaseCapture.Call()
		if w.drag != nil {
			w.endDrag()
			return 0
		}
		if !w.opening && w.onCat(cx, cy) {
			pSetTimer.Call(hwnd, timerClick, clickWait, 0)
		}
		return 0
	case wmLButtonDbl:
		if !w.opening && w.cat.Mode != cat.Carried && w.onCat(cx, cy) {
			w.openApp()
		}
		return 0
	case wmRButtonUp:
		if w.onCat(cx, cy) {
			w.showMenu()
		}
		return 0
	case wmDestroy:
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, m, wp, lp)
	return r
}
