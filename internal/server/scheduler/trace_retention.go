package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	coreaudit "github.com/icloudbb/buildmax/internal/core/audit"
	coretask "github.com/icloudbb/buildmax/internal/core/task"
	blob "github.com/icloudbb/buildmax/internal/infra/objectstore"
)

// defaultTraceSweepInterval mirrors the audit sweep: retention is measured in
// days, so nothing depends on this being prompt, but hourly means a restart or a
// freshly set window takes effect within the hour rather than the day.
const defaultTraceSweepInterval = time.Hour

// traceSweepBatch bounds how many runs one query pulls, and traceSweepMaxBatches
// how many such queries one sweep runs, so turning retention on against a large
// backlog is spread across ticks instead of held in one pass.
const (
	traceSweepBatch      = 500
	traceSweepMaxBatches = 20
)

// traceRetentionStore is the run-side of a trace prune: find the runs whose
// trace has expired, and clear a run's pointer once its trace is gone.
type traceRetentionStore interface {
	ListTaskRunsWithExpiredTrace(ctx context.Context, cutoff time.Time, limit int) ([]coretask.RunTraceRef, error)
	ClearTaskRunTracePath(ctx context.Context, taskRunID string) error
}

// TraceRetainer expires the durable trace of a run that ended longer ago than
// the configured window.
//
// It removes two things per run and in this order: the trace object, then the
// run's pointer to it. Doing it that way means a crash between the two leaves a
// run pointing at an object that is already gone, which the next sweep finds and
// finishes -- the reverse would leave an object no run names, which nothing ever
// would. Every sweep that removed anything records a TracesPruned audit event,
// so a trace that is missing by policy is distinguishable from one that was
// lost.
type TraceRetainer struct {
	store    traceRetentionStore
	storage  blob.RunStorage
	writer   coreaudit.Writer
	window   time.Duration
	interval time.Duration
	stopCh   chan struct{}
	doneCh   chan struct{}
	now      func() time.Time
}

func (r *TraceRetainer) log() *slog.Logger { return componentLog("trace_retention") }

// NewTraceRetainer returns a retainer, or nil when nothing should be removed.
//
// A nil store or storage, or a window of zero, returns nil: keeping every trace
// is the default, and a deployment that has not chosen a retention policy must
// not get one by accident. Use 0 for the default interval.
func NewTraceRetainer(store traceRetentionStore, storage blob.RunStorage, writer coreaudit.Writer, retentionDays int, interval time.Duration) *TraceRetainer {
	if store == nil || storage == nil || retentionDays <= 0 {
		return nil
	}
	if interval <= 0 {
		interval = defaultTraceSweepInterval
	}
	return &TraceRetainer{
		store:    store,
		storage:  storage,
		writer:   writer,
		window:   time.Duration(retentionDays) * 24 * time.Hour,
		interval: interval,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
		now:      time.Now,
	}
}

// Start launches the sweep loop. Calling it on a nil retainer is a no-op, so a
// deployment that keeps everything needs no branch at the call site.
func (r *TraceRetainer) Start() {
	if r == nil {
		return
	}
	go r.loop()
	r.log().Info("started", "window", r.window, "interval", r.interval)
}

// Stop signals the loop to exit and blocks until it has finished.
func (r *TraceRetainer) Stop() {
	if r == nil {
		return
	}
	close(r.stopCh)
	<-r.doneCh
	r.log().Info("stopped")
}

func (r *TraceRetainer) loop() {
	defer close(r.doneCh)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	r.sweep(context.Background())
	for {
		select {
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.sweep(context.Background())
		}
	}
}

// sweep removes the traces the window has expired and records that it did.
//
// It returns how many traces went, which is what the tests assert on; the loop
// ignores it. A single object that will not delete is logged and left with its
// pointer intact, so the next sweep retries it rather than the whole sweep
// stalling on one run.
func (r *TraceRetainer) sweep(ctx context.Context) int64 {
	if r == nil {
		return 0
	}
	cutoff := r.now().Add(-r.window)

	var removed int64
	for range traceSweepMaxBatches {
		refs, err := r.store.ListTaskRunsWithExpiredTrace(ctx, cutoff, traceSweepBatch)
		if err != nil {
			r.log().WarnContext(ctx, "list expired traces failed", "err", err, "cutoff", cutoff, "removed", removed)
			break
		}
		if len(refs) == 0 {
			break
		}
		progressed := 0
		for _, ref := range refs {
			if err := r.storage.DeleteRunGlobal(ctx, blob.RunObjectRef{
				SpaceID:   ref.SpaceID,
				TaskID:    ref.TaskID,
				TaskRunID: ref.TaskRunID,
				RelPath:   ref.TracePath,
			}); err != nil {
				r.log().WarnContext(ctx, "delete trace failed", "err", err, "task_run", ref.TaskRunID)
				continue
			}
			if err := r.store.ClearTaskRunTracePath(ctx, ref.TaskRunID); err != nil {
				r.log().WarnContext(ctx, "clear trace pointer failed", "err", err, "task_run", ref.TaskRunID)
				continue
			}
			removed++
			progressed++
		}
		// Every run in the batch failed to advance. Retrying immediately would
		// spin the loop over the same rows, so stop and let the next tick try.
		if progressed == 0 {
			break
		}
	}
	if removed == 0 {
		return 0
	}

	r.log().InfoContext(ctx, "traces expired", "removed", removed, "cutoff", cutoff, "window", r.window)
	r.recordPrune(ctx, removed, cutoff)
	return removed
}

// recordPrune writes the event that says a missing trace is policy, not loss.
func (r *TraceRetainer) recordPrune(ctx context.Context, removed int64, cutoff time.Time) {
	if r.writer == nil {
		return
	}
	detail := fmt.Sprintf("%d traces for runs ended on or before %s", removed, cutoff.UTC().Format(time.RFC3339))
	if err := r.writer.RecordAuditEvent(ctx, coreaudit.Event{
		ActorType:  coreaudit.ActorSystem,
		ActorID:    coreaudit.ActorOperator,
		Action:     coreaudit.TracesPruned,
		TargetType: "trace",
		Detail:     detail,
	}); err != nil {
		// The deletions already happened; a dropped record here cannot undo
		// them, so it lands in the log rather than failing the sweep.
		r.log().ErrorContext(ctx, "trace prune not recorded", "err", err, "removed", removed, "cutoff", cutoff)
	}
}
