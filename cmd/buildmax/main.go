// Package main is the entry point for the BuildMax CLI.
package main

import (
	"fmt"
	"os"

	"github.com/gougoujiang/buildmax/internal/config"
	log "github.com/gougoujiang/buildmax/internal/infra/log"
	"github.com/gougoujiang/buildmax/internal/interface/cli"
)

func main() {
	s, _ := config.LoadSettings()
	log.Init(log.LogConfig{LogsDir: config.LogsDir(), Level: config.LogLevel(s.LogLevel), AlsoStdout: false})
	root := cli.NewRootCommand()
	if err := root.Execute(); err != nil {
		code := cli.ExitCodeFor(err)
		if _, isExit := err.(*cli.ExitError); !isExit {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
		os.Exit(code)
	}
}
