package handlers

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// blockedJob returns a job that parks until release is closed, plus a channel that
// is closed once the job has actually started running.
func blockedJob(started chan<- struct{}, release <-chan struct{}) *turnJob {
	return newTurnJob(func() {
		close(started)
		<-release
	})
}

func TestTurnRegistrySecondTurnQueuesBehindTheFirst(t *testing.T) {
	r := newTurnRegistry()
	started := make(chan struct{})
	release := make(chan struct{})

	if pos, err := r.Submit("v_1", blockedJob(started, release)); err != nil || pos != 0 {
		t.Fatalf("first Submit = %d, %v; want 0, nil", pos, err)
	}
	<-started

	var ran []string
	var mu sync.Mutex
	record := func(name string) *turnJob {
		return newTurnJob(func() {
			mu.Lock()
			ran = append(ran, name)
			mu.Unlock()
		})
	}

	second := record("second")
	pos, err := r.Submit("v_1", second)
	if err != nil || pos != 1 {
		t.Fatalf("second Submit = %d, %v; want 1, nil", pos, err)
	}
	third := record("third")
	if pos, err := r.Submit("v_1", third); err != nil || pos != 2 {
		t.Fatalf("third Submit = %d, %v; want 2, nil", pos, err)
	}
	if got := r.Waiting("v_1"); got != 2 {
		t.Errorf("Waiting = %d, want 2", got)
	}

	close(release)
	<-third.done

	mu.Lock()
	defer mu.Unlock()
	if len(ran) != 2 || ran[0] != "second" || ran[1] != "third" {
		t.Errorf("run order = %v, want [second third]", ran)
	}
}

// A turn in another conversation is not held up by a busy one.
func TestTurnRegistryConversationsAreIndependent(t *testing.T) {
	r := newTurnRegistry()
	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	if _, err := r.Submit("v_busy", blockedJob(started, release)); err != nil {
		t.Fatalf("Submit busy: %v", err)
	}
	<-started

	other := newTurnJob(func() {})
	if pos, err := r.Submit("v_other", other); err != nil || pos != 0 {
		t.Fatalf("Submit other = %d, %v; want 0, nil", pos, err)
	}
	select {
	case <-other.done:
	case <-time.After(2 * time.Second):
		t.Fatal("a turn in another conversation was blocked by a busy one")
	}
}

func TestTurnRegistryOnDequeueFiresOnlyForWaitingTurns(t *testing.T) {
	r := newTurnRegistry()
	started := make(chan struct{})
	release := make(chan struct{})

	first := blockedJob(started, release)
	firstDequeued := false
	first.onDequeue = func() { firstDequeued = true }
	if _, err := r.Submit("v_1", first); err != nil {
		t.Fatalf("Submit first: %v", err)
	}
	<-started

	second := newTurnJob(func() {})
	dequeued := make(chan struct{})
	second.onDequeue = func() { close(dequeued) }
	if _, err := r.Submit("v_1", second); err != nil {
		t.Fatalf("Submit second: %v", err)
	}

	close(release)
	<-second.done
	select {
	case <-dequeued:
	default:
		t.Error("a turn that waited should report starting via onDequeue")
	}
	if firstDequeued {
		t.Error("a turn that started immediately should not report a dequeue")
	}
}

func TestTurnRegistryRejectsPastTheCap(t *testing.T) {
	r := newTurnRegistry()
	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	if _, err := r.Submit("v_1", blockedJob(started, release)); err != nil {
		t.Fatalf("Submit running turn: %v", err)
	}
	<-started

	for i := 0; i < maxQueuedTurns; i++ {
		if _, err := r.Submit("v_1", newTurnJob(func() {})); err != nil {
			t.Fatalf("Submit #%d: %v", i, err)
		}
	}
	if _, err := r.Submit("v_1", newTurnJob(func() {})); !errors.Is(err, errTurnQueueFull) {
		t.Fatalf("Submit past the cap = %v, want errTurnQueueFull", err)
	}
	if got := r.Waiting("v_1"); got != maxQueuedTurns {
		t.Errorf("Waiting = %d, want %d", got, maxQueuedTurns)
	}
}

func TestTurnRegistryRunSyncWaitsForItsTurn(t *testing.T) {
	r := newTurnRegistry()
	started := make(chan struct{})
	release := make(chan struct{})
	if _, err := r.Submit("v_1", blockedJob(started, release)); err != nil {
		t.Fatalf("Submit running turn: %v", err)
	}
	<-started

	ran := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- r.RunSync(context.Background(), "v_1", func() { close(ran) })
	}()

	select {
	case <-ran:
		t.Fatal("RunSync ran its turn while the conversation was busy")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("RunSync: %v", err)
	}
	select {
	case <-ran:
	default:
		t.Error("RunSync returned without running its turn")
	}
}

// A caller that goes away before its turn starts must not have its turn run: the
// stream it would be written to is gone.
func TestTurnRegistryRunSyncDropsOnCallerCancel(t *testing.T) {
	r := newTurnRegistry()
	started := make(chan struct{})
	release := make(chan struct{})
	if _, err := r.Submit("v_1", blockedJob(started, release)); err != nil {
		t.Fatalf("Submit running turn: %v", err)
	}
	<-started

	ctx, cancel := context.WithCancel(context.Background())
	var ran bool
	var mu sync.Mutex
	done := make(chan error, 1)
	go func() {
		done <- r.RunSync(ctx, "v_1", func() {
			mu.Lock()
			ran = true
			mu.Unlock()
		})
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()

	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("RunSync after cancel = %v, want context.Canceled", err)
	}

	// Let the queue drain past the dropped job.
	tail := newTurnJob(func() {})
	if _, err := r.Submit("v_1", tail); err != nil {
		t.Fatalf("Submit tail: %v", err)
	}
	close(release)
	<-tail.done

	mu.Lock()
	defer mu.Unlock()
	if ran {
		t.Error("a dropped turn should not run")
	}
}

func TestTurnRegistryForgetsIdleConversations(t *testing.T) {
	r := newTurnRegistry()
	job := newTurnJob(func() {})
	if _, err := r.Submit("v_1", job); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	<-job.done

	deadline := time.Now().Add(2 * time.Second)
	for {
		r.mu.Lock()
		n := len(r.queues)
		r.mu.Unlock()
		if n == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("idle conversation queue was not released, %d still held", n)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
