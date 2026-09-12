package session

import (
	"testing"
	"time"

	"github.com/icloudbb/buildmax/internal/core/llm"
)

func day(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, time.UTC)
	if err != nil {
		panic(err)
	}
	return t
}

func usd(total int64) *llm.Cost { return &llm.Cost{Currency: "USD", Total: total} }

func TestParseUsageGroup(t *testing.T) {
	for in, want := range map[string]UsageGroup{
		"":          GroupByDay,
		"day":       GroupByDay,
		"workspace": GroupByWorkspace,
		"model":     GroupByModel,
	} {
		got, ok := ParseUsageGroup(in)
		if !ok || got != want {
			t.Errorf("ParseUsageGroup(%q) = %q,%v want %q,true", in, got, ok, want)
		}
	}
	if _, ok := ParseUsageGroup("hour"); ok {
		t.Error("ParseUsageGroup accepted an unknown group")
	}
}

func TestAggregateUsageGroupsByModelHeaviestFirst(t *testing.T) {
	rows := []ItemSummary{
		{ID: "a", Kind: KindUser, CreatedAt: day("2026-09-01 10:00"), Model: "opus", PromptTokens: 100, CompletionTokens: 10, Cost: usd(500)},
		{ID: "b", Kind: KindUser, CreatedAt: day("2026-09-01 11:00"), Model: "haiku", PromptTokens: 40, CompletionTokens: 4, Cost: usd(20)},
		{ID: "c", Kind: KindUser, CreatedAt: day("2026-09-02 09:00"), Model: "opus", PromptTokens: 60, CompletionTokens: 6, Cost: usd(300)},
	}
	r := AggregateUsage(rows, GroupByModel, nil, time.UTC)
	if len(r.Buckets) != 2 {
		t.Fatalf("buckets = %d, want 2", len(r.Buckets))
	}
	// opus is heaviest by cost, so it sorts first.
	if r.Buckets[0].Key != "opus" || r.Buckets[0].Sessions != 2 || r.Buckets[0].PromptTokens != 160 {
		t.Errorf("opus bucket = %+v", r.Buckets[0])
	}
	if r.Buckets[0].Cost == nil || r.Buckets[0].Cost.Total != 800 {
		t.Errorf("opus cost = %+v", r.Buckets[0].Cost)
	}
	if r.Buckets[1].Key != "haiku" {
		t.Errorf("second bucket = %q, want haiku", r.Buckets[1].Key)
	}
	if r.Total.Sessions != 3 || r.Total.PromptTokens != 200 || r.Total.Cost.Total != 820 {
		t.Errorf("total = %+v", r.Total)
	}
}

func TestAggregateUsageGroupsByDayChronologically(t *testing.T) {
	rows := []ItemSummary{
		{ID: "b", Kind: KindUser, CreatedAt: day("2026-09-03 09:00"), PromptTokens: 1},
		{ID: "a", Kind: KindUser, CreatedAt: day("2026-09-01 23:00"), PromptTokens: 1},
		{ID: "c", Kind: KindUser, CreatedAt: day("2026-09-02 00:00"), PromptTokens: 1},
	}
	r := AggregateUsage(rows, GroupByDay, nil, time.UTC)
	got := []string{r.Buckets[0].Key, r.Buckets[1].Key, r.Buckets[2].Key}
	want := []string{"2026-09-01", "2026-09-02", "2026-09-03"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("day order = %v, want %v", got, want)
			break
		}
	}
}

func TestAggregateUsageDayKeyUsesLocation(t *testing.T) {
	// 2026-09-01 23:00 UTC is 2026-09-02 in a +02:00 zone; the day key must
	// follow the reader's zone rather than UTC.
	loc := time.FixedZone("east", 2*3600)
	rows := []ItemSummary{{ID: "a", Kind: KindUser, CreatedAt: day("2026-09-01 23:00"), PromptTokens: 1}}
	r := AggregateUsage(rows, GroupByDay, nil, loc)
	if r.Buckets[0].Key != "2026-09-02" {
		t.Errorf("day key = %q, want 2026-09-02 in +02:00", r.Buckets[0].Key)
	}
}

