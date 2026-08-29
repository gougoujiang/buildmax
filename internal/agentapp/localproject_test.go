package agentapp

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/localproject"
)

func initRepo(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test User"},
		{"commit", "--allow-empty", "-m", "initial"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func addWorktree(t *testing.T, repo, path, branch string) {
	t.Helper()
	cmd := exec.Command("git", "-C", repo, "worktree", "add", "-b", branch, path, "HEAD")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v\n%s", err, out)
	}
}

func newProjectManager(t *testing.T) *ProjectManager {
	t.Helper()
	return NewProjectManager(filepath.Join(t.TempDir(), "projects"))
}

// The acceptance criterion the feature rests on: a checkout and its linked
// worktree are one Project with two Workspace roots.
func TestResolveGivesAWorktreeTheCheckoutsProject(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	initRepo(t, repo)
	linked := filepath.Join(base, "worktrees", "memory")
	addWorktree(t, repo, linked, "worktree/memory")

	m := newProjectManager(t)
	ctx := context.Background()

	primary, err := m.Resolve(ctx, repo)
	if err != nil {
		t.Fatalf("Resolve(repo): %v", err)
	}
	fromWorktree, err := m.Resolve(ctx, linked)
	if err != nil {
		t.Fatalf("Resolve(worktree): %v", err)
	}
	if fromWorktree.ID != primary.ID {
		t.Errorf("worktree resolved to project %s, want the checkout's %s", fromWorktree.ID, primary.ID)
	}
	if primary.Kind != localproject.KindGit {
		t.Errorf("Kind = %s, want %s", primary.Kind, localproject.KindGit)
	}
	if primary.Name != "repo" {
		t.Errorf("Name = %q, want the checkout directory name", primary.Name)
	}
}

// A session started deep inside a checkout belongs to the repository, and the
// Project opens at its root rather than wherever it was first entered.
func TestResolveFromASubdirectoryFindsTheRepository(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	initRepo(t, repo)
	nested := filepath.Join(repo, "internal", "deep")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	m := newProjectManager(t)
	ctx := context.Background()

	fromRoot, err := m.Resolve(ctx, repo)
	if err != nil {
		t.Fatalf("Resolve(repo): %v", err)
	}
	fromNested, err := m.Resolve(ctx, nested)
	if err != nil {
		t.Fatalf("Resolve(nested): %v", err)
	}
	if fromNested.ID != fromRoot.ID {
		t.Errorf("a subdirectory resolved to project %s, want %s", fromNested.ID, fromRoot.ID)
	}
	if filepath.Base(fromNested.DefaultWorkspace) != "repo" {
		t.Errorf("DefaultWorkspace = %q, want the checkout root", fromNested.DefaultWorkspace)
	}
}

func TestResolvePlainDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "notes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	p, err := newProjectManager(t).Resolve(context.Background(), dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if p.Kind != localproject.KindDirectory {
		t.Errorf("Kind = %s, want %s", p.Kind, localproject.KindDirectory)
	}
	if p.GitCommonDir != "" {
		t.Errorf("GitCommonDir = %q, want empty", p.GitCommonDir)
	}
}

// Two spellings of one directory are one Project. Without this, a person who
// reaches their work through a symlink gets a second memory domain and no
// indication why the first one went quiet.
func TestResolveDoesNotDuplicateASymlinkedDirectory(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("symlink creation needs privileges on Windows")
	}
	base := t.TempDir()
	real := filepath.Join(base, "notes")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "shortcut")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	m := newProjectManager(t)
	ctx := context.Background()
	direct, err := m.Resolve(ctx, real)
	if err != nil {
		t.Fatalf("Resolve(real): %v", err)
	}
	viaLink, err := m.Resolve(ctx, link)
	if err != nil {
		t.Fatalf("Resolve(link): %v", err)
	}
	if viaLink.ID != direct.ID {
		t.Errorf("the symlinked spelling made a second project %s, want %s", viaLink.ID, direct.ID)
	}
}

func TestResolveRefusesAMissingOrNonDirectoryWorkspace(t *testing.T) {
	base := t.TempDir()
	file := filepath.Join(base, "a-file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	m := newProjectManager(t)
	for _, workspace := range []string{filepath.Join(base, "no-such-dir"), file} {
		if _, err := m.Resolve(context.Background(), workspace); err == nil {
			t.Errorf("Resolve(%q) succeeded, want a refusal", workspace)
		}
	}
}
