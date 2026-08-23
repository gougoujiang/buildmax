package runterminal

import (
	"context"
	"testing"
	"time"
)

func TestGroupWaitsForWorkInFlight(t *testing.T) {
	g := NewGroup()
	release := make(chan struct{})
	done := make(chan struct{})

	g.Go(func() {
		<-release
		close(done)
	})

	short, cancelShort := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancelShort()
	if g.Wait(short) {
		t.Fatal("Wait returned true while a callback was still running")
	}

	close(release)
	<-done

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if !g.Wait(ctx) {
		t.Fatal("Wait did not observe the finished callback")
	}
}

// After the group closes, a late callback runs inline rather than being
// dropped: the caller is a worker's final report, and losing a workflow advance
// because the timing was unlucky is worse than making that report wait.
func TestGroupRunsLateWorkInline(t *testing.T) {
	g := NewGroup()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if !g.Wait(ctx) {
		t.Fatal("Wait blocked on an empty group")
	}

	ran := false
	g.Go(func() { ran = true })
	if !ran {
		t.Error("a callback submitted after Wait was neither run inline nor waited for")
	}
}

// A nil group is what a test that never shuts down has, and must still run the
// callback.
func TestNilGroupStillRuns(t *testing.T) {
	var g *Group
	done := make(chan struct{})
	g.Go(func() { close(done) })
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("nil group did not run the callback")
	}
	if !g.Wait(context.Background()) {
		t.Error("nil group Wait = false")
	}
}
