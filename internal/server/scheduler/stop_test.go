package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

// blockingRunner holds a dispatch until it is released, the way local_process
// mode holds one for the whole length of the agent run.
type blockingRunner struct {
	started  chan struct{}
	release  chan struct{}
	mu       sync.Mutex
	ctxEnded bool
	once     sync.Once
}

func newBlockingRunner() *blockingRunner {
	return &blockingRunner{started: make(chan struct{}), release: make(chan struct{})}
}

func (b *blockingRunner) Run(ctx context.Context, _ model.TaskRun, _ string) (string, *string, *time.Time, error) {
	b.once.Do(func() { close(b.started) })
	select {
	case <-ctx.Done():
		b.mu.Lock()
		b.ctxEnded = true
		b.mu.Unlock()
		return "local_process", nil, nil, nil
	case <-b.release:
		return "local_process", nil, nil, nil
	}
}

func (b *blockingRunner) sawItsContextEnd() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.ctxEnded
}

// The case this design exists for: stopping must not wait for an agent run to
// finish. Claiming stops at once, and the dispatch already in flight is asked
// to stop rather than waited on indefinitely.
func TestStopAsksAnInFlightDispatchToStop(t *testing.T) {
	spy := newSpyTaskRunStore("r_stop_inflight_0000000")
	runner := newBlockingRunner()
	s, err := NewSchedulerWithPollInterval(spy, runner, nil, 5*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	s.Start()

	select {
	case <-runner.started:
	case <-time.After(2 * time.Second):
		t.Fatal("the scheduler never dispatched the pending run")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	start := time.Now()
	if !s.Stop(ctx) {
		t.Fatal("Stop reported that a dispatch was still running")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Stop took %v; it waited for the run instead of ending it", elapsed)
	}
	if !runner.sawItsContextEnd() {
		t.Error("the dispatch was abandoned rather than asked to stop")
	}
}

// A dispatch that ignores the request is left behind rather than holding the
// process open — the run is closed later by the stale-run reaper.
func TestStopGivesUpOnADispatchThatWillNotStop(t *testing.T) {
	spy := newSpyTaskRunStore("r_stop_stuck_000000000")
	stuck := make(chan struct{})
	defer close(stuck)
	s, err := NewSchedulerWithPollInterval(spy, runnerFunc(func(context.Context, model.TaskRun, string) (string, *string, *time.Time, error) {
		<-stuck
		return "local_process", nil, nil, nil
	}), nil, 5*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	s.Start()
	time.Sleep(50 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if s.Stop(ctx) {
		t.Error("Stop claimed a stuck dispatch had finished")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Stop waited %v on a stuck dispatch", elapsed)
	}
}

// A worker that stopped because the server is stopping has already reported its
// own outcome. Recording a dispatch failure over it would replace what the run
// produced with a message about the server.
func TestStopDoesNotRewriteARunItsWorkerAlreadyReported(t *testing.T) {
	taskRunID := "r_stop_reported_0000000"
	spy := newSpyTaskRunStore(taskRunID)
	s, err := NewSchedulerWithPollInterval(spy, runnerFunc(func(ctx context.Context, _ model.TaskRun, _ string) (string, *string, *time.Time, error) {
		<-ctx.Done()
		// What exec reports for a worker that was signalled.
		return "", nil, nil, context.Canceled
	}), nil, 5*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	s.Start()
	time.Sleep(50 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	s.Stop(ctx)

	spy.mu.Lock()
	defer spy.mu.Unlock()
	if spy.lastUpdateStatus != nil {
		t.Errorf("the run was rewritten to %q after its worker was asked to stop", spy.lastUpdateStatus.status)
	}
}

// Only one run is dispatched at a time, which is what the loop did when it
// dispatched inline. Moving dispatch to its own goroutine must not quietly
// become unbounded concurrency.
func TestOnlyOneDispatchRunsAtATime(t *testing.T) {
	spy := newSpyTaskRunStore("r_stop_oneatatime_00000")
	var mu sync.Mutex
	concurrent, peak := 0, 0
	release := make(chan struct{})
	defer close(release)

	s, err := NewSchedulerWithPollInterval(spy, runnerFunc(func(context.Context, model.TaskRun, string) (string, *string, *time.Time, error) {
		mu.Lock()
		concurrent++
		if concurrent > peak {
			peak = concurrent
		}
		mu.Unlock()
		<-release
		mu.Lock()
		concurrent--
		mu.Unlock()
		return "local_process", nil, nil, nil
	}), nil, 5*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	s.Start()
	time.Sleep(100 * time.Millisecond) // many ticks, one slot

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	s.Stop(ctx)

	mu.Lock()
	defer mu.Unlock()
	if peak > maxConcurrentDispatch {
		t.Errorf("peak concurrent dispatches = %d, want at most %d", peak, maxConcurrentDispatch)
	}
}

// runnerFunc adapts a function to WorkerRunner.
type runnerFunc func(context.Context, model.TaskRun, string) (string, *string, *time.Time, error)

func (f runnerFunc) Run(ctx context.Context, run model.TaskRun, token string) (string, *string, *time.Time, error) {
	return f(ctx, run, token)
}
