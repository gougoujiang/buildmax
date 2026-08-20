package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// compactingHistory mirrors session.Session: HistoryMessages returns the suffix after the
// compaction boundary, and AddCompaction replaces the stored summary and advances the boundary.
type compactingHistory struct {
	messages []llm.Message
	idx      int
	summary  string
}

func (h *compactingHistory) HistoryMessages() []llm.Message {
	if h.idx > 0 && h.idx <= len(h.messages) {
		return h.messages[h.idx:]
	}
	return h.messages
}

func (h *compactingHistory) Append(m llm.Message) error {
	h.messages = append(h.messages, m)
	return nil
}

func (h *compactingHistory) PriorSummary() string { return h.summary }

func (h *compactingHistory) AddCompaction(summary string, summarizedCount int) {
	h.summary = summary
	h.idx += summarizedCount
	if h.idx > len(h.messages) {
		h.idx = len(h.messages)
	}
}

var _ CompactionHistory = (*compactingHistory)(nil)

// windowedClient reports a fixed context window, records the system prompt of every call,
// and always returns final content so one RunLoop call performs exactly one iteration.
type windowedClient struct {
	window   int
	systems  []string
	lastSent []llm.Message
}

func (c *windowedClient) ChatCompletionBlocking(ctx context.Context, messages []llm.Message, tools []llm.ToolDef) (llm.Completion, error) {
	if len(messages) > 0 && messages[0].Role == "system" {
		c.systems = append(c.systems, messages[0].Content)
	}
	c.lastSent = append([]llm.Message(nil), messages...)
	return llm.Completion{Content: "done"}, nil
}

func (c *windowedClient) ChatCompletionStreaming(ctx context.Context, messages []llm.Message, tools []llm.ToolDef, onDelta func(string)) (llm.Completion, error) {
	return c.ChatCompletionBlocking(ctx, messages, tools)
}

func (c *windowedClient) ContextWindow() int { return c.window }

var _ llm.LLMClient = (*windowedClient)(nil)

// factCompactor models a summarizer that faithfully carries forward every "FACT:" line it is
// shown — including lines inside a previous summary fed back to it. A fact therefore
// disappears only if it was never presented, which is precisely the accumulation defect.
type factCompactor struct {
	calls [][]llm.Message
}

func (c *factCompactor) Compact(ctx context.Context, msgs []llm.Message) (string, error) {
	c.calls = append(c.calls, msgs)
	var facts []string
	for _, m := range msgs {
		for _, line := range strings.Split(m.Content, "\n") {
			if strings.HasPrefix(line, "FACT:") {
				facts = append(facts, line)
			}
		}
	}
	return strings.Join(facts, "\n"), nil
}

// bigCompactor returns a summary far larger than any budget, to exercise clamping.
type bigCompactor struct{}

func (c *bigCompactor) Compact(ctx context.Context, msgs []llm.Message) (string, error) {
	return strings.Repeat("filler line of summary text\n", 4000), nil
}

const testContextWindow = 4000

// fillerMessage is large enough that a handful of them cross the compaction threshold.
func fillerMessage() llm.Message {
	return llm.Message{Role: "user", Content: strings.Repeat("x", 2000)}
}

func runOnce(t *testing.T, client llm.LLMClient, h MessageHistory, comp ContextCompactor) {
	t.Helper()
	_, _, err := RunLoop(context.Background(), RunLoopOpts{
		LLMClient:    client,
		SystemPrompt: testSystemPrompt,
		ToolRegistry: newTestToolRegistry(),
		MaxIter:      DefaultMaxIterations,
		History:      h,
		Compactor:    comp,
	})
	if err != nil {
		t.Fatalf("RunLoop: %v", err)
	}
}

// fillToThreshold appends messages until the history is over the compaction threshold.
func fillToThreshold(h *compactingHistory) {
	for EstimateTokens(h.HistoryMessages()) <= int(float64(testContextWindow)*compactionThreshold) {
		_ = h.Append(fillerMessage())
	}
}

