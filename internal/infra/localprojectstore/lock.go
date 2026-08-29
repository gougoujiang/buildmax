package localprojectstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/localproject"
	"github.com/gougoujiang/buildmax/internal/infra/flock"
)

// LockFile serializes create, relink, rename, and hard delete. Ordinary lookup
// reads the projection without it.
const LockFile = "writer.lock"

// Contention here is two processes starting in the same repository at the same
// moment, holding the lock for one small write. Waiting briefly is therefore
// the right answer where a session lock's is to refuse immediately: nobody is
// queued behind a person's turn, and failing a session start over a few
// milliseconds of overlap would be the surprising outcome.
const (
	lockWait      = 2 * time.Second
	lockRetryStep = 20 * time.Millisecond
)

// acquireCatalog takes the catalog writer lock, waiting up to lockWait. It
// reports localproject.ErrCatalogBusy when another process still holds it,
// which a caller may treat as fatal (creating a Project) or ignore (advancing
// last_used_at).
func acquireCatalog(ctx context.Context, rootDir string) (*flock.Lock, error) {
	if err := os.MkdirAll(rootDir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(rootDir, LockFile)
	holder := fmt.Appendf(nil, "pid %d\n", os.Getpid())

	deadline := time.Now().Add(lockWait)
	for {
		lock, err := flock.TryAcquire(path, holder)
		if err == nil {
			return lock, nil
		}
		if !errors.Is(err, flock.ErrHeld) {
			// A filesystem that cannot lock is reported rather than treated as
			// free: claiming exclusivity we do not have is how two Project IDs
			// get minted for one repository.
			return nil, fmt.Errorf("lock %s: %w", path, err)
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("%w: %s held by %s", localproject.ErrCatalogBusy, path, flock.Holder(path))
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(lockRetryStep):
		}
	}
}
