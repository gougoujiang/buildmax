package agentapp

import (
	"context"

	"github.com/gougoujiang/buildmax/internal/core/agent"
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

Be concise. Use bullet points. Omit pleasantries and filler.

When the session's live notes and task list are given below, treat them as a relevance signal:
spend your detail on material that bears on what is still open, and compress everything already
settled to a single line each. Do not copy the notes into the summary — they are kept separately
and will still be present.`

// liveStatePreamble labels the live state when it is handed to the summarizer. Without it the
// model is asked to preserve "what matters" over an unbounded transcript with no way to tell
// what is still open, and spends its budget evenly on material of very unequal value.
const liveStatePreamble = "The session's live notes and task list, for judging what is still relevant:"

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
	messages := make([]llm.Message, 0, len(msgs)+2)
	messages = append(messages, llm.Message{Role: "system", Content: compactionSystemPrompt})
	// The run's durable state travels on the context, so the summarizer can be told what is
	// still live without widening the ContextCompactor interface.
	if store, ok := agent.NoteStoreFromContext(ctx); ok {
		if live := agent.RenderSessionState("", store.Notes(), store.Todos(), 0); live != "" {
			messages = append(messages, llm.Message{Role: "user", Content: liveStatePreamble + "\n\n" + live})
		}
	}
	messages = append(messages, msgs...)
	completion, err := c.client.ChatCompletionBlocking(ctx, messages, nil)
	return completion.Content, err
}