// TestCompaction_SummaryAccumulatesAcrossCompactions is the regression test for the defect the
// design record calls out: before the fix, compaction N summarized only the newly discarded
// messages, so the summary it replaced — and everything that summary covered — was lost.
func TestCompaction_SummaryAccumulatesAcrossCompactions(t *testing.T) {
	const fact = "FACT: the contract is governed by New York law"
	client := &windowedClient{window: testContextWindow}
	comp := &factCompactor{}
	h := &compactingHistory{}

	// The fact is stated once, in the oldest message, and never repeated.
	_ = h.Append(llm.Message{Role: "user", Content: fact + "\n" + strings.Repeat("x", 2000)})
	fillToThreshold(h)

	for pass := 1; pass <= 3; pass++ {
		runOnce(t, client, h, comp)
		if !strings.Contains(h.summary, fact) {
			t.Fatalf("pass %d: fact lost from stored summary; got %q", pass, h.summary)
		}
		fillToThreshold(h)
	}

	if len(comp.calls) < 3 {
		t.Fatalf("expected at least 3 compactions, got %d", len(comp.calls))
	}
	// From the second compaction on, the prior summary must be the first thing handed to the
	// summarizer — that is what carries the older material forward.
	for i, call := range comp.calls[1:] {
		if len(call) == 0 || !strings.HasPrefix(call[0].Content, priorSummaryPreamble) {
			t.Errorf("compaction %d: first message is not the prior summary", i+2)
		}
	}
}

// TestCompaction_AdvancesBoundaryByRealMessagesOnly asserts the synthetic prior-summary message
// is not counted against the history: counting it would silently drop one real message per pass.
func TestCompaction_AdvancesBoundaryByRealMessagesOnly(t *testing.T) {
	client := &windowedClient{window: testContextWindow}
	comp := &factCompactor{}
	h := &compactingHistory{}
	fillToThreshold(h)

	runOnce(t, client, h, comp)

	summarized := len(comp.calls[0]) // no prior summary on the first pass, so this is all real
	if h.idx != summarized {
		t.Fatalf("boundary = %d, want %d (messages actually summarized)", h.idx, summarized)
	}
	// The messages behind the boundary must be exactly the ones handed to the summarizer.
	// An off-by-one here means a real message was dropped without ever being summarized.
	for i := 0; i < h.idx; i++ {
		if h.messages[i].Content != comp.calls[0][i].Content {
			t.Fatalf("message %d is behind the boundary but was not the one summarized", i)
		}
	}
}

// TestCompaction_SystemPromptCarriesOneBlock guards the duplication the design record notes:
// the caller used to append the persisted summary and RunLoop appended the new one on top.
func TestCompaction_SystemPromptCarriesOneBlock(t *testing.T) {
	client := &windowedClient{window: testContextWindow}
	comp := &factCompactor{}
	h := &compactingHistory{}
	fillToThreshold(h)

	for pass := 0; pass < 3; pass++ {
		runOnce(t, client, h, comp)
		fillToThreshold(h)
	}

	if len(client.systems) == 0 {
		t.Fatal("no system prompts recorded")
	}
	for i, sys := range client.systems {
		open := strings.Count(sys, "<context_compaction>")
		if open > 1 {
			t.Errorf("call %d: system prompt has %d compaction blocks, want at most 1", i, open)
		}
		if open != strings.Count(sys, "</context_compaction>") {
			t.Errorf("call %d: unbalanced compaction block", i)
		}
	}
}

