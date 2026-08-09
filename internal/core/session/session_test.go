package session

import (
	"github.com/gougoujiang/buildmax/internal/core/llm"
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
	s.Append(llm.Message{Role: "user", Content: "first"})
	if len(s.Messages) != 1 || s.Messages[0].Content != "first" {
		t.Errorf("after first Append: Messages = %v", s.Messages)
	}
	s.Append(llm.Message{Role: "assistant", Content: "reply"})
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
	s.Append(llm.Message{Role: "user", Content: "Hello world"})
	s.Append(llm.Message{Role: "assistant", Content: "Hi"})
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
	s.Append(llm.Message{Role: "user", Content: content})
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
	s.Append(llm.Message{Role: "user", Content: "user said this"})
	EnsureTitleFromFirstUserMessage(s, 100)
	if s.Title != "existing" {
		t.Errorf("Title() = %q, want existing (no-op)", s.Title)
	}
}

func TestEnsureTitleFromFirstUserMessage_NoOpWhenNoUserMessages(t *testing.T) {
	s := NewSession("")
	s.Append(llm.Message{Role: "assistant", Content: "only assistant"})
	EnsureTitleFromFirstUserMessage(s, 100)
	if s.Title != "" {
		t.Errorf("Title() = %q, want empty (no user message)", s.Title)
	}
}

func TestHistoryMessages_NoCompaction(t *testing.T) {
	s := NewSession("")
	s.Append(llm.Message{Role: "user", Content: "a"})
	s.Append(llm.Message{Role: "assistant", Content: "b"})
	got := s.HistoryMessages()
	if len(got) != 2 {
		t.Fatalf("HistoryMessages() len = %d, want 2", len(got))
	}
}

func TestAddCompaction_FiltersOldMessages(t *testing.T) {
	s := NewSession("")
	for i := 0; i < 5; i++ {
		s.Append(llm.Message{Role: "user", Content: "msg"})
	}
	// Compact first 3 messages.
	s.AddCompaction("summary of first 3", 3)
	got := s.HistoryMessages()
	if len(got) != 2 {
		t.Fatalf("after compacting 3 of 5, HistoryMessages() len = %d, want 2", len(got))
	}
	if s.CompactionSummary != "summary of first 3" {
		t.Errorf("CompactionSummary = %q", s.CompactionSummary)
	}
	if s.CompactionIdx != 3 {
		t.Errorf("CompactionIdx = %d, want 3", s.CompactionIdx)
	}
}

func TestAddCompaction_MultipleCompactions(t *testing.T) {
	s := NewSession("")
	for i := 0; i < 10; i++ {
		s.Append(llm.Message{Role: "user", Content: "msg"})
	}
	s.AddCompaction("summary 1", 4)
	s.AddCompaction("summary 2", 3) // compact 3 more of the remaining 6
	got := s.HistoryMessages()
	if len(got) != 3 {
		t.Fatalf("after two compactions (4+3 of 10), HistoryMessages() len = %d, want 3", len(got))
	}
	if s.CompactionIdx != 7 {
		t.Errorf("CompactionIdx = %d, want 7", s.CompactionIdx)
	}
	if s.CompactionSummary != "summary 2" {
		t.Errorf("CompactionSummary = %q, want 'summary 2'", s.CompactionSummary)
	}
}

func TestAddCompaction_ClampsToBounds(t *testing.T) {
	s := NewSession("")
	s.Append(llm.Message{Role: "user", Content: "only"})
	s.AddCompaction("summary", 999) // more than total messages
	got := s.HistoryMessages()
	if len(got) != 0 {
		t.Errorf("after over-compaction, HistoryMessages() len = %d, want 0", len(got))
	}
	if s.CompactionIdx != 1 {
		t.Errorf("CompactionIdx = %d, want 1 (clamped)", s.CompactionIdx)
	}
}
