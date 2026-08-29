package agentapp

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// compactSessionClient answers the compaction call with a summary and every other call —
// the note checkpoint — with nothing to store.
type compactSessionClient struct {
	mu       sync.Mutex
	profiles []llm.CallProfile
}

func (c *compactSessionClient) ChatCompletionBlocking(_ context.Context, req llm.Request) (llm.Completion, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.profiles = append(c.profiles, req.Profile)
	if req.Profile == llm.ProfileCompaction {
		return llm.Completion{
			Content: "SUMMARY OF EARLIER WORK",
			Usage:   llm.Usage{PromptTokens: 100, CompletionTokens: 10},
		}, nil
	}
	return llm.Completion{}, nil
}

func (c *compactSessionClient) ChatCompletionStreaming(ctx context.Context, req llm.Request, _ func(string)) (llm.Completion, error) {
	return c.ChatCompletionBlocking(ctx, req)
}

func (c *compactSessionClient) ContextWindow() int { return 40_000 }

var _ llm.LLMClient = (*compactSessionClient)(nil)

// TestAgentApp_CompactSessionReplacesHistory is the on-demand path end to end: the session's
// model-visible history shrinks to what the result reports, the summary is durable, and the
// summarizing call lands in the session's totals rather than nowhere.
func TestAgentApp_CompactSessionReplacesHistory(t *testing.T) {
	app := makeAgentAppForHookTests(t)
	client := &compactSessionClient{}
	app.llmClients.clients["stub"] = client

	sess, err := app.OpenSession("")
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	defer app.CloseSession(sess)
	for i := 0; i < 8; i++ {
		if err := sess.Append(llm.Message{Role: "user", Content: strings.Repeat("x", 2000)}); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	res, err := app.CompactSession(context.Background(), sess)
	if err != nil {
		t.Fatalf("CompactSession: %v", err)
	}
	if res.Summarized == 0 {
		t.Fatalf("nothing compacted (reason %q)", res.Reason)
	}
	if got := len(sess.HistoryMessages()); got != res.Kept {
		t.Errorf("history holds %d model-visible messages, want the %d reported kept", got, res.Kept)
	}
	if sess.PriorSummary() != "SUMMARY OF EARLIER WORK" {
		t.Errorf("stored summary = %q, want the summarizer's output", sess.PriorSummary())
	}
	if res.Status.ContextTokens >= res.BeforeTokens {
		t.Errorf("context went from %d to %d tokens, want it to shrink", res.BeforeTokens, res.Status.ContextTokens)
	}
	if sess.PromptTokens() != 100 || sess.CompletionTokens() != 10 {
		t.Errorf("session totals = %d in / %d out, want the compaction call folded in",
			sess.PromptTokens(), sess.CompletionTokens())
	}
}

// TestAgentApp_CompactSessionOnAShortSession pins that there is nothing to apologize for when
// a session is too small to compact: no error, no boundary, and a reason to report.
func TestAgentApp_CompactSessionOnAShortSession(t *testing.T) {
	app := makeAgentAppForHookTests(t)
	app.llmClients.clients["stub"] = &compactSessionClient{}

	sess, err := app.OpenSession("")
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	defer app.CloseSession(sess)
	if err := sess.Append(llm.Message{Role: "user", Content: "hello"}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	res, err := app.CompactSession(context.Background(), sess)
	if err != nil {
		t.Fatalf("CompactSession: %v", err)
	}
	if res.Summarized != 0 || res.Reason == "" {
		t.Errorf("result = %+v, want nothing compacted with a reason", res)
	}
	if sess.PriorSummary() != "" {
		t.Errorf("summary = %q, want none stored", sess.PriorSummary())
	}
}
