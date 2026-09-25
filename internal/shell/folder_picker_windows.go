//go:build windows

package shell

import (
	"errors"
	"file-search/internal/varAndConst"
	"fmt"
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

// ErrCancelled возвращается, если пользователь закрыл окно выбора папки.
var ErrCancelled = errors.New("выбор папки отменён")

var (
	procCoCreateInstance             = ole32.NewProc("CoCreateInstance")
	procCoTaskMemFree                = ole32.NewProc("CoTaskMemFree")
	procSHCreateItemFromParsingNameW = shell32.NewProc("SHCreateItemFromParsingName")
)

type comGUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var (
	clsidFileOpenDialog = comGUID{0xDC1C5A9C, 0xE88A, 0x4DDE, [8]byte{0xA5, 0xA1, 0x60, 0xF8, 0x2A, 0x20, 0xAE, 0xF7}}
	iidIFileOpenDialog  = comGUID{0xD57C7288, 0xD4AD, 0x4768, [8]byte{0xBE, 0x02, 0x9D, 0x96, 0x95, 0x32, 0xD9, 0x60}}
	iidIShellItem       = comGUID{0x43826D1E, 0xE718, 0x42EE, [8]byte{0xBC, 0x55, 0xA1, 0xE2, 0x61, 0xC3, 0x7B, 0xFE}}
)

const (
	clsctxInprocServer = 0x1

	fosNoChangeDir      = 0x00000008
	fosPickFolders      = 0x00000020
	fosForceFileSystem  = 0x00000040
	fosPathMustExist    = 0x00000800
	sigdnFileSysPath    = 0x80058000
	hresultErrCancelled = 0x800704C7 // HRESULT_FROM_WIN32(ERROR_CANCELLED)

	// Номера методов в таблице виртуальных функций IFileOpenDialog.
	vtRelease    = 2
	vtShow       = 3
	vtSetOptions = 9
	vtGetOptions = 10
	vtSetFolder  = 12
	vtSetTitle   = 17
	vtGetResult  = 20

	// Номера методов IShellItem.
	vtGetDisplayName = 5
)

// comObject — указатель на COM-объект: первое поле объекта указывает на vtable.
type comObject struct {
	vtbl *[64]uintptr
}

func (object *comObject) call(method int, args ...uintptr) uintptr {
	all := append([]uintptr{uintptr(unsafe.Pointer(object))}, args...)
	hr, _, _ := syscall.SyscallN(object.vtbl[method], all...)
	return hr
}

func (object *comObject) release() {
	if object != nil {
		object.call(vtRelease)
	}
}

// PickFolder открывает стандартное окно Windows «Выбор папки».
// owner — HWND главного окна (0, если неизвестен), initialDir — папка,
// которая откроется первой (может быть пустой строкой).
// Функция блокирует вызывающую горутину до закрытия окна.
func PickFolder(owner uintptr, title, initialDir string) (string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hr, _, _ := procCoInitializeEx.Call(0, varAndConst.CoinitApartmentThreaded)
	if hresultFailed(hr) && uint32(hr) != varAndConst.RpcEChangedMode {
		return "", fmt.Errorf("не удалось инициализировать COM (HRESULT 0x%08X)", uint32(hr))
	}
	if !hresultFailed(hr) {
		defer procCoUninitialize.Call()
	}

	var dialog *comObject
	hr, _, _ = procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidFileOpenDialog)),
		0,
		clsctxInprocServer,
		uintptr(unsafe.Pointer(&iidIFileOpenDialog)),
		uintptr(unsafe.Pointer(&dialog)),
	)
	if hresultFailed(hr) || dialog == nil {
		return "", fmt.Errorf("не удалось создать окно выбора папки (HRESULT 0x%08X)", uint32(hr))
	}
	defer dialog.release()

	var options uint32
	dialog.call(vtGetOptions, uintptr(unsafe.Pointer(&options)))
	options |= fosPickFolders | fosForceFileSystem | fosPathMustExist | fosNoChangeDir
	if hr = dialog.call(vtSetOptions, uintptr(options)); hresultFailed(hr) {
		return "", fmt.Errorf("не удалось настроить окно выбора папки (HRESULT 0x%08X)", uint32(hr))
	}

	if title != "" {
		if titleUTF16, err := syscall.UTF16PtrFromString(title); err == nil {
			dialog.call(vtSetTitle, uintptr(unsafe.Pointer(titleUTF16)))
		}
	}

	if initialDir != "" {
		if info, err := os.Stat(initialDir); err == nil && info.IsDir() {
			if folder := shellItemFromPath(initialDir); folder != nil {
				dialog.call(vtSetFolder, uintptr(unsafe.Pointer(folder)))
				folder.release()
			}
		}
	}

	hr = dialog.call(vtShow, owner)
	if uint32(hr) == hresultErrCancelled {
		return "", ErrCancelled
	}
	if hresultFailed(hr) {
		return "", fmt.Errorf("ошибка окна выбора папки (HRESULT 0x%08X)", uint32(hr))
	}

	var item *comObject
	if hr = dialog.call(vtGetResult, uintptr(unsafe.Pointer(&item))); hresultFailed(hr) || item == nil {
		return "", fmt.Errorf("не удалось получить выбранную папку (HRESULT 0x%08X)", uint32(hr))
	}
	defer item.release()

	var namePtr *uint16
	if hr = item.call(vtGetDisplayName, sigdnFileSysPath, uintptr(unsafe.Pointer(&namePtr))); hresultFailed(hr) || namePtr == nil {
		return "", fmt.Errorf("выбранный объект не является папкой файловой системы (HRESULT 0x%08X)", uint32(hr))
	}
	defer procCoTaskMemFree.Call(uintptr(unsafe.Pointer(namePtr)))

	return utf16PtrToString(namePtr), nil
}

func shellItemFromPath(path string) *comObject {
	pathUTF16, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil
	}
	var item *comObject
	hr, _, _ := procSHCreateItemFromParsingNameW.Call(
		uintptr(unsafe.Pointer(pathUTF16)),
		0,
		uintptr(unsafe.Pointer(&iidIShellItem)),
		uintptr(unsafe.Pointer(&item)),
	)
	if hresultFailed(hr) {
		return nil
	}
	return item
}

func utf16PtrToString(p *uint16) string {
	length := 0
	for *(*uint16)(unsafe.Add(unsafe.Pointer(p), length*2)) != 0 {
		length++
	}
	return syscall.UTF16ToString(unsafe.Slice(p, length))
}