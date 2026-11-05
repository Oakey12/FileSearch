//go:build windows

package shell

import (
	"syscall"
	"unsafe"
)

const spiGetWorkArea = 0x0030

var procSystemParametersInfoW = syscall.NewLazyDLL("user32.dll").NewProc("SystemParametersInfoW")

// WorkArea возвращает размер рабочей области основного монитора (экран без панели задач)
// в физических пикселях.
func WorkArea() (width, height int) {
	var r struct{ Left, Top, Right, Bottom int32 }
	if ok, _, _ := procSystemParametersInfoW.Call(spiGetWorkArea, 0, uintptr(unsafe.Pointer(&r)), 0); ok == 0 {
		return 0, 0
	}
	return int(r.Right - r.Left), int(r.Bottom - r.Top)
}
