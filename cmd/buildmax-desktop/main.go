// Package main is the entry point for the BuildMax desktop app (Wails).
package main

import (
	"fmt"
	"os"

	"buildmax/desktop"
	"buildmax/internal/config"
	log "buildmax/internal/log"
	desktopcmd "buildmax/internal/cmd/desktop"
)

func main() {
	log.Init(log.LogConfig{
		LogsDir:    config.LogsDir(),
		Level:      config.LogLevel(),
		Filename:   "buildmax-desktop.log",
		AlsoStdout: false,
	})
	if err := desktopcmd.Run(desktop.Assets); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
