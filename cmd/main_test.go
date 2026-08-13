package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFindFiles(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "Документы")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(root, "report.txt"),
		filepath.Join(sub, "REPORT-final.pdf"),
		filepath.Join(sub, "photo.jpg"),
	} {
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	result, err := findFiles(context.Background(), root, "RePoRt")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matches) != 2 {
		t.Fatalf("ожидалось 2 совпадения, получено %d: %v", len(result.Matches), result.Matches)
	}
}

func TestFindFilesNoMatches(t *testing.T) {
	root := t.TempDir()
	result, err := findFiles(context.Background(), root, "missing")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matches) != 0 {
		t.Fatalf("ожидалось 0 совпадений, получено %d", len(result.Matches))
	}
}

func TestFindFilesCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := findFiles(ctx, t.TempDir(), "anything")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ожидалась context.Canceled, получено %v", err)
	}
	if len(result.Matches) != 0 {
		t.Fatalf("после немедленной отмены получено совпадений: %d", len(result.Matches))
	}
}

func TestResultCellText(t *testing.T) {
	match := fileMatch{Name: "report.pdf", Type: "pdf", Size: 1024, Folder: `C:\Docs`}
	if got := resultCellText([]fileMatch{match}, 0, 0); got != "Имя" {
		t.Fatalf("заголовок = %q", got)
	}
	if got := resultCellText([]fileMatch{match}, 1, 0); got != "report.pdf" {
		t.Fatalf("имя = %q", got)
	}
	if got := resultCellText([]fileMatch{match}, 1, 2); got != "1.00 КБ" {
		t.Fatalf("размер = %q", got)
	}
}
