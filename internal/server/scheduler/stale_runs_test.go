package scheduler

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

// fakeStaleStore records what the reaper did and lets a test control what it
// finds.
type fakeStaleStore struct {
	stale            []model.TaskRun
	canceled         []model.TaskRun
	listErr          error
	cancelListErr    error
	lastCutoff       int64
	lastCancelCutoff int64
	failed           []model.UpdateTaskRunInput
	synced           []string
	updateErr        error
}

func (f *fakeStaleStore) ListStaleTaskRuns(_ context.Context, cutoffUnix int64, _ int) ([]model.TaskRun, error) {
	f.lastCutoff = cutoffUnix
	return f.stale, f.listErr
}

func (f *fakeStaleStore) ListCancelRequestedTaskRuns(_ context.Context, cutoffUnix int64, _ int) ([]model.TaskRun, error) {
	f.lastCancelCutoff = cutoffUnix
	return f.canceled, f.cancelListErr
}

func (f *fakeStaleStore) UpdateRun(_ context.Context, in model.UpdateTaskRunInput) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	f.failed = append(f.failed, in)
	return nil
}

func (f *fakeStaleStore) SyncTaskFromRun(_ context.Context, taskRunID string) error {
	f.synced = append(f.synced, taskRunID)
	return nil
}

func newStaleFixture(stale ...model.TaskRun) (*fakeStaleStore, *StaleRunReaper) {
	f := &fakeStaleStore{stale: stale}
	return f, NewStaleRunReaper(f, 6*time.Hour, time.Hour)
}

// TestReaperClosesAbandonedRuns is the safety net that makes a per-run
// credential viable. Only a worker moves a run out of SCHEDULED or RUNNING, so
// a run whose worker died — or whose token expired before it could report —
// would otherwise sit there forever, showing as in-progress work that will never
// finish.
func TestReaperClosesAbandonedRuns(t *testing.T) {
	store, reaper := newStaleFixture(
		model.TaskRun{ID: "r_1", Status: string(model.RunStatusRunning)},
		model.TaskRun{ID: "r_2", Status: string(model.RunStatusScheduled)},
	)

	now := time.Unix(1_800_000_000, 0)
	reaper.Sweep(context.Background(), now)

	if len(store.failed) != 2 {
		t.Fatalf("failed %d runs, want 2", len(store.failed))
	}
	for _, in := range store.failed {
		if in.Status != model.RunStatusFailed {
			t.Errorf("run %s status = %q, want FAILED", in.TaskRunID, in.Status)
		}
		if in.EndedAt == nil || *in.EndedAt != now.Unix() {
			t.Errorf("run %s ended_at = %v, want %d", in.TaskRunID, in.EndedAt, now.Unix())
		}
		// The message names the timeout rather than guessing a cause: from the
		// server a dead worker, an evicted pod, and an expired credential are
		// indistinguishable, and inventing one would mislead whoever reads it.
		if in.ErrorMessage == nil || !strings.Contains(*in.ErrorMessage, "6h0m0s") {
			t.Errorf("run %s message = %v, want it to name the timeout", in.TaskRunID, in.ErrorMessage)
		}
	}
	// The task's denormalized status has to follow, or Portal keeps showing work
	// in progress even though the run is closed.
	if len(store.synced) != 2 {
		t.Errorf("synced %v, want both runs", store.synced)
	}
}

