//go:build !windows

// Package catwin показывает рыжего кота; на этой платформе не поддерживается.
package catwin

import "errors"

// Hooks — что делает окно кота для вызывающего.
type Hooks struct {
	Ready func(quit func())
	Open  func()
}

// Run на не-Windows не работает: окно кота построено на Win32.
func Run(Hooks) error {
	return errors.New("окно кота работает только в Windows")
}
