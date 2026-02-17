package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"buildmax/internal/config"
	"buildmax/internal/executor"
	"buildmax/internal/server"
	"buildmax/internal/storage/blob"
	"buildmax/internal/storage/entity"

	"github.com/spf13/cobra"
)

type defaultWorkspacePaths struct{}

func (defaultWorkspacePaths) PersistentWorkspaceDir(workspaceID string) string {
	return config.PersistentWorkspaceDir(workspaceID)
}
func (defaultWorkspacePaths) RuntimeWorkspaceDir(workspaceID, taskID string) string {
	return config.RuntimeWorkspaceDir(workspaceID, taskID)
}
func (defaultWorkspacePaths) ArtifactDir(workspaceID, taskID, artifactID string) string {
	return config.ArtifactDir(workspaceID, taskID, artifactID)
}

func newServerCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Start the HTTP server (backend for portal)",
		Long:  "Start the HTTP server. Listens on port 5678 by default. Override with --port or BUILDMAX_SERVER_PORT.",
		RunE:  runServer,
	}
	cmd.Flags().Int("port", 0, "port to listen on (default: 5678 or BUILDMAX_SERVER_PORT)")
	return cmd
}

func runServer(cmd *cobra.Command, _ []string) error {
	portFlag, _ := cmd.Flags().GetInt("port")
	port, err := config.ResolveServerPort(portFlag)
	if err != nil {
		return err
	}
	dsn := config.MySQLDSN()
	serverEnv, err := config.LoadServerEnv()
	if err != nil {
		return err
	}
	ctx := context.Background()
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
	persistRoot := config.PersistentWorkspaceDir
	artifactDir := config.ArtifactDir
	persistStorage, err := config.BuildPersistStorage(wsCfg, persistRoot, s3Client)
	if err != nil {
		return fmt.Errorf("persist storage: %w", err)
	}
	artifactStorage, err := config.BuildArtifactStorage(wsCfg, artifactDir, s3Client)
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
		SessionsDir:      config.SessionsDir(),
		WorkspacesDir:    config.WorkspacesDir(),
		JWTSecret:        serverEnv.JWTSecret,
		CORSOrigin:       serverEnv.CORSOrigin,
	}
	// Start the task executor (polls for PENDING tasks and runs buildmax -p).
	runner, err := executor.New(st, st, defaultWorkspacePaths{}, persistStorage, artifactStorage)
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
