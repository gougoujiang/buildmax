package blob

import (
	"testing"
)

func TestPersistObjectKey(t *testing.T) {
	key, err := PersistObjectKey("workspaces", "ws1", "a/b.txt")
	if err != nil {
		t.Fatal(err)
	}
	if key != "workspaces/ws1/persist/a/b.txt" {
		t.Errorf("got %q", key)
	}
	_, err = PersistObjectKey("w", "ws", "../x")
	if err != ErrInvalidPath {
		t.Errorf("want ErrInvalidPath, got %v", err)
	}
}

func TestArtifactResultKey(t *testing.T) {
	key := ArtifactResultKey("workspaces", "ws1", "task1", "art1")
	if key != "workspaces/ws1/artifacts/task1/art1/result.md" {
		t.Errorf("got %q", key)
	}
}

func TestPersistPrefix(t *testing.T) {
	p := PersistPrefix("workspaces", "ws1")
	if p != "workspaces/ws1/persist/" {
		t.Errorf("got %q", p)
	}
}
