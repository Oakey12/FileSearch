//go:build windows

package preview

import (
	"syscall"
	"time"
	"unsafe"
)

var (
	user32Proc       = syscall.NewLazyDLL("user32.dll")
	findWindowWProc  = user32Proc.NewProc("FindWindowW")
	setWindowPosProc = user32Proc.NewProc("SetWindowPos")
)

const (
	swpNoSize     = 0x0001
	swpNoZOrder   = 0x0004
	swpNoActivate = 0x0010
)

// moveWindowByTitle ждёт создания нативного окна Fyne и перемещает его,
// не меняя размер, Z-порядок и состояние активности.
func moveWindowByTitle(title string, x, y int32) {
	titleUTF16, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return
	}

	for range 40 {
		handle, _, _ := findWindowWProc.Call(
			0,
			uintptr(unsafe.Pointer(titleUTF16)),
		)
		if handle != 0 {
			setWindowPosProc.Call(
				handle,
				0,
				uintptr(int64(x)),
				uintptr(int64(y)),
				0,
				0,
				swpNoSize|swpNoZOrder|swpNoActivate,
			)
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
}
