// Package schema is what a database says has been done to it.
//
// It exists because two layers have to name the same fact and neither may own
// it: internal/infra/db reports the applied steps, and the system
// administration route reads them. A type in either one would make the other
// depend on it.
//
// A Migration here is a step that was applied, which is not the db.Migration
// that defines a step to apply. One is a record, the other is code with an
// Apply function; they meet only at the id.
package schema

import (
	"context"
	"time"
)

// Migration is one applied schema step.
//
// The set of applied migrations is what tells an operator whether a database
// matches the binary talking to it — the question behind "we upgraded and
// something is wrong".
type Migration struct {
	ID        string    `json:"id"`
	AppliedAt time.Time `json:"applied_at"`
}

// Store reports what has been done to the database.
type Store interface {
	// AppliedMigrations returns applied migrations, oldest first.
	AppliedMigrations(ctx context.Context) ([]Migration, error)
}
