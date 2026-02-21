// Package blob provides pluggable storage for workspace persist files and artifact files.
package blob

import (
	"context"
	"io"
)

// PersistStorage reads/writes persistent workspace files (uploads, Explore) and can materialize them to a local dir.
// PutChatGlobal/GetChatGlobal use run-scoped path: chats/<chatID>/<chatRunID>/global/.
// PutChatRunArtifacts/GetChatRunArtifacts use run-scoped path: chats/<chatID>/<chatRunID>/artifacts/.
type PersistStorage interface {
	Put(ctx context.Context, workspaceID string, relPath string, r io.Reader) error
	Get(ctx context.Context, workspaceID string, relPath string) ([]byte, error)
	ListFiles(ctx context.Context, workspaceID string) ([]string, error)
	MaterializeToDir(ctx context.Context, workspaceID string, dstDir string) error
	PutChatGlobal(ctx context.Context, workspaceID, chatID, chatRunID, relPath string, r io.Reader) error
	GetChatGlobal(ctx context.Context, workspaceID, chatID, chatRunID, relPath string) ([]byte, error)
	PutChatRunArtifacts(ctx context.Context, workspaceID, chatID, chatRunID, relPath string, r io.Reader) error
	GetChatRunArtifacts(ctx context.Context, workspaceID, chatID, chatRunID, relPath string) ([]byte, error)
}

// ArtifactStorage reads/writes run output files. Path: artifacts/<chatID>/<chatRunID>/<relPath>. One namespace per chat run.
// PutResult/GetResult are for result.md. PutArtifactFile/GetArtifactFile support multiple files per run.
type ArtifactStorage interface {
	PutResult(ctx context.Context, workspaceID, chatID, chatRunID string, data []byte) error
	GetResult(ctx context.Context, workspaceID, chatID, chatRunID string) ([]byte, error)
	PutArtifactFile(ctx context.Context, workspaceID, chatID, chatRunID, relPath string, r io.Reader) error
	GetArtifactFile(ctx context.Context, workspaceID, chatID, chatRunID, relPath string) ([]byte, error)
}
