package agent

import (
	"errors"
	"sync"
)

// DefaultMaxQueuedMessages caps how many messages one queue holds. The cap exists
// so a user holding down enter during a long run cannot commit the runtime to an
// unbounded backlog of turns it will still be working through minutes later.
const DefaultMaxQueuedMessages = 10

// ErrQueueFull is returned by Enqueue when the queue is at its cap.
var ErrQueueFull = errors.New("message queue full")

// MessageQueue holds user messages that arrived while a run was in flight, in the
// order they were submitted. Surfaces (CLI/TUI, Desktop, Portal) own one queue per
// conversation and drain it at their turn boundary; the queue itself has no opinion
// about when that is.
//
// A cancelled run drops the queue: the messages behind it were written for a run
// the user has since abandoned, and delivering them afterwards would be a surprise.
// Callers do that explicitly through Drop.
//
// Safe for concurrent use.
type MessageQueue struct {
	mu    sync.Mutex
	items []string
	max   int
}

// NewMessageQueue returns an empty queue holding at most max messages.
// A max of zero or less uses DefaultMaxQueuedMessages.
func NewMessageQueue(max int) *MessageQueue {
	if max <= 0 {
		max = DefaultMaxQueuedMessages
	}
	return &MessageQueue{max: max}
}

// Enqueue appends text and returns its 1-based position in the queue.
// It returns ErrQueueFull when the queue is at its cap, leaving the queue unchanged.
func (q *MessageQueue) Enqueue(text string) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) >= q.max {
		return 0, ErrQueueFull
	}
	q.items = append(q.items, text)
	return len(q.items), nil
}

// Dequeue removes and returns the oldest message. The bool is false when empty.
func (q *MessageQueue) Dequeue() (string, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return "", false
	}
	text := q.items[0]
	q.items = q.items[1:]
	return text, true
}

// DropLast removes and returns the most recently queued message, which is what an
// "undo" on the input reaches for. The bool is false when empty.
func (q *MessageQueue) DropLast() (string, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return "", false
	}
	last := len(q.items) - 1
	text := q.items[last]
	q.items = q.items[:last]
	return text, true
}

// Drop clears the queue and returns how many messages were discarded.
func (q *MessageQueue) Drop() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	n := len(q.items)
	q.items = nil
	return n
}

// Len returns how many messages are waiting.
func (q *MessageQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

// Snapshot returns a copy of the waiting messages, oldest first, for display.
func (q *MessageQueue) Snapshot() []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return nil
	}
	out := make([]string, len(q.items))
	copy(out, q.items)
	return out
}