func TestAggregateUsageSinceExcludesEarlier(t *testing.T) {
	since := day("2026-09-02 00:00")
	rows := []ItemSummary{
		{ID: "old", Kind: KindUser, CreatedAt: day("2026-09-01 12:00"), PromptTokens: 100},
		{ID: "new", Kind: KindUser, CreatedAt: day("2026-09-02 12:00"), PromptTokens: 5},
	}
	r := AggregateUsage(rows, GroupByDay, &since, time.UTC)
	if r.Total.Sessions != 1 || r.Total.PromptTokens != 5 {
		t.Errorf("since filter kept the wrong rows: total = %+v", r.Total)
	}
}

func TestAggregateUsageSkipsSubagents(t *testing.T) {
	rows := []ItemSummary{
		{ID: "u", Kind: KindUser, CreatedAt: day("2026-09-01 10:00"), PromptTokens: 10},
		{ID: "s", Kind: KindSubagent, CreatedAt: day("2026-09-01 10:00"), PromptTokens: 999},
	}
	r := AggregateUsage(rows, GroupByDay, nil, time.UTC)
	if r.Total.Sessions != 1 || r.Total.PromptTokens != 10 {
		t.Errorf("subagent row was counted: total = %+v", r.Total)
	}
}

func TestAggregateUsageMarksUnpricedWorkIncomplete(t *testing.T) {
	rows := []ItemSummary{
		{ID: "priced", Kind: KindUser, CreatedAt: day("2026-09-01 10:00"), PromptTokens: 10, Cost: usd(100)},
		{ID: "unpriced", Kind: KindUser, CreatedAt: day("2026-09-01 11:00"), PromptTokens: 20}, // did work, no cost
	}
	r := AggregateUsage(rows, GroupByDay, nil, time.UTC)
	b := r.Buckets[0]
	if b.Cost == nil || b.Cost.Total != 100 {
		t.Errorf("cost = %+v, want the priced session's 100", b.Cost)
	}
	if !b.CostIncomplete {
		t.Error("a bucket with unpriced work was not marked incomplete")
	}
	if !r.Total.CostIncomplete {
		t.Error("total was not marked incomplete")
	}
}

func TestAggregateUsageIdleUnpricedSessionIsNotIncomplete(t *testing.T) {
	// A session that did no work and has no cost is not a hole in the total.
	rows := []ItemSummary{
		{ID: "priced", Kind: KindUser, CreatedAt: day("2026-09-01 10:00"), PromptTokens: 10, Cost: usd(100)},
		{ID: "idle", Kind: KindUser, CreatedAt: day("2026-09-01 11:00")},
	}
	r := AggregateUsage(rows, GroupByDay, nil, time.UTC)
	if r.Total.CostIncomplete {
		t.Error("an idle unpriced session should not mark the total incomplete")
	}
}

func TestAggregateUsageMixedCurrencyIsIncomplete(t *testing.T) {
	rows := []ItemSummary{
		{ID: "a", Kind: KindUser, CreatedAt: day("2026-09-01 10:00"), PromptTokens: 10, Cost: usd(100)},
		{ID: "b", Kind: KindUser, CreatedAt: day("2026-09-01 11:00"), PromptTokens: 10, Cost: &llm.Cost{Currency: "EUR", Total: 50}},
	}
	r := AggregateUsage(rows, GroupByDay, nil, time.UTC)
	b := r.Buckets[0]
	if b.Cost.Total != 100 {
		t.Errorf("cost = %d, want the first currency's 100 with the other dropped", b.Cost.Total)
	}
	if !b.CostIncomplete {
		t.Error("a mixed-currency bucket was not marked incomplete")
	}
}

func TestAggregateUsageWorkspaceGroupingAndEmptyKey(t *testing.T) {
	rows := []ItemSummary{
		{ID: "a", Kind: KindUser, CreatedAt: day("2026-09-01 10:00"), Workspace: "/home/x/proj", PromptTokens: 10, Cost: usd(10)},
		{ID: "b", Kind: KindUser, CreatedAt: day("2026-09-01 11:00"), PromptTokens: 5, Cost: usd(5)},
	}
	r := AggregateUsage(rows, GroupByWorkspace, nil, time.UTC)
	keys := map[string]bool{}
	for _, b := range r.Buckets {
		keys[b.Key] = true
	}
	if !keys["/home/x/proj"] || !keys["(none)"] {
		t.Errorf("workspace keys = %v, want the path and (none)", keys)
	}
}
