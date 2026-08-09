package agentapp

import (
	"context"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

const compactionSystemPrompt = `Summarize this AI agent conversation for context compaction.
The summary will replace the detailed message history to free up context space, so it must be self-contained.

Preserve in your summary:
- The user's original goal and key requirements
- Files read and why they were relevant
- Files created or modified and what was changed
- Commands run with important results
- Unresolved errors or open decisions
- Current plan or todo state

Be concise. Use bullet points. Omit pleasantries and filler.`

// LLMCompactor implements agent.ContextCompactor using the same LLM client as the agent run.
// It calls the model once with a summarize prompt over the messages to compact.
type LLMCompactor struct {
	client llm.LLMClient
}

// NewLLMCompactor creates a compactor backed by the given LLM client.
func NewLLMCompactor(client llm.LLMClient) *LLMCompactor {
	return &LLMCompactor{client: client}
}

// Compact summarizes msgs into a short text suitable for injection into the system prompt.
func (c *LLMCompactor) Compact(ctx context.Context, msgs []llm.Message) (string, error) {
	messages := make([]llm.Message, 0, len(msgs)+1)
	messages = append(messages, llm.Message{Role: "system", Content: compactionSystemPrompt})
	messages = append(messages, msgs...)
	summary, _, _, err := c.client.ChatCompletionBlocking(ctx, messages, nil)
	return summary, err
}
