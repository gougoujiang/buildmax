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

func TestTaskBuildmaxObjectKey(t *testing.T) {
	key, err := TaskBuildmaxObjectKey("workspaces", "ws1", "task1", "logs/buildmax.log")
	if err != nil {
		t.Fatal(err)
	}
	if key != "workspaces/ws1/tasks/task1/buildmax/logs/buildmax.log" {
		t.Errorf("got %q", key)
	}
	key, err = TaskBuildmaxObjectKey("w", "ws", "tid", "sessions/sessions.json")
	if err != nil {
		t.Fatal(err)
	}
	if key != "w/ws/tasks/tid/buildmax/sessions/sessions.json" {
		t.Errorf("got %q", key)
	}
	_, err = TaskBuildmaxObjectKey("w", "ws", "tid", "../x")
	if err != ErrInvalidPath {
		t.Errorf("want ErrInvalidPath for .. path, got %v", err)
	}
	_, err = TaskBuildmaxObjectKey("w", "ws", "tid", "")
	if err != ErrInvalidPath {
		t.Errorf("want ErrInvalidPath for empty path, got %v", err)
	}
	_, err = TaskBuildmaxObjectKey("w", "ws", "tid", "/abs")
	if err != ErrInvalidPath {
		t.Errorf("want ErrInvalidPath for absolute path, got %v", err)
	}
}
