// Package entity provides user and workspace persistence (MySQL via GORM).
package entity

import (
	"context"
	"fmt"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// Store implements UserStore, WorkspaceStore, ChatStore, ChatRunStore, and ArtifactStore with a MySQL backend.
type Store struct {
	db *gorm.DB
}

// New opens a MySQL connection with the given DSN and runs AutoMigrate for User, Workspace, Agent, Chat, ChatRun, Artifact, and ArtifactItem.
// The context can be used for connection timeout; the returned Store holds the DB for the process lifetime.
// Table names are singular (user, workspace, chat). Existing DBs with plural tables require a one-time migration.
func New(ctx context.Context, dsn string) (*Store, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	if err := db.WithContext(ctx).AutoMigrate(&User{}, &Workspace{}, &Agent{}, &Chat{}, &ChatRun{}, &Artifact{}, &ArtifactItem{}); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes the underlying DB connection. Optional for server lifecycle.
func (s *Store) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
