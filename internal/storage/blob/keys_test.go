package blob

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
	key := ArtifactResultKey("workspaces", "ws1", "chat1", "run1")
	if key != "workspaces/ws1/artifacts/chat1/run1/result.md" {
		t.Errorf("got %q", key)
	}
	key = ArtifactResultKeyScope(ObjectKeyScope{Prefix: "workspaces", WorkspaceID: "ws1", ChatID: "chat1", ChatRunID: "run1"})
	if key != "workspaces/ws1/artifacts/chat1/run1/result.md" {
		t.Errorf("ArtifactResultKeyScope got %q", key)
	}
}

func TestArtifactFileKey(t *testing.T) {
	key, err := ArtifactFileKey("workspaces", "ws1", "chat1", "run1", "result.md")
	if err != nil {
		t.Fatal(err)
	}
	if key != "workspaces/ws1/artifacts/chat1/run1/result.md" {
		t.Errorf("got %q", key)
	}
	key, err = ArtifactFileKey("w", "ws", "c", "r", "sub/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if key != "w/ws/artifacts/c/r/sub/file.txt" {
		t.Errorf("got %q", key)
	}
}

func TestPersistPrefix(t *testing.T) {
	p := PersistPrefix("workspaces", "ws1")
	if p != "workspaces/ws1/home/" {
		t.Errorf("got %q", p)
	}
}

func TestChatBuildmaxObjectKey(t *testing.T) {
	// ChatBuildmaxObjectKey delegates to ChatRunGlobalObjectKey (segment "global").
	key, err := ChatBuildmaxObjectKey("workspaces", "ws1", "chat1", "run1", "logs/buildmax.log")
	if err != nil {
		t.Fatal(err)
	}
	if key != "workspaces/ws1/chats/chat1/run1/global/logs/buildmax.log" {
		t.Errorf("got %q", key)
	}
}

func TestChatRunGlobalObjectKey(t *testing.T) {
	key, err := ChatRunGlobalObjectKey("workspaces", "ws1", "chat1", "run1", "logs/buildmax.log")
	if err != nil {
		t.Fatal(err)
	}
	if key != "workspaces/ws1/chats/chat1/run1/global/logs/buildmax.log" {
		t.Errorf("got %q", key)
	}
	key, err = ChatRunGlobalObjectKey("w", "ws", "cid", "rid", "sessions/sessions.json")
	if err != nil {
		t.Fatal(err)
	}
	if key != "w/ws/chats/cid/rid/global/sessions/sessions.json" {
		t.Errorf("got %q", key)
	}
	_, err = ChatRunGlobalObjectKey("w", "ws", "cid", "rid", "../x")
	if err != ErrInvalidPath {
		t.Errorf("want ErrInvalidPath for .. path, got %v", err)
	}
	_, err = ChatRunGlobalObjectKey("w", "ws", "cid", "rid", "")
	if err != ErrInvalidPath {
		t.Errorf("want ErrInvalidPath for empty path, got %v", err)
	}
	_, err = ChatRunGlobalObjectKey("w", "ws", "cid", "rid", "/abs")
	if err != ErrInvalidPath {
		t.Errorf("want ErrInvalidPath for absolute path, got %v", err)
	}
}

func TestChatRunArtifactsObjectKey(t *testing.T) {
	key, err := ChatRunArtifactsObjectKey("workspaces", "ws1", "chat1", "run1", "result.md")
	if err != nil {
		t.Fatal(err)
	}
	if key != "workspaces/ws1/chats/chat1/run1/artifacts/result.md" {
		t.Errorf("got %q", key)
	}
}
