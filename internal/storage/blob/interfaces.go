// Package blob provides pluggable storage for workspace persist files and artifact files.
package blob

import (
	"context"
	"io"
)

// PersistStorage reads/writes persistent workspace files (uploads, Explore) and can materialize them to a local dir.
type PersistStorage interface {
	Put(ctx context.Context, workspaceID string, relPath string, r io.Reader) error
	Get(ctx context.Context, workspaceID string, relPath string) ([]byte, error)
	ListFiles(ctx context.Context, workspaceID string) ([]string, error)
	MaterializeToDir(ctx context.Context, workspaceID string, dstDir string) error
}

// ArtifactStorage reads/writes artifact result files (e.g. result.md).
type ArtifactStorage interface {
	PutResult(ctx context.Context, workspaceID, taskID, artifactID string, data []byte) error
	GetResult(ctx context.Context, workspaceID, taskID, artifactID string) ([]byte, error)
}
