//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// minBackdropBuild — первая сборка Windows 11 с типами подложки окна (Wails: SupportsBackdropTypes).
const minBackdropBuild = 22621

// colorKey — цвет окна под прозрачным веб-видом. Почти чёрный, чтобы по краям
// меню (скруглённые углы, тень) не было светлой каймы. Кот этого цвета не использует.
const colorKey = 0x0E0C0C // RGB(12, 12, 14) в формате COLORREF (0x00BBGGRR)

// colorKeyRGB — тот же цвет для options.RGBA в app_cat.go.
const colorKeyR, colorKeyG, colorKeyB = 12, 12, 14

// cutOutBackground делает окно кота прозрачным там, где виден цвет colorKey.
// Нужен на Windows до 11 (22621): настоящей прозрачности окна там у Wails нет,
// и вместо кота с фоном получается чёрный прямоугольник.
func cutOutBackground(title string) {
	user32 := syscall.NewLazyDLL("user32.dll")
	name, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return
	}
	hwnd, _, _ := user32.NewProc("FindWindowW").Call(0, uintptr(unsafe.Pointer(name)))
	if hwnd == 0 {
		return
	}
	const (
		gwlExStyle  = ^uintptr(19) // -20
		wsExLayered = 0x00080000
		lwaColorKey = 0x1
	)
	style, _, _ := user32.NewProc("GetWindowLongPtrW").Call(hwnd, gwlExStyle)
	user32.NewProc("SetWindowLongPtrW").Call(hwnd, gwlExStyle, style|wsExLayered)
	user32.NewProc("SetLayeredWindowAttributes").Call(hwnd, colorKey, 0, lwaColorKey)
}

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
