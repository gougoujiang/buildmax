package agentapp

import (
	"context"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	corehook "github.com/gougoujiang/buildmax/internal/core/hook"
	"github.com/gougoujiang/buildmax/internal/infra/hook"
)

// trackingDriver records every Run call and lets the test choose the
// returned HookOutput.
type trackingDriver struct {
	typeName string
	calls    []corehook.Entry
	output   agent.HookOutput
}

func (d *trackingDriver) Type() string { return d.typeName }
func (d *trackingDriver) Run(_ context.Context, entry corehook.Entry, _ agent.HookInput) agent.HookOutput {
	d.calls = append(d.calls, entry)
	return d.output
}

// TestHookManager_DispatchesByType verifies that the manager routes each
// entry to the driver matching its Type field.
func TestHookManager_DispatchesByType(t *testing.T) {
	cmd := &trackingDriver{typeName: corehook.TypeCommand}
	http := &trackingDriver{typeName: corehook.TypeHTTP}
	m := NewHookManager(corehook.Config{
		PreToolUse: []corehook.Entry{
			{Type: corehook.TypeCommand, Matcher: "writefile", Command: "x"},
			{Type: corehook.TypeHTTP, Matcher: "writefile", URL: "https://example"},
		},
	}, map[string]hook.Driver{
		corehook.TypeCommand: cmd,
		corehook.TypeHTTP:    http,
	})

	out := m.Run(context.Background(), agent.HookInput{Event: agent.HookPreToolUse, ToolName: "writefile"})
	if out.Blocked() {
		t.Errorf("expected allow, got %+v", out)
	}
	if len(cmd.calls) != 1 || cmd.calls[0].Command != "x" {
		t.Errorf("command driver calls = %+v", cmd.calls)
	}
	if len(http.calls) != 1 || http.calls[0].URL != "https://example" {
		t.Errorf("http driver calls = %+v", http.calls)
	}
}

// TestHookManager_MatcherFiltering asserts that only entries matching the
// tool name are dispatched.
func TestHookManager_MatcherFiltering(t *testing.T) {
	cmd := &trackingDriver{typeName: corehook.TypeCommand}
	m := NewHookManager(corehook.Config{
		PreToolUse: []corehook.Entry{
			{Type: corehook.TypeCommand, Matcher: "^writefile$", Command: "write"},
			{Type: corehook.TypeCommand, Matcher: "^bash$", Command: "bash"},
		},
	}, map[string]hook.Driver{corehook.TypeCommand: cmd})

	m.Run(context.Background(), agent.HookInput{Event: agent.HookPreToolUse, ToolName: "writefile"})

	if len(cmd.calls) != 1 || cmd.calls[0].Command != "write" {
		t.Errorf("expected only matching entry to run; got %+v", cmd.calls)
	}
}

// TestHookManager_FirstBlockWinsButOthersStillRun asserts the aggregation
// rule: first blocking decision wins, but every matching entry still runs.
func TestHookManager_FirstBlockWinsButOthersStillRun(t *testing.T) {
	first := &trackingDriver{typeName: "first", output: agent.HookOutput{Decision: agent.HookDecisionBlock, Reason: "first"}}
	second := &trackingDriver{typeName: "second", output: agent.HookOutput{Decision: agent.HookDecisionBlock, Reason: "second"}}
	m := NewHookManager(corehook.Config{
		PreToolUse: []corehook.Entry{
			{Type: "first", Matcher: "x"},
			{Type: "second", Matcher: "x"},
		},
	}, map[string]hook.Driver{"first": first, "second": second})

	out := m.Run(context.Background(), agent.HookInput{Event: agent.HookPreToolUse, ToolName: "x"})
	if !out.Blocked() || out.Reason != "first" {
		t.Errorf("expected block from first, got %+v", out)
	}
	if len(first.calls) != 1 || len(second.calls) != 1 {
		t.Errorf("expected both drivers to run; first=%d second=%d", len(first.calls), len(second.calls))
	}
}

// TestHookManager_UnknownTypeSkipped asserts that an entry whose resolved
// type has no registered driver is skipped (not a panic).
func TestHookManager_UnknownTypeSkipped(t *testing.T) {
	m := NewHookManager(corehook.Config{
		PreToolUse: []corehook.Entry{{Type: "agent", Matcher: "x", Prompt: "..."}},
	}, map[string]hook.Driver{})
	out := m.Run(context.Background(), agent.HookInput{Event: agent.HookPreToolUse, ToolName: "x"})
	if out.Blocked() {
		t.Errorf("expected allow when driver missing, got %+v", out)
	}
}

// TestHookManager_AdvisoryEventsIgnoreMatcher asserts that for non-tool
// events entries with a non-empty matcher are skipped, and entries with an
// empty matcher run.
func TestHookManager_AdvisoryEventsIgnoreMatcher(t *testing.T) {
	cmd := &trackingDriver{typeName: corehook.TypeCommand}
	m := NewHookManager(corehook.Config{
		Stop: []corehook.Entry{
			{Type: corehook.TypeCommand, Matcher: "writefile", Command: "skip"},
			{Type: corehook.TypeCommand, Command: "run"},
		},
	}, map[string]hook.Driver{corehook.TypeCommand: cmd})

	m.Run(context.Background(), agent.HookInput{Event: agent.HookStop})
	if len(cmd.calls) != 1 || cmd.calls[0].Command != "run" {
		t.Errorf("expected only no-matcher entry to run, got %+v", cmd.calls)
	}
}

// TestHookManager_StatusReportsCounts asserts that Status surfaces
// per-event counts and configured driver types.
func TestHookManager_StatusReportsCounts(t *testing.T) {
	cmd := &trackingDriver{typeName: corehook.TypeCommand}
	m := NewHookManager(corehook.Config{
		PreToolUse: []corehook.Entry{{Type: corehook.TypeCommand}, {Type: corehook.TypeCommand}},
		Stop:       []corehook.Entry{{Type: corehook.TypeCommand}},
	}, map[string]hook.Driver{corehook.TypeCommand: cmd})

	st := m.Status()
	if st.TotalHooks != 3 {
		t.Errorf("TotalHooks = %d, want 3", st.TotalHooks)
	}
	if st.EventCounts[corehook.EventPreToolUse] != 2 || st.EventCounts[corehook.EventStop] != 1 {
		t.Errorf("EventCounts = %+v", st.EventCounts)
	}
	if len(st.Types) != 1 || st.Types[0] != corehook.TypeCommand {
		t.Errorf("Types = %v", st.Types)
	}
}
