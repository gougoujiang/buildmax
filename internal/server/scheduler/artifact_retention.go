package scheduler

import (
	"context"
	"fmt"
	"time"

	coreartifact "github.com/gougoujiang/buildmax/internal/core/artifact"
	coreaudit "github.com/gougoujiang/buildmax/internal/core/audit"
)

// defaultArtifactSweepInterval is how often retention is applied.
//
// Hourly, matching the audit retainer: the grace period is measured in days, so
// nothing depends on this being prompt, and a server restarted more often than
// once a day still applies the policy. A deployment leaving the grace period at
// zero gets "reclaimed within the hour", not "reclaimed on the request" — the
// tombstone is what takes effect immediately, and it already has.
const defaultArtifactSweepInterval = time.Hour

// artifactSweepBatch bounds one pass over each phase. A deployment turning this
// on for the first time may have every artifact it ever deleted still holding
// bytes, and taking them in one statement would hold the table while it ran.
const artifactSweepBatch = 200

// ArtifactContentRemover reclaims one artifact's object.
//
// Declared here rather than taken from the artifact service so the retainer
// depends on the one operation it performs. Removal is idempotent by contract,
// which is what lets a sweep interrupted between the removal and the record
// simply run again.
type ArtifactContentRemover interface {
	RemoveArtifact(ctx context.Context, ref coreartifact.Ref) error
}

// ArtifactRetainer applies an artifact's expiry and reclaims the objects of
// artifacts that are already tombstoned.
//
// It is the only thing in the system that removes artifact content outside the
// upload-rollback path, and the only reader of ExpiresAt. Two phases, in order:
// expiry tombstones what has run out, then the purge reclaims what tombstones
// have released, so a file that expires on this tick has its bytes taken on the
// next one at the earliest — and only once the grace period has passed.
type ArtifactRetainer struct {
	store   coreartifact.PurgeStore
	content ArtifactContentRemover
	writer  coreaudit.Writer
	// grace delays reclaiming an object after its tombstone. Zero reclaims at
	// the next sweep, which is the default: the artifact is already gone at the
	// authorization boundary, and holding the bytes after that is cost and
	// exposure rather than safety.
	grace    time.Duration
	interval time.Duration
	stopCh   chan struct{}
	doneCh   chan struct{}
	now      func() time.Time
}

// NewArtifactRetainer returns a retainer, or nil when it could do nothing.
//
// Unlike the audit retainer there is no window that switches it off: an
// artifact whose bytes are never reclaimed is a leak rather than a retained
// record, so the sweep always runs where both halves are present. Use 0 for the
// default interval.
func NewArtifactRetainer(store coreartifact.PurgeStore, content ArtifactContentRemover, writer coreaudit.Writer, graceDays int, interval time.Duration) *ArtifactRetainer {
	if store == nil || content == nil {
		return nil
	}
	if interval <= 0 {
		interval = defaultArtifactSweepInterval
	}
	grace := time.Duration(0)
	if graceDays > 0 {
		grace = time.Duration(graceDays) * 24 * time.Hour
	}
	return &ArtifactRetainer{
		store:    store,
		content:  content,
		writer:   writer,
		grace:    grace,
		interval: interval,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
		now:      time.Now,
	}
}

// Start launches the sweep loop. Calling it on a nil retainer is a no-op, so a
// deployment without artifact storage needs no branch at the call site.
func (a *ArtifactRetainer) Start() {
	if a == nil {
		return
	}
	go a.loop()
	a.log().Info("started", "grace", a.grace, "interval", a.interval)
}

// Stop signals the loop to exit and blocks until it has finished.
func (a *ArtifactRetainer) Stop() {
	if a == nil {
		return
	}
	close(a.stopCh)
	<-a.doneCh
	a.log().Info("stopped")
}

