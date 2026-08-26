package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/gougoujiang/buildmax/internal/bootstrap"
	coretask "github.com/gougoujiang/buildmax/internal/core/task"
)

// Exit status is what the scheduler reads, and under Kubernetes it decides
// whether the Job starts another pod. A run that already reported its own
// outcome must not be reported again as a failed dispatch, and neither must a
// run this worker never owned.
//
// Non-zero is reserved for the case a restart can fix: a worker that died
// before claiming its run, which leaves the run SCHEDULED for a fresh pod.
func TestWorkerExitCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{name: "a run that finished", err: nil, want: 0},
		{name: "a run that was canceled", err: coretask.ErrRunCanceled, want: 0},
		{name: "a run interrupted by shutdown", err: coretask.ErrRunInterrupted, want: 0},
		{name: "a run wrapped in context", err: errors.Join(errors.New("worker run"), coretask.ErrRunInterrupted), want: 0},
		{name: "a run another worker had claimed", err: bootstrap.ErrAlreadyClaimed, want: 0},
		// The status guard reaches this wrapped; a restarted pod takes that
		// path, not the transition's.
		{name: "a run no longer SCHEDULED", err: fmt.Errorf("%w (status=RUNNING)", bootstrap.ErrAlreadyClaimed), want: 0},
		{name: "a run that failed at its work", err: errors.New("the model refused"), want: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := workerExitCode("rt_1", tc.err); got != tc.want {
				t.Errorf("workerExitCode(%v) = %d, want %d", tc.err, got, tc.want)
			}
		})
	}
}
