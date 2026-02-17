package session

import (
	"context"
	"errors"
	"strings"
	"testing"

	"buildmax/internal/llm"
)

// fakeChatFunc returns a TitleChatClient that replies with the given fixed response.
func fakeChatFunc(reply string, err error) TitleChatClient {
	return TitleChatFunc(func(_ context.Context, _ []llm.Message) (string, error) {
		return reply, err
	})
}

func TestGenerateTitle_Success(t *testing.T) {
	s := NewSession("")
	s.Append(llm.Message{Role: "user", Content: "How do I sort a slice in Go?"})
	s.Append(llm.Message{Role: "assistant", Content: "You can use sort.Slice..."})

	title, err := GenerateTitle(context.Background(), fakeChatFunc("Sorting Slices in Go", nil), s.Messages())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "Sorting Slices in Go" {
		t.Errorf("title = %q, want %q", title, "Sorting Slices in Go")
	}
}

func TestGenerateTitle_StripsQuotes(t *testing.T) {
	s := NewSession("")
	s.Append(llm.Message{Role: "user", Content: "Hello"})

	title, err := GenerateTitle(context.Background(), fakeChatFunc(`"My Chat Title"`, nil), s.Messages())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "My Chat Title" {
		t.Errorf("title = %q, want %q", title, "My Chat Title")
	}
}

func TestGenerateTitle_LLMError_Fallthrough(t *testing.T) {
	s := NewSession("")
	s.Append(llm.Message{Role: "user", Content: "Hello"})

	_, err := GenerateTitle(context.Background(), fakeChatFunc("", errors.New("api error")), s.Messages())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGenerateTitle_NoUserMessage(t *testing.T) {
	title, err := GenerateTitle(context.Background(), fakeChatFunc("Should Not Call", nil), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "" {
		t.Errorf("title = %q, want empty", title)
	}
}

func TestGenerateTitle_TruncatesLongAssistantReply(t *testing.T) {
	s := NewSession("")
	s.Append(llm.Message{Role: "user", Content: "Tell me a story"})
	s.Append(llm.Message{Role: "assistant", Content: strings.Repeat("a", 2000)})

	// The chat func captures messages to verify assistant content was truncated.
	var captured []llm.Message
	titleClient := TitleChatFunc(func(_ context.Context, msgs []llm.Message) (string, error) {
		captured = msgs
		return "A Short Story", nil
	})
	title, err := GenerateTitle(context.Background(), titleClient, s.Messages())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "A Short Story" {
		t.Errorf("title = %q, want %q", title, "A Short Story")
	}

	// System + user + assistant = 3 messages.
	if len(captured) != 3 {
		t.Fatalf("expected 3 messages sent to LLM, got %d", len(captured))
	}
	assistantContent := captured[2].Content
	if len([]rune(assistantContent)) > 500 {
		t.Errorf("assistant content not truncated: %d runes", len([]rune(assistantContent)))
	}
}

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