// TestReaperClosesUnconfirmedCancels covers the case a cooperative cancel
// cannot: the worker that was supposed to stop is gone. Without this the run
// keeps showing as in progress after someone pressed stop, which reads as the
// cancel having done nothing.
func TestReaperClosesUnconfirmedCancels(t *testing.T) {
	store := &fakeStaleStore{
		canceled: []model.TaskRun{{ID: "r_1", Status: string(model.RunStatusRunning)}},
	}
	reaper := NewStaleRunReaper(store, 6*time.Hour, time.Hour)

	now := time.Unix(1_800_000_000, 0)
	reaper.Sweep(context.Background(), now)

	if len(store.failed) != 1 {
		t.Fatalf("finished %d runs, want 1", len(store.failed))
	}
	got := store.failed[0]
	if got.Status != model.RunStatusCanceled {
		t.Errorf("status = %q, want CANCELED — a stop that was asked for is not a failure", got.Status)
	}
	if got.EndedAt == nil || *got.EndedAt != now.Unix() {
		t.Errorf("ended_at = %v, want %d", got.EndedAt, now.Unix())
	}
	if got.ErrorMessage == nil || !strings.Contains(*got.ErrorMessage, defaultCancelGrace.String()) {
		t.Errorf("message = %v, want it to name the grace period", got.ErrorMessage)
	}
	if want := now.Add(-defaultCancelGrace).Unix(); store.lastCancelCutoff != want {
		t.Errorf("cancel cutoff = %d, want %d — a run asked to stop a moment ago is still the worker's to end", store.lastCancelCutoff, want)
	}
	if len(store.synced) != 1 {
		t.Errorf("synced %v, want the canceled run", store.synced)
	}
}

// A failing cancel query must not stop the abandoned sweep: they answer
// different questions and one being unavailable is no reason to skip the other.
func TestReaperSweepsAbandonedRunsWhenTheCancelQueryFails(t *testing.T) {
	store := &fakeStaleStore{
		stale:         []model.TaskRun{{ID: "r_1", Status: string(model.RunStatusRunning)}},
		cancelListErr: errors.New("database is away"),
	}
	reaper := NewStaleRunReaper(store, 6*time.Hour, time.Hour)

	reaper.Sweep(context.Background(), time.Unix(1_800_000_000, 0))

	if len(store.failed) != 1 || store.failed[0].Status != model.RunStatusFailed {
		t.Errorf("failed = %v, want the abandoned run failed", store.failed)
	}
}

func TestReaperCutoffIsTheTimeoutAgo(t *testing.T) {
	store, reaper := newStaleFixture()
	now := time.Unix(1_800_000_000, 0)

	reaper.Sweep(context.Background(), now)

	if want := now.Add(-6 * time.Hour).Unix(); store.lastCutoff != want {
		t.Errorf("cutoff = %d, want %d", store.lastCutoff, want)
	}
}

// TestReaperSurvivesFailures keeps a sweep from taking the server down with it.
// Every run it touches has already stopped progressing, so a failed sweep costs
// nothing that the next tick cannot recover.
func TestReaperSurvivesFailures(t *testing.T) {
	t.Run("the query fails", func(t *testing.T) {
		store, reaper := newStaleFixture(model.TaskRun{ID: "r_1"})
		store.listErr = errors.New("database is away")
		reaper.Sweep(context.Background(), time.Now())
		if len(store.failed) != 0 {
			t.Error("runs were failed from a query that errored")
		}
	})

	t.Run("the update fails", func(t *testing.T) {
		store, reaper := newStaleFixture(model.TaskRun{ID: "r_1"})
		store.updateErr = errors.New("database is away")
		reaper.Sweep(context.Background(), time.Now())
		if len(store.synced) != 0 {
			t.Error("a task was synced from a run that was never failed")
		}
	})
}

// TestNilReaperIsInert covers the deployment with no store: Start, Stop, and
// Sweep all have to be safe so the caller does not need a nil check.
func TestNilReaperIsInert(t *testing.T) {
	var reaper *StaleRunReaper
	if got := NewStaleRunReaper(nil, 0, 0); got != nil {
		t.Error("a reaper was built with no store to sweep")
	}
	reaper.Start()
	reaper.Sweep(context.Background(), time.Now())
	reaper.Stop()
}

func TestReaperDefaultsAreApplied(t *testing.T) {
	store := &fakeStaleStore{}
	reaper := NewStaleRunReaper(store, 0, 0)
	if reaper.timeout != defaultRunTimeout {
		t.Errorf("timeout = %v, want %v", reaper.timeout, defaultRunTimeout)
	}
	if reaper.interval != defaultStaleSweepInterval {
		t.Errorf("interval = %v, want %v", reaper.interval, defaultStaleSweepInterval)
	}
	if reaper.cancelGrace != defaultCancelGrace {
		t.Errorf("cancel grace = %v, want %v", reaper.cancelGrace, defaultCancelGrace)
	}
}
