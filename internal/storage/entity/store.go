// Package entity provides persistence for BuildMax backend entities (MySQL via GORM).
package entity

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

// New opens a MySQL connection with the given DSN and runs AutoMigrate for User, Team, TeamMember, Issue, Agent, Task, TaskRun, TaskRunArtifact, QuotaTier, Conversation, ConversationMessage, and UserWebhookKey.
// If the legacy artifact table exists, migrates data to task_run_artifact and drops artifact/artifact_item and task columns.
// If the old task_run_output_file table exists, migrates its data to task_run_artifact and drops it.
// GORM logger is configured to ignore ErrRecordNotFound so expected "not found" lookups (e.g. GetNextPendingTaskRun when idle) do not spam the console.
func New(ctx context.Context, dsn string) (*Store, error) {
	gormLogger := logger.New(log.New(os.Stdout, "\r\n", log.LstdFlags), logger.Config{
		IgnoreRecordNotFoundError: true,
	})
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: gormLogger})
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	if err := db.WithContext(ctx).AutoMigrate(&User{}, &Team{}, &TeamMember{}, &Issue{}, &Agent{}, &Task{}, &TaskRun{}, &TaskRunArtifact{}, &QuotaTier{}, &Conversation{}, &ConversationMessage{}, &UserWebhookKey{}); err != nil {
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

// migrateFromArtifactTables copies artifact_item into task_run_artifact (by task_run_id from artifact), then drops artifact and artifact_item, and drops legacy task artifact columns if present.
func migrateFromArtifactTables(ctx context.Context, db *gorm.DB) error {
	var exists int
	err := db.WithContext(ctx).Raw("SELECT 1 FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'artifact' LIMIT 1").Scan(&exists).Error
	if err != nil || exists == 0 {
		return nil
	}
	// Copy artifact_item -> task_run_artifact using artifact.task_run_id
	err = db.WithContext(ctx).Exec(`INSERT INTO task_run_artifact (task_run_id, relative_path)
		SELECT a.task_run_id, i.relative_path FROM artifact_item i INNER JOIN artifact a ON i.artifact_id = a.artifact_id
		ON DUPLICATE KEY UPDATE relative_path = VALUES(relative_path)`).Error
	if err != nil {
		return err
	}
	_ = db.WithContext(ctx).Exec("DROP TABLE IF EXISTS artifact_item").Error
	_ = db.WithContext(ctx).Exec("DROP TABLE IF EXISTS artifact").Error
	// Drop legacy columns from task/chat (ignore if already dropped)
	_ = db.WithContext(ctx).Exec("ALTER TABLE task DROP COLUMN last_artifact_id").Error
	_ = db.WithContext(ctx).Exec("ALTER TABLE task DROP COLUMN artifact_seq").Error
	_ = db.WithContext(ctx).Exec("ALTER TABLE chat DROP COLUMN last_artifact_id").Error
	_ = db.WithContext(ctx).Exec("ALTER TABLE chat DROP COLUMN artifact_seq").Error
	return nil
}

// migrateTaskRunOutputFileToArtifact copies task_run_output_file into task_run_artifact and drops the old table.
func migrateTaskRunOutputFileToArtifact(ctx context.Context, db *gorm.DB) error {
	var exists int
	err := db.WithContext(ctx).Raw("SELECT 1 FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'task_run_output_file' LIMIT 1").Scan(&exists).Error
	if err != nil || exists == 0 {
		return nil
	}
	err = db.WithContext(ctx).Exec(`INSERT INTO task_run_artifact (task_run_id, relative_path)
		SELECT task_run_id, relative_path FROM task_run_output_file`).Error
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Exec("DROP TABLE task_run_output_file").Error
}

// Close closes the underlying DB connection. Optional for server lifecycle.
func (s *Store) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
