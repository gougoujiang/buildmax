package session

import (
	"sort"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// UsageGroup is the dimension a usage report groups sessions by. The three are
// the questions the report answers — which day, which workspace, which model
// the week's spend went to — and nothing derives one from another, so they are
// distinct choices rather than a single configurable key.
type UsageGroup string

const (
	GroupByDay       UsageGroup = "day"
	GroupByWorkspace UsageGroup = "workspace"
	GroupByModel     UsageGroup = "model"
)

// ParseUsageGroup validates a group name. An empty string is GroupByDay, the
// temporal view "what did this week cost" asks for by default.
func ParseUsageGroup(s string) (UsageGroup, bool) {
	switch UsageGroup(s) {
	case "", GroupByDay:
		return GroupByDay, true
	case GroupByWorkspace:
		return GroupByWorkspace, true
	case GroupByModel:
		return GroupByModel, true
	default:
		return "", false
	}
}

// UsageBucket is one group's summed usage, and the Total field of a report is
// the same shape over every session. Cost is nil when nothing in the bucket was
// priced; CostIncomplete says the money understates the bucket because some of
// it was unpriced or a currency the sum could not fold in.
type UsageBucket struct {
	Key              string    `json:"key"`
	Sessions         int       `json:"sessions"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	CacheReadTokens  int       `json:"cache_read_tokens"`
	CacheWriteTokens int       `json:"cache_write_tokens"`
	Cost             *llm.Cost `json:"cost,omitempty"`
	CostIncomplete   bool      `json:"cost_incomplete,omitempty"`
}

// UsageReport is a cross-session usage aggregation: one bucket per group, plus
// the total over every session counted. It is folded from the index projection
// alone, so it costs one file read no matter how many sessions it covers.
type UsageReport struct {
	GroupBy UsageGroup    `json:"group_by"`
	Since   *time.Time    `json:"since,omitempty"`
	Buckets []UsageBucket `json:"buckets"`
	Total   UsageBucket   `json:"total"`
}

// AggregateUsage folds session rows into a usage report grouped by groupBy.
//
// A row is counted when it is a user session (subagent rows never reach the
// index, but a hidden one is skipped defensively) and, when since is set,
// started at or after it — a session belongs to its start day, so the same
// timestamp scopes the window and keys the day grouping, and the two never
// disagree about which week a session falls in. loc is the zone the day key is
// computed in, so "today" means the reader's today rather than UTC's.
func AggregateUsage(rows []ItemSummary, groupBy UsageGroup, since *time.Time, loc *time.Location) UsageReport {
	if loc == nil {
		loc = time.Local
	}
	report := UsageReport{GroupBy: groupBy, Since: since}
	byKey := map[string]*UsageBucket{}
	for _, r := range rows {
		if r.Kind == KindSubagent {
			continue
		}
		if since != nil && r.CreatedAt.Before(*since) {
			continue
		}
		key := usageKey(r, groupBy, loc)
		b, ok := byKey[key]
		if !ok {
			b = &UsageBucket{Key: key}
			byKey[key] = b
		}
		addUsage(b, r)
		addUsage(&report.Total, r)
	}

	for _, b := range byKey {
		report.Buckets = append(report.Buckets, *b)
	}
	sortBuckets(report.Buckets, groupBy)
	report.Total.Key = "total"
	return report
}

func usageKey(r ItemSummary, groupBy UsageGroup, loc *time.Location) string {
	switch groupBy {
	case GroupByWorkspace:
		if r.Workspace == "" {
			return "(none)"
		}
		return r.Workspace
	case GroupByModel:
		if r.Model == "" {
			return "(unspecified)"
		}
		return r.Model
	default: // GroupByDay
		return r.CreatedAt.In(loc).Format("2006-01-02")
	}
}

// addUsage folds one row into a bucket. A row with no priced cost still counts
// its tokens and marks the bucket incomplete, so the money is never a total
// that silently dropped the sessions no model priced.
func addUsage(b *UsageBucket, r ItemSummary) {
	b.Sessions++
	b.PromptTokens += r.PromptTokens
	b.CompletionTokens += r.CompletionTokens
	b.CacheReadTokens += r.CacheReadTokens
	b.CacheWriteTokens += r.CacheWriteTokens
	if r.CostIncomplete {
		b.CostIncomplete = true
	}
	if r.Cost == nil {
		// A session that did any work but was never priced makes the bucket's
		// cost understate it; one that did nothing does not.
		if r.PromptTokens > 0 || r.CompletionTokens > 0 {
			b.CostIncomplete = true
		}
		return
	}
	if b.Cost == nil {
		total := *r.Cost
		b.Cost = &total
		return
	}
	if summed, ok := b.Cost.Add(*r.Cost); ok {
		*b.Cost = summed
	} else {
		// A currency the running sum cannot fold in is dropped from the money
		// rather than converted at an invented rate; the flag says so.
		b.CostIncomplete = true
	}
}

// sortBuckets orders a report for reading: days ascending so a week reads left
// to right, and workspace and model heaviest first so the line that cost the
// most is the one a reader sees.
func sortBuckets(buckets []UsageBucket, groupBy UsageGroup) {
	if groupBy == GroupByDay {
		sort.Slice(buckets, func(i, j int) bool { return buckets[i].Key < buckets[j].Key })
		return
	}
	sort.Slice(buckets, func(i, j int) bool {
		a, b := buckets[i], buckets[j]
		ac, bc := costTotal(a.Cost), costTotal(b.Cost)
		if ac != bc {
			return ac > bc
		}
		at := a.PromptTokens + a.CompletionTokens
		bt := b.PromptTokens + b.CompletionTokens
		if at != bt {
			return at > bt
		}
		return a.Key < b.Key
	})
}

func costTotal(c *llm.Cost) int64 {
	if c == nil {
		return 0
	}
	return c.Total
}
