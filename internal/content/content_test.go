package content

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

func TestDecodeDOCX(t *testing.T) {
	xmlDocument := `<w:document xmlns:w="word"><w:body>` +
		`<w:p><w:r><w:t>Первая строка</w:t></w:r></w:p>` +
		`<w:tbl><w:tr><w:tc><w:p><w:r><w:t>A1</w:t></w:r></w:p></w:tc>` +
		`<w:tc><w:p><w:r><w:t>B1</w:t></w:r></w:p></w:tc></w:tr></w:tbl>` +
		`</w:body></w:document>`

	text, truncated, err := decodeDOCX(bytes.NewBufferString(xmlDocument), 1024)
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

func writeZip(t *testing.T, path string, parts map[string]string) {
	t.Helper()
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	for name, body := range parts {
		file, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(file, body); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buffer.Bytes(), 0o600); err != nil {
		t.Fatal(err)
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
		text, encoding, ok := DecodeText(c.data)
		if !ok || text != c.want || encoding != c.encoding {
			t.Errorf("%s: получено %q/%q/%v, ожидалось %q/%q", c.name, text, encoding, ok, c.want, c.encoding)
		}
	}
	if _, _, ok := DecodeText([]byte{'M', 'Z', 0, 0, 3}); ok {
		t.Error("двоичные данные должны распознаваться как не текст")
	}
}

func TestExtractFormats(t *testing.T) {
	dir := t.TempDir()

	docx := filepath.Join(dir, "a.docx")
	writeZip(t, docx, map[string]string{"word/document.xml": `<w:document xmlns:w="word"><w:p><w:r><w:t>Договор поставки</w:t></w:r></w:p></w:document>`})

	pptx := filepath.Join(dir, "a.pptx")
	writeZip(t, pptx, map[string]string{"ppt/slides/slide1.xml": `<p:sld xmlns:a="a" xmlns:p="p"><a:t>План продаж</a:t></p:sld>`})

	book := excelize.NewFile()
	book.SetCellValue("Sheet1", "A1", "Счёт Билайн")
	book.SetCellValue("Sheet1", "B2", 12500)
	xlsx := filepath.Join(dir, "a.xlsx")
	if err := book.SaveAs(xlsx); err != nil {
		t.Fatal(err)
	}

	txt := filepath.Join(dir, "a.txt")
	cp1251, _ := charmap.Windows1251.NewEncoder().String("Накладная номер 7")
	if err := os.WriteFile(txt, []byte(cp1251), 0o600); err != nil {
		t.Fatal(err)
	}

	cases := map[string][]string{
		docx: {"Договор поставки"},
		pptx: {"План продаж"},
		xlsx: {"Счёт Билайн", "12500"},
		txt:  {"Накладная номер 7"},
	}
	for path, wants := range cases {
		text, err := Extract(path, MaxText)
		if err != nil {
			t.Fatalf("%s: %v", filepath.Base(path), err)
		}
		for _, want := range wants {
			if !bytes.Contains([]byte(text), []byte(want)) {
				t.Errorf("%s: в тексте %q нет %q", filepath.Base(path), text, want)
			}
		}
	}
}

func TestExtractPDF(t *testing.T) {
	text, err := Extract(filepath.Join("testdata", "invoice.pdf"), MaxText)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Счёт на оплату", "Sennheiser", "12 500"} {
		if !bytes.Contains([]byte(text), []byte(want)) {
			t.Errorf("в тексте PDF нет %q: %q", want, text)
		}
	}
}

func TestCacheReusesAndInvalidates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.txt")
	os.WriteFile(path, []byte("первый"), 0o600)
	cache := NewCache(1 << 20)
	doc := cache.Get(path, 12, 1)
	if doc.Norm != "первый" {
		t.Fatalf("norm = %q", doc.Norm)
	}
	os.WriteFile(path, []byte("второй"), 0o600)
	if again := cache.Get(path, 12, 1); again != doc {
		t.Fatal("при тех же размере и времени должен вернуться документ из кэша")
	}
	if fresh := cache.Get(path, 12, 2); fresh.Norm != "второй" {
		t.Fatalf("после изменения файла ожидался новый текст, получено %q", fresh.Norm)
	}
}

func TestNormalize(t *testing.T) {
	if got := Normalize("СЧЁТ Ёлка"); got != "счет елка" {
		t.Fatalf("Normalize = %q", got)
	}
}
