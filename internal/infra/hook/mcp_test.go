package hook

import (
	"context"
	"errors"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	corehook "github.com/gougoujiang/buildmax/internal/core/hook"
)

type stubMCPCaller struct {
	wantServer string
	wantTool   string
	gotInput   map[string]any
	result     string
	err        error
}

func (s *stubMCPCaller) CallMCPTool(_ context.Context, server, tool string, input map[string]any) (string, error) {
	if server != s.wantServer || tool != s.wantTool {
		return "", errors.New("unexpected server/tool")
	}
	s.gotInput = input
	return s.result, s.err
}

// TestMCPDriver_PassesSubstitutedInput verifies that ${field} placeholders are
// resolved from the HookInput payload before the MCP call.
func TestMCPDriver_PassesSubstitutedInput(t *testing.T) {
	caller := &stubMCPCaller{wantServer: "scanner", wantTool: "scan", result: ""}
	d := NewMCPDriver(caller)

	out := d.Run(context.Background(),
		corehook.Entry{
			Server: "scanner",
			Tool:   "scan",
			Input:  map[string]any{"path": "${tool_args.path}", "event": "${event}"},
		},
		agent.HookInput{
			Event:    agent.HookPreToolUse,
			ToolName: "writefile",
			ToolArgs: map[string]any{"path": "/tmp/out.txt"},
		},
	)
	if out.Blocked() {
		t.Errorf("expected allow, got %+v", out)
	}
	if caller.gotInput["path"] != "/tmp/out.txt" {
		t.Errorf("path = %v, want /tmp/out.txt", caller.gotInput["path"])
	}
	if caller.gotInput["event"] != "PreToolUse" {
		t.Errorf("event = %v, want PreToolUse", caller.gotInput["event"])
	}
}

// TestMCPDriver_ToolResultBlocks asserts that a JSON-shaped tool result with
// {decision:block} is honored.
func TestMCPDriver_ToolResultBlocks(t *testing.T) {
	caller := &stubMCPCaller{wantServer: "scanner", wantTool: "scan", result: `{"decision":"block","reason":"flagged"}`}
	d := NewMCPDriver(caller)
	out := d.Run(context.Background(),
		corehook.Entry{Server: "scanner", Tool: "scan"},
		agent.HookInput{Event: agent.HookPreToolUse, ToolName: "writefile"},
	)
	if !out.Blocked() {
		t.Fatalf("expected block, got %+v", out)
	}
	if out.Reason != "flagged" {
		t.Errorf("reason = %q", out.Reason)
	}
}

// TestMCPDriver_FailsOpenOnError asserts that an MCP error returns allow.
func TestMCPDriver_FailsOpenOnError(t *testing.T) {
	caller := &stubMCPCaller{wantServer: "scanner", wantTool: "scan", err: errors.New("server unreachable")}
	d := NewMCPDriver(caller)
	out := d.Run(context.Background(),
		corehook.Entry{Server: "scanner", Tool: "scan"},
		agent.HookInput{Event: agent.HookPreToolUse, ToolName: "writefile"},
	)
	if out.Blocked() {
		t.Errorf("expected allow on caller error, got %+v", out)
	}
}

// TestMCPDriver_MissingFieldsFailOpen asserts that an entry missing server
// or tool returns allow rather than panicking.
func TestMCPDriver_MissingFieldsFailOpen(t *testing.T) {
	d := NewMCPDriver(&stubMCPCaller{})
	out := d.Run(context.Background(), corehook.Entry{}, agent.HookInput{Event: agent.HookPreToolUse})
	if out.Blocked() {
		t.Errorf("expected allow when server/tool empty, got %+v", out)
	}
}

func TestMCPDriver_Type(t *testing.T) {
	if NewMCPDriver(nil).Type() != corehook.TypeMCP {
		t.Errorf("Type() = %q", NewMCPDriver(nil).Type())
	}
}
