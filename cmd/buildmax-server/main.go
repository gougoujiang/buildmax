// Package main is the entry point for the BuildMax HTTP server (backend for portal).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"buildmax/internal/bootstrap"
	"buildmax/internal/config"
	log "buildmax/internal/infra/log"
)

func main() {
	log.Init(log.LogConfig{LogsDir: config.LogsDir(), Level: config.LogLevel(), Filename: "buildmax-server.log", AlsoStdout: true})
	portFlag := flag.Int("port", 0, "port to listen on (default: 5678 or BUILDMAX_SERVER_PORT)")
	flag.Parse()

	port, err := config.ResolveServerPort(*portFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	ctx := context.Background()
	if err := bootstrap.RunServer(ctx, port); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
