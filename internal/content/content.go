// Package content извлекает текст из документов для поиска по содержимому.
package content

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"file-search/internal/onec"

	"github.com/ledongthuc/pdf"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
)

// MaxText — сколько текста брать из одного документа (байт UTF-8).
const MaxText = 8 << 20

// maxFileSize — файлы крупнее не читаются при поиске: это почти всегда
// архивы, сканы или выгрузки, где поиск по тексту бессмыслен и долог.
const maxFileSize = 200 << 20

var textExtensions = map[string]bool{
	".txt": true, ".log": true, ".md": true, ".json": true, ".xml": true, ".csv": true, ".tsv": true,
	".go": true, ".js": true, ".ts": true, ".html": true, ".htm": true, ".css": true, ".sql": true,
	".yaml": true, ".yml": true, ".ini": true, ".conf": true, ".cfg": true, ".toml": true,
	".bat": true, ".cmd": true, ".ps1": true, ".py": true, ".cs": true, ".java": true,
	".c": true, ".h": true, ".cpp": true, ".php": true, ".sh": true,
}

// IsText сообщает, что файл — обычный текст (по расширению).
func IsText(path string) bool {
	return textExtensions[strings.ToLower(filepath.Ext(path))]
}

// Searchable сообщает, умеет ли пакет доставать текст из файла.
func Searchable(path string) bool {
	switch ext := strings.ToLower(filepath.Ext(path)); ext {
	case ".docx", ".xlsx", ".xlsm", ".pptx", ".pdf", ".epf", ".erf", ".ert":
		return true
	default:
		return textExtensions[ext]
	}
}

// Extract возвращает текст документа, не больше limit байт.
func Extract(path string, limit int) (text string, err error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.Size() > maxFileSize {
		return "", fmt.Errorf("файл больше %d МБ", maxFileSize>>20)
	}

	switch ext := strings.ToLower(filepath.Ext(path)); ext {
	case ".docx":
		text, _, err = DOCXText(path, limit)
	case ".xlsx", ".xlsm":
		text, err = zipText(path, limit, isSheetPart, sheetText)
	case ".pptx":
		text, err = zipText(path, limit, isSlidePart, xmlElementText("t"))
	case ".pdf":
		text, err = pdfText(path, limit)
	case ".epf", ".erf", ".ert":
		// Внешние обработки и отчёты 1С — текст всех модулей.
		if text, err = onec.ExtractText(path); err == nil && len(text) > limit {
			text = string(trimPartialRune([]byte(text[:limit])))
		}
	default:
		if !textExtensions[ext] {
			return "", errors.New("формат не поддерживается")
		}
		var ok bool
		text, _, ok, err = ReadText(path, limit)
		if err == nil && !ok {
			err = errors.New("двоичный файл")
		}
	}
	return text, err
}

// --- Обычный текст -------------------------------------------------------

// ReadText читает текстовый файл (до limit байт) и определяет кодировку.
// ok = false, если файл похож на двоичный.
func ReadText(path string, limit int) (text, encoding string, ok bool, err error) {
	file, err := os.Open(path)
	if err != nil {
		return "", "", false, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, int64(limit)))
	if err != nil {
		return "", "", false, err
	}
	text, encoding, ok = DecodeText(data)
	return text, encoding, ok, nil
}

// DecodeText определяет кодировку: UTF-8 (с BOM и без), UTF-16 с BOM,
// иначе Windows-1251 — типичная кодировка старых русских текстовых файлов.
// ok = false, если данные похожи на двоичный файл.
func DecodeText(data []byte) (text, encoding string, ok bool) {
	switch {
	case bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}):
		return string(trimPartialRune(data[3:])), "UTF-8", true
	case bytes.HasPrefix(data, []byte{0xFF, 0xFE}), bytes.HasPrefix(data, []byte{0xFE, 0xFF}):
		decoded, err := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder().Bytes(data[:len(data)&^1])
		if err != nil {
			return "", "", false
		}
		return string(decoded), "UTF-16", true
	}

	head := data[:min(len(data), 8192)]
	if bytes.IndexByte(head, 0) >= 0 {
		return "", "", false
	}
	if trimmed := trimPartialRune(data); utf8.Valid(trimmed) {
		return string(trimmed), "UTF-8", true
	}
	decoded, err := charmap.Windows1251.NewDecoder().Bytes(data)
	if err != nil {
		return "", "", false
	}
	return string(decoded), "Windows-1251", true
}

// trimPartialRune отрезает недописанный символ UTF-8 в конце, который
// появляется, когда чтение обрезано по лимиту.
func trimPartialRune(data []byte) []byte {
	for cut := 0; cut < utf8.UTFMax && cut < len(data); cut++ {
		r, size := utf8.DecodeLastRune(data[:len(data)-cut])
		if r != utf8.RuneError || size > 1 {
			return data[:len(data)-cut]
		}
	}
	return data
}

// --- Word ----------------------------------------------------------------

