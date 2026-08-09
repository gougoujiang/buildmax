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
