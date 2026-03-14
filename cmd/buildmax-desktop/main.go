// Package main is the entry point for the BuildMax desktop app (Wails).
package main

import (
	"embed"
	"fmt"
	"os"

	"buildmax/internal/config"
	log "buildmax/internal/log"
	"buildmax/internal/cmd/desktop"
)

//go:embed all:frontend
var frontendAssets embed.FS

func main() {
	log.Init(log.LogConfig{
		LogsDir:    config.LogsDir(),
		Level:      config.LogLevel(),
		Filename:   "buildmax-desktop.log",
		AlsoStdout: false,
	})
	if err := desktop.Run(frontendAssets); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
