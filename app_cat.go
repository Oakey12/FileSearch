package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"

	"file-search/internal/catwin"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Рыжий кот — тот же exe, запущенный с ключом --cat: окно без фона и рамки поверх
// всех окон, в котором виден только сам кот (см. пакет catwin — это обычное окно Win32
// с попиксельной прозрачностью, оно не зависит от WebView и работает и на Windows 7).
// Окно поиска на это время прячется и возвращается в центр экрана, когда процесс кота
// завершится.

const catFlag = "--cat"

// Строки, которые процесс кота печатает главному окну: кот нарисовался (можно
// прятать поиск) и пользователь вызвал окно поиска двойным кликом (можно показывать).
const (
	catReady = "cat:ready"
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
	cmd := exec.Command(exe, catFlag)
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
		a.showFromCat() // если кот закрылся без двойного клика (упал, сняли задачу)
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

// runCat — процесс кота: показывает окно и сообщает главному окну по stdout,
// что кот готов и что его позвали обратно.
func runCat() error {
	return catwin.Run(catwin.Hooks{
		Ready: func(quit func()) {
			fmt.Println(catReady)
			go quitWithParent(quit)
		},
		Open: func() { fmt.Println(catOpen) },
	})
}

// quitWithParent закрывает кота, когда главное окно закрылось (stdin кончился).
// Если кота запустили вручную, без канала от главного окна, ждать нечего.
func quitWithParent(quit func()) {
	if st, err := os.Stdin.Stat(); err != nil || st.Mode()&os.ModeNamedPipe == 0 {
		return
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
	quit()
}
