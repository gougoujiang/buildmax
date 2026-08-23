package turnqueue

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDrainRefusesNewTurns(t *testing.T) {
	r := NewRegistry()
	r.Drain()

	if _, err := r.Submit("conv-1", NewJob(func() {})); !errors.Is(err, ErrDraining) {
		t.Fatalf("Submit after Drain: err = %v, want ErrDraining", err)
	}
	if err := r.RunSync(context.Background(), "conv-1", func() {}); !errors.Is(err, ErrDraining) {
		t.Fatalf("RunSync after Drain: err = %v, want ErrDraining", err)
	}
}

func TestWaitLetsARunningTurnFinish(t *testing.T) {
	r := NewRegistry()
	release := make(chan struct{})
	finished := make(chan struct{})

	started := make(chan struct{})
	if _, err := r.Submit("conv-1", NewJob(func() {
		close(started)
		<-release
		close(finished)
	})); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	<-started

	r.Drain()

	// Still running, so Wait must not return yet.
	shortCtx, cancelShort := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancelShort()
	if r.Wait(shortCtx) {
		t.Fatal("Wait returned true while a turn was still running")
	}

	close(release)
	<-finished

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if !r.Wait(ctx) {
		t.Fatal("Wait did not observe the finished turn")
	}
}

func TestWaitReturnsWhenNothingIsRunning(t *testing.T) {
	r := NewRegistry()
	r.Drain()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if !r.Wait(ctx) {
		t.Fatal("Wait blocked on an idle registry")
	}
}

// A queued turn is the running goroutine's responsibility, so the count must
// not double for it — otherwise Wait would never see zero.
func TestQueuedTurnsDoNotLeakTheActiveCount(t *testing.T) {
	r := NewRegistry()
	release := make(chan struct{})
	started := make(chan struct{})

	if _, err := r.Submit("conv-1", NewJob(func() {
		close(started)
		<-release
	})); err != nil {
		t.Fatalf("Submit first: %v", err)
	}
	<-started
	if pos, err := r.Submit("conv-1", NewJob(func() {})); err != nil || pos != 1 {
		t.Fatalf("Submit second: pos = %d, err = %v; want 1, nil", pos, err)
	}

	r.Drain()
	close(release)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if !r.Wait(ctx) {
		t.Fatal("Wait never saw both turns finish")
	}
}
