// Package main is the entry point for the BuildMax CLI.
package main

import (
	"fmt"
	"os"

	log "buildmax/internal/log"
)

// Version is the application version, shown by the version subcommand.
var Version = "0.0.1"

func main() {
	log.Init()
	root := newRootCommand()
	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
