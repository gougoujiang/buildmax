package session

import (
	"buildmax/internal/core/model"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewSession(t *testing.T) {
	title := "My chat"
	before := time.Now()
	s := NewSession(title)
	after := time.Now()

	if s.ID == "" {
		t.Error("ID() is empty")
	}
	if _, err := uuid.Parse(s.ID); err != nil {
		t.Errorf("ID() is not a valid UUID: %v", err)
	}
	if s.Title != title {
		t.Errorf("Title() = %q, want %q", s.Title, title)
	}
	createdAt := s.CreatedAt
	if createdAt.Before(before) || createdAt.After(after) {
		t.Errorf("CreatedAt() = %v, want between %v and %v", createdAt, before, after)
	}
	if s.Messages != nil {
		t.Errorf("Messages = %v, want nil (empty)", s.Messages)
	}
}

func TestNewSession_EmptyTitle(t *testing.T) {
	s := NewSession("")
	if s.Title != "" {
		t.Errorf("Title() = %q, want empty", s.Title)
	}
}

func TestAppend_UpdatesSessionState(t *testing.T) {
	s := NewSession("")
	if len(s.Messages) != 0 {
		t.Fatalf("initial len(Messages) = %d, want 0", len(s.Messages))
	}
	s.Append(model.Message{Role: "user", Content: "first"})
	if len(s.Messages) != 1 || s.Messages[0].Content != "first" {
		t.Errorf("after first Append: Messages = %v", s.Messages)
	}
	s.Append(model.Message{Role: "assistant", Content: "reply"})
	if len(s.Messages) != 2 || s.Messages[1].Content != "reply" {
		t.Errorf("after second Append: Messages = %v", s.Messages)
	}
}

func TestID_Title_SetTitle(t *testing.T) {
	s := NewSession("old")
	if s.Title != "old" {
		t.Errorf("Title() = %q, want old", s.Title)
	}
	s.Title = "new"
	if s.Title != "new" {
		t.Errorf("after SetTitle: Title() = %q, want new", s.Title)
	}
}

func TestEnsureTitleFromFirstUserMessage_SetsTitle(t *testing.T) {
	s := NewSession("")
	s.Append(model.Message{Role: "user", Content: "Hello world"})
	s.Append(model.Message{Role: "assistant", Content: "Hi"})
	EnsureTitleFromFirstUserMessage(s, 100)
	if s.Title != "Hello world" {
		t.Errorf("Title() = %q, want Hello world", s.Title)
	}
}

func TestEnsureTitleFromFirstUserMessage_Truncates(t *testing.T) {
	s := NewSession("")
	var content string
	for i := 0; i < 150; i++ {
		content += "x"
	}
	s.Append(model.Message{Role: "user", Content: content})
	EnsureTitleFromFirstUserMessage(s, 100)
	got := s.Title
	if len([]rune(got)) != 100 {
		t.Errorf("title rune length = %d, want 100", len([]rune(got)))
	}
	if got != content[:100] {
		t.Errorf("title not truncated correctly")
	}
}

func TestEnsureTitleFromFirstUserMessage_NoOpWhenTitleSet(t *testing.T) {
	s := NewSession("existing")
	s.Append(model.Message{Role: "user", Content: "user said this"})
	EnsureTitleFromFirstUserMessage(s, 100)
	if s.Title != "existing" {
		t.Errorf("Title() = %q, want existing (no-op)", s.Title)
	}
}

func TestEnsureTitleFromFirstUserMessage_NoOpWhenNoUserMessages(t *testing.T) {
	s := NewSession("")
	s.Append(model.Message{Role: "assistant", Content: "only assistant"})
	EnsureTitleFromFirstUserMessage(s, 100)
	if s.Title != "" {
		t.Errorf("Title() = %q, want empty (no user message)", s.Title)
	}
}

func TestSaveToDir_CreatesFileWithValidJSON(t *testing.T) {
	dir := t.TempDir()
	s := NewSession("test title")
	s.Append(model.Message{Role: "user", Content: "hello"})

	if err := SaveToDir(s, dir); err != nil {
		t.Fatalf("SaveToDir: %v", err)
	}
	path := filepath.Join(dir, s.ID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var got Session
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("JSON invalid: %v", err)
	}
	if got.ID != s.ID || got.Title != s.Title {
		t.Errorf("id=%q title=%q, want %q %q", got.ID, got.Title, s.ID, s.Title)
	}
	if got.CreatedAt.IsZero() {
		t.Error("created_at empty")
	}
	if len(got.Messages) != 1 || got.Messages[0].Content != "hello" {
		t.Errorf("messages = %v", got.Messages)
	}
}

func TestLoadFromDir_AfterSaveReturnsSameSession(t *testing.T) {
	dir := t.TempDir()
	s := NewSession("round-trip")
	s.Append(model.Message{Role: "user", Content: "a"})
	s.Append(model.Message{Role: "assistant", Content: "b"})
	if err := SaveToDir(s, dir); err != nil {
		t.Fatalf("SaveToDir: %v", err)
	}

	loaded, err := LoadFromDir(dir, s.ID)
	if err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	if loaded.ID != s.ID {
		t.Errorf("ID() = %q, want %q", loaded.ID, s.ID)
	}
	if loaded.Title != s.Title {
		t.Errorf("Title() = %q, want %q", loaded.Title, s.Title)
	}
	// RFC3339 has second precision; round-trip may truncate subsecond
	if !loaded.CreatedAt.Truncate(time.Second).Equal(s.CreatedAt.Truncate(time.Second)) {
		t.Errorf("CreatedAt() = %v, want %v", loaded.CreatedAt, s.CreatedAt)
	}
	got := loaded.Messages
	want := s.Messages
	if len(got) != len(want) {
		t.Fatalf("len(Messages) = %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i].Role != want[i].Role || got[i].Content != want[i].Content {
			t.Errorf("Messages[%d] = %+v, want %+v", i, got[i], want[i])
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
	s.Append(model.Message{Role: "user", Content: "Hi"})
	s.Append(model.Message{Role: "assistant", Content: "Hello"})
	if err := SaveToDir(s, dir); err != nil {
		t.Fatalf("SaveToDir: %v", err)
	}
	loaded, err := LoadFromDir(dir, s.ID)
	if err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	got := loaded.Messages
	want := s.Messages
	if len(got) != len(want) {
		t.Fatalf("len(Messages) = %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i].Role != want[i].Role || got[i].Content != want[i].Content {
			t.Errorf("Messages[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestSaveToDir_LoadFromDir_UsageRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := NewSession("")
	s.Append(model.Message{Role: "user", Content: "Hi"})
	s.PromptTokens, s.CompletionTokens = 50, 30
	if err := SaveToDir(s, dir); err != nil {
		t.Fatalf("SaveToDir: %v", err)
	}
	loaded, err := LoadFromDir(dir, s.ID)
	if err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	if loaded.PromptTokens != 50 || loaded.CompletionTokens != 30 {
		t.Errorf("loaded usage: prompt=%d completion=%d, want 50, 30", loaded.PromptTokens, loaded.CompletionTokens)
	}
	loaded.PromptTokens += 10; loaded.CompletionTokens += 5
	if loaded.PromptTokens != 60 || loaded.CompletionTokens != 35 {
		t.Errorf("after AddUsage: prompt=%d completion=%d, want 60, 35", loaded.PromptTokens, loaded.CompletionTokens)
	}
}

func TestLoadSessions_MissingFile(t *testing.T) {
	dir := t.TempDir()
	entries, err := LoadSessions(dir)
	if err != nil {
		t.Errorf("LoadSessions(missing file): err = %v, want nil", err)
	}
	if entries == nil || len(entries) != 0 {
		t.Errorf("LoadSessions(missing file): entries = %v, want empty slice", entries)
	}
}

func TestLoadSessions_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")
	if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatalf("write invalid file: %v", err)
	}
	entries, err := LoadSessions(dir)
	if err == nil {
		t.Error("LoadSessions(invalid JSON): err = nil, want non-nil")
	}
	if entries != nil {
		t.Errorf("LoadSessions(invalid JSON): entries = %v, want nil", entries)
	}
}

func TestLoadSessions_ValidRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := []SessionItem{
		{ID: "a", Title: "first", Workspace: "/ws1", CreatedAt: "2026-01-31T10:00:00Z"},
		{ID: "b", Title: "second", Workspace: "/ws2", CreatedAt: "2026-01-31T11:00:00Z"},
	}
	if err := UpsertSessionItem(dir, want[0]); err != nil {
		t.Fatalf("UpsertSessionItem first: %v", err)
	}
	if err := UpsertSessionItem(dir, want[1]); err != nil {
		t.Fatalf("UpsertSessionItem second: %v", err)
	}
	got, err := LoadSessions(dir)
	if err != nil {
		t.Fatalf("LoadSessions after write: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("LoadSessions: len = %d, want 2", len(got))
	}
	for i := range want {
		if got[i].ID != want[i].ID || got[i].Title != want[i].Title || got[i].Workspace != want[i].Workspace || got[i].CreatedAt != want[i].CreatedAt {
			t.Errorf("entry %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestUpsertSessionItem_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")
	if _, err := os.Stat(path); err == nil {
		t.Fatal("sessions.json should not exist yet")
	}
	entry := SessionItem{ID: "id1", Title: "t1", Workspace: "/w", CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := UpsertSessionItem(dir, entry); err != nil {
		t.Fatalf("UpsertSessionItem: %v", err)
	}
	entries, err := LoadSessions(dir)
	if err != nil {
		t.Fatalf("LoadSessions: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	if entries[0].ID != entry.ID || entries[0].Title != entry.Title || entries[0].Workspace != entry.Workspace || entries[0].CreatedAt != entry.CreatedAt {
		t.Errorf("got %+v, want %+v", entries[0], entry)
	}
}

func TestUpsertSessionItem_UpdatesExisting(t *testing.T) {
	dir := t.TempDir()
	old := SessionItem{ID: "same", Title: "old title", Workspace: "/old", CreatedAt: "2026-01-31T10:00:00Z"}
	if err := UpsertSessionItem(dir, old); err != nil {
		t.Fatalf("UpsertSessionItem initial: %v", err)
	}
	updated := SessionItem{ID: "same", Title: "new title", Workspace: "/new", CreatedAt: "2026-01-31T10:00:00Z"}
	if err := UpsertSessionItem(dir, updated); err != nil {
		t.Fatalf("UpsertSessionItem update: %v", err)
	}
	entries, err := LoadSessions(dir)
	if err != nil {
		t.Fatalf("LoadSessions: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	if entries[0].Title != "new title" || entries[0].Workspace != "/new" {
		t.Errorf("title/workspace not updated: got %+v", entries[0])
	}
	if entries[0].CreatedAt != old.CreatedAt {
		t.Errorf("created_at changed: got %q, want %q", entries[0].CreatedAt, old.CreatedAt)
	}
}

func TestSortByCreatedAtDesc(t *testing.T) {
	entries := []SessionItem{
		{ID: "old", Title: "a", CreatedAt: "2026-01-31T10:00:00Z"},
		{ID: "new", Title: "b", CreatedAt: "2026-01-31T14:00:00Z"},
		{ID: "mid", Title: "c", CreatedAt: "2026-01-31T12:00:00Z"},
	}
	SortByCreatedAtDesc(entries)
	if entries[0].ID != "new" || entries[1].ID != "mid" || entries[2].ID != "old" {
		t.Errorf("order = %s, %s, %s, want new, mid, old", entries[0].ID, entries[1].ID, entries[2].ID)
	}
}

func TestSortByCreatedAtDesc_InvalidTimesLast(t *testing.T) {
	entries := []SessionItem{
		{ID: "bad", Title: "x", CreatedAt: "not-a-date"},
		{ID: "good", Title: "y", CreatedAt: "2026-01-31T12:00:00Z"},
	}
	SortByCreatedAtDesc(entries)
	if entries[0].ID != "good" || entries[1].ID != "bad" {
		t.Errorf("order = %s, %s, want good, bad", entries[0].ID, entries[1].ID)
	}
}

func TestLastByCreatedAt_Empty(t *testing.T) {
	got := LastByCreatedAt(nil)
	if got != nil {
		t.Errorf("LastByCreatedAt(nil) = %v, want nil", got)
	}
	got = LastByCreatedAt([]SessionItem{})
	if got != nil {
		t.Errorf("LastByCreatedAt(empty) = %v, want nil", got)
	}
}

func TestLastByCreatedAt_ReturnsLatest(t *testing.T) {
	entries := []SessionItem{
		{ID: "old", CreatedAt: "2026-01-31T10:00:00Z"},
		{ID: "new", CreatedAt: "2026-01-31T12:00:00Z"},
		{ID: "mid", CreatedAt: "2026-01-31T11:00:00Z"},
	}
	got := LastByCreatedAt(entries)
	if got == nil || got.ID != "new" {
		t.Errorf("LastByCreatedAt = %v, want entry with ID new", got)
	}
}
