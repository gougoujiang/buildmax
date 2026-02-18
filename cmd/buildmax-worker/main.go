// Package main is the entry point for the BuildMax worker (runs a single task via API + direct storage).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"buildmax/internal/config"
	log "buildmax/internal/log"
	"buildmax/internal/workercmd"
)

func main() {
	// Default worker to debug log level when not set, so ephemeral worker logs are detailed enough to triage.
	if os.Getenv(config.EnvKeyBuildmaxLogLevel) == "" {
		os.Setenv(config.EnvKeyBuildmaxLogLevel, "debug")
	}
	log.Init("buildmax-worker.log", true)
	taskID := flag.String("task-id", "", "task ID to run (required)")
	flag.Parse()
	if *taskID == "" {
		slog.Error("worker: --task-id is required")
		fmt.Fprintf(os.Stderr, "error: --task-id is required\n")
		os.Exit(1)
	}
	ctx := context.Background()
	if err := workercmd.RunWorker(ctx, *taskID); err != nil {
		if errors.Is(err, workercmd.ErrAlreadyClaimed) {
			os.Exit(2) // task not run (already claimed by another worker)
		}
		slog.Error("worker run failed", "task_id", *taskID, "err", err)
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
