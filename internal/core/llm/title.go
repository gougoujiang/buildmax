package llm

import (
	"context"
	"strings"
)

// taskTitlePrompt asks the model for a short task title from a single request.
const taskTitlePrompt = `Generate a short task title (3-5 words) from this user request. Return ONLY the title, no quotes or punctuation.`

// titleGenerator implements TitleGenerator by prompting an LLMClient and
// stripping surrounding quotes from the reply. It holds the routed client, so
// the model router — not this type — decides which provider answers.
type titleGenerator struct {
	client LLMClient
}

// NewTitleGenerator returns a TitleGenerator backed by client.
func NewTitleGenerator(client LLMClient) TitleGenerator {
	return &titleGenerator{client: client}
}

func (g *titleGenerator) GenerateTitle(ctx context.Context, input string) (string, int, int, error) {
	if input == "" {
		return "", 0, 0, nil
	}
	msgs := []Message{
		{Role: "system", Content: taskTitlePrompt},
		{Role: "user", Content: input},
	}
	completion, err := g.client.ChatCompletionBlocking(ctx, Request{Messages: msgs, Profile: ProfileTitle})
	if err != nil {
		return "", 0, 0, err
	}
	return TrimTitle(completion.Content), completion.Usage.PromptTokens, completion.Usage.CompletionTokens, nil
}

// TrimTitle removes surrounding whitespace and one layer of matching quotes
// (", ', or `) from a model-generated title. It does not cap length; a caller
// that needs a bound applies its own.
func TrimTitle(s string) string {
	s = strings.TrimSpace(s)
	for _, q := range []string{`"`, `'`, "`"} {
		if len(s) >= 2 && strings.HasPrefix(s, q) && strings.HasSuffix(s, q) {
			s = s[len(q) : len(s)-len(q)]
		}
	}
	return strings.TrimSpace(s)
}
