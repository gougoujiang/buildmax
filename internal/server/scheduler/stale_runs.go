package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

// defaultRunTimeout is how long a run may sit in SCHEDULED or RUNNING before it
// is treated as abandoned.
//
// It is generous on purpose. Failing a run that is merely slow destroys real
// work, while failing one that is genuinely gone only replaces a row nobody can
// act on with one that says what happened.
const defaultRunTimeout = 6 * time.Hour

// defaultStaleSweepInterval is how often runs are checked.
//
// Abandoned runs do not need this cadence — they have already stopped
// progressing. A cancel does: someone is waiting for the run to stop, and this
// sweep is what finishes the ones no worker will report.
const defaultStaleSweepInterval = time.Minute

// defaultCancelGrace is how long a cancel request may go unreported before the
// server finishes the run itself.
//
// A worker notices a cancel on its next poll and then still has to stop its
// agent, upload what it produced, and report — so the grace has to outlast an
// orderly stop, or this would race the worker and mark runs canceled that were
// seconds from saying so themselves.
const defaultCancelGrace = 2 * time.Minute

// staleRunLimit bounds one sweep so a backlog cannot hold the loop or the
// database for an unbounded time. The next tick continues.
const staleRunLimit = 100

// StaleRunStore is the narrow store surface the reaper needs.
type StaleRunStore interface {
	ListStaleTaskRuns(ctx context.Context, cutoffUnix int64, limit int) ([]model.TaskRun, error)
	ListCancelRequestedTaskRuns(ctx context.Context, cutoffUnix int64, limit int) ([]model.TaskRun, error)
	UpdateRun(ctx context.Context, in model.UpdateTaskRunInput) error
	SyncTaskFromRun(ctx context.Context, taskRunID string) error
}

// StaleRunReaper finishes runs that nothing else will finish.
//
// Two cases, one loop. A run whose worker never reported an outcome stays
// SCHEDULED or RUNNING forever, because the only thing that moves it is the
// worker itself: that was already possible when a pod was evicted or a process
// was killed, and run tokens add one more way, since a run outliving its token
// can no longer report anything. A run someone canceled has the same problem
// for the same reason — the cancel is a request its worker honors, and a worker
// that is gone honors nothing. Either way the run is over and the record should
// say so.
//
// See docs/design/worker-run-token.md.
type StaleRunReaper struct {
	runs        StaleRunStore
	timeout     time.Duration
	cancelGrace time.Duration
	interval    time.Duration
	stopCh      chan struct{}
	doneCh      chan struct{}
}

// NewStaleRunReaper returns a reaper for runs, or nil when there is no store to
// sweep — so a caller does not need to check before starting it. Zero values
// use the defaults.
func NewStaleRunReaper(runs StaleRunStore, timeout, interval time.Duration) *StaleRunReaper {
	if runs == nil {
		return nil
	}
	if timeout <= 0 {
		timeout = defaultRunTimeout
	}
	if interval <= 0 {
		interval = defaultStaleSweepInterval
	}
	return &StaleRunReaper{
		runs:        runs,
		timeout:     timeout,
		cancelGrace: defaultCancelGrace,
		interval:    interval,
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
	}
}

// Start launches the sweep loop. Calling it on a nil reaper is a no-op.
func (c *StaleRunReaper) Start() {
	if c == nil {
		return
	}
	go c.loop()
	slog.Info("stale run reaper started", "run_timeout", c.timeout, "cancel_grace", c.cancelGrace, "interval", c.interval)
}

// Stop signals the loop to exit and blocks until it has finished.
func (c *StaleRunReaper) Stop() {
	if c == nil {
		return
	}
	close(c.stopCh)
	<-c.doneCh
	slog.Info("stale run reaper stopped")
}

func (c *StaleRunReaper) loop() {
	defer close(c.doneCh)
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.Sweep(context.Background(), time.Now())
		}
	}
}

// Sweep finishes every run the reaper is responsible for: those dispatched
// longer ago than the timeout, and those asked to stop longer ago than the
// grace. It is exported so a test can drive one pass without a clock.
func (c *StaleRunReaper) Sweep(ctx context.Context, now time.Time) {
	if c == nil {
		return
	}
	c.sweepCanceled(ctx, now)
	c.sweepAbandoned(ctx, now)
}

// sweepAbandoned fails runs whose worker never reported an outcome.
func (c *StaleRunReaper) sweepAbandoned(ctx context.Context, now time.Time) {
	stale, err := c.runs.ListStaleTaskRuns(ctx, now.Add(-c.timeout).Unix(), staleRunLimit)
	if err != nil {
		slog.Warn("stale run sweep failed", "err", err)
		return
	}
	message := "this run was abandoned: no worker reported an outcome within " + c.timeout.String()
	for _, run := range stale {
		c.finish(ctx, run, model.RunStatusFailed, message, now, "failed an abandoned task run")
	}
}

// sweepCanceled finishes runs whose cancel request nobody honored.
//
// It runs before the abandoned sweep so a run that is both canceled and
// abandoned ends as CANCELED. That is the more informative of the two answers:
// it names why the run stopped, and the person who asked is the one waiting.
func (c *StaleRunReaper) sweepCanceled(ctx context.Context, now time.Time) {
	canceled, err := c.runs.ListCancelRequestedTaskRuns(ctx, now.Add(-c.cancelGrace).Unix(), staleRunLimit)
	if err != nil {
		slog.Warn("canceled run sweep failed", "err", err)
		return
	}
	message := "this run was canceled: no worker confirmed the cancel within " + c.cancelGrace.String()
	for _, run := range canceled {
		c.finish(ctx, run, model.RunStatusCanceled, message, now, "canceled a task run whose worker never confirmed")
	}
}

// finish writes one terminal outcome and syncs the run's task.
//
// The message names what the server observed rather than guessing a cause,
// because from here a dead worker, an evicted pod, and an expired credential
// look identical.
func (c *StaleRunReaper) finish(ctx context.Context, run model.TaskRun, status model.RunStatus, message string, now time.Time, logMsg string) {
	endedAt := now.Unix()
	if err := c.runs.UpdateRun(ctx, model.UpdateTaskRunInput{
		TaskRunID:    run.TaskRunID,
		Status:       status,
		EndedAt:      &endedAt,
		ErrorMessage: &message,
	}); err != nil {
		slog.Warn("could not finish an unreported run", "task_run_id", run.TaskRunID, "status", status, "err", err)
		return
	}
	if err := c.runs.SyncTaskFromRun(ctx, run.TaskRunID); err != nil {
		slog.Warn("could not sync a task from its unreported run", "task_run_id", run.TaskRunID, "err", err)
	}
	slog.Warn(logMsg, "task_run_id", run.TaskRunID, "status_was", run.Status)
}
