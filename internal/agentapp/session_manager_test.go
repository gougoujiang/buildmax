package agentapp

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/session"
)

// --- session persistence ---

func TestSaveSession_CreatesFileWithValidJSON(t *testing.T) {
	dir := t.TempDir()
	s := session.NewSession("test title")
	if err := s.Append(llm.Message{Role: "user", Content: "hello"}); err != nil {
		t.Fatal(err)
	}

	if err := saveSession(s, dir); err != nil {
		t.Fatalf("saveSession: %v", err)
	}
	path := filepath.Join(dir, s.ID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var got session.Session
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

func TestLoadSession_AfterSaveReturnsSameSession(t *testing.T) {
	dir := t.TempDir()
	s := session.NewSession("round-trip")
	if err := s.Append(llm.Message{Role: "user", Content: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Append(llm.Message{Role: "assistant", Content: "b"}); err != nil {
		t.Fatal(err)
	}
	if err := saveSession(s, dir); err != nil {
		t.Fatalf("saveSession: %v", err)
	}

	loaded, err := LoadSession(dir, s.ID)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if loaded.ID != s.ID {
		t.Errorf("ID() = %q, want %q", loaded.ID, s.ID)
	}
	if loaded.Title != s.Title {
		t.Errorf("Title() = %q, want %q", loaded.Title, s.Title)
	}
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

func TestLoadSession_MissingFileReturnsError(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadSession(dir, "nonexistent-id")
	if err == nil {
		t.Fatal("LoadSession: want error for missing file")
	}
	if !errors.Is(err, session.ErrSessionNotFound) {
		t.Errorf("error should wrap ErrSessionNotFound: %v", err)
	}
}

func TestLoadSession_InvalidJSONReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad-id.json")
	if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := LoadSession(dir, "bad-id")
	if err == nil {
		t.Fatal("LoadSession: want error for invalid JSON")
	}
}

func TestSaveLoad_UsageRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := session.NewSession("")
	if err := s.Append(llm.Message{Role: "user", Content: "Hi"}); err != nil {
		t.Fatal(err)
	}
	s.PromptTokens, s.CompletionTokens = 50, 30
	// A resumed session keeps its cached breakdown too. Losing it on restart
	// would make a long cached session report the same totals as an uncached
	// one, which is exactly the comparison caching exists to change.
	s.CacheReadTokens, s.CacheWriteTokens = 40, 10
	if err := saveSession(s, dir); err != nil {
		t.Fatalf("saveSession: %v", err)
	}
	loaded, err := LoadSession(dir, s.ID)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if loaded.PromptTokens != 50 || loaded.CompletionTokens != 30 {
		t.Errorf("loaded usage: prompt=%d completion=%d, want 50, 30", loaded.PromptTokens, loaded.CompletionTokens)
	}
	if loaded.CacheReadTokens != 40 || loaded.CacheWriteTokens != 10 {
		t.Errorf("loaded cache usage: read=%d write=%d, want 40, 10", loaded.CacheReadTokens, loaded.CacheWriteTokens)
	}
}

// TestSaveLoad_DurableStateRoundTrip covers the property that makes notes worth having: they
// are stored on the session, not in the message list, so they outlive both trimming and a
// restart. If they did not persist they would be no better than a tool result.
func TestSaveLoad_DurableStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := session.NewSession("")
	s.SetNotes([]agent.Note{{Text: "the client is the lessee"}}, 12)
	s.SetTodos([]agent.Todo{{Content: "draft the notice", Status: agent.TodoInProgress}}, 12)
	if err := saveSession(s, dir); err != nil {
		t.Fatalf("saveSession: %v", err)
	}

	loaded, err := LoadSession(dir, s.ID)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if len(loaded.Notes()) != 1 || loaded.Notes()[0].Text != "the client is the lessee" {
		t.Fatalf("notes = %+v, want the stored note", loaded.Notes())
	}
	if loaded.Notes()[0].WrittenIteration != 12 {
		t.Errorf("note WrittenIteration = %d, want 12", loaded.Notes()[0].WrittenIteration)
	}
	if len(loaded.Todos()) != 1 || loaded.Todos()[0].Status != agent.TodoInProgress {
		t.Errorf("todos = %+v, want one in-progress entry", loaded.Todos())
	}
}

func TestLoadSessionList_MissingFile(t *testing.T) {
	dir := t.TempDir()
	entries, err := LoadSessionList(dir)
	if err != nil {
		t.Errorf("LoadSessionList(missing file): err = %v, want nil", err)
	}
	if entries == nil || len(entries) != 0 {
		t.Errorf("LoadSessionList(missing file): entries = %v, want empty slice", entries)
	}
}

func TestLoadSessionList_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")
	if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatalf("write invalid file: %v", err)
	}
	entries, err := LoadSessionList(dir)
	if err == nil {
		t.Error("LoadSessionList(invalid JSON): err = nil, want non-nil")
	}
	if entries != nil {
		t.Errorf("LoadSessionList(invalid JSON): entries = %v, want nil", entries)
	}
}

