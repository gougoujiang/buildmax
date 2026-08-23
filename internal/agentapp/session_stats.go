package agentapp

import (
	"fmt"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/session"
	"github.com/gougoujiang/buildmax/internal/infra/trace"
)

// SessionStats is one session's statistics, assembled from the two records
// that hold them.
//
// The session file is authoritative for tokens and money: it accumulated them
// turn by turn at the rates in force for each, and no later read can restate
// that. The traces are authoritative for everything time-shaped, and for the
// per-run detail the session file never kept — durations, denials, which model
// ran, how much a delegation did.
//
// They are kept apart rather than merged into one flat number because they can
// legitimately disagree: a run that died before writing run_end is in the
// session's totals and missing from the trace fold, and a reader shown one
// blended figure would have no way to notice.
type SessionStats struct {
	ID        string    `json:"id"`
	Title     string    `json:"title,omitempty"`
	Workspace string    `json:"workspace,omitempty"`
	CreatedAt time.Time `json:"created_at"`

	// Usage and Cost are the session's own accumulated totals.
	Usage llm.Usage `json:"usage"`
	Cost  *llm.Cost `json:"cost,omitempty"`
	// CostIncomplete says part of the session could not be priced, so Cost
	// understates it rather than covering it.
	CostIncomplete bool `json:"cost_incomplete,omitempty"`

	// Conversation is the shape of the stored history.
	Conversation session.ConversationStats `json:"conversation"`
	// Runs is the fold over this session's traces. Runs.Runs == 0 means no
	// trace was found, which is not the same as a session that never ran.
	Runs trace.SessionSummary `json:"runs"`
}

// LoadSessionStats assembles one session's statistics. sessionsDir and
// tracesDir are the two roots; id names the session.
//
// A missing trace directory is not an error — tracing is fail-open and nothing
// prunes it today, so its absence is a normal state that the returned Runs
// reports rather than a failure to load the session.
func LoadSessionStats(sessionsDir, tracesDir, id string) (SessionStats, error) {
	sess, err := LoadSession(sessionsDir, id)
	if err != nil {
		return SessionStats{}, err
	}
	out := SessionStats{
		ID:             sess.ID,
		Title:          sess.Title,
		CreatedAt:      sess.CreatedAt,
		Usage:          sess.Usage(),
		Cost:           sess.Cost,
		CostIncomplete: sess.CostIncomplete,
		Conversation:   session.Stats(sess),
	}
	if items, err := LoadSessionList(sessionsDir); err == nil {
		for _, it := range items {
			if it.ID == id {
				out.Workspace = it.Workspace
				break
			}
		}
	}
	// A trace read that fails leaves the run fold empty rather than failing
	// the whole answer: the session half is already complete and useful.
	runs, err := trace.SummarizeSession(tracesDir, id)
	if err != nil {
		return out, fmt.Errorf("read traces for session %s: %w", id, err)
	}
	out.Runs = runs
	return out, nil
}

// ModelTime is the part of a session's wall clock that was not a tool call:
// model latency plus the loop's own work. ok is false when the traces did not
// measure enough to answer — no completed run, or tool time exceeding the wall
// clock, which parallel tool execution makes possible and which would turn
// into a negative answer.
func (s SessionStats) ModelTime() (time.Duration, bool) {
	if s.Runs.Wall <= 0 || s.Runs.ToolWall > s.Runs.Wall {
		return 0, false
	}
	return s.Runs.Wall - s.Runs.ToolWall, true
}

// ContextPeakShare is how close the session came to its context window, and
// ok=false when the traces recorded no window to compare against.
func (s SessionStats) ContextPeakShare() (float64, bool) {
	if s.Runs.ContextWindow <= 0 || s.Runs.PeakContextTokens <= 0 {
		return 0, false
	}
	return float64(s.Runs.PeakContextTokens) / float64(s.Runs.ContextWindow), true
}

// CacheSaved is what prompt caching is estimated to have saved this session,
// and ok=false when nothing here was priced or caching cost more than it
// saved. Reporting a saving on a session that only ever wrote cache entries
// would be the false claim the whole cost path avoids.
func (s SessionStats) CacheSaved() (int64, bool) {
	if s.Cost == nil {
		return 0, false
	}
	saved := s.Cost.Saved()
	return saved, saved > 0
}
