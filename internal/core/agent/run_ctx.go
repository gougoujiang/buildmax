package agent

import "context"

// The launch context for detached work. A tool that starts something owned
// beyond its own call — a subagent trace, a background job — needs to record
// which run and which tool call launched it. The run loop stamps the tool-call
// ID around each execution; the runtime assembly stamps the run ID when it
// opens the run. Both are provenance facts, not control flow.

type toolCallKey struct{}

// CtxWithToolCall marks ctx as executing the given tool call.
func CtxWithToolCall(ctx context.Context, toolCallID string) context.Context {
	if toolCallID == "" {
		return ctx
	}
	return context.WithValue(ctx, toolCallKey{}, toolCallID)
}

// ToolCallFromCtx returns the executing tool call's ID, or "".
func ToolCallFromCtx(ctx context.Context) string {
	id, _ := ctx.Value(toolCallKey{}).(string)
	return id
}

type runIDKey struct{}

// CtxWithRunID marks ctx with the identity of the run (trace run) it belongs to.
func CtxWithRunID(ctx context.Context, runID string) context.Context {
	if runID == "" {
		return ctx
	}
	return context.WithValue(ctx, runIDKey{}, runID)
}

// RunIDFromCtx returns the current run's ID, or "".
func RunIDFromCtx(ctx context.Context) string {
	id, _ := ctx.Value(runIDKey{}).(string)
	return id
}

type subagentScopeKey struct{}

// CtxMarkSubagent marks ctx as executing inside a subagent run. A subagent's
// session is discarded when it returns, so tools that detach work owned by a
// session refuse under this mark rather than hand the work to an owner nobody
// can see.
func CtxMarkSubagent(ctx context.Context) context.Context {
	return context.WithValue(ctx, subagentScopeKey{}, true)
}

// SubagentFromCtx reports whether ctx runs inside a subagent.
func SubagentFromCtx(ctx context.Context) bool {
	v, _ := ctx.Value(subagentScopeKey{}).(bool)
	return v
}
