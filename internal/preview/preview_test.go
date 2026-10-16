package preview

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
	"golang.org/x/text/encoding/charmap"
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

func TestDecodeText(t *testing.T) {
	cp1251, _ := charmap.Windows1251.NewEncoder().String("Привет, мир")
	cases := []struct {
		name, encoding, want string
		data                 []byte
	}{
		{"utf8", "UTF-8", "Привет", []byte("Привет")},
		{"utf8 bom", "UTF-8", "Привет", append([]byte{0xEF, 0xBB, 0xBF}, "Привет"...)},
		{"utf16le bom", "UTF-16", "Да", []byte{0xFF, 0xFE, 0x14, 0x04, 0x30, 0x04}},
		{"cp1251", "Windows-1251", "Привет, мир", []byte(cp1251)},
		// Обрезка по лимиту посреди двухбайтного символа не должна ломать UTF-8.
		{"cut rune", "UTF-8", "При", []byte("Прив")[:7]},
	}
	for _, c := range cases {
		text, encoding, ok := decodeText(c.data)
		if !ok || text != c.want || encoding != c.encoding {
			t.Errorf("%s: получено %q/%q/%v, ожидалось %q/%q", c.name, text, encoding, ok, c.want, c.encoding)
		}
	}
	if _, _, ok := decodeText([]byte{'M', 'Z', 0, 0, 3}); ok {
		t.Error("двоичные данные должны распознаваться как не текст")
	}
}

func TestLoadKinds(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, data []byte) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}

	book := excelize.NewFile()
	book.SetCellValue("Sheet1", "A1", "Имя")
	book.SetCellValue("Sheet1", "B2", 42)
	xlsx := filepath.Join(dir, "book.xlsx")
	if err := book.SaveAs(xlsx); err != nil {
		t.Fatal(err)
	}

	cases := map[string]string{
		write("notes.txt", []byte("hello")): KindText,
		write("photo.PNG", []byte{0x89}):    KindImage,
		write("data.bin", []byte{1, 2, 3}):  KindInfo,
		write("old.doc", []byte{0}):         KindInfo,
		xlsx:                                KindSheet,
	}
	for path, want := range cases {
		result, err := Load(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if result.Kind != want {
			t.Errorf("%s: kind = %q, ожидалось %q", filepath.Base(path), result.Kind, want)
		}
	}

	result, _ := Load(xlsx)
	if got := result.Sheets[0].Rows; len(got) != 2 || got[0][0] != "Имя" || got[1][1] != "42" {
		t.Errorf("строки листа: %q", got)
	}
}
