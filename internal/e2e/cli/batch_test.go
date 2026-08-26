package clie2e

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// docs/design/parallel-tool-execution.md D3 promises a run's message list is
// byte-identical whether the concurrency limit is 1 or 16, and D2 that calls
// never reorder. Nothing below this level can hold it to that: the promise is
// about the real scheduler driving the real permission gate through a real
// process, and a unit test that swaps any of the three is proving something
// else. docs/design/end-to-end-testing.md §6 puts the assertion here for that
// reason.
//
// The comparison is the second request the run makes, because that is the
// first one carrying what the batch produced: the assistant message with its
// tool calls, and one tool result per call.
const (
	sequential = "agent:\n  max_parallel_tools: 1\n"
	concurrent = "agent:\n  max_parallel_tools: 8\n"
)

func TestABatchOfReadsProducesOneCanonicalHistory(t *testing.T) {
	workspace := batchWorkspace(t)

	one := batchRun(t, "batched-reads.json", workspace, sequential, nil)
	eight := batchRun(t, "batched-reads.json", workspace, concurrent, nil)

	assertSameHistory(t, one, eight)
	for _, run := range []batchResult{one, eight} {
		assertOneResultPerCall(t, run, 3)
		if got := toolOrder(run.result); len(got) != 3 {
			t.Fatalf("%s: tool_end events = %v, want three reads", run.label, got)
		}
	}
}

// TestABatchWithAGatedWriteProducesOneCanonicalHistory is the case the design
// singles out: a write among read-only neighbours. The write is a barrier, so
// it is also where a scheduler that grouped too eagerly would show up — and
// where D4 is tested, since print mode has no approval handler and the pinned
// Ask collapses to a denial that still owes the history a result.
func TestABatchWithAGatedWriteProducesOneCanonicalHistory(t *testing.T) {
	workspace := batchWorkspace(t)
	pinned := map[string]string{"Read": "allow", "Write": "ask"}

	one := batchRun(t, "batched-write-among-reads.json", workspace, sequential, pinned)
	eight := batchRun(t, "batched-write-among-reads.json", workspace, concurrent, pinned)

	assertSameHistory(t, one, eight)

	for _, run := range []batchResult{one, eight} {
		assertOneResultPerCall(t, run, 3)
		if _, err := os.Stat(filepath.Join(workspace, "out.txt")); !os.IsNotExist(err) {
			t.Fatalf("%s: the refused write reached the workspace (stat err = %v)", run.label, err)
		}
		denied := run.result.events("tool_denied")
		if len(denied) != 1 || denied[0]["tool"] != "Write" {
			t.Fatalf("%s: tool_denied = %v, want one Write", run.label, denied)
		}
		// D2: the reads keep the order the model emitted them in, and the
		// denial in the middle does not cost the batch its second read.
		if got := toolOrder(run.result); len(got) != 2 {
			t.Fatalf("%s: tools that ran = %v, want the two reads", run.label, got)
		}
	}
}

type batchResult struct {
	label   string
	result  runResult
	history []byte
}

// batchRun replays one scenario at one concurrency limit and returns the
// request that carried the batch's results back to the model.
func batchRun(t *testing.T, scenario, workspace, agentSettings string, permissions map[string]string) batchResult {
	t.Helper()
	server := startModel(t, scenario)
	home := writeHomeWith(t, server, permissions, agentSettings)

	result := run(t, home, workspace, "-p", "work through the batch", "--output", "jsonl")
	if result.exitCode != 0 {
		t.Fatalf("%s: exit code = %d, want 0\nstdout:\n%s\nstderr:\n%s",
			agentSettings, result.exitCode, result.stdout, result.stderr)
	}
	if remaining := server.Remaining(); remaining != 0 {
		t.Fatalf("%s: unconsumed scenario steps = %d, want 0", agentSettings, remaining)
	}
	requests := server.Requests()
	if len(requests) != 2 {
		t.Fatalf("%s: model calls = %d, want 2 (the batch, then the closing turn)", agentSettings, len(requests))
	}
	return batchResult{label: agentSettings, result: result, history: requests[1].Body}
}

