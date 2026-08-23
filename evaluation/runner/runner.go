package runner

import (
	"context"
	"fmt"

	"github.com/gougoujiang/buildmax/evaluation/adapter"
	"github.com/gougoujiang/buildmax/evaluation/contract"
	"github.com/gougoujiang/buildmax/evaluation/grader"
)

// DefaultResamples is how many bootstrap draws a comparison takes. Two thousand
// is enough for a stable 95% percentile interval at the task counts a suite
// realistically holds, and cheap next to running the trials themselves.
const DefaultResamples = 2000

// Runner executes an experiment against one subject.
type Runner struct {
	Adapter *adapter.CLI
	Graders grader.Registry
	// BundleRoot is where trial evidence is written.
	BundleRoot string
	// Trials overrides each task's own repetition count when higher. Section 12
	// estimates pass@1 from independent attempts, so raising it is how a
	// scheduled run buys a tighter interval than a pull-request run needs.
	Trials int
	// KeepFailures leaves the temporary workspace of a failed trial in place so
	// a contributor can look at it. The bundle records where.
	KeepFailures bool
	// Progress receives one line per finished trial. Nil discards them.
	Progress func(string)
}

// Result is one subject's outcome across a suite.
type Result struct {
	Subject  contract.SubjectManifest
	Bundles  []contract.TrialBundle
	Outcomes TaskOutcomes
	Metrics  contract.SuiteMetrics
}

// Run executes every task the given number of times and returns the subject's
// result. It writes each trial's bundle under BundleRoot as it goes, so an
// interrupted experiment leaves the evidence it already gathered.
func (r *Runner) Run(ctx context.Context, tasks []TaskEntry, subject contract.SubjectManifest, experimentID string) (Result, error) {
	if r.Adapter == nil {
		return Result{}, fmt.Errorf("runner has no adapter")
	}
	graders := r.Graders
	if graders == nil {
		graders = grader.Builtin()
	}

	result := Result{Subject: subject, Outcomes: TaskOutcomes{}}
	metrics := contract.SuiteMetrics{SubjectID: subject.ID, Faults: map[contract.TrialStatus]int{}}
	var durations []int64

	for _, entry := range tasks {
		if metrics.Suite == "" {
			metrics.Suite = entry.Task.Suite
		}
		trials := entry.Task.Trials
		if trials <= 0 {
			trials = 1
		}
		if r.Trials > trials {
			trials = r.Trials
		}

		for i := 0; i < trials; i++ {
			if err := ctx.Err(); err != nil {
				// A cancelled experiment stops, but what it already wrote
				// stands: the bundles on disk are as valid as they were a
				// moment ago.
				return result, err
			}
			bundle, err := r.runOne(ctx, entry, subject, experimentID, i, graders)
			if err != nil {
				return result, err
			}
			result.Bundles = append(result.Bundles, bundle)
			durations = append(durations, int64(bundle.Duration))

			metrics.Trials++
			if bundle.Status.Scored() {
				metrics.Scored++
				passed := bundle.Status == contract.StatusPassed
				result.Outcomes[entry.Task.ID] = append(result.Outcomes[entry.Task.ID], passed)
				if passed {
					metrics.Passed++
				}
			} else {
				metrics.Faults[bundle.Status]++
			}
			for _, name := range contract.CriticalFailures(bundle.Graders) {
				metrics.CriticalFailures = append(metrics.CriticalFailures, contract.CriticalFailure{
					TaskID: entry.Task.ID, Grader: name, Detail: bundle.FailureClass,
				})
			}
			addUsage(&metrics.Usage, bundle.Usage)

			if r.Progress != nil {
				r.Progress(fmt.Sprintf("%-40s trial %d/%d  %s", entry.Task.ID, i+1, trials, bundle.Status))
			}
		}
	}

	_, _, metrics.PassRate = result.Outcomes.PassRate()
	metrics.IntervalLow, metrics.IntervalHigh = Wilson(metrics.Passed, metrics.Scored, Z95)
	metrics.ConsistencyRate = result.Outcomes.ConsistencyRate()
	metrics.MedianMS = percentileOf(durations, 0.5)
	metrics.P95MS = percentileOf(durations, 0.95)
	result.Metrics = metrics
	return result, nil
}

// runOne executes and grades a single attempt.
func (r *Runner) runOne(ctx context.Context, entry TaskEntry, subject contract.SubjectManifest,
	experimentID string, index int, graders grader.Registry) (contract.TrialBundle, error) {

	res, err := r.Adapter.Run(ctx, adapter.Trial{
		Task:         entry.Task,
		TaskDir:      entry.Dir,
		Subject:      subject,
		ExperimentID: experimentID,
		TrialID:      fmt.Sprintf("%s-%s-%d", experimentID, entry.Task.ID, index),
		Index:        index,
	}, r.BundleRoot)
	if err != nil {
		return contract.TrialBundle{}, fmt.Errorf("task %s trial %d: %w", entry.Task.ID, index, err)
	}

	bundle := res.Bundle
	if res.Gradable {
		bundle.Graders = graders.GradeAll(ctx, entry.Task, grader.Input{
			Workspace: res.Workspace,
			TaskDir:   entry.Dir,
			TrialDir:  res.TrialDir,
			Bundle:    bundle,
		})
		bundle.Status = contract.DecideStatus(bundle.Graders)
	}

	// Grading has to finish before the workspace goes away, which is why
	// cleanup happens here rather than in the adapter.
	keep := r.KeepFailures && bundle.Status != contract.StatusPassed
	if keep {
		bundle.Reproduce.Note = "workspace kept at " + res.Workspace
	} else if res.Cleanup != nil {
		res.Cleanup()
	}

	if _, err := contract.WriteBundle(r.BundleRoot, bundle); err != nil {
		return contract.TrialBundle{}, fmt.Errorf("task %s trial %d: %w", entry.Task.ID, index, err)
	}
	return bundle, nil
}

// addUsage accumulates a trial's consumption into a suite total. Cost is summed
// only while every trial so far could be priced: a total mixing priced and
// unpriced trials understates spend while looking exact, so the incomplete flag
// is what a reader needs rather than a larger-looking number.
func addUsage(total *contract.Usage, trial contract.Usage) {
	total.LLMCalls += trial.LLMCalls
	total.ToolCalls += trial.ToolCalls
	total.PromptTokens += trial.PromptTokens
	total.CompletionTokens += trial.CompletionTokens
	total.CacheReadTokens += trial.CacheReadTokens
	total.CacheWriteTokens += trial.CacheWriteTokens

	if trial.CostIncomplete {
		total.CostIncomplete = true
	}
	if trial.Cost == nil {
		// An unpriced trial makes the total incomplete rather than leaving it
		// unchanged, which would report the priced subset as the whole.
		if total.Cost != nil {
			total.CostIncomplete = true
		}
		return
	}
	if total.Cost == nil {
		sum := *trial.Cost
		total.Cost = &sum
		total.Currency = trial.Currency
		return
	}
	if total.Currency != trial.Currency {
		// Two currencies cannot be added. Keeping the first and flagging the
		// total is honest; converting would invent a rate.
		total.CostIncomplete = true
		return
	}
	*total.Cost += *trial.Cost
}
