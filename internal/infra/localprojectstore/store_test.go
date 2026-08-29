package localprojectstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/localproject"
)

func gitKey(commonDir string) localproject.Key {
	return localproject.Key{Kind: localproject.KindGit, Locator: commonDir}
}

func dirKey(root string) localproject.Key {
	return localproject.Key{Kind: localproject.KindDirectory, Locator: root}
}

// propose builds the record Resolve would register for key when none exists.
func propose(t *testing.T, key localproject.Key, workspace string) localproject.Project {
	t.Helper()
	p, err := localproject.New(key, localproject.NameForWorkspace(workspace), workspace, time.Now())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return p
}

func resolve(t *testing.T, s *FileStore, key localproject.Key, workspace string) localproject.Project {
	t.Helper()
	p, err := s.Resolve(context.Background(), key, propose(t, key, workspace))
	if err != nil {
		t.Fatalf("Resolve(%s): %v", key.Locator, err)
	}
	return p
}

// newStore returns a store and the base directory the test should build its
// locators under. t.TempDir mints a fresh directory per call, so a test that
// calls it twice for what is meant to be one path silently describes two.
func newStore(t *testing.T) (*FileStore, string) {
	t.Helper()
	base := t.TempDir()
	return NewFileStore(filepath.Join(base, "projects")), base
}

// The property the whole feature rests on: a repository's worktrees resolve to
// one Project, and unrelated roots never merge into it.
func TestResolveGroupsWorktreesAndSeparatesUnrelatedRoots(t *testing.T) {
	s, base := newStore(t)
	common := filepath.Join(base, "repo", ".git")

	primary := resolve(t, s, gitKey(common), filepath.Join(base, "repo"))
	worktree := resolve(t, s, gitKey(common), filepath.Join(base, "repo", "worktrees", "a"))
	if primary.ID != worktree.ID {
		t.Errorf("worktree resolved to %s, want the primary checkout's %s", worktree.ID, primary.ID)
	}

	// A second clone has its own common directory, so it is its own Project
	// even though it holds the same code.
	clone := resolve(t, s, gitKey(filepath.Join(base, "clone", ".git")), filepath.Join(base, "clone"))
	if clone.ID == primary.ID {
		t.Error("an independent clone was merged into the original project")
	}

	plain := resolve(t, s, dirKey(filepath.Join(base, "notes")), filepath.Join(base, "notes"))
	if plain.Kind != localproject.KindDirectory {
		t.Errorf("Kind = %s, want %s", plain.Kind, localproject.KindDirectory)
	}
}

func TestResolveTouchesLastUsedWithoutChangingIdentity(t *testing.T) {
	s, base := newStore(t)
	key := gitKey(filepath.Join(base, "repo", ".git"))
	workspace := filepath.Join(base, "repo")

	first := resolve(t, s, key, workspace)

	later := first.LastUsedAt.Add(time.Hour)
	s.now = func() time.Time { return later }
	second := resolve(t, s, key, workspace)

	if second.ID != first.ID {
		t.Fatalf("id changed on re-resolve: %s -> %s", first.ID, second.ID)
	}
	if !second.LastUsedAt.Equal(later.UTC()) {
		t.Errorf("LastUsedAt = %v, want %v", second.LastUsedAt, later.UTC())
	}
	if !second.CreatedAt.Equal(first.CreatedAt) {
		t.Errorf("CreatedAt = %v, want it unchanged at %v", second.CreatedAt, first.CreatedAt)
	}
}

// Resolve creates under the catalog lock and re-checks after taking it, so
// callers racing on one new repository agree on a single Project.
func TestResolveIsSerializedForOneKey(t *testing.T) {
	s, base := newStore(t)
	key := gitKey(filepath.Join(base, "repo", ".git"))
	workspace := filepath.Join(base, "repo")

	const callers = 4
	ids := make([]string, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range callers {
		wg.Go(func() {
			<-start
			p, err := s.Resolve(context.Background(), key, propose(t, key, workspace))
			ids[i], errs[i] = p.ID, err
		})
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("caller %d: %v", i, err)
		}
	}
	for i, id := range ids {
		if id != ids[0] {
			t.Fatalf("caller %d got project %s, caller 0 got %s: one repository minted two identities", i, id, ids[0])
		}
	}
	rows, err := s.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("catalog holds %d projects, want 1: %+v", len(rows), rows)
	}
}

