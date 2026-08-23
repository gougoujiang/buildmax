package agent

import (
	"context"
	"sync"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// DelegatedStats is what runs delegated from one run spent.
//
// The token and cost fields break the RunStats fields beside them down rather
// than adding to them, the same way the cache counts break the prompt count
// down: a reader that sums both counts a delegation twice.
//
// ToolCalls is the exception and is additional. A delegation is one tool call
// of the parent, already counted there; the calls the child made are its own
// and are counted nowhere else.
type DelegatedStats struct {
	// Runs counts delegated runs, not the calls that started them.
	Runs             int
	PromptTokens     int
	CompletionTokens int
	CacheReadTokens  int
	CacheWriteTokens int
	ToolCalls        int
	Cost             *llm.Cost
	CostIncomplete   bool
}

// DelegatedUsage collects what delegated runs spent, so the run that started
// them reports the bill it is actually paying.
//
// It travels on the context because a delegation reaches the loop as an
// ordinary tool call, and llm.Tool.Execute returns a string. Widening that
// interface so one tool could report spend would put accounting into every
// tool in the registry. The lock is not decoration: tool calls within one
// iteration can execute in parallel, so two subagents can report at once.
type DelegatedUsage struct {
	mu  sync.Mutex
	acc DelegatedStats
}

type delegatedUsageKey struct{}

// CtxWithDelegatedUsage returns a context carrying u as the accumulator that
// delegated runs report to. RunLoop installs its own, so a subagent's own
// delegations accrue to the subagent and reach the parent only through the
// subagent's totals.
func CtxWithDelegatedUsage(ctx context.Context, u *DelegatedUsage) context.Context {
	return context.WithValue(ctx, delegatedUsageKey{}, u)
}

// DelegatedUsageFromCtx returns the accumulator for the run that owns ctx, or
// nil when nothing is collecting. Nil is a normal state — a subagent runner
// invoked outside a run has nobody to report to — and every method tolerates it.
func DelegatedUsageFromCtx(ctx context.Context) *DelegatedUsage {
	u, _ := ctx.Value(delegatedUsageKey{}).(*DelegatedUsage)
	return u
}

// Report folds one finished delegated run into the accumulator. stats are the
// child's own totals, which already include whatever it delegated in turn, so
// a chain of delegations arrives here once rather than at every level.
func (d *DelegatedUsage) Report(stats RunStats) {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.acc.Runs++
	d.acc.PromptTokens += stats.PromptTokens
	d.acc.CompletionTokens += stats.CompletionTokens
	d.acc.CacheReadTokens += stats.CacheReadTokens
	d.acc.CacheWriteTokens += stats.CacheWriteTokens
	d.acc.ToolCalls += stats.ToolCalls
	if stats.Delegated != nil {
		// The child counted its own delegations' tool calls the same way, and
		// that number is additional at every level.
		d.acc.ToolCalls += stats.Delegated.ToolCalls
	}
	mergeCost(&d.acc.Cost, &d.acc.CostIncomplete, stats.Cost, stats.CostIncomplete)
}

// Drain returns what has accumulated since the last call and resets. The
// caller folds it into a running total, so returning the delta rather than the
// whole keeps repeated folding from double counting.
func (d *DelegatedUsage) Drain() DelegatedStats {
	if d == nil {
		return DelegatedStats{}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	out := d.acc
	d.acc = DelegatedStats{}
	return out
}

// absorb folds drained delegated spend into the run's totals and its
// breakdown. The totals become inclusive: what the run cost is what it cost,
// whoever executed the calls.
func (s *RunStats) absorb(d DelegatedStats) {
	if d.Runs == 0 {
		return
	}
	s.PromptTokens += d.PromptTokens
	s.CompletionTokens += d.CompletionTokens
	s.CacheReadTokens += d.CacheReadTokens
	s.CacheWriteTokens += d.CacheWriteTokens
	mergeCost(&s.Cost, &s.CostIncomplete, d.Cost, d.CostIncomplete)

	if s.Delegated == nil {
		s.Delegated = &DelegatedStats{}
	}
	s.Delegated.Runs += d.Runs
	s.Delegated.PromptTokens += d.PromptTokens
	s.Delegated.CompletionTokens += d.CompletionTokens
	s.Delegated.CacheReadTokens += d.CacheReadTokens
	s.Delegated.CacheWriteTokens += d.CacheWriteTokens
	s.Delegated.ToolCalls += d.ToolCalls
	mergeCost(&s.Delegated.Cost, &s.Delegated.CostIncomplete, d.Cost, d.CostIncomplete)
}

// mergeCost adds add into dst, marking the total incomplete when the two
// cannot be reconciled. Two currencies in one total have no exchange rate
// here, and inventing one would produce a figure that is wrong in both.
func mergeCost(dst **llm.Cost, incomplete *bool, add *llm.Cost, addIncomplete bool) {
	if addIncomplete {
		*incomplete = true
	}
	if add == nil {
		return
	}
	if *dst == nil {
		total := *add
		*dst = &total
		return
	}
	summed, ok := (*dst).Add(*add)
	if !ok {
		*incomplete = true
		return
	}
	*dst = &summed
}
