package settings

import (
	"errors"
	"os"
	"path/filepath"
)

// Документы интерфейса (заметки, холст) хранятся как есть — JSON, который
// формирует интерфейс. Go только надёжно сохраняет и отдаёт его.
var docNames = map[string]string{
	"notes": "notes.json",
	"board": "board.json",
}

const maxDocSize = 32 << 20

func docPath(name string) (string, error) {
	file, ok := docNames[name]
	if !ok {
		return "", errors.New("неизвестный документ")
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "FileSearch", file), nil
}

// ReadDoc возвращает сохранённый документ или пустую строку, если его ещё нет.
func ReadDoc(name string) (string, error) {
	path, err := docPath(name)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return string(data), err
}

// WriteDoc атомарно сохраняет документ: сначала во временный файл, потом
// переименование, чтобы сбой посреди записи не испортил заметки.
func WriteDoc(name, data string) error {
	if len(data) > maxDocSize {
		return errors.New("заметки слишком большие для сохранения")
	}
	path, err := docPath(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(data), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
