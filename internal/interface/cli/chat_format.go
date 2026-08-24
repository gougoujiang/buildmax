// Package cli provides the BuildMax CLI commands and interactive Bubble Tea UI.

package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/llm"

	"github.com/charmbracelet/glamour"
)

const maxArgsDisplayLen = 60

// ASCII banner for "BuildMAX" (block style, 6 lines; mixed case).
const bannerArt = `
 ______        _ _     _ ______         _    _ 
(____  \      (_) |   | |  ___ \   /\  \ \  / /
 ____)  )_   _ _| | _ | | | _ | | /  \  \ \/ / 
|  __  (| | | | | |/ || | || || |/ /\ \  )  (  
| |__)  ) |_| | | ( (_| | || || | |__| |/ /\ \ 
|______/ \____|_|_|\____|_||_||_|______/_/  \_\
`

// bannerWithVersion returns the ASCII banner plus a version line.
func bannerWithVersion(version string) string {
	art := strings.TrimPrefix(bannerArt, "\n")
	if version != "" {
		art += fmt.Sprintf("\n  v%s\n", version)
	} else {
		art += "\n"
	}
	return art
}

// toolDisplayName converts a snake_case tool name to a short display name (first word, capitalised).
func toolDisplayName(name string) string {
	first := strings.SplitN(name, "_", 2)[0]
	if first == "" {
		return name
	}
	return strings.ToUpper(first[:1]) + first[1:]
}

// buildToolResultMap returns a map from tool_call_id to success (true) or failure (false)
// by scanning tool-role messages in the message list.
func buildToolResultMap(msgs []llm.Message) map[string]bool {
	result := make(map[string]bool)
	for _, m := range msgs {
		if m.Role == "tool" && m.ToolCallID != "" {
			result[m.ToolCallID] = !strings.HasPrefix(m.Content, "error:")
		}
	}
	return result
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

// renderMarkdown renders markdown text for terminal display using glamour.
// style must be "dark" or "light" (pre-detected before the TUI starts);
// passing an empty string falls back to "dark" so glamour never queries the terminal.
func renderMarkdown(text, style string, width int) string {
	if strings.TrimSpace(text) == "" {
		return text
	}
	if width <= 0 {
		width = 80
	}
	if style == "" {
		style = "dark"
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(style),
		glamour.WithWordWrap(width-4),
	)
	if err != nil {
		return text
	}
	out, err := r.Render(text)
	if err != nil {
		return text
	}
	return strings.TrimRight(out, "\n")
}

// formatUserMsgForScrollback formats a user message for printing to the terminal scrollback.
func formatUserMsgForScrollback(text string) string {
	var b strings.Builder
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if i == 0 {
			b.WriteString(messageBarStyle.Render("> ") + userMessageStyle.Render(line))
		} else {
			b.WriteString("  " + userMessageStyle.Render(line))
		}
		if i < len(lines)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// formatQueuedMsgForScrollback formats a message the user typed while a run was in
// flight. The dim treatment and position marker say "typed, waiting" rather than
// "sent", which is the one thing the user needs to read off the transcript.
func formatQueuedMsgForScrollback(text string, position int) string {
	return queuedPrefixedLines(fmt.Sprintf("⏸ queued #%d ", position), text)
}

// formatBlockedMsgForScrollback formats a queued message a hook refused.
func formatBlockedMsgForScrollback(text, reason string) string {
	return queuedPrefixedLines("⨯ blocked ("+reason+") ", text)
}

// formatUnqueuedMsgForScrollback formats a queued message the user took back.
func formatUnqueuedMsgForScrollback(text string) string {
	return queuedPrefixedLines("⏵ unqueued ", text)
}

func queuedPrefixedLines(prefix, text string) string {
	var b strings.Builder
	lines := strings.Split(text, "\n")
	indent := strings.Repeat(" ", len([]rune(prefix)))
	for i, line := range lines {
		if i == 0 {
			b.WriteString(queuedMessageStyle.Render(prefix + line))
		} else {
			b.WriteString(queuedMessageStyle.Render(indent + line))
		}
		if i < len(lines)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// formatAssistantMsgForScrollback formats a completed assistant reply for printing to the terminal scrollback.
// style is the pre-detected glamour style ("dark" or "light").
func formatAssistantMsgForScrollback(text string, width int, style string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	rendered := renderMarkdown(text, style, width)
	lines := strings.Split(rendered, "\n")
	// Strip leading blank lines that glamour adds
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	for i, line := range lines {
		if i == 0 {
			b.WriteString(assistantGlyphStyle.Render("◆ ") + line)
		} else {
			b.WriteString("  " + line)
		}
		if i < len(lines)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// formatMsgForScrollback returns a formatted string for printing a historical message to the scrollback.
// toolResults maps tool_call_id to success (true) or failure (false); nil means treat all as pending.
// Returns empty string for messages that should not be printed (system, tool role, etc.).
func formatMsgForScrollback(m llm.Message, width int, style string, toolResults map[string]bool) string {
	switch m.Role {
	case "user":
		if m.Content == "" {
			return ""
		}
		return formatUserMsgForScrollback(m.Content)
	case "assistant":
		if m.Content == "" && len(m.ToolCalls) == 0 {
			return ""
		}
		var parts []string
		if m.Content != "" {
			parts = append(parts, formatAssistantMsgForScrollback(m.Content, width, style))
		}
		for _, tc := range m.ToolCalls {
			args := shortArgs(tc.Arguments)
			displayName := toolDisplayName(tc.Name)
			var glyph string
			if success, ok := toolResults[tc.ID]; ok {
				if success {
					glyph = toolGlyphSuccessStyle.Render("•")
				} else {
					glyph = toolGlyphFailStyle.Render("•")
				}
			} else {
				glyph = toolGlyphPendingStyle.Render("•")
			}
			line := "  " + glyph + " " + displayName
			if args != "" {
				line += " (" + args + ")"
			}
			parts = append(parts, line)
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

// buildMessagesForScrollback formats only the conversation messages (no banner) as a string
// for printing to the terminal scrollback. Used when resuming a session mid-TUI.
func buildMessagesForScrollback(messages []llm.Message, width int, style string) string {
	var b strings.Builder
	toolResults := buildToolResultMap(messages)
	for _, msg := range messages {
		line := formatMsgForScrollback(msg, width, style, toolResults)
		if line != "" {
			b.WriteString(line)
			b.WriteString("\n\n")
		}
	}
	return b.String()
}

// buildHistoryForScrollback formats the session banner and message history as a single string
// suitable for printing to stdout before the TUI program starts.
// style is the pre-detected glamour style ("dark" or "light").
func buildHistoryForScrollback(messages []llm.Message, width int, style string) string {
	var b strings.Builder
	b.WriteString(bannerWithVersion(config.Version))
	b.WriteString("\n")
	toolResults := buildToolResultMap(messages)
	for _, msg := range messages {
		line := formatMsgForScrollback(msg, width, style, toolResults)
		if line != "" {
			b.WriteString(line)
			b.WriteString("\n\n")
		}
	}
	return b.String()
}
