// Package main is the entry point for the BuildMax worker (runs a single task run via API + direct storage).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"buildmax/internal/bootstrap"
	"buildmax/internal/config"
	log "buildmax/internal/infra/log"
)

func main() {
	// Default worker to debug log level when not set, so ephemeral worker logs are detailed enough to triage.
	level := config.LogLevel()
	if level == "" {
		level = "debug"
	}
	log.Init(log.LogConfig{LogsDir: config.LogsDir(), Level: level, Filename: "buildmax-worker.log", AlsoStdout: true})
	taskRunID := flag.String("task-run-id", "", "task run ID to run (required)")
	flag.Parse()
	if *taskRunID == "" {
		slog.Error("worker: --task-run-id is required")
		fmt.Fprintf(os.Stderr, "error: --task-run-id is required\n")
		os.Exit(1)
	}
	ctx := context.Background()
	if err := bootstrap.RunWorker(ctx, *taskRunID); err != nil {
		if errors.Is(err, bootstrap.ErrAlreadyClaimed) {
			os.Exit(2) // run not executed (already claimed by another worker)
		}
		slog.Error("worker run failed", "task_run_id", *taskRunID, "err", err)
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
