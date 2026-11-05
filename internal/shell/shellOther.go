//go:build !windows

package shell

import "errors"

var errUnsupported = errors.New("функция доступна только в Windows")

func ShowInExplorer(path string) (bool, error) {
	return false, errUnsupported
}

func OpenFile(path string) error {
	return errUnsupported
}
