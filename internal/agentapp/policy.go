package agentapp

import (
	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// interactivePolicy defers all decisions to each tool's own ArgChecker / PolicyProvider.
// Pair with an ApprovalHandler at the call site to surface Ask prompts to the user.
type interactivePolicy struct{}

func (interactivePolicy) Check(_ string, _ map[string]any) llm.ToolAction {
	return llm.ToolActionAllow
}

// nonInteractivePolicy defers all decisions to each tool's own declarations.
// Since no ApprovalHandler is set on non-interactive surfaces, any Ask returned by a tool
// automatically collapses to Deny in the agent resolution loop.
type nonInteractivePolicy struct{}

func (nonInteractivePolicy) Check(_ string, _ map[string]any) llm.ToolAction {
	return llm.ToolActionAllow
}

// NewInteractivePolicy returns the policy for interactive surfaces (CLI TUI, Desktop).
// Tool-declared Ask actions will surface an approval prompt via the ApprovalHandler.
func NewInteractivePolicy() agent.ToolPolicy {
	return interactivePolicy{}
}

// NewNonInteractivePolicy returns the policy for non-interactive surfaces (worker, print mode,
// portal conversation). Tool-declared Ask actions collapse to Deny because no ApprovalHandler
// is set on these surfaces.
func NewNonInteractivePolicy() agent.ToolPolicy {
	return nonInteractivePolicy{}
}
