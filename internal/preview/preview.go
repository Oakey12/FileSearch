package preview

import (
	"archive/zip"
	"encoding/xml"
	"errors"
	"file-search/internal/uiTheme"
	"file-search/internal/varAndConst"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/xuri/excelize/v2"

	_ "golang.org/x/image/bmp"
)

const (
	maxExcelRows = 500
	maxExcelCols = 50

	// PreviewWindowX и PreviewWindowY задают начальное положение окна
	// предпросмотра в пикселях от левого верхнего угла основного монитора.
	// Измените эти два значения, чтобы настроить положение окна.
	PreviewWindowX int32 = 350
	PreviewWindowY int32 = 400
)

// showPreview создаёт отдельное окно ОС, которое можно перемещать независимо.
func ShowPreview(_ fyne.Window, path string) {
	title := "Предпросмотр: " + filepath.Base(path)
	previewWindow := fyne.CurrentApp().NewWindow(title)
	previewWindow.Resize(fyne.NewSize(850, 600))
	previewWindow.SetContent(container.NewCenter(widget.NewLabel("Загрузка предпросмотра…")))
	previewWindow.Show()
	uiTheme.DarkTitleBar(previewWindow)
	go moveWindowByTitle(title, PreviewWindowX, PreviewWindowY)

	go func() {
		preview, err := createPreview(path)
		fyne.Do(func() {
			if err != nil {
				message := widget.NewLabel("Не удалось открыть файл:\n" + err.Error())
				message.Wrapping = fyne.TextWrapWord
				previewWindow.SetContent(container.NewCenter(message))
				return
			}
			previewWindow.SetContent(preview)
		})
	}()
}

func createPreview(path string) (fyne.CanvasObject, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".bmp":
		return createImagePreview(path)
	case ".txt", ".log", ".md", ".json", ".xml", ".csv",
		".go", ".js", ".ts", ".html", ".css", ".sql",
		".yaml", ".yml", ".ini", ".conf":
		return createTextPreview(path)
	case ".docx":
		return createDOCXPreview(path)
	case ".xlsx", ".xlsm", ".xltx", ".xltm":
		return createXLSXPreview(path)
	case ".doc", ".xls":
		return createFileInfoPreview(path, info,
			"Старый бинарный формат Office не поддерживается встроенным просмотром."), nil
	default:
		return createFileInfoPreview(path, info,
			"Встроенный просмотр этого формата не поддерживается."), nil
	}
}

func createImagePreview(path string) (fyne.CanvasObject, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	decodedImage, _, err := image.Decode(file)
	if err != nil {
		return nil, err
	}
	preview := canvas.NewImageFromImage(decodedImage)
	preview.FillMode = canvas.ImageFillContain
	preview.ScaleMode = canvas.ImageScaleSmooth
	return container.NewStack(preview), nil
}

func createTextPreview(path string) (fyne.CanvasObject, error) {
	text, truncated, err := readLimitedText(path)
	if err != nil {
		return nil, err
	}
	if truncated {
		text += "\n\n--- Предпросмотр ограничен первым 1 МБ ---"
	}
	return newReadOnlyText(text), nil
}

func createDOCXPreview(path string) (fyne.CanvasObject, error) {
	text, truncated, err := extractDOCXText(path, varAndConst.MaxPreviewSize)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(text) == "" {
		text = "Документ не содержит доступного для предпросмотра текста."
	}
	if truncated {
		text += "\n\n--- Предпросмотр ограничен первым 1 МБ текста ---"
	}
	return newReadOnlyText(text), nil
}

