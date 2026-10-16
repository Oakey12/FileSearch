package search

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"file-search/internal/content"
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

func TestRunMultipleWordsAndYo(t *testing.T) {
	root := makeTree(t, "Отчёт за июль.txt", "отчет за август.txt", "июль.txt")
	matches, _, err := collect(t, context.Background(), root, "июль отчет", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].Name != "Отчёт за июль.txt" {
		t.Fatalf("ожидался только «Отчёт за июль.txt», получено %+v", matches)
	}
}

func TestRunContent(t *testing.T) {
	root := t.TempDir()
	write := func(name, text string) {
		if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.txt", "Поставщик: ООО Билайн.\nСчёт на оплату № 1834, итого 12 500 руб.")
	write("b.txt", "Счёт без поставщика")
	write("счет-шаблон.txt", "пусто")
	write("photo.jpg", "счет оплату") // не текстовый формат — содержимое не читается

	// Имя файла не содержит слов — в обычном режиме ничего.
	if matches, _, _ := collect(t, context.Background(), root, "счет оплату", Options{}); len(matches) != 0 {
		t.Fatalf("поиск по имени не должен смотреть в содержимое: %+v", matches)
	}

	docs := content.NewCache(1 << 20)
	matches, _, err := collect(t, context.Background(), root, `счет "на оплату"`, Options{Content: true, Docs: docs})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].Name != "a.txt" {
		t.Fatalf("ожидался только a.txt, получено %+v", matches)
	}
	m := matches[0]
	if m.Hits != 1 || !strings.Contains(m.Snippet, "Счёт на оплату") || strings.Contains(m.Snippet, "\n") {
		t.Fatalf("фрагмент/число вхождений: %q, %d", m.Snippet, m.Hits)
	}

	// Слово только в имени файла тоже подходит в режиме содержимого.
	matches, _, _ = collect(t, context.Background(), root, "шаблон", Options{Content: true, Docs: docs})
	if len(matches) != 1 || matches[0].Name != "счет-шаблон.txt" {
		t.Fatalf("ожидалось совпадение по имени, получено %+v", matches)
	}
}

func TestParseQuery(t *testing.T) {
	got := ParseQuery(`  Отчёт   "За Июль"  2026 `).Terms
	want := []string{"отчет", "за июль", "2026"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("ParseQuery = %q", got)
	}
}
