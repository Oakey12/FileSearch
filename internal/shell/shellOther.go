//go:build !windows

package shell

import "errors"

// ErrCancelled возвращается, если пользователь закрыл окно выбора папки.
var ErrCancelled = errors.New("выбор папки отменён")

var errUnsupported = errors.New("функция доступна только в Windows")

func PickFolder(owner uintptr, title, initialDir string) (string, error) {
	return "", errUnsupported
}

func ShowInExplorer(path string) (bool, error) {
	return false, errUnsupported
}