func TestLoadSessionList_ValidRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := []session.SessionItem{
		{ID: "a", Title: "first", Workspace: "/ws1", CreatedAt: "2026-01-31T10:00:00Z"},
		{ID: "b", Title: "second", Workspace: "/ws2", CreatedAt: "2026-01-31T11:00:00Z"},
	}
	if err := UpsertSessionItem(dir, want[0]); err != nil {
		t.Fatalf("UpsertSessionItem first: %v", err)
	}
	if err := UpsertSessionItem(dir, want[1]); err != nil {
		t.Fatalf("UpsertSessionItem second: %v", err)
	}
	got, err := LoadSessionList(dir)
	if err != nil {
		t.Fatalf("LoadSessionList after write: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("LoadSessionList: len = %d, want 2", len(got))
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
	entry := session.SessionItem{ID: "id1", Title: "t1", Workspace: "/w", CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := UpsertSessionItem(dir, entry); err != nil {
		t.Fatalf("UpsertSessionItem: %v", err)
	}
	entries, err := LoadSessionList(dir)
	if err != nil {
		t.Fatalf("LoadSessionList: %v", err)
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
	old := session.SessionItem{ID: "same", Title: "old title", Workspace: "/old", CreatedAt: "2026-01-31T10:00:00Z"}
	if err := UpsertSessionItem(dir, old); err != nil {
		t.Fatalf("UpsertSessionItem initial: %v", err)
	}
	updated := session.SessionItem{ID: "same", Title: "new title", Workspace: "/new", CreatedAt: "2026-01-31T10:00:00Z"}
	if err := UpsertSessionItem(dir, updated); err != nil {
		t.Fatalf("UpsertSessionItem update: %v", err)
	}
	entries, err := LoadSessionList(dir)
	if err != nil {
		t.Fatalf("LoadSessionList: %v", err)
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

func TestRenameSession_UpdatesIndexAndSessionFile(t *testing.T) {
	dir := t.TempDir()
	s := session.NewSession("old")
	if err := saveSession(s, dir); err != nil {
		t.Fatalf("saveSession: %v", err)
	}
	if err := UpsertSessionItem(dir, session.SessionItem{ID: s.ID, Title: "old", Workspace: "/w", CreatedAt: s.CreatedAt.Format(time.RFC3339)}); err != nil {
		t.Fatalf("UpsertSessionItem: %v", err)
	}
	if err := RenameSession(dir, s.ID, "new title"); err != nil {
		t.Fatalf("RenameSession: %v", err)
	}
	entries, err := LoadSessionList(dir)
	if err != nil {
		t.Fatalf("LoadSessionList: %v", err)
	}
	if entries[0].Title != "new title" {
		t.Fatalf("index title = %q, want new title", entries[0].Title)
	}
	loaded, err := LoadSession(dir, s.ID)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if loaded.Title != "new title" {
		t.Fatalf("session title = %q, want new title", loaded.Title)
	}
}

func TestSetSessionPinned_PreservesAcrossUpsert(t *testing.T) {
	dir := t.TempDir()
	item := session.SessionItem{ID: "id1", Title: "title", Workspace: "/w", CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := UpsertSessionItem(dir, item); err != nil {
		t.Fatalf("UpsertSessionItem: %v", err)
	}
	if err := SetSessionPinned(dir, item.ID, true); err != nil {
		t.Fatalf("SetSessionPinned: %v", err)
	}
	item.Title = "updated"
	if err := UpsertSessionItem(dir, item); err != nil {
		t.Fatalf("UpsertSessionItem update: %v", err)
	}
	entries, err := LoadSessionList(dir)
	if err != nil {
		t.Fatalf("LoadSessionList: %v", err)
	}
	if !entries[0].Pinned {
		t.Fatal("pinned flag should survive session upsert")
	}
	if entries[0].Title != "updated" {
		t.Fatalf("title = %q, want updated", entries[0].Title)
	}
}

func TestDeleteSessionsByWorkspace(t *testing.T) {
	dir := t.TempDir()
	items := []session.SessionItem{
		{ID: "a", Title: "a", Workspace: "/w", CreatedAt: "2026-01-31T10:00:00Z"},
		{ID: "b", Title: "b", Workspace: "/other", CreatedAt: "2026-01-31T11:00:00Z"},
	}
	for _, item := range items {
		if err := UpsertSessionItem(dir, item); err != nil {
			t.Fatalf("UpsertSessionItem: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, item.ID+".json"), []byte("{}"), 0644); err != nil {
			t.Fatalf("write session file: %v", err)
		}
	}
	deleted, err := DeleteSessionsByWorkspace(dir, "/w")
	if err != nil {
		t.Fatalf("DeleteSessionsByWorkspace: %v", err)
	}
	if len(deleted) != 1 || deleted[0] != "a" {
		t.Fatalf("deleted = %v, want [a]", deleted)
	}
	entries, err := LoadSessionList(dir)
	if err != nil {
		t.Fatalf("LoadSessionList: %v", err)
	}
	if len(entries) != 1 || entries[0].ID != "b" {
		t.Fatalf("entries = %+v, want only b", entries)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.json")); !os.IsNotExist(err) {
		t.Fatalf("a.json should be deleted, err=%v", err)
	}
}

func TestDeleteSession_RemovesIndexEntryAndFile(t *testing.T) {
	dir := t.TempDir()
	for _, id := range []string{"keep", "drop"} {
		if err := UpsertSessionItem(dir, session.SessionItem{ID: id, Title: id, CreatedAt: "2026-01-31T10:00:00Z"}); err != nil {
			t.Fatalf("UpsertSessionItem: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, id+".json"), []byte("{}"), 0644); err != nil {
			t.Fatalf("write session file: %v", err)
		}
	}

	if err := DeleteSession(dir, "drop"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	entries, err := LoadSessionList(dir)
	if err != nil {
		t.Fatalf("LoadSessionList: %v", err)
	}
	if len(entries) != 1 || entries[0].ID != "keep" {
		t.Fatalf("entries = %+v, want only keep", entries)
	}
	if _, err := os.Stat(filepath.Join(dir, "drop.json")); !os.IsNotExist(err) {
		t.Fatalf("drop.json should be deleted, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "keep.json")); err != nil {
		t.Fatalf("keep.json should survive: %v", err)
	}
}

// --- atomic replacement ---

// sessionDirNoise reports files in dir that are neither a session file nor the
// index. Replacement writes a temp file first, so a leaked one shows up here.
func sessionDirNoise(t *testing.T, dir string, want map[string]bool) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	var out []string
	for _, e := range entries {
		if !want[e.Name()] {
			out = append(out, e.Name())
		}
	}
	return out
}

func TestSessionWrites_LeaveNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	s := session.NewSession("first")
	if err := s.Append(llm.Message{Role: "user", Content: "hello"}); err != nil {
		t.Fatal(err)
	}

	// Two rounds, because the second replaces files the first created and that
	// is the path where a leftover temp file would appear.
	for round := range 2 {
		if err := saveSession(s, dir); err != nil {
			t.Fatalf("saveSession round %d: %v", round, err)
		}
		if err := upsertSessionItem(dir, session.SessionItem{ID: s.ID, Title: s.Title, CreatedAt: "2026-01-31T10:00:00Z"}); err != nil {
			t.Fatalf("upsertSessionItem round %d: %v", round, err)
		}
	}

	want := map[string]bool{s.ID + ".json": true, "sessions.json": true}
	if noise := sessionDirNoise(t, dir, want); noise != nil {
		t.Errorf("unexpected files in session dir: %v", noise)
	}
}

func TestSaveSession_ReplacingWithShorterContentLeavesNoTail(t *testing.T) {
	dir := t.TempDir()
	s := session.NewSession("a deliberately long session title that makes the first file bigger")
	for range 20 {
		if err := s.Append(llm.Message{Role: "user", Content: strings.Repeat("padding ", 40)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := saveSession(s, dir); err != nil {
		t.Fatalf("saveSession: %v", err)
	}

	// Replacement, not overwrite: the shorter document must not sit on top of
	// the longer one's tail and leave the file unparsable.
	short := session.NewSession("short")
	short.ID = s.ID
	if err := saveSession(short, dir); err != nil {
		t.Fatalf("saveSession short: %v", err)
	}

	got, err := LoadSession(dir, s.ID)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if got.Title != "short" {
		t.Errorf("title = %q, want %q", got.Title, "short")
	}
	if len(got.Messages) != 0 {
		t.Errorf("messages = %d, want 0", len(got.Messages))
	}
}

// --- title generation ---

func TestCleanTitle(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`  "Sorting Slices"  `, "Sorting Slices"},
		{`'Hello World'`, "Hello World"},
		{"  Plain Title  ", "Plain Title"},
		{strings.Repeat("x", 200), strings.Repeat("x", 100)},
		{`""`, ""},
	}
	for _, tt := range tests {
		got := cleanTitle(tt.input)
		if got != tt.want {
			t.Errorf("cleanTitle(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
