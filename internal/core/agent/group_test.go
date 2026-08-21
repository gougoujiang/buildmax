package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// readOnlyTool declares AccessReadOnly, making it eligible to share a group.
type readOnlyTool struct{ *mockTool }

func (readOnlyTool) Access(_ map[string]any) llm.Access { return llm.AccessReadOnly }

func newPending(t *testing.T, spec string) []pendingCall {
	t.Helper()
	var out []pendingCall
	for _, s := range strings.Fields(spec) {
		c := pendingCall{call: llm.ToolCall{ID: s, Name: s}, args: map[string]any{}}
		switch {
		case strings.HasPrefix(s, "r"):
			c.tool = readOnlyTool{&mockTool{name: s}}
		case strings.HasPrefix(s, "w"):
			c.tool = &mockTool{name: s} // undeclared, so AccessWrite
		case strings.HasPrefix(s, "u"): // unknown tool
		case strings.HasPrefix(s, "x"): // already decided (bad arguments)
			c.decided = true
		}
		out = append(out, c)
	}
	return out
}

func shape(groups [][]pendingCall) string {
	var b strings.Builder
	for i, g := range groups {
		if i > 0 {
			b.WriteByte('|')
		}
		for j := range g {
			if j > 0 {
				b.WriteByte(',')
			}
			b.WriteString(g[j].call.Name)
		}
	}
	return b.String()
}

// TestGroupCalls_SequentialByDefault is D6: a limit of one restores the
// pre-refactor behaviour exactly, which is what every surface uses today.
func TestGroupCalls_SequentialByDefault(t *testing.T) {
	calls := newPending(t, "r1 r2 r3")
	for _, limit := range []int{0, 1} {
		if got := shape(groupCalls(calls, limit)); got != "r1|r2|r3" {
			t.Errorf("limit %d: shape = %q, want each call alone", limit, got)
		}
	}
}

func TestGroupCalls_AdjacentReadsMerge(t *testing.T) {
	for _, tc := range []struct{ spec, want string }{
		{"r1 r2 r3", "r1,r2,r3"},
		{"r1 w1 r2", "r1|w1|r2"},
		{"r1 r2 w1 r3", "r1,r2|w1|r3"},
		{"w1 w2", "w1|w2"},
		{"r1 u1 r2", "r1|u1|r2"}, // unknown tool is a barrier
		{"r1 x1 r2", "r1|x1|r2"}, // already-decided call is a barrier
		{"r1", "r1"},
	} {
		if got := shape(groupCalls(newPending(t, tc.spec), 8)); got != tc.want {
			t.Errorf("groupCalls(%q) = %q, want %q", tc.spec, got, tc.want)
		}
	}
}

// TestGroupCalls_NeverReorders is D2. [Write a, Read a] and [Read a, Write a]
// mean different things, so grouping may only merge neighbours.
func TestGroupCalls_NeverReorders(t *testing.T) {
	calls := newPending(t, "w1 r1 r2 w2 r3")
	var flat []string
	for _, g := range groupCalls(calls, 8) {
		for i := range g {
			flat = append(flat, g[i].call.Name)
		}
	}
	if got := strings.Join(flat, " "); got != "w1 r1 r2 w2 r3" {
		t.Errorf("call order = %q, want it unchanged", got)
	}
}

// TestGroupCalls_GroupsAreWindows keeps the stages mutating the real elements
// rather than copies, which is how a result reaches the commit stage.
func TestGroupCalls_GroupsAreWindows(t *testing.T) {
	calls := newPending(t, "r1 r2")
	groups := groupCalls(calls, 8)
	groups[0][0].result = "written through"
	if calls[0].result != "written through" {
		t.Error("a group must be a window into the batch, not a copy")
	}
}

func TestGroupCalls_Empty(t *testing.T) {
	if got := groupCalls(nil, 8); len(got) != 0 {
		t.Errorf("groupCalls(nil) = %v, want no groups", got)
	}
}

// echoTool returns its "id" argument, so a result identifies which call
// produced it.
type echoTool struct{ name string }

func (t *echoTool) Name() string                       { return t.name }
func (t *echoTool) Description() string                { return "echo" }
func (t *echoTool) Parameters() any                    { return map[string]any{} }
func (t *echoTool) Access(_ map[string]any) llm.Access { return llm.AccessReadOnly }
func (t *echoTool) Execute(_ context.Context, args map[string]any) (string, error) {
	id, _ := args["id"].(string)
	return "result-" + id, nil
}

// TestHistoryIsSchedulerIndependent is D3, and the acceptance condition the
// whole design rests on: the message list a run produces must not depend on how
// its tool calls were scheduled. Grouping changes with the limit; the history
// must not.
func TestHistoryIsSchedulerIndependent(t *testing.T) {
	run := func(limit int) []llm.Message {
		t.Helper()
		calls := make([]llm.ToolCall, 0, 5)
		for _, id := range []string{"a", "b", "c", "d", "e"} {
			calls = append(calls, llm.ToolCall{ID: id, Name: "echo", Arguments: `{"id":"` + id + `"}`})
		}
		mock := &mockLLMClient{responses: []mockResponse{
			{toolCalls: calls},
			{content: "done"},
		}}
		sess := newTestBuffer()
		_, _, err := runLoopWithUserMsg(context.Background(), mock,
			newTestToolRegistry(&echoTool{name: "echo"}), sess, "go", func(o *RunLoopOpts) {
				o.MaxParallelTools = limit
			})
		if err != nil {
			t.Fatalf("RunLoop(limit=%d): %v", limit, err)
		}
		return sess.messages
	}

	// Limit 2 as well as 8: an odd batch size against an even limit is where a
	// grouping bug would land results in the wrong order.
	sequential := run(1)
	pairs := run(2)
	grouped := run(8)

	for i := range sequential {
		if i < len(pairs) && sequential[i].Content != pairs[i].Content {
			t.Errorf("message %d differs at limit 2:\n limit 1: %+v\n limit 2: %+v", i, sequential[i], pairs[i])
		}
	}
	if len(pairs) != len(sequential) {
		t.Errorf("limit 2 produced %d messages, limit 1 produced %d", len(pairs), len(sequential))
	}

	if len(sequential) != len(grouped) {
		t.Fatalf("message counts differ: %d vs %d", len(sequential), len(grouped))
	}
	for i := range sequential {
		a, b := sequential[i], grouped[i]
		if a.Role != b.Role || a.Content != b.Content || a.ToolCallID != b.ToolCallID {
			t.Errorf("message %d differs:\n limit 1: %+v\n limit 8: %+v", i, a, b)
		}
	}

	// And the results are in call order, not completion order.
	var ids []string
	for _, m := range grouped {
		if m.Role == "tool" {
			ids = append(ids, m.ToolCallID)
		}
	}
	if got := strings.Join(ids, ""); got != "abcde" {
		t.Errorf("tool result order = %q, want call order abcde", got)
	}
}
