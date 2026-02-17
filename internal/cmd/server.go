package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"buildmax/internal/config"
	"buildmax/internal/executor"
	"buildmax/internal/server"
	"buildmax/internal/store"

	"github.com/spf13/cobra"
)

const defaultServerPort = 5678

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
	port, err := resolveServerPort(cmd)
	if err != nil {
		return err
	}
	dsn := config.MySQLDSN()
	jwtSecret := os.Getenv("BUILDMAX_JWT_SECRET")
	if jwtSecret == "" {
		return fmt.Errorf("BUILDMAX_JWT_SECRET is required for server mode")
	}
	ctx := context.Background()
	st, err := store.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	if err := st.BackfillTaskWorkspaceID(ctx); err != nil {
		slog.Warn("backfill task workspace_id", "err", err)
	}
	corsOrigin := os.Getenv("BUILDMAX_CORS_ORIGIN")
	if corsOrigin == "" {
		corsOrigin = "http://localhost:5173"
	}
	cfg := server.Config{
		Addr:           ":" + strconv.Itoa(port),
		UserStore:      st,
		WorkspaceStore: st,
		ProjectStore:   st,
		TaskStore:      st,
		ArtifactStore:  st,
		SessionsDir:    config.SessionsDir(),
		JWTSecret:      jwtSecret,
		CORSOrigin:     corsOrigin,
	}
	// Start the task executor (polls for PENDING tasks and runs buildmax -p).
	runner := executor.New(st)
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

// resolveServerPort returns the port to use: flag --port, else BUILDMAX_SERVER_PORT, else defaultServerPort.
// Returns an error if BUILDMAX_SERVER_PORT is set but not a valid number.
func resolveServerPort(cmd *cobra.Command) (int, error) {
	port, _ := cmd.Flags().GetInt("port")
	if port > 0 {
		return port, nil
	}
	env := os.Getenv("BUILDMAX_SERVER_PORT")
	if env == "" {
		return defaultServerPort, nil
	}
	port, err := strconv.Atoi(env)
	if err != nil {
		return 0, fmt.Errorf("invalid BUILDMAX_SERVER_PORT %q: %w", env, err)
	}
	if port <= 0 {
		return 0, fmt.Errorf("BUILDMAX_SERVER_PORT must be positive, got %d", port)
	}
	return port, nil
}
