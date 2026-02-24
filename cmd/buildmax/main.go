// Package main is the entry point for the BuildMax CLI.
package main

import (
	"fmt"
	"os"

	"buildmax/internal/cmd"
	"buildmax/internal/config"
	log "buildmax/internal/log"
)

func main() {
	log.Init(log.LogConfig{LogsDir: config.LogsDir(), Level: config.LogLevel(), AlsoStdout: false})
	root := cmd.NewRootCommand()
	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
