package worktree

import (
	"context"

	"errors"
	"github.com/gougoujiang/buildmax/internal/core/session"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func initRepo(t *testing.T, root string) {
	t.Helper()
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test User"},
	} {
		run(t, root, args...)
	}
	writeFile(t, root, "tracked.txt", "one\n")
	run(t, root, "add", ".")
	run(t, root, "commit", "-m", "initial")
}

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// testCtx carries a session identity, which is what a refusal names.
func testCtx() context.Context {
	return session.CtxWithSessionID(context.Background(), "test-session")
}

// testRoot stands in for agentapp's MovableRoot, which this package must not
// import.
type testRoot struct {
	mu  sync.Mutex
	dir string
}

func newTestRoot(dir string) *testRoot { return &testRoot{dir: dir} }

func (r *testRoot) Root() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.dir
}

func (r *testRoot) Set(dir string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dir = filepath.Clean(dir)
}

func newManager(t *testing.T, repo string) (*Manager, *testRoot) {
	t.Helper()
	root := newTestRoot(repo)
	m := NewManager(root)
	t.Cleanup(func() { _ = m.Close() })
	return m, root
}

func TestCreateEntersAndMovesTheRoot(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo)
	m, root := newManager(t, repo)

	// A dirty tree must be reported, not resolved: the conversation may
	// describe code the new tree cannot see (D6).
	writeFile(t, repo, "tracked.txt", "changed\n")

	created, err := m.Create(testCtx(), "refactor")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Branch != "worktree/refactor" {
		t.Errorf("branch = %q, want the namespaced branch", created.Branch)
	}
	if len(created.LeftBehind) != 1 || created.LeftBehind[0] != "tracked.txt" {
		t.Errorf("LeftBehind = %v, want the uncommitted file that stayed in the original tree", created.LeftBehind)
	}
	if !sameDir(root.Root(), created.Path) {
		t.Errorf("root = %q, want the session moved into %q", root.Root(), created.Path)
	}
	if !sameDir(m.Current(), created.Path) {
		t.Errorf("Current = %q, want %q", m.Current(), created.Path)
	}

	// The worktree directory must not show up in the repository's status.
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), ".buildmax") {
		t.Errorf("git status reports the worktree directory:\n%s", out)
	}
}

func TestLeaveReturnsToTheLaunchDirectory(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo)
	m, root := newManager(t, repo)

	created, err := m.Create(testCtx(), "work")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !sameDir(root.Root(), created.Path) {
		t.Fatal("Create did not enter the worktree")
	}
	m.Leave()
	if !sameDir(root.Root(), repo) {
		t.Errorf("root = %q after Leave, want the launch directory %q", root.Root(), repo)
	}
	if m.Current() != "" {
		t.Errorf("Current = %q after Leave, want empty", m.Current())
	}
}

// D3: the set of reachable roots stays closed. A directory that is not a
// worktree of this repository cannot be entered, whatever it is.
func TestEnterRefusesAnythingButAWorktreeOfThisRepo(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo)
	m, root := newManager(t, repo)

	elsewhere := t.TempDir()
	initRepo(t, elsewhere)

	for _, dir := range []string{elsewhere, t.TempDir(), filepath.Join(repo, "subdir")} {
		if err := m.Enter(testCtx(), dir); !errors.Is(err, ErrNotAWorktree) {
			t.Errorf("Enter(%q) = %v, want ErrNotAWorktree", dir, err)
		}
	}
	if !sameDir(root.Root(), repo) {
		t.Errorf("root moved to %q after refused entries", root.Root())
	}
}

// D3: an idle worktree may be entered, including one this session did not
// create — a resumed session has to be able to return to its own work.
func TestEnterAcceptsAnIdleWorktree(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo)

	first, _ := newManager(t, repo)
	created, err := first.Create(testCtx(), "handover")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	first.Leave() // the tree is now idle

	second, root := newManager(t, repo)
	if err := second.Enter(testCtx(), created.Path); err != nil {
		t.Fatalf("Enter an idle worktree: %v", err)
	}
	if !sameDir(root.Root(), created.Path) {
		t.Errorf("root = %q, want the entered worktree", root.Root())
	}
}

// D3: a worktree a live session holds is refused, and the refusal names who is
// there. Two writers in one tree is the race the feature exists to avoid.
func TestEnterRefusesAnOccupiedWorktree(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo)

	holder, _ := newManager(t, repo)
	created, err := holder.Create(testCtx(), "busy")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	other := NewManager(newTestRoot(repo))
	t.Cleanup(func() { _ = other.Close() })
	err = other.Enter(testCtx(), created.Path)
	if !errors.Is(err, ErrOccupied) {
		t.Fatalf("Enter an occupied worktree = %v, want ErrOccupied", err)
	}
	if !strings.Contains(err.Error(), "test-session") {
		t.Errorf("refusal = %q, want it to name the holding session", err)
	}

	// Once the holder leaves, the same entry succeeds.
	holder.Leave()
	if err := other.Enter(testCtx(), created.Path); err != nil {
		t.Fatalf("Enter after the holder left: %v", err)
	}
}

