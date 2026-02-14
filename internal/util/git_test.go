package util

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCurrentBranch_NonRepo(t *testing.T) {
	dir := t.TempDir()
	got := CurrentBranch(dir)
	if got != "" {
		t.Errorf("CurrentBranch(non-repo dir) = %q, want %q", got, "")
	}
}

func TestCurrentBranch_Repo(t *testing.T) {
	dir := t.TempDir()
	// git init and checkout -b so we have a named branch
	for _, c := range []*exec.Cmd{
		exec.Command("git", "init"),
		exec.Command("git", "checkout", "-b", "test-branch"),
	} {
		c.Dir = dir
		if err := c.Run(); err != nil {
			t.Skipf("git not available or init failed: %v", err)
		}
	}
	got := CurrentBranch(dir)
	if got != "test-branch" {
		t.Errorf("CurrentBranch(repo) = %q, want %q", got, "test-branch")
	}
}

func TestCurrentBranch_InvalidDir(t *testing.T) {
	got := CurrentBranch(filepath.FromSlash("/nonexistent/dir"))
	if got != "" {
		t.Errorf("CurrentBranch(invalid dir) = %q, want %q", got, "")
	}
}
