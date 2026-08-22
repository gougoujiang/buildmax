package agent

import (
	"context"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

type ctxCapturingTool struct {
	sawToolCall string
	sawRunID    string
}

func (c *ctxCapturingTool) Name() string        { return "capture" }
func (c *ctxCapturingTool) Description() string { return "captures launch context" }
func (c *ctxCapturingTool) Parameters() any     { return map[string]any{"type": "object"} }
func (c *ctxCapturingTool) Execute(ctx context.Context, _ map[string]any) (string, error) {
	c.sawToolCall = ToolCallFromCtx(ctx)
	c.sawRunID = RunIDFromCtx(ctx)
	return "ok", nil
}

// The loop stamps each execution with its tool-call ID, so detached work can
// record which call launched it.
func TestExecuteCallStampsToolCallID(t *testing.T) {
	tool := &ctxCapturingTool{}
	c := &pendingCall{
		call: llm.ToolCall{ID: "call_42", Name: "capture"},
		tool: tool,
		args: map[string]any{},
	}
	ctx := CtxWithRunID(context.Background(), "rt_parent")
	executeCall(ctx, RunLoopOpts{}, c)
	if c.result != "ok" {
		t.Fatalf("result = %q", c.result)
	}
	if tool.sawToolCall != "call_42" {
		t.Errorf("tool saw tool call %q, want call_42", tool.sawToolCall)
	}
	if tool.sawRunID != "rt_parent" {
		t.Errorf("tool saw run %q, want rt_parent", tool.sawRunID)
	}
}

func TestRunCtxEmptyValues(t *testing.T) {
	ctx := context.Background()
	if got := ToolCallFromCtx(ctx); got != "" {
		t.Errorf("ToolCallFromCtx = %q, want empty", got)
	}
	if got := RunIDFromCtx(ctx); got != "" {
		t.Errorf("RunIDFromCtx = %q, want empty", got)
	}
	if CtxWithToolCall(ctx, "") != ctx || CtxWithRunID(ctx, "") != ctx {
		t.Error("empty IDs should not allocate a new context")
	}
}
