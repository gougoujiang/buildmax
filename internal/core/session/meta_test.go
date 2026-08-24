package session

import (
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

func TestNewMetaHidesSubagentsByDefault(t *testing.T) {
	user := NewMeta("s1", KindUser, testTime)
	if user.Hidden {
		t.Error("a user session must not start hidden")
	}
	sub := NewMeta("s2", KindSubagent, testTime)
	if !sub.Hidden {
		t.Error("a subagent session must start hidden")
	}
	if user.CreatedAt != user.UpdatedAt {
		t.Error("created and updated must start equal")
	}
}

func TestMetaValidate(t *testing.T) {
	if err := NewMeta("s1", KindUser, testTime).Validate(); err != nil {
		t.Fatalf("valid meta rejected: %v", err)
	}
	if err := (Meta{Kind: KindUser}).Validate(); err == nil {
		t.Error("meta with no id accepted")
	}
	if err := (Meta{ID: "s1", Kind: "bogus"}).Validate(); err == nil {
		t.Error("meta with an unknown kind accepted")
	}
}

func TestApplyMetaUpdateChangesOnlyNamedFields(t *testing.T) {
	m := NewMeta("s1", KindUser, testTime)
	m.Title = "before"
	later := testTime.Add(time.Hour)

	title := "after"
	got := ApplyMetaUpdate(m, MetaUpdate{Title: &title}, later)

	if got.Title != "after" {
		t.Errorf("title = %q, want after", got.Title)
	}
	if got.Workspace != m.Workspace || got.SelectedModel != m.SelectedModel || got.Pinned != m.Pinned {
		t.Error("fields with a nil update pointer must not change")
	}
	if !got.UpdatedAt.Equal(later) {
		t.Errorf("updated_at = %v, want %v", got.UpdatedAt, later)
	}
	if m.Title != "before" {
		t.Error("ApplyMetaUpdate mutated its input")
	}
}

func TestApplyMetaUpdateAccumulatesUsage(t *testing.T) {
	m := NewMeta("s1", KindUser, testTime)
	m = ApplyMetaUpdate(m, MetaUpdate{AddPromptTokens: 100, AddCompletionTokens: 20}, testTime)
	m = ApplyMetaUpdate(m, MetaUpdate{AddPromptTokens: 50, AddCompletionTokens: 5}, testTime)
	if m.PromptTokens != 150 || m.CompletionTokens != 25 {
		t.Errorf("usage = %+v, want 150/25", m)
	}
}

func TestApplyMetaUpdateSumsCostAndFlagsMismatchedCurrency(t *testing.T) {
	m := NewMeta("s1", KindUser, testTime)
	m = ApplyMetaUpdate(m, MetaUpdate{AddCost: &llm.Cost{Currency: "USD", Total: 100}}, testTime)
	m = ApplyMetaUpdate(m, MetaUpdate{AddCost: &llm.Cost{Currency: "USD", Total: 50}}, testTime)
	if m.Cost == nil || m.Cost.Total != 150 {
		t.Fatalf("cost = %+v, want total 150", m.Cost)
	}
	if m.CostIncomplete {
		t.Fatal("matching-currency sum marked incomplete")
	}

	// A currency this build cannot convert must not silently vanish or produce
	// an invented total; the earlier total stands and is labelled incomplete.
	m = ApplyMetaUpdate(m, MetaUpdate{AddCost: &llm.Cost{Currency: "EUR", Total: 10}}, testTime)
	if m.Cost.Total != 150 {
		t.Errorf("total changed on currency mismatch: %+v", m.Cost)
	}
	if !m.CostIncomplete {
		t.Error("currency mismatch did not mark the total incomplete")
	}
}

func TestApplyMetaUpdatePreservesLineageAndForkedFrom(t *testing.T) {
	// MetaUpdate has no field for these, so this is really a compile-time
	// guarantee; the test documents the intent for a reader who might be
	// tempted to add one.
	m := NewMeta("s1", KindSubagent, testTime)
	m.ParentSessionID = "parent"
	m.AgentType = "explorer"
	got := ApplyMetaUpdate(m, MetaUpdate{}, testTime)
	if got.ParentSessionID != "parent" || got.AgentType != "explorer" || got.Kind != KindSubagent {
		t.Errorf("lineage changed: %+v", got)
	}
}
