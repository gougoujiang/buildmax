package agentapp

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeHost drives the scheduler without a model. RunPrompt and RunBackgroundEvent
// record what they see and, when runFn is set, defer to it so a test can block or
// fail a turn deterministically.
type fakeHost struct {
	mu      sync.Mutex
	opened  int
	closed  int
	prompts []string
	events  []string
	openErr error
	runFn   func(ctx context.Context, label string) (RunResult, error)
}

func (f *fakeHost) OpenSession(string) (*SessionContext, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.openErr != nil {
		return nil, f.openErr
	}
	f.opened++
	return &SessionContext{}, nil
}

func (f *fakeHost) CloseSession(*SessionContext) {
	f.mu.Lock()
	f.closed++
	f.mu.Unlock()
}

func (f *fakeHost) RunPrompt(ctx context.Context, _ *SessionContext, prompt string, _ RunPromptOpts) (RunResult, error) {
	f.mu.Lock()
	f.prompts = append(f.prompts, prompt)
	fn := f.runFn
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, prompt)
	}
	return RunResult{Reply: prompt}, nil
}

func (f *fakeHost) RunBackgroundEvent(ctx context.Context, _ *SessionContext, ev BackgroundEvent, _ RunPromptOpts) (RunResult, error) {
	f.mu.Lock()
	f.events = append(f.events, ev.JobID)
	fn := f.runFn
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, ev.JobID)
	}
	return RunResult{Reply: ev.JobID}, nil
}

func (f *fakeHost) closedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

// recorder captures the lifecycle callbacks in order and signals when Done
// fires. closedAtDone is the host's close count observed inside Done, so a test
// can assert the session was released before the run's last callback.
type recorder struct {
	host *fakeHost

	mu           sync.Mutex
	started      int
	turnDone     int
	turnErrs     []error
	dequeued     []string
	doneErr      error
	closedAtDone int
	done         chan struct{}
}

func newRecorder(h *fakeHost) *recorder { return &recorder{host: h, done: make(chan struct{})} }

func (r *recorder) RunOpts() RunPromptOpts { return RunPromptOpts{} }
func (r *recorder) OnStart(*SessionContext) {
	r.mu.Lock()
	r.started++
	r.mu.Unlock()
}
func (r *recorder) TurnDone(RunResult) {
	r.mu.Lock()
	r.turnDone++
	r.mu.Unlock()
}
func (r *recorder) TurnError(err error) {
	r.mu.Lock()
	r.turnErrs = append(r.turnErrs, err)
	r.mu.Unlock()
}
func (r *recorder) Dequeued(next string, _ []string) {
	r.mu.Lock()
	r.dequeued = append(r.dequeued, next)
	r.mu.Unlock()
}
func (r *recorder) Done(_ RunResult, err error) {
	r.mu.Lock()
	r.doneErr = err
	r.closedAtDone = r.host.closedCount()
	r.mu.Unlock()
	close(r.done)
}

func (r *recorder) wait(t *testing.T) {
	t.Helper()
	select {
	case <-r.done:
	case <-time.After(2 * time.Second):
		t.Fatal("run did not finish within 2s")
	}
}

func TestRunSchedulerIdleRunsImmediately(t *testing.T) {
	h := &fakeHost{}
	rec := newRecorder(h)
	s := NewRunScheduler()

	pos, err := s.Submit(context.Background(), "k", "", "hello", func() (RunHost, error) { return h, nil }, rec)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if pos != 0 {
		t.Fatalf("idle submit position = %d, want 0", pos)
	}
	rec.wait(t)

	if got := h.prompts; len(got) != 1 || got[0] != "hello" {
		t.Fatalf("prompts = %v, want [hello]", got)
	}
	if rec.started != 1 {
		t.Fatalf("OnStart count = %d, want 1", rec.started)
	}
	if rec.turnDone != 1 || rec.doneErr != nil {
		t.Fatalf("turnDone=%d doneErr=%v, want 1 and nil", rec.turnDone, rec.doneErr)
	}
	if h.opened != 1 {
		t.Fatalf("opened=%d, want 1", h.opened)
	}
	if s.Busy("k") {
		t.Fatal("key still busy after run finished")
	}
}

