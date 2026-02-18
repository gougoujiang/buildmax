// Package blob provides pluggable storage for workspace persist files and artifact files.
package blob

import (
	"context"
	"io"
)

// PersistStorage reads/writes persistent workspace files (uploads, Explore) and can materialize them to a local dir.
// PutTaskBuildmax writes a task buildmax file (logs, sessions, settings) under tasks/<taskID>/buildmax/; local_fs impl is no-op.
// GetTaskBuildmax reads a task buildmax file; local_fs returns ErrNotFound (caller may fall back to local path).
type PersistStorage interface {
	Put(ctx context.Context, workspaceID string, relPath string, r io.Reader) error
	Get(ctx context.Context, workspaceID string, relPath string) ([]byte, error)
	ListFiles(ctx context.Context, workspaceID string) ([]string, error)
	MaterializeToDir(ctx context.Context, workspaceID string, dstDir string) error
	PutTaskBuildmax(ctx context.Context, workspaceID, taskID, relPath string, r io.Reader) error
	GetTaskBuildmax(ctx context.Context, workspaceID, taskID, relPath string) ([]byte, error)
}

// ArtifactStorage reads/writes artifact result files (e.g. result.md).
type ArtifactStorage interface {
	PutResult(ctx context.Context, workspaceID, taskID, artifactID string, data []byte) error
	GetResult(ctx context.Context, workspaceID, taskID, artifactID string) ([]byte, error)
}
