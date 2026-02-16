package session

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"buildmax/internal/llm"

	"github.com/google/uuid"
)

func TestNewSession(t *testing.T) {
	title := "My chat"
	before := time.Now()
	s := NewSession(title)
	after := time.Now()

	if s.ID() == "" {
		t.Error("ID() is empty")
	}
	if _, err := uuid.Parse(s.ID()); err != nil {
		t.Errorf("ID() is not a valid UUID: %v", err)
	}
	if s.Title() != title {
		t.Errorf("Title() = %q, want %q", s.Title(), title)
	}
	createdAt := s.CreatedAt()
	if createdAt.Before(before) || createdAt.After(after) {
		t.Errorf("CreatedAt() = %v, want between %v and %v", createdAt, before, after)
	}
	msgs := s.Messages()
	if msgs != nil {
		t.Errorf("Messages() = %v, want nil (empty)", msgs)
	}
}

func TestNewSession_EmptyTitle(t *testing.T) {
	s := NewSession("")
	if s.Title() != "" {
		t.Errorf("Title() = %q, want empty", s.Title())
	}
}

func TestAppend_MessagesReturnsCopy(t *testing.T) {
	s := NewSession("")
	s.Append(llm.Message{Role: "user", Content: "Hi"})
	s.Append(llm.Message{Role: "assistant", Content: "Hello"})

	m1 := s.Messages()
	m2 := s.Messages()
	if len(m1) != 2 || len(m2) != 2 {
		t.Fatalf("Messages() length: m1=%d m2=%d, want 2", len(m1), len(m2))
	}
	// Copy is independent: same length and order
	if m1[0].Content != "Hi" || m1[1].Content != "Hello" {
		t.Errorf("m1: got %q, %q", m1[0].Content, m1[1].Content)
	}
	if m2[0].Content != "Hi" || m2[1].Content != "Hello" {
		t.Errorf("m2: got %q, %q", m2[0].Content, m2[1].Content)
	}
	// Mutating the returned slice does not affect session
	m1 = append(m1, llm.Message{Role: "user", Content: "extra"})
	if len(s.Messages()) != 2 {
		t.Errorf("after mutating m1, Messages() length = %d, want 2", len(s.Messages()))
	}
}

func TestAppend_UpdatesSessionState(t *testing.T) {
	s := NewSession("")
	if len(s.Messages()) != 0 {
		t.Fatalf("initial Messages() length = %d, want 0", len(s.Messages()))
	}
	s.Append(llm.Message{Role: "user", Content: "first"})
	msgs := s.Messages()
	if len(msgs) != 1 || msgs[0].Content != "first" {
		t.Errorf("after first Append: Messages() = %v", msgs)
	}
	s.Append(llm.Message{Role: "assistant", Content: "reply"})
	msgs = s.Messages()
	if len(msgs) != 2 || msgs[1].Content != "reply" {
		t.Errorf("after second Append: Messages() = %v", msgs)
	}
}

func TestID_Title_SetTitle(t *testing.T) {
	s := NewSession("old")
	if s.Title() != "old" {
		t.Errorf("Title() = %q, want old", s.Title())
	}
	s.SetTitle("new")
	if s.Title() != "new" {
		t.Errorf("after SetTitle: Title() = %q, want new", s.Title())
	}
}

func TestSaveToDir_CreatesFileWithValidJSON(t *testing.T) {
	dir := t.TempDir()
	s := NewSession("test title")
	s.Append(llm.Message{Role: "user", Content: "hello"})

	if err := SaveToDir(s, dir); err != nil {
		t.Fatalf("SaveToDir: %v", err)
	}
	path := filepath.Join(dir, s.ID()+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var f sessionFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("JSON invalid: %v", err)
	}
	if f.ID != s.ID() || f.Title != s.Title() {
		t.Errorf("id=%q title=%q, want %q %q", f.ID, f.Title, s.ID(), s.Title())
	}
	if f.CreatedAt == "" {
		t.Error("created_at empty")
	}
	if _, err := time.Parse(time.RFC3339, f.CreatedAt); err != nil {
		t.Errorf("created_at not RFC3339: %v", err)
	}
	if len(f.Messages) != 1 || f.Messages[0].Content != "hello" {
		t.Errorf("messages = %v", f.Messages)
	}
}

func TestLoadFromDir_AfterSaveReturnsSameSession(t *testing.T) {
	dir := t.TempDir()
	s := NewSession("round-trip")
	s.Append(llm.Message{Role: "user", Content: "a"})
	s.Append(llm.Message{Role: "assistant", Content: "b"})
	if err := SaveToDir(s, dir); err != nil {
		t.Fatalf("SaveToDir: %v", err)
	}

	loaded, err := LoadFromDir(dir, s.ID())
	if err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	if loaded.ID() != s.ID() {
		t.Errorf("ID() = %q, want %q", loaded.ID(), s.ID())
	}
	if loaded.Title() != s.Title() {
		t.Errorf("Title() = %q, want %q", loaded.Title(), s.Title())
	}
	// RFC3339 has second precision; round-trip may truncate subsecond
	if !loaded.CreatedAt().Truncate(time.Second).Equal(s.CreatedAt().Truncate(time.Second)) {
		t.Errorf("CreatedAt() = %v, want %v", loaded.CreatedAt(), s.CreatedAt())
	}
	got := loaded.Messages()
	want := s.Messages()
	if len(got) != len(want) {
		t.Fatalf("Messages() length = %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i].Role != want[i].Role || got[i].Content != want[i].Content {
			t.Errorf("Messages()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestLoadFromDir_MissingFileReturnsError(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadFromDir(dir, "nonexistent-id")
	if err == nil {
		t.Fatal("LoadFromDir: want error for missing file")
	}
	if !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("error should wrap ErrSessionNotFound: %v", err)
	}
}

func TestLoadFromDir_InvalidJSONReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad-id.json")
	if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := LoadFromDir(dir, "bad-id")
	if err == nil {
		t.Fatal("LoadFromDir: want error for invalid JSON")
	}
}

func TestSaveToDir_LoadFromDir_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := NewSession("")
	s.Append(llm.Message{Role: "user", Content: "Hi"})
	s.Append(llm.Message{Role: "assistant", Content: "Hello"})
	if err := SaveToDir(s, dir); err != nil {
		t.Fatalf("SaveToDir: %v", err)
	}
	loaded, err := LoadFromDir(dir, s.ID())
	if err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	got := loaded.Messages()
	want := s.Messages()
	if len(got) != len(want) {
		t.Fatalf("Messages() length = %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i].Role != want[i].Role || got[i].Content != want[i].Content {
			t.Errorf("Messages()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}
