// Session title generation via LLM.
package session

import (
	"context"
	"log/slog"
	"strings"

	"buildmax/internal/core/model"
)

// TitleChatClient is the interface for a simple chat completion (no tools), used for title generation.
// Callers can pass an infra LLM adapter via TitleChatFunc or a test double. Usage is returned for metering (e.g. billing).
type TitleChatClient interface {
	Chat(ctx context.Context, messages []model.Message) (string, model.Usage, error)
}

// TitleChatFunc adapts a function to TitleChatClient so closures and *llm.Client wrappers can be used.
type TitleChatFunc func(ctx context.Context, messages []model.Message) (string, model.Usage, error)

// Chat implements TitleChatClient.
func (f TitleChatFunc) Chat(ctx context.Context, messages []model.Message) (string, model.Usage, error) {
	return f(ctx, messages)
}

// titleSystemPrompt instructs the LLM to produce a concise session title.
const titleSystemPrompt = `Generate a short, descriptive title (3-8 words) for this conversation. Return ONLY the title text, nothing else. Do not use quotes or punctuation at the start or end.`

// taskTitleSystemPrompt instructs the LLM to produce a short task title from a single user input.
const taskTitleSystemPrompt = `Generate a short task title (3-5 words) from this user request. Return ONLY the title, no quotes or punctuation.`

// GenerateTitleFromInput uses an LLM to produce a concise title from a single input string (e.g. task prompt).
// Returns the cleaned title string and usage for metering. Reuses cleanTitle for trimming and quote stripping.
func GenerateTitleFromInput(ctx context.Context, client TitleChatClient, input string) (string, model.Usage, error) {
	if input == "" {
		return "", model.Usage{}, nil
	}
	messages := []model.Message{
		{Role: "system", Content: taskTitleSystemPrompt},
		{Role: "user", Content: input},
	}
	slog.Debug("generating task title via LLM")
	title, usage, err := client.Chat(ctx, messages)
	if err != nil {
		return "", model.Usage{}, err
	}
	title = cleanTitle(title)
	slog.Debug("generated task title", "title", title)
	return title, usage, nil
}

// GenerateTitle uses an LLM to produce a concise title from the conversation messages.
// It extracts the first user message and first assistant reply (if any) to keep the
// prompt small and cheap. Returns the cleaned title string and usage for metering.
func GenerateTitle(ctx context.Context, client TitleChatClient, messages []model.Message) (string, model.Usage, error) {
	// Build a small context: first user message + first assistant reply.
	var titleMsgs []model.Message
	titleMsgs = append(titleMsgs, model.Message{Role: "system", Content: titleSystemPrompt})

	var gotUser, gotAssistant bool
	for _, m := range messages {
		if !gotUser && m.Role == "user" {
			titleMsgs = append(titleMsgs, model.Message{Role: "user", Content: m.Content})
			gotUser = true
			continue
		}
		if gotUser && !gotAssistant && m.Role == "assistant" && m.Content != "" {
			// Truncate long assistant replies to keep the prompt small.
			content := m.Content
			if len([]rune(content)) > 500 {
				content = string([]rune(content)[:500])
			}
			titleMsgs = append(titleMsgs, model.Message{Role: "assistant", Content: content})
			gotAssistant = true
			break
		}
	}
	if !gotUser {
		return "", model.Usage{}, nil
	}

	slog.Debug("generating session title via LLM")
	title, usage, err := client.Chat(ctx, titleMsgs)
	if err != nil {
		return "", model.Usage{}, err
	}

	title = cleanTitle(title)
	slog.Debug("generated session title", "title", title)
	return title, usage, nil
}

// EnsureTitleFromFirstUserMessage sets the session title from the first user message
// if the title is empty and at least one user message exists, truncated to maxLen runes.
// No-op otherwise.
func EnsureTitleFromFirstUserMessage(s *Session, maxLen int) {
	if s.Title() != "" {
		return
	}
	msgs := s.Messages()
	for _, m := range msgs {
		if m.Role == "user" {
			content := m.Content
			runes := []rune(content)
			if len(runes) > maxLen {
				content = string(runes[:maxLen])
			}
			s.SetTitle(content)
			return
		}
	}
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
