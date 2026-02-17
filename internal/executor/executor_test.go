package executor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyWorkspaceContents_SrcMissing(t *testing.T) {
	dst := t.TempDir()
	err := copyWorkspaceContents(filepath.Join(dst, "nonexistent"), dst)
	if err != nil {
		t.Errorf("copyWorkspaceContents(missing src) = %v, want nil", err)
	}
}

func TestCopyWorkspaceContents_SrcNotDir(t *testing.T) {
	f := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(f, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	err := copyWorkspaceContents(f, dst)
	if err != nil {
		t.Errorf("copyWorkspaceContents(file as src) = %v, want nil", err)
	}
}

func TestCopyWorkspaceContents_EmptyDir(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dstSub := filepath.Join(dst, "out")
	if err := copyWorkspaceContents(src, dstSub); err != nil {
		t.Fatalf("copyWorkspaceContents(empty dir): %v", err)
	}
	entries, err := os.ReadDir(dstSub)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected empty dst, got %d entries", len(entries))
	}
}

func TestCopyWorkspaceContents_OneFile(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "in.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	dstSub := filepath.Join(dst, "out")
	if err := copyWorkspaceContents(src, dstSub); err != nil {
		t.Fatalf("copyWorkspaceContents: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dstSub, "in.txt"))
	if err != nil {
		t.Fatalf("ReadFile copied: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("copied content = %q, want hello", data)
	}
}

func TestCopyWorkspaceContents_NestedDirAndFile(t *testing.T) {
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "a", "b"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "a", "b", "f.txt"), []byte("nested"), 0644); err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	dstSub := filepath.Join(dst, "out")
	if err := copyWorkspaceContents(src, dstSub); err != nil {
		t.Fatalf("copyWorkspaceContents: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dstSub, "a", "b", "f.txt"))
	if err != nil {
		t.Fatalf("ReadFile nested: %v", err)
	}
	if string(data) != "nested" {
		t.Errorf("nested content = %q, want nested", data)
	}
}

