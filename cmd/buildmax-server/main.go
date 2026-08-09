// Package main is the entry point for the BuildMax HTTP server (backend for portal).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/gougoujiang/buildmax/internal/bootstrap"
	"github.com/gougoujiang/buildmax/internal/config"
	log "github.com/gougoujiang/buildmax/internal/infra/log"
)

func main() {
	sc, _ := config.LoadServerConfig()
	log.Init(log.LogConfig{LogsDir: config.LogsDir(), Level: config.LogLevel(sc.LogLevel), Filename: "buildmax-server.log", AlsoStdout: true})

	portFlag := flag.Int("port", 0, "port to listen on (overrides server.yaml port, default 5678)")
	flag.Parse()

	ctx := context.Background()
	if err := bootstrap.RunServer(ctx, *portFlag); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
