package main

import (
	"errors"
	"file-search/internal/mainFunc"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type searchResult struct {
	Matches []string
	Skipped int
}

func main() {
	application := app.NewWithID("ru.filesearch.desktop")
	application.Settings().SetTheme(theme.DarkTheme())
	window := application.NewWindow("Поиск файлов")
	window.Resize(fyne.NewSize(900, 600))
	window.CenterOnScreen()

	folderEntry := widget.NewEntry()
	folderEntry.SetPlaceHolder(`Например: G:\РЕГЛАМЕНТЫ IT-отдела`)

	nameEntry := widget.NewEntry()
	nameEntry.SetPlaceHolder("Полное имя файла или его часть")

	status := widget.NewLabel("Выберите папку и введите имя файла")
	status.Wrapping = fyne.TextWrapWord

	var matches []string
	selectedIndex := -1
	searchGeneration := 0

	resultList := widget.NewList(
		func() int { return len(matches) },
		func() fyne.CanvasObject {
			label := widget.NewLabel("")
			label.Truncation = fyne.TextTruncateEllipsis
			return label
		},
		func(id widget.ListItemID, object fyne.CanvasObject) {
			if id < 0 || id >= len(matches) {
				return
			}
			object.(*widget.Label).SetText(fmt.Sprintf("%d. %s", id+1, matches[id]))
		},
	)

	openButton := widget.NewButton("Показать в Проводнике", nil)
	previewButton := widget.NewButton("Предпросмотр файла", nil)
	openButton.Disable()

	previewButton.OnTapped = func() {
		if selectedIndex < 0 || selectedIndex >= len(matches) {
			return
		}

		showPreview(window, matches[selectedIndex])
	}

	openButton.OnTapped = func() {
		if selectedIndex < 0 || selectedIndex >= len(matches) {
			return
		}
		path := matches[selectedIndex]
		selected, err := mainFunc.ShowInExplorer(path)
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

	resultList.OnSelected = func(id widget.ListItemID) {
		selectedIndex = id
		previewButton.Enable()
		openButton.Enable()
	}

	resultList.OnUnselected = func(id widget.ListItemID) {
		if selectedIndex == id {
			selectedIndex = -1
			previewButton.Disable()
			openButton.Disable()
		}
	}

	chooseFolderButton := widget.NewButton("Выбрать папку…", func() {
		dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil {
				dialog.ShowError(err, window)
				return
			}
			if uri != nil {
				folderEntry.SetText(uri.Path())
			}
		}, window)
	})

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
		searchGeneration++
		currentSearch := searchGeneration
		searchButton.Disable()
		chooseFolderButton.Disable()
		openButton.Disable()
		previewButton.Disable()
		resultList.UnselectAll()
		status.SetText("Идёт поиск…")

		go func() {
			result, searchErr := findFiles(absoluteRoot, query)
			fyne.Do(func() {
				if currentSearch != searchGeneration {
					return
				}
				searchButton.Enable()
				chooseFolderButton.Enable()
				if searchErr != nil {
					status.SetText("Поиск завершился с ошибкой")
					dialog.ShowError(searchErr, window)
					return
				}

				matches = result.Matches
				selectedIndex = -1
				resultList.Refresh()
				if len(matches) == 0 {
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
		searchGeneration++
		searchButton.Enable()
		chooseFolderButton.Enable()

		folderEntry.SetText("")
		nameEntry.SetText("")

		matches = nil
		selectedIndex = -1

		resultList.UnselectAll()
		resultList.Refresh()

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
		container.NewHBox(searchButton, newSearchButton),
		widget.NewSeparator(),
		status,
	)

	actions := container.NewHBox(
		layout.NewSpacer(),
		previewButton,
		openButton,
	)

	window.SetContent(container.NewBorder(form, actions, nil, nil, resultList))
	window.ShowAndRun()
}

func findFiles(root, query string) (searchResult, error) {
	var result searchResult
	needle := strings.ToLower(query)

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
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
			result.Matches = append(result.Matches, path)
		}
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrPermission) {
		return result, err
	}
	return result, nil
}