// TestCompaction_SummaryStaysWithinBudget asserts the one slot that can never be trimmed does
// not grow without bound, even when the summarizer ignores its instructions entirely.
func TestCompaction_SummaryStaysWithinBudget(t *testing.T) {
	client := &windowedClient{window: testContextWindow}
	comp := &bigCompactor{}
	h := &compactingHistory{}
	limit := maxSummaryChars(testContextWindow)

	for pass := 0; pass < 10; pass++ {
		fillToThreshold(h)
		runOnce(t, client, h, comp)
		if len(h.summary) > limit+len(summaryClampMarker) {
			t.Fatalf("pass %d: summary is %d chars, budget is %d", pass, len(h.summary), limit)
		}
	}
	if h.summary == "" {
		t.Fatal("no summary was stored")
	}
}

// TestRunLoop_SeedsSummaryFromHistory covers the cross-turn path: a session compacted in an
// earlier turn must keep its summary without the caller folding it into SystemPrompt.
func TestRunLoop_SeedsSummaryFromHistory(t *testing.T) {
	client := &windowedClient{window: 0} // no window: compaction cannot fire, so this is pure seeding
	h := &compactingHistory{summary: "PRIOR SUMMARY"}
	_ = h.Append(llm.Message{Role: "user", Content: "hello"})

	runOnce(t, client, h, &factCompactor{})

	if len(client.systems) != 1 {
		t.Fatalf("recorded %d system prompts, want 1", len(client.systems))
	}
	if !strings.Contains(client.systems[0], "<context_compaction>\nPRIOR SUMMARY\n</context_compaction>") {
		t.Errorf("system prompt missing seeded summary:\n%s", client.systems[0])
	}
}

func TestWithPriorSummary(t *testing.T) {
	msgs := []llm.Message{{Role: "user", Content: "a"}}

	if got := withPriorSummary("", msgs); len(got) != 1 {
		t.Errorf("no prior summary: len = %d, want 1", len(got))
	}

	got := withPriorSummary("old stuff", msgs)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Role != "user" {
		t.Errorf("synthetic message role = %q, want \"user\"", got[0].Role)
	}
	if !strings.Contains(got[0].Content, "old stuff") {
		t.Errorf("synthetic message does not carry the summary: %q", got[0].Content)
	}
	if got[1].Content != "a" {
		t.Errorf("original message not preserved: %q", got[1].Content)
	}
	if len(msgs) != 1 || msgs[0].Content != "a" {
		t.Error("input slice was mutated")
	}
}

func TestClampSummary(t *testing.T) {
	if got := clampSummary("short", 100); got != "short" {
		t.Errorf("under budget: got %q, want unchanged", got)
	}
	if got := clampSummary("anything", 0); got != "anything" {
		t.Errorf("zero budget disables clamping: got %q", got)
	}

	long := strings.Repeat("line of text\n", 500)
	got := clampSummary(long, 100)
	if len(got) > 100+len(summaryClampMarker) {
		t.Errorf("clamped length = %d, want <= %d", len(got), 100+len(summaryClampMarker))
	}
	if !strings.HasSuffix(got, summaryClampMarker) {
		t.Errorf("clamped summary is not marked: %q", got)
	}
	// The back-off should land on a line boundary, so the kept text ends with a whole line.
	kept := strings.TrimSuffix(got, summaryClampMarker)
	if !strings.HasSuffix(strings.TrimRight(kept, "\n"), "line of text") {
		t.Errorf("clamp cut mid-line: %q", kept)
	}
}

func TestMaxSummaryChars(t *testing.T) {
	if got := maxSummaryChars(0); got != defaultMaxSummaryChars {
		t.Errorf("unknown window: got %d, want %d", got, defaultMaxSummaryChars)
	}
	if got := maxSummaryChars(200_000); got != 16_000 {
		t.Errorf("200k window: got %d, want 16000", got)
	}
}

func TestRenderCompactionBlock(t *testing.T) {
	if got := RenderCompactionBlock(""); got != "" {
		t.Errorf("empty summary renders %q, want \"\"", got)
	}
	got := RenderCompactionBlock("s")
	if strings.Count(got, "<context_compaction>") != 1 || !strings.Contains(got, "\ns\n") {
		t.Errorf("unexpected block: %q", got)
	}
}
