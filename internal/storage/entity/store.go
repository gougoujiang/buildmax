// Package entity provides user and workspace persistence (MySQL via GORM).
package entity

import (
	"context"
	"fmt"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// Store implements UserStore, WorkspaceStore, ChatStore, and ChatRunStore with a MySQL backend.
type Store struct {
	db *gorm.DB
}

// New opens a MySQL connection with the given DSN and runs AutoMigrate for User, Workspace, Agent, Chat, ChatRun, and ChatRunOutputFile.
// If the legacy artifact table exists, migrates data to chat_run_output_file and drops artifact/artifact_item and chat columns.
func New(ctx context.Context, dsn string) (*Store, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	if err := db.WithContext(ctx).AutoMigrate(&User{}, &Workspace{}, &Agent{}, &Chat{}, &ChatRun{}, &ChatRunOutputFile{}); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	if err := migrateFromArtifactTables(ctx, db); err != nil {
		return nil, fmt.Errorf("migrate from artifact: %w", err)
	}
	return &Store{db: db}, nil
}

// migrateFromArtifactTables copies artifact_item into chat_run_output_file (by chat_run_id from artifact), then drops artifact and artifact_item, and drops chat.last_artifact_id and chat.artifact_seq if present.
func migrateFromArtifactTables(ctx context.Context, db *gorm.DB) error {
	var exists int
	err := db.WithContext(ctx).Raw("SELECT 1 FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'artifact' LIMIT 1").Scan(&exists).Error
	if err != nil || exists == 0 {
		return nil
	}
	// Copy artifact_item -> chat_run_output_file using artifact.chat_run_id
	err = db.WithContext(ctx).Exec(`INSERT INTO chat_run_output_file (chat_run_id, relative_path)
		SELECT a.chat_run_id, i.relative_path FROM artifact_item i INNER JOIN artifact a ON i.artifact_id = a.artifact_id
		ON DUPLICATE KEY UPDATE relative_path = VALUES(relative_path)`).Error
	if err != nil {
		return err
	}
	_ = db.WithContext(ctx).Exec("DROP TABLE IF EXISTS artifact_item").Error
	_ = db.WithContext(ctx).Exec("DROP TABLE IF EXISTS artifact").Error
	// Drop legacy columns from chat (ignore if already dropped)
	_ = db.WithContext(ctx).Exec("ALTER TABLE chat DROP COLUMN last_artifact_id").Error
	_ = db.WithContext(ctx).Exec("ALTER TABLE chat DROP COLUMN artifact_seq").Error
	return nil
}

// Close closes the underlying DB connection. Optional for server lifecycle.
func (s *Store) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
