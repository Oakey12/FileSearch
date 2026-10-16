// Package preview готовит содержимое файла для панели предпросмотра.
// Пакет ничего не рисует: он возвращает данные, а показывает их интерфейс.
package preview

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"file-search/internal/content"
	"file-search/internal/onec"

	"github.com/xuri/excelize/v2"
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
	KindPDF   = "pdf"
	KindInfo  = "info"
	KindOneC  = "onec" // файл 1С: сводка и начало главного модуля
)

// oneCPreviewLines — сколько строк главного модуля показывать в предпросмотре.
const oneCPreviewLines = 400

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
	URL       string  `json:"url,omitempty"` // адрес картинки или PDF, заполняет приложение

	OneC *onec.Summary `json:"onec,omitempty"`
}

// Sheet — лист Excel, обрезанный до maxExcelRows × maxExcelCols.
type Sheet struct {
	Name      string     `json:"name"`
	Rows      [][]string `json:"rows"`
	Truncated bool       `json:"truncated,omitempty"`
}

var imageExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".bmp": true, ".webp": true, ".ico": true,
}

// Servable сообщает, что файл показывается по адресу (картинка или PDF),
// а не передаётся в интерфейс содержимым.
func Servable(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return imageExtensions[ext] || ext == ".pdf"
}

// IsOneC сообщает, что файл — обработка, отчёт, конфигурация или расширение 1С.
// .md считается файлом 1С, только если это составной документ 1С 7.7, а не Markdown.
func IsOneC(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".epf", ".erf", ".ert", ".cf", ".cfe":
		return true
	case ".md":
		return isOneC77(path)
	}
	return false
}

func newResult(path string) (Result, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Result{}, err
	}
	if info.IsDir() {
		return Result{}, errors.New("это папка, а не файл")
	}
	ext := strings.ToLower(filepath.Ext(path))
	return Result{
		Kind:     KindInfo,
		Path:     path,
		Name:     info.Name(),
		Ext:      strings.TrimPrefix(ext, "."),
		Size:     info.Size(),
		Modified: info.ModTime().UnixMilli(),
	}, nil
}

// OneC — предпросмотр уже открытого файла 1С (приложение держит его в общем
// с декодером кеше). openErr — ошибка открытия: она показывается заметкой.
func OneC(path string, f *onec.File, openErr error) (Result, error) {
	result, err := newResult(path)
	if err != nil {
		return result, err
	}
	if openErr != nil {
		result.Note = "Не удалось прочитать файл 1С: " + openErr.Error()
		return result, nil
	}
	summary := f.Summary(oneCPreviewLines)
	if summary.Sections == 0 {
		result.Note = "В файле 1С нет модулей с кодом."
		return result, nil
	}
	result.Kind = KindOneC
	result.OneC = &summary
	result.Encoding = "1С"
	return result, nil
}

// Load читает файл и возвращает данные для предпросмотра.
func Load(path string) (Result, error) {
	if IsOneC(path) {
		f, err := onec.Open(path)
		return OneC(path, f, err)
	}
	result, err := newResult(path)
	if err != nil {
		return result, err
	}
	ext := "." + result.Ext

	switch {
	case imageExtensions[ext]:
		result.Kind = KindImage
	case ext == ".pdf":
		result.Kind = KindPDF
	case content.IsText(path):
		err = loadText(path, &result)
	case ext == ".docx":
		err = loadDOCX(path, &result)
	case ext == ".pptx":
		err = loadSlides(path, &result)
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
	text, encoding, ok, err := content.ReadText(path, MaxTextSize)
	if err != nil {
		return err
	}
	if !ok {
		result.Note = "Файл похож на двоичный, текст не показывается."
		return nil
	}
	result.Kind = KindText
	result.Text = text
	result.Encoding = encoding
	result.Truncated = result.Size > MaxTextSize
	return nil
}

func loadDOCX(path string, result *Result) error {
	text, truncated, err := content.DOCXText(path, MaxTextSize)
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

func loadSlides(path string, result *Result) error {
	text, err := content.Extract(path, MaxTextSize)
	if err != nil {
		return err
	}
	if strings.TrimSpace(text) == "" {
		result.Note = "В презентации нет текста для предпросмотра."
		return nil
	}
	result.Kind = KindText
	result.Text = text
	return nil
}

// isOneC77 — .md оказался конфигурацией 1С 7.7, а не Markdown.
func isOneC77(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, 8)
	n, _ := f.Read(head)
	return onec.IsOLE(head[:n])
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
