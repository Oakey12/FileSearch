package main

import (
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

	result, err := findFiles(root, "RePoRt")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matches) != 2 {
		t.Fatalf("ожидалось 2 совпадения, получено %d: %v", len(result.Matches), result.Matches)
	}
}

func TestFindFilesNoMatches(t *testing.T) {
	root := t.TempDir()
	result, err := findFiles(root, "missing")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matches) != 0 {
		t.Fatalf("ожидалось 0 совпадений, получено %d", len(result.Matches))
	}
}
