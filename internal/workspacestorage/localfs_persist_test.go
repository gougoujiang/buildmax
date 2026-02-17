package workspacestorage

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalFSPersistStorage_PutGetListMaterialize(t *testing.T) {
	root := t.TempDir()
	persistRoot := func(workspaceID string) string {
		return filepath.Join(root, workspaceID, "persist")
	}
	s := NewLocalFSPersistStorage(persistRoot)
	ctx := context.Background()
	ws := "ws1"

	if err := s.Put(ctx, ws, "f1.txt", bytes.NewReader([]byte("hello"))); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(ctx, ws, "sub/f2.txt", bytes.NewReader([]byte("world"))); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListFiles(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("list len = %d, want 2", len(list))
	}
	data, err := s.Get(ctx, ws, "f1.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Errorf("got %q", data)
	}
	dst := t.TempDir()
	if err := s.MaterializeToDir(ctx, ws, dst); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dst, "f1.txt"))
	if string(b) != "hello" {
		t.Errorf("materialized f1 = %q", b)
	}
	b, _ = os.ReadFile(filepath.Join(dst, "sub", "f2.txt"))
	if string(b) != "world" {
		t.Errorf("materialized f2 = %q", b)
	}
}
