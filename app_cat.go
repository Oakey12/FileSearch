package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"file-search/internal/shell"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Рыжий кот — тот же exe, запущенный с ключом --cat: прозрачное окно без рамки
// поверх всех окон, в котором виден только сам кот. Окно поиска на это время
// прячется и возвращается в центр экрана, когда процесс кота завершится.

const catFlag = "--cat"

// Размер окна кота в точках интерфейса (как CSS-пиксели), см. VIEW_* в cat.ts.
const (
	catWidth  = 360
	catHeight = 220
)

// Строки, которые процесс кота печатает главному окну: кот нарисовался (можно
// прятать поиск) и анимация открытия дошла до места окна (можно показывать поиск).
const (
	catReady = "cat:ready"
	catTitle = "Рыжик" // заголовок окна кота; по нему cutOutBackground находит окно
	catOpen  = "cat:open"
)

func isCatProcess() bool {
	return slices.Contains(os.Args[1:], catFlag)
}

// TurnIntoCat запускает кота и прячет окно поиска, как только кот появится.
func (a *App) TurnIntoCat() error {
	a.catMu.Lock()
	defer a.catMu.Unlock()
	if a.catOn {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("не удалось превратиться в кота: %w", err)
	}
	w, h := runtime.WindowGetSize(a.ctx)
	args := []string{catFlag, fmt.Sprintf("--app=%dx%d", w, h)}
	if runtime.WindowIsMaximised(a.ctx) {
		args = append(args, "--max")
	}
	if savedTheme() == "light" {
		args = append(args, "--light")
	}
	cmd := exec.Command(exe, args...)
	cmd.Env = catEnv(os.Environ())
	// Кот живёт, пока открыт его stdin: если главное окно упадёт, кот уйдёт следом.
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("не удалось превратиться в кота: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("не удалось превратиться в кота: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("не удалось превратиться в кота: %w", err)
	}
	a.catOn = true

	go func() {
		lines := bufio.NewScanner(stdout)
		for lines.Scan() {
			switch strings.TrimSpace(lines.Text()) {
			case catReady:
				runtime.WindowHide(a.ctx)
			case catOpen:
				a.showFromCat()
			}
		}
	}()
	go func() {
		_ = cmd.Wait()
		stdin.Close()
		a.catMu.Lock()
		a.catOn = false
		a.catMu.Unlock()
		a.showFromCat() // если кот закрылся без анимации (упал, сняли задачу)
	}()
	return nil
}

// showFromCat возвращает окно поиска в центр экрана. Центрируем до показа, чтобы
// окно не мелькнуло на старом месте.
func (a *App) showFromCat() {
	if !runtime.WindowIsMaximised(a.ctx) {
		runtime.WindowCenter(a.ctx)
	}
	runtime.WindowShow(a.ctx)
	runtime.WindowUnminimise(a.ctx)
}

// CatSetup — всё, что нужно окну кота: экран без панели задач (физические пиксели),
// а для анимации открытия — размер и тема окна поиска.
func (a *App) CatSetup() CatSetup {
	s := parseCatArgs(os.Args[1:])
	s.Width, s.Height = shell.WorkArea()
	return s
}

// CatOpened — анимация открытия дошла до места окна поиска: пусть оно появляется.
func (a *App) CatOpened() {
	fmt.Println(catOpen)
}

// CatSetup — параметры окна кота.
type CatSetup struct {
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	AppWidth  int  `json:"appWidth"`
	AppHeight int  `json:"appHeight"`
	Maximised bool `json:"maximised"`
	Light     bool `json:"light"`
}

func parseCatArgs(args []string) CatSetup {
	s := CatSetup{AppWidth: 1180, AppHeight: 740}
	for _, arg := range args {
		switch {
		case arg == "--max":
			s.Maximised = true
		case arg == "--light":
			s.Light = true
		case strings.HasPrefix(arg, "--app="):
			var w, h int
			if n, _ := fmt.Sscanf(strings.TrimPrefix(arg, "--app="), "%dx%d", &w, &h); n == 2 && w > 0 && h > 0 {
				s.AppWidth, s.AppHeight = w, h
			}
		}
	}
	return s
}

// catEnv — окружение для процесса кота. Под wails dev у кота должен быть свой
// порт dev-сервера, иначе он столкнётся с главным окном.
func catEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if strings.HasPrefix(strings.ToLower(kv), "devserver=") {
			kv = "devserver=127.0.0.1:34116"
		}
		out = append(out, kv)
	}
	return out
}

// catPage подменяет стартовую страницу на cat.html.
func catPage(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			r.URL.Path = "/cat.html"
		}
		next.ServeHTTP(w, r)
	})
}

func runCat() error {
	app := NewApp()
	return wails.Run(&options.App{
		Title:            catTitle,
		Width:            catWidth,
		Height:           catHeight,
		Frameless:        true,
		DisableResize:    true,
		AlwaysOnTop:      true,
		StartHidden:      true,                                                    // окно показывает cat.ts, когда поставит кота на место
		BackgroundColour: &options.RGBA{R: colorKeyR, G: colorKeyG, B: colorKeyB}, // на Windows до 11 этот цвет вырезается (cutOutBackground)
		AssetServer:      &assetserver.Options{Assets: assets, Middleware: catPage},
		OnStartup: func(ctx context.Context) {
			app.ctx = ctx
			go quitWithParent(ctx)
		},
		OnDomReady: func(ctx context.Context) {
			if !supportsBackdrop() {
				cutOutBackground(catTitle)
			}
			fmt.Println(catReady)
			// Страховка: если интерфейс не показал окно сам, кот не должен пропасть вместе с поиском.
			time.AfterFunc(3*time.Second, func() { runtime.WindowShow(ctx) })
		},
		Bind: []any{app},
		Windows: &windows.Options{
			WebviewIsTransparent: true,
			// Старые Windows (7, Server 2008–2012): аппаратная графика в layered-окне рисуется
			// чёрной, поэтому для кота включаем программную отрисовку.
			WebviewGpuIsDisabled: !supportsBackdrop(),
			// Только на Windows 11 22621+: на старых версиях Wails включает размытие фона за окном.
			WindowIsTranslucent:               supportsBackdrop(),
			BackdropType:                      windows.None,
			DisableFramelessWindowDecorations: true,
			DisablePinchZoom:                  true,
		},
	})
}

// quitWithParent закрывает кота, когда главное окно закрылось (stdin кончился).
// Если кота запустили вручную, без канала от главного окна, ждать нечего.
func quitWithParent(ctx context.Context) {
	if st, err := os.Stdin.Stat(); err != nil || st.Mode()&os.ModeNamedPipe == 0 {
		return
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
	runtime.Quit(ctx)
}
