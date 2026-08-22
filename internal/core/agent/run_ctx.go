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
