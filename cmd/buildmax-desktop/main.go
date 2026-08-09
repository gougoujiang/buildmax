// Package main is the entry point for the BuildMax desktop app (Wails).
package main

import (
	"fmt"
	"os"

	"github.com/gougoujiang/buildmax/desktop"
	"github.com/gougoujiang/buildmax/internal/config"
	log "github.com/gougoujiang/buildmax/internal/infra/log"
	desktopcmd "github.com/gougoujiang/buildmax/internal/interface/desktop"
)

func main() {
	// The frontend bundle is embedded only under the `desktop` build tag. Without
	// it the app would open a blank window, so fail with an actionable message.
	if !desktop.Embedded {
		fmt.Fprintln(os.Stderr, "error: this binary was built without the embedded frontend.")
		fmt.Fprintln(os.Stderr, "Build the desktop app with: ./make build")
		fmt.Fprintln(os.Stderr, "Or directly with: cd cmd/buildmax-desktop && wails build -tags desktop")
		os.Exit(1)
	}
	s, _ := config.LoadSettings()
	log.Init(log.LogConfig{
		LogsDir:    config.LogsDir(),
		Level:      config.LogLevel(s.LogLevel),
		Filename:   "buildmax-desktop.log",
		AlsoStdout: false,
	})
	if err := desktopcmd.Run(desktop.Assets); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
