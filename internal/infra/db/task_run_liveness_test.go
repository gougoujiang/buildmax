package db

import (
	"testing"
	"time"

	coretask "github.com/gougoujiang/buildmax/internal/core/task"
)

// TestTaskRunLivenessQueries covers the pair that turns a worker's existing
// poll into an observation the reaper can act on. Both are guarded by status,
// because the caller is an HTTP handler racing the worker's own terminal report
// for the same row.
func TestTaskRunLivenessQueries(t *testing.T) {
	s, ctx := newTestStore(t)
	user := newTestUser(t, s, "liveness-store")

	conv, err := s.CreateConversation(ctx, user, "portal", user)
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	task, err := s.CreateTask(ctx, &coretask.CreateInput{
		TeamID:         conv.TeamID,
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

	transition := func(from, to coretask.RunStatus) {
		t.Helper()
		ok, err := s.TransitionTaskRun(ctx, coretask.TransitionRunInput{
			TaskRunID: runID, ExpectedStatus: from, NewStatus: to,
		})
		if err != nil || !ok {
			t.Fatalf("transition %s -> %s: ok=%v err=%v", from, to, ok, err)
		}
	}
	lost := func(cutoff time.Time) []coretask.Run {
		t.Helper()
		got, err := s.ListLostWorkerTaskRuns(ctx, cutoff, 10)
		if err != nil {
			t.Fatalf("ListLostWorkerTaskRuns: %v", err)
		}
		return got
	}

	seen := time.Unix(1_800_000_000, 0).UTC()
	transition(coretask.RunStatusPending, coretask.RunStatusScheduled)

	// A run with no recorded signal is never reaped for silence. Absence of
	// evidence is not the observation this sweep exists to make; the run
	// timeout is what covers it.
	if got := lost(seen.Add(time.Hour)); len(got) != 0 {
		t.Fatalf("lost = %+v, want none before any signal was recorded", got)
	}

	if err := s.MarkTaskRunSeen(ctx, runID, seen); err != nil {
		t.Fatalf("MarkTaskRunSeen: %v", err)
	}
	// SCHEDULED is deliberately outside the sweep: a worker that has claimed
	// its run may be materializing a workspace or pulling plugin packages
	// without touching the API, and that is not a dead process.
	if got := lost(seen.Add(time.Hour)); len(got) != 0 {
		t.Fatalf("lost = %+v, want a SCHEDULED run left to the run timeout", got)
	}

	transition(coretask.RunStatusScheduled, coretask.RunStatusRunning)
	if err := s.MarkTaskRunSeen(ctx, runID, seen); err != nil {
		t.Fatalf("MarkTaskRunSeen while running: %v", err)
	}

	// A run heard from after the cutoff is still working.
	if got := lost(seen.Add(-time.Second)); len(got) != 0 {
		t.Fatalf("lost = %+v, want none for a run seen after the cutoff", got)
	}
	got := lost(seen.Add(time.Second))
	if len(got) != 1 || got[0].ID != runID {
		t.Fatalf("lost = %+v, want the silent run %s", got, runID)
	}
	if got[0].LastSeenAt == nil || !got[0].LastSeenAt.Equal(seen) {
		t.Errorf("last_seen_at = %v, want %v", got[0].LastSeenAt, seen)
	}

	// The status guard is what makes the column mean "last seen while working":
	// the worker's terminal PATCH is itself a call scoped to this run, and
	// letting it stamp would push the timestamp past the moment work stopped.
	transition(coretask.RunStatusRunning, coretask.RunStatusSucceeded)
	if err := s.MarkTaskRunSeen(ctx, runID, seen.Add(time.Hour)); err != nil {
		t.Fatalf("MarkTaskRunSeen after terminal: %v", err)
	}
	after, err := s.GetTaskRun(ctx, runID)
	if err != nil {
		t.Fatalf("GetTaskRun: %v", err)
	}
	if after.LastSeenAt == nil || !after.LastSeenAt.Equal(seen) {
		t.Errorf("last_seen_at = %v, want it frozen at %v once the run ended", after.LastSeenAt, seen)
	}
	if got := lost(seen.Add(time.Hour)); len(got) != 0 {
		t.Errorf("lost = %+v, want a finished run left alone", got)
	}
}
