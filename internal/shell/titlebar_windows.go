//go:build windows

package shell

import (
	"syscall"
	"unsafe"
)

const dwmwaUseImmersiveDarkMode = 20

var procDwmSetWindowAttribute = syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")

// SetDarkTitleBar включает тёмную рамку окна независимо от темы Windows.
func SetDarkTitleBar(hwnd uintptr) {
	if hwnd == 0 || procDwmSetWindowAttribute.Find() != nil {
		return
	}
	enabled := int32(1)
	procDwmSetWindowAttribute.Call(hwnd, dwmwaUseImmersiveDarkMode, uintptr(unsafe.Pointer(&enabled)), unsafe.Sizeof(enabled))
}
