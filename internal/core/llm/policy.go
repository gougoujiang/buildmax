package llm

// ToolAction is the outcome of a policy check before a tool call executes.
type ToolAction int

const (
	// ToolActionAllow executes the tool unconditionally.
	ToolActionAllow ToolAction = iota
	// ToolActionAsk requests interactive approval before execution.
	ToolActionAsk
	// ToolActionDeny blocks execution and returns an error result to the model.
	ToolActionDeny
)

// PolicyProvider is an optional interface a Tool can implement to declare its default action.
// The agent uses this as a fallback when no arg-level check applies.
type PolicyProvider interface {
	DefaultAction() ToolAction
}

// ArgChecker is an optional interface a Tool can implement to make arg-level policy decisions
// before Execute is called. Return ToolActionAllow to proceed normally.
type ArgChecker interface {
	CheckArgs(args map[string]any) ToolAction
}
