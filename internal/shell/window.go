package shell

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
)

// WindowHandle возвращает HWND окна Fyne (0, если окно ещё не создано или это не Windows).
// Вызывать из главного потока Fyne.
func WindowHandle(window fyne.Window) uintptr {
	native, ok := window.(driver.NativeWindow)
	if !ok {
		return 0
	}
	var hwnd uintptr
	native.RunNative(func(context any) {
		if windowsContext, ok := context.(driver.WindowsWindowContext); ok {
			hwnd = windowsContext.HWND
		}
	})
	return hwnd
}
