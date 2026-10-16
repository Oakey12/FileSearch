//go:build windows

package onec

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
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
	vtOpenStorage  = 6 // IStorage
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

// writeOLE копирует src в dst и заменяет потоки по путям вида
// «Хранилище/Вложенное/Поток» (у .ert модуль лежит в корне: «MD Programm text»).
func writeOLE(src, dst string, streams map[string][]byte) error {
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

	for path, data := range streams {
		if err := replaceStream(out, path, data); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	if hr := out.call(vtCommit, 0); failed(hr) {
		return fmt.Errorf("не удалось сохранить файл (0x%08X)", uint32(hr))
	}
	return nil
}

// replaceStream открывает вложенные хранилища по пути и перезаписывает поток.
func replaceStream(root *comObj, path string, data []byte) error {
	parts := strings.Split(path, "/")
	cur := root
	var opened []*comObj
	defer func() {
		// Изменения вложенных хранилищ фиксируются снизу вверх.
		for i := len(opened) - 1; i >= 0; i-- {
			opened[i].call(vtCommit, 0)
			opened[i].release()
		}
	}()
	for _, part := range parts[:len(parts)-1] {
		name, err := syscall.UTF16PtrFromString(part)
		if err != nil {
			return err
		}
		var sub *comObj
		if hr := cur.call(vtOpenStorage, uintptr(unsafe.Pointer(name)), 0, stgmReadWrite|stgmShareExclusive, 0, 0, uintptr(unsafe.Pointer(&sub))); failed(hr) || sub == nil {
			return fmt.Errorf("не найдено хранилище %q (0x%08X)", part, uint32(hr))
		}
		opened = append(opened, sub)
		cur = sub
	}
	leaf, err := syscall.UTF16PtrFromString(parts[len(parts)-1])
	if err != nil {
		return err
	}
	var s *comObj
	if hr := cur.call(vtCreateStream, uintptr(unsafe.Pointer(leaf)), stgmCreate|stgmReadWrite|stgmShareExclusive, 0, 0, uintptr(unsafe.Pointer(&s))); failed(hr) || s == nil {
		return fmt.Errorf("не удалось записать поток (0x%08X)", uint32(hr))
	}
	defer s.release()
	if len(data) == 0 {
		return nil
	}
	var written uint32
	if hr := s.call(vtWrite, uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), uintptr(unsafe.Pointer(&written))); failed(hr) || int(written) != len(data) {
		return errors.New("поток записан не полностью")
	}
	return nil
}
