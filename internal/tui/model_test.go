package tui

import (
	"strings"
	"testing"

	"buildmax/internal/llm"
	"buildmax/internal/session"
)

func TestFormatMessage(t *testing.T) {
	tests := []struct {
		name     string
		msg      llm.Message
		wantSub  []string // each must appear in the joined output
		dontWant []string // none of these should appear
	}{
		{
			name: "user message",
			msg:  llm.Message{Role: "user", Content: "hello"},
			wantSub: []string{
				"you:",
				"hello",
			},
		},
		{
			name: "assistant message",
			msg:  llm.Message{Role: "assistant", Content: "Hi there"},
			wantSub: []string{
				"assistant:",
				"Hi there",
			},
		},
		{
			name: "assistant with tool_calls",
			msg: llm.Message{
				Role:    "assistant",
				Content: "Reading the file.",
				ToolCalls: []llm.ToolCall{
					{ID: "1", Name: "read_file", Arguments: `{"path":"/user/test/log.txt"}`},
				},
			},
			wantSub: []string{
				"assistant:",
				"Reading the file.",
				" * ",
				"read_file",
				"/user/test/log.txt",
			},
		},
		{
			name: "tool message",
			msg:  llm.Message{Role: "tool", Content: "file contents here"},
			wantSub: []string{
				" * result:",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := formatMessage(tt.msg)
			got := strings.Join(lines, "\n")
			for _, sub := range tt.wantSub {
				if !strings.Contains(got, sub) {
					t.Errorf("formatMessage() output %q does not contain %q", got, sub)
				}
			}
			for _, sub := range tt.dontWant {
				if strings.Contains(got, sub) {
					t.Errorf("formatMessage() output %q should not contain %q", got, sub)
				}
			}
		})
	}
}

func TestBuildViewportContent(t *testing.T) {
	// Use a real session so we need to import session and create one
	// buildViewportContent is in the same package and takes *session.Session
	// We can test via the session package
	sess := session.NewSession("")
	sess.Append(llm.Message{Role: "user", Content: "hi"})
	sess.Append(llm.Message{Role: "assistant", Content: "Hello!"})

	content := buildViewportContent(sess, "0.0.1", 80, false, 0)
	if !strings.Contains(content, "AI Agent TUI") {
		t.Errorf("buildViewportContent() should contain banner tagline, got: %s", content)
	}
	if !strings.Contains(content, "v0.0.1") {
		t.Errorf("buildViewportContent() should contain version v0.0.1, got: %s", content)
	}
	if !strings.Contains(content, "you:") {
		t.Errorf("buildViewportContent() should contain you:, got: %s", content)
	}
	if !strings.Contains(content, "assistant:") {
		t.Errorf("buildViewportContent() should contain assistant:, got: %s", content)
	}
}

func TestBuildViewportContent_BusyCarousel(t *testing.T) {
	sess := session.NewSession("")
	sess.Append(llm.Message{Role: "user", Content: "hi"})
	content := buildViewportContent(sess, "", 80, true, 0)
	if !strings.Contains(content, "assistant:") {
		t.Errorf("buildViewportContent(busy=true) should contain assistant:, got: %s", content)
	}
	if !strings.Contains(content, ".") {
		t.Errorf("buildViewportContent(busy=true, carouselDots=0) should contain carousel dot, got: %s", content)
	}
	content1 := buildViewportContent(sess, "", 80, true, 1)
	if !strings.Contains(content1, "..") {
		t.Errorf("buildViewportContent(carouselDots=1) should contain .., got: %s", content1)
	}
	content2 := buildViewportContent(sess, "", 80, true, 2)
	if !strings.Contains(content2, "...") {
		t.Errorf("buildViewportContent(carouselDots=2) should contain ..., got: %s", content2)
	}
}

func TestWrapLine(t *testing.T) {
	tests := []struct {
		line   string
		width  int
		expect int // number of lines
	}{
		{"short", 80, 1},
		{"", 10, 1},
		{"abcdefghij", 5, 2},
		{"abc", 3, 1},
		{"abcd", 2, 2},
	}
	for _, tt := range tests {
		got := wrapLine(tt.line, tt.width)
		if len(got) != tt.expect {
			t.Errorf("wrapLine(%q, %d) returned %d lines, want %d: %q", tt.line, tt.width, len(got), tt.expect, got)
		}
	}
	// Word wrap: break at space so "Operations" is not split into "Op" + "erations"
	got := wrapLine("**File Operations:**", 12)
	if len(got) != 2 {
		t.Fatalf("wrapLine(\"**File Operations:**\", 12) want 2 lines, got %d: %q", len(got), got)
	}
	if got[0] != "**File " || got[1] != "Operations:**" {
		t.Errorf("wrapLine(\"**File Operations:**\", 12) = %q, want [\"**File \", \"Operations:**\"]", got)
	}
}
