package hook

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/agent"
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
func (PromptDriver) Type() string { return config.HookTypePrompt }

// Run renders the configured prompt and asks the LLM for a decision.
func (d *PromptDriver) Run(ctx context.Context, entry config.HookEntry, in agent.HookInput) agent.HookOutput {
	if d == nil || d.caller == nil {
		slog.Warn("hook: prompt driver has no caller; failing open", "event", in.Event)
		return agent.HookOutput{}
	}
	if entry.Prompt == "" {
		slog.Warn("hook: prompt entry missing prompt", "event", in.Event)
		return agent.HookOutput{}
	}

	payload, err := json.Marshal(in)
	if err != nil {
		slog.Warn("hook: marshal prompt arguments failed; failing open", "event", in.Event, "err", err)
		return agent.HookOutput{}
	}
	prompt := strings.ReplaceAll(entry.Prompt, "$ARGUMENTS", string(payload))

	callCtx, cancel := context.WithTimeout(ctx, resolveTimeout(entry.Timeout))
	defer cancel()

	resp, err := d.caller.CompleteHookPrompt(callCtx, entry.Model, prompt)
	if err != nil {
		slog.Warn("hook: prompt llm call failed; failing open",
			"event", in.Event,
			"model", entry.Model,
			"err", err,
		)
		return agent.HookOutput{}
	}

	out, ok := parseHookOutput([]byte(resp))
	if !ok {
		slog.Debug("hook: prompt response not parsable; allowing", "event", in.Event, "response", truncate(resp, 200))
		return agent.HookOutput{}
	}
	if out.Blocked() {
		slog.Info("hook: prompt blocked", "event", in.Event, "tool", in.ToolName, "model", entry.Model, "reason", out.Reason)
	}
	return out
}
