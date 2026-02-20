// Package main is the entry point for the BuildMax worker (runs a single task run via API + direct storage).
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
	level := config.LogLevel()
	if level == "" {
		level = "debug"
	}
	log.Init(config.LogsDir(), level, "buildmax-worker.log", true)
	runID := flag.String("task-run-id", "", "task run ID to run (required)")
	flag.Parse()
	if *runID == "" {
		slog.Error("worker: --task-run-id is required")
		fmt.Fprintf(os.Stderr, "error: --task-run-id is required\n")
		os.Exit(1)
	}
	ctx := context.Background()
	if err := workercmd.RunWorker(ctx, *runID); err != nil {
		if errors.Is(err, workercmd.ErrAlreadyClaimed) {
			os.Exit(2) // run not executed (already claimed by another worker)
		}
		slog.Error("worker run failed", "run_id", *runID, "err", err)
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
