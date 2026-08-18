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

// defaultStaleSweepInterval is how often runs are checked. Nothing depends on
// the timing: the runs this finds have already stopped progressing.
const defaultStaleSweepInterval = 5 * time.Minute

// staleRunLimit bounds one sweep so a backlog cannot hold the loop or the
// database for an unbounded time. The next tick continues.
const staleRunLimit = 100

// StaleRunStore is the narrow store surface the reaper needs.
type StaleRunStore interface {
	ListStaleTaskRuns(ctx context.Context, cutoffUnix int64, limit int) ([]model.TaskRun, error)
	UpdateRun(ctx context.Context, in model.UpdateTaskRunInput) error
	SyncTaskFromRun(ctx context.Context, taskRunID string) error
}

// StaleRunReaper fails runs whose worker never reported an outcome.
//
// Without it such a run stays SCHEDULED or RUNNING forever, because the only
// thing that moves it is the worker itself. That was already possible when a
// pod was evicted or a process was killed; run tokens add one more way, since a
// run outliving its token can no longer report anything. Either way the run is
// over and the record should say so.
//
// See docs/design/worker-run-token.md.
type StaleRunReaper struct {
	runs     StaleRunStore
	timeout  time.Duration
	interval time.Duration
	stopCh   chan struct{}
	doneCh   chan struct{}
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
		runs:     runs,
		timeout:  timeout,
		interval: interval,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
}

// Start launches the sweep loop. Calling it on a nil reaper is a no-op.
func (c *StaleRunReaper) Start() {
	if c == nil {
		return
	}
	go c.loop()
	slog.Info("stale run reaper started", "run_timeout", c.timeout, "interval", c.interval)
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

// Sweep fails every run that has been dispatched for longer than the timeout.
// It is exported so a test can drive one pass without a clock.
func (c *StaleRunReaper) Sweep(ctx context.Context, now time.Time) {
	if c == nil {
		return
	}
	cutoff := now.Add(-c.timeout).Unix()
	stale, err := c.runs.ListStaleTaskRuns(ctx, cutoff, staleRunLimit)
	if err != nil {
		slog.Warn("stale run sweep failed", "err", err)
		return
	}
	for _, run := range stale {
		c.fail(ctx, run, now)
	}
}

// fail records one abandoned run. The message names the timeout rather than
// guessing a cause, because from here a dead worker, an evicted pod, and an
// expired credential look identical.
func (c *StaleRunReaper) fail(ctx context.Context, run model.TaskRun, now time.Time) {
	endedAt := now.Unix()
	message := "this run was abandoned: no worker reported an outcome within " + c.timeout.String()
	if err := c.runs.UpdateRun(ctx, model.UpdateTaskRunInput{
		TaskRunID:    run.TaskRunID,
		Status:       model.RunStatusFailed,
		EndedAt:      &endedAt,
		ErrorMessage: &message,
	}); err != nil {
		slog.Warn("could not fail an abandoned run", "task_run_id", run.TaskRunID, "err", err)
		return
	}
	if err := c.runs.SyncTaskFromRun(ctx, run.TaskRunID); err != nil {
		slog.Warn("could not sync a task from its abandoned run", "task_run_id", run.TaskRunID, "err", err)
	}
	slog.Warn("failed an abandoned task run", "task_run_id", run.TaskRunID, "status_was", run.Status)
}
