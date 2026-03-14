package desktop

import (
	"io/fs"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

// Run starts the Wails desktop application. assets is the embedded frontend
// filesystem (e.g. from //go:embed all:frontend in the main package).
// It creates an App, wires lifecycle hooks, and calls wails.Run.
func Run(assets fs.FS) error {
	app := NewApp()
	return wails.Run(&options.App{
		Title:  "BuildMax",
		Width:  900,
		Height: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.Startup,
		OnShutdown: app.Shutdown,
		Bind:       []interface{}{app},
	})
}
