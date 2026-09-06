package db

import (
	"testing"

	coretask "github.com/gougoujiang/buildmax/internal/core/task"
)

// baseCheckpointOf reads a run's recorded workspace base as a checkpoint public
// id, or "" when the run has none. It reads through the join rather than the
// domain type so the test pins what the column holds.
func baseCheckpointOf(t *testing.T, s *Store, runPublicID string) string {
	t.Helper()
	var basePub string
	s.db.Raw(
		"SELECT c.public_id FROM task_run r JOIN workspace_checkpoint c ON c.id = r.workspace_base_checkpoint_id WHERE r.public_id = ?",
		runPublicID,
	).Scan(&basePub)
	return basePub
}

func createdByOf(t *testing.T, s *Store, taskID string) string {
	t.Helper()
	task, err := s.GetTask(t.Context(), taskID)
	if err != nil || task == nil {
		t.Fatalf("GetTask: %v", err)
	}
	return task.CreatedBy
}

// A run continuing a Task starts from the Task's committed workspace head. The
// first run has none; the head is established by its seed; the next run inherits
// it. See docs/design/task-workspace-checkpoints.md §5.2.
func TestContinueRunInheritsTheTaskWorkspaceHead(t *testing.T) {
	s, spaceID, taskID, firstRun := seedTaskForCheckpoint(t)
	ctx := t.Context()

	// The first run predates any checkpoint, so it has no base.
	if got := baseCheckpointOf(t, s, firstRun); got != "" {
		t.Errorf("first run base = %q, want none", got)
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
		TaskID: taskID, Input: "keep going", CreatedBy: createdByOf(t, s, taskID),
	})
	if err != nil {
		t.Fatalf("continue run: %v", err)
	}
	if got := baseCheckpointOf(t, s, next.ID); got != seed.ID {
		t.Errorf("continue base = %q, want the task head %q", got, seed.ID)
	}
}

// Retry repeats an attempt, so it receives the exact base the repeated run
// received — never that run's result, and never a head that advanced past it.
// See §5.3.
func TestRetryRunInheritsTheRepeatedRunsBaseNotItsResult(t *testing.T) {
	s, spaceID, taskID, firstRun := seedTaskForCheckpoint(t)
	ctx := t.Context()
	createdBy := createdByOf(t, s, taskID)

	seed, err := s.FinalizeWorkspaceCheckpoint(ctx, coretask.FinalizeCheckpointInput{
		SpaceID: spaceID, TaskID: taskID, SourceTaskRunID: firstRun,
		Kind: coretask.CheckpointKindSeed, PayloadFormat: coretask.PayloadFormatTarZstV1,
		PayloadSHA256: hex64('a'), StorageKey: "k/seed", SizeBytes: 10, UncompressedBytes: 20, EntryCount: 3,
	})
	if err != nil {
		t.Fatalf("finalize seed: %v", err)
	}
	finishRun(t, s, ctx, firstRun)

	// A run that continues from the seed and then commits a successful result,
	// which advances the head past its own base.
	run2, err := s.CreateTaskRun(ctx, coretask.CreateRunInput{TaskID: taskID, Input: "work", CreatedBy: createdBy})
	if err != nil {
		t.Fatalf("continue run: %v", err)
	}
	if got := baseCheckpointOf(t, s, run2.ID); got != seed.ID {
		t.Fatalf("run2 base = %q, want the seed %q", got, seed.ID)
	}
	result, err := s.FinalizeWorkspaceCheckpoint(ctx, coretask.FinalizeCheckpointInput{
		SpaceID: spaceID, TaskID: taskID, SourceTaskRunID: run2.ID, BaseCheckpointID: &seed.ID,
		Kind: coretask.CheckpointKindSuccessful, PayloadFormat: coretask.PayloadFormatTarZstV1,
		PayloadSHA256: hex64('b'), StorageKey: "k/result", SizeBytes: 11, UncompressedBytes: 22, EntryCount: 4,
	})
	if err != nil {
		t.Fatalf("finalize result: %v", err)
	}
	if result.ID == seed.ID {
		t.Fatal("result checkpoint should differ from the seed")
	}
	finishRun(t, s, ctx, run2.ID)

	retry, err := s.CreateTaskRun(ctx, coretask.CreateRunInput{
		TaskID: taskID, Input: "work", CreatedBy: createdBy, RetryOfTaskRunID: &run2.ID,
	})
	if err != nil {
		t.Fatalf("retry run: %v", err)
	}
	// The head is now the result, but retry must take run2's base (the seed).
	if got := baseCheckpointOf(t, s, retry.ID); got != seed.ID {
		t.Errorf("retry base = %q, want the repeated run's base %q (not its result %q)", got, seed.ID, result.ID)
	}
}
