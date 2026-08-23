package agentapp

import (
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/session"
)

func testPricing() llm.Pricing {
	return llm.Pricing{
		Currency:          "USD",
		InputPerMTok:      3_000_000_000,
		CacheReadPerMTok:  300_000_000,
		CacheWritePerMTok: 3_750_000_000,
		OutputPerMTok:     15_000_000_000,
	}
}

func newCostSession() *SessionContext {
	return NewSessionContext(session.NewSession(""), "test-model")
}

// The session's spend accumulates as it runs rather than being recomputed on
// read, because the model — and so the rates — can change between turns. A
// total derived later from whatever is configured then would restate turns that
// were already paid for at a different price.
func TestSessionCostAccumulatesAcrossTurns(t *testing.T) {
	sess := newCostSession()
	usage := llm.Usage{PromptTokens: 100_000, CompletionTokens: 1_000, CacheReadTokens: 90_000}

	addSessionCost(sess, usage, testPricing())
	addSessionCost(sess, usage, testPricing())

	if sess.Cost == nil {
		t.Fatal("two priced turns produced no cost")
	}
	if sess.CostIncomplete {
		t.Error("a fully priced session should not be marked incomplete")
	}
	one, _ := llm.EstimateCost(usage, testPricing())
	if sess.Cost.Total != one.Total*2 {
		t.Errorf("total = %d, want %d", sess.Cost.Total, one.Total*2)
	}
	if sess.Cost.Baseline != one.Baseline*2 {
		t.Errorf("baseline = %d, want %d", sess.Cost.Baseline, one.Baseline*2)
	}
}

// A turn that did work and could not be priced leaves a hole in the money. The
// total is labelled rather than quietly understating the session, because a
// number that absorbed an unpriced turn as free is a claim nobody made.
func TestAnUnpricedTurnMarksTheSessionIncomplete(t *testing.T) {
	sess := newCostSession()
	usage := llm.Usage{PromptTokens: 100_000, CompletionTokens: 1_000}

	addSessionCost(sess, usage, testPricing())
	addSessionCost(sess, usage, llm.Pricing{})

	if sess.Cost == nil {
		t.Fatal("the priced turn should still have produced a cost")
	}
	if !sess.CostIncomplete {
		t.Error("an unpriced turn should mark the total incomplete")
	}
	one, _ := llm.EstimateCost(usage, testPricing())
	if sess.Cost.Total != one.Total {
		t.Errorf("total = %d, want only the priced turn's %d", sess.Cost.Total, one.Total)
	}
}

// A turn the provider never reported usage for is already surfaced as
// unmeasured by the token counts. Counting it again as an unpriced turn would
// present one gap as two.
func TestATurnWithNoUsageDoesNotMarkTheSessionIncomplete(t *testing.T) {
	sess := newCostSession()
	addSessionCost(sess, llm.Usage{}, testPricing())

	if sess.Cost != nil {
		t.Errorf("an unmeasured turn produced a cost: %+v", sess.Cost)
	}
	if sess.CostIncomplete {
		t.Error("an unmeasured turn should not read as an unpriced one")
	}
}

// BuildMax holds no exchange rate. Converting would produce a total that is
// wrong in both currencies, so the earlier total stands and says it is partial.
func TestSwitchingCurrencyMidSessionMarksTheTotalIncomplete(t *testing.T) {
	sess := newCostSession()
	usage := llm.Usage{PromptTokens: 100_000, CompletionTokens: 1_000}

	addSessionCost(sess, usage, testPricing())
	before := *sess.Cost

	euros := testPricing()
	euros.Currency = "EUR"
	addSessionCost(sess, usage, euros)

	if sess.Cost.Currency != "USD" || sess.Cost.Total != before.Total {
		t.Errorf("the running total moved: %+v, want %+v", *sess.Cost, before)
	}
	if !sess.CostIncomplete {
		t.Error("a second currency should mark the total incomplete")
	}
}
