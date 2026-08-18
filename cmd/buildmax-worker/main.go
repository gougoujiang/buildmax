// Package main is the entry point for the BuildMax worker (runs a single task run via API + direct storage).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/gougoujiang/buildmax/internal/bootstrap"
	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/model"
	log "github.com/gougoujiang/buildmax/internal/infra/log"
)

func main() {
	sc, _ := config.LoadServerConfig()
	// Default worker to debug log level when not explicitly set, so ephemeral worker logs are detailed enough to triage.
	level := config.LogLevel(sc.LogLevel)
	if level == "info" && sc.LogLevel == "" {
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
			os.Exit(2)
		}
		// A canceled run did what it was told and has already reported CANCELED.
		// Exiting non-zero here would make the scheduler treat an honored
		// instruction as a dispatch failure.
		if errors.Is(err, model.ErrRunCanceled) {
			slog.Info("worker run canceled", "task_run_id", *taskRunID)
			return
		}
		slog.Error("worker run failed", "task_run_id", *taskRunID, "err", err)
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
