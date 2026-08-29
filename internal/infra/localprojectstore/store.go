package localprojectstore

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/localproject"
)

// FileStore keeps every Project as a bundle under rootDir, with a rebuildable
// catalog projection and one writer lock beside them.
type FileStore struct {
	rootDir string
	// now is injectable so a test can assert on recorded times without
	// sleeping; production always passes time.Now.
	now func() time.Time
}

// NewFileStore returns a store over the Project bundles in rootDir.
func NewFileStore(rootDir string) *FileStore {
	return &FileStore{rootDir: rootDir, now: time.Now}
}

// Dir is the projects root this store writes under.
func (s *FileStore) Dir() string { return s.rootDir }

func (s *FileStore) bundleDir(id string) string { return filepath.Join(s.rootDir, id) }

// Resolve implements localproject.Store.
func (s *FileStore) Resolve(ctx context.Context, key localproject.Key, proposed localproject.Project) (localproject.Project, error) {
	if key.Locator == "" {
		return localproject.Project{}, errors.New("localprojectstore: resolve needs a locator")
	}

	// The unlocked read is the common case by a wide margin: every session
	// start after the first resolves an existing Project. Taking the lock only
	// to touch or to create keeps that path free of contention.
	found, err := s.findByKey(ctx, key)
	if err != nil && !errors.Is(err, localproject.ErrNotFound) {
		return localproject.Project{}, err
	}
	if err == nil {
		return s.touch(ctx, found), nil
	}

	lock, err := acquireCatalog(ctx, s.rootDir)
	if err != nil {
		return localproject.Project{}, err
	}
	defer func() { _ = lock.Release() }()

	// Repeat the lookup now that nothing else can be creating. Without this,
	// CLI and Desktop opening one new repository at the same moment would each
	// see no Project and write two, splitting the memory they exist to share.
	found, err = s.findByKey(ctx, key)
	if err == nil {
		return s.touch(ctx, found), nil
	}
	if !errors.Is(err, localproject.ErrNotFound) {
		return localproject.Project{}, err
	}

	if proposed.Key() != key {
		return localproject.Project{}, fmt.Errorf("localprojectstore: proposed project does not match %s locator %q", key.Kind, key.Locator)
	}
	if err := WriteMeta(s.bundleDir(proposed.ID), proposed); err != nil {
		return localproject.Project{}, err
	}
	if _, err := RebuildIndex(s.rootDir); err != nil {
		// The bundle is written and is authoritative, so the Project exists.
		// A projection that failed to update is repaired by the next read.
		slog.Warn("rebuild project index failed", "err", err)
	}
	return proposed, nil
}

// touch advances last_used_at, which orders the picker.
//
// Failure is logged and swallowed: this is presentation ordering, and refusing
// to start a session because a recency stamp could not be written would trade a
// real capability for a cosmetic one. It runs unlocked for the same reason —
// the value it races over is the one it is writing.
func (s *FileStore) touch(ctx context.Context, p localproject.Project) localproject.Project {
	touched := localproject.Apply(p, localproject.Update{TouchLastUsed: true}, s.now())
	if err := WriteMeta(s.bundleDir(touched.ID), touched); err != nil {
		slog.Warn("touch project failed", "project_id", touched.ID, "err", err)
		return p
	}
	if err := s.reindex(ctx, touched); err != nil {
		slog.Warn("reindex project failed", "project_id", touched.ID, "err", err)
	}
	return touched
}

// Find implements localproject.Store.
func (s *FileStore) Find(ctx context.Context, key localproject.Key) (localproject.Project, error) {
	if key.Locator == "" {
		return localproject.Project{}, fmt.Errorf("%w: empty locator", localproject.ErrNotFound)
	}
	return s.findByKey(ctx, key)
}

