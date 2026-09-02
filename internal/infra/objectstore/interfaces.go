// Package objectstore provides pluggable blob storage for team workspace files and task-run artifacts.
package objectstore

import (
	"context"
	"io"
)

type RunObjectRef struct {
	TeamID    string
	TaskID    string
	TaskRunID string
	RelPath   string
}

type RunRef struct {
	TeamID    string
	TaskID    string
	TaskRunID string
}

// HomeStorage reads and writes persistent team home files (Put/Get/ListFiles/MaterializeToDir).
// Key space: <prefix>/<teamID>/home/<relPath>.
type HomeStorage interface {
	Put(ctx context.Context, teamID string, relPath string, r io.Reader) error
	Get(ctx context.Context, teamID string, relPath string) ([]byte, error)
	ListFiles(ctx context.Context, teamID string) ([]string, error)
	MaterializeToDir(ctx context.Context, teamID string, dstDir string) error
}

// RunStorage reads and writes run-scoped files: the task run global dir (BUILDMAX_HOME state)
// and the task run artifacts dir. Key space: <teamID>/tasks/<taskID>/<runID>/{global,artifacts}/.
// For local-FS deployments these files live on worker disk; all methods are no-ops or return apierr.ErrNotFound.
type RunStorage interface {
	PutRunGlobal(ctx context.Context, ref RunObjectRef, r io.Reader) error
	GetRunGlobal(ctx context.Context, ref RunObjectRef) ([]byte, error)
	PutRunArtifacts(ctx context.Context, ref RunObjectRef, r io.Reader) error
	GetRunArtifacts(ctx context.Context, ref RunObjectRef) ([]byte, error)
}

// PersistStorage is the composite interface for components that need both home-file and
// run-scoped storage (e.g. the task-run worker). Most components only need HomeStorage or RunStorage.
type PersistStorage interface {
	HomeStorage
	RunStorage
}

// RunOutputStorage reads/writes run output files in the task run's team-owned
// artifact namespace. One namespace exists per task run.
// PutResult/GetResult are for result.md. PutRunOutputFile/GetRunOutputFile support multiple files per run.
type RunOutputStorage interface {
	PutResult(ctx context.Context, ref RunRef, data []byte) error
	GetResult(ctx context.Context, ref RunRef) ([]byte, error)
	PutRunOutputFile(ctx context.Context, ref RunObjectRef, r io.Reader) error
	GetRunOutputFile(ctx context.Context, ref RunObjectRef) ([]byte, error)
}
