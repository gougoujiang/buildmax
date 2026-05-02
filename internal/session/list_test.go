package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	llm "buildmax/internal/infra/llm"
)

func TestLoadList_MissingFile(t *testing.T) {
	dir := t.TempDir()
	entries, err := LoadList(dir)
	if err != nil {
		t.Errorf("LoadList(missing file): err = %v, want nil", err)
	}
	if entries == nil || len(entries) != 0 {
		t.Errorf("LoadList(missing file): entries = %v, want empty slice", entries)
	}
}

func TestLoadList_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, listFileName)
	if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatalf("write invalid file: %v", err)
	}
	entries, err := LoadList(dir)
	if err == nil {
		t.Error("LoadList(invalid JSON): err = nil, want non-nil")
	}
	if entries != nil {
		t.Errorf("LoadList(invalid JSON): entries = %v, want nil", entries)
	}
}

func TestLoadList_ValidRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := []ListEntry{
		{ID: "a", Title: "first", Workspace: "/ws1", CreatedAt: "2026-01-31T10:00:00Z"},
		{ID: "b", Title: "second", Workspace: "/ws2", CreatedAt: "2026-01-31T11:00:00Z"},
	}
	if err := UpsertListEntry(dir, want[0]); err != nil {
		t.Fatalf("UpsertListEntry first: %v", err)
	}
	if err := UpsertListEntry(dir, want[1]); err != nil {
		t.Fatalf("UpsertListEntry second: %v", err)
	}
	got, err := LoadList(dir)
	if err != nil {
		t.Fatalf("LoadList after write: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("LoadList: len = %d, want 2", len(got))
	}
	for i := range want {
		if got[i].ID != want[i].ID || got[i].Title != want[i].Title || got[i].Workspace != want[i].Workspace || got[i].CreatedAt != want[i].CreatedAt {
			t.Errorf("entry %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestUpsertListEntry_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, listFileName)
	if _, err := os.Stat(path); err == nil {
		t.Fatal("sessions.json should not exist yet")
	}
	entry := ListEntry{ID: "id1", Title: "t1", Workspace: "/w", CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := UpsertListEntry(dir, entry); err != nil {
		t.Fatalf("UpsertListEntry: %v", err)
	}
	entries, err := LoadList(dir)
	if err != nil {
		t.Fatalf("LoadList: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	if entries[0].ID != entry.ID || entries[0].Title != entry.Title || entries[0].Workspace != entry.Workspace || entries[0].CreatedAt != entry.CreatedAt {
		t.Errorf("got %+v, want %+v", entries[0], entry)
	}
}

func TestUpsertListEntry_UpdatesExisting(t *testing.T) {
	dir := t.TempDir()
	old := ListEntry{ID: "same", Title: "old title", Workspace: "/old", CreatedAt: "2026-01-31T10:00:00Z"}
	if err := UpsertListEntry(dir, old); err != nil {
		t.Fatalf("UpsertListEntry initial: %v", err)
	}
	updated := ListEntry{ID: "same", Title: "new title", Workspace: "/new", CreatedAt: "2026-01-31T10:00:00Z"}
	if err := UpsertListEntry(dir, updated); err != nil {
		t.Fatalf("UpsertListEntry update: %v", err)
	}
	entries, err := LoadList(dir)
	if err != nil {
		t.Fatalf("LoadList: %v", err)
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
	entries := []ListEntry{
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
	entries := []ListEntry{
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
	got = LastByCreatedAt([]ListEntry{})
	if got != nil {
		t.Errorf("LastByCreatedAt(empty) = %v, want nil", got)
	}
}

func TestLastByCreatedAt_ReturnsLatest(t *testing.T) {
	entries := []ListEntry{
		{ID: "old", CreatedAt: "2026-01-31T10:00:00Z"},
		{ID: "new", CreatedAt: "2026-01-31T12:00:00Z"},
		{ID: "mid", CreatedAt: "2026-01-31T11:00:00Z"},
	}
	got := LastByCreatedAt(entries)
	if got == nil || got.ID != "new" {
		t.Errorf("LastByCreatedAt = %v, want entry with ID new", got)
	}
}

func TestEnsureTitleFromFirstUserMessage_SetsTitle(t *testing.T) {
	s := NewSession("")
	s.Append(llm.Message{Role: "user", Content: "Hello world"})
	s.Append(llm.Message{Role: "assistant", Content: "Hi"})
	EnsureTitleFromFirstUserMessage(s, 100)
	if s.Title() != "Hello world" {
		t.Errorf("Title() = %q, want Hello world", s.Title())
	}
}

func TestEnsureTitleFromFirstUserMessage_Truncates(t *testing.T) {
	s := NewSession("")
	var content string
	for i := 0; i < 150; i++ {
		content += "x"
	}
	s.Append(llm.Message{Role: "user", Content: content})
	EnsureTitleFromFirstUserMessage(s, 100)
	got := s.Title()
	if len([]rune(got)) != 100 {
		t.Errorf("title rune length = %d, want 100", len([]rune(got)))
	}
	if got != content[:100] {
		t.Errorf("title not truncated correctly")
	}
}

func TestEnsureTitleFromFirstUserMessage_NoOpWhenTitleSet(t *testing.T) {
	s := NewSession("existing")
	s.Append(llm.Message{Role: "user", Content: "user said this"})
	EnsureTitleFromFirstUserMessage(s, 100)
	if s.Title() != "existing" {
		t.Errorf("Title() = %q, want existing (no-op)", s.Title())
	}
}

func TestEnsureTitleFromFirstUserMessage_NoOpWhenNoUserMessages(t *testing.T) {
	s := NewSession("")
	s.Append(llm.Message{Role: "assistant", Content: "only assistant"})
	EnsureTitleFromFirstUserMessage(s, 100)
	if s.Title() != "" {
		t.Errorf("Title() = %q, want empty (no user message)", s.Title())
	}
}
