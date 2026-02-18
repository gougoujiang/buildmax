// Package main is the entry point for the BuildMax worker (runs a single task via API + direct storage).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"buildmax/internal/config"
	"buildmax/internal/executor"
	log "buildmax/internal/log"
	"buildmax/internal/storage/blob"

	"github.com/google/uuid"
)

func main() {
	log.Init()
	taskID := flag.String("task-id", "", "task ID to run (required)")
	flag.Parse()
	if *taskID == "" {
		fmt.Fprintf(os.Stderr, "error: --task-id is required\n")
		os.Exit(1)
	}

	baseURL := config.WorkerServerURL()
	token := config.WorkerToken()
	if baseURL == "" {
		fmt.Fprintf(os.Stderr, "error: %s is required\n", config.EnvKeyBuildmaxServerURL)
		os.Exit(1)
	}
	if token == "" {
		fmt.Fprintf(os.Stderr, "error: %s is required\n", config.EnvKeyBuildmaxWorkerToken)
		os.Exit(1)
	}

	workspacesDir := config.WorkspacesDir()
	if workspacesDir == "" {
		fmt.Fprintf(os.Stderr, "error: %s is required for worker\n", config.EnvKeyBuildmaxWorkspacesDir)
		os.Exit(1)
	}
	if abs, err := filepath.Abs(workspacesDir); err == nil {
		workspacesDir = abs
	}

	ctx := context.Background()
	task, err := executor.GetWorkerTask(ctx, baseURL, token, *taskID, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: get task: %v\n", err)
		os.Exit(1)
	}
	if task == nil {
		fmt.Fprintf(os.Stderr, "error: task not found\n")
		os.Exit(1)
	}
	if task.Status != "PENDING" {
		fmt.Fprintf(os.Stderr, "error: task not pending (status=%s)\n", task.Status)
		os.Exit(1)
	}

	sessionID := uuid.New().String()
	updater := &executor.WorkerHTTPUpdater{BaseURL: baseURL, Token: token}
	now := time.Now().Unix()
	if err := updater.UpdateTaskStatus(ctx, *taskID, "RUNNING", &now, nil, nil, nil, &sessionID, nil); err != nil {
		fmt.Fprintf(os.Stderr, "error: mark RUNNING: %v\n", err)
		os.Exit(1)
	}

	wsCfg := config.LoadWorkspaceStorageConfig()
	var s3Client blob.S3Client
	if wsCfg.PersistProvider == config.ProviderMinIO || wsCfg.ArtifactProvider == config.ProviderMinIO {
		var s3Err error
		s3Client, s3Err = config.BuildS3Client(ctx, wsCfg)
		if s3Err != nil {
			fmt.Fprintf(os.Stderr, "error: S3 client: %v\n", s3Err)
			os.Exit(1)
		}
	}
	persistStorage, err := config.BuildPersistStorage(wsCfg, config.PersistentWorkspaceDir, s3Client)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: persist storage: %v\n", err)
		os.Exit(1)
	}
	artifactStorage, err := config.BuildArtifactStorage(wsCfg, config.ArtifactDir, s3Client)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: artifact storage: %v\n", err)
		os.Exit(1)
	}

	paths := executor.NewWorkspacePathsFromRoot(workspacesDir)
	if err := executor.RunTask(ctx, task, sessionID, paths, persistStorage, artifactStorage, updater); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}
