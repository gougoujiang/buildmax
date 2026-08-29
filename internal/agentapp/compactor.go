package agentapp

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/session"
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
func (c *LLMCompactor) Compact(ctx context.Context, msgs []llm.Message) (string, llm.Usage, error) {
	messages := make([]llm.Message, 0, len(msgs)+2)
	messages = append(messages, llm.Message{Role: "system", Content: compactionSystemPrompt})
	// The run's durable state travels on the context, so the summarizer can be told what is
	// still live without widening the ContextCompactor interface.
	if store, ok := agent.NoteStoreFromContext(ctx); ok {
		if live := agent.RenderSessionState("", store.Notes(), store.Todos()); live != "" {
			messages = append(messages, llm.Message{Role: "user", Content: liveStatePreamble + "\n\n" + live})
		}
	}
	messages = append(messages, msgs...)
	completion, err := c.client.ChatCompletionBlocking(ctx, llm.Request{Messages: messages, Profile: llm.ProfileCompaction})
	return completion.Content, completion.Usage, err
}

// CompactResult is what one compaction the user asked for did to a session.
//
// Summarized == 0 with no error means the pass found nothing worth replacing,
// and Reason says why; the caller reports that rather than a failure.
type CompactResult struct {
	Summarized int
	Kept       int
	Reason     string
	// BeforeTokens is the estimated context size the session carried into the
	// compaction, so a surface can say what the pass actually freed. The size
	// afterwards is Status.ContextTokens.
	BeforeTokens int
	// Status is the session's usage re-estimated after the boundary moved. A
	// surface holding a context gauge would otherwise keep showing the size the
	// compaction just removed.
	Status RunUsage
}

// CompactSession compacts a session's context on demand.
//
// It is the same pass RunLoop makes on its own when the window fills, run
// without the fill test, and it takes the session's turn lock for the same
// reason a turn does: it rewrites the model-visible history, and doing that
// under a running turn would race it.
func (a *AgentApp) CompactSession(ctx context.Context, sess *SessionContext) (CompactResult, error) {
	sess, modelName, client, err := a.resolveRunContext(sess)
	if err != nil {
		return CompactResult{}, err
	}
	if err := a.turns.begin(sess.ID()); err != nil {
		return CompactResult{}, fmt.Errorf("session %s: %w", sess.ID(), err)
	}
	defer a.turns.end(sess.ID())

	before := a.estimateRunUsage(sess, modelName, client.ContextWindow())
	// The checkpointer writes notes and todos through the context, the same way
	// it reaches the session during a run.
	ctx = session.CtxWithSessionID(ctx, sess.ID())
	ctx = agent.CtxWithNoteStore(ctx, sess)

	res, stats, err := agent.Compact(ctx, agent.RunLoopOpts{
		LLMClient:    client,
		Pricing:      a.pricingFor(sess),
		History:      sess,
		Compactor:    NewLLMCompactor(client),
		Checkpointer: NewNoteCheckpointer(client),
		Hooks:        a.hooks,
		SessionID:    sess.ID(),
		Workspace:    a.workspace.Root(),
	})
	// Folded in before the error is returned: the summarizing call is spent
	// whether or not its summary was usable, and a session whose totals omit it
	// under-reports for good.
	if _, ferr := a.finalizeTurn(sess, nil, stats); ferr != nil {
		slog.Warn("could not record what the compaction spent", "err", ferr)
	}
	if err != nil {
		return CompactResult{}, fmt.Errorf("compact session: %w", err)
	}
	return CompactResult{
		Summarized:   res.Summarized,
		Kept:         res.Kept,
		Reason:       res.Reason,
		BeforeTokens: before.ContextTokens,
		Status:       a.estimateRunUsage(sess, modelName, client.ContextWindow()),
	}, nil
}
