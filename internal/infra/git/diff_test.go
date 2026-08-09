package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestReadWorkspaceShowsChangedFiles(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init")
	git(t, root, "config", "user.email", "test@example.com")
	git(t, root, "config", "user.name", "Test User")

	writeFile(t, root, "modified.txt", "one\n")
	writeFile(t, root, "deleted.txt", "gone\n")
	writeFile(t, root, "renamed_old.txt", "move\n")
	git(t, root, "add", ".")
	git(t, root, "commit", "-m", "initial")

	writeFile(t, root, "modified.txt", "one\ntwo\n")
	if err := os.Remove(filepath.Join(root, "deleted.txt")); err != nil {
		t.Fatal(err)
	}
	git(t, root, "mv", "renamed_old.txt", "renamed_new.txt")
	writeFile(t, root, "added.txt", "new\n")

	diff, err := ReadWorkspace(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]ChangeStatus{}
	for _, f := range diff.Files {
		got[f.Path] = f.Status
		if f.Path == "modified.txt" && f.Patch == "" {
			t.Fatalf("modified file should include a patch: %+v", f)
		}
	}
	want := map[string]ChangeStatus{
		"added.txt":       StatusAdded,
		"modified.txt":    StatusModified,
		"deleted.txt":     StatusDeleted,
		"renamed_new.txt": StatusRenamed,
	}
	for path, status := range want {
		if got[path] != status {
			t.Fatalf("status for %s = %q, want %q; all=%v", path, got[path], status, got)
		}
	}
}

func git(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("git %v failed: %v\n%s", args, err, out)
	}
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
