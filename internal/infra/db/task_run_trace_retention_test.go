package db

import (
	"testing"
	"time"

	coretask "github.com/icloudbb/buildmax/internal/core/task"
)

// TestTaskRunTraceRetentionQueries covers the pair a trace-retention sweep runs
// against real MySQL: find the ended runs that still point at a trace, then
// clear a run's pointer once its trace is gone. Both are here rather than in a
// unit test because they turn on a join to the task and space and on the
// nullable ended_at/trace_path columns, which only a real database proves.
func TestTaskRunTraceRetentionQueries(t *testing.T) {
	s, ctx := newTestStore(t)
	user := newTestUser(t, s, "trace-retention-store")

	conv, err := s.CreateConversation(ctx, user, "portal", user)
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	task, err := s.CreateTask(ctx, &coretask.CreateInput{
		SpaceID:        conv.SpaceID,
		ConversationID: conv.ID,
		Input:          "input",
		CreatedBy:      user,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	t.Cleanup(func() {
		_ = s.db.WithContext(ctx).Delete(&taskRunRow{}, "task_id = ?", task.ID)
		_ = s.db.WithContext(ctx).Delete(&taskRow{}, "task_id = ?", task.ID)
		_ = s.db.WithContext(ctx).Delete(&conversationRow{}, "conversation_id = ?", conv.ID)
	})
	if task.LastRunID == nil {
		t.Fatal("CreateTask should create the first run")
	}
	runID := *task.LastRunID

	// Drive the run to a terminal state that carries an end time and a trace,
	// the shape a finished run has when its trace is a retention candidate.
	ended := time.Unix(1_700_000_000, 0).UTC()
	tracePath := "sessions/s1/traces/" + runID + ".jsonl"
	for _, step := range []struct{ from, to coretask.RunStatus }{
		{coretask.RunStatusPending, coretask.RunStatusScheduled},
		{coretask.RunStatusScheduled, coretask.RunStatusRunning},
	} {
		if ok, err := s.TransitionTaskRun(ctx, coretask.TransitionRunInput{
			TaskRunID: runID, ExpectedStatus: step.from, NewStatus: step.to,
		}); err != nil || !ok {
			t.Fatalf("transition %s -> %s: ok=%v err=%v", step.from, step.to, ok, err)
		}
	}
	if ok, err := s.TransitionTaskRun(ctx, coretask.TransitionRunInput{
		TaskRunID: runID, ExpectedStatus: coretask.RunStatusRunning, NewStatus: coretask.RunStatusSucceeded,
		EndedAt: &ended, TracePath: &tracePath,
	}); err != nil || !ok {
		t.Fatalf("transition to SUCCEEDED: ok=%v err=%v", ok, err)
	}

	// A cutoff before the run ended finds nothing: the window has not reached it.
	if refs, err := s.ListTaskRunsWithExpiredTrace(ctx, ended.Add(-time.Hour), 10); err != nil {
		t.Fatalf("list before cutoff: %v", err)
	} else if len(refs) != 0 {
		t.Fatalf("want no expired trace before the cutoff, got %+v", refs)
	}

	// A cutoff after it returns the run with the coordinates a backend needs.
	refs, err := s.ListTaskRunsWithExpiredTrace(ctx, ended.Add(time.Hour), 10)
	if err != nil {
		t.Fatalf("list after cutoff: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("want one expired trace, got %d", len(refs))
	}
	got := refs[0]
	if got.TaskRunID != runID || got.TaskID != task.ID || got.SpaceID != conv.SpaceID || got.TracePath != tracePath {
		t.Fatalf("ref = %+v, want run %s task %s space %s trace %s",
			got, runID, task.ID, conv.SpaceID, tracePath)
	}

	// Clearing the pointer takes the run out of the candidate set and leaves it
	// reporting no trace rather than one that would fail to load.
	if err := s.ClearTaskRunTracePath(ctx, runID); err != nil {
		t.Fatalf("ClearTaskRunTracePath: %v", err)
	}
	if refs, err := s.ListTaskRunsWithExpiredTrace(ctx, ended.Add(time.Hour), 10); err != nil {
		t.Fatalf("list after clear: %v", err)
	} else if len(refs) != 0 {
		t.Fatalf("a cleared run must not be a candidate, got %+v", refs)
	}
	run, err := s.GetTaskRun(ctx, runID)
	if err != nil {
		t.Fatalf("GetTaskRun: %v", err)
	}
	if run.TracePath != nil {
		t.Errorf("trace pointer must be nil after clear, got %q", *run.TracePath)
	}

	// Clearing again is not an error: the sweep only needs the pointer empty.
	if err := s.ClearTaskRunTracePath(ctx, runID); err != nil {
		t.Errorf("second clear must be a no-op, got %v", err)
	}
}