func (a *ArtifactRetainer) loop() {
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

// sweep applies expiry, then reclaims what is due. It returns the counts, which
// is what the tests assert on; the loop ignores them.
func (a *ArtifactRetainer) sweep(ctx context.Context) (expired int, purged int) {
	if a == nil {
		return 0, 0
	}
	expired = a.expire(ctx)
	purged, bytes := a.purge(ctx)
	if purged > 0 {
		a.recordPurge(ctx, purged, bytes)
	}
	return expired, purged
}

// expire tombstones artifacts whose own expiry has passed, and records each.
//
// Per artifact rather than as a summary: this is the one tombstone no member
// asked for, so a reader looking for why an artifact is gone has to find
// something naming it, exactly as artifact.deleted would.
func (a *ArtifactRetainer) expire(ctx context.Context) int {
	now := a.now().UTC()
	gone, err := a.store.ExpireArtifacts(ctx, now, artifactSweepBatch)
	if err != nil {
		a.log().WarnContext(ctx, "expire failed", "err", err, "expired", len(gone))
		// Not fatal: whatever it did tombstone is real, and the rest is retried
		// on the next tick.
	}
	for _, it := range gone {
		a.record(ctx, coreaudit.Event{
			TeamID:     it.TeamID,
			ActorType:  coreaudit.ActorSystem,
			ActorID:    coreaudit.ActorOperator,
			Action:     coreaudit.ArtifactExpired,
			TargetType: "artifact",
			TargetID:   it.ArtifactID,
		})
	}
	if len(gone) > 0 {
		a.log().InfoContext(ctx, "artifacts expired", "expired", len(gone))
	}
	return len(gone)
}

// purge reclaims the objects of artifacts tombstoned before the cutoff.
//
// The object goes first and the record second. The reverse would let a crash in
// between leave a row claiming the bytes are gone while they are still billed
// for; this way a crash leaves a row that says the object is still there, and
// the next sweep removes what is already absent — which the content store's
// contract makes a success.
func (a *ArtifactRetainer) purge(ctx context.Context) (count int, bytes int64) {
	cutoff := a.now().UTC().Add(-a.grace)
	due, err := a.store.PurgeableArtifacts(ctx, cutoff, artifactSweepBatch)
	if err != nil {
		a.log().WarnContext(ctx, "read purgeable failed", "err", err, "cutoff", cutoff)
		return 0, 0
	}
	for _, it := range due {
		ref := coreartifact.Ref{TeamID: it.TeamID, ArtifactID: it.ArtifactID}
		if err := a.content.RemoveArtifact(ctx, ref); err != nil {
			// Left for the next sweep. The row still names the object, so
			// nothing is lost but time.
			a.log().WarnContext(ctx, "remove content failed", "err", err, "artifact", it.ArtifactID)
			continue
		}
		marked, err := a.store.MarkArtifactPurged(ctx, it.ArtifactID)
		if err != nil {
			a.log().WarnContext(ctx, "mark purged failed", "err", err, "artifact", it.ArtifactID)
			continue
		}
		if !marked {
			// Another sweep got there first. The removal above was a no-op on
			// absent content; counting it here would double the bytes.
			continue
		}
		count++
		bytes += it.SizeBytes
	}
	if count > 0 {
		a.log().InfoContext(ctx, "artifact content reclaimed", "purged", count, "bytes", bytes, "cutoff", cutoff)
	}
	return count, bytes
}

func (a *ArtifactRetainer) recordPurge(ctx context.Context, count int, bytes int64) {
	a.record(ctx, coreaudit.Event{
		ActorType:  coreaudit.ActorSystem,
		ActorID:    coreaudit.ActorOperator,
		Action:     coreaudit.ArtifactsPurged,
		TargetType: "artifact",
		Detail:     fmt.Sprintf("%d artifacts, %d bytes", count, bytes),
	})
}

// record writes one event, or drops it with a log. A retention sweep must not
// fail because the trail is unreachable: the bytes are already gone.
func (a *ArtifactRetainer) record(ctx context.Context, ev coreaudit.Event) {
	if a.writer == nil {
		return
	}
	if err := a.writer.RecordAuditEvent(ctx, ev); err != nil {
		a.log().ErrorContext(ctx, "retention not recorded", "err", err, "action", ev.Action)
	}
}
