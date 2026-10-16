// Package search ищет файлы по части имени, обходя каталоги в несколько потоков.
//
// Параллельный обход заметно ускоряет поиск на сетевых дисках, где каждое
// чтение каталога упирается в задержку сети, а не в процессор.
package search

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"file-search/internal/content"
)

// Match — найденный файл. Поля сокращены: при больших выдачах они уходят
// во фронтенд десятками тысяч.
type Match struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	Dir      string `json:"dir"`
	Ext      string `json:"ext"`
	Size     int64  `json:"size"`
	Modified int64  `json:"mod"`            // Unix-время в миллисекундах
	Snippet  string `json:"snip,omitempty"` // фрагмент текста с найденным словом
	Hits     int    `json:"hits,omitempty"` // сколько раз слово встретилось в тексте
}

// Stats — итог поиска.
type Stats struct {
	Found   int   `json:"found"`
	Dirs    int64 `json:"dirs"`
	Skipped int64 `json:"skipped"`
	Limited bool  `json:"limited"`
}

// Options настраивает поиск. Нулевые значения заменяются значениями по умолчанию.
type Options struct {
	Workers    int           // число потоков обхода
	MaxResults int           // после стольких совпадений поиск останавливается
	FlushEvery time.Duration // как часто отдавать накопленные совпадения
	FlushSize  int           // отдать пачку раньше, если накопилось столько совпадений

	// Content включает поиск по содержимому документов: слова ищутся
	// в тексте и в имени файла. Docs — кэш извлечённого текста (обязателен).
	Content bool
	Docs    *content.Cache
}

const (
	DefaultMaxResults = 100_000
	defaultFlushEvery = 80 * time.Millisecond
	defaultFlushSize  = 2_000
)

func (o *Options) normalize() {
	if o.Workers <= 0 {
		o.Workers = min(16, max(4, runtime.NumCPU()*2))
	}
	if o.MaxResults <= 0 {
		o.MaxResults = DefaultMaxResults
	}
	if o.FlushEvery <= 0 {
		o.FlushEvery = defaultFlushEvery
	}
	if o.FlushSize <= 0 {
		o.FlushSize = defaultFlushSize
	}
}

// Run ищет в root файлы по запросу: все слова запроса должны встретиться
// в имени файла (или, с Options.Content, в имени либо тексте документа).
// Регистр и разница «е»/«ё» не учитываются.
// Совпадения отдаются пачками в onBatch из одной горутины, по порядку.
// При отмене ctx возвращает уже собранную статистику и context.Canceled.
func Run(ctx context.Context, root, query string, opts Options, onBatch func([]Match)) (Stats, error) {
	opts.normalize()

	q := ParseQuery(query)
	if q.Empty() {
		return Stats{}, errors.New("введите имя файла или его часть")
	}
	if opts.Content && opts.Docs == nil {
		opts.Docs = content.NewCache(64 << 20)
	}
	info, err := os.Stat(root)
	if err != nil {
		return Stats{}, fmt.Errorf("не удалось открыть папку: %w", err)
	}
	if !info.IsDir() {
		return Stats{}, errors.New("указанный путь не является папкой")
	}

	ctx, stop := context.WithCancel(ctx)
	defer stop()

	var (
		queue   = newDirQueue(root)
		dirs    atomic.Int64
		skipped atomic.Int64
		limited atomic.Bool

		mu      sync.Mutex
		pending []Match
		found   int
		ready   = make(chan struct{}, 1)
	)

	// Обходчики: каждый берёт каталог из очереди, кладёт подкаталоги обратно,
	// а совпадения — в общий буфер.
	var workers sync.WaitGroup
	for range opts.Workers {
		workers.Go(func() {
			for {
				dir, ok := queue.pop(ctx)
				if !ok {
					return
				}
				subdirs, matches, readErr := scanDir(ctx, dir, q, opts)
				dirs.Add(1)
				if readErr != nil {
					skipped.Add(1)
				}
				queue.done(subdirs)

				if len(matches) == 0 {
					continue
				}
				mu.Lock()
				room := opts.MaxResults - found
				if len(matches) >= room {
					matches = matches[:room]
					limited.Store(true)
					stop()
				}
				found += len(matches)
				pending = append(pending, matches...)
				full := len(pending) >= opts.FlushSize
				mu.Unlock()
				if full {
					select {
					case ready <- struct{}{}:
					default:
					}
				}
			}
		})
	}

	finished := make(chan struct{})
	go func() {
		workers.Wait()
		close(finished)
	}()

	flush := func() {
		mu.Lock()
		batch := pending
		pending = nil
		mu.Unlock()
		if len(batch) > 0 {
			onBatch(batch)
		}
	}

	ticker := time.NewTicker(opts.FlushEvery)
	defer ticker.Stop()
	for running := true; running; {
		select {
		case <-ticker.C:
			flush()
		case <-ready:
			flush()
		case <-finished:
			running = false
		}
	}
	flush()

	stats := Stats{Found: found, Dirs: dirs.Load(), Skipped: skipped.Load(), Limited: limited.Load()}
	if !stats.Limited && ctx.Err() != nil {
		return stats, context.Canceled
	}
	return stats, nil
}

