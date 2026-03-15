// Package blob provides pluggable storage for user files and task-run artifact files.
package blob

import (
	"context"
	"io"
)

type RunObjectRef struct {
	UserID         string
	ConversationID string
	TaskID         string
	TaskRunID      string
	RelPath        string
}

type RunRef struct {
	UserID         string
	ConversationID string
	TaskID         string
	TaskRunID      string
}

// PersistStorage reads/writes persistent user files and can materialize them to a local dir.
// PutTaskGlobal/GetTaskGlobal use run-scoped path: conversations/<conversationID>/tasks/<taskID>/<taskRunID>/global/.
// PutTaskRunArtifacts/GetTaskRunArtifacts use run-scoped path: conversations/<conversationID>/tasks/<taskID>/<taskRunID>/artifacts/.
type PersistStorage interface {
	Put(ctx context.Context, userID string, relPath string, r io.Reader) error
	Get(ctx context.Context, userID string, relPath string) ([]byte, error)
	ListFiles(ctx context.Context, userID string) ([]string, error)
	MaterializeToDir(ctx context.Context, userID string, dstDir string) error
	PutTaskGlobal(ctx context.Context, ref RunObjectRef, r io.Reader) error
	GetTaskGlobal(ctx context.Context, ref RunObjectRef) ([]byte, error)
	PutTaskRunArtifacts(ctx context.Context, ref RunObjectRef, r io.Reader) error
	GetTaskRunArtifacts(ctx context.Context, ref RunObjectRef) ([]byte, error)
}

// ArtifactStorage reads/writes run output files. Path: artifacts/<conversationID>/<taskID>/<taskRunID>/<relPath>. One namespace per task run.
// PutResult/GetResult are for result.md. PutArtifactFile/GetArtifactFile support multiple files per run.
type ArtifactStorage interface {
	PutResult(ctx context.Context, ref RunRef, data []byte) error
	GetResult(ctx context.Context, ref RunRef) ([]byte, error)
	PutArtifactFile(ctx context.Context, ref RunObjectRef, r io.Reader) error
	GetArtifactFile(ctx context.Context, ref RunObjectRef) ([]byte, error)
}
