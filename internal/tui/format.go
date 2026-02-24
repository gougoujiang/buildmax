// Package tui provides the Bubble Tea TUI models and views.

package tui

import (
	"encoding/json"
	"strings"

	"buildmax/internal/llm"
	"buildmax/internal/session"

	"github.com/charmbracelet/lipgloss"
)

const maxArgsDisplayLen = 60

// messageBarStyle is the light sky blue vertical bar at the start of user/assistant lines.
var messageBarStyle = lipgloss.NewStyle().Foreground(lightSkyBlue)

// userMessageStyle gives user message text a lighter, tinted color so it's easy to distinguish from assistant messages.
var userMessageStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#B8D4E3"))

// formatMessage returns display lines for a single chat message:
// user → content only (leading ">" added in buildViewportContent); assistant → content plus " * name (args)" per tool call (leading bullet in buildViewportContent);
// tool → optional " * result: ..." or omit.
func formatMessage(m llm.Message) []string {
	var lines []string
	switch m.Role {
	case "user":
		lines = append(lines, m.Content)
	case "assistant":
		lines = append(lines, m.Content)
		for _, tc := range m.ToolCalls {
			args := shortArgs(tc.Arguments)
			lines = append(lines, " * "+tc.Name+" ("+args+")")
		}
	case "tool":
		// optional: show tool result briefly
		s := m.Content
		if len(s) > 40 {
			s = s[:37] + "..."
		}
		lines = append(lines, " * result: "+s)
	default:
		// system or unknown: skip for display
	}
	return lines
}

// shortArgs returns a short summary of JSON arguments for display (e.g. first arg or truncated).
func shortArgs(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if len(raw) <= maxArgsDisplayLen {
		return raw
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return raw[:maxArgsDisplayLen-3] + "..."
	}
	// Prefer "path" or "file" or first key for read_file-style args
	for _, k := range []string{"path", "file", "filename"} {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok {
				if len(s) > maxArgsDisplayLen {
					return s[:maxArgsDisplayLen-3] + "..."
				}
				return s
			}
		}
	}
	for _, v := range m {
		if s, ok := v.(string); ok {
			if len(s) > maxArgsDisplayLen {
				return s[:maxArgsDisplayLen-3] + "..."
			}
			return s
		}
	}
	return raw[:maxArgsDisplayLen-3] + "..."
}

// wrapLine breaks line into chunks of at most width runes, preferring to break at spaces so words are not split.
func wrapLine(line string, width int) []string {
	if width <= 0 {
		return []string{line}
	}
	var out []string
	runes := []rune(line)
	start := 0
	for start < len(runes) {
		end := start + width
		if end > len(runes) {
			end = len(runes)
		}
		if end == len(runes) {
			out = append(out, string(runes[start:end]))
			start = end
			continue
		}
		// Prefer break at last space in this chunk so we don't split a word.
		lastSpace := -1
		for i := start; i < end; i++ {
			if runes[i] == ' ' || runes[i] == '\t' {
				lastSpace = i
			}
		}
		if lastSpace >= start {
			out = append(out, string(runes[start:lastSpace+1]))
			start = lastSpace + 1
		} else {
			out = append(out, string(runes[start:end]))
			start = end
		}
	}
	if len(out) == 0 {
		out = append(out, "")
	}
	return out
}

// ViewportContentOpts holds display options for building viewport content (version, width, busy state, carousel, streaming tail).
type ViewportContentOpts struct {
	Version      string
	Width        int
	Busy         bool
	CarouselDots int
	StreamingTail string
}

// buildViewportContent returns the full scrollable content: banner plus message lines (with bar for user/assistant, wrapped to width).
// If opts.StreamingTail is non-empty, appends it as the current assistant line (same style as "• " + content).
// If opts.Busy is true and opts.StreamingTail is empty, appends a carousel line "• ." / ".." / "..." based on opts.CarouselDots (0, 1, 2).
func buildViewportContent(sess *session.Session, opts ViewportContentOpts) string {
	width := opts.Width
	if width <= 0 {
		width = 80
	}
	var b strings.Builder
	// Top margin so the banner is not clipped by the terminal title/tab bar.
	b.WriteString("\n\n")
	b.WriteString(bannerWithVersion(opts.Version))
	b.WriteString("\n")
	messages := sess.Messages()
	if len(messages) > 0 {
		b.WriteString("\n") // margin between banner and first message
	}
	for i, m := range messages {
		if i > 0 {
			b.WriteString("\n") // margin between messages
		}
		for _, line := range formatMessage(m) {
			var prefix string
			switch m.Role {
			case "user":
				prefix = messageBarStyle.Render("> ") + userMessageStyle.Render(line)
			case "assistant":
				prefix = messageBarStyle.Render("• ") + line
			default:
				prefix = "  " + line
			}
			for _, w := range wrapLine(prefix, width) {
				b.WriteString(w)
				b.WriteString("\n")
			}
		}
	}
	if opts.StreamingTail != "" {
		if len(messages) > 0 {
			b.WriteString("\n")
		}
		line := messageBarStyle.Render("• ") + opts.StreamingTail
		for _, w := range wrapLine(line, width) {
			b.WriteString(w)
			b.WriteString("\n")
		}
	} else if opts.Busy {
		if len(messages) > 0 {
			b.WriteString("\n") // margin before carousel, same as between messages
		}
		dots := []string{".", "..", "..."}
		idx := opts.CarouselDots % 3
		line := messageBarStyle.Render("• ") + dots[idx]
		for _, w := range wrapLine(line, width) {
			b.WriteString(w)
			b.WriteString("\n")
		}
	}
	return b.String()
}
