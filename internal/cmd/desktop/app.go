// Package desktop implements the BuildMax desktop app (Wails) and is used by cmd/buildmax-desktop.
package desktop

import (
	"context"

	"buildmax/internal/config"
)

// App holds desktop application state and implements Wails lifecycle hooks.
type App struct{}

// NewApp returns a new App instance.
func NewApp() *App {
	return &App{}
}

// Startup is called by Wails when the app is starting. It ensures the
// application data directory is set (config.DataDir / BUILDMAX_HOME) so
// future agent integration uses the same paths as the CLI.
func (a *App) Startup(ctx context.Context) {
	_ = config.DataDir()
}

// Shutdown is called by Wails when the app is shutting down. No-op for this task.
func (a *App) Shutdown(ctx context.Context) {}
