package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"file-search/internal/onec"
	"file-search/internal/preview"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// onecCache — последний открытый файл 1С. Его разбор у большой конфигурации
// занимает до секунды, поэтому предпросмотр и декодер пользуются одним разбором.
//
// mu охраняет поля и всю работу с файлом (onec.File не для нескольких горутин).
// Сам разбор идёт без замка — он долгий, а файл в это время может понадобиться
// декодеру.
type onecCache struct {
	mu   sync.Mutex
	path string
	size int64
	mod  time.Time
	file *onec.File

	previewMu     sync.Mutex
	cancelPreview context.CancelFunc
}

func fileStamp(path string) (int64, time.Time, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, time.Time{}, err
	}
	return info.Size(), info.ModTime(), nil
}

// open возвращает разбор файла из кеша или разбирает его заново.
func (c *onecCache) open(ctx context.Context, path string) (*onec.File, error) {
	size, mod, err := fileStamp(path)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	if c.file != nil && c.path == path && c.size == size && c.mod.Equal(mod) {
		f := c.file
		c.mu.Unlock()
		return f, nil
	}
	c.mu.Unlock()

	f, err := onec.OpenContext(ctx, path)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.path, c.size, c.mod, c.file = path, size, mod, f // держим только последний файл
	c.mu.Unlock()
	return f, nil
}

// use выполняет fn с файлом под замком.
func (c *onecCache) use(ctx context.Context, path string, fn func(f *onec.File) error) error {
	f, err := c.open(ctx, path)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return fn(f)
}

// forget выбрасывает разбор файла: после сохранения он устарел.
func (c *onecCache) forget(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.path == path {
		c.path, c.file = "", nil
	}
}

// previewOneC — предпросмотр файла 1С. Новый предпросмотр прерывает разбор
// предыдущего: пользователь листает список, и ждать старый файл незачем.
func (a *App) previewOneC(path string) (preview.Result, error) {
	ctx, cancel := context.WithCancel(context.Background())
	a.onec.previewMu.Lock()
	if a.onec.cancelPreview != nil {
		a.onec.cancelPreview()
	}
	a.onec.cancelPreview = cancel
	a.onec.previewMu.Unlock()
	defer cancel()

	var result preview.Result
	err := a.onec.use(ctx, path, func(f *onec.File) error {
		var err error
		result, err = preview.OneC(path, f, nil)
		return err
	})
	if errors.Is(err, context.Canceled) {
		return preview.Result{}, err // ответ уже никому не нужен
	}
	if err != nil {
		return preview.OneC(path, nil, err)
	}
	return result, nil
}

var onecFilters = []runtime.FileFilter{
	{DisplayName: "Файлы 1С (*.epf, *.erf, *.cf, *.cfe, *.ert, *.md)", Pattern: "*.epf;*.erf;*.cf;*.cfe;*.ert;*.md"},
	{DisplayName: "1С:Предприятие 8 (*.epf, *.erf, *.cf, *.cfe)", Pattern: "*.epf;*.erf;*.cf;*.cfe"},
	{DisplayName: "1С:Предприятие 7.7 (*.ert, *.md)", Pattern: "*.ert;*.md"},
}

// PickOneCFile открывает стандартное окно выбора файла 1С.
func (a *App) PickOneCFile() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "Открыть файл 1С", Filters: onecFilters})
}

// OpenOneC разбирает файл 1С (.epf, .erf, .cf, .cfe — 1С 8; .ert, .md — 1С 7.7)
// и возвращает список модулей и форм. Тексты — через OneCSectionText.
func (a *App) OpenOneC(path string) (onec.Document, error) {
	var doc onec.Document
	err := a.onec.use(context.Background(), path, func(f *onec.File) error {
		doc = f.Outline()
		return nil
	})
	return doc, err
}

// OneCSectionText — текст модуля или раздела файла 1С.
func (a *App) OneCSectionText(path, id string) (onec.SectionContent, error) {
	var c onec.SectionContent
	err := a.onec.use(context.Background(), path, func(f *onec.File) error {
		var err error
		c, err = f.Content(id)
		return err
	})
	return c, err
}

// SaveOneC («Сохранить как…») записывает правки в новый файл того же формата
// (.epf → .epf, .ert → .ert) и возвращает его путь. Пустая строка — окно закрыли.
func (a *App) SaveOneC(path string, edits map[string]string) (string, error) {
	ext := filepath.Ext(path)
	target, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:            "Сохранить как",
		DefaultDirectory: filepath.Dir(path),
		DefaultFilename:  strings.TrimSuffix(filepath.Base(path), ext) + " (изменённый)" + ext,
		Filters:          []runtime.FileFilter{{DisplayName: "Тот же формат (*" + ext + ")", Pattern: "*" + ext}},
	})
	if err != nil || target == "" {
		return "", err
	}
	if !strings.EqualFold(filepath.Ext(target), ext) {
		target += ext // формат файла не меняется
	}
	if strings.EqualFold(filepath.Clean(target), filepath.Clean(path)) {
		return a.SaveOneCInPlace(path, edits)
	}
	return target, a.saveOneC(path, target, edits)
}

// SaveOneCInPlace («Сохранить») записывает правки в исходный файл. Перед этим
// исходник копируется в %APPDATA%\FileSearch\Резервные копии. Возвращает путь копии.
func (a *App) SaveOneCInPlace(path string, edits map[string]string) (string, error) {
	backup, err := backupFile(path)
	if err != nil {
		return "", fmt.Errorf("не удалось сделать резервную копию, файл не изменён: %w", err)
	}
	if err := a.saveOneC(path, path, edits); err != nil {
		return "", err
	}
	return backup, nil
}

func (a *App) saveOneC(src, dst string, edits map[string]string) error {
	// Правки накладываются на разбор файла с диска (из кеша, если файл не менялся).
	// После сохранения разбор устарел: в нём правки, а на диске — новый файл.
	defer a.onec.forget(src)
	err := a.onec.use(context.Background(), src, func(f *onec.File) error {
		return f.SaveTo(dst, edits)
	})
	if errors.Is(err, os.ErrPermission) {
		return errors.New("файл занят другой программой (например, открыт в 1С) или нет прав на запись")
	}
	return err
}

const (
	maxBackups     = 50
	maxBackupBytes = 4 << 30 // копии конфигураций бывают по гигабайту
)

// backupFile копирует файл в папку резервных копий (потоком — файл может быть
// огромным) и оставляет не больше maxBackups копий общим размером до maxBackupBytes.
func backupFile(path string) (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "FileSearch", "Резервные копии")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	ext := filepath.Ext(path)
	name := strings.TrimSuffix(filepath.Base(path), ext) + " " + time.Now().Format("2006-01-02 15-04-05") + ext
	target := filepath.Join(dir, name)
	if err := copyFile(path, target); err != nil {
		os.Remove(target)
		return "", err
	}
	pruneBackups(dir, target)
	return target, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// pruneBackups удаляет старые копии сверх лимитов; только что сделанную — никогда.
func pruneBackups(dir, keep string) {
	entries, _ := os.ReadDir(dir)
	type backup struct {
		path string
		size int64
		mod  time.Time
	}
	var list []backup
	for _, e := range entries {
		if info, err := e.Info(); err == nil && !info.IsDir() {
			list = append(list, backup{filepath.Join(dir, e.Name()), info.Size(), info.ModTime()})
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].mod.After(list[j].mod) }) // новые первыми
	var total int64
	for i, b := range list {
		total += b.size
		if b.path != keep && (i >= maxBackups || total > maxBackupBytes) {
			_ = os.Remove(b.path)
		}
	}
}
