package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	coreworkflow "github.com/icloudbb/buildmax/internal/core/workflow"
)

// fakeReconciler records which runs a sweep reconciled and can be told which
// runs are due and which reconciles fail.
type fakeReconciler struct {
	mu           sync.Mutex
	due          []coreworkflow.Run
	listErr      error
	reconcileErr map[string]error
	reconciled   []string
	// signal, when set, is closed the first time runID is reconciled, so a
	// lifecycle test can wait for the startup sweep without polling the clock.
	signal chan struct{}
	runID  string
}

func (f *fakeReconciler) ListDueWorkflowRuns(_ context.Context, _ time.Time, _ int) ([]coreworkflow.Run, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	return append([]coreworkflow.Run(nil), f.due...), nil
}

func (f *fakeReconciler) Reconcile(_ context.Context, workflowRunID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reconciled = append(f.reconciled, workflowRunID)
	if f.signal != nil && workflowRunID == f.runID {
		close(f.signal)
		f.signal = nil
	}
	if f.reconcileErr != nil {
		return f.reconcileErr[workflowRunID]
	}
	return nil
}

func (f *fakeReconciler) reconciledIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.reconciled...)
}

func newLoopForTest(t *testing.T, r WorkflowReconciler) *WorkflowRecoveryLoop {
	t.Helper()
	loop, err := NewWorkflowRecoveryLoop(r, time.Hour) // long interval; tests drive sweeps directly
	if err != nil {
		t.Fatalf("NewWorkflowRecoveryLoop: %v", err)
	}
	return loop
}

// A sweep reconciles every due run, in the order the due query returned them.
func TestWorkflowRecoveryLoopSweepReconcilesDueRuns(t *testing.T) {
	f := &fakeReconciler{due: []coreworkflow.Run{{ID: "wr_1"}, {ID: "wr_2"}, {ID: "wr_3"}}}
	newLoopForTest(t, f).sweep(context.Background())

	got := f.reconciledIDs()
	want := []string{"wr_1", "wr_2", "wr_3"}
	if len(got) != len(want) {
		t.Fatalf("reconciled %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("reconciled %v, want %v", got, want)
		}
	}
}

// One run whose reconcile fails does not stop the sweep: the others still get a
// pass, so a single stuck run cannot strand the rest of the backlog.
func TestWorkflowRecoveryLoopContinuesPastAReconcileError(t *testing.T) {
	f := &fakeReconciler{
		due:          []coreworkflow.Run{{ID: "wr_1"}, {ID: "wr_2"}, {ID: "wr_3"}},
		reconcileErr: map[string]error{"wr_2": errors.New("boom")},
	}
	newLoopForTest(t, f).sweep(context.Background())

	if got := f.reconciledIDs(); len(got) != 3 {
		t.Fatalf("reconciled %v, want all three attempted despite the middle failure", got)
	}
}

// A failed due-run scan is logged and the sweep returns without reconciling
// anything, rather than panicking or stopping the loop.
func TestWorkflowRecoveryLoopSweepToleratesAListError(t *testing.T) {
	f := &fakeReconciler{listErr: errors.New("db down")}
	newLoopForTest(t, f).sweep(context.Background())
	if got := f.reconciledIDs(); len(got) != 0 {
		t.Fatalf("reconciled %v on a list error, want none", got)
	}
}

// Start sweeps immediately -- a fresh process recovers a run stranded before it
// began without waiting a full interval -- and Stop returns once the loop has
// drained.
func TestWorkflowRecoveryLoopStartSweepsThenStops(t *testing.T) {
	f := &fakeReconciler{
		due:    []coreworkflow.Run{{ID: "wr_start"}},
		signal: make(chan struct{}),
		runID:  "wr_start",
	}
	sig := f.signal
	loop := newLoopForTest(t, f)
	loop.Start()

	select {
	case <-sig:
	case <-time.After(2 * time.Second):
		t.Fatal("startup sweep did not reconcile the due run within 2s")
	}

	done := make(chan struct{})
	go func() { loop.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return within 2s")
	}
}
