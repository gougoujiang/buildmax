package runner

import "github.com/gougoujiang/buildmax/evaluation/contract"

// Summarize derives one subject's outcome vector from the trials it produced.
//
// It takes bundles rather than being folded into the execution loop because a
// bundle is the contract's unit of evidence, and evidence that arrives some
// other way deserves the same arithmetic. An external benchmark imported from
// Harbor has no execution loop here at all, and a second summarizer written for
// it would be a second definition of what a pass rate counts — which harness
// faults are in the denominator, whether a timeout is scored, how consistency
// is measured. Those are section 12's decisions, and they get exactly one
// implementation.
//
// Bundle order is the attempt order: a task's attempts are recorded as they
// appear, so a caller pairing on index has to hand them over in index order.
func Summarize(subject contract.SubjectManifest, bundles []contract.TrialBundle) Result {
	result := Result{Subject: subject, Bundles: bundles, Outcomes: TaskOutcomes{}}
	metrics := contract.SuiteMetrics{SubjectID: subject.ID, Faults: map[contract.TrialStatus]int{}}
	durations := make([]int64, 0, len(bundles))

	for _, bundle := range bundles {
		if metrics.Suite == "" {
			metrics.Suite = bundle.Suite
		}
		durations = append(durations, int64(bundle.Duration))

		metrics.Trials++
		if bundle.Status.Scored() {
			metrics.Scored++
			passed := bundle.Status == contract.StatusPassed
			result.Outcomes[bundle.TaskID] = append(result.Outcomes[bundle.TaskID], passed)
			if passed {
				metrics.Passed++
			}
		} else {
			metrics.Faults[bundle.Status]++
		}
		for _, name := range contract.CriticalFailures(bundle.Graders) {
			metrics.CriticalFailures = append(metrics.CriticalFailures, contract.CriticalFailure{
				TaskID: bundle.TaskID, Grader: name, Detail: bundle.FailureClass,
			})
		}
		addUsage(&metrics.Usage, bundle.Usage)
	}

	_, _, metrics.PassRate = result.Outcomes.PassRate()
	metrics.IntervalLow, metrics.IntervalHigh = Wilson(metrics.Passed, metrics.Scored, Z95)
	metrics.ConsistencyRate = result.Outcomes.ConsistencyRate()
	metrics.MedianMS = percentileOf(durations, 0.5)
	metrics.P95MS = percentileOf(durations, 0.95)
	result.Metrics = metrics
	return result
}
