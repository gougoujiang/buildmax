package db

import (
	"testing"

	coretask "github.com/icloudbb/buildmax/internal/core/task"
)

// GetRunWorkspaceBase returns the checkpoint a run may restore from, and nil for
// a first run that has none.
func TestGetRunWorkspaceBaseReturnsTheRunsBase(t *testing.T) {
	s, spaceID, taskID, firstRun := seedTaskForCheckpoint(t)
	ctx := t.Context()

	// A first run has no base.
	if base, err := s.GetRunWorkspaceBase(ctx, firstRun); err != nil || base != nil {
		t.Fatalf("first run base = (%v, %v), want (nil, nil)", base, err)
	}

	seed, err := s.FinalizeWorkspaceCheckpoint(ctx, coretask.FinalizeCheckpointInput{
		SpaceID: spaceID, TaskID: taskID, SourceTaskRunID: firstRun,
		Kind: coretask.CheckpointKindSeed, PayloadFormat: coretask.PayloadFormatTarZstV1,
		PayloadSHA256: hex64('a'), StorageKey: "k/seed", SizeBytes: 10, UncompressedBytes: 20, EntryCount: 3,
	})
	if err != nil {
		t.Fatalf("finalize seed: %v", err)
	}
	finishRun(t, s, ctx, firstRun)

	next, err := s.CreateTaskRun(ctx, coretask.CreateRunInput{
		TaskID: taskID, Input: "go on", CreatedBy: createdByOf(t, s, taskID),
	})
	if err != nil {
		t.Fatalf("continue run: %v", err)
	}
	base, err := s.GetRunWorkspaceBase(ctx, next.ID)
	if err != nil {
		t.Fatalf("GetRunWorkspaceBase: %v", err)
	}
	if base == nil || base.ID != seed.ID {
		t.Fatalf("base = %v, want the seed %q", base, seed.ID)
	}
	// The descriptor carries what the worker needs to fetch and verify the bytes.
	if base.PayloadSHA256 != hex64('a') || base.PayloadFormat != coretask.PayloadFormatTarZstV1 || base.EntryCount != 3 {
		t.Errorf("base descriptor = %+v", base)
	}
}

// RecordWorkspaceRestore writes the run's restore outcome, and reports not-found
// for a run that does not exist.
func TestRecordWorkspaceRestore(t *testing.T) {
	s, _, _, firstRun := seedTaskForCheckpoint(t)
	ctx := t.Context()

	msg := "extract: tar was truncated"
	if err := s.RecordWorkspaceRestore(ctx, firstRun, coretask.WorkspaceRestoreFailed, &msg); err != nil {
		t.Fatalf("RecordWorkspaceRestore: %v", err)
	}
	var got struct {
		Status string
		Err    string
	}
	s.db.WithContext(ctx).Raw(
		"SELECT workspace_restore_status AS status, COALESCE(workspace_restore_error, '') AS err FROM task_run WHERE public_id = ?", firstRun,
	).Scan(&got)
	if got.Status != string(coretask.WorkspaceRestoreFailed) || got.Err != msg {
		t.Errorf("recorded status=%q error=%q", got.Status, got.Err)
	}

	if err := s.RecordWorkspaceRestore(ctx, "nonexistent-run", coretask.WorkspaceRestoreRestored, nil); err == nil {
		t.Error("recording against a missing run did not error")
	}
}
