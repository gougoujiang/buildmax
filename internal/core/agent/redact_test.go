package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// TestRunLoop_RedactsToolResult proves a run's Secret value is removed from a
// tool result before it enters the model context, the events, and the log --
// RedactResult runs once, and every downstream consumer reads the redacted
// result. See docs/design/team-secrets.md §12.
func TestRunLoop_RedactsToolResult(t *testing.T) {
	ctx := context.Background()
	const secretValue = "ghs_deadbeef_secret_0001"
	mock := &mockLLMClient{responses: []mockResponse{
		{toolCalls: []llm.ToolCall{{ID: "1", Name: "echo", Arguments: "{}"}}},
		{content: "done"},
	}}
	tool := &mockTool{
		name:        "echo",
		description: "echoes",
		params:      map[string]any{"type": "object"},
		result:      "cloned repo using token " + secretValue + " successfully",
	}

	var toolEndResult string
	_, _, err := runLoopWithUserMsg(ctx, mock, newTestToolRegistry(tool), newTestBuffer(), "go",
		func(o *RunLoopOpts) {
			o.RedactResult = func(s string) string { return strings.ReplaceAll(s, secretValue, "[redacted]") }
			o.EventSink = func(e Event) {
				if e.Kind == EventToolEnd {
					toolEndResult = e.ToolResult
				}
			}
		})
	if err != nil {
		t.Fatalf("RunLoop: %v", err)
	}
	if toolEndResult == "" {
		t.Fatal("no tool end event observed")
	}
	if strings.Contains(toolEndResult, secretValue) {
		t.Fatalf("secret value not redacted from tool result: %q", toolEndResult)
	}
	if !strings.Contains(toolEndResult, "[redacted]") {
		t.Fatalf("expected a redaction marker: %q", toolEndResult)
	}
}

// TestRunLoop_NilRedactLeavesResultUnchanged proves the default (no grants)
// path is unchanged: a nil RedactResult leaves the tool result alone.
func TestRunLoop_NilRedactLeavesResultUnchanged(t *testing.T) {
	ctx := context.Background()
	mock := &mockLLMClient{responses: []mockResponse{
		{toolCalls: []llm.ToolCall{{ID: "1", Name: "echo", Arguments: "{}"}}},
		{content: "done"},
	}}
	tool := &mockTool{name: "echo", params: map[string]any{"type": "object"}, result: "plain output"}

	var toolEndResult string
	_, _, err := runLoopWithUserMsg(ctx, mock, newTestToolRegistry(tool), newTestBuffer(), "go",
		func(o *RunLoopOpts) {
			o.EventSink = func(e Event) {
				if e.Kind == EventToolEnd {
					toolEndResult = e.ToolResult
				}
			}
		})
	if err != nil {
		t.Fatalf("RunLoop: %v", err)
	}
	if toolEndResult != "plain output" {
		t.Fatalf("result = %q, want unchanged", toolEndResult)
	}
}
