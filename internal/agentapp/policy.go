package agentapp

import (
	"log/slog"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// configuredPolicy answers from the user's tools.permissions block, deferring to
// whatever policy the surface supplied when no rule matches.
type configuredPolicy struct {
	res      config.PermissionResolution
	fallback agent.ToolPolicy
}

// NewConfiguredPolicy layers settings.yaml rules over a surface policy. Invalid
// actions are logged once and skipped: one bad rule must not stop the agent.
func NewConfiguredPolicy(res config.PermissionResolution, fallback agent.ToolPolicy) agent.ToolPolicy {
	for _, bad := range res.Invalid {
		slog.Warn("tools.permissions: unknown action, rule ignored", "rule", bad)
	}
	if len(res.Entries) == 0 {
		if fallback != nil {
			return fallback
		}
		return agent.AllowAllPolicy()
	}
	return configuredPolicy{res: res, fallback: fallback}
}

func (p configuredPolicy) Check(name, scope string, args map[string]any) (llm.ToolAction, bool) {
	if e, ok := p.res.Lookup(name, scope); ok {
		switch e.Action {
		case config.PermissionDeny:
			return llm.ToolActionDeny, true
		case config.PermissionAsk:
			return llm.ToolActionAsk, true
		case config.PermissionAllow:
			return llm.ToolActionAllow, true
		}
	}
	if p.fallback != nil {
		return p.fallback.Check(name, scope, args)
	}
	return llm.ToolActionAllow, false
}
