package streamhub

import (
	"testing"
)

func TestMemStreamHub_AppendBufferDone(t *testing.T) {
	hub := NewStreamHub()
	runID := "r_abc123"

	if got := hub.Buffer(runID); got != "" {
		t.Errorf("Buffer empty run: got %q, want \"\"", got)
	}

	hub.Append(runID, "hello ")
	hub.Append(runID, "world")
	if got := hub.Buffer(runID); got != "hello world" {
		t.Errorf("Buffer after appends: got %q, want \"hello world\"", got)
	}

	hub.Done(runID)
	if got := hub.Buffer(runID); got != "" {
		t.Errorf("Buffer after Done: got %q, want \"\"", got)
	}
}

func TestMemStreamHub_EmptyDeltaIgnored(t *testing.T) {
	hub := NewStreamHub()
	runID := "r_xyz"
	hub.Append(runID, "")
	hub.Append(runID, "a")
	if got := hub.Buffer(runID); got != "a" {
		t.Errorf("Buffer: got %q, want \"a\"", got)
	}
}

func TestMemStreamHub_SubscribeReceivesDeltasAndDone(t *testing.T) {
	hub := NewStreamHub()
	runID := "r_sub"
	events, unsub := hub.Subscribe(runID)
	defer unsub()

	hub.Append(runID, "one ")
	hub.Append(runID, "two ")
	var received []string
	go func() {
		hub.Append(runID, "three")
		hub.Done(runID)
	}()
	for msg := range events {
		received = append(received, msg)
		if msg == StreamEventDone {
			break
		}
	}

	want := []string{"one ", "two ", "three", StreamEventDone}
	if len(received) != len(want) {
		t.Errorf("received %d events, want %d: %v", len(received), len(want), received)
	} else {
		for i := range want {
			if received[i] != want[i] {
				t.Errorf("received[%d] = %q, want %q", i, received[i], want[i])
			}
		}
	}
}
