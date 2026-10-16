package search

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func makeTree(t *testing.T, files ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, file := range files {
		path := filepath.Join(root, file)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func collect(t *testing.T, ctx context.Context, root, query string, opts Options) ([]Match, Stats, error) {
	t.Helper()
	var all []Match
	stats, err := Run(ctx, root, query, opts, func(batch []Match) { all = append(all, batch...) })
	sort.Slice(all, func(i, j int) bool { return all[i].Path < all[j].Path })
	return all, stats, err
}

func TestRunFindsCaseInsensitiveInSubfolders(t *testing.T) {
	root := makeTree(t,
		"report.txt",
		filepath.Join("Документы", "REPORT-final.pdf"),
		filepath.Join("Документы", "Глубже", "old-report.docx"),
		filepath.Join("Документы", "photo.jpg"),
	)

	matches, stats, err := collect(t, context.Background(), root, "RePoRt", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 3 || stats.Found != 3 {
		t.Fatalf("ожидалось 3 совпадения, получено %d (%+v)", len(matches), stats)
	}
	if stats.Dirs != 3 {
		t.Fatalf("ожидалось 3 просмотренных каталога, получено %d", stats.Dirs)
	}
	for _, m := range matches {
		if m.Path != filepath.Join(m.Dir, m.Name) || m.Size != 1 || m.Modified == 0 {
			t.Fatalf("неполные данные совпадения: %+v", m)
		}
	}
	if got := matches[0].Ext; got != "pdf" && got != "docx" && got != "txt" {
		t.Fatalf("расширение = %q", got)
	}
}

func TestRunCyrillicQuery(t *testing.T) {
	root := makeTree(t, "Отчёт за июль.xlsx", "отчёт.pdf", "план.docx")
	matches, _, err := collect(t, context.Background(), root, "ОТЧЁТ", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 {
		t.Fatalf("ожидалось 2 совпадения, получено %d", len(matches))
	}
}

func TestRunNoMatches(t *testing.T) {
	root := makeTree(t, "a.txt")
	matches, stats, err := collect(t, context.Background(), root, "missing", Options{})
	if err != nil || len(matches) != 0 || stats.Found != 0 {
		t.Fatalf("matches=%d stats=%+v err=%v", len(matches), stats, err)
	}
}

func TestRunCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := collect(t, ctx, makeTree(t, "a.txt"), "a", Options{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ожидалась context.Canceled, получено %v", err)
	}
}

func TestRunLimit(t *testing.T) {
	var files []string
	for i := range 50 {
		files = append(files, filepath.Join(fmt.Sprintf("d%d", i%5), fmt.Sprintf("file%d.txt", i)))
	}
	matches, stats, err := collect(t, context.Background(), makeTree(t, files...), "file", Options{MaxResults: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 10 || !stats.Limited {
		t.Fatalf("ожидалось ровно 10 совпадений и Limited, получено %d, %+v", len(matches), stats)
	}
}

func TestRunValidatesInput(t *testing.T) {
	root := makeTree(t, "a.txt")
	if _, err := Run(context.Background(), root, "  ", Options{}, func([]Match) {}); err == nil {
		t.Fatal("пустой запрос должен давать ошибку")
	}
	if _, err := Run(context.Background(), filepath.Join(root, "a.txt"), "a", Options{}, func([]Match) {}); err == nil {
		t.Fatal("файл вместо папки должен давать ошибку")
	}
}
