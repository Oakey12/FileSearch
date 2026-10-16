package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"file-search/internal/content"
	"file-search/internal/preview"
	"file-search/internal/search"
	"file-search/internal/settings"
	"file-search/internal/shell"
	"file-search/internal/update"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// updateCheckInterval — как часто в фоне проверять, не вышла ли новая версия.
const updateCheckInterval = 15 * time.Minute

// Префикс адресов картинок и PDF для предпросмотра. Файлы отдаются только
// по выданному в Preview токену, а не по произвольному пути.
const filePrefix = "/preview-file/"

// docsCacheSize — сколько памяти держать под извлечённый текст документов.
// Повторный поиск по содержимому той же папки идёт из памяти.
const docsCacheSize = 256 << 20

// App — методы, которые вызывает интерфейс.
type App struct {
	ctx    context.Context
	recent *settings.Recent
	docs   *content.Cache

	searchMu     sync.Mutex
	cancelSearch context.CancelFunc

	filesMu sync.Mutex
	files   map[string]string // токен → путь
	fileIDs []string          // порядок выдачи, чтобы держать не больше maxFiles

	updateMu sync.Mutex
	release  *update.Release

	onec onecCache // последний открытый файл 1С — общий для декодера и предпросмотра
}

const maxFiles = 32

func NewApp() *App {
	return &App{files: map[string]string{}, docs: content.NewCache(docsCacheSize)}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.recent = settings.OpenRecent()
	update.CleanupOld()
	go a.watchUpdates()
}

// Version возвращает текущую версию программы.
func (a *App) Version() string {
	return update.CurrentVersion
}

// --- Папки -------------------------------------------------------------

// PickFolder открывает стандартное окно Windows «Выбор папки».
// Пустая строка — пользователь закрыл окно.
func (a *App) PickFolder(initial string) (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:            "Папка для поиска",
		DefaultDirectory: initial,
	})
}

// RecentFolders возвращает недавние папки поиска.
func (a *App) RecentFolders() []string {
	return a.recent.List()
}

// ForgetFolder убирает папку из недавних.
func (a *App) ForgetFolder(dir string) []string {
	a.recent.Remove(dir)
	return a.recent.List()
}

// --- Поиск -------------------------------------------------------------

// SearchBatch — очередная пачка совпадений поиска id.
type SearchBatch struct {
	ID    int            `json:"id"`
	Items []search.Match `json:"items"`
}

// SearchDone — итог поиска id.
type SearchDone struct {
	ID       int          `json:"id"`
	Stats    search.Stats `json:"stats"`
	Millis   int64        `json:"ms"`
	Canceled bool         `json:"canceled"`
	Error    string       `json:"error,omitempty"`
}

// Search запускает поиск и сразу возвращается. Результаты приходят событиями
// "search:batch" и "search:done". Новый поиск отменяет предыдущий;
// по id интерфейс отбрасывает опоздавшие пачки.
// inContent — искать слова и в тексте документов (txt, docx, xlsx, pptx, pdf).
func (a *App) Search(id int, root, query string, inContent bool) {
	root = strings.Trim(strings.TrimSpace(root), `"`)
	if root == "" {
		runtime.EventsEmit(a.ctx, "search:done", SearchDone{ID: id, Error: "Выберите папку для поиска"})
		return
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}

	ctx, cancel := context.WithCancel(a.ctx)
	a.searchMu.Lock()
	if a.cancelSearch != nil {
		a.cancelSearch()
	}
	a.cancelSearch = cancel
	a.searchMu.Unlock()

	go func() {
		defer cancel()
		started := time.Now()
		opts := search.Options{Content: inContent, Docs: a.docs}
		stats, err := search.Run(ctx, root, query, opts, func(items []search.Match) {
			runtime.EventsEmit(a.ctx, "search:batch", SearchBatch{ID: id, Items: items})
		})

		done := SearchDone{ID: id, Stats: stats, Millis: time.Since(started).Milliseconds()}
		switch {
		case errors.Is(err, context.Canceled):
			done.Canceled = true
		case err != nil:
			done.Error = err.Error()
		default:
			a.recent.Add(root)
		}
		runtime.EventsEmit(a.ctx, "search:done", done)
	}()
}

// CancelSearch останавливает текущий поиск. Найденное остаётся в списке.
func (a *App) CancelSearch() {
	a.searchMu.Lock()
	defer a.searchMu.Unlock()
	if a.cancelSearch != nil {
		a.cancelSearch()
		a.cancelSearch = nil
	}
}

// --- Действия с файлом -------------------------------------------------

// ShowInExplorer открывает Проводник с выделенным файлом.
func (a *App) ShowInExplorer(path string) error {
	_, err := shell.ShowInExplorer(path)
	return err
}

// OpenFile открывает файл программой по умолчанию.
func (a *App) OpenFile(path string) error {
	return shell.OpenFile(path)
}

// Preview готовит предпросмотр файла.
func (a *App) Preview(path string) (preview.Result, error) {
	if preview.IsOneC(path) {
		return a.previewOneC(path)
	}
	result, err := preview.Load(path)
	if err != nil {
		return result, err
	}
	if preview.Servable(path) {
		result.URL = filePrefix + a.allowFile(path) + "/" + url.PathEscape(result.Name)
	}
	return result, nil
}

