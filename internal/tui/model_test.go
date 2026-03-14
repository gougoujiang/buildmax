package tui

import (
	"strings"
	"testing"

	"buildmax/internal/llm"
	"buildmax/internal/session"

	tea "github.com/charmbracelet/bubbletea"
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
				"hello",
			},
		},
		{
			name: "assistant message",
			msg:  llm.Message{Role: "assistant", Content: "Hi there"},
			wantSub: []string{
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

	content := buildViewportContent(sess, ViewportContentOpts{Version: "0.0.1", Width: 80})
	if !strings.Contains(content, "v0.0.1") {
		t.Errorf("buildViewportContent() should contain version v0.0.1, got: %s", content)
	}
	if !strings.Contains(content, ">") {
		t.Errorf("buildViewportContent() should contain > for user line, got: %s", content)
	}
	if !strings.Contains(content, "hi") {
		t.Errorf("buildViewportContent() should contain user message hi, got: %s", content)
	}
	if !strings.Contains(content, "•") {
		t.Errorf("buildViewportContent() should contain • for assistant line, got: %s", content)
	}
	if !strings.Contains(content, "Hello!") {
		t.Errorf("buildViewportContent() should contain assistant message Hello!, got: %s", content)
	}
}

func TestBuildViewportContent_Margin(t *testing.T) {
	sess := session.NewSession("")
	sess.Append(llm.Message{Role: "user", Content: "hi"})
	sess.Append(llm.Message{Role: "assistant", Content: "Hello!"})
	content := buildViewportContent(sess, ViewportContentOpts{Width: 80})
	// After banner, message lines must start with left margin (two spaces) before ">" or "•".
	lines := strings.Split(content, "\n")
	var foundMargin bool
	for _, line := range lines {
		if strings.Contains(line, ">") || strings.Contains(line, "•") {
			if strings.HasPrefix(line, "  ") {
				foundMargin = true
				break
			}
		}
	}
	if !foundMargin {
		t.Errorf("buildViewportContent() should have message lines starting with two spaces (margin), got content (excerpt): %s", content[:min(200, len(content))])
	}
}

func TestBuildViewportContent_WrapByContentWidth(t *testing.T) {
	// Long plain line: with width 80 and margins, content width is 74. First line of assistant message should have many visible chars (no early ANSI wrap).
	longLine := strings.Repeat("a", 100)
	sess := session.NewSession("")
	sess.Append(llm.Message{Role: "assistant", Content: longLine})
	content := buildViewportContent(sess, ViewportContentOpts{Width: 80})
	lines := strings.Split(content, "\n")
	var firstBulletLine string
	for _, line := range lines {
		if strings.Contains(line, "•") && strings.Contains(line, "a") {
			firstBulletLine = line
			break
		}
	}
	if firstBulletLine == "" {
		t.Fatal("buildViewportContent() should contain a line with bullet and content")
	}
	// Line is margin (2) + ANSI + "• " + content. Rune count should be at least 74 (margin + prefix + 70 content).
	if len([]rune(firstBulletLine)) < 74 {
		t.Errorf("buildViewportContent() first assistant line should have at least 74 runes (wrap by content width), got %d: %q", len([]rune(firstBulletLine)), firstBulletLine)
	}
}

func TestBuildViewportContent_BusyCarousel(t *testing.T) {
	sess := session.NewSession("")
	sess.Append(llm.Message{Role: "user", Content: "hi"})
	content := buildViewportContent(sess, ViewportContentOpts{Width: 80, Busy: true})
	if !strings.Contains(content, "•") {
		t.Errorf("buildViewportContent(busy=true) should contain • for carousel, got: %s", content)
	}
	if !strings.Contains(content, ".") {
		t.Errorf("buildViewportContent(busy=true, carouselDots=0) should contain carousel dot, got: %s", content)
	}
	content1 := buildViewportContent(sess, ViewportContentOpts{Width: 80, Busy: true, CarouselDots: 1})
	if !strings.Contains(content1, "..") {
		t.Errorf("buildViewportContent(carouselDots=1) should contain .., got: %s", content1)
	}
	content2 := buildViewportContent(sess, ViewportContentOpts{Width: 80, Busy: true, CarouselDots: 2})
	if !strings.Contains(content2, "...") {
		t.Errorf("buildViewportContent(carouselDots=2) should contain ..., got: %s", content2)
	}
}

func TestModelFocusToggle(t *testing.T) {
	opts := TUIOpts{
		Session: session.NewSession(""),
		Version: "0.0.1",
	}
	m := NewModel(opts)
	if !m.FocusInput() {
		t.Error("initial focus should be on input")
	}
	// Tab: switch to viewport focus
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	mod, ok := m2.(*Model)
	if !ok {
		t.Fatalf("Update returned %T, want *Model", m2)
	}
	if mod.FocusInput() {
		t.Error("after Tab, focus should be on viewport")
	}
	// Tab again: switch back to input focus
	m3, _ := mod.Update(tea.KeyMsg{Type: tea.KeyTab})
	mod2, ok := m3.(*Model)
	if !ok {
		t.Fatalf("Update returned %T, want *Model", m3)
	}
	if !mod2.FocusInput() {
		t.Error("after second Tab, focus should be on input again")
	}
}

func TestViewKeepsFooterAtBottom(t *testing.T) {
	sess := session.NewSession("")
	sess.Append(llm.Message{Role: "assistant", Content: "short"})
	m := NewModel(TUIOpts{
		Session:   sess,
		ModelName: "test-model",
		Workspace: "/tmp/workspace",
		Version:   "0.0.1",
	})

	m2, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	mod, ok := m2.(*Model)
	if !ok {
		t.Fatalf("Update returned %T, want *Model", m2)
	}

	lines := strings.Split(mod.View(), "\n")
	if got := len(lines); got != 12 {
		t.Fatalf("View() rendered %d lines, want terminal height 12", got)
	}
	last := lines[len(lines)-1]
	if !strings.Contains(last, "model: test-model") {
		t.Fatalf("footer should be on the last line, got %q", last)
	}
}

func TestViewportScrollShowsOlderMessages(t *testing.T) {
	sess := session.NewSession("")
	for i := 0; i < 8; i++ {
		sess.Append(llm.Message{Role: "assistant", Content: strings.Repeat(string(rune('A'+i)), 18)})
	}
	m := NewModel(TUIOpts{
		Session: sess,
		Version: "0.0.1",
	})

	m2, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 10})
	mod, ok := m2.(*Model)
	if !ok {
		t.Fatalf("Update returned %T, want *Model", m2)
	}
	before := mod.View()
	if !strings.Contains(before, "HHHHHHHHHHHHHHHHHH") {
		t.Fatalf("initial view should show the latest message, got %q", before)
	}

	m3, _ := mod.Update(tea.KeyMsg{Type: tea.KeyUp})
	scrolled, ok := m3.(*Model)
	if !ok {
		t.Fatalf("Update returned %T, want *Model", m3)
	}
	after := scrolled.View()
	if !strings.Contains(after, "GGGGGGGGGGGGGGGGGG") {
		t.Fatalf("scrolled view should expose older messages, got %q", after)
	}
	if after == before {
		t.Fatal("scrolling up should change the visible viewport content")
	}
}

