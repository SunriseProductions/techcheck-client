package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/wizard"
)

//go:embed all:frontend
var assets embed.FS

func main() {
	app := wizard.NewApp()

	err := wails.Run(&options.App{
		Title:  "Sunrise Tech Check",
		Width:  720,
		Height: 640,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: app.Startup,
		Bind:      []interface{}{app},
	})
	if err != nil {
		panic(err)
	}
}
