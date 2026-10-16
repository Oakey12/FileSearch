package onec

import (
	"errors"
	"os"
	"sync"
	"time"
)

// fileSource — файл на диске, из которого данные читаются по мере надобности,
// а не целиком в память: у конфигураций .cf бывает под гигабайт.
//
// Дескриптор держится открытым только во время операции (разбор, чтение
// модуля, сохранение) и закрывается в release: в Windows открытый файл
// нельзя заменить, а сохранение «на месте» переименовывает новый файл поверх.
type fileSource struct {
	path string
	size int64
	mod  time.Time

	mu sync.Mutex
	f  *os.File
}

var errFileChanged = errors.New("файл изменился на диске — откройте его заново")

func newFileSource(path string) (*fileSource, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	return &fileSource{path: path, size: info.Size(), mod: info.ModTime()}, nil
}

// ReadAt можно вызывать из нескольких горутин (os.File.ReadAt это допускает);
// замок нужен только на открытие.
func (s *fileSource) ReadAt(p []byte, off int64) (int, error) {
	f, err := s.file()
	if err != nil {
		return 0, err
	}
	return f.ReadAt(p, off)
}

func (s *fileSource) file() (*os.File, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f == nil {
		f, err := os.Open(s.path)
		if err != nil {
			return nil, err
		}
		// Адреса элементов взяты из файла при разборе: если его подменили,
		// читать по ним нельзя.
		info, err := f.Stat()
		if err != nil || info.Size() != s.size || !info.ModTime().Equal(s.mod) {
			f.Close()
			return nil, errFileChanged
		}
		s.f = f
	}
	return s.f, nil
}

// release закрывает файл; следующее чтение откроет его снова.
func (s *fileSource) release() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f != nil {
		s.f.Close()
		s.f = nil
	}
}
