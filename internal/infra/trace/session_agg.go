package trace

import (
	"bufio"
	"encoding/json"
	"io"
	"sort"
	"time"
)

// sessionAgg folds run traces into a SessionSummary.
//
// The counts split by where they are truthful. Tokens and cost are taken from
// each top-level run_end, because a run's totals already include what it
// delegated — summing the subagent files too would bill every delegation
// twice. Everything else is counted from the records themselves, across
// parent and subagent files alike, because "which tools ran and for how long"
// is a question about the whole session, not about who executed it.
type sessionAgg struct {
	out   *SessionSummary
	tools map[string]*SessionToolStats
	// Models a top-level run used are listed before ones only a subagent
	// used. Both are first-seen order within their group; leading with a
	// subagent's model would name the session after work it delegated.
	models    map[string]bool
	topModels []string
	subModels []string
	firstTS   time.Time
	lastTS    time.Time
	delegate  RecordDelegated
	hasDeleg  bool
}

func newSessionAgg(out *SessionSummary) *sessionAgg {
	return &sessionAgg{
		out:    out,
		tools:  make(map[string]*SessionToolStats),
		models: make(map[string]bool),
	}
}

func (a *sessionAgg) tool(name string) *SessionToolStats {
	t, ok := a.tools[name]
	if !ok {
		t = &SessionToolStats{Name: name}
		a.tools[name] = t
	}
	return t
}

// foldRun reads one trace file. Unparseable lines are skipped for the same
// reason Summarize skips them: a trace can be cut short mid-line by a killed
// process, and a partial answer about a run that died beats no answer.
func (a *sessionAgg) foldRun(r io.Reader) {
	var (
		isSub    bool
		complete bool
		start    time.Time
		end      time.Time
	)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec Record
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}
		ts := parseTS(rec.TS)
		a.observeTS(ts)

		switch rec.Type {
		case "run_start":
			isSub = rec.IsSubagent
			start = ts
			if rec.Model != "" && !a.models[rec.Model] {
				a.models[rec.Model] = true
				if isSub {
					a.subModels = append(a.subModels, rec.Model)
				} else {
					a.topModels = append(a.topModels, rec.Model)
				}
			}
		case "llm_start":
			a.out.LLMCalls++
			if rec.ContextTokens > a.out.PeakContextTokens {
				a.out.PeakContextTokens = rec.ContextTokens
			}
			if rec.ContextWindow > a.out.ContextWindow {
				a.out.ContextWindow = rec.ContextWindow
			}
		case "tool_end":
			a.out.ToolCalls++
			d := time.Duration(rec.DurationMS) * time.Millisecond
			a.out.ToolWall += d
			t := a.tool(rec.Tool)
			t.Calls++
			t.Wall += d
			if d > t.MaxWall {
				t.MaxWall = d
			}
			if rec.ErrorKind != "" {
				a.out.ToolFailures++
				if t.Failures == nil {
					t.Failures = make(map[string]int)
				}
				t.Failures[rec.ErrorKind]++
			}
		case "tool_denied":
			a.out.ToolDenials++
			a.tool(rec.Tool).Denials++
		case "context_compacted":
			a.out.Compactions++
		case "run_end":
			complete = true
			end = ts
			if rec.Error != "" {
				a.out.Failed++
			}
			if isSub {
				break
			}
			a.out.PromptTokens += rec.PromptTokens
			a.out.CompletionTokens += rec.CompletionTokens
			a.out.CacheReadTokens += rec.CacheReadTokens
			a.out.CacheWriteTokens += rec.CacheWriteTokens
			a.addCost(rec.Cost, rec.CostIncomplete)
			a.addDelegated(rec.Delegated)
		}
	}

	if isSub {
		a.out.Subagents++
	} else {
		a.out.Runs++
	}
	if !complete {
		// A run without run_end recorded no totals, so it is missing from the
		// tokens and the money above. Saying so is the difference between a
		// short session and a session someone killed.
		a.out.Incomplete++
		return
	}
	if !isSub && !start.IsZero() && !end.IsZero() && end.After(start) {
		// Top-level only: a subagent runs inside its parent's elapsed time,
		// and adding it would report a session as taking longer than it did.
		a.out.Wall += end.Sub(start)
	}
}

func (a *sessionAgg) observeTS(ts time.Time) {
	if ts.IsZero() {
		return
	}
	if a.firstTS.IsZero() || ts.Before(a.firstTS) {
		a.firstTS = ts
	}
	if ts.After(a.lastTS) {
		a.lastTS = ts
	}
}

func (a *sessionAgg) addCost(c *RecordCost, incomplete bool) {
	if incomplete {
		a.out.CostIncomplete = true
	}
	if c == nil {
		return
	}
	if a.out.Cost == nil {
		total := *c
		a.out.Cost = &total
		return
	}
	if !addRecordCost(a.out.Cost, *c) {
		// Two currencies and no exchange rate here. The earlier total stands
		// and says it is partial.
		a.out.CostIncomplete = true
	}
}

func (a *sessionAgg) addDelegated(d *RecordDelegated) {
	if d == nil {
		return
	}
	a.hasDeleg = true
	a.delegate.Runs += d.Runs
	a.delegate.PromptTokens += d.PromptTokens
	a.delegate.CompletionTokens += d.CompletionTokens
	a.delegate.CacheReadTokens += d.CacheReadTokens
	a.delegate.CacheWriteTokens += d.CacheWriteTokens
	a.delegate.ToolCalls += d.ToolCalls
	if d.CostIncomplete {
		a.delegate.CostIncomplete = true
	}
	if d.Cost == nil {
		return
	}
	if a.delegate.Cost == nil {
		total := *d.Cost
		a.delegate.Cost = &total
		return
	}
	if !addRecordCost(a.delegate.Cost, *d.Cost) {
		a.delegate.CostIncomplete = true
	}
}

// finish materializes the derived views once every run has been folded.
func (a *sessionAgg) finish() {
	a.out.StartedAt = a.firstTS
	a.out.EndedAt = a.lastTS
	a.out.Models = append(a.topModels, a.subModels...)
	if a.hasDeleg {
		d := a.delegate
		a.out.Delegated = &d
	}
	a.out.Tools = make([]SessionToolStats, 0, len(a.tools))
	for _, t := range a.tools {
		a.out.Tools = append(a.out.Tools, *t)
	}
	sort.Slice(a.out.Tools, func(i, j int) bool {
		x, y := a.out.Tools[i], a.out.Tools[j]
		if x.Wall != y.Wall {
			return x.Wall > y.Wall
		}
		if x.Calls != y.Calls {
			return x.Calls > y.Calls
		}
		return x.Name < y.Name
	})
}

// addRecordCost adds b into a, reporting false when the two are quoted in
// currencies that cannot be added. A zero-currency operand is treated as
// nothing rather than as a mismatch, matching llm.Cost.Add.
func addRecordCost(a *RecordCost, b RecordCost) bool {
	if b.Currency == "" {
		return true
	}
	if a.Currency == "" {
		*a = b
		return true
	}
	if a.Currency != b.Currency {
		return false
	}
	a.Uncached += b.Uncached
	a.CacheRead += b.CacheRead
	a.CacheWrite += b.CacheWrite
	a.Output += b.Output
	a.Total += b.Total
	a.Baseline += b.Baseline
	return true
}

// parseTS reads a record timestamp, returning the zero time when it is absent
// or malformed. A missing timestamp costs a duration, not the whole fold.
func parseTS(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
