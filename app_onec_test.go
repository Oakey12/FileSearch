package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func copyTestdata(t *testing.T, name string) string {
	t.Helper()
	dst := filepath.Join(t.TempDir(), name)
	if err := copyFile(filepath.Join("internal", "onec", "testdata", name), dst); err != nil {
		t.Fatal(err)
	}
	return dst
}

func TestOneCCacheReusesAndRefreshes(t *testing.T) {
	path := copyTestdata(t, "sample77.ert")
	var c onecCache
	a, err := c.open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := c.open(context.Background(), path)
	if a != b {
		t.Fatal("тот же неизменённый файл должен браться из кеша")
	}

	// Файл изменили на диске — разбор устарел.
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	if d, _ := c.open(context.Background(), path); d == a {
		t.Fatal("после изменения файла нужен новый разбор")
	}

	e, _ := c.open(context.Background(), path)
	c.forget(path)
	if f, _ := c.open(context.Background(), path); f == e {
		t.Fatal("после forget нужен новый разбор")
	}
}

func TestOneCCacheCancelled(t *testing.T) {
	path := copyTestdata(t, "sample77.ert")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var c onecCache
	// Маленький файл разбирается без пауз, поэтому отмена может и не успеть;
	// важно, что отменённый разбор не остаётся в кеше испорченным.
	if f, err := c.open(ctx, path); err == nil && f == nil {
		t.Fatal("nil без ошибки")
	}
	if f, err := c.open(context.Background(), path); err != nil || f == nil {
		t.Fatalf("после отмены файл должен открываться: %v", err)
	}
}

func TestPruneBackupsKeepsNewestAndCurrent(t *testing.T) {
	dir := t.TempDir()
	base := time.Now().Add(-time.Hour)
	var newest string
	for i := 0; i < maxBackups+5; i++ {
		p := filepath.Join(dir, "копия "+strconv.Itoa(i)+".epf")
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		mod := base.Add(time.Duration(i) * time.Second)
		os.Chtimes(p, mod, mod)
		newest = p
	}
	// Только что сделанная копия «старше» всех — она всё равно остаётся.
	keep := filepath.Join(dir, "копия 0.epf")
	pruneBackups(dir, keep)
	entries, _ := os.ReadDir(dir)
	if len(entries) != maxBackups+1 {
		t.Fatalf("осталось %d копий, ожидалось %d", len(entries), maxBackups+1)
	}
	for _, p := range []string{keep, newest} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("удалена копия, которую надо оставить: %s", filepath.Base(p))
		}
	}
}
