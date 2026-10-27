//go:build windows

package remote

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

var (
	mpr                  = syscall.NewLazyDLL("mpr.dll")
	procAddConnection    = mpr.NewProc("WNetAddConnection2W")
	procCancelConnection = mpr.NewProc("WNetCancelConnection2W")
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	procDiskFreeSpaceEx  = kernel32.NewProc("GetDiskFreeSpaceExW")
)

// netResource — NETRESOURCEW из winnetwk.h.
type netResource struct {
	Scope       uint32
	Type        uint32
	DisplayType uint32
	Usage       uint32
	LocalName   *uint16
	RemoteName  *uint16
	Comment     *uint16
	Provider    *uint16
}

// Коды Windows, которые стоит объяснить по-русски.
const (
	errAccessDenied     = 5
	errBadNetPath       = 53
	errNetNameNotFound  = 67
	errBadUserOrPass    = 86
	errBadDevice        = 1200
	errNoLogonServers   = 1311
	errLogonFailure     = 1326
	errAccountRestrict  = 1327
	errAccountDisabled  = 1331
	errNetworkUnreach   = 1231
	errHostUnreach      = 1232
	errSessionCredConfl = 1219
)

func ipcPath(host string) (*uint16, error) {
	return syscall.UTF16PtrFromString(`\\` + host + `\IPC$`)
}

// Connect открывает сеанс к \\host\IPC$ под указанной учётной записью.
// Пустой user — текущий вход Windows. Сеанс действует для всего процесса,
// поэтому админ-шары \\host\C$ становятся доступны обычным вызовам os.
func Connect(host, user, password string) error {
	remote, err := ipcPath(host)
	if err != nil {
		return err
	}
	nr := netResource{RemoteName: remote}

	var userPtr, passPtr *uint16
	if user != "" {
		if userPtr, err = syscall.UTF16PtrFromString(user); err != nil {
			return err
		}
		if passPtr, err = syscall.UTF16PtrFromString(password); err != nil {
			return err
		}
	}
	add := func() uint32 {
		r, _, _ := procAddConnection.Call(uintptr(unsafe.Pointer(&nr)), uintptr(unsafe.Pointer(passPtr)), uintptr(unsafe.Pointer(userPtr)), 0)
		return uint32(r)
	}
	code := add()
	if code == errSessionCredConfl {
		// К этому хосту уже есть сеанс под другой учёткой — сбрасываем и пробуем заново.
		Disconnect(host)
		code = add()
	}
	if code == 0 {
		return nil
	}
	return explain(host, code)
}

// Disconnect закрывает сеанс к хосту, открытый Connect.
func Disconnect(host string) {
	if remote, err := ipcPath(host); err == nil {
		procCancelConnection.Call(uintptr(unsafe.Pointer(remote)), 0, 1)
	}
}

func explain(host string, code uint32) error {
	switch code {
	case errAccessDenied, errBadUserOrPass, errLogonFailure:
		return errors.New("Неверное имя пользователя или пароль, либо у учётной записи нет прав администратора")
	case errAccountDisabled, errAccountRestrict:
		return errors.New("Учётная запись отключена или ей запрещён сетевой вход")
	case errBadNetPath, errNetNameNotFound, errNetworkUnreach, errHostUnreach, errNoLogonServers, errBadDevice:
		return fmt.Errorf("Компьютер %s недоступен: проверьте адрес, сеть и что на нём включён общий доступ к файлам (SMB)", host)
	}
	return fmt.Errorf("Не удалось подключиться к %s (код Windows %d)", host, code)
}

// diskSpace возвращает размер и свободное место диска; нули, если узнать не вышло.
func diskSpace(path string) (total, free int64) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0
	}
	var avail, tot, fr uint64
	r, _, _ := procDiskFreeSpaceEx.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&avail)), uintptr(unsafe.Pointer(&tot)), uintptr(unsafe.Pointer(&fr)))
	if r == 0 {
		return 0, 0
	}
	return int64(tot), int64(fr)
}
