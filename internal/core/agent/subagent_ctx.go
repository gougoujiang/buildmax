package agent

import "context"

type subagentScopeKey struct{}

// CtxMarkSubagent marks ctx as executing inside a subagent run. A subagent's
// session is discarded when it returns, so tools that detach work owned by a
// session (background jobs) refuse under this mark rather than hand the work
// to an owner nobody can see.
func CtxMarkSubagent(ctx context.Context) context.Context {
	return context.WithValue(ctx, subagentScopeKey{}, true)
}

// SubagentFromCtx reports whether ctx runs inside a subagent.
func SubagentFromCtx(ctx context.Context) bool {
	v, _ := ctx.Value(subagentScopeKey{}).(bool)
	return v
}
