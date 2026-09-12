package audit

import "context"

// runContextKey carries the public id of the task run a request is serving.
type runContextKey struct{}

// ContextWithRun tags ctx with the task run a request serves, so an audit event
// recorded downstream — a worker uploading an artifact, the gateway refusing a
// call against a quota — can name the run that caused it without every call site
// threading the id by hand. An empty id leaves ctx unchanged.
func ContextWithRun(ctx context.Context, taskRunID string) context.Context {
	if taskRunID == "" {
		return ctx
	}
	return context.WithValue(ctx, runContextKey{}, taskRunID)
}

// RunFromContext returns the task run id tagged onto ctx, or "" when the action
// is not being taken on behalf of a run. The write sink uses it to stamp an
// event that did not name a run explicitly, which is why an event's own TaskRunID
// still wins: a caller that knows the run better than the ambient context should
// not be overridden by it.
func RunFromContext(ctx context.Context) string {
	id, _ := ctx.Value(runContextKey{}).(string)
	return id
}
