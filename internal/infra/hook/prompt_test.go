package hook

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	corehook "github.com/gougoujiang/buildmax/internal/core/hook"
)

type stubLLMCaller struct {
	gotPrompt string
	gotModel  string
	response  string
	err       error
}

func (s *stubLLMCaller) CompleteHookPrompt(_ context.Context, model, prompt string) (string, error) {
	s.gotModel = model
	s.gotPrompt = prompt
	return s.response, s.err
}

// TestPromptDriver_ArgumentsInterpolated asserts that "$ARGUMENTS" is
// replaced with the HookInput JSON.
func TestPromptDriver_ArgumentsInterpolated(t *testing.T) {
	caller := &stubLLMCaller{response: ""}
	d := NewPromptDriver(caller)
	d.Run(context.Background(),
		corehook.Entry{Prompt: "judge: $ARGUMENTS", Model: "fast"},
		agent.HookInput{Event: agent.HookPreToolUse, ToolName: "writefile"},
	)
	if !strings.Contains(caller.gotPrompt, `"event":"PreToolUse"`) || !strings.Contains(caller.gotPrompt, `"tool_name":"writefile"`) {
		t.Errorf("rendered prompt missing payload fields: %q", caller.gotPrompt)
	}
	if caller.gotModel != "fast" {
		t.Errorf("model = %q, want fast", caller.gotModel)
	}
}

// TestPromptDriver_JSONBlocks asserts that the LLM response can block.
func TestPromptDriver_JSONBlocks(t *testing.T) {
	caller := &stubLLMCaller{response: `{"decision":"block","reason":"model says no"}`}
	d := NewPromptDriver(caller)
	out := d.Run(context.Background(),
		corehook.Entry{Prompt: "$ARGUMENTS"},
		agent.HookInput{Event: agent.HookPreToolUse, ToolName: "x"},
	)
	if !out.Blocked() {
		t.Fatalf("expected block, got %+v", out)
	}
	if out.Reason != "model says no" {
		t.Errorf("reason = %q", out.Reason)
	}
}

// TestPromptDriver_PlainTextAllows asserts that a non-JSON response is
// treated as allow.
func TestPromptDriver_PlainTextAllows(t *testing.T) {
	caller := &stubLLMCaller{response: "looks fine to me"}
	d := NewPromptDriver(caller)
	out := d.Run(context.Background(),
		corehook.Entry{Prompt: "$ARGUMENTS"},
		agent.HookInput{Event: agent.HookPreToolUse, ToolName: "x"},
	)
	if out.Blocked() {
		t.Errorf("expected allow on non-JSON response, got %+v", out)
	}
}

// TestPromptDriver_FailsOpenOnError asserts that an LLM error returns allow.
func TestPromptDriver_FailsOpenOnError(t *testing.T) {
	caller := &stubLLMCaller{err: errors.New("network down")}
	d := NewPromptDriver(caller)
	out := d.Run(context.Background(),
		corehook.Entry{Prompt: "$ARGUMENTS"},
		agent.HookInput{Event: agent.HookPreToolUse, ToolName: "x"},
	)
	if out.Blocked() {
		t.Errorf("expected allow on LLM error, got %+v", out)
	}
}

func TestPromptDriver_Type(t *testing.T) {
	if NewPromptDriver(nil).Type() != corehook.TypePrompt {
		t.Errorf("Type() = %q", NewPromptDriver(nil).Type())
	}
}
