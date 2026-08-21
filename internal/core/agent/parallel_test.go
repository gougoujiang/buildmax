package agent

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// span records when a call entered and left Execute, which is how the tests
// below tell overlap from sequence.
type span struct {
	name       string
	start, end time.Time
}

// timedTool sleeps, records its span, and reports the access it was built with.
type timedTool struct {
	name   string
	access llm.Access
	sleep  time.Duration
	panics bool

	mu    sync.Mutex
	spans *[]span
}

func (t *timedTool) Name() string                       { return t.name }
func (t *timedTool) Description() string                { return "timed" }
func (t *timedTool) Parameters() any                    { return map[string]any{} }
func (t *timedTool) Access(_ map[string]any) llm.Access { return t.access }

func (t *timedTool) Execute(_ context.Context, args map[string]any) (string, error) {
	if t.panics {
		panic("tool exploded")
	}
	start := time.Now()
	time.Sleep(t.sleep)
	t.mu.Lock()
	id, _ := args["id"].(string)
	*t.spans = append(*t.spans, span{name: t.name + ":" + id, start: start, end: time.Now()})
	t.mu.Unlock()
	return "ok", nil
}

func callsFor(name string, ids ...string) []llm.ToolCall {
	out := make([]llm.ToolCall, 0, len(ids))
	for _, id := range ids {
		out = append(out, llm.ToolCall{ID: id, Name: name, Arguments: fmt.Sprintf(`{"id":%q}`, id)})
	}
	return out
}

func runBatch(t *testing.T, limit int, tools []llm.Tool, calls []llm.ToolCall) (*testBuffer, time.Duration) {
	t.Helper()
	registry := llm.NewToolRegistry()
	registry.AppendTools(tools...)
	sess := newTestBuffer()
	mock := &mockLLMClient{responses: []mockResponse{{toolCalls: calls}, {content: "done"}}}

	start := time.Now()
	_, _, err := runLoopWithUserMsg(context.Background(), mock, registry, sess, "go", func(o *RunLoopOpts) {
		o.MaxParallelTools = limit
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("RunLoop: %v", err)
	}
	return sess, elapsed
}

// TestParallel_ReadsOverlap is the point of the whole design: independent
// read-only calls finish in about the time of one, not the sum.
func TestParallel_ReadsOverlap(t *testing.T) {
	const sleep = 100 * time.Millisecond
	var spans []span
	tool := &timedTool{name: "read", access: llm.AccessReadOnly, sleep: sleep, spans: &spans}

	_, elapsed := runBatch(t, 4, []llm.Tool{tool}, callsFor("read", "a", "b", "c"))

	if elapsed > sleep*2 {
		t.Errorf("three %v calls took %v; want them overlapped, not summed", sleep, elapsed)
	}
	if len(spans) != 3 {
		t.Fatalf("recorded %d spans, want 3", len(spans))
	}
	if !overlaps(spans[0], spans[1]) || !overlaps(spans[1], spans[2]) {
		t.Errorf("spans did not overlap: %v", spans)
	}
}

// TestParallel_SequentialWhenLimitIsOne is D6: the escape hatch has to work.
func TestParallel_SequentialWhenLimitIsOne(t *testing.T) {
	const sleep = 60 * time.Millisecond
	var spans []span
	tool := &timedTool{name: "read", access: llm.AccessReadOnly, sleep: sleep, spans: &spans}

	_, elapsed := runBatch(t, 1, []llm.Tool{tool}, callsFor("read", "a", "b", "c"))

	if elapsed < sleep*3 {
		t.Errorf("limit 1 finished in %v; want the calls run one after another", elapsed)
	}
}

// TestParallel_WriteIsABarrier: a write must never overlap a neighbour, or
// [Write a, Read a] would stop meaning what the model wrote.
func TestParallel_WriteIsABarrier(t *testing.T) {
	const sleep = 50 * time.Millisecond
	var spans []span
	read := &timedTool{name: "read", access: llm.AccessReadOnly, sleep: sleep, spans: &spans}
	write := &timedTool{name: "write", access: llm.AccessWrite, sleep: sleep, spans: &spans}

	calls := []llm.ToolCall{
		{ID: "1", Name: "read", Arguments: `{"id":"1"}`},
		{ID: "2", Name: "read", Arguments: `{"id":"2"}`},
		{ID: "3", Name: "write", Arguments: `{"id":"3"}`},
		{ID: "4", Name: "read", Arguments: `{"id":"4"}`},
	}
	runBatch(t, 8, []llm.Tool{read, write}, calls)

	byName := map[string]span{}
	for _, s := range spans {
		byName[s.name] = s
	}
	w := byName["write:3"]
	for _, other := range []string{"read:1", "read:2", "read:4"} {
		if overlaps(w, byName[other]) {
			t.Errorf("write overlapped %s: %v vs %v", other, w, byName[other])
		}
	}
	if !overlaps(byName["read:1"], byName["read:2"]) {
		t.Error("the two reads before the write should still have overlapped")
	}
}

// TestParallel_PanicIsContained keeps one bad tool from taking the run down or
// stranding the siblings running beside it.
func TestParallel_PanicIsContained(t *testing.T) {
	var spans []span
	ok := &timedTool{name: "read", access: llm.AccessReadOnly, sleep: 10 * time.Millisecond, spans: &spans}
	boom := &timedTool{name: "boom", access: llm.AccessReadOnly, panics: true, spans: &spans}

	calls := []llm.ToolCall{
		{ID: "1", Name: "read", Arguments: `{"id":"1"}`},
		{ID: "2", Name: "boom", Arguments: `{"id":"2"}`},
		{ID: "3", Name: "read", Arguments: `{"id":"3"}`},
	}
	sess, _ := runBatch(t, 8, []llm.Tool{ok, boom}, calls)

	results := toolResults(sess)
	if len(results) != 3 {
		t.Fatalf("got %d tool results, want one per call even with a panic", len(results))
	}
	if got := results["2"]; got == "" || !contains(got, "panicked") {
		t.Errorf("result for the panicking call = %q, want it to report the panic", got)
	}
	if results["1"] != "ok" || results["3"] != "ok" {
		t.Errorf("siblings were stranded: %v", results)
	}
}

// TestParallel_EveryCallGetsExactlyOneResult is D4. A batch that half-executes
// still has to leave a well-formed history, or the next LLM call is malformed.
func TestParallel_EveryCallGetsExactlyOneResult(t *testing.T) {
	var spans []span
	read := &timedTool{name: "read", access: llm.AccessReadOnly, sleep: time.Millisecond, spans: &spans}

	calls := append(callsFor("read", "a", "b"),
		llm.ToolCall{ID: "c", Name: "nonexistent", Arguments: `{}`},
		llm.ToolCall{ID: "d", Name: "read", Arguments: `{bad json`},
	)
	sess, _ := runBatch(t, 8, []llm.Tool{read}, calls)

	results := toolResults(sess)
	for _, id := range []string{"a", "b", "c", "d"} {
		if _, ok := results[id]; !ok {
			t.Errorf("call %q got no tool result", id)
		}
	}
	if len(results) != 4 {
		t.Errorf("got %d results for 4 calls", len(results))
	}
}

func overlaps(a, b span) bool { return a.start.Before(b.end) && b.start.Before(a.end) }

func toolResults(sess *testBuffer) map[string]string {
	out := map[string]string{}
	for _, m := range sess.messages {
		if m.Role == "tool" {
			out[m.ToolCallID] = m.Content
		}
	}
	return out
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
