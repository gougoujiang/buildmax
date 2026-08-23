package contract

import "time"

// Experiment is one measurement run: a set of subjects over a dataset, with a
// repetition count. It is recorded beside its trials so a bundle directory
// explains itself without the command that produced it.
type Experiment struct {
	ContractVersion int        `json:"contract_version"`
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	CreatedAt       time.Time  `json:"created_at"`
	Dataset         DatasetRef `json:"dataset"`

	// Subjects are what was measured. A comparison names two of them; a single
	// qualification run has one. They are stored in full rather than by ID
	// because the manifest is the only thing that makes an old result legible.
	Subjects []SubjectManifest `json:"subjects"`
	// Baseline is the subject ID other subjects are compared against, empty
	// when the experiment is not a comparison.
	Baseline string `json:"baseline,omitempty"`

	// Trials is the repetition count applied to every task, overriding the
	// task's own default when higher. Section 12 estimates pass@1 from
	// independent attempts, so one attempt per task is a measurement without an
	// error bar rather than a cheaper version of the same result.
	Trials int `json:"trials"`

	// Tasks are the task IDs included, recorded because a suite filter that is
	// not written down turns two runs of "the same" experiment into an
	// unpaired comparison.
	Tasks []string `json:"tasks"`
}

// SuiteMetrics is one suite's result vector for one subject. Section 12 asks
// for a vector rather than a score: the fields here are separate because
// nothing in this struct is allowed to be averaged into the others.
type SuiteMetrics struct {
	Suite     string `json:"suite"`
	SubjectID string `json:"subject_id"`

	Trials int `json:"trials"`
	// Scored is the number of trials that judged the subject. The pass rate
	// below is over this, not over Trials, so harness faults neither count as
	// failures nor silently shrink the denominator without being visible.
	Scored int `json:"scored"`
	Passed int `json:"passed"`

	// PassRate is Passed over Scored, and IntervalLow/High its confidence
	// interval. A rate without an interval invites reading a two-trial
	// difference as a regression.
	PassRate     float64 `json:"pass_rate"`
	IntervalLow  float64 `json:"interval_low"`
	IntervalHigh float64 `json:"interval_high"`

	// ConsistencyRate is the share of tasks passing every attempt: pass^k, for
	// suites where consistency rather than best-of-k is the product promise.
	ConsistencyRate float64 `json:"consistency_rate,omitempty"`

	// Faults counts the statuses that blamed the harness, by status. Section 12
	// reports these rather than dropping them, because a suite losing a third
	// of its trials otherwise looks like one that ran clean.
	Faults map[TrialStatus]int `json:"faults,omitempty"`

	// CriticalFailures names the task and grader of every critical failure.
	// These are gates, not inputs to a rate.
	CriticalFailures []CriticalFailure `json:"critical_failures,omitempty"`

	Usage    Usage `json:"usage"`
	MedianMS int64 `json:"median_ms,omitempty"`
	P95MS    int64 `json:"p95_ms,omitempty"`
}

// CriticalFailure is one hard-gate violation, named so a release decision can
// cite it.
type CriticalFailure struct {
	TaskID string `json:"task_id"`
	Grader string `json:"grader"`
	Detail string `json:"detail,omitempty"`
}

// Comparison is a candidate measured against a baseline over the same tasks.
// Pairing is by task and trial index, which is why TrialBundle records the
// index rather than relying on file order.
type Comparison struct {
	ContractVersion int    `json:"contract_version"`
	ExperimentID    string `json:"experiment_id"`
	Suite           string `json:"suite"`
	BaselineID      string `json:"baseline_id"`
	CandidateID     string `json:"candidate_id"`

	Baseline  SuiteMetrics `json:"baseline"`
	Candidate SuiteMetrics `json:"candidate"`

	// Delta is candidate pass rate minus baseline, with the interval of the
	// paired difference. The interval is the point of a paired design: two
	// separately-computed rates can both move without their difference being
	// distinguishable from noise.
	Delta     float64 `json:"delta"`
	DeltaLow  float64 `json:"delta_low"`
	DeltaHigh float64 `json:"delta_high"`

	// Improved and Regressed name tasks whose outcome changed direction.
	// Unscorable names tasks the comparison could not judge on either side,
	// which section 12 requires shown rather than omitted: a task silently
	// dropped from both arms reads as agreement.
	Improved   []string `json:"improved,omitempty"`
	Regressed  []string `json:"regressed,omitempty"`
	Unscorable []string `json:"unscorable,omitempty"`
}
