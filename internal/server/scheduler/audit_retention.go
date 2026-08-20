package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

// defaultAuditSweepInterval is how often the retention window is applied.
//
// Retention is measured in days, so nothing depends on this being prompt. It is
// hourly rather than daily so that a server restarted more often than once a
// day still applies the policy, and so an operator who sets a window does not
// wait until tomorrow to see it take effect.
const defaultAuditSweepInterval = time.Hour

// auditPruneBatch bounds one delete. A deployment that turns retention on for
// the first time may have years of events past the cutoff, and removing them in
// one statement would hold the table while it ran. The sweep repeats until a
// batch comes back short.
const auditPruneBatch = 500

// auditPruneMaxBatches bounds one sweep, so clearing a large backlog is spread
// over several ticks instead of occupying the loop. What is left is removed on
// the next one.
const auditPruneMaxBatches = 20

// AuditRetainer expires audit events older than the configured window.
//
// It is the only thing in the system that removes a governance record, and it
// leaves one behind when it does: every sweep that deleted anything writes an
// AuditEventsPruned event naming the cutoff and the count. That event is what
// distinguishes a trail shortened by policy from a trail somebody truncated —
// without it, the two look identical to a reader, and the wrong one of them is
// the kind of thing an audit trail exists to make visible.
type AuditRetainer struct {
	store    model.AuditPruneStore
	writer   model.AuditWriter
	window   time.Duration
	interval time.Duration
	stopCh   chan struct{}
	doneCh   chan struct{}
	now      func() time.Time
}

// NewAuditRetainer returns a retainer, or nil when nothing should be removed.
//
// A nil store or a window of zero returns nil: keeping every event is the
// default, and a deployment that has not chosen a retention policy must not get
// one by accident. Use 0 for the default interval.
func NewAuditRetainer(store model.AuditPruneStore, writer model.AuditWriter, retentionDays int, interval time.Duration) *AuditRetainer {
	if store == nil || retentionDays <= 0 {
		return nil
	}
	if interval <= 0 {
		interval = defaultAuditSweepInterval
	}
	return &AuditRetainer{
		store:    store,
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
func (a *AuditRetainer) Start() {
	if a == nil {
		return
	}
	go a.loop()
	a.log().Info("started", "window", a.window, "interval", a.interval)
}

// Stop signals the loop to exit and blocks until it has finished.
func (a *AuditRetainer) Stop() {
	if a == nil {
		return
	}
	close(a.stopCh)
	<-a.doneCh
	a.log().Info("stopped")
}

func (a *AuditRetainer) loop() {
	defer close(a.doneCh)
	ticker := time.NewTicker(a.interval)
	defer ticker.Stop()

	a.sweep(context.Background())
	for {
		select {
		case <-a.stopCh:
			return
		case <-ticker.C:
			a.sweep(context.Background())
		}
	}
}

// sweep removes what the window has expired and records that it did.
//
// It returns how many events went, which is what the tests assert on; the
// caller in the loop ignores it.
func (a *AuditRetainer) sweep(ctx context.Context) int64 {
	if a == nil {
		return 0
	}
	cutoff := a.now().Add(-a.window).Unix()

	// Read the oldest timestamp before deleting, so the recorded event can say
	// what the sweep actually removed rather than only what it was permitted
	// to. A window of 90 days on a deployment three days old removes nothing,
	// and saying "events before <cutoff>" there would describe a deletion that
	// did not happen.
	oldest, err := a.store.OldestAuditEventAt(ctx)
	if err != nil {
		a.log().WarnContext(ctx, "read oldest event failed", "err", err)
		// Not fatal: the count below is still exact, and only the message is
		// poorer for it.
		oldest = 0
	}

	var removed int64
	for range auditPruneMaxBatches {
		n, err := a.store.PruneAuditEvents(ctx, cutoff, auditPruneBatch)
		if err != nil {
			a.log().WarnContext(ctx, "prune failed", "err", err, "cutoff", cutoff, "removed", removed)
			break
		}
		removed += n
		if n < auditPruneBatch {
			break
		}
	}
	if removed == 0 {
		return 0
	}

	a.log().InfoContext(ctx, "events expired", "removed", removed, "cutoff", cutoff, "window", a.window)
	a.recordPrune(ctx, removed, oldest, cutoff)
	return removed
}

// recordPrune writes the event that says a gap in the trail is policy.
//
// Recording a deletion into the table it deleted from is deliberate. The event
// is younger than the cutoff that produced it, so it survives its own sweep,
// and by the time the window moves past it a later sweep has said the same
// thing about a later stretch. What a reader must never find is a trail that
// silently starts partway through.
func (a *AuditRetainer) recordPrune(ctx context.Context, removed, oldest, cutoff int64) {
	if a.writer == nil {
		return
	}
	detail := fmt.Sprintf("%d events before %s", removed, time.Unix(cutoff, 0).UTC().Format(time.RFC3339))
	if oldest > 0 {
		detail = fmt.Sprintf("%d events from %s to %s",
			removed,
			time.Unix(oldest, 0).UTC().Format(time.RFC3339),
			time.Unix(cutoff, 0).UTC().Format(time.RFC3339),
		)
	}
	if err := a.writer.RecordAuditEvent(ctx, model.AuditEvent{
		ActorType:  model.AuditActorSystem,
		ActorID:    model.AuditActorOperator,
		Action:     model.AuditEventsPruned,
		TargetType: "audit_event",
		Detail:     detail,
	}); err != nil {
		// A dropped record here is worse than most: it is the one that
		// explains the gap. It cannot fail the deletion, which already
		// happened, so the log is where it lands.
		a.log().ErrorContext(ctx, "prune not recorded", "err", err, "removed", removed, "cutoff", cutoff)
	}
}