func TestRunSchedulerQueuesWhileBusyAndDrainsInOrder(t *testing.T) {
	block := make(chan struct{})
	h := &fakeHost{runFn: func(ctx context.Context, label string) (RunResult, error) {
		if label == "A" {
			<-block // hold the first turn until B and C are queued
		}
		return RunResult{Reply: label}, nil
	}}
	rec := newRecorder(h)
	s := NewRunScheduler()

	if pos, _ := s.Submit(context.Background(), "k", "", "A", func() (RunHost, error) { return h, nil }, rec); pos != 0 {
		t.Fatalf("first submit position = %d, want 0", pos)
	}
	// The slot is reserved synchronously, so these queue rather than start runs.
	if pos, _ := s.Submit(context.Background(), "k", "", "B", func() (RunHost, error) { return h, nil }, rec); pos != 1 {
		t.Fatalf("B position = %d, want 1", pos)
	}
	if pos, _ := s.Submit(context.Background(), "k", "", "C", func() (RunHost, error) { return h, nil }, rec); pos != 2 {
		t.Fatalf("C position = %d, want 2", pos)
	}
	close(block)
	rec.wait(t)

	if got := h.prompts; len(got) != 3 || got[0] != "A" || got[1] != "B" || got[2] != "C" {
		t.Fatalf("prompts = %v, want [A B C]", got)
	}
	if len(rec.dequeued) != 2 || rec.dequeued[0] != "B" || rec.dequeued[1] != "C" {
		t.Fatalf("dequeued = %v, want [B C]", rec.dequeued)
	}
	if rec.turnDone != 3 {
		t.Fatalf("turnDone = %d, want 3", rec.turnDone)
	}
	if h.opened != 1 {
		t.Fatalf("opened = %d, want 1 (one session for the whole run)", h.opened)
	}
}

func TestRunSchedulerCancelDropsQueueAndStopsRun(t *testing.T) {
	block := make(chan struct{})
	h := &fakeHost{runFn: func(ctx context.Context, label string) (RunResult, error) {
		select {
		case <-ctx.Done():
			return RunResult{}, ctx.Err()
		case <-block:
			return RunResult{Reply: label}, nil
		}
	}}
	rec := newRecorder(h)
	s := NewRunScheduler()

	if _, err := s.Submit(context.Background(), "k", "", "A", func() (RunHost, error) { return h, nil }, rec); err != nil {
		t.Fatalf("submit A: %v", err)
	}
	if pos, _ := s.Submit(context.Background(), "k", "", "B", func() (RunHost, error) { return h, nil }, rec); pos != 1 {
		t.Fatalf("B position = %d, want 1", pos)
	}
	s.Cancel("k")
	rec.wait(t)

	if got := h.prompts; len(got) != 1 || got[0] != "A" {
		t.Fatalf("prompts = %v, want [A] (B must not run after cancel)", got)
	}
	if !errors.Is(rec.doneErr, context.Canceled) {
		t.Fatalf("doneErr = %v, want context.Canceled", rec.doneErr)
	}
	if len(rec.dequeued) != 0 {
		t.Fatalf("dequeued = %v, want none (queue dropped)", rec.dequeued)
	}
	if s.Busy("k") {
		t.Fatal("key still busy after cancel")
	}
	_ = block
}

func TestRunSchedulerClosesSessionBeforeDone(t *testing.T) {
	h := &fakeHost{}
	rec := newRecorder(h)
	s := NewRunScheduler()

	if _, err := s.Submit(context.Background(), "k", "", "hello", func() (RunHost, error) { return h, nil }, rec); err != nil {
		t.Fatalf("submit: %v", err)
	}
	rec.wait(t)

	if rec.closedAtDone < 1 {
		t.Fatalf("session close count at Done = %d, want >= 1 (released before the last callback)", rec.closedAtDone)
	}
}

