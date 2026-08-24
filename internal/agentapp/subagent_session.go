package agentapp

import (
	"context"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/session"
	tools "github.com/gougoujiang/buildmax/internal/tool"
)

// newSubAgentSession gives one subagent run its own hidden session bundle.
//
// §9: a subagent never writes into the parent's journal or durable state, and
// its bundle records the immediate parent session, run, tool call, agent type,
// and delegation depth. Being hidden keeps it out of the picker and out of
// --continue while leaving it on disk, which is what makes a failed delegation
// inspectable after the fact instead of vanishing with the Task result.
func (a *AgentApp) newSubAgentSession(ctx context.Context, opts tools.SubAgentRunOpts) (tools.SubAgentSession, error) {
	if a == nil || a.sessionManager == nil {
		// No store to write to. An in-memory session is the honest fallback:
		// the delegation still runs, it just leaves no bundle behind.
		return nil, nil
	}
	parentID, _ := session.SessionIDFromContext(ctx)
	model := opts.Model
	if model == "" {
		model = a.DefaultModelName()
	}
	// Depth is not tracked on the context today, so it is recorded as one
	// level below whatever delegated: honest for the common case, and a
	// placeholder a nested-delegation change would replace rather than a
	// number invented to look complete.
	return a.sessionManager.CreateSubagent(model, session.Meta{
		ParentSessionID:  parentID,
		ParentRunID:      traceRunFromContext(ctx).runID,
		ParentToolCallID: agent.ToolCallFromCtx(ctx),
		AgentType:        opts.Description,
		DelegationDepth:  1,
	})
}
