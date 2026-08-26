package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddListAndRemoveAWorktree(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo)
	ctx := context.Background()

	path := filepath.Join(repo, ".buildmax", "worktrees", "refactor")
	if err := AddWorktree(ctx, repo, path, "worktree/refactor"); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}

	all, err := ListWorktrees(ctx, repo)
	if err != nil {
		t.Fatalf("ListWorktrees: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("ListWorktrees returned %d trees, want the primary plus the new one", len(all))
	}
	if !all[0].Main {
		t.Error("the first tree should be the primary checkout")
	}
	added := all[1]
	if added.Main {
		t.Error("the added tree must not be reported as primary")
	}
	if added.Branch != "worktree/refactor" {
		t.Errorf("branch = %q, want the branch it was created on without its ref prefix", added.Branch)
	}
	if added.Head == "" {
		t.Error("head should name the commit the tree starts from")
	}

	if err := RemoveWorktree(ctx, repo, path, false); err != nil {
		t.Fatalf("RemoveWorktree: %v", err)
	}
	all, err = ListWorktrees(ctx, repo)
	if err != nil {
		t.Fatalf("ListWorktrees after remove: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("ListWorktrees returned %d trees after removal, want only the primary", len(all))
	}
}

// A second worktree on an existing branch name must fail rather than pick a
// different name: a model that meant to return to a tree would otherwise get a
// second one. See docs/design/workspace-root-and-worktrees.md D9.
func TestAddWorktreeFailsOnACollidingBranch(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo)
	ctx := context.Background()

	first := filepath.Join(repo, ".buildmax", "worktrees", "one")
	if err := AddWorktree(ctx, repo, first, "worktree/shared"); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}
	second := filepath.Join(repo, ".buildmax", "worktrees", "two")
	err := AddWorktree(ctx, repo, second, "worktree/shared")
	if err == nil {
		t.Fatal("adding a second worktree on the same branch succeeded; it must fail")
	}
	if !strings.Contains(err.Error(), "worktree/shared") {
		t.Errorf("error = %q, want it to name the branch that collided", err)
	}
}

// The admin directory is where per-worktree state belongs: outside the working
// tree, so a file written there never appears in the user's git status.
func TestAdminDirIsOutsideTheWorkingTree(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo)
	ctx := context.Background()

	path := filepath.Join(repo, ".buildmax", "worktrees", "admin")
	if err := AddWorktree(ctx, repo, path, "worktree/admin"); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}
	admin, err := AdminDir(ctx, path)
	if err != nil {
		t.Fatalf("AdminDir: %v", err)
	}
	if !strings.Contains(filepath.ToSlash(admin), "/worktrees/admin") {
		t.Fatalf("AdminDir = %q, want the repository's per-worktree directory", admin)
	}

	if err := os.WriteFile(filepath.Join(admin, "occupied"), []byte("session"), 0o600); err != nil {
		t.Fatal(err)
	}
	dirty, err := UncommittedPaths(ctx, path)
	if err != nil {
		t.Fatalf("UncommittedPaths: %v", err)
	}
	if len(dirty) != 0 {
		t.Fatalf("git reports %v as changed; a file in the admin directory must be invisible to status", dirty)
	}
}

func TestUncommittedPathsReportsWhatStaysBehind(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo)
	ctx := context.Background()

	writeFile(t, repo, "tracked.txt", "changed\n")
	writeFile(t, repo, "new.txt", "untracked\n")

	paths, err := UncommittedPaths(ctx, repo)
	if err != nil {
		t.Fatalf("UncommittedPaths: %v", err)
	}
	got := strings.Join(paths, " ")
	if !strings.Contains(got, "tracked.txt") || !strings.Contains(got, "new.txt") {
		t.Fatalf("UncommittedPaths = %v, want both the modified and the untracked file", paths)
	}
}

// Removing a worktree is refused when it holds commits nothing else reaches,
// so the count has to be right in both directions.
func TestUnreachableCommitsCountsOnlyWhatWouldBeLost(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo)
	ctx := context.Background()

	path := filepath.Join(repo, ".buildmax", "worktrees", "work")
	if err := AddWorktree(ctx, repo, path, "worktree/work"); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}

	n, err := UnreachableCommits(ctx, path)
	if err != nil {
		t.Fatalf("UnreachableCommits: %v", err)
	}
	if n != 0 {
		t.Fatalf("a fresh worktree has %d unreachable commits, want 0", n)
	}

	writeFile(t, path, "work.txt", "done\n")
	git(t, path, "add", ".")
	git(t, path, "commit", "-m", "work")

	n, err = UnreachableCommits(ctx, path)
	if err != nil {
		t.Fatalf("UnreachableCommits after a commit: %v", err)
	}
	if n != 1 {
		t.Fatalf("UnreachableCommits = %d after one commit, want 1", n)
	}
}

func TestExcludeLocallyIsIdempotentAndLeavesGitignoreAlone(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo)
	ctx := context.Background()

	if err := ExcludeLocally(ctx, repo, "/.buildmax/worktrees/"); err != nil {
		t.Fatalf("ExcludeLocally: %v", err)
	}
	if err := ExcludeLocally(ctx, repo, "/.buildmax/worktrees/"); err != nil {
		t.Fatalf("second ExcludeLocally: %v", err)
	}

	body, err := os.ReadFile(filepath.Join(repo, ".git", "info", "exclude"))
	if err != nil {
		t.Fatalf("read info/exclude: %v", err)
	}
	if n := strings.Count(string(body), "/.buildmax/worktrees/"); n != 1 {
		t.Fatalf("pattern appears %d times, want exactly one", n)
	}
	if _, err := os.Stat(filepath.Join(repo, ".gitignore")); !os.IsNotExist(err) {
		t.Error("a tracked .gitignore was created; the exclusion must stay local to the clone")
	}

	// The point of excluding it: a worktree under that path leaves the
	// repository clean.
	path := filepath.Join(repo, ".buildmax", "worktrees", "hidden")
	if err := AddWorktree(ctx, repo, path, "worktree/hidden"); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}
	dirty, err := UncommittedPaths(ctx, repo)
	if err != nil {
		t.Fatalf("UncommittedPaths: %v", err)
	}
	if len(dirty) != 0 {
		t.Fatalf("git reports %v; an excluded worktree directory must not show up", dirty)
	}
}
