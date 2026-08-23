package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gougoujiang/buildmax/evaluation/adapter"
	"github.com/gougoujiang/buildmax/evaluation/contract"
	"github.com/gougoujiang/buildmax/evaluation/grader"
)

// oracleTimeout bounds a reference solution. An oracle is meant to be a direct
// solution, so one that runs longer than this is stuck rather than working.
const oracleTimeout = 2 * time.Minute

// Preflight checks that a task measures what it claims to, without running a
// model.
//
// Section 8.1 asks five things of a committed task. Four are checked here:
//
//   - the hidden material stays outside the trial workspace;
//   - the initial state does not already satisfy the required graders, because
//     a task that starts finished passes for every subject and distinguishes
//     none of them;
//   - the oracle completes the task; and
//   - every required grader accepts the oracle, since a task whose own
//     reference solution fails is measuring its graders rather than a subject.
//
// The fifth — that repeated oracle runs are deterministic enough — needs
// repetition policy this function does not own, and is left to the caller.
//
// Only deterministic graders take part. A trace grader has no trace to read
// when nothing ran an agent, and a model grader would need a provider; running
// either here would report an unknown verdict as a task defect.
func Preflight(ctx context.Context, entry TaskEntry) error {
	deterministic := deterministicGraders(entry.Task)
	if len(deterministic) == 0 {
		// Nothing here can be checked without a model. Say so rather than
		// returning a pass that means "not examined".
		return fmt.Errorf("task %s has no required deterministic grader, so preflight cannot check it", entry.Task.ID)
	}

	workspace, err := os.MkdirTemp("", "buildmax-preflight-*")
	if err != nil {
		return fmt.Errorf("create preflight workspace: %w", err)
	}
	defer func() { _ = os.RemoveAll(workspace) }()

	if err := adapter.Materialize(entry.Dir, workspace); err != nil {
		return fmt.Errorf("task %s: materialize: %w", entry.Task.ID, err)
	}
	leaked, err := adapter.VerifyBoundary(entry.Dir, workspace)
	if err != nil {
		return fmt.Errorf("task %s: check boundary: %w", entry.Task.ID, err)
	}
	if len(leaked) > 0 {
		return fmt.Errorf("task %s: hidden material is reachable in the workspace: %s",
			entry.Task.ID, strings.Join(leaked, ", "))
	}

	graders := grader.Builtin()
	input := grader.Input{Workspace: workspace, TaskDir: entry.Dir}

	if entry.Task.Negative {
		// The outcome is that something did not happen, so the initial state
		// passing the deterministic graders is correct rather than a defect.
		// What the task then owes is a process assertion: without one it says
		// only that nothing happened, and a subject that never ran would
		// satisfy it too.
		if !hasRequiredProcessGrader(entry.Task) {
			return fmt.Errorf("task %s is negative but has no required trace or model grader, "+
				"so a subject that did nothing at all would pass it", entry.Task.ID)
		}
	} else {
		before := graders.GradeAll(ctx, contract.Task{Graders: deterministic}, input)
		if contract.DecideStatus(before) == contract.StatusPassed {
			return fmt.Errorf("task %s: the untouched initial state already passes every required grader, "+
				"so the task asks for something already true", entry.Task.ID)
		}
	}

	if len(entry.Task.Oracle) == 0 {
		return fmt.Errorf("task %s: has no oracle, so nothing shows the task is solvable", entry.Task.ID)
	}
	if err := runOracle(ctx, entry, workspace); err != nil {
		return fmt.Errorf("task %s: %w", entry.Task.ID, err)
	}

	after := graders.GradeAll(ctx, contract.Task{Graders: deterministic}, input)
	if status := contract.DecideStatus(after); status != contract.StatusPassed {
		return fmt.Errorf("task %s: the oracle did not satisfy its own graders (%s): %s",
			entry.Task.ID, status, explain(after))
	}
	return nil
}

// PreflightSuite checks every task and reports all failures rather than the
// first: a contributor fixing a suite wants the whole list.
func PreflightSuite(ctx context.Context, tasks []TaskEntry) error {
	var problems []error
	for _, entry := range tasks {
		if err := Preflight(ctx, entry); err != nil {
			problems = append(problems, err)
		}
	}
	return errors.Join(problems...)
}

// runOracle executes the reference solution in the workspace.
func runOracle(ctx context.Context, entry TaskEntry, workspace string) error {
	runCtx, cancel := context.WithTimeout(ctx, oracleTimeout)
	defer cancel()

	// A relative path resolves against the oracle directory, the same way a
	// grader command resolves against the graders directory: both live outside
	// the workspace, and neither may be satisfied by something inside it.
	//
	// The resolved path is made absolute because the command runs with the
	// workspace as its working directory. Leaving it relative would resolve it
	// against the workspace instead — the one place it must not come from.
	oracleDir, err := filepath.Abs(filepath.Join(entry.Dir, contract.OracleDir))
	if err != nil {
		return fmt.Errorf("resolve oracle directory: %w", err)
	}
	resolve := func(s string) string {
		if strings.HasPrefix(s, "./") {
			return filepath.Join(oracleDir, filepath.FromSlash(s))
		}
		return s
	}

	args := append([]string(nil), entry.Task.Oracle...)
	name := resolve(args[0])
	args = args[1:]
	for i, a := range args {
		args[i] = resolve(a)
	}

	cmd := exec.CommandContext(runCtx, name, args...)
	cmd.Dir = workspace
	cmd.WaitDelay = 10 * time.Second
	out, err := cmd.CombinedOutput()
	if runCtx.Err() != nil {
		return fmt.Errorf("the oracle exceeded %s", oracleTimeout)
	}
	if err != nil {
		return fmt.Errorf("the oracle failed: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// hasRequiredProcessGrader reports whether the task gates on evidence that only
// a real run produces. It is what makes a negative task measure something: a
// deterministic grader can see that nothing changed, but only a trace can show
// the subject did the work without crossing the line.
func hasRequiredProcessGrader(task contract.Task) bool {
	for _, g := range task.Graders {
		if g.Required && (g.Kind == contract.GraderTrace || g.Kind == contract.GraderModel) {
			return true
		}
	}
	return false
}

// deterministicGraders returns the required graders preflight can evaluate.
func deterministicGraders(task contract.Task) []contract.GraderRef {
	var refs []contract.GraderRef
	for _, g := range task.Graders {
		if g.Required && g.Kind == contract.GraderDeterministic {
			refs = append(refs, g)
		}
	}
	return refs
}

func explain(results []contract.GraderResult) string {
	var parts []string
	for _, r := range results {
		if r.Verdict == contract.VerdictPass {
			continue
		}
		detail := r.Explanation
		if r.Error != "" {
			detail = r.Error
		}
		parts = append(parts, r.Name+": "+detail)
	}
	return strings.Join(parts, "; ")
}
