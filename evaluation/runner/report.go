package runner

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/gougoujiang/buildmax/evaluation/contract"
)

// WriteReport renders one subject's result.
//
// The layout follows section 12: a vector, never a single number. Pass rate,
// consistency, harness faults, and critical failures are printed as separate
// lines because none of them may be averaged into another — a trust violation
// that a rising pass rate could absorb is exactly what section 7.5 forbids.
func WriteReport(w io.Writer, result Result) {
	m := result.Metrics
	fmt.Fprintf(w, "Suite       : %s\n", orDash(m.Suite))
	fmt.Fprintf(w, "Subject     : %s (%s)\n", orDash(result.Subject.Name), shortID(m.SubjectID))
	fmt.Fprintf(w, "Model       : %s via %s\n",
		orDash(result.Subject.Model.Target), orDash(result.Subject.Model.Transport))
	fmt.Fprintf(w, "Build       : %s\n", buildLine(result.Subject.Build))

	fmt.Fprintf(w, "\nPass rate   : %.0f%% (%d/%d scored)  95%% CI [%.0f%%, %.0f%%]\n",
		m.PassRate*100, m.Passed, m.Scored, m.IntervalLow*100, m.IntervalHigh*100)
	fmt.Fprintf(w, "Consistency : %.0f%% of tasks passed every attempt\n", m.ConsistencyRate*100)
	fmt.Fprintf(w, "Trials      : %d run, %d scored\n", m.Trials, m.Scored)

	if unscored := m.Trials - m.Scored; unscored > 0 {
		// Faults are printed with their own denominator rather than folded into
		// the pass rate. A suite that lost a third of its trials must not read
		// like one that ran clean.
		fmt.Fprintf(w, "Unscored    : %d trial(s) — %s\n", unscored, faultLine(m.Faults))
	}

	fmt.Fprintf(w, "Duration    : median %s, p95 %s\n", ms(m.MedianMS), ms(m.P95MS))
	fmt.Fprintf(w, "Usage       : %d model calls, %d tool calls, %d prompt + %d completion tokens\n",
		m.Usage.LLMCalls, m.Usage.ToolCalls, m.Usage.PromptTokens, m.Usage.CompletionTokens)
	fmt.Fprintf(w, "Cost        : %s\n", costLine(m.Usage))

	if len(m.CriticalFailures) > 0 {
		fmt.Fprintf(w, "\nCRITICAL FAILURES (%d) — these are gates, not inputs to the rate above:\n",
			len(m.CriticalFailures))
		for _, f := range m.CriticalFailures {
			fmt.Fprintf(w, "  %s: %s%s\n", f.TaskID, f.Grader, detailSuffix(f.Detail))
		}
	}

	fmt.Fprintf(w, "\nPer task:\n")
	for _, id := range sortedTaskIDs(result.Outcomes) {
		attempts := result.Outcomes[id]
		passed := 0
		for _, ok := range attempts {
			if ok {
				passed++
			}
		}
		fmt.Fprintf(w, "  %-40s %d/%d\n", id, passed, len(attempts))
	}
	for _, b := range result.Bundles {
		if b.Status.Scored() || b.Status == contract.StatusPassed {
			continue
		}
		fmt.Fprintf(w, "  %-40s %s: %s\n", b.TaskID, b.Status, bound(b.Error, 160))
	}
}

