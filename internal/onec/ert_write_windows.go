//go:build windows

package onec

import (
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

// Запись .ert через системные составные документы Windows (OLE Structured Storage):
// весь исходный файл копируется как есть (IStorage::CopyTo), затем заменяется
// только поток с текстом модуля. Так сохраняются формы, таблицы и всё прочее,
// что мы не разбираем.

var (
	ole32                = syscall.NewLazyDLL("ole32.dll")
	procStgOpenStorage   = ole32.NewProc("StgOpenStorage")
	procStgCreateDocfile = ole32.NewProc("StgCreateDocfile")
	procCoInitializeEx   = ole32.NewProc("CoInitializeEx")
	procCoUninitialize   = ole32.NewProc("CoUninitialize")
)

const (
	stgmRead           = 0x00000000
	stgmReadWrite      = 0x00000002
	stgmShareDenyWrite = 0x00000020
	stgmShareExclusive = 0x00000010
	stgmCreate         = 0x00001000
	stgmTransacted     = 0x00010000

	// Номера методов в таблицах виртуальных функций.
	vtRelease      = 2
	vtCreateStream = 3 // IStorage
	vtCopyTo       = 7 // IStorage
	vtCommit       = 9 // IStorage
	vtWrite        = 4 // IStream
)

type comObj struct{ vtbl *[32]uintptr }

func (o *comObj) call(method int, args ...uintptr) uintptr {
	all := append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)
	hr, _, _ := syscall.SyscallN(o.vtbl[method], all...)
	return hr
}

func (o *comObj) release() {
	if o != nil {
		o.call(vtRelease)
	}
}

func failed(hr uintptr) bool { return int32(uint32(hr)) < 0 }

// writeERT копирует src в dst, заменяя поток streamName на data.
func writeERT(src, dst, streamName string, data []byte) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if hr, _, _ := procCoInitializeEx.Call(0, 0x2); !failed(hr) {
		defer procCoUninitialize.Call()
	}

	srcName, err := syscall.UTF16PtrFromString(src)
	if err != nil {
		return err
	}
	dstName, err := syscall.UTF16PtrFromString(dst)
	if err != nil {
		return err
	}
	stream, err := syscall.UTF16PtrFromString(streamName)
	if err != nil {
		return err
	}

	var in *comObj
	if hr, _, _ := procStgOpenStorage.Call(uintptr(unsafe.Pointer(srcName)), 0, stgmRead|stgmShareDenyWrite, 0, 0, uintptr(unsafe.Pointer(&in))); failed(hr) || in == nil {
		return fmt.Errorf("не удалось открыть исходный файл (0x%08X)", uint32(hr))
	}
	defer in.release()

	var out *comObj
	if hr, _, _ := procStgCreateDocfile.Call(uintptr(unsafe.Pointer(dstName)), stgmCreate|stgmReadWrite|stgmShareExclusive|stgmTransacted, 0, uintptr(unsafe.Pointer(&out))); failed(hr) || out == nil {
		return fmt.Errorf("не удалось создать файл (0x%08X)", uint32(hr))
	}
	defer out.release()

	if hr := in.call(vtCopyTo, 0, 0, 0, uintptr(unsafe.Pointer(out))); failed(hr) {
		return fmt.Errorf("не удалось скопировать содержимое (0x%08X)", uint32(hr))
	}

	var s *comObj
	if hr := out.call(vtCreateStream, uintptr(unsafe.Pointer(stream)), stgmCreate|stgmReadWrite|stgmShareExclusive, 0, 0, uintptr(unsafe.Pointer(&s))); failed(hr) || s == nil {
		return fmt.Errorf("не удалось записать модуль (0x%08X)", uint32(hr))
	}
	var written uint32
	var hr uintptr
	if len(data) > 0 {
		hr = s.call(vtWrite, uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), uintptr(unsafe.Pointer(&written)))
	}
	s.release()
	if failed(hr) || int(written) != len(data) {
		return errors.New("модуль записан не полностью")
	}
	if hr := out.call(vtCommit, 0); failed(hr) {
		return fmt.Errorf("не удалось сохранить файл (0x%08X)", uint32(hr))
	}
	return nil
}
