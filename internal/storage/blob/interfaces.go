// Package blob provides pluggable storage for workspace persist files and artifact files.
package blob

import (
	"context"
	"io"
)

// PersistStorage reads/writes persistent workspace files (uploads, Explore) and can materialize them to a local dir.
// PutChatBuildmax/GetChatBuildmax use run-scoped path: chats/<chatID>/<chatRunID>/global/.
// PutChatRunArtifacts/GetChatRunArtifacts use run-scoped path: chats/<chatID>/<chatRunID>/artifacts/.
type PersistStorage interface {
	Put(ctx context.Context, workspaceID string, relPath string, r io.Reader) error
	Get(ctx context.Context, workspaceID string, relPath string) ([]byte, error)
	ListFiles(ctx context.Context, workspaceID string) ([]string, error)
	MaterializeToDir(ctx context.Context, workspaceID string, dstDir string) error
	PutChatBuildmax(ctx context.Context, workspaceID, chatID, chatRunID, relPath string, r io.Reader) error
	GetChatBuildmax(ctx context.Context, workspaceID, chatID, chatRunID, relPath string) ([]byte, error)
	PutChatRunArtifacts(ctx context.Context, workspaceID, chatID, chatRunID, relPath string, r io.Reader) error
	GetChatRunArtifacts(ctx context.Context, workspaceID, chatID, chatRunID, relPath string) ([]byte, error)
}

// ArtifactStorage reads/writes artifact files. Path: artifacts/<chatID>/<chatRunID>/<artifactID>/<relPath>.
// PutResult/GetResult are for the primary result (result.md). PutArtifactFile/GetArtifactFile support multiple files per artifact.
type ArtifactStorage interface {
	PutResult(ctx context.Context, workspaceID, chatID, chatRunID, artifactID string, data []byte) error
	GetResult(ctx context.Context, workspaceID, chatID, chatRunID, artifactID string) ([]byte, error)
	PutArtifactFile(ctx context.Context, workspaceID, chatID, chatRunID, artifactID, relPath string, r io.Reader) error
	GetArtifactFile(ctx context.Context, workspaceID, chatID, chatRunID, artifactID, relPath string) ([]byte, error)
}
