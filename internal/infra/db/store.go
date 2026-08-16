// Package db provides persistence for BuildMax backend entities (MySQL via GORM).
package db

import (
	"context"
	"fmt"
	"log"
	"os"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Store implements the backend store interfaces with a MySQL backend.
type Store struct {
	db *gorm.DB
}

// New opens a MySQL connection with the given DSN, runs AutoMigrate for all schema rows,
// seeds default quota tiers, and applies one-time data migrations (see migration.go).
// GORM logger is configured to ignore ErrRecordNotFound so expected "not found" lookups
// (e.g. GetNextPendingTaskRun when idle) do not spam the console.
func New(ctx context.Context, dsn string) (*Store, error) {
	gormLogger := logger.New(log.New(os.Stdout, "\r\n", log.LstdFlags), logger.Config{
		IgnoreRecordNotFoundError: true,
	})
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: gormLogger})
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	if err := db.WithContext(ctx).AutoMigrate(&userRow{}, &teamRow{}, &teamMemberRow{}, &workflowRow{}, &workflowRunRow{}, &workflowStepRunRow{}, &issueRow{}, &agentRow{}, &taskRow{}, &taskRunRow{}, &taskRunArtifactRow{}, &quotaTierRow{}, &conversationRow{}, &conversationMessageRow{}, &userWebhookKeyRow{}, &loginCodeRow{}, &llmCallRow{}, &llmModelRow{}); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	if err := (&Store{db: db}).SeedDefaultQuotaTiers(ctx); err != nil {
		return nil, fmt.Errorf("seed quota tiers: %w", err)
	}
	if err := migrateFromArtifactTables(ctx, db); err != nil {
		return nil, fmt.Errorf("migrate from artifact: %w", err)
	}
	if err := migrateTaskRunOutputFileToArtifact(ctx, db); err != nil {
		return nil, fmt.Errorf("migrate task_run_output_file to task_run_artifact: %w", err)
	}
	return &Store{db: db}, nil
}

// Ping verifies the database is reachable and the pool can hand out a
// connection. Used by the server's readiness probe, so it must stay cheap
// enough to run on every probe interval.
func (s *Store) Ping(ctx context.Context) error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

// Close closes the underlying DB connection. Optional for server lifecycle.
func (s *Store) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
