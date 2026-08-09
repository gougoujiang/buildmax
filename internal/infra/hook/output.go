package hook

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/agent"
)

// parseHookOutput tries to decode an agent.HookOutput from a hook's textual
// response. Returns (output, true) when the body looks like a JSON object
// that includes the "decision" or "reason" key; otherwise (zero, false).
// Used by every driver whose response channel carries text (command stdout,
// HTTP body, MCP tool text, prompt completion).
func parseHookOutput(b []byte) (agent.HookOutput, bool) {
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return agent.HookOutput{}, false
	}
	var raw struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	if err := json.Unmarshal(trimmed, &raw); err != nil {
		return agent.HookOutput{}, false
	}
	if raw.Decision == "" && raw.Reason == "" {
		return agent.HookOutput{}, false
	}
	out := agent.HookOutput{Reason: raw.Reason}
	if raw.Decision == string(agent.HookDecisionBlock) {
		out.Decision = agent.HookDecisionBlock
	}
	return out, true
}

// resolveTimeout returns the duration to enforce on one hook invocation;
// zero or negative entries fall back to config.DefaultHookTimeoutSecs.
func resolveTimeout(seconds int) time.Duration {
	if seconds <= 0 {
		return time.Duration(config.DefaultHookTimeoutSecs) * time.Second
	}
	return time.Duration(seconds) * time.Second
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