func createXLSXPreview(path string) (fyne.CanvasObject, error) {
	book, err := excelize.OpenFile(path)
	if err != nil {
		return nil, err
	}
	defer book.Close()

	sheetNames := book.GetSheetList()
	if len(sheetNames) == 0 {
		return nil, errors.New("в книге не найдено листов")
	}

	sheets := make(map[string][][]string, len(sheetNames))
	truncated := make(map[string]bool, len(sheetNames))
	for _, name := range sheetNames {
		rows, wasTruncated, readErr := readExcelSheet(book, name)
		if readErr != nil {
			return nil, fmt.Errorf("лист %q: %w", name, readErr)
		}
		sheets[name] = rows
		truncated[name] = wasTruncated
	}

	selectedSheet := sheetNames[0]
	info := widget.NewLabel("")
	info.Wrapping = fyne.TextWrapWord
	var table *widget.Table
	refreshInfo := func() {
		message := fmt.Sprintf("Лист: %s · строк: %d", selectedSheet, len(sheets[selectedSheet]))
		if truncated[selectedSheet] {
			message += fmt.Sprintf(" · показаны первые %d строк и %d столбцов", maxExcelRows, maxExcelCols)
		}
		info.SetText(message)
	}

	table = widget.NewTable(
		func() (int, int) { return excelTableSize(sheets[selectedSheet]) },
		func() fyne.CanvasObject {
			label := widget.NewLabel("")
			label.Truncation = fyne.TextTruncateEllipsis
			return label
		},
		func(id widget.TableCellID, object fyne.CanvasObject) {
			object.(*widget.Label).SetText(excelCell(sheets[selectedSheet], id.Row, id.Col))
		},
	)
	table.SetColumnWidth(0, 140)

	sheetSelect := widget.NewSelect(sheetNames, func(name string) {
		selectedSheet = name
		table.Refresh()
		refreshInfo()
	})
	sheetSelect.SetSelected(selectedSheet)
	refreshInfo()
	header := container.NewBorder(nil, nil, widget.NewLabel("Лист:"), nil, sheetSelect)
	return container.NewBorder(container.NewVBox(header, info, widget.NewSeparator()), nil, nil, nil, table), nil
}

func readExcelSheet(book *excelize.File, sheet string) ([][]string, bool, error) {
	iterator, err := book.Rows(sheet)
	if err != nil {
		return nil, false, err
	}
	defer iterator.Close()

	rows := make([][]string, 0, maxExcelRows)
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

func excelTableSize(rows [][]string) (int, int) {
	columns := 1
	for _, row := range rows {
		if len(row) > columns {
			columns = len(row)
		}
	}
	return len(rows), columns
}

func excelCell(rows [][]string, row, column int) string {
	if row < 0 || row >= len(rows) || column < 0 || column >= len(rows[row]) {
		return ""
	}
	return rows[row][column]
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

func readLimitedText(path string) (string, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", false, err
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, varAndConst.MaxPreviewSize+1))
	if err != nil {
		return "", false, err
	}
	truncated := len(data) > varAndConst.MaxPreviewSize
	if truncated {
		data = data[:varAndConst.MaxPreviewSize]
	}
	return string(data), truncated, nil
}

// newReadOnlyText показывает текст файла чёрным на белом фоне. Текст можно
// выделить и скопировать, но не изменить.
func newReadOnlyText(text string) fyne.CanvasObject {
	label := widget.NewLabel(text)
	label.Wrapping = fyne.TextWrapOff
	label.Selectable = true
	return uiTheme.Paper(container.NewScroll(label))
}

func createFileInfoPreview(path string, info os.FileInfo, reason string) fyne.CanvasObject {
	extension := filepath.Ext(path)
	if extension == "" {
		extension = "без расширения"
	}
	message := fmt.Sprintf(
		"%s\n\nИмя: %s\nТип: %s\nРазмер: %s\nИзменён: %s\n\nПуть:\n%s",
		reason, info.Name(), extension, FormatFileSize(info.Size()),
		info.ModTime().Format("02.01.2006 15:04:05"), path,
	)
	label := widget.NewLabel(message)
	label.Wrapping = fyne.TextWrapWord
	return container.NewVScroll(label)
}

func FormatFileSize(size int64) string {
	const (
		kilobyte = 1024
		megabyte = kilobyte * 1024
		gigabyte = megabyte * 1024
	)
	switch {
	case size >= gigabyte:
		return fmt.Sprintf("%.2f ГБ", float64(size)/gigabyte)
	case size >= megabyte:
		return fmt.Sprintf("%.2f МБ", float64(size)/megabyte)
	case size >= kilobyte:
		return fmt.Sprintf("%.2f КБ", float64(size)/kilobyte)
	default:
		return fmt.Sprintf("%d байт", size)
	}
}
