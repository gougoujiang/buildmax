package bootstrap

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

// waitForCause waits for ctx to end and returns why, or fails the test.
func waitForCause(t *testing.T, ctx context.Context) error {
	t.Helper()
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-time.After(2 * time.Second):
		t.Fatal("the run's context never ended")
		return nil
	}
}

// The signal reaching the run as a *cause* is the whole mechanism: RunTask
// decides what to report by reading it, so a shutdown that arrived as a plain
// cancellation would leave the run unable to say why it stopped.
func TestShutdownReachesTheRunAsAnInterruption(t *testing.T) {
	processCtx, stopProcess := context.WithCancel(context.Background())
	runCtx, cancelRun := context.WithCancelCause(context.WithoutCancel(processCtx))
	defer cancelRun(nil)

	interruptRunOnShutdown(processCtx, runCtx, cancelRun)
	stopProcess()

	if cause := waitForCause(t, runCtx); !errors.Is(cause, model.ErrRunInterrupted) {
		t.Fatalf("run stopped because %v, want ErrRunInterrupted", cause)
	}
}

// A cancel already recorded on the server wins: the run stopped because someone
// asked it to, and a shutdown arriving afterwards does not rewrite that.
func TestAShutdownAfterACancelDoesNotRewriteTheReason(t *testing.T) {
	processCtx, stopProcess := context.WithCancel(context.Background())
	defer stopProcess()
	runCtx, cancelRun := context.WithCancelCause(context.WithoutCancel(processCtx))
	defer cancelRun(nil)

	interruptRunOnShutdown(processCtx, runCtx, cancelRun)
	cancelRun(model.ErrRunCanceled)
	stopProcess()

	if cause := waitForCause(t, runCtx); !errors.Is(cause, model.ErrRunCanceled) {
		t.Fatalf("run stopped because %v, want ErrRunCanceled", cause)
	}
}

// A run that finishes normally must not be marked interrupted by the process
// exiting afterwards.
func TestAFinishedRunIsNotInterruptedByTheProcessStopping(t *testing.T) {
	processCtx, stopProcess := context.WithCancel(context.Background())
	runCtx, cancelRun := context.WithCancelCause(context.WithoutCancel(processCtx))

	interruptRunOnShutdown(processCtx, runCtx, cancelRun)
	cancelRun(nil) // what RunWorker's defer does when the run is over
	stopProcess()

	if cause := waitForCause(t, runCtx); errors.Is(cause, model.ErrRunInterrupted) {
		t.Fatalf("a finished run was recorded as interrupted")
	}
}
