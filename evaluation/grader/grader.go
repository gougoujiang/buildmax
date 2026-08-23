// Package grader turns a finished trial into per-dimension verdicts.
//
// Graders are kept separate from the adapter deliberately. Execution reports
// what happened; grading decides what it means. Section 7.4 depends on the
// split: an adapter that also judged could not report a grader failure as
// anything other than the subject's.
package grader

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/gougoujiang/buildmax/evaluation/contract"
)

// Input is everything a grader may read about one trial.
type Input struct {
	// Ref is this grader's entry on the task, including its configuration.
	Ref contract.GraderRef
	// Workspace is the final state. Section 7.1 makes it the authoritative
	// answer wherever a deterministic check can reach one.
	Workspace string
	// TaskDir is the task directory, whose graders subdirectory holds material
	// the trial never saw.
	TaskDir string
	// TrialDir is the bundle directory, holding the durable trace.
	TrialDir string
	// Bundle is the execution evidence recorded so far.
	Bundle contract.TrialBundle
}

// Grader judges one dimension of a trial.
//
// A grader returns a result rather than an error even when it fails: a grader
// that could not reach a verdict is itself a recorded fact, and turning that
// into a Go error would let a caller drop it into the same bucket as a failing
// subject.
type Grader interface {
	Grade(ctx context.Context, in Input) contract.GraderResult
}

// Registry maps grader names to implementations.
type Registry map[string]Grader

// Builtin returns the graders the slice ships.
func Builtin() Registry {
	return Registry{
		"files":   Files{},
		"command": Command{},
		"trace":   Trace{},
	}
}

// Names lists the registered graders, sorted.
func (r Registry) Names() []string {
	names := make([]string, 0, len(r))
	for name := range r {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// GradeAll runs a task's graders in order and returns their results.
//
// An unknown grader produces a recorded error rather than a skipped entry. A
// task naming a grader this build does not have has not been evaluated, and
// silently omitting it would let the trial pass on the graders that happened to
// exist.
func (r Registry) GradeAll(ctx context.Context, task contract.Task, in Input) []contract.GraderResult {
	results := make([]contract.GraderResult, 0, len(task.Graders))
	for _, ref := range task.Graders {
		started := time.Now()
		g, ok := r[ref.Name]
		if !ok {
			results = append(results, contract.GraderResult{
				Name: ref.Name, Version: ref.Version, Kind: ref.Kind,
				Required: ref.Required, Critical: ref.Critical,
				Verdict: contract.VerdictUnknown,
				Error:   fmt.Sprintf("no grader named %q is registered; have %v", ref.Name, r.Names()),
			})
			continue
		}
		trialIn := in
		trialIn.Ref = ref
		res := g.Grade(ctx, trialIn)
		res.Name, res.Version, res.Kind = ref.Name, ref.Version, ref.Kind
		res.Required, res.Critical = ref.Required, ref.Critical
		// Timing is recorded here rather than left to each grader, and it
		// overwrites unconditionally. Treating a zero as "the grader did not
		// set one" would be indistinguishable from a grader that finished
		// inside a millisecond, which most file assertions do.
		res.Duration = contract.FromDuration(time.Since(started))
		results = append(results, res)
	}
	return results
}

// failed builds a result for a grader that reached a verdict of fail.
func failed(explanation string) contract.GraderResult {
	return contract.GraderResult{Verdict: contract.VerdictFail, Explanation: explanation}
}

// passed builds a result for a grader that reached a verdict of pass.
func passed(explanation string) contract.GraderResult {
	return contract.GraderResult{Verdict: contract.VerdictPass, Explanation: explanation}
}

// broken builds a result for a grader that could not reach a verdict at all,
// which section 8.3 records as grader_error rather than as a failing subject.
func broken(err error) contract.GraderResult {
	return contract.GraderResult{Verdict: contract.VerdictUnknown, Error: err.Error()}
}
