package trace

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SessionSummary is every run a session recorded, folded into one answer.
//
// It is the time-shaped half of a session's statistics: the history knows what
// was said and how big it was, and only the traces know when, for how long,
// against which model, and how much of it a delegation did.
//
// It is best-effort by construction. Tracing is fail-open, a trace can be cut
// short by a killed process, and nothing prunes the directory today — so a
// reader is told what was and was not complete rather than handed a total that
// quietly covers less than it claims.
type SessionSummary struct {
	SessionID string `json:"session_id"`
	// Runs counts top-level runs; Subagents counts the delegated runs filed
	// beside them. A subagent's tokens are already inside its parent's totals,
	// so only top-level runs are summed into the figures below.
	Runs      int `json:"runs"`
	Subagents int `json:"subagents"`
	// Incomplete counts runs whose trace has no run_end: killed, crashed, or
	// still going. Their totals are missing from everything here.
	Incomplete int `json:"incomplete"`
	// Failed counts runs that ended with an error.
	Failed int `json:"failed"`

	StartedAt time.Time `json:"started_at,omitempty"`
	EndedAt   time.Time `json:"ended_at,omitempty"`
	// Wall is the summed elapsed time of top-level runs — the time a user
	// spent waiting, not the span from the first run to the last, which would
	// count the hours a session sat idle.
	Wall time.Duration `json:"wall_ns,omitempty"`
	// ToolWall is the summed duration of tool calls. The rest of Wall is
	// model latency and the loop's own work; the split is the point, because
	// a session dominated by tools and one dominated by the model want
	// different fixes.
	ToolWall time.Duration `json:"tool_wall_ns,omitempty"`

	LLMCalls     int `json:"llm_calls"`
	ToolCalls    int `json:"tool_calls"`
	ToolFailures int `json:"tool_failures"`
	ToolDenials  int `json:"tool_denials"`
	Compactions  int `json:"compactions"`

	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	CacheReadTokens  int `json:"cache_read_tokens"`
	CacheWriteTokens int `json:"cache_write_tokens"`

	// Cost is the summed estimate over top-level runs, nil when none of them
	// could be priced. CostIncomplete says a run did work that could not be
	// priced, so the total understates the session.
	Cost           *RecordCost `json:"cost,omitempty"`
	CostIncomplete bool        `json:"cost_incomplete,omitempty"`
	// Delegated is what the subagent runs cost. Like everywhere else, it
	// breaks the totals above down rather than adding to them.
	Delegated *RecordDelegated `json:"delegated,omitempty"`

	// PeakContextTokens is the largest prompt a call in this session carried,
	// against the window it had. Together they say how close the session came
	// to the ceiling that forces compaction.
	PeakContextTokens int `json:"peak_context_tokens,omitempty"`
	ContextWindow     int `json:"context_window,omitempty"`

	// Models names the models the session ran against, in first-seen order. A
	// session can switch models mid-way, and a single "model" field would then
	// name whichever run happened to be read last.
	Models []string `json:"models,omitempty"`

	// Tools is the per-tool breakdown, slowest total first.
	Tools []SessionToolStats `json:"tools,omitempty"`
}

// SessionToolStats is one tool's time and outcomes across a session.
type SessionToolStats struct {
	Name  string `json:"name"`
	Calls int    `json:"calls"`
	// Wall is the summed duration of its calls, and MaxWall the slowest single
	// one, so an outlier is not averaged away.
	Wall    time.Duration `json:"wall_ns,omitempty"`
	MaxWall time.Duration `json:"max_wall_ns,omitempty"`
	// Failures counts calls that could not complete, by kind. A tool that ran
	// and reported a bad outcome is not here.
	Failures map[string]int `json:"failures,omitempty"`
	Denials  int            `json:"denials,omitempty"`
}

// SummarizeSession folds every trace a session recorded. dir is the traces
// root; the session's runs live in the directory named after its id.
//
// A session with no traces is not an error: tracing may have failed open, or
// the run may predate it. The zero SessionSummary with Runs == 0 says so.
func SummarizeSession(sessionTraceDir string) (SessionSummary, error) {
	var out SessionSummary
	entries, err := os.ReadDir(sessionTraceDir)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return out, err
	}

	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
			files = append(files, e.Name())
		}
	}
	// By name, which is the run's public id: not chronological, but stable, so
	// two reads of one session agree on the order of everything derived here.
	sort.Strings(files)

	agg := newSessionAgg(&out)
	for _, name := range files {
		f, err := os.Open(filepath.Join(sessionTraceDir, name))
		if err != nil {
			// One unreadable run is not a reason to answer nothing about the
			// rest, but it must not pass as a run that recorded nothing.
			out.Incomplete++
			continue
		}
		agg.foldRun(f)
		_ = f.Close()
	}
	agg.finish()
	return out, nil
}
