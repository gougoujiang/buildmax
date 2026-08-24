package agent

import "github.com/gougoujiang/buildmax/internal/core/llm"

// Tool outcome statuses.
//
// These are the vocabulary a durable history records for a finished call.
// ToolStatusDenied is distinct from ToolStatusFailed because they mean
// different things to whoever reads the transcript later: denied means
// BuildMax refused, failed means the tool tried and could not.
const (
	ToolStatusCompleted = "completed"
	ToolStatusFailed    = "failed"
	ToolStatusDenied    = "denied"
)

// ToolCallStart identifies one approved call that is about to enter its tool.
type ToolCallStart struct {
	ID   string
	Name string
}

// ToolOutcome is one call's observed result.
type ToolOutcome struct {
	ID     string
	Name   string
	Status string
	Result string
	Parts  []llm.ContentPart
}

// ToolBoundaryHistory is an optional extension of MessageHistory implemented by
// histories durable enough to record when a call crossed into its tool, in the
// same way CompactionHistory extends it for the compaction boundary.
//
// The distinction it buys is the one an interrupted run cannot otherwise make.
// An assistant tool call proves only that the model asked. Without a record
// written before the tool ran, a crash leaves no way to tell a call that never
// started from one that may already have changed the world, and a resumed run
// would have to either retry it — possibly twice — or drop it. See
// docs/design/local-session-storage.md §7.3.
//
// A history that does not implement this still works: RunLoop falls back to
// appending the tool-role message alone, which is what an in-memory history
// wants and all any of them can honestly offer.
type ToolBoundaryHistory interface {
	MessageHistory
	// ToolExecutionStarted records that these approved calls are about to run.
	// It must not return until that record is durable, because everything it
	// is for depends on surviving the tool it precedes.
	ToolExecutionStarted(calls []ToolCallStart) error
	// AppendToolResult records one call's observed outcome and projects it into
	// the conversation, replacing the tool-role Append a plain history takes.
	AppendToolResult(out ToolOutcome) error
}

// outcomeOf classifies a finished call for the durable history.
//
// The three inputs are already tracked for other reasons: decided means the
// call was resolved without running, executed means the run stage ran it, and
// errKind names how it failed. Reading the status off them keeps one answer to
// "what happened to this call" rather than a second flag to keep in step.
func outcomeOf(c *pendingCall) ToolOutcome {
	out := ToolOutcome{
		ID:     c.call.ID,
		Name:   c.call.Name,
		Result: c.result,
		Parts:  c.parts,
		Status: ToolStatusCompleted,
	}
	switch {
	case !c.executed:
		// Never reached its tool: refused by policy, unknown, loop-guarded, or
		// rejected at parse. Whatever the reason, nothing outside BuildMax ran.
		out.Status = ToolStatusDenied
	case c.errKind != "":
		out.Status = ToolStatusFailed
	}
	return out
}

// recordToolBoundary durably records the calls in group that are about to run,
// before any of them does. Calls already decided are skipped: they never reach
// a tool, so there is no boundary for them to cross.
func recordToolBoundary(opts RunLoopOpts, group []pendingCall) error {
	h, ok := opts.History.(ToolBoundaryHistory)
	if !ok {
		return nil
	}
	var starts []ToolCallStart
	for i := range group {
		if group[i].decided {
			continue
		}
		starts = append(starts, ToolCallStart{ID: group[i].call.ID, Name: group[i].call.Name})
	}
	if len(starts) == 0 {
		return nil
	}
	return h.ToolExecutionStarted(starts)
}

// appendToolOutcome records one finished call, through the durable path when
// the history offers one and as a plain tool-role message otherwise.
func appendToolOutcome(opts RunLoopOpts, c *pendingCall) error {
	if h, ok := opts.History.(ToolBoundaryHistory); ok {
		return h.AppendToolResult(outcomeOf(c))
	}
	return opts.History.Append(llm.Message{
		Role:       "tool",
		Content:    c.result,
		ToolCallID: c.call.ID,
		Parts:      c.parts,
	})
}
