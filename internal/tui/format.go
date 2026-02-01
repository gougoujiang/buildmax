// Package tui provides the Bubble Tea TUI models and views.

package tui

import (
	"encoding/json"
	"strings"

	"buildmax/internal/llm"
	"buildmax/internal/session"
)

const maxArgsDisplayLen = 60

// formatMessage returns display lines for a single chat message:
// user → "you: content"; assistant → "assistant: content" plus " * name (args)" per tool call;
// tool → optional " * result: ..." or omit.
func formatMessage(m llm.Message) []string {
	var lines []string
	switch m.Role {
	case "user":
		lines = append(lines, "you: "+m.Content)
	case "assistant":
		lines = append(lines, "assistant: "+m.Content)
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

// buildViewportContent returns the full scrollable content: ASCII banner plus all message lines.
func buildViewportContent(sess *session.Session, version string) string {
	var b strings.Builder
	b.WriteString(bannerWithVersion(version))
	b.WriteString("\n")
	for _, m := range sess.Messages() {
		for _, line := range formatMessage(m) {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return b.String()
}