func TestMouseWheelOverInputScrollsTextareaInsteadOfChat(t *testing.T) {
	sess := session.NewSession("")
	sess.Append(llm.Message{Role: "assistant", Content: "chat-bottom"})

	m := NewModel(TUIOpts{
		Session: sess,
		Version: "0.0.1",
	})
	m.inputBlock.SetValue(strings.Join([]string{
		"input-1",
		"input-2",
		"input-3",
		"input-4",
		"input-5",
		"input-6",
	}, "\n"))
	m.inputBlock.SyncHeight()

	m2, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 10})
	mod, ok := m2.(*Model)
	if !ok {
		t.Fatalf("Update returned %T, want *Model", m2)
	}

	before := mod.View()
	if !strings.Contains(before, "input-1") {
		t.Fatalf("initial input view should show the first input line, got %q", before)
	}
	if strings.Contains(before, "input-6") {
		t.Fatalf("initial input view should not show the last input line yet, got %q", before)
	}

	m3, _ := mod.Update(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonWheelDown,
	})
	scrolled, ok := m3.(*Model)
	if !ok {
		t.Fatalf("Update returned %T, want *Model", m3)
	}

	after := scrolled.View()
	if !strings.Contains(after, "input-6") {
		t.Fatalf("mouse wheel over input should reveal lower input lines, got %q", after)
	}
	if !strings.Contains(after, "chat-bottom") {
		t.Fatalf("mouse wheel over input should not hide chat viewport content, got %q", after)
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
	// Word wrap: break at space so "**File " is first line; remainder wraps to "Operations:*" then "*" (3 lines total)
	got := wrapLine("**File Operations:**", 12)
	if len(got) != 3 {
		t.Fatalf("wrapLine(\"**File Operations:**\", 12) want 3 lines, got %d: %q", len(got), got)
	}
	if got[0] != "**File " || got[1] != "Operations:*" || got[2] != "*" {
		t.Errorf("wrapLine(\"**File Operations:**\", 12) = %q, want [\"**File \", \"Operations:*\", \"*\"]", got)
	}
}