// D4/D5: removal refuses to discard work unless told to.
func TestRemoveRefusesToDiscardWork(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo)
	m, _ := newManager(t, repo)
	ctx := testCtx()

	created, err := m.Create(ctx, "unsaved")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	writeFile(t, created.Path, "draft.txt", "work in progress\n")

	err = m.Remove(ctx, created.Path, false)
	if !errors.Is(err, ErrHasWork) {
		t.Fatalf("Remove with an uncommitted file = %v, want ErrHasWork", err)
	}
	if !strings.Contains(err.Error(), "draft.txt") {
		t.Errorf("refusal = %q, want it to name what would be lost", err)
	}

	// A commit no other branch reaches counts too.
	run(t, created.Path, "add", ".")
	run(t, created.Path, "commit", "-m", "wip")
	err = m.Remove(ctx, created.Path, false)
	if !errors.Is(err, ErrHasWork) {
		t.Fatalf("Remove with an unreachable commit = %v, want ErrHasWork", err)
	}

	if err := m.Remove(ctx, created.Path, true); err != nil {
		t.Fatalf("Remove with discard: %v", err)
	}
	if _, err := os.Stat(created.Path); !os.IsNotExist(err) {
		t.Error("the worktree directory is still there after a discarding removal")
	}
}

func TestRemoveACleanWorktreeAndItsBranch(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo)
	m, root := newManager(t, repo)
	ctx := testCtx()

	created, err := m.Create(ctx, "clean")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := m.Remove(ctx, created.Path, false); err != nil {
		t.Fatalf("Remove a clean worktree: %v", err)
	}
	if !sameDir(root.Root(), repo) {
		t.Errorf("root = %q after removing the tree it was in, want the launch directory", root.Root())
	}
	cmd := exec.Command("git", "branch", "--list", created.Branch)
	cmd.Dir = repo
	out, _ := cmd.Output()
	if strings.TrimSpace(string(out)) != "" {
		t.Errorf("branch %s survived removal: %q", created.Branch, out)
	}
}

func TestListReportsOccupancy(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo)
	m, _ := newManager(t, repo)
	ctx := testCtx()

	if _, err := m.Create(ctx, "alpha"); err != nil {
		t.Fatalf("Create alpha: %v", err)
	}
	m.Leave()
	if _, err := m.Create(ctx, "beta"); err != nil {
		t.Fatalf("Create beta: %v", err)
	}

	all, err := m.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("List returned %d worktrees, want 2 (the primary checkout is not one)", len(all))
	}
	byName := map[string]Info{}
	for _, info := range all {
		byName[info.Name] = info
	}
	if byName["alpha"].Occupied {
		t.Error("alpha was left; it must not report as occupied")
	}
	if !byName["beta"].Current || !byName["beta"].Occupied {
		t.Errorf("beta = %+v, want the current, occupied tree", byName["beta"])
	}
}

func TestCreateRejectsNamesThatAreNotOneSegment(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo)
	m, _ := newManager(t, repo)

	for _, name := range []string{"../escape", "a/b", "", ".hidden", strings.Repeat("x", 65)} {
		if _, err := m.Create(testCtx(), name); err == nil {
			t.Errorf("Create(%q) succeeded; a name must be one path segment", name)
		}
	}
}

// D9: a colliding name fails rather than being suffixed, so a model that meant
// to return to a tree does not silently get a second one.
func TestCreateFailsOnACollidingName(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo)
	m, _ := newManager(t, repo)
	ctx := testCtx()

	if _, err := m.Create(ctx, "same"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	m.Leave()
	if _, err := m.Create(ctx, "same"); err == nil {
		t.Fatal("a second worktree with the same name succeeded; it must fail")
	}
}

func TestOutsideARepositoryEverythingRefuses(t *testing.T) {
	plain := t.TempDir()
	m, _ := newManager(t, plain)

	if _, err := m.Create(testCtx(), "x"); !errors.Is(err, ErrNotARepository) {
		t.Errorf("Create outside a repository = %v, want ErrNotARepository", err)
	}
	if _, err := m.List(testCtx()); !errors.Is(err, ErrNotARepository) {
		t.Errorf("List outside a repository = %v, want ErrNotARepository", err)
	}
}
