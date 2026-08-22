package hook

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	corehook "github.com/gougoujiang/buildmax/internal/core/hook"
)

// PromptDriver runs a single-turn LLM prompt as a hook. The literal
// "$ARGUMENTS" inside entry.Prompt is replaced with the HookInput JSON so
// the model can inspect every field of the event payload. The response is
// parsed as JSON looking for {decision, reason}; anything else is treated
// as allow.
//
// Note: PromptDriver makes one LLM call per matching event, which can
// dominate turn latency if used on hot paths (PreToolUse / PostToolUse).
// Pair it with a cheap default model in settings.yaml.
type PromptDriver struct {
	caller LLMCaller
}

// NewPromptDriver returns a driver backed by caller.
func NewPromptDriver(caller LLMCaller) *PromptDriver { return &PromptDriver{caller: caller} }

// Type satisfies Driver.
func (PromptDriver) Type() string { return corehook.TypePrompt }

// Run renders the configured prompt and asks the LLM for a decision.
func (d *PromptDriver) Run(ctx context.Context, entry corehook.Entry, in agent.HookInput) agent.HookOutput {
	if d == nil || d.caller == nil {
		componentLog().Warn("prompt driver has no caller; failing open", "event", in.Event)
		return agent.HookOutput{}
	}
	if entry.Prompt == "" {
		componentLog().Warn("prompt entry missing prompt", "event", in.Event)
		return agent.HookOutput{}
	}

	payload, err := json.Marshal(in)
	if err != nil {
		componentLog().Warn("marshal prompt arguments failed; failing open", "event", in.Event, "err", err)
		return agent.HookOutput{}
	}
	prompt := strings.ReplaceAll(entry.Prompt, "$ARGUMENTS", string(payload))

	callCtx, cancel := context.WithTimeout(ctx, resolveTimeout(entry.Timeout))
	defer cancel()

	resp, err := d.caller.CompleteHookPrompt(callCtx, entry.Model, prompt)
	if err != nil {
		componentLog().Warn("prompt llm call failed; failing open",
			"event", in.Event,
			"model", entry.Model,
			"err", err,
		)
		return agent.HookOutput{}
	}

	out, ok := parseHookOutput([]byte(resp))
	if !ok {
		componentLog().Debug("prompt response not parsable; allowing", "event", in.Event, "response", truncate(resp, 200))
		return agent.HookOutput{}
	}
	if out.Blocked() {
		componentLog().Info("prompt blocked", "event", in.Event, "tool", in.ToolName, "model", entry.Model, "reason", out.Reason)
	}
	return out
}
