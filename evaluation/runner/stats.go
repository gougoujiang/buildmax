package runner

import (
	"math"
	"math/rand/v2"
	"sort"
)

// Z95 is the standard normal quantile for a 95% two-sided interval.
const Z95 = 1.959963984540054

// Wilson returns the Wilson score interval for successes out of trials.
//
// It is used rather than the textbook normal approximation because evaluation
// runs are small. Ten passes out of ten give a normal interval of zero width,
// which reads as certainty from a sample that cannot support it; Wilson keeps
// the interval away from the boundary and stays defined when the count is zero
// or complete.
//
// Trials of zero returns [0,1]: no evidence is not evidence of nothing.
func Wilson(successes, trials int, z float64) (low, high float64) {
	if trials <= 0 {
		return 0, 1
	}
	n := float64(trials)
	p := float64(successes) / n
	z2 := z * z

	denominator := 1 + z2/n
	center := (p + z2/(2*n)) / denominator
	margin := z / denominator * math.Sqrt(p*(1-p)/n+z2/(4*n*n))

	low, high = clamp01(center-margin), clamp01(center+margin)

	// At the boundaries the interval is closed exactly, which the arithmetic
	// above reaches only to within rounding — zero passes computes a lower
	// bound near 1e-18. Reporting that instead of zero is not more precise, it
	// is a number a reader has to decide to ignore.
	if successes == 0 {
		low = 0
	}
	if successes == trials {
		high = 1
	}
	return low, high
}

// TaskOutcomes is one subject's per-task attempts: true for a scored pass,
// false for a scored failure. Trials the harness could not run are absent
// rather than false, because section 12 keeps them out of the rate.
type TaskOutcomes map[string][]bool

// PassRate is scored passes over scored attempts.
func (o TaskOutcomes) PassRate() (passed, scored int, rate float64) {
	for _, attempts := range o {
		for _, ok := range attempts {
			scored++
			if ok {
				passed++
			}
		}
	}
	if scored == 0 {
		return 0, 0, 0
	}
	return passed, scored, float64(passed) / float64(scored)
}

// ConsistencyRate is pass^k: the share of tasks that passed every attempt.
//
// A task with no scored attempt is excluded rather than counted as
// inconsistent. It was never measured, and calling that a consistency failure
// would let an infrastructure outage look like a flaky subject.
func (o TaskOutcomes) ConsistencyRate() float64 {
	measured, consistent := 0, 0
	for _, attempts := range o {
		if len(attempts) == 0 {
			continue
		}
		measured++
		all := true
		for _, ok := range attempts {
			if !ok {
				all = false
				break
			}
		}
		if all {
			consistent++
		}
	}
	if measured == 0 {
		return 0
	}
	return float64(consistent) / float64(measured)
}

// taskRate is one task's pass rate for one subject, and how many attempts it
// rests on.
func taskRate(attempts []bool) (float64, int) {
	if len(attempts) == 0 {
		return 0, 0
	}
	passed := 0
	for _, ok := range attempts {
		if ok {
			passed++
		}
	}
	return float64(passed) / float64(len(attempts)), len(attempts)
}

// Comparison is a candidate measured against a baseline over shared tasks.
type Comparison struct {
	// Delta is the candidate's mean per-task rate minus the baseline's, over
	// tasks both subjects actually ran.
	Delta float64
	// Low and High bound Delta. The interval is over the paired difference
	// rather than over each rate separately: two rates can both move while
	// their difference stays indistinguishable from noise, and a reader
	// comparing two separate intervals cannot see that.
	Low, High float64
	// Improved and Regressed name tasks whose rate moved in each direction.
	Improved, Regressed []string
	// Unscorable names tasks one side or the other never scored. Section 12
	// requires them shown: a task silently dropped from both arms reads as
	// agreement.
	Unscorable []string
	// Paired is how many tasks the delta rests on.
	Paired int
}

// ComparePaired computes the paired difference between two subjects and a
// percentile bootstrap interval over tasks.
//
// Resampling is over tasks rather than over trials because tasks are what vary
// independently: two attempts at the same task share its difficulty, and
// treating them as independent draws would report an interval far narrower than
// the evidence supports.
//
// The seed is a parameter so a report is reproducible. An interval that moves
// between two readings of the same data is not a measurement.
func ComparePaired(baseline, candidate TaskOutcomes, seed uint64, resamples int) Comparison {
	var (
		cmp    Comparison
		deltas []float64
	)

	tasks := make([]string, 0, len(baseline)+len(candidate))
	seen := map[string]bool{}
	for id := range baseline {
		tasks = append(tasks, id)
		seen[id] = true
	}
	for id := range candidate {
		if !seen[id] {
			tasks = append(tasks, id)
		}
	}
	sort.Strings(tasks)

	for _, id := range tasks {
		baseRate, baseN := taskRate(baseline[id])
		candRate, candN := taskRate(candidate[id])
		if baseN == 0 || candN == 0 {
			cmp.Unscorable = append(cmp.Unscorable, id)
			continue
		}
		d := candRate - baseRate
		deltas = append(deltas, d)
		switch {
		case d > 0:
			cmp.Improved = append(cmp.Improved, id)
		case d < 0:
			cmp.Regressed = append(cmp.Regressed, id)
		}
	}

	cmp.Paired = len(deltas)
	if cmp.Paired == 0 {
		return cmp
	}
	cmp.Delta = mean(deltas)
	cmp.Low, cmp.High = bootstrapInterval(deltas, seed, resamples)
	return cmp
}

// bootstrapInterval resamples the per-task differences with replacement and
// returns the 2.5th and 97.5th percentiles of the resampled means.
func bootstrapInterval(deltas []float64, seed uint64, resamples int) (low, high float64) {
	if len(deltas) == 0 {
		return 0, 0
	}
	if len(deltas) == 1 || resamples <= 0 {
		// One task supports no interval. Returning the point estimate as both
		// bounds says that plainly; inventing a spread would imply evidence
		// that a single pair does not carry.
		return deltas[0], deltas[0]
	}

	// A seeded PCG rather than the global source: this is a statistic, not a
	// secret, and a report has to be reproducible from its inputs.
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	means := make([]float64, resamples)
	sample := make([]float64, len(deltas))
	for i := range means {
		for j := range sample {
			sample[j] = deltas[rng.IntN(len(deltas))]
		}
		means[i] = mean(sample)
	}
	sort.Float64s(means)
	return percentile(means, 0.025), percentile(means, 0.975)
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(p * float64(len(sorted)))
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	total := 0.0
	for _, x := range xs {
		total += x
	}
	return total / float64(len(xs))
}

func clamp01(x float64) float64 {
	switch {
	case x < 0:
		return 0
	case x > 1:
		return 1
	default:
		return x
	}
}

// percentileOf returns the p-th percentile of unsorted durations in
// milliseconds, used for the latency figures section 12 asks for.
func percentileOf(values []int64, p float64) int64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	idx := int(p * float64(len(sorted)))
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
