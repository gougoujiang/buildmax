package runner

import (
	"math"
	"testing"
)

func TestWilsonKeepsAnIntervalOnPerfectRuns(t *testing.T) {
	low, high := Wilson(10, 10, Z95)
	if low <= 0 || low >= 1 {
		t.Errorf("low = %v; ten out of ten must not read as certainty", low)
	}
	if high < 0.99 {
		t.Errorf("high = %v, want close to 1", high)
	}

	// The normal approximation would give zero width here, which is the reason
	// Wilson was chosen. Guard the property rather than the constant.
	if high-low < 0.05 {
		t.Errorf("interval [%v, %v] is too tight for ten trials", low, high)
	}
}

func TestWilsonNarrowsWithEvidence(t *testing.T) {
	smallLow, smallHigh := Wilson(4, 5, Z95)
	largeLow, largeHigh := Wilson(400, 500, Z95)
	if (largeHigh - largeLow) >= (smallHigh - smallLow) {
		t.Errorf("500 trials gave a wider interval (%v) than 5 (%v)",
			largeHigh-largeLow, smallHigh-smallLow)
	}
}

func TestWilsonEdges(t *testing.T) {
	if low, high := Wilson(0, 0, Z95); low != 0 || high != 1 {
		t.Errorf("no trials gave [%v, %v], want the full range", low, high)
	}
	low, high := Wilson(0, 8, Z95)
	if low != 0 {
		t.Errorf("zero passes gave low = %v, want 0", low)
	}
	if high <= 0 || high > 1 {
		t.Errorf("zero passes gave high = %v, want a positive bound below 1", high)
	}
}

func TestPassRateExcludesUnscoredAttempts(t *testing.T) {
	// Task "b" ran once and the harness lost the other attempt, so it is
	// absent rather than false.
	outcomes := TaskOutcomes{
		"a": {true, true},
		"b": {false},
		"c": {},
	}
	passed, scored, rate := outcomes.PassRate()
	if passed != 2 || scored != 3 {
		t.Errorf("passed/scored = %d/%d, want 2/3", passed, scored)
	}
	if math.Abs(rate-2.0/3.0) > 1e-9 {
		t.Errorf("rate = %v, want 2/3", rate)
	}
}

func TestConsistencyRateIgnoresUnmeasuredTasks(t *testing.T) {
	outcomes := TaskOutcomes{
		"always":    {true, true, true},
		"sometimes": {true, false, true},
		"never-ran": {},
	}
	// One of the two measured tasks was consistent. The unmeasured one must not
	// count as inconsistent: an outage is not flakiness.
	if got := outcomes.ConsistencyRate(); math.Abs(got-0.5) > 1e-9 {
		t.Errorf("consistency = %v, want 0.5", got)
	}
	if got := (TaskOutcomes{}).ConsistencyRate(); got != 0 {
		t.Errorf("empty consistency = %v, want 0", got)
	}
}

func TestComparePairedFindsDirectionAndPairsByTask(t *testing.T) {
	baseline := TaskOutcomes{
		"improves":  {false, false},
		"regresses": {true, true},
		"steady":    {true, true},
		"only-base": {true},
	}
	candidate := TaskOutcomes{
		"improves":  {true, true},
		"regresses": {false, false},
		"steady":    {true, true},
		"only-cand": {true},
	}

	cmp := ComparePaired(baseline, candidate, 1, 500)
	if cmp.Paired != 3 {
		t.Errorf("paired = %d, want 3", cmp.Paired)
	}
	if len(cmp.Improved) != 1 || cmp.Improved[0] != "improves" {
		t.Errorf("improved = %v, want [improves]", cmp.Improved)
	}
	if len(cmp.Regressed) != 1 || cmp.Regressed[0] != "regresses" {
		t.Errorf("regressed = %v, want [regresses]", cmp.Regressed)
	}
	// A task only one side ran is unscorable, not agreement.
	if len(cmp.Unscorable) != 2 {
		t.Errorf("unscorable = %v, want both one-sided tasks", cmp.Unscorable)
	}
	// +1 and -1 and 0 average to zero.
	if math.Abs(cmp.Delta) > 1e-9 {
		t.Errorf("delta = %v, want 0", cmp.Delta)
	}
}

func TestComparePairedIntervalCoversZeroWhenNothingChanged(t *testing.T) {
	same := TaskOutcomes{}
	for _, id := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		same[id] = []bool{true, false}
	}
	cmp := ComparePaired(same, same, 7, 2000)
	if cmp.Delta != 0 {
		t.Errorf("delta = %v, want 0", cmp.Delta)
	}
	if cmp.Low > 0 || cmp.High < 0 {
		t.Errorf("interval [%v, %v] excludes zero for identical arms", cmp.Low, cmp.High)
	}
}

func TestComparePairedIntervalExcludesZeroOnAConsistentGain(t *testing.T) {
	baseline, candidate := TaskOutcomes{}, TaskOutcomes{}
	for _, id := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"} {
		baseline[id] = []bool{false, false}
		candidate[id] = []bool{true, true}
	}
	cmp := ComparePaired(baseline, candidate, 7, 2000)
	if cmp.Delta != 1 {
		t.Errorf("delta = %v, want 1", cmp.Delta)
	}
	if cmp.Low <= 0 {
		t.Errorf("interval [%v, %v] includes zero despite every task improving", cmp.Low, cmp.High)
	}
}

func TestComparePairedIsReproducible(t *testing.T) {
	baseline, candidate := TaskOutcomes{}, TaskOutcomes{}
	for i, id := range []string{"a", "b", "c", "d", "e", "f"} {
		baseline[id] = []bool{i%2 == 0, true}
		candidate[id] = []bool{true, i%3 == 0}
	}
	first := ComparePaired(baseline, candidate, 42, 1000)
	second := ComparePaired(baseline, candidate, 42, 1000)
	if first.Low != second.Low || first.High != second.High {
		t.Errorf("the same seed gave [%v,%v] then [%v,%v]; a report must be reproducible",
			first.Low, first.High, second.Low, second.High)
	}
}

func TestComparePairedOnOneTaskReportsNoSpread(t *testing.T) {
	cmp := ComparePaired(
		TaskOutcomes{"only": {false}},
		TaskOutcomes{"only": {true}},
		1, 1000)
	// One pair cannot support an interval. Saying so beats inventing a spread.
	if cmp.Low != cmp.Delta || cmp.High != cmp.Delta {
		t.Errorf("one task gave [%v, %v] around %v, want no spread", cmp.Low, cmp.High, cmp.Delta)
	}
}

func TestComparePairedWithNothingInCommon(t *testing.T) {
	cmp := ComparePaired(
		TaskOutcomes{"a": {true}},
		TaskOutcomes{"b": {true}},
		1, 100)
	if cmp.Paired != 0 || cmp.Delta != 0 {
		t.Errorf("disjoint arms gave paired=%d delta=%v, want 0 and 0", cmp.Paired, cmp.Delta)
	}
	if len(cmp.Unscorable) != 2 {
		t.Errorf("unscorable = %v, want both tasks", cmp.Unscorable)
	}
}

func TestPercentileOf(t *testing.T) {
	values := []int64{500, 100, 300, 200, 400}
	if got := percentileOf(values, 0.5); got != 300 {
		t.Errorf("median = %d, want 300", got)
	}
	if got := percentileOf(values, 0.95); got != 500 {
		t.Errorf("p95 = %d, want 500", got)
	}
	if got := percentileOf(nil, 0.5); got != 0 {
		t.Errorf("empty median = %d, want 0", got)
	}
}
