// Session title generation via LLM.
package session

import (
	"context"
	"log/slog"
	"strings"

	"buildmax/internal/llm"
)

// ChatFunc makes a simple chat completion call (no tools).
// Used for lightweight tasks like title generation.
type ChatFunc func(ctx context.Context, messages []llm.Message) (string, error)

// titleSystemPrompt instructs the LLM to produce a concise session title.
const titleSystemPrompt = `Generate a short, descriptive title (3-8 words) for this conversation. Return ONLY the title text, nothing else. Do not use quotes or punctuation at the start or end.`

// GenerateTitle uses an LLM to produce a concise title from the conversation messages.
// It extracts the first user message and first assistant reply (if any) to keep the
// prompt small and cheap. Returns the cleaned title string.
func GenerateTitle(ctx context.Context, chatFn ChatFunc, messages []llm.Message) (string, error) {
	// Build a small context: first user message + first assistant reply.
	var titleMsgs []llm.Message
	titleMsgs = append(titleMsgs, llm.Message{Role: "system", Content: titleSystemPrompt})

	var gotUser, gotAssistant bool
	for _, m := range messages {
		if !gotUser && m.Role == "user" {
			titleMsgs = append(titleMsgs, llm.Message{Role: "user", Content: m.Content})
			gotUser = true
			continue
		}
		if gotUser && !gotAssistant && m.Role == "assistant" && m.Content != "" {
			// Truncate long assistant replies to keep the prompt small.
			content := m.Content
			if len([]rune(content)) > 500 {
				content = string([]rune(content)[:500])
			}
			titleMsgs = append(titleMsgs, llm.Message{Role: "assistant", Content: content})
			gotAssistant = true
			break
		}
	}
	if !gotUser {
		return "", nil
	}

	slog.Debug("generating session title via LLM")
	title, err := chatFn(ctx, titleMsgs)
	if err != nil {
		return "", err
	}

	title = cleanTitle(title)
	slog.Debug("generated session title", "title", title)
	return title, nil
}

// cleanTitle strips surrounding whitespace, quotes, and trailing punctuation
// from an LLM-generated title.
func cleanTitle(s string) string {
	s = strings.TrimSpace(s)
	// Strip surrounding quotes (single or double).
	for _, q := range []string{`"`, `'`, "`"} {
		if len(s) >= 2 && strings.HasPrefix(s, q) && strings.HasSuffix(s, q) {
			s = s[len(q) : len(s)-len(q)]
		}
	}
	s = strings.TrimSpace(s)
	// Cap at 100 runes.
	runes := []rune(s)
	if len(runes) > 100 {
		s = string(runes[:100])
	}
	return s
}
