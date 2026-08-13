package main

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestDecodeDOCXDocument(t *testing.T) {
	xmlDocument := `<w:document xmlns:w="word"><w:body>` +
		`<w:p><w:r><w:t>Первая строка</w:t></w:r></w:p>` +
		`<w:tbl><w:tr><w:tc><w:p><w:r><w:t>A1</w:t></w:r></w:p></w:tc>` +
		`<w:tc><w:p><w:r><w:t>B1</w:t></w:r></w:p></w:tc></w:tr></w:tbl>` +
		`</w:body></w:document>`

	text, truncated, err := decodeDOCXDocument(bytes.NewBufferString(xmlDocument), 1024)
	if err != nil {
		t.Fatal(err)
	}
	if truncated {
		t.Fatal("короткий документ не должен быть усечён")
	}
	if text != "Первая строка\nA1\n\tB1" {
		t.Fatalf("неожиданный текст: %q", text)
	}
}

func TestExtractDOCXText(t *testing.T) {
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	file, err := archive.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(file, `<w:document xmlns:w="word"><w:p><w:r><w:t>Тест</w:t></w:r></w:p></w:document>`); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "test.docx")
	if err := os.WriteFile(path, buffer.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	text, _, err := extractDOCXText(path, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if text != "Тест" {
		t.Fatalf("получено %q", text)
	}
}

func TestExcelHelpers(t *testing.T) {
	rows := [][]string{{"A1", "B1"}, {"A2"}}
	rowCount, columnCount := excelTableSize(rows)
	if rowCount != 2 || columnCount != 2 {
		t.Fatalf("размер таблицы: %dx%d", rowCount, columnCount)
	}
	if got := excelCell(rows, 1, 1); got != "" {
		t.Fatalf("пустая ячейка содержит %q", got)
	}
}