// WriteComparison renders a candidate against a baseline.
func WriteComparison(w io.Writer, baseline, candidate Result, cmp Comparison) {
	fmt.Fprintf(w, "Baseline    : %s (%s)  %.0f%%\n",
		orDash(baseline.Subject.Name), shortID(baseline.Subject.ID), baseline.Metrics.PassRate*100)
	fmt.Fprintf(w, "Candidate   : %s (%s)  %.0f%%\n",
		orDash(candidate.Subject.Name), shortID(candidate.Subject.ID), candidate.Metrics.PassRate*100)
	fmt.Fprintf(w, "\nPaired delta: %+.1f points over %d task(s), 95%% CI [%+.1f, %+.1f]\n",
		cmp.Delta*100, cmp.Paired, cmp.Low*100, cmp.High*100)

	// The verdict is about the interval, not the point estimate. A delta whose
	// interval spans zero is the measurement saying it cannot tell, and
	// reporting it as a change is how noise becomes a regression.
	switch {
	case cmp.Paired == 0:
		fmt.Fprintf(w, "Verdict     : nothing to compare — no task was scored on both sides\n")
	case cmp.Low > 0:
		fmt.Fprintf(w, "Verdict     : the candidate is better; the interval excludes zero\n")
	case cmp.High < 0:
		fmt.Fprintf(w, "Verdict     : the candidate is worse; the interval excludes zero\n")
	default:
		fmt.Fprintf(w, "Verdict     : indistinguishable at this sample size; the interval spans zero\n")
	}

	if len(cmp.Improved) > 0 {
		fmt.Fprintf(w, "\nImproved (%d): %s\n", len(cmp.Improved), strings.Join(cmp.Improved, ", "))
	}
	if len(cmp.Regressed) > 0 {
		fmt.Fprintf(w, "Regressed (%d): %s\n", len(cmp.Regressed), strings.Join(cmp.Regressed, ", "))
	}
	if len(cmp.Unscorable) > 0 {
		fmt.Fprintf(w, "Unscorable (%d): %s\n", len(cmp.Unscorable), strings.Join(cmp.Unscorable, ", "))
	}

	// Critical failures survive a favourable delta. A candidate that improved
	// everywhere and broke a boundary has not passed.
	for _, r := range []Result{baseline, candidate} {
		if len(r.Metrics.CriticalFailures) == 0 {
			continue
		}
		fmt.Fprintf(w, "\n%s has %d critical failure(s), which no delta offsets:\n",
			orDash(r.Subject.Name), len(r.Metrics.CriticalFailures))
		for _, f := range r.Metrics.CriticalFailures {
			fmt.Fprintf(w, "  %s: %s\n", f.TaskID, f.Grader)
		}
	}
}

func sortedTaskIDs(outcomes TaskOutcomes) []string {
	ids := make([]string, 0, len(outcomes))
	for id := range outcomes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func faultLine(faults map[contract.TrialStatus]int) string {
	if len(faults) == 0 {
		return "none recorded"
	}
	statuses := make([]string, 0, len(faults))
	for status := range faults {
		statuses = append(statuses, string(status))
	}
	sort.Strings(statuses)
	parts := make([]string, 0, len(statuses))
	for _, s := range statuses {
		parts = append(parts, fmt.Sprintf("%s %d", s, faults[contract.TrialStatus(s)]))
	}
	return strings.Join(parts, ", ")
}

func costLine(u contract.Usage) string {
	if u.Cost == nil {
		// "unavailable" rather than zero: BuildMax does not know what an
		// unpriced model charges, and a zero would read as free.
		return "unavailable (the model carried no pricing)"
	}
	line := fmt.Sprintf("%.4f %s", float64(*u.Cost)/1e9, u.Currency)
	if u.CostIncomplete {
		line += " (incomplete — part of the run could not be priced)"
	}
	return line
}

func buildLine(b contract.BuildIdentity) string {
	line := fmt.Sprintf("%s (%s)", orDash(b.Version), orDash(b.Commit))
	if b.Dirty != "" {
		line += " + uncommitted changes " + shortID(b.Dirty)
	}
	return line
}

func detailSuffix(detail string) string {
	if detail == "" {
		return ""
	}
	return " — " + detail
}

func ms(v int64) string {
	if v <= 0 {
		return "—"
	}
	if v < 1000 {
		return fmt.Sprintf("%dms", v)
	}
	return fmt.Sprintf("%.1fs", float64(v)/1000)
}

func shortID(id string) string {
	id = strings.TrimPrefix(id, "sha256:")
	if len(id) <= 12 {
		return orDash(id)
	}
	return id[:12]
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func bound(s string, max int) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\n", " ")
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
