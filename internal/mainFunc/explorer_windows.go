//go:build windows

package mainFunc

import (
	"file-search/internal/varAndConst"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"
)

var (
	shell32                   = syscall.NewLazyDLL("shell32.dll")
	ole32                     = syscall.NewLazyDLL("ole32.dll")
	procILCreateFromPathW     = shell32.NewProc("ILCreateFromPathW")
	procILFree                = shell32.NewProc("ILFree")
	procSHOpenFolderAndSelect = shell32.NewProc("SHOpenFolderAndSelectItems")
	procCoInitializeEx        = ole32.NewProc("CoInitializeEx")
	procCoUninitialize        = ole32.NewProc("CoUninitialize")
)

func ShowInExplorer(path string) (selected bool, err error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return false, err
	}
	path = absolutePath

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hr, _, _ := procCoInitializeEx.Call(0, varAndConst.CoinitApartmentThreaded)
	coInitialized := !hresultFailed(hr)
	if hresultFailed(hr) && uint32(hr) != varAndConst.RpcEChangedMode {
		return openParentFolder(path, fmt.Errorf("не удалось инициализировать Windows Shell (HRESULT 0x%08X)", uint32(hr)))
	}
	if coInitialized {
		defer procCoUninitialize.Call()
	}

	pathUTF16, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false, err
	}

	pidl, _, _ := procILCreateFromPathW.Call(uintptr(unsafe.Pointer(pathUTF16)))
	if pidl == 0 {
		return openParentFolder(path, fmt.Errorf("Windows Shell не распознал путь к файлу"))
	}
	defer procILFree.Call(pidl)

	hr, _, _ = procSHOpenFolderAndSelect.Call(pidl, 0, 0, 0)
	if hresultFailed(hr) {
		return openParentFolder(path, fmt.Errorf("не удалось выделить файл (HRESULT 0x%08X)", uint32(hr)))
	}
	return true, nil
}

func openParentFolder(path string, selectErr error) (bool, error) {
	if err := exec.Command("explorer.exe", filepath.Dir(path)).Start(); err != nil {
		return false, fmt.Errorf("%v; не удалось открыть папку: %w", selectErr, err)
	}
	return false, nil
}

func hresultFailed(hr uintptr) bool {
	return int32(uint32(hr)) < 0
}
