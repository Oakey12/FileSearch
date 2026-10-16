//go:build !windows

package onec

import "errors"

func writeERT(src, dst, streamName string, data []byte) error {
	return errors.New("запись .ert доступна только в Windows")
}
