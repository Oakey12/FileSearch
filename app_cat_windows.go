//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// minBackdropBuild — первая сборка Windows 11 с типами подложки окна (Wails: SupportsBackdropTypes).
const minBackdropBuild = 22621

// supportsBackdrop сообщает, можно ли включать WindowIsTranslucent без размытия.
// На более старых Windows Wails заменяет это «размытием фона за окном» — вокруг кота
// появляется мутный прямоугольник, поэтому там опцию не включаем.
func supportsBackdrop() bool {
	var info struct {
		Size                uint32
		Major, Minor, Build uint32
		Platform            uint32
		CSD                 [128]uint16
	}
	info.Size = uint32(unsafe.Sizeof(info))
	proc := syscall.NewLazyDLL("ntdll.dll").NewProc("RtlGetVersion")
	if proc.Find() != nil {
		return false
	}
	if r, _, _ := proc.Call(uintptr(unsafe.Pointer(&info))); r != 0 {
		return false
	}
	return info.Build >= minBackdropBuild
}
