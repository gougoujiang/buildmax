package scheduler

import (
	"context"
	"time"

	"github.com/gougoujiang/buildmax/internal/infra/objectstore"
)

// defaultCheckpointSweepInterval is how often orphan payloads are reclaimed.
// Hourly, matching the artifact retainer: the grace period is in days, so
// nothing depends on this being prompt.
const defaultCheckpointSweepInterval = time.Hour

// CheckpointBlobStore lists the checkpoint payloads in the object store and
// removes one by its storage key. Declared here, not taken from a service, so
// the sweep depends on just the two operations it performs; both are read-only
// or idempotent, which is what lets an interrupted sweep simply run again.
type CheckpointBlobStore interface {
	ListBlobs(ctx context.Context) ([]objectstore.ObjectInfo, error)
	Delete(ctx context.Context, storageKey string) error
}

// CheckpointReferenceStore reports every storage key a checkpoint row still
// names, so the sweep can tell a committed payload from an orphan.
type CheckpointReferenceStore interface {
	ReferencedCheckpointStorageKeys(ctx context.Context) (map[string]struct{}, error)
}

// CheckpointOrphanSweeper reclaims checkpoint payloads that no checkpoint row
// references and that are older than a grace period — the bytes a worker
// uploaded but whose pointer was never committed, from a finalize that failed or
// a worker that died between upload and commit (§8, §12.4).
//
// The grace period is what makes it safe against the commit protocol's
// bytes-before-pointer ordering: a payload written moments ago whose pointer is
// still in flight is too recent to be a candidate, so the sweep never races a
// completing upload. A re-referenced payload is re-uploaded by its worker, which
// refreshes its modification time, so it too stays out of reach.
type CheckpointOrphanSweeper struct {
	blobs CheckpointBlobStore
	refs  CheckpointReferenceStore
	// grace holds a payload for this long after it was written before it can be
	// reclaimed. It must exceed the longest expected gap between a worker's
	// upload and its finalize, or a slow commit could lose its own bytes.
	grace    time.Duration
	interval time.Duration
	stopCh   chan struct{}
	doneCh   chan struct{}
	now      func() time.Time
}

// NewCheckpointOrphanSweeper returns a sweeper, or nil when it could do nothing
// (a deployment with no checkpoint storage). Use 0 for the default interval.
func NewCheckpointOrphanSweeper(blobs CheckpointBlobStore, refs CheckpointReferenceStore, graceDays int, interval time.Duration) *CheckpointOrphanSweeper {
	if blobs == nil || refs == nil {
		return nil
	}
	if interval <= 0 {
		interval = defaultCheckpointSweepInterval
	}
	grace := time.Duration(0)
	if graceDays > 0 {
		grace = time.Duration(graceDays) * 24 * time.Hour
	}
	return &CheckpointOrphanSweeper{
		blobs:    blobs,
		refs:     refs,
		grace:    grace,
		interval: interval,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
		now:      time.Now,
	}
}

// Start launches the sweep loop. A no-op on a nil sweeper, so a deployment
// without checkpoint storage needs no branch at the call site.
func (c *CheckpointOrphanSweeper) Start() {
	if c == nil {
		return
	}
	go c.loop()
	c.log().Info("started", "grace", c.grace, "interval", c.interval)
}

// Stop signals the loop to exit and blocks until it has finished.
func (c *CheckpointOrphanSweeper) Stop() {
	if c == nil {
		return
	}
	close(c.stopCh)
	<-c.doneCh
	c.log().Info("stopped")
}

func (c *CheckpointOrphanSweeper) loop() {
	defer close(c.doneCh)
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	c.sweep(context.Background())
	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.sweep(context.Background())
		}
	}
}

// sweep deletes every orphan payload past the grace period. It returns the
// count, which the tests assert on; the loop ignores it. A read failure on
// either half aborts the pass rather than risk deleting a payload whose row it
// could not confirm — the next tick tries again.
func (c *CheckpointOrphanSweeper) sweep(ctx context.Context) int {
	if c == nil {
		return 0
	}
	referenced, err := c.refs.ReferencedCheckpointStorageKeys(ctx)
	if err != nil {
		c.log().WarnContext(ctx, "read referenced keys failed; skipping sweep", "err", err)
		return 0
	}
	blobs, err := c.blobs.ListBlobs(ctx)
	if err != nil {
		c.log().WarnContext(ctx, "list payloads failed; skipping sweep", "err", err)
		return 0
	}
	cutoff := c.now().UTC().Add(-c.grace)
	deleted := 0
	for _, b := range blobs {
		if _, ok := referenced[b.Key]; ok {
			continue
		}
		// Too recent to judge: its pointer may be committing right now. Bytes
		// before pointer means a live payload is always newer than the grace, so
		// only a payload strictly older than the cutoff is a candidate — one
		// written at the cutoff instant is spared.
		if !b.ModTime.Before(cutoff) {
			continue
		}
		if err := c.blobs.Delete(ctx, b.Key); err != nil {
			c.log().WarnContext(ctx, "delete orphan payload failed", "err", err, "key", b.Key)
			continue
		}
		deleted++
	}
	if deleted > 0 {
		c.log().InfoContext(ctx, "orphan checkpoint payloads reclaimed", "deleted", deleted, "cutoff", cutoff)
	}
	return deleted
}