// findByKey resolves a locator through the catalog, rebuilding it when it
// cannot answer. A locator present twice is refused rather than resolved: see
// localproject.ErrDuplicateLocator.
func (s *FileStore) findByKey(ctx context.Context, key localproject.Key) (localproject.Project, error) {
	rows, err := s.List(ctx)
	if err != nil {
		return localproject.Project{}, err
	}
	matched := matchKey(rows, key)
	if len(matched) == 0 {
		// The catalog may simply be behind — a bundle written by a build that
		// failed to reindex, or a row dropped by a partial repair. Rebuild once
		// before concluding the Project does not exist, since concluding that
		// wrongly mints a second identity for the same repository.
		rebuilt, rerr := RebuildIndex(s.rootDir)
		if rerr != nil {
			return localproject.Project{}, rerr
		}
		matched = matchKey(rebuilt, key)
	}
	switch len(matched) {
	case 0:
		return localproject.Project{}, fmt.Errorf("%w: %s %s", localproject.ErrNotFound, key.Kind, key.Locator)
	case 1:
		return s.Get(ctx, matched[0].ID)
	default:
		return localproject.Project{}, fmt.Errorf("%w: %s is claimed by %s and %s",
			localproject.ErrDuplicateLocator, key.Locator, matched[0].ID, matched[1].ID)
	}
}

func matchKey(rows []localproject.Summary, key localproject.Key) []localproject.Summary {
	var out []localproject.Summary
	for _, r := range rows {
		if r.Kind == key.Kind && r.Locator == key.Locator {
			out = append(out, r)
		}
	}
	return out
}

// Get implements localproject.Store.
func (s *FileStore) Get(ctx context.Context, id string) (localproject.Project, error) {
	if id == "" {
		return localproject.Project{}, fmt.Errorf("%w: empty project id", localproject.ErrNotFound)
	}
	return ReadMeta(s.bundleDir(id))
}

// List implements localproject.Store.
func (s *FileStore) List(ctx context.Context) ([]localproject.Summary, error) {
	rows, err := ReadIndex(s.rootDir)
	if err != nil {
		return RebuildIndex(s.rootDir)
	}
	if rows == nil {
		return RebuildIndex(s.rootDir)
	}
	return rows, nil
}

// Update implements localproject.Store.
func (s *FileStore) Update(ctx context.Context, id string, update localproject.Update) error {
	lock, err := acquireCatalog(ctx, s.rootDir)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Release() }()

	current, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	next := localproject.Apply(current, update, s.now())
	if next.Key() != current.Key() {
		// A relink moves a locator, so it has to prove the destination is free
		// while holding the lock. Landing on one another Project already claims
		// would join two memory domains with no user decision behind it.
		if other, err := s.findByKey(ctx, next.Key()); err == nil && other.ID != id {
			return fmt.Errorf("%w: %s is already held by %s", localproject.ErrDuplicateLocator, next.Key().Locator, other.ID)
		}
	}
	if err := WriteMeta(s.bundleDir(id), next); err != nil {
		return err
	}
	return s.reindex(ctx, next)
}

// Delete implements localproject.Store.
func (s *FileStore) Delete(ctx context.Context, id string) error {
	if _, err := s.Get(ctx, id); err != nil {
		return err
	}
	lock, err := acquireCatalog(ctx, s.rootDir)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Release() }()

	if err := os.RemoveAll(s.bundleDir(id)); err != nil {
		return err
	}
	_, err = RebuildIndex(s.rootDir)
	return err
}

// reindex replaces one row in place, so a Project whose bundle is unreadable
// does not vanish from the catalog every time an unrelated one is written.
func (s *FileStore) reindex(ctx context.Context, p localproject.Project) error {
	rows, err := s.List(ctx)
	if err != nil {
		return err
	}
	row := p.Summarize()
	for i := range rows {
		if rows[i].ID == p.ID {
			rows[i] = row
			return WriteIndex(s.rootDir, rows)
		}
	}
	return WriteIndex(s.rootDir, append(rows, row))
}
