// Package main is the entry point for the BuildMax HTTP server (backend for portal).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"buildmax/internal/config"
	log "buildmax/internal/log"
	"buildmax/internal/servercmd"
)

func main() {
	log.Init("buildmax-server.log", true)
	portFlag := flag.Int("port", 0, "port to listen on (default: 5678 or BUILDMAX_SERVER_PORT)")
	flag.Parse()

	port, err := config.ResolveServerPort(*portFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	ctx := context.Background()
	if err := servercmd.RunServer(ctx, port); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
