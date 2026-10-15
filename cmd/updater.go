package main

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"time"

	"file-search/internal/update"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// updateCheckInterval — как часто в фоне проверять, не вышла ли новая версия.
const updateCheckInterval = 15 * time.Minute

// updateDotColor — цвет кружка «есть обновление».
var updateDotColor = color.NRGBA{R: 0xF4, G: 0x43, B: 0x36, A: 0xFF}

// newUpdateControls создаёт кнопку «Обновить» и кружок справа от неё.
// Кнопка проверяет latest.json на сервере Nginx, скачивает новую версию,
// подменяет exe и перезапускает приложение. Кружок виден, только когда
// на сервере есть версия новее установленной.
func newUpdateControls(application fyne.App, window fyne.Window) fyne.CanvasObject {
	indicator := container.NewCenter(container.NewGridWrap(fyne.NewSize(10, 10), canvas.NewCircle(updateDotColor)))
	indicator.Hide()
	setAvailable := func(available bool) {
		if available {
			indicator.Show()
		} else {
			indicator.Hide()
		}
	}

	button := widget.NewButton("Обновить", nil)
	button.OnTapped = func() {
		button.Disable()
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			release, newer, err := update.CheckLatest(ctx)
			fyne.Do(func() {
				button.Enable()
				if err == nil || errors.Is(err, update.ErrNoReleases) {
					setAvailable(newer)
				}
				switch {
				case errors.Is(err, update.ErrNoReleases):
					dialog.ShowInformation("Обновление", "На сервере пока нет опубликованной версии.", window)
				case err != nil:
					dialog.ShowError(err, window)
				case !newer:
					dialog.ShowInformation("Обновление", "Установлена последняя версия: "+update.CurrentVersion, window)
				default:
					confirmUpdate(application, window, button, release)
				}
			})
		}()
	}

	go watchUpdates(setAvailable)
	return container.NewHBox(button, indicator)
}

// watchUpdates при запуске и затем каждые updateCheckInterval тихо проверяет сервер.
// Если сервер недоступен, кружок остаётся в прежнем состоянии.
func watchUpdates(setAvailable func(bool)) {
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		_, newer, err := update.CheckLatest(ctx)
		cancel()
		if err == nil || errors.Is(err, update.ErrNoReleases) {
			fyne.Do(func() { setAvailable(newer) })
		}
		time.Sleep(updateCheckInterval)
	}
}

func confirmUpdate(application fyne.App, window fyne.Window, button *widget.Button, release update.Release) {
	message := fmt.Sprintf("Доступна версия %s (установлена %s).", release.Version, update.CurrentVersion)
	if release.Notes != "" {
		message += "\n\n" + release.Notes
	}
	message += "\n\nСкачать обновление и перезапустить программу?"

	dialog.ShowConfirm("Доступно обновление", message, func(ok bool) {
		if ok {
			installUpdate(application, window, button, release)
		}
	}, window)
}

func installUpdate(application fyne.App, window fyne.Window, button *widget.Button, release update.Release) {
	button.Disable()

	ctx, cancel := context.WithCancel(context.Background())
	bar := widget.NewProgressBar()
	label := widget.NewLabel("Скачивание версии " + release.Version + "…")
	progress := dialog.NewCustom("Обновление", "Отмена", container.NewVBox(label, bar), window)
	progress.SetOnClosed(cancel)
	progress.Resize(fyne.NewSize(400, 0))
	progress.Show()

	go func() {
		path, err := update.Download(ctx, release, func(done, total int64) {
			if total <= 0 {
				return
			}
			fyne.Do(func() { bar.SetValue(float64(done) / float64(total)) })
		})
		if err == nil {
			err = update.Apply(path)
		}
		fyne.Do(func() {
			if err == nil {
				// Новая версия уже запущена, старую закрываем.
				application.Quit()
				return
			}
			progress.Hide()
			button.Enable()
			if !errors.Is(err, context.Canceled) {
				dialog.ShowError(err, window)
			}
		})
	}()
}
