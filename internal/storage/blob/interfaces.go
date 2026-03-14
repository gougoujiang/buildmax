// Package blob provides pluggable storage for workspace persist files and artifact files.
package blob

import (
	"context"
	"io"
)

type RunObjectRef struct {
	WorkspaceID string
	ChatID      string
	ChatRunID   string
	RelPath     string
}

type RunRef struct {
	WorkspaceID string
	ChatID      string
	ChatRunID   string
}

// PersistStorage reads/writes persistent workspace files (uploads, Explore) and can materialize them to a local dir.
// PutChatGlobal/GetChatGlobal use run-scoped path: chats/<chatID>/<chatRunID>/global/.
// PutChatRunArtifacts/GetChatRunArtifacts use run-scoped path: chats/<chatID>/<chatRunID>/artifacts/.
type PersistStorage interface {
	Put(ctx context.Context, workspaceID string, relPath string, r io.Reader) error
	Get(ctx context.Context, workspaceID string, relPath string) ([]byte, error)
	ListFiles(ctx context.Context, workspaceID string) ([]string, error)
	MaterializeToDir(ctx context.Context, workspaceID string, dstDir string) error
	PutChatGlobal(ctx context.Context, ref RunObjectRef, r io.Reader) error
	GetChatGlobal(ctx context.Context, ref RunObjectRef) ([]byte, error)
	PutChatRunArtifacts(ctx context.Context, ref RunObjectRef, r io.Reader) error
	GetChatRunArtifacts(ctx context.Context, ref RunObjectRef) ([]byte, error)
}

// ArtifactStorage reads/writes run output files. Path: artifacts/<chatID>/<chatRunID>/<relPath>. One namespace per chat run.
// PutResult/GetResult are for result.md. PutArtifactFile/GetArtifactFile support multiple files per run.
type ArtifactStorage interface {
	PutResult(ctx context.Context, ref RunRef, data []byte) error
	GetResult(ctx context.Context, ref RunRef) ([]byte, error)
	PutArtifactFile(ctx context.Context, ref RunObjectRef, r io.Reader) error
	GetArtifactFile(ctx context.Context, ref RunObjectRef) ([]byte, error)
}
