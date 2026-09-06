package taskrun

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	coretask "github.com/gougoujiang/buildmax/internal/core/task"
	"github.com/gougoujiang/buildmax/internal/infra/workerclient"
	"github.com/gougoujiang/buildmax/internal/infra/wsarchive"
)

// CheckpointPayloadStore writes a captured checkpoint's immutable bytes to the
// object store, content-addressed by the Space and the payload's SHA-256. The
// worker uploads the bytes and never names their key; the server derives the one
// canonical key when it records the pointer. See
// docs/design/task-workspace-checkpoints.md §8.
type CheckpointPayloadStore interface {
	Put(ctx context.Context, spaceID, sha256hex string, src io.Reader) (string, error)
}

// Provisional checkpoint capture limits. §12.3 requires benchmark-derived
// defaults over representative source, build, and data workspaces before they
// are fixed; these are conservative stand-ins that bound an enumeration or
// expansion blow-up without cutting off an ordinary tree. wsarchive enforces the
// three dimensions it supports today; the stored-byte, per-file, and
// expansion-ratio limits §12.3 also names arrive with the codec that enforces
// them.
var checkpointLimits = wsarchive.Limits{
	MaxUncompressedBytes: 4 << 30, // 4 GiB of regular-file bytes
	MaxEntries:           500_000,
	MaxPathDepth:         64,
}

// seedWorkspaceIfFirstRun captures the freshly materialized workspace as the
// Task's seed checkpoint when this run has no base — the first run of a Task. A
// run that already has a base restores it instead, which a later slice adds;
// until then the materialized files stand in for it.
//
// A deployment without checkpoint storage (a CLI or eval run with no server to
// record the pointer) leaves Checkpoints nil and seeds nothing.
func seedWorkspaceIfFirstRun(ctx context.Context, input RunTaskInput, task *coretask.Task, run *coretask.Run, dirs runDirs) error {
	if input.Checkpoints == nil || input.WorkerAPI.BaseURL == "" || input.WorkerAPI.Token == "" {
		return nil
	}
	base, err := workerclient.GetWorkspaceBase(ctx, input.WorkerAPI, run.ID)
	if err != nil {
		return fmt.Errorf("read workspace base: %w", err)
	}
	if base != nil {
		return nil
	}
	stagingDir := filepath.Join(dirs.runDir, "checkpoint-staging")
	return captureAndFinalizeSeed(ctx, input.Checkpoints, input.WorkerAPI, stagingDir, dirs.runWorkspace, task.SpaceID, run.ID)
}

// captureAndFinalizeSeed archives workspaceDir, uploads the bytes, and records
// the pointer, in that order — the commit protocol's bytes-before-pointer rule,
// so a recorded checkpoint never points at bytes that are not there (§8). The
// archive is staged outside workspaceDir so it is never part of what it captures
// (§12.1).
func captureAndFinalizeSeed(ctx context.Context, store CheckpointPayloadStore, cfg workerclient.WorkerAPIClientConfig, stagingDir, workspaceDir, spaceID, taskRunID string) error {
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return fmt.Errorf("create checkpoint staging dir: %w", err)
	}
	tmp, err := os.CreateTemp(stagingDir, "seed-*.tar.zst")
	if err != nil {
		return fmt.Errorf("create seed archive: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	// Tee the archive through a hash so the digest covers the exact stored bytes,
	// as the commit protocol requires (§8) — wsarchive.Create does not digest.
	hash := sha256.New()
	res, createErr := wsarchive.Create(io.MultiWriter(tmp, hash), workspaceDir, checkpointLimits)
	closeErr := tmp.Close()
	if createErr != nil {
		return fmt.Errorf("capture seed archive: %w", createErr)
	}
	if closeErr != nil {
		return fmt.Errorf("finish seed archive: %w", closeErr)
	}
	info, err := os.Stat(tmpName)
	if err != nil {
		return fmt.Errorf("stat seed archive: %w", err)
	}
	sha := hex.EncodeToString(hash.Sum(nil))

	f, err := os.Open(tmpName)
	if err != nil {
		return fmt.Errorf("open seed archive: %w", err)
	}
	_, putErr := store.Put(ctx, spaceID, sha, f)
	_ = f.Close()
	if putErr != nil {
		return fmt.Errorf("upload seed payload: %w", putErr)
	}

	if _, err := workerclient.FinalizeSeedCheckpoint(ctx, cfg, taskRunID, workerclient.SeedCheckpointRequest{
		PayloadFormat:     wsarchive.PayloadFormat,
		PayloadSHA256:     sha,
		SizeBytes:         info.Size(),
		UncompressedBytes: res.UncompressedBytes,
		EntryCount:        res.EntryCount,
	}); err != nil {
		return fmt.Errorf("finalize seed checkpoint: %w", err)
	}
	return nil
}
