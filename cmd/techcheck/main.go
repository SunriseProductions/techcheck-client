package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/report"
	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/updater"
	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/wizard"
)

//go:embed all:frontend
var assets embed.FS

func main() {
	// Helper mode: when launched with --apply-update we are a detached copy
	// performing the in-place swap, not the GUI. Do this before wails.Run.
	if len(os.Args) > 1 && os.Args[1] == "--apply-update" {
		if err := updater.RunApplyUpdateFromArgs(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "apply-update:", err)
			os.Exit(1)
		}
		return
	}

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
		Mac: &mac.Options{
			About: aboutInfo(report.ToolVersion, app.ITContactEmail()),
		},
	})
	if err != nil {
		panic(err)
	}
}
