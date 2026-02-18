// Package main is the entry point for the BuildMax CLI.
package main

import (
	"fmt"
	"os"

	"buildmax/internal/cmd"
	log "buildmax/internal/log"
)

func main() {
	log.Init("", false)
	root := cmd.NewRootCommand()
	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
