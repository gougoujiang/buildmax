package tool

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// scriptedClient answers each turn from a fixed script: the first completion
// carries the tool calls, the second ends the run.
type scriptedClient struct {
	mu    sync.Mutex
	turn  int
	calls []llm.ToolCall
}

func (c *scriptedClient) ChatCompletionBlocking(_ context.Context, _ llm.Request) (llm.Completion, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.turn++
	if c.turn == 1 {
		return llm.Completion{ToolCalls: c.calls}, nil
	}
	return llm.Completion{Content: "done"}, nil
}

func (c *scriptedClient) ChatCompletionStreaming(ctx context.Context, req llm.Request, _ func(string)) (llm.Completion, error) {
	return c.ChatCompletionBlocking(ctx, req)
}

func (c *scriptedClient) ContextWindow() int { return 0 }

// sleepingReadTool is read-only and takes a measurable amount of time, so a
// batch of them shows whether the sub-agent's loop overlapped them.
type sleepingReadTool struct{ d time.Duration }

func (t *sleepingReadTool) Name() string        { return ToolNameRead }
func (t *sleepingReadTool) Description() string { return "sleep" }
func (t *sleepingReadTool) Parameters() any     { return map[string]any{"type": "object"} }
func (t *sleepingReadTool) Access(_ map[string]any) llm.Access {
	return llm.AccessReadOnly
}

func (t *sleepingReadTool) Execute(_ context.Context, _ map[string]any) (string, error) {
	time.Sleep(t.d)
	return "read", nil
}

// A sub-agent runs its own RunLoop, and without the limit that loop schedules
// every call on its own. The exploration agent types are read-only by
// construction, so the setting has to reach them or it misses the workload it
// was written for.
func TestSubAgentRunner_MaxParallelToolsReachesTheNestedLoop(t *testing.T) {
	const sleep = 80 * time.Millisecond
	calls := []llm.ToolCall{
		{ID: "a", Name: ToolNameRead, Arguments: "{}"},
		{ID: "b", Name: ToolNameRead, Arguments: "{}"},
		{ID: "c", Name: ToolNameRead, Arguments: "{}"},
	}

	run := func(t *testing.T, opts ...SubAgentRunnerOption) time.Duration {
		t.Helper()
		runner, err := NewDefaultSubAgentRunner(&scriptedClient{calls: calls}, nil, nil, opts...)
		if err != nil {
			t.Fatalf("NewDefaultSubAgentRunner: %v", err)
		}
		start := time.Now()
		reply, err := runner.RunSubAgent(context.Background(), SubAgentRunOpts{
			Tools:       []llm.Tool{&sleepingReadTool{d: sleep}},
			Description: "explore",
		}, "go")
		if err != nil {
			t.Fatalf("RunSubAgent: %v", err)
		}
		if reply != "done" {
			t.Fatalf("reply = %q, want %q", reply, "done")
		}
		return time.Since(start)
	}

	t.Run("parallel", func(t *testing.T) {
		if elapsed := run(t, WithSubAgentMaxParallelTools(4)); elapsed >= 3*sleep {
			t.Errorf("three %v reads took %v; want them overlapped", sleep, elapsed)
		}
	})

	t.Run("sequential without the option", func(t *testing.T) {
		if elapsed := run(t); elapsed < 3*sleep {
			t.Errorf("three %v reads took %v; want them sequential", sleep, elapsed)
		}
	})
}
