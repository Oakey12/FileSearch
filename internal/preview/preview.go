// Package preview готовит содержимое файла для панели предпросмотра.
// Пакет ничего не рисует: он возвращает данные, а показывает их интерфейс.
package preview

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

	"github.com/xuri/excelize/v2"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
)

const (
	// MaxTextSize — сколько байт текста читать для предпросмотра.
	MaxTextSize = 1024 * 1024

	maxExcelRows = 500
	maxExcelCols = 50
)

// Kind — способ показа файла.
const (
	KindText  = "text"
	KindImage = "image"
	KindSheet = "sheet"
	KindInfo  = "info"
)

// Result — всё, что нужно панели предпросмотра.
type Result struct {
	Kind      string  `json:"kind"`
	Path      string  `json:"path"`
	Name      string  `json:"name"`
	Ext       string  `json:"ext"`
	Size      int64   `json:"size"`
	Modified  int64   `json:"mod"`
	Text      string  `json:"text,omitempty"`
	Encoding  string  `json:"encoding,omitempty"`
	Truncated bool    `json:"truncated,omitempty"`
	Sheets    []Sheet `json:"sheets,omitempty"`
	Note      string  `json:"note,omitempty"`
	Image     string  `json:"image,omitempty"` // адрес картинки, заполняет приложение
}

// Sheet — лист Excel, обрезанный до maxExcelRows × maxExcelCols.
type Sheet struct {
	Name      string     `json:"name"`
	Rows      [][]string `json:"rows"`
	Truncated bool       `json:"truncated,omitempty"`
}

var textExtensions = map[string]bool{
	".txt": true, ".log": true, ".md": true, ".json": true, ".xml": true, ".csv": true, ".tsv": true,
	".go": true, ".js": true, ".ts": true, ".html": true, ".htm": true, ".css": true, ".sql": true,
	".yaml": true, ".yml": true, ".ini": true, ".conf": true, ".cfg": true, ".toml": true,
	".bat": true, ".cmd": true, ".ps1": true, ".py": true, ".cs": true, ".java": true,
	".c": true, ".h": true, ".cpp": true, ".php": true, ".sh": true,
}

var imageExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".bmp": true, ".webp": true, ".ico": true,
}

// IsImage сообщает, показывается ли файл как картинка.
func IsImage(path string) bool {
	return imageExtensions[strings.ToLower(filepath.Ext(path))]
}

// Load читает файл и возвращает данные для предпросмотра.
func Load(path string) (Result, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Result{}, err
	}
	if info.IsDir() {
		return Result{}, errors.New("это папка, а не файл")
	}

	ext := strings.ToLower(filepath.Ext(path))
	result := Result{
		Kind:     KindInfo,
		Path:     path,
		Name:     info.Name(),
		Ext:      strings.TrimPrefix(ext, "."),
		Size:     info.Size(),
		Modified: info.ModTime().UnixMilli(),
	}

	switch {
	case imageExtensions[ext]:
		result.Kind = KindImage
	case textExtensions[ext]:
		err = loadText(path, &result)
	case ext == ".docx":
		err = loadDOCX(path, &result)
	case ext == ".xlsx" || ext == ".xlsm" || ext == ".xltx" || ext == ".xltm":
		err = loadSheets(path, &result)
	case ext == ".doc" || ext == ".xls":
		result.Note = "Старый формат Office не показывается во встроенном просмотре. Откройте файл в Word или Excel."
	default:
		result.Note = "Этот формат не показывается во встроенном просмотре."
	}
	return result, err
}

func loadText(path string, result *Result) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, MaxTextSize+1))
	if err != nil {
		return err
	}
	if len(data) > MaxTextSize {
		data = data[:MaxTextSize]
		result.Truncated = true
	}

	text, encoding, ok := decodeText(data)
	if !ok {
		result.Note = "Файл похож на двоичный, текст не показывается."
		return nil
	}
	result.Kind = KindText
	result.Text = text
	result.Encoding = encoding
	return nil
}

// decodeText определяет кодировку: UTF-8 (с BOM и без), UTF-16 с BOM,
// иначе Windows-1251 — типичная кодировка старых русских текстовых файлов.
// ok = false, если данные похожи на двоичный файл.
func decodeText(data []byte) (text, encoding string, ok bool) {
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
// появляется, когда чтение обрезано по MaxTextSize.
func trimPartialRune(data []byte) []byte {
	for cut := 0; cut < utf8.UTFMax && cut < len(data); cut++ {
		r, size := utf8.DecodeLastRune(data[:len(data)-cut])
		if r != utf8.RuneError || size > 1 {
			return data[:len(data)-cut]
		}
	}
	return data
}

func loadDOCX(path string, result *Result) error {
	text, truncated, err := extractDOCXText(path, MaxTextSize)
	if err != nil {
		return err
	}
	if strings.TrimSpace(text) == "" {
		result.Note = "В документе нет текста для предпросмотра."
		return nil
	}
	result.Kind = KindText
	result.Text = text
	result.Truncated = truncated
	return nil
}

func loadSheets(path string, result *Result) error {
	book, err := excelize.OpenFile(path)
	if err != nil {
		return err
	}
	defer book.Close()

	for _, name := range book.GetSheetList() {
		rows, truncated, err := readExcelSheet(book, name)
		if err != nil {
			return fmt.Errorf("лист %q: %w", name, err)
		}
		result.Sheets = append(result.Sheets, Sheet{Name: name, Rows: rows, Truncated: truncated})
	}
	if len(result.Sheets) == 0 {
		return errors.New("в книге не найдено листов")
	}
	result.Kind = KindSheet
	return nil
}

func readExcelSheet(book *excelize.File, sheet string) ([][]string, bool, error) {
	iterator, err := book.Rows(sheet)
	if err != nil {
		return nil, false, err
	}
	defer iterator.Close()

	rows := make([][]string, 0, 64)
	truncated := false
	for iterator.Next() {
		if len(rows) >= maxExcelRows {
			truncated = true
			break
		}
		columns, err := iterator.Columns()
		if err != nil {
			return nil, false, err
		}
		if len(columns) > maxExcelCols {
			columns = columns[:maxExcelCols]
			truncated = true
		}
		rows = append(rows, columns)
	}
	if err := iterator.Error(); err != nil {
		return nil, false, err
	}
	return rows, truncated, nil
}

func extractDOCXText(path string, limit int64) (string, bool, error) {
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
		return decodeDOCXDocument(reader, limit)
	}
	return "", false, errors.New("в DOCX не найден word/document.xml")
}

func decodeDOCXDocument(reader io.Reader, limit int64) (string, bool, error) {
	decoder := xml.NewDecoder(reader)
	var builder strings.Builder
	truncated := false
	appendText := func(value string) {
		if truncated || value == "" {
			return
		}
		remaining := int(limit) - builder.Len()
		if remaining <= 0 {
			truncated = true
			return
		}
		if len(value) > remaining {
			value = value[:remaining]
			truncated = true
		}
		builder.WriteString(value)
	}

	for {
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
				appendText(text)
			case "tab":
				appendText("\t")
			case "br", "cr":
				appendText("\n")
			}
		case xml.EndElement:
			switch element.Name.Local {
			case "p":
				appendText("\n")
			case "tc":
				appendText("\t")
			case "tr":
				appendText("\n")
			}
		}
		if truncated {
			break
		}
	}
	return strings.TrimSpace(builder.String()), truncated, nil
}
