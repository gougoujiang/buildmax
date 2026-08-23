package main

import (
	"errors"
	"testing"

	"github.com/gougoujiang/buildmax/internal/bootstrap"
	"github.com/gougoujiang/buildmax/internal/core/model"
)

// Exit status is what the scheduler reads, and under Kubernetes it decides
// whether the Job starts another pod. A run that already reported its own
// outcome must not be reported again as a failed dispatch.
func TestWorkerExitCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{name: "a run that finished", err: nil, want: 0},
		{name: "a run that was canceled", err: model.ErrRunCanceled, want: 0},
		{name: "a run interrupted by shutdown", err: model.ErrRunInterrupted, want: 0},
		{name: "a run wrapped in context", err: errors.Join(errors.New("worker run"), model.ErrRunInterrupted), want: 0},
		{name: "a run another worker had claimed", err: bootstrap.ErrAlreadyClaimed, want: 2},
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