// DOCXText возвращает текст документа Word без оформления.
func DOCXText(path string, limit int) (string, bool, error) {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return "", false, err
	}
	defer archive.Close()

	for _, file := range archive.File {
		if file.Name != "word/document.xml" {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			return "", false, err
		}
		defer reader.Close()
		return decodeDOCX(reader, limit)
	}
	return "", false, errors.New("в DOCX не найден word/document.xml")
}

func decodeDOCX(reader io.Reader, limit int) (string, bool, error) {
	decoder := xml.NewDecoder(reader)
	out := newLimitedBuilder(limit)

	for !out.full {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", false, err
		}
		switch element := token.(type) {
		case xml.StartElement:
			switch element.Name.Local {
			case "t":
				var text string
				if err := decoder.DecodeElement(&text, &element); err != nil {
					return "", false, err
				}
				out.write(text)
			case "tab":
				out.write("\t")
			case "br", "cr":
				out.write("\n")
			}
		case xml.EndElement:
			switch element.Name.Local {
			case "p", "tr":
				out.write("\n")
			case "tc":
				out.write("\t")
			}
		}
	}
	return strings.TrimSpace(out.String()), out.full, nil
}

// --- Excel и PowerPoint --------------------------------------------------

func isSheetPart(name string) bool {
	return name == "xl/sharedStrings.xml" || (strings.HasPrefix(name, "xl/worksheets/sheet") && strings.HasSuffix(name, ".xml"))
}

func isSlidePart(name string) bool {
	return strings.HasPrefix(name, "ppt/slides/slide") && strings.HasSuffix(name, ".xml")
}

// zipText собирает текст из XML-частей документа Office. Читается сам XML,
// без разбора книги целиком, — так в разы быстрее, чем через excelize.
func zipText(path string, limit int, want func(string) bool, extract func(io.Reader, *limitedBuilder) error) (string, error) {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer archive.Close()

	out := newLimitedBuilder(limit)
	for _, file := range archive.File {
		if out.full || !want(file.Name) {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			return "", err
		}
		err = extract(reader, out)
		reader.Close()
		if err != nil {
			return "", err
		}
		out.write("\n")
	}
	return out.String(), nil
}

// xmlElementText пишет содержимое всех элементов с данным именем.
func xmlElementText(local string) func(io.Reader, *limitedBuilder) error {
	return func(reader io.Reader, out *limitedBuilder) error {
		decoder := xml.NewDecoder(reader)
		for !out.full {
			token, err := decoder.Token()
			if errors.Is(err, io.EOF) {
				return nil
			}
			if err != nil {
				return err
			}
			if element, ok := token.(xml.StartElement); ok && element.Name.Local == local {
				var text string
				if err := decoder.DecodeElement(&text, &element); err != nil {
					return err
				}
				out.write(text)
				out.write(" ")
			}
		}
		return nil
	}
}

// sheetText берёт строки (<t>) и значения ячеек (<v>), кроме ссылок на
// общие строки (t="s"): их текст уже взят из sharedStrings.xml.
func sheetText(reader io.Reader, out *limitedBuilder) error {
	decoder := xml.NewDecoder(reader)
	sharedRef := false
	for !out.full {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		element, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		switch element.Name.Local {
		case "c":
			sharedRef = false
			for _, attr := range element.Attr {
				if attr.Name.Local == "t" && attr.Value == "s" {
					sharedRef = true
				}
			}
		case "t", "v":
			var text string
			if err := decoder.DecodeElement(&text, &element); err != nil {
				return err
			}
			if element.Name.Local == "v" && sharedRef {
				continue
			}
			out.write(text)
			out.write(" ")
		}
	}
	return nil
}

// --- PDF -----------------------------------------------------------------

// pdfText достаёт текстовый слой PDF. В сканах его нет — там вернётся пусто.
func pdfText(path string, limit int) (text string, err error) {
	// Библиотека паникует на некоторых повреждённых файлах.
	defer func() {
		if r := recover(); r != nil {
			text, err = "", fmt.Errorf("не удалось прочитать PDF: %v", r)
		}
	}()
	file, reader, err := pdf.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	plain, err := reader.GetPlainText()
	if err != nil {
		return "", err
	}
	data, err := io.ReadAll(io.LimitReader(plain, int64(limit)))
	if err != nil {
		return "", err
	}
	return string(trimPartialRune(data)), nil
}

// --- Вспомогательное -----------------------------------------------------

type limitedBuilder struct {
	strings.Builder
	limit int
	full  bool
}

func newLimitedBuilder(limit int) *limitedBuilder {
	return &limitedBuilder{limit: limit}
}

func (b *limitedBuilder) write(s string) {
	if b.full || s == "" {
		return
	}
	if room := b.limit - b.Len(); len(s) > room {
		s = string(trimPartialRune([]byte(s[:max(room, 0)])))
		b.full = true
	}
	b.WriteString(s)
}
