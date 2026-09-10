package workflow

import "testing"

func TestValidRunStatusTransition(t *testing.T) {
	allowed := map[RunStatus][]RunStatus{
		RunStatusPending: {RunStatusRunning, RunStatusFailed, RunStatusCanceled},
		RunStatusRunning: {RunStatusSucceeded, RunStatusFailed, RunStatusCanceled},
	}
	all := []RunStatus{RunStatusPending, RunStatusRunning, RunStatusSucceeded, RunStatusFailed, RunStatusCanceled}
	for _, from := range all {
		ok := make(map[RunStatus]bool)
		for _, to := range allowed[from] {
			ok[to] = true
			if !ValidRunStatusTransition(from, to) {
				t.Errorf("%s -> %s should be allowed", from, to)
			}
		}
		for _, to := range all {
			if !ok[to] && ValidRunStatusTransition(from, to) {
				t.Errorf("%s -> %s should be refused", from, to)
			}
		}
	}
	// Terminal statuses never move, including to themselves.
	for _, from := range []RunStatus{RunStatusSucceeded, RunStatusFailed, RunStatusCanceled} {
		if ValidRunStatusTransition(from, from) {
			t.Errorf("terminal %s must not transition to itself", from)
		}
	}
}

func TestValidStepRunTransition(t *testing.T) {
	allowed := map[StepRunStatus][]StepRunStatus{
		StepRunStatusPending: {StepRunStatusRunning, StepRunStatusBlocked, StepRunStatusFailed, StepRunStatusCanceled},
		StepRunStatusRunning: {StepRunStatusSucceeded, StepRunStatusFailed, StepRunStatusCanceled},
	}
	all := []StepRunStatus{
		StepRunStatusPending, StepRunStatusRunning, StepRunStatusSucceeded,
		StepRunStatusFailed, StepRunStatusCanceled, StepRunStatusBlocked,
	}
	for _, from := range all {
		ok := make(map[StepRunStatus]bool)
		for _, to := range allowed[from] {
			ok[to] = true
			if !ValidStepRunTransition(from, to) {
				t.Errorf("%s -> %s should be allowed", from, to)
			}
		}
		for _, to := range all {
			if !ok[to] && ValidStepRunTransition(from, to) {
				t.Errorf("%s -> %s should be refused", from, to)
			}
		}
	}
}

func TestStatusTerminal(t *testing.T) {
	if RunStatusTerminal(RunStatusPending) || RunStatusTerminal(RunStatusRunning) {
		t.Error("pending/running runs are not terminal")
	}
	for _, s := range []RunStatus{RunStatusSucceeded, RunStatusFailed, RunStatusCanceled} {
		if !RunStatusTerminal(s) {
			t.Errorf("%s run should be terminal", s)
		}
	}
	if StepRunStatusTerminal(StepRunStatusPending) || StepRunStatusTerminal(StepRunStatusRunning) {
		t.Error("pending/running steps are not terminal")
	}
	// Blocked is terminal alongside the natural ends.
	for _, s := range []StepRunStatus{StepRunStatusSucceeded, StepRunStatusFailed, StepRunStatusCanceled, StepRunStatusBlocked} {
		if !StepRunStatusTerminal(s) {
			t.Errorf("%s step should be terminal", s)
		}
	}
}
