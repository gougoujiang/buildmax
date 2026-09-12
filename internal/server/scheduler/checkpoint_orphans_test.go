package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/icloudbb/buildmax/internal/infra/objectstore"
)

type fakeCheckpointBlobs struct {
	blobs     []objectstore.ObjectInfo
	deleted   []string
	listErr   error
	deleteErr map[string]error
}

func (f *fakeCheckpointBlobs) ListBlobs(context.Context) ([]objectstore.ObjectInfo, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.blobs, nil
}

func (f *fakeCheckpointBlobs) Delete(_ context.Context, key string) error {
	if err := f.deleteErr[key]; err != nil {
		return err
	}
	f.deleted = append(f.deleted, key)
	return nil
}

type fakeCheckpointRefs struct {
	referenced map[string]struct{}
	err        error
}

func (f *fakeCheckpointRefs) ReferencedCheckpointStorageKeys(context.Context) (map[string]struct{}, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.referenced, nil
}

func newSweeperAt(blobs *fakeCheckpointBlobs, refs *fakeCheckpointRefs, graceDays int, now time.Time) *CheckpointOrphanSweeper {
	s := NewCheckpointOrphanSweeper(blobs, refs, graceDays, time.Hour)
	s.now = func() time.Time { return now }
	return s
}

// TestSweepDeletesOnlyUnreferencedAgedPayloads is the core of the sweep: a blob
// is reclaimed only when no row names it and it is older than the grace period.
func TestSweepDeletesOnlyUnreferencedAgedPayloads(t *testing.T) {
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	blobs := &fakeCheckpointBlobs{blobs: []objectstore.ObjectInfo{
		{Key: "sp/workspace/blobs/sha256/orphan-old", ModTime: now.Add(-48 * time.Hour)}, // reclaim
		{Key: "sp/workspace/blobs/sha256/orphan-new", ModTime: now.Add(-1 * time.Hour)},  // too recent
		{Key: "sp/workspace/blobs/sha256/referenced", ModTime: now.Add(-72 * time.Hour)}, // still referenced
	}}
	refs := &fakeCheckpointRefs{referenced: map[string]struct{}{
		"sp/workspace/blobs/sha256/referenced": {},
	}}
	// One-day grace: orphan-old (2 days) is past it, orphan-new (1 hour) is not.
	s := newSweeperAt(blobs, refs, 1, now)

	if got := s.sweep(context.Background()); got != 1 {
		t.Fatalf("deleted = %d, want 1", got)
	}
	if len(blobs.deleted) != 1 || blobs.deleted[0] != "sp/workspace/blobs/sha256/orphan-old" {
		t.Fatalf("deleted = %v, want only the aged orphan", blobs.deleted)
	}
}

// TestSweepZeroGraceStillSpareTheJustWritten pins that even with no configured
// grace, a payload written at the sweep instant is not reclaimed — the cutoff is
// strictly in the past, so an in-flight upload's bytes survive.
func TestSweepZeroGraceSparesTheJustWritten(t *testing.T) {
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	blobs := &fakeCheckpointBlobs{blobs: []objectstore.ObjectInfo{
		{Key: "sp/workspace/blobs/sha256/just-now", ModTime: now},
		{Key: "sp/workspace/blobs/sha256/a-minute-ago", ModTime: now.Add(-time.Minute)},
	}}
	s := newSweeperAt(blobs, &fakeCheckpointRefs{referenced: map[string]struct{}{}}, 0, now)

	if got := s.sweep(context.Background()); got != 1 {
		t.Fatalf("deleted = %d, want 1 (only the older orphan)", got)
	}
	if len(blobs.deleted) != 1 || blobs.deleted[0] != "sp/workspace/blobs/sha256/a-minute-ago" {
		t.Fatalf("deleted = %v, want only a-minute-ago", blobs.deleted)
	}
}

// TestSweepSkipsOnReadFailure pins that a failure reading either half aborts the
// pass without deleting anything — the sweep must never delete a payload whose
// row it could not confirm.
func TestSweepSkipsOnReadFailure(t *testing.T) {
	now := time.Now().UTC()
	aged := []objectstore.ObjectInfo{{Key: "sp/workspace/blobs/sha256/x", ModTime: now.Add(-72 * time.Hour)}}

	refErr := &fakeCheckpointRefs{err: errors.New("db down")}
	blobs := &fakeCheckpointBlobs{blobs: aged}
	if got := newSweeperAt(blobs, refErr, 0, now).sweep(context.Background()); got != 0 || len(blobs.deleted) != 0 {
		t.Fatalf("a reference read failure must delete nothing, got %d deleted %v", got, blobs.deleted)
	}

	listErr := &fakeCheckpointBlobs{blobs: aged, listErr: errors.New("store down")}
	if got := newSweeperAt(listErr, &fakeCheckpointRefs{referenced: map[string]struct{}{}}, 0, now).sweep(context.Background()); got != 0 || len(listErr.deleted) != 0 {
		t.Fatalf("a list failure must delete nothing, got %d deleted %v", got, listErr.deleted)
	}
}

// TestSweepContinuesPastDeleteError pins that one blob that will not delete does
// not strand the rest.
func TestSweepContinuesPastDeleteError(t *testing.T) {
	now := time.Now().UTC()
	blobs := &fakeCheckpointBlobs{
		blobs: []objectstore.ObjectInfo{
			{Key: "sp/workspace/blobs/sha256/stuck", ModTime: now.Add(-72 * time.Hour)},
			{Key: "sp/workspace/blobs/sha256/ok", ModTime: now.Add(-72 * time.Hour)},
		},
		deleteErr: map[string]error{"sp/workspace/blobs/sha256/stuck": errors.New("nope")},
	}
	s := newSweeperAt(blobs, &fakeCheckpointRefs{referenced: map[string]struct{}{}}, 0, now)
	if got := s.sweep(context.Background()); got != 1 {
		t.Fatalf("deleted = %d, want 1 (the one that could)", got)
	}
	if len(blobs.deleted) != 1 || blobs.deleted[0] != "sp/workspace/blobs/sha256/ok" {
		t.Fatalf("deleted = %v, want only ok", blobs.deleted)
	}
}

// TestNewCheckpointOrphanSweeperNilWhenIncomplete pins the nil-safe construction
// used at the call site.
func TestNewCheckpointOrphanSweeperNilWhenIncomplete(t *testing.T) {
	if NewCheckpointOrphanSweeper(nil, &fakeCheckpointRefs{}, 0, 0) != nil {
		t.Error("a sweeper with no blob store should be nil")
	}
	if NewCheckpointOrphanSweeper(&fakeCheckpointBlobs{}, nil, 0, 0) != nil {
		t.Error("a sweeper with no reference store should be nil")
	}
	var s *CheckpointOrphanSweeper
	s.Start() // must not panic
	s.Stop()  // must not panic
	if got := s.sweep(context.Background()); got != 0 {
		t.Errorf("nil sweep = %d, want 0", got)
	}
}
