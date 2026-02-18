// Package main is the entry point for the BuildMax worker (runs a single task via API + direct storage).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	log "buildmax/internal/log"
	"buildmax/internal/workercmd"
)

func main() {
	log.Init()
	taskID := flag.String("task-id", "", "task ID to run (required)")
	flag.Parse()
	if *taskID == "" {
		fmt.Fprintf(os.Stderr, "error: --task-id is required\n")
		os.Exit(1)
	}
	ctx := context.Background()
	if err := workercmd.RunWorker(ctx, *taskID); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
