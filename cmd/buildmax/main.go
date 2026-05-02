// Package main is the entry point for the BuildMax CLI.
package main

import (
	"fmt"
	"os"

	"buildmax/internal/config"
	log "buildmax/internal/infra/log"
	"buildmax/internal/interface/cli"
)

func main() {
	log.Init(log.LogConfig{LogsDir: config.LogsDir(), Level: config.LogLevel(), AlsoStdout: false})
	root := cli.NewRootCommand()
	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
