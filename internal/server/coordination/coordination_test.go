package coordination

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	infra "github.com/gougoujiang/buildmax/internal/infra/coordination"
	wsconn "github.com/gougoujiang/buildmax/internal/server/websocket"
)

// twoBackends returns two independent backends over one shared Redis, standing
// in for two server replicas.
func twoBackends(t *testing.T) (*infra.Backend, *infra.Backend) {
	t.Helper()
	mr := miniredis.RunT(t)
	a, err := infra.New(context.Background(), infra.Options{Address: mr.Addr()})
	if err != nil {
		t.Fatalf("New a: %v", err)
	}
	b, err := infra.New(context.Background(), infra.Options{Address: mr.Addr()})
	if err != nil {
		t.Fatalf("New b: %v", err)
	}
	t.Cleanup(func() { _ = a.Close(); _ = b.Close() })
	return a, b
}

func TestStreamHubDeliversAcrossReplicas(t *testing.T) {
	a, b := twoBackends(t)
	ctx := context.Background()
	hubA := NewStreamHub(ctx, a)
	hubB := NewStreamHub(ctx, b)

	events, unsub := hubB.Subscribe("task1")
	defer unsub()
	time.Sleep(50 * time.Millisecond) // let the tail read the current end

	hubA.Append("task1", "delta-from-A")

	select {
	case got := <-events:
		if got != "delta-from-A" {
			t.Fatalf("got %q, want delta-from-A", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a delta appended on replica A never reached a subscriber on replica B")
	}

	hubA.Done("task1")
	select {
	case got := <-events:
		if got != wsconn.StreamEventDone {
			t.Fatalf("got %q, want the done sentinel", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Done on replica A never reached the subscriber on replica B")
	}
}

func TestEventBusDeliversAcrossReplicas(t *testing.T) {
	a, b := twoBackends(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	busA := NewEventBus(ctx, a)
	busB := NewEventBus(ctx, b)
	time.Sleep(50 * time.Millisecond) // let B's subscription register

	busA.PublishEvent([]byte("event-from-A"))

	select {
	case got := <-busB.Incoming():
		if string(got) != "event-from-A" {
			t.Fatalf("got %q, want event-from-A", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("an event published on replica A never reached replica B's incoming")
	}
}

func TestTurnLockerSerializesAcrossReplicas(t *testing.T) {
	a, b := twoBackends(t)
	lockerA := NewTurnLocker(a)
	lockerB := NewTurnLocker(b)
	ctx := context.Background()

	lease, err := lockerA.Acquire(ctx, "conv-1")
	if err != nil {
		t.Fatalf("acquire on A: %v", err)
	}

	acquired := make(chan struct{}, 1)
	go func() {
		l, err := lockerB.Acquire(ctx, "conv-1")
		if err != nil {
			return
		}
		l.Release()
		acquired <- struct{}{}
	}()
	select {
	case <-acquired:
		t.Fatal("replica B ran a turn for a conversation replica A was still running")
	case <-time.After(300 * time.Millisecond):
	}

	lease.Release()
	select {
	case <-acquired:
	case <-time.After(2 * time.Second):
		t.Fatal("replica B never got the conversation lease after A released it")
	}
}
