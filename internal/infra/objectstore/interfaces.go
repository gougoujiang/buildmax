// Package objectstore provides pluggable blob storage for space workspace files and task-run artifacts.
package objectstore

import (
	"context"
	"io"
)

type RunObjectRef struct {
	SpaceID   string
	TaskID    string
	TaskRunID string
	RelPath   string
}

// HomeStorage reads and writes persistent space home files (Put/Get/ListFiles/MaterializeToDir).
// Key space: <prefix>/<spaceID>/home/<relPath>.
type HomeStorage interface {
	Put(ctx context.Context, spaceID string, relPath string, r io.Reader) error
	Get(ctx context.Context, spaceID string, relPath string) ([]byte, error)
	ListFiles(ctx context.Context, spaceID string) ([]string, error)
	MaterializeToDir(ctx context.Context, spaceID string, dstDir string) error
}

// RunStorage reads and writes the task run global dir (BUILDMAX_HOME state).
// Key space: <spaceID>/tasks/<taskID>/<runID>/global/.
// For local-FS deployments these files live on worker disk; all methods are no-ops or return apierr.ErrNotFound.
type RunStorage interface {
	PutRunGlobal(ctx context.Context, ref RunObjectRef, r io.Reader) error
	GetRunGlobal(ctx context.Context, ref RunObjectRef) ([]byte, error)
	// DeleteRunGlobal removes one run-global file. A file that is not there is
	// not an error, so a retention sweep is idempotent across restarts. It is
	// the one run-global write the local_fs backend performs, since that backend
	// keeps these files on the server's own disk rather than in a persist root.
	DeleteRunGlobal(ctx context.Context, ref RunObjectRef) error
}

// PersistStorage is the composite interface for components that need both home-file and
// run-scoped storage (e.g. the task-run worker). Most components only need HomeStorage or RunStorage.
type PersistStorage interface {
	HomeStorage
	RunStorage
}
