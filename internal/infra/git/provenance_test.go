package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func initRepo(t *testing.T, root string) {
	t.Helper()
	git(t, root, "init")
	git(t, root, "config", "user.email", "test@example.com")
	git(t, root, "config", "user.name", "Test User")
	writeFile(t, root, "tracked.txt", "one\n")
	git(t, root, "add", ".")
	git(t, root, "commit", "-m", "initial")
}

// The check must be for the directory's own .git. `git status` inside a plain
// directory answers for the nearest enclosing repository, which would give a
// plugin the commit of whatever checkout it happens to sit inside.
func TestIsRepositoryOnlyMatchesACheckoutRoot(t *testing.T) {
	root := t.TempDir()
	initRepo(t, root)
	nested := filepath.Join(root, "plugins", "code-review")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	if !IsRepository(root) {
		t.Error("a checkout root should be a repository")
	}
	if IsRepository(nested) {
		t.Error("a plain directory inside a checkout must not count as one")
	}
	if IsRepository(t.TempDir()) {
		t.Error("an unrelated directory should not be a repository")
	}
	if IsRepository("") {
		t.Error("an empty path should not be a repository")
	}
}

func TestReadStatusCleanCheckout(t *testing.T) {
	root := t.TempDir()
	initRepo(t, root)

	st, err := ReadStatus(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Commit) != 40 {
		t.Errorf("Commit = %q, want a full hash", st.Commit)
	}
	if st.Branch == "" || st.Detached {
		t.Errorf("want a named branch, got %+v", st)
	}
	if st.Dirty {
		t.Error("a fresh commit should not be dirty")
	}
}

// Tracked edits and untracked files both mean the directory is not the commit
// it names.
func TestReadStatusDirtyCases(t *testing.T) {
	tests := []struct {
		name  string
		dirty func(t *testing.T, root string)
	}{
		{"modified tracked file", func(t *testing.T, root string) {
			writeFile(t, root, "tracked.txt", "one\ntwo\n")
		}},
		{"untracked file", func(t *testing.T, root string) {
			writeFile(t, root, "scratch.txt", "new\n")
		}},
		{"staged change", func(t *testing.T, root string) {
			writeFile(t, root, "tracked.txt", "staged\n")
			git(t, root, "add", "tracked.txt")
		}},
		{"deleted file", func(t *testing.T, root string) {
			if err := os.Remove(filepath.Join(root, "tracked.txt")); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			initRepo(t, root)
			tc.dirty(t, root)

			st, err := ReadStatus(context.Background(), root)
			if err != nil {
				t.Fatal(err)
			}
			if !st.Dirty {
				t.Errorf("want dirty, got %+v", st)
			}
			if st.Commit == "" {
				t.Error("a dirty tree still names a commit")
			}
		})
	}
}

func TestReadStatusDetachedHead(t *testing.T) {
	root := t.TempDir()
	initRepo(t, root)
	head := strings.TrimSpace(mustRunGit(context.Background(), root, "rev-parse", "HEAD"))
	git(t, root, "checkout", "--detach", head)

	st, err := ReadStatus(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Detached || st.Branch != "" {
		t.Errorf("want a detached head with no branch, got %+v", st)
	}
	if st.Commit != head {
		t.Errorf("Commit = %q, want %q", st.Commit, head)
	}
}

func TestReadStatusRejectsAPlainDirectory(t *testing.T) {
	if _, err := ReadStatus(context.Background(), t.TempDir()); err == nil {
		t.Error("a plain directory should not produce a status")
	}
}

// A plugin developed locally and never pushed is ordinary, not an error.
func TestReadRemoteURL(t *testing.T) {
	root := t.TempDir()
	initRepo(t, root)
	if got := ReadRemoteURL(context.Background(), root); got != "" {
		t.Errorf("no remote should read as empty, got %q", got)
	}

	want := "git@code.example.com:agents/code-review.git"
	git(t, root, "remote", "add", "origin", want)
	if got := ReadRemoteURL(context.Background(), root); got != want {
		t.Errorf("ReadRemoteURL = %q, want %q", got, want)
	}
	if got := ReadRemoteURL(context.Background(), t.TempDir()); got != "" {
		t.Errorf("a plain directory has no remote, got %q", got)
	}
}
