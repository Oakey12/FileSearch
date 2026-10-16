package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

// Цвета окна постоянные и не зависят от светлой или тёмной темы Windows.
var (
	windowBackground = &options.RGBA{R: 30, G: 30, B: 32, A: 255}
	titleBar         = windows.RGB(30, 30, 32)
	titleText        = windows.RGB(242, 242, 244)
	titleTextDim     = windows.RGB(154, 154, 161)
)

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:            "Поиск файлов",
		Width:            1180,
		Height:           740,
		MinWidth:         760,
		MinHeight:        480,
		BackgroundColour: windowBackground,
		AssetServer: &assetserver.Options{
			Assets:     assets,
			Middleware: app.imageMiddleware,
		},
		OnStartup: app.startup,
		Bind:      []any{app},
		Windows: &windows.Options{
			Theme: windows.Dark,
			CustomTheme: &windows.ThemeSettings{
				DarkModeTitleBar:          titleBar,
				DarkModeTitleBarInactive:  titleBar,
				DarkModeTitleText:         titleText,
				DarkModeTitleTextInactive: titleTextDim,
				DarkModeBorder:            titleBar,
				DarkModeBorderInactive:    titleBar,
			},
			DisablePinchZoom: true,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
