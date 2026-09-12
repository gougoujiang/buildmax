package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"time"

	coreworkflow "github.com/icloudbb/buildmax/internal/core/workflow"
)

func (l *WorkflowRecoveryLoop) log() *slog.Logger { return componentLog("workflow_recovery") }

const (
	// defaultWorkflowRecoveryInterval is how often the loop re-scans for due
	// Workflow runs. It bounds how long a run stranded by a lost callback waits:
	// its worst case is this interval plus the reconcile observe interval the
	// service schedules while a step is still running.
	defaultWorkflowRecoveryInterval = 30 * time.Second
	// workflowRecoveryDueBatch bounds how many due runs one sweep reconciles, so
	// a large backlog after an outage is worked through over a few sweeps rather
	// than in one unbounded pass.
	workflowRecoveryDueBatch = 100
)

// WorkflowReconciler is the Workflow application service narrowed to the two
// calls the recovery loop makes: find runs that want a pass, and advance one.
// The loop owns no execution state -- Reconcile owns the lease, the fold, the
// dispatch, and the schedule -- so two replicas running the loop cannot
// duplicate work: the reconciliation lease and guarded transitions decide who
// advances a run, not process-local election.
type WorkflowReconciler interface {
	ListDueWorkflowRuns(ctx context.Context, now time.Time, limit int) ([]coreworkflow.Run, error)
	Reconcile(ctx context.Context, workflowRunID string) error
}

// WorkflowRecoveryLoop sweeps due Workflow runs and reconciles each, so a run
// stranded by a lost terminal callback or a Server restart is recovered from
// durable state rather than never. It sweeps once at startup and then on a
// fixed interval.
//
// A failed sweep is logged and retried on the next tick rather than stopping
// the Server, the same fail-open stance the other scheduler loops take. A run
// whose reconcile fails is left with a near-future schedule by the service, so
// it is retried on a later sweep rather than every tick.
type WorkflowRecoveryLoop struct {
	reconciler WorkflowReconciler
	interval   time.Duration
	batch      int
	// now is the clock, injectable so a test can drive due times without waiting
	// on the wall clock.
	now    func() time.Time
	stopCh chan struct{}
	doneCh chan struct{}
}

// NewWorkflowRecoveryLoop returns a loop for the given reconciler. Use 0 for the
// default poll interval.
func NewWorkflowRecoveryLoop(reconciler WorkflowReconciler, interval time.Duration) (*WorkflowRecoveryLoop, error) {
	if reconciler == nil {
		return nil, errors.New("workflow recovery loop: reconciler must not be nil")
	}
	if interval <= 0 {
		interval = defaultWorkflowRecoveryInterval
	}
	return &WorkflowRecoveryLoop{
		reconciler: reconciler,
		interval:   interval,
		batch:      workflowRecoveryDueBatch,
		now:        func() time.Time { return time.Now().UTC() },
		stopCh:     make(chan struct{}),
		doneCh:     make(chan struct{}),
	}, nil
}

// WithClock overrides the loop's clock. It exists for tests that need a run's
// scheduled time to be due without waiting on the wall clock; production uses
// the real clock.
func (l *WorkflowRecoveryLoop) WithClock(now func() time.Time) *WorkflowRecoveryLoop {
	if now != nil {
		l.now = now
	}
	return l
}

// Start launches the loop. A nil loop is a no-op so the caller does not have to
// check.
func (l *WorkflowRecoveryLoop) Start() {
	if l == nil {
		return
	}
	go l.loop()
	l.log().Info("started", "interval", l.interval)
}

// Stop signals the loop to exit and blocks until it has finished.
func (l *WorkflowRecoveryLoop) Stop() {
	if l == nil {
		return
	}
	close(l.stopCh)
	<-l.doneCh
	l.log().Info("stopped")
}

func (l *WorkflowRecoveryLoop) loop() {
	defer close(l.doneCh)
	// Sweep once immediately: a fresh process must recover runs stranded before
	// it started, not wait a full interval to notice them.
	l.sweep(context.Background())
	ticker := time.NewTicker(l.interval)
	defer ticker.Stop()
	for {
		select {
		case <-l.stopCh:
			return
		case <-ticker.C:
			l.sweep(context.Background())
		}
	}
}

// sweep reconciles every run due at the current time. Each Reconcile claims its
// own lease, so a run another replica is already advancing is skipped without
// this sweep having to know.
func (l *WorkflowRecoveryLoop) sweep(ctx context.Context) {
	now := l.now()
	due, err := l.reconciler.ListDueWorkflowRuns(ctx, now, l.batch)
	if err != nil {
		l.log().WarnContext(ctx, "list due workflow runs failed", "err", err)
		return
	}
	for i := range due {
		start := l.now()
		if err := l.reconciler.Reconcile(ctx, due[i].ID); err != nil {
			l.log().WarnContext(ctx, "reconcile failed",
				"workflow_run_id", due[i].ID, "err", err)
			continue
		}
		l.log().InfoContext(ctx, "reconciled",
			"workflow_run_id", due[i].ID, "latency_ms", l.now().Sub(start).Milliseconds())
	}
}
