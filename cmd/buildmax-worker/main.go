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
	chatRunID := flag.String("chat-run-id", "", "chat run ID to run (required)")
	flag.Parse()
	if *chatRunID == "" {
		slog.Error("worker: --chat-run-id is required")
		fmt.Fprintf(os.Stderr, "error: --chat-run-id is required\n")
		os.Exit(1)
	}
	ctx := context.Background()
	if err := workercmd.RunWorker(ctx, *chatRunID); err != nil {
		if errors.Is(err, workercmd.ErrAlreadyClaimed) {
			os.Exit(2) // run not executed (already claimed by another worker)
		}
		slog.Error("worker run failed", "chat_run_id", *chatRunID, "err", err)
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
