package contract

// Verdict is one grader's judgement. It is three-valued because section 10.3
// requires a model grader to have an unknown path: a judge forced to choose
// between pass and fail on evidence it cannot read will choose one, and a
// coerced verdict is indistinguishable from a considered one afterwards.
type Verdict string

const (
	VerdictPass    Verdict = "pass"
	VerdictFail    Verdict = "fail"
	VerdictUnknown Verdict = "unknown"
)

// GraderResult is what one grader concluded about one trial.
type GraderResult struct {
	Name    string     `json:"name"`
	Version int        `json:"version"`
	Kind    GraderKind `json:"kind"`
	// Required and Critical are copied from the task's GraderRef rather than
	// looked up. A bundle is read years after the task may have changed, and a
	// verdict whose gating weight has to be resolved elsewhere is a verdict
	// that will eventually be re-interpreted.
	Required bool `json:"required"`
	Critical bool `json:"critical,omitempty"`

	Verdict Verdict `json:"verdict"`
	// Score is the dimension's value where one exists, in [0,1]. Absent for a
	// grader that only decides, which is not the same as scoring zero.
	Score *float64 `json:"score,omitempty"`
	// Explanation is why, in the grader's own words. Deterministic graders put
	// their output here; model graders put their reasoning.
	Explanation string `json:"explanation,omitempty"`

	// Subject identifies the model that produced a model grader's verdict, and
	// Usage what it cost. Section 10.3 makes a judge a measured component
	// rather than an oracle, and a judge whose own configuration is unrecorded
	// cannot be re-calibrated against the labels it was validated on.
	Subject string `json:"subject,omitempty"`
	Usage   *Usage `json:"usage,omitempty"`

	Duration Duration `json:"duration,omitempty"`
	// Error means the grader could not reach a verdict at all, as distinct
	// from reaching VerdictUnknown deliberately.
	Error string `json:"error,omitempty"`
}

// DecideStatus derives a trial's terminal status from its grader results,
// assuming execution itself completed. A caller that already knows the run
// failed, timed out, or was cancelled records that status instead: this
// function only distinguishes the outcomes grading can distinguish.
//
// A definite required failure wins over an inconclusive one. Once any required
// grader has said fail on evidence it could read, the trial has failed the task
// as written, and waiting on another grader's uncertainty would only turn a
// known result into an unknown one.
func DecideStatus(results []GraderResult) TrialStatus {
	for _, r := range results {
		if r.Required && r.Verdict == VerdictFail {
			return StatusFailed
		}
	}
	for _, r := range results {
		if !r.Required {
			continue
		}
		if r.Error != "" || r.Verdict == VerdictUnknown {
			return StatusGraderError
		}
	}
	return StatusPassed
}

// CriticalFailures returns the names of critical graders that failed. Section
// 7.5 keeps these outside every suite summary, so a report needs them by name
// rather than as a count folded into a pass rate.
func CriticalFailures(results []GraderResult) []string {
	var names []string
	for _, r := range results {
		if r.Critical && r.Verdict == VerdictFail {
			names = append(names, r.Name)
		}
	}
	return names
}