func TestRunSchedulerOpenSessionFailureReleasesSlot(t *testing.T) {
	h := &fakeHost{openErr: errors.New("boom")}
	rec := newRecorder(h)
	s := NewRunScheduler()

	pos, err := s.Submit(context.Background(), "k", "", "hello", func() (RunHost, error) { return h, nil }, rec)
	if err == nil {
		t.Fatal("expected error when OpenSession fails")
	}
	if pos != 0 {
		t.Fatalf("position = %d, want 0 on error", pos)
	}
	if s.Busy("k") {
		t.Fatal("key still busy after a failed open; the slot leaked")
	}
}

func TestRunSchedulerAfterRunKeyIsReusable(t *testing.T) {
	h := &fakeHost{}
	rec1 := newRecorder(h)
	s := NewRunScheduler()
	if _, err := s.Submit(context.Background(), "k", "", "first", func() (RunHost, error) { return h, nil }, rec1); err != nil {
		t.Fatalf("submit first: %v", err)
	}
	rec1.wait(t)

	rec2 := newRecorder(h)
	if pos, err := s.Submit(context.Background(), "k", "", "second", func() (RunHost, error) { return h, nil }, rec2); err != nil || pos != 0 {
		t.Fatalf("second submit pos=%d err=%v, want 0 and nil", pos, err)
	}
	rec2.wait(t)
	if h.opened != 2 {
		t.Fatalf("opened = %d, want 2 (a session per run)", h.opened)
	}
}

func TestRunSchedulerStartEventRunsThenDrainsPrompts(t *testing.T) {
	h := &fakeHost{}
	rec := newRecorder(h)
	s := NewRunScheduler()

	popped := false
	pop := func() (BackgroundEvent, bool) {
		popped = true
		return BackgroundEvent{JobID: "job-1"}, true
	}
	started, err := s.StartEvent(context.Background(), "k", "sess", func() (RunHost, error) { return h, nil }, pop, rec)
	if err != nil || !started {
		t.Fatalf("StartEvent started=%v err=%v, want true and nil", started, err)
	}
	if !popped {
		t.Fatal("pop was not called on an idle key")
	}
	rec.wait(t)

	if got := h.events; len(got) != 1 || got[0] != "job-1" {
		t.Fatalf("events = %v, want [job-1]", got)
	}
	if rec.started != 1 || rec.turnDone != 1 {
		t.Fatalf("started=%d turnDone=%d, want 1 and 1", rec.started, rec.turnDone)
	}
}

func TestRunSchedulerStartEventDeclinesAndDoesNotPopWhenBusy(t *testing.T) {
	block := make(chan struct{})
	h := &fakeHost{runFn: func(ctx context.Context, label string) (RunResult, error) {
		<-block
		return RunResult{Reply: label}, nil
	}}
	rec := newRecorder(h)
	s := NewRunScheduler()

	// Occupy the key with a foreground run.
	if _, err := s.Submit(context.Background(), "k", "", "A", func() (RunHost, error) { return h, nil }, rec); err != nil {
		t.Fatalf("submit A: %v", err)
	}

	popped := false
	started, err := s.StartEvent(context.Background(), "k", "sess", func() (RunHost, error) { return h, nil }, func() (BackgroundEvent, bool) {
		popped = true
		return BackgroundEvent{JobID: "job-1"}, true
	}, newRecorder(h))
	if err != nil {
		t.Fatalf("StartEvent err = %v", err)
	}
	if started {
		t.Fatal("StartEvent started a second run on a busy key")
	}
	if popped {
		t.Fatal("StartEvent popped the event on a busy key; it must be left parked")
	}
	close(block)
	rec.wait(t)
}
