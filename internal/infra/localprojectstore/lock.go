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

// Contention on these locks is two processes of one Project holding it for a
// single small write -- starting in the same repository at the same moment, or
// replacing the memory document. Waiting briefly is therefore the right answer
// where a session lock's is to refuse immediately: nobody is queued behind a
// person's turn, and failing over a few milliseconds of overlap would be the
// surprising outcome.
const (
	lockWait      = 2 * time.Second
	lockRetryStep = 20 * time.Millisecond
)

// releaser is the half of a held lock a caller needs. Naming it keeps the
// flock type out of the signatures that only ever release.
type releaser interface{ Release() error }

// acquireCatalog takes the catalog writer lock, waiting up to lockWait. It
// reports localproject.ErrCatalogBusy when another process still holds it,
// which a caller may treat as fatal (creating a Project) or ignore (advancing
// last_used_at).
func acquireCatalog(ctx context.Context, rootDir string) (*flock.Lock, error) {
	if err := os.MkdirAll(rootDir, 0o700); err != nil {
		return nil, err
	}
	return waitForLock(ctx, filepath.Join(rootDir, LockFile))
}

// waitForLock takes the lock at path, waiting up to lockWait.
func waitForLock(ctx context.Context, path string) (*flock.Lock, error) {
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