func (a *App) allowFile(path string) string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	token := hex.EncodeToString(buf)

	a.filesMu.Lock()
	defer a.filesMu.Unlock()
	a.files[token] = path
	a.fileIDs = append(a.fileIDs, token)
	if len(a.fileIDs) > maxFiles {
		delete(a.files, a.fileIDs[0])
		a.fileIDs = a.fileIDs[1:]
	}
	return token
}

// fileMiddleware отдаёт картинки и PDF для предпросмотра по токену из Preview.
// Адрес: /preview-file/<токен>/<имя файла> — имя нужно только просмотрщику PDF.
// Middleware, а не Handler: так запросы не перехватывает сервер разработки Vite.
func (a *App) fileMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rest, ok := strings.CutPrefix(r.URL.Path, filePrefix)
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		token, _, _ := strings.Cut(rest, "/")
		a.filesMu.Lock()
		path, ok := a.files[token]
		a.filesMu.Unlock()
		if !ok || !preview.Servable(path) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFile(w, r, path)
	})
}

// --- Декодер 1С ----------------------------------------------------------

// SaveOneCText сохраняет текст модуля в файл .bsl/.txt (UTF-8 с BOM — так его понимает 1С).
func (a *App) SaveOneCText(suggestedName, text string) (string, error) {
	target, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Сохранить модуль в файл",
		DefaultFilename: suggestedName,
		Filters: []runtime.FileFilter{
			{DisplayName: "Модуль 1С (*.bsl)", Pattern: "*.bsl"},
			{DisplayName: "Текст (*.txt)", Pattern: "*.txt"},
		},
	})
	if err != nil || target == "" {
		return "", err
	}
	data := "\uFEFF" + strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\n", "\r\n")
	return target, os.WriteFile(target, []byte(data), 0o644)
}

// --- Заметки -----------------------------------------------------------

// LoadNotes возвращает сохранённые заметки ("notes") или холст ("board") —
// JSON, который интерфейс сам формирует. Пустая строка — сохранений ещё нет.
func (a *App) LoadNotes(kind string) (string, error) {
	return settings.ReadDoc(kind)
}

// SaveNotes сохраняет заметки или холст в %APPDATA%\FileSearch.
func (a *App) SaveNotes(kind, data string) error {
	return settings.WriteDoc(kind, data)
}

// --- Обновления --------------------------------------------------------

// UpdateInfo — результат проверки обновлений.
type UpdateInfo struct {
	Available bool   `json:"available"`
	Current   string `json:"current"`
	Version   string `json:"version,omitempty"`
	Notes     string `json:"notes,omitempty"`
}

// CheckUpdate проверяет latest.json на сервере Nginx.
func (a *App) CheckUpdate() (UpdateInfo, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()

	info := UpdateInfo{Current: update.CurrentVersion}
	release, newer, err := update.CheckLatest(ctx)
	switch {
	case errors.Is(err, update.ErrNoReleases):
		return info, nil
	case errors.Is(err, update.ErrUnavailable):
		return info, errors.New("Сервер обновлений недоступен. Проверьте подключение к сети.")
	case err != nil:
		return info, err
	}

	a.updateMu.Lock()
	if newer {
		a.release = &release
	} else {
		a.release = nil
	}
	a.updateMu.Unlock()

	info.Available = newer
	info.Version = release.Version
	info.Notes = release.Notes
	return info, nil
}

// InstallUpdate скачивает найденную CheckUpdate версию, ставит её вместо
// текущего exe и перезапускает программу. Ход скачивания приходит событием
// "update:progress" со значением от 0 до 1.
func (a *App) InstallUpdate() error {
	a.updateMu.Lock()
	release := a.release
	a.updateMu.Unlock()
	if release == nil {
		return errors.New("сначала проверьте наличие обновления")
	}

	path, err := update.Download(a.ctx, *release, func(done, total int64) {
		if total > 0 {
			runtime.EventsEmit(a.ctx, "update:progress", float64(done)/float64(total))
		}
	})
	if err != nil {
		return err
	}
	if err := update.Apply(path); err != nil {
		return err
	}
	// Новая версия уже запущена — закрываем старую.
	runtime.Quit(a.ctx)
	return nil
}

// watchUpdates при запуске и затем каждые updateCheckInterval тихо проверяет
// сервер и сообщает интерфейсу событием "update:available".
// Если сервер недоступен, интерфейс ничего не получает и кружок не меняется.
func (a *App) watchUpdates() {
	for {
		if info, err := a.CheckUpdate(); err == nil {
			runtime.EventsEmit(a.ctx, "update:available", info)
		}
		select {
		case <-a.ctx.Done():
			return
		case <-time.After(updateCheckInterval):
		}
	}
}

// SaveWindowTheme запоминает тему для следующего запуска (см. savedTheme).
func (a *App) SaveWindowTheme(theme string) error {
	path := themeFile()
	if path == "" || (theme != "light" && theme != "dark") {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(theme), 0o644)
}
