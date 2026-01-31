package session

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"buildmax/internal/llm"
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
