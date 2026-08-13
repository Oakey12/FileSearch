package main

import (
	"context"
	"errors"
	"file-search/internal/preview"
	"file-search/internal/shell"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type searchResult struct {
	Matches []fileMatch
	Skipped int
}

type fileMatch struct {
	Path     string
	Name     string
	Type     string
	Size     int64
	Modified time.Time
	Folder   string
}

func calculateColumnWidth(matches []fileMatch, column int) float32 {
	const (
		minWidth = float32(70)
		maxWidth = float32(600)
		padding  = float32(30)
	)

	width := fyne.MeasureText(
		resultCellText(nil, 0, column),
		theme.TextSize(),
		fyne.TextStyle{Bold: true},
	).Width + padding

	for row := range matches {
		text := resultCellText(matches, row+1, column)

		textWidth := fyne.MeasureText(
			text,
			theme.TextSize(),
			fyne.TextStyle{},
		).Width + padding

		if textWidth > width {
			width = textWidth
		}
	}

	if width < minWidth {
		return minWidth
	}
	if width > maxWidth {
		return maxWidth
	}

	return width
}

func main() {
	application := app.NewWithID("ru.filesearch.desktop")
	application.Settings().SetTheme(theme.DarkTheme())

	window := application.NewWindow("Поиск файлов")
	window.Resize(fyne.NewSize(900, 600))
	window.CenterOnScreen()

	folderEntry := widget.NewEntry()
	folderEntry.SetPlaceHolder(`Например: G:\РЕГЛАМЕНТЫ`)

	nameEntry := widget.NewEntry()
	nameEntry.SetPlaceHolder("Полное имя файла или его часть")

	status := widget.NewLabel("Выберите папку и введите имя файла")
	status.Wrapping = fyne.TextWrapWord

	var matches []fileMatch
	selectedIndex := -1
	searchGeneration := 0
	var cancelSearch context.CancelFunc

	var resultTable *widget.Table

	resultTable = widget.NewTable(
		func() (int, int) {
			return len(matches) + 1, 5
		},
		func() fyne.CanvasObject {
			label := widget.NewLabel("")
			label.Truncation = fyne.TextTruncateEllipsis
			return label
		},
		func(id widget.TableCellID, object fyne.CanvasObject) {
			label := object.(*widget.Label)

			label.TextStyle = fyne.TextStyle{
				Bold: id.Row == 0,
			}
			label.SetText(resultCellText(matches, id.Row, id.Col))
		},
	)
	resultTable.SetColumnWidth(0, 210)
	resultTable.SetColumnWidth(1, 70)
	resultTable.SetColumnWidth(2, 90)
	resultTable.SetColumnWidth(3, 145)
	resultTable.SetColumnWidth(4, 330)

	openButton := widget.NewButton("Показать в Проводнике", nil)
	previewButton := widget.NewButton("Предпросмотр файла", nil)
	openButton.Disable()
	previewButton.Disable()

	previewButton.OnTapped = func() {
		if selectedIndex < 0 || selectedIndex >= len(matches) {
			return
		}

		preview.ShowPreview(window, matches[selectedIndex].Path)
	}

	openButton.OnTapped = func() {
		if selectedIndex < 0 || selectedIndex >= len(matches) {
			return
		}
		path := matches[selectedIndex].Path
		selected, err := shell.ShowInExplorer(path)
		if err != nil {
			dialog.ShowError(err, window)
			return
		}
		if selected {
			status.SetText("Файл выделен в Проводнике: " + path)
		} else {
			status.SetText("Открыта папка файла: " + filepath.Dir(path))
		}
	}

	var lastHeaderClick time.Time
	lastHeaderColumn := -1

	resultTable.OnSelected = func(id widget.TableCellID) {
		if id.Row == 0 {
			now := time.Now()
			doubleClick := id.Col == lastHeaderColumn &&
				!lastHeaderClick.IsZero() &&
				now.Sub(lastHeaderClick) <= 500*time.Millisecond

			lastHeaderClick = now
			lastHeaderColumn = id.Col
			resultTable.Unselect(id)

			if doubleClick {
				width := calculateColumnWidth(matches, id.Col)
				resultTable.SetColumnWidth(id.Col, width)
				lastHeaderClick = time.Time{}
				lastHeaderColumn = -1
			}
			return
		}
		selectedIndex = id.Row - 1
		previewButton.Enable()
		openButton.Enable()
	}

	resultTable.OnUnselected = func(id widget.TableCellID) {
		if selectedIndex == id.Row-1 {
			selectedIndex = -1
			previewButton.Disable()
			openButton.Disable()
		}
	}

	chooseFolderButton := widget.NewButton("Выбрать папку…", func() {
		folderDialog := dialog.NewFolderOpen(
			func(uri fyne.ListableURI, err error) {
				if err != nil {
					dialog.ShowError(err, window)
					return
				}

				if uri != nil {
					folderEntry.SetText(uri.Path())
				}
			},
			window,
		)

		folderDialog.Resize(fyne.NewSize(900, 650))
		folderDialog.Show()
	})

	cancelButton := widget.NewButton("Отмена поиска", func() {
		if cancelSearch == nil {
			return
		}
		cancelSearch()
		cancelSearch = nil
		status.SetText("Отмена поиска…")
	})
	cancelButton.Disable()

	searchButton := widget.NewButton("Найти", nil)
	searchButton.Importance = widget.HighImportance
	searchButton.OnTapped = func() {
		root := strings.Trim(strings.TrimSpace(folderEntry.Text), "\"")
		query := strings.TrimSpace(nameEntry.Text)
		if root == "" || query == "" {
			dialog.ShowInformation("Не заполнены поля", "Укажите папку и имя файла для поиска.", window)
			return
		}

		absoluteRoot, err := filepath.Abs(root)
		if err != nil {
			dialog.ShowError(fmt.Errorf("ошибка в пути: %w", err), window)
			return
		}
		info, err := os.Stat(absoluteRoot)
		if err != nil {
			dialog.ShowError(fmt.Errorf("не удалось открыть папку: %w", err), window)
			return
		}
		if !info.IsDir() {
			dialog.ShowError(errors.New("указанный путь не является папкой"), window)
			return
		}

		folderEntry.SetText(absoluteRoot)
		if cancelSearch != nil {
			cancelSearch()
		}
		searchContext, cancel := context.WithCancel(context.Background())
		cancelSearch = cancel
		searchGeneration++
		currentSearch := searchGeneration
		searchButton.Disable()
		chooseFolderButton.Disable()
		cancelButton.Enable()
		openButton.Disable()
		previewButton.Disable()
		resultTable.UnselectAll()
		status.SetText("Идёт поиск.")
		startSearchAnimation(searchContext, currentSearch, func() int { return searchGeneration }, status)

		go func() {
			result, searchErr := findFiles(searchContext, absoluteRoot, query)
			fyne.Do(func() {
				if currentSearch != searchGeneration {
					return
				}
				cancel()
				cancelSearch = nil
				searchButton.Enable()
				chooseFolderButton.Enable()
				cancelButton.Disable()
				if searchErr != nil && !errors.Is(searchErr, context.Canceled) {
					status.SetText("Поиск завершился с ошибкой")
					dialog.ShowError(searchErr, window)
					return
				}

				matches = result.Matches
				selectedIndex = -1
				resultTable.Refresh()
				if errors.Is(searchErr, context.Canceled) {
					status.SetText(fmt.Sprintf("Поиск отменён · найдено файлов: %d", len(matches)))
				} else if len(matches) == 0 {
					status.SetText("Файлы не найдены")
				} else {
					status.SetText(fmt.Sprintf("Найдено файлов: %d", len(matches)))
				}
				if result.Skipped > 0 {
					status.SetText(fmt.Sprintf("%s · пропущено недоступных объектов: %d", status.Text, result.Skipped))
				}
			})
		}()
	}

	newSearchButton := widget.NewButton("Новый поиск", func() {
		if cancelSearch != nil {
			cancelSearch()
			cancelSearch = nil
		}
		searchGeneration++
		searchButton.Enable()
		chooseFolderButton.Enable()
		cancelButton.Disable()

		folderEntry.SetText("")
		nameEntry.SetText("")

		matches = nil
		selectedIndex = -1

		resultTable.UnselectAll()
		resultTable.Refresh()

		previewButton.Disable()
		openButton.Disable()

		status.SetText("Выберите папку и введите имя файла")
		window.Canvas().Focus(folderEntry)
	})

	nameEntry.OnSubmitted = func(string) { searchButton.OnTapped() }

	form := container.NewVBox(
		widget.NewLabelWithStyle("Папка для поиска", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewBorder(nil, nil, nil, chooseFolderButton, folderEntry),
		widget.NewLabelWithStyle("Имя файла", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		nameEntry,
		container.NewHBox(searchButton, newSearchButton, layout.NewSpacer(), cancelButton),
		widget.NewSeparator(),
		status,
	)

	actions := container.NewHBox(
		layout.NewSpacer(),
		previewButton,
		openButton,
	)

	window.SetContent(container.NewBorder(form, actions, nil, nil, resultTable))
	window.ShowAndRun()
}

func findFiles(ctx context.Context, root, query string) (searchResult, error) {
	var result searchResult
	needle := strings.ToLower(query)

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			result.Skipped++
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if strings.Contains(strings.ToLower(entry.Name()), needle) {
			info, infoErr := entry.Info()
			if infoErr != nil {
				result.Skipped++
				return nil
			}
			extension := strings.TrimPrefix(strings.ToLower(filepath.Ext(entry.Name())), ".")
			if extension == "" {
				extension = "файл"
			}
			result.Matches = append(result.Matches, fileMatch{
				Path: path, Name: entry.Name(), Type: extension,
				Size: info.Size(), Modified: info.ModTime(), Folder: filepath.Dir(path),
			})
		}
		return nil
	})
	if errors.Is(err, fs.ErrPermission) {
		return result, nil
	}
	return result, err
}

func resultCellText(matches []fileMatch, row, column int) string {
	headers := []string{"Имя", "Тип", "Размер", "Изменён", "Папка"}
	if row == 0 {
		if column >= 0 && column < len(headers) {
			return headers[column]
		}
		return ""
	}
	index := row - 1
	if index < 0 || index >= len(matches) {
		return ""
	}
	match := matches[index]
	switch column {
	case 0:
		return match.Name
	case 1:
		return match.Type
	case 2:
		return preview.FormatFileSize(match.Size)
	case 3:
		return match.Modified.Format("02.01.2006 15:04")
	case 4:
		return match.Folder
	default:
		return ""
	}
}

func startSearchAnimation(ctx context.Context, generation int, currentGeneration func() int, status *widget.Label) {
	go func() {
		ticker := time.NewTicker(450 * time.Millisecond)
		defer ticker.Stop()
		dots := 1
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				dots = dots%3 + 1
				text := "Идёт поиск" + strings.Repeat(".", dots)
				fyne.Do(func() {
					if generation == currentGeneration() && ctx.Err() == nil {
						status.SetText(text)
					}
				})
			}
		}
	}()
}
