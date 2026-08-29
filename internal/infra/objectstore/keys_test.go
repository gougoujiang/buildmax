package objectstore

import (
	"testing"
)

func TestCleanRelPath(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"empty", "", "", true},
		{"dot", ".", "", true},
		{"absolute", "/a/b", "", true},
		{"traversal", "..", "", true},
		{"traversal prefix", "../x", "", true},
		{"traversal mid", "a/../b", "", true},
		{"simple", "a.txt", "a.txt", false},
		{"nested", "a/b/c.txt", "a/b/c.txt", false},
		{"backslash", "a\\b", "a/b", false},
		{"double slash", "a//b", "a/b", false},
		{"leading slash", "/a", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CleanRelPath(tt.in)
			if (err != nil) != tt.wantErr {
				t.Errorf("CleanRelPath(%q) err = %v, wantErr %v", tt.in, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("CleanRelPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

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

func TestRunOutputResultKey(t *testing.T) {
	key := RunOutputResultKey("workspaces", "u1", "conv1", "chat1", "run1")
	if key != "workspaces/u1/artifacts/conv1/chat1/run1/result.md" {
		t.Errorf("got %q", key)
	}
	key = runOutputResultKey(runKeyScope{Prefix: "workspaces", CreatedBy: "u1", ConversationID: "conv1", TaskID: "chat1", TaskRunID: "run1"})
	if key != "workspaces/u1/artifacts/conv1/chat1/run1/result.md" {
		t.Errorf("runOutputResultKey got %q", key)
	}
}

func TestRunOutputFileKey(t *testing.T) {
	key, err := RunOutputFileKey("workspaces", "u1", "conv1", "chat1", "run1", "result.md")
	if err != nil {
		t.Fatal(err)
	}
	if key != "workspaces/u1/artifacts/conv1/chat1/run1/result.md" {
		t.Errorf("got %q", key)
	}
	key, err = RunOutputFileKey("w", "u", "conv", "c", "r", "sub/file.txt")
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

func TestRunGlobalObjectKey(t *testing.T) {
	key, err := RunGlobalObjectKey("workspaces", "u1", "conv1", "chat1", "run1", "logs/buildmax.log")
	if err != nil {
		t.Fatal(err)
	}
	if key != "workspaces/u1/conversations/conv1/tasks/chat1/run1/global/logs/buildmax.log" {
		t.Errorf("got %q", key)
	}
	key, err = RunGlobalObjectKey("w", "u", "conv", "cid", "rid", "sessions/sessions.json")
	if err != nil {
		t.Fatal(err)
	}
	if key != "w/u/conversations/conv/tasks/cid/rid/global/sessions/sessions.json" {
		t.Errorf("got %q", key)
	}
	_, err = RunGlobalObjectKey("w", "u", "conv", "cid", "rid", "../x")
	if err != ErrInvalidPath {
		t.Errorf("want ErrInvalidPath for .. path, got %v", err)
	}
	_, err = RunGlobalObjectKey("w", "u", "conv", "cid", "rid", "")
	if err != ErrInvalidPath {
		t.Errorf("want ErrInvalidPath for empty path, got %v", err)
	}
	_, err = RunGlobalObjectKey("w", "u", "conv", "cid", "rid", "/abs")
	if err != ErrInvalidPath {
		t.Errorf("want ErrInvalidPath for absolute path, got %v", err)
	}
}

func TestRunArtifactsObjectKey(t *testing.T) {
	key, err := RunArtifactsObjectKey("workspaces", "u1", "conv1", "chat1", "run1", "result.md")
	if err != nil {
		t.Fatal(err)
	}
	if key != "workspaces/u1/conversations/conv1/tasks/chat1/run1/artifacts/result.md" {
		t.Errorf("got %q", key)
	}
}
