//go:build !windows

package onec

import "errors"

func writeOLE(src, dst string, streams map[string][]byte) error {
	return errors.New("запись файлов 1С 7.7 доступна только в Windows")
}