// scanDir читает один каталог. На Windows сведения о файле приходят вместе
// со списком каталога, поэтому entry.Info() не делает лишних запросов к диску.
func scanDir(ctx context.Context, dir string, q Query, opts Options) (subdirs []string, matches []Match, err error) {
	entries, err := os.ReadDir(dir)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			subdirs = append(subdirs, filepath.Join(dir, name))
			continue
		}
		var (
			snippet string
			hits    int
		)
		switch {
		case q.matchName(name):
			// имя подходит — текст читать не нужно
		case !opts.Content || !content.Searchable(name) || ctx.Err() != nil:
			continue
		}

		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}
		path := filepath.Join(dir, name)
		if opts.Content && content.Searchable(name) {
			doc := opts.Docs.Get(path, info.Size(), info.ModTime().UnixNano())
			ok, s, h := q.matchDoc(name, doc)
			if !ok {
				continue
			}
			snippet, hits = s, h
		}
		matches = append(matches, Match{
			Path:     path,
			Name:     name,
			Dir:      dir,
			Ext:      strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), "."),
			Size:     info.Size(),
			Modified: info.ModTime().UnixMilli(),
			Snippet:  snippet,
			Hits:     hits,
		})
	}
	return subdirs, matches, err
}

// dirQueue — очередь каталогов без ограничения размера. Обход в ширину:
// сначала находятся файлы ближе к корню, их пользователь обычно и ищет.
type dirQueue struct {
	mu     sync.Mutex
	cond   *sync.Cond
	items  []string
	head   int
	active int // сколько каталогов сейчас читается
}

func newDirQueue(root string) *dirQueue {
	q := &dirQueue{items: []string{root}}
	q.cond = sync.NewCond(&q.mu)
	return q
}

// pop возвращает следующий каталог. false — обход закончен или отменён.
func (q *dirQueue) pop(ctx context.Context) (string, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for q.head == len(q.items) && q.active > 0 && ctx.Err() == nil {
		q.cond.Wait()
	}
	if ctx.Err() != nil || q.head == len(q.items) {
		q.cond.Broadcast()
		return "", false
	}
	dir := q.items[q.head]
	q.items[q.head] = ""
	q.head++
	if q.head > 4096 && q.head*2 > len(q.items) {
		q.items = append(q.items[:0], q.items[q.head:]...)
		q.head = 0
	}
	q.active++
	return dir, true
}

// done отмечает каталог прочитанным и добавляет найденные в нём подкаталоги.
func (q *dirQueue) done(subdirs []string) {
	q.mu.Lock()
	q.items = append(q.items, subdirs...)
	q.active--
	q.mu.Unlock()
	q.cond.Broadcast()
}