func TestGetReadsTheBundleNotTheProjection(t *testing.T) {
	s, base := newStore(t)
	key := dirKey(filepath.Join(base, "notes"))
	p := resolve(t, s, key, key.Locator)

	// A damaged projection must not make a Project unopenable: the bundle is
	// authoritative and the catalog is rebuildable from it.
	if err := os.WriteFile(filepath.Join(s.Dir(), IndexFile), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("corrupt index: %v", err)
	}
	got, err := s.Get(context.Background(), p.ID)
	if err != nil {
		t.Fatalf("Get with a corrupt index: %v", err)
	}
	if got.ID != p.ID {
		t.Errorf("Get returned %s, want %s", got.ID, p.ID)
	}

	rows, err := s.List(context.Background())
	if err != nil {
		t.Fatalf("List after corruption: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != p.ID {
		t.Errorf("List did not rebuild the projection: %+v", rows)
	}
}

func TestResolveFindsAProjectMissingFromTheProjection(t *testing.T) {
	s, base := newStore(t)
	key := gitKey(filepath.Join(base, "repo", ".git"))
	workspace := filepath.Join(base, "repo")
	first := resolve(t, s, key, workspace)

	// An empty-but-valid catalog is the dangerous shape: read literally it says
	// "no such project", and acting on that mints a second identity for a
	// repository that already has one.
	if err := WriteIndex(s.Dir(), nil); err != nil {
		t.Fatalf("WriteIndex: %v", err)
	}
	second := resolve(t, s, key, workspace)
	if second.ID != first.ID {
		t.Errorf("resolve after a lost projection minted %s, want %s", second.ID, first.ID)
	}
}

func TestResolveRefusesADuplicateLocator(t *testing.T) {
	s, base := newStore(t)
	key := gitKey(filepath.Join(base, "repo", ".git"))
	workspace := filepath.Join(base, "repo")
	resolve(t, s, key, workspace)

	// A second bundle claiming the same locator is damage — a copied directory,
	// a half-finished repair. Choosing between them would silently join or
	// split a memory domain, so the store refuses until a person fixes it.
	twin := propose(t, key, workspace)
	if err := WriteMeta(filepath.Join(s.Dir(), twin.ID), twin); err != nil {
		t.Fatalf("write twin: %v", err)
	}
	if _, err := RebuildIndex(s.Dir()); err != nil {
		t.Fatalf("RebuildIndex: %v", err)
	}

	_, err := s.Resolve(context.Background(), key, propose(t, key, workspace))
	if !errors.Is(err, localproject.ErrDuplicateLocator) {
		t.Fatalf("Resolve = %v, want ErrDuplicateLocator", err)
	}
}

func TestUpdateRenamesAndRelinks(t *testing.T) {
	s, base := newStore(t)
	oldCommon := filepath.Join(base, "old", ".git")
	p := resolve(t, s, gitKey(oldCommon), filepath.Join(base, "old"))

	name := "renamed"
	newCommon := filepath.Join(base, "moved", ".git")
	newRoot := filepath.Join(base, "moved")
	if err := s.Update(context.Background(), p.ID, localproject.Update{
		Name:             &name,
		GitCommonDir:     &newCommon,
		DefaultWorkspace: &newRoot,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	// The relinked Project answers at its new locator and nowhere else, under
	// the same id — which is what keeps its sessions and memory attached.
	got, err := s.Resolve(context.Background(), gitKey(newCommon), propose(t, gitKey(newCommon), newRoot))
	if err != nil {
		t.Fatalf("resolve after relink: %v", err)
	}
	if got.ID != p.ID {
		t.Errorf("relinked project resolved to %s, want %s", got.ID, p.ID)
	}
	if got.Name != "renamed" {
		t.Errorf("Name = %q, want renamed", got.Name)
	}

	stale, err := s.Resolve(context.Background(), gitKey(oldCommon), propose(t, gitKey(oldCommon), filepath.Join(base, "old")))
	if err != nil {
		t.Fatalf("resolve at the old locator: %v", err)
	}
	if stale.ID == p.ID {
		t.Error("the old locator still resolves to the relinked project")
	}
}

func TestUpdateRefusesARelinkOntoAClaimedLocator(t *testing.T) {
	s, base := newStore(t)
	occupied := filepath.Join(base, "a", ".git")
	resolve(t, s, gitKey(occupied), filepath.Join(base, "a"))
	mover := resolve(t, s, gitKey(filepath.Join(base, "b", ".git")), filepath.Join(base, "b"))

	err := s.Update(context.Background(), mover.ID, localproject.Update{GitCommonDir: &occupied})
	if !errors.Is(err, localproject.ErrDuplicateLocator) {
		t.Fatalf("Update = %v, want ErrDuplicateLocator", err)
	}
	// The refusal must leave the mover where it was rather than half-applied.
	after, err := s.Get(context.Background(), mover.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if after.GitCommonDir == occupied {
		t.Error("a refused relink was written anyway")
	}
}

func TestDeleteRemovesOnlyTheBundle(t *testing.T) {
	s, base := newStore(t)
	kept := resolve(t, s, dirKey(filepath.Join(base, "kept")), filepath.Join(base, "kept"))
	gone := resolve(t, s, dirKey(filepath.Join(base, "gone")), filepath.Join(base, "gone"))

	if err := s.Delete(context.Background(), gone.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(context.Background(), gone.ID); !errors.Is(err, localproject.ErrNotFound) {
		t.Errorf("Get after Delete = %v, want ErrNotFound", err)
	}
	if _, err := s.Get(context.Background(), kept.ID); err != nil {
		t.Errorf("Delete took the neighbouring project with it: %v", err)
	}
	if err := s.Delete(context.Background(), gone.ID); !errors.Is(err, localproject.ErrNotFound) {
		t.Errorf("second Delete = %v, want ErrNotFound", err)
	}
}

func TestGetUnknownIsNotFound(t *testing.T) {
	s, _ := newStore(t)
	for _, id := range []string{"", "no-such-project"} {
		if _, err := s.Get(context.Background(), id); !errors.Is(err, localproject.ErrNotFound) {
			t.Errorf("Get(%q) = %v, want ErrNotFound", id, err)
		}
	}
}

// A copied bundle answers for the Project it was copied from unless the
// directory name is checked against the id it holds.
func TestRebuildIndexSkipsAMisplacedBundle(t *testing.T) {
	s, base := newStore(t)
	key := dirKey(filepath.Join(base, "notes"))
	p := resolve(t, s, key, key.Locator)

	copied := filepath.Join(s.Dir(), "not-the-id")
	if err := os.MkdirAll(copied, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(s.Dir(), p.ID, MetaFile))
	if err != nil {
		t.Fatalf("read meta: %v", err)
	}
	if err := os.WriteFile(filepath.Join(copied, MetaFile), data, 0o600); err != nil {
		t.Fatalf("write copy: %v", err)
	}

	rows, err := RebuildIndex(s.Dir())
	if err != nil {
		t.Fatalf("RebuildIndex: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != p.ID {
		t.Errorf("rebuild admitted a misplaced bundle: %+v", rows)
	}
}

func TestBundleFilesArePrivate(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("Windows does not carry these mode bits")
	}
	s, base := newStore(t)
	key := dirKey(filepath.Join(base, "notes"))
	p := resolve(t, s, key, key.Locator)

	for path, want := range map[string]os.FileMode{
		filepath.Join(s.Dir(), p.ID, MetaFile): 0o600,
		filepath.Join(s.Dir(), IndexFile):      0o600,
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s mode = %v, want %v", path, got, want)
		}
	}
}
