package agent

import (
	"errors"
	"sync"
	"testing"
)

func TestMessageQueueFIFO(t *testing.T) {
	q := NewMessageQueue(0)
	for _, text := range []string{"first", "second", "third"} {
		if _, err := q.Enqueue(text); err != nil {
			t.Fatalf("Enqueue(%q): %v", text, err)
		}
	}
	if q.Len() != 3 {
		t.Fatalf("Len = %d, want 3", q.Len())
	}
	for _, want := range []string{"first", "second", "third"} {
		got, ok := q.Dequeue()
		if !ok || got != want {
			t.Fatalf("Dequeue = %q, %v; want %q, true", got, ok, want)
		}
	}
	if _, ok := q.Dequeue(); ok {
		t.Error("Dequeue on empty queue returned ok")
	}
}

func TestMessageQueueEnqueueReportsPosition(t *testing.T) {
	q := NewMessageQueue(0)
	for i, want := range []int{1, 2, 3} {
		got, err := q.Enqueue("m")
		if err != nil {
			t.Fatalf("Enqueue #%d: %v", i, err)
		}
		if got != want {
			t.Errorf("position = %d, want %d", got, want)
		}
	}
}

// A full queue rejects rather than evicting: dropping the oldest message would
// silently lose a turn the user already believes is scheduled.
func TestMessageQueueFullRejects(t *testing.T) {
	q := NewMessageQueue(2)
	if _, err := q.Enqueue("a"); err != nil {
		t.Fatalf("Enqueue a: %v", err)
	}
	if _, err := q.Enqueue("b"); err != nil {
		t.Fatalf("Enqueue b: %v", err)
	}
	if _, err := q.Enqueue("c"); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("Enqueue c error = %v, want ErrQueueFull", err)
	}
	if q.Len() != 2 {
		t.Errorf("Len = %d, want 2", q.Len())
	}
	if got, _ := q.Dequeue(); got != "a" {
		t.Errorf("head = %q, want %q", got, "a")
	}
}

func TestMessageQueueDropLast(t *testing.T) {
	q := NewMessageQueue(0)
	_, _ = q.Enqueue("keep")
	_, _ = q.Enqueue("undo me")
	got, ok := q.DropLast()
	if !ok || got != "undo me" {
		t.Fatalf("DropLast = %q, %v; want %q, true", got, ok, "undo me")
	}
	if q.Len() != 1 {
		t.Errorf("Len = %d, want 1", q.Len())
	}
	if _, _ = q.DropLast(); q.Len() != 0 {
		t.Errorf("Len after second DropLast = %d, want 0", q.Len())
	}
	if _, ok := q.DropLast(); ok {
		t.Error("DropLast on empty queue returned ok")
	}
}

func TestMessageQueueDropClears(t *testing.T) {
	q := NewMessageQueue(0)
	_, _ = q.Enqueue("a")
	_, _ = q.Enqueue("b")
	if n := q.Drop(); n != 2 {
		t.Errorf("Drop = %d, want 2", n)
	}
	if q.Len() != 0 {
		t.Errorf("Len = %d, want 0", q.Len())
	}
	if n := q.Drop(); n != 0 {
		t.Errorf("Drop on empty = %d, want 0", n)
	}
}

func TestMessageQueueSnapshotIsACopy(t *testing.T) {
	q := NewMessageQueue(0)
	_, _ = q.Enqueue("a")
	_, _ = q.Enqueue("b")
	snap := q.Snapshot()
	snap[0] = "mutated"
	if got := q.Snapshot(); got[0] != "a" {
		t.Errorf("queue head = %q, want %q — Snapshot must not alias the queue", got[0], "a")
	}
	if NewMessageQueue(0).Snapshot() != nil {
		t.Error("Snapshot of empty queue should be nil")
	}
}

func TestMessageQueueConcurrentAccess(t *testing.T) {
	q := NewMessageQueue(100)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = q.Enqueue("m")
		}()
	}
	wg.Wait()
	if q.Len() != 50 {
		t.Errorf("Len = %d, want 50", q.Len())
	}
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			q.Dequeue()
		}()
	}
	wg.Wait()
	if q.Len() != 0 {
		t.Errorf("Len = %d, want 0", q.Len())
	}
}
