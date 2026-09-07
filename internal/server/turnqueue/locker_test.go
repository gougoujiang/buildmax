package turnqueue

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeLease records its release.
type fakeLease struct {
	fence    int64
	released *bool
	mu       *sync.Mutex
}

func (l *fakeLease) Fence() int64 { return l.fence }
func (l *fakeLease) Release() {
	l.mu.Lock()
	*l.released = true
	l.mu.Unlock()
}

// fakeLocker records acquisitions and can be made to fail.
type fakeLocker struct {
	mu        sync.Mutex
	acquired  []string
	released  bool
	failWith  error
	nextFence int64
}

func (f *fakeLocker) Acquire(ctx context.Context, conversationID string) (Lease, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failWith != nil {
		return nil, f.failWith
	}
	f.acquired = append(f.acquired, conversationID)
	f.nextFence++
	return &fakeLease{fence: f.nextFence, released: &f.released, mu: &f.mu}, nil
}

func TestRunLockedAcquiresAndReleasesAroundTheTurn(t *testing.T) {
	locker := &fakeLocker{}
	r := NewRegistry(locker)

	ran := make(chan struct{})
	if _, err := r.Submit("conv_1", NewJob(func() { close(ran) })); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	select {
	case <-ran:
	case <-time.After(time.Second):
		t.Fatal("turn did not run under a locker")
	}

	// The turn ran, so the lease was acquired for that conversation and released
	// after — Release happens in a defer, so give the goroutine a moment.
	time.Sleep(50 * time.Millisecond)
	locker.mu.Lock()
	defer locker.mu.Unlock()
	if len(locker.acquired) != 1 || locker.acquired[0] != "conv_1" {
		t.Fatalf("acquired = %v, want [conv_1]", locker.acquired)
	}
	if !locker.released {
		t.Fatal("the conversation lease was not released after the turn")
	}
}

func TestRunLockedSkipsTheTurnWhenTheLeaseCannotBeAcquired(t *testing.T) {
	locker := &fakeLocker{failWith: errors.New("redis down")}
	r := NewRegistry(locker)

	ran := false
	job := NewJob(func() { ran = true })
	if _, err := r.Submit("conv_1", job); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	// Done still closes so a waiter unblocks, even though the turn did not run.
	select {
	case <-job.Done:
	case <-time.After(time.Second):
		t.Fatal("job.Done never closed after a failed lease acquisition")
	}
	if ran {
		t.Fatal("the turn ran without holding the conversation lease")
	}
}
