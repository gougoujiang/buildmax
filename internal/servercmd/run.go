package servercmd

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strconv"

	"buildmax/internal/config"
	"buildmax/internal/executor"
	"buildmax/internal/server"
	"buildmax/internal/storage/blob"
	"buildmax/internal/storage/entity"
)

// RunServer loads server env and workspaces dir, opens the DB, builds blob storage,
// creates and starts the task executor, then runs the HTTP server until shutdown.
// The port argument should already be resolved (e.g. via config.ResolveServerPort).
func RunServer(ctx context.Context, port int) error {
	serverEnv, err := config.LoadServerEnv()
	if err != nil {
		return err
	}
	workspacesDir := config.WorkspacesDir()
	if workspacesDir == "" {
		return fmt.Errorf("%s is required for server mode", config.EnvKeyBuildmaxWorkspacesDir)
	}
	if abs, err := filepath.Abs(workspacesDir); err == nil {
		workspacesDir = abs
	}
	dsn := config.MySQLDSN()
	st, err := entity.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	if err := st.BackfillTaskWorkspaceID(ctx); err != nil {
		slog.Warn("backfill task workspace_id", "err", err)
	}

	wsCfg := config.LoadWorkspaceStorageConfig()
	var s3Client blob.S3Client
	if wsCfg.PersistProvider == config.ProviderMinIO || wsCfg.ArtifactProvider == config.ProviderMinIO {
		var s3Err error
		s3Client, s3Err = config.BuildS3Client(ctx, wsCfg)
		if s3Err != nil {
			return fmt.Errorf("S3 client: %w", s3Err)
		}
	}
	persistStorage, err := config.BuildPersistStorage(wsCfg, config.PersistentWorkspaceDir, s3Client)
	if err != nil {
		return fmt.Errorf("persist storage: %w", err)
	}
	artifactStorage, err := config.BuildArtifactStorage(wsCfg, config.ArtifactDir, s3Client)
	if err != nil {
		return fmt.Errorf("artifact storage: %w", err)
	}

	cfg := server.Config{
		Addr:             ":" + strconv.Itoa(port),
		UserStore:        st,
		WorkspaceStore:   st,
		ProjectStore:     st,
		TaskStore:        st,
		ArtifactStore:    st,
		PersistStorage:   persistStorage,
		ArtifactStorage:  artifactStorage,
		WorkspacesDir:    workspacesDir,
		JWTSecret:        serverEnv.JWTSecret,
		CORSOrigin:       serverEnv.CORSOrigin,
		WorkerToken:      config.WorkerToken(),
	}
	workerPath := config.WorkerBinaryPath()
	runner, err := executor.NewRunner(st, workerPath)
	if err != nil {
		return fmt.Errorf("executor: %w", err)
	}
	runner.Start()
	defer runner.Stop()

	s := server.New(cfg)
	slog.Info("server starting", "addr", cfg.Addr)
	err = s.Run()
	slog.Info("server stopped")
	if err != nil {
		return err
	}
	return nil
}
