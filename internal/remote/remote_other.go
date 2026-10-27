//go:build !windows

package remote

import "errors"

// Connect на не-Windows не поддерживается: нужны сетевые сеансы Windows.
func Connect(host, user, password string) error {
	return errors.New("подключение к удалённому ПК работает только в Windows")
}

// Disconnect ничего не делает.
func Disconnect(host string) {}

func diskSpace(path string) (total, free int64) { return 0, 0 }
