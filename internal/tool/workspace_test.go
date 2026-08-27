package tool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// movingRoot is a Workspace whose value changes between calls, standing in for
// a session that entered a worktree mid-run.
type movingRoot struct{ dir string }

func (m *movingRoot) Root() string { return m.dir }

// TestToolsFollowAMovingRoot is the property this plumbing exists for: a tool
// resolves against the root in force when it is called, not the one it was
// built with. A tool that captured the launch directory would keep writing to
// the tree the session left, and nothing downstream would notice.
func TestToolsFollowAMovingRoot(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()
	if err := os.WriteFile(filepath.Join(first, "where.txt"), []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(second, "where.txt"), []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}

	ws := &movingRoot{dir: first}
	read := NewReadFile(ws)
	args := map[string]any{"file_path": "where.txt"}

	got, err := read.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("read in first root: %v", err)
	}
	if !strings.Contains(got, "first") {
		t.Fatalf("read before the move = %q, want the file from the first root", got)
	}

	ws.dir = second

	got, err = read.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("read in second root: %v", err)
	}
	if !strings.Contains(got, "second") {
		t.Fatalf("read after the move = %q, want the file from the second root", got)
	}
}

// TestContainmentFollowsTheMovingRoot asserts the escape check moves with the
// root: the old tree is outside the new root and must be refused like any
// other outside path, or moving the root would quietly widen what is
// reachable.
func TestContainmentFollowsTheMovingRoot(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()
	if err := os.WriteFile(filepath.Join(first, "where.txt"), []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}

	ws := &movingRoot{dir: first}
	read := NewReadFile(ws)
	ws.dir = second

	_, err := read.Execute(context.Background(), map[string]any{
		"file_path": filepath.Join(first, "where.txt"),
	})
	if err == nil {
		t.Fatal("reading the previous root succeeded; it is outside the current root and must be refused")
	}
}

// TestBashRunsInTheCurrentRoot covers the other half: Bash sets the child's
// working directory per call, so a moved root moves where commands run.
func TestBashRunsInTheCurrentRoot(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()

	ws := &movingRoot{dir: first}
	b := NewBash(ws)
	ws.dir = second

	out, err := b.Execute(context.Background(), map[string]any{"command": "pwd"})
	if err != nil {
		t.Fatalf("pwd: %v", err)
	}
	// macOS reaches a temporary directory through a symlink, so compare what
	// the shell resolved rather than the literal path.
	wantSuffix := filepath.Base(second)
	if !strings.Contains(strings.TrimSpace(out), wantSuffix) {
		t.Fatalf("pwd = %q, want a directory under %q", strings.TrimSpace(out), second)
	}
}
