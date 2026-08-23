package model

import (
	"context"
	"time"
)

// SchemaMigration is one applied schema step.
//
// The set of applied migrations is what tells an operator whether a database
// matches the binary talking to it — the question behind "we upgraded and
// something is wrong".
type SchemaMigration struct {
	ID        string    `json:"id"`
	AppliedAt time.Time `json:"applied_at"`
}

// SchemaStore reports what has been done to the database.
type SchemaStore interface {
	// AppliedMigrations returns applied migrations, oldest first.
	AppliedMigrations(ctx context.Context) ([]SchemaMigration, error)
}
