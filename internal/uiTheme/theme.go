// Package uiTheme задаёт постоянные цвета приложения, не зависящие от темы Windows.
package uiTheme

import (
	"file-search/internal/shell"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
)

// Dark возвращает основную тему: всегда тёмные цвета Fyne.
// Светлая/тёмная тема Windows и переменная FYNE_THEME на неё не влияют.
func Dark() fyne.Theme {
	return fixedTheme{variant: theme.VariantDark}
}

// Paper показывает content чёрным текстом на белом фоне (текстовый предпросмотр).
func Paper(content fyne.CanvasObject) fyne.CanvasObject {
	background := canvas.NewRectangle(color.White)
	return container.NewThemeOverride(
		container.NewStack(background, content),
		fixedTheme{variant: theme.VariantLight, paper: true},
	)
}

// DarkTitleBar делает рамку окна тёмной. Fyne при создании окна красит её
// по теме Windows, поэтому вызывать нужно после window.Show().
func DarkTitleBar(window fyne.Window) {
	shell.SetDarkTitleBar(shell.WindowHandle(window))
}

type fixedTheme struct {
	variant fyne.ThemeVariant
	paper   bool
}

func (t fixedTheme) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	if t.paper {
		switch name {
		case theme.ColorNameBackground, theme.ColorNameInputBackground:
			return color.White
		case theme.ColorNameForeground:
			return color.Black
		}
	}
	return theme.DefaultTheme().Color(name, t.variant)
}

func (fixedTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (fixedTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (fixedTheme) Size(name fyne.ThemeSizeName) float32 {
	return theme.DefaultTheme().Size(name)
}
