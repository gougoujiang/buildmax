package objectstore

import (
	"testing"
)

func TestPersistObjectKey(t *testing.T) {
	key, err := PersistObjectKey("workspaces", "ws1", "a/b.txt")
	if err != nil {
		t.Fatal(err)
	}
	if key != "workspaces/ws1/home/a/b.txt" {
		t.Errorf("got %q", key)
	}
	_, err = PersistObjectKey("w", "ws", "../x")
	if err != ErrInvalidPath {
		t.Errorf("want ErrInvalidPath, got %v", err)
	}
}

func TestArtifactResultKey(t *testing.T) {
	key := ArtifactResultKey("workspaces", "u1", "conv1", "chat1", "run1")
	if key != "workspaces/u1/artifacts/conv1/chat1/run1/result.md" {
		t.Errorf("got %q", key)
	}
	key = ArtifactResultKeyScope(ObjectKeyScope{Prefix: "workspaces", UserID: "u1", ConversationID: "conv1", TaskID: "chat1", TaskRunID: "run1"})
	if key != "workspaces/u1/artifacts/conv1/chat1/run1/result.md" {
		t.Errorf("ArtifactResultKeyScope got %q", key)
	}
}

func TestArtifactFileKey(t *testing.T) {
	key, err := ArtifactFileKey("workspaces", "u1", "conv1", "chat1", "run1", "result.md")
	if err != nil {
		t.Fatal(err)
	}
	if key != "workspaces/u1/artifacts/conv1/chat1/run1/result.md" {
		t.Errorf("got %q", key)
	}
	key, err = ArtifactFileKey("w", "u", "conv", "c", "r", "sub/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if key != "w/u/artifacts/conv/c/r/sub/file.txt" {
		t.Errorf("got %q", key)
	}
}

func TestPersistPrefix(t *testing.T) {
	p := PersistPrefix("workspaces", "ws1")
	if p != "workspaces/ws1/home/" {
		t.Errorf("got %q", p)
	}
}

func TestTaskBuildmaxObjectKey(t *testing.T) {
	// TaskBuildmaxObjectKey delegates to TaskRunGlobalObjectKey (segment "global").
	key, err := TaskBuildmaxObjectKey("workspaces", "u1", "conv1", "chat1", "run1", "logs/buildmax.log")
	if err != nil {
		t.Fatal(err)
	}
	if key != "workspaces/u1/conversations/conv1/tasks/chat1/run1/global/logs/buildmax.log" {
		t.Errorf("got %q", key)
	}
}

func TestTaskRunGlobalObjectKey(t *testing.T) {
	key, err := TaskRunGlobalObjectKey("workspaces", "u1", "conv1", "chat1", "run1", "logs/buildmax.log")
	if err != nil {
		t.Fatal(err)
	}
	if key != "workspaces/u1/conversations/conv1/tasks/chat1/run1/global/logs/buildmax.log" {
		t.Errorf("got %q", key)
	}
	key, err = TaskRunGlobalObjectKey("w", "u", "conv", "cid", "rid", "sessions/sessions.json")
	if err != nil {
		t.Fatal(err)
	}
	if key != "w/u/conversations/conv/tasks/cid/rid/global/sessions/sessions.json" {
		t.Errorf("got %q", key)
	}
	_, err = TaskRunGlobalObjectKey("w", "u", "conv", "cid", "rid", "../x")
	if err != ErrInvalidPath {
		t.Errorf("want ErrInvalidPath for .. path, got %v", err)
	}
	_, err = TaskRunGlobalObjectKey("w", "u", "conv", "cid", "rid", "")
	if err != ErrInvalidPath {
		t.Errorf("want ErrInvalidPath for empty path, got %v", err)
	}
	_, err = TaskRunGlobalObjectKey("w", "u", "conv", "cid", "rid", "/abs")
	if err != ErrInvalidPath {
		t.Errorf("want ErrInvalidPath for absolute path, got %v", err)
	}
}

func TestTaskRunArtifactsObjectKey(t *testing.T) {
	key, err := TaskRunArtifactsObjectKey("workspaces", "u1", "conv1", "chat1", "run1", "result.md")
	if err != nil {
		t.Fatal(err)
	}
	if key != "workspaces/u1/conversations/conv1/tasks/chat1/run1/artifacts/result.md" {
		t.Errorf("got %q", key)
	}
}
