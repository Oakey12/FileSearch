// Package settings хранит недавние папки поиска в профиле пользователя.
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const maxRecent = 8

// Recent — список недавних папок, последние сверху.
type Recent struct {
	mu    sync.Mutex
	path  string
	items []string
}

type fileFormat struct {
	Recent []string `json:"recent"`
}

// OpenRecent читает список из %APPDATA%\FileSearch\settings.json.
// Если файла нет или он повреждён, список просто пустой.
func OpenRecent() *Recent {
	r := &Recent{}
	if dir, err := os.UserConfigDir(); err == nil {
		r.path = filepath.Join(dir, "FileSearch", "settings.json")
	}
	return r.load()
}

func (r *Recent) load() *Recent {
	if r.path == "" {
		return r
	}
	data, err := os.ReadFile(r.path)
	if err != nil {
		return r
	}
	var stored fileFormat
	if json.Unmarshal(data, &stored) == nil {
		r.items = stored.Recent
	}
	return r
}

// List возвращает копию списка.
func (r *Recent) List() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string{}, r.items...)
}

// Add поднимает папку в начало списка.
func (r *Recent) Add(dir string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := []string{dir}
	for _, item := range r.items {
		if !strings.EqualFold(item, dir) && len(items) < maxRecent {
			items = append(items, item)
		}
	}
	r.items = items
	r.save()
}

// Remove убирает папку из списка.
func (r *Recent) Remove(dir string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := r.items[:0]
	for _, item := range r.items {
		if !strings.EqualFold(item, dir) {
			items = append(items, item)
		}
	}
	r.items = items
	r.save()
}

func (r *Recent) save() {
	if r.path == "" {
		return
	}
	data, err := json.MarshalIndent(fileFormat{Recent: r.items}, "", "  ")
	if err != nil {
		return
	}
	if os.MkdirAll(filepath.Dir(r.path), 0o755) != nil {
		return
	}
	tmp := r.path + ".tmp"
	if os.WriteFile(tmp, data, 0o644) == nil {
		_ = os.Rename(tmp, r.path)
	}
}