// assertSameHistory compares the two runs byte for byte, and prints the first
// differing message rather than two request bodies when they disagree: the
// failure worth reading is which message moved, not how long the prompt is.
func assertSameHistory(t *testing.T, a, b batchResult) {
	t.Helper()
	if bytes.Equal(a.history, b.history) {
		return
	}
	left, right := messages(t, a.history), messages(t, b.history)
	for i := 0; i < len(left) || i < len(right); i++ {
		switch {
		case i >= len(left):
			t.Fatalf("limit 8 sent a %d%s message the sequential run did not: %s", i+1, ordinal(i+1), right[i])
		case i >= len(right):
			t.Fatalf("the sequential run sent a %d%s message limit 8 did not: %s", i+1, ordinal(i+1), left[i])
		case !bytes.Equal(left[i], right[i]):
			t.Fatalf("message %d differs between concurrency limits 1 and 8\n limit 1: %s\n limit 8: %s", i+1, left[i], right[i])
		}
	}
	t.Fatalf("the histories differ outside the message list\n limit 1: %s\n limit 8: %s", a.history, b.history)
}

// assertOneResultPerCall holds the history to D4 — every tool call gets
// exactly one result — and to D2, by requiring the results to arrive in call
// order rather than in whatever order the workers finished.
//
// It also stops the comparison above from passing on two empty histories,
// which is the way a byte-equality test fails to test anything.
func assertOneResultPerCall(t *testing.T, run batchResult, want int) {
	t.Helper()
	var body struct {
		Messages []struct {
			Role      string `json:"role"`
			ToolCalls []struct {
				ID string `json:"id"`
			} `json:"tool_calls"`
			ToolCallID string `json:"tool_call_id"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(run.history, &body); err != nil {
		t.Fatalf("%s: decode recorded request: %v", run.label, err)
	}
	var called, answered []string
	for _, m := range body.Messages {
		for _, c := range m.ToolCalls {
			called = append(called, c.ID)
		}
		if m.Role == "tool" {
			answered = append(answered, m.ToolCallID)
		}
	}
	if len(called) != want {
		t.Fatalf("%s: the batch carried %d tool calls, want %d — the scenario is not exercising a batch", run.label, len(called), want)
	}
	if !slices.Equal(called, answered) {
		t.Fatalf("%s: results do not match the calls in order\n calls:   %v\n results: %v", run.label, called, answered)
	}
}

// messages pulls the message list out of a recorded request so a mismatch can
// name the message rather than the whole body.
func messages(t *testing.T, body []byte) [][]byte {
	t.Helper()
	var req struct {
		Messages []json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("decode recorded request: %v\n%s", err, body)
	}
	out := make([][]byte, len(req.Messages))
	for i, m := range req.Messages {
		out[i] = m
	}
	return out
}

func ordinal(n int) string {
	switch {
	case n%100 >= 11 && n%100 <= 13:
		return "th"
	case n%10 == 1:
		return "st"
	case n%10 == 2:
		return "nd"
	case n%10 == 3:
		return "rd"
	}
	return "th"
}

// toolOrder is the tools that finished, in the order they finished reporting.
func toolOrder(r runResult) []string {
	var out []string
	for _, e := range r.events("tool_end") {
		name, _ := e["tool"].(string)
		out = append(out, name)
	}
	return out
}

// batchWorkspace is shared by both runs of a comparison on purpose: file
// contents reach the model inside the tool results, so two workspaces would
// make the histories differ for a reason that has nothing to do with the
// scheduler. Every scripted call here reads, and the one write is refused.
func batchWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range map[string]string{
		"alpha.txt": "the first file\n",
		"beta.txt":  "the second file\n",
		"gamma.txt": "the third file\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	return dir
}
