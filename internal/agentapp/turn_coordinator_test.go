package agentapp

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestTurnCoordinatorRefusesOverlapPerSession(t *testing.T) {
	var c turnCoordinator
	if err := c.begin("s1"); err != nil {
		t.Fatalf("first begin: %v", err)
	}
	if err := c.begin("s1"); !errors.Is(err, ErrTurnActive) {
		t.Fatalf("second begin = %v, want ErrTurnActive", err)
	}
	if err := c.begin("s2"); err != nil {
		t.Fatalf("other session blocked: %v", err)
	}
	c.end("s1")
	if err := c.begin("s1"); err != nil {
		t.Fatalf("begin after end: %v", err)
	}
}

// Under concurrency exactly one caller may hold a session at a time; run with
// the race detector to also prove the map itself is safe.
func TestTurnCoordinatorConcurrent(t *testing.T) {
	var c turnCoordinator
	const attempts = 100
	var wg sync.WaitGroup
	var mu sync.Mutex
	holders := 0
	maxHolders := 0
	for range attempts {
		wg.Go(func() {
			if err := c.begin("s"); err != nil {
				return
			}
			mu.Lock()
			holders++
			if holders > maxHolders {
				maxHolders = holders
			}
			mu.Unlock()
			mu.Lock()
			holders--
			mu.Unlock()
			c.end("s")
		})
	}
	wg.Wait()
	if maxHolders != 1 {
		t.Fatalf("max concurrent holders = %d, want 1", maxHolders)
	}
	if err := c.begin("s"); err != nil {
		t.Fatalf("session left held after all ends: %v", err)
	}
}

// A second RunPrompt against a session with an active turn is refused before
// any history write, not raced.
func TestRunPromptRefusesConcurrentTurn(t *testing.T) {
	app := makeAgentAppForHookTests(t)
	sess, err := app.OpenSession("")
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	if err := app.turns.begin(sess.ID); err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer app.turns.end(sess.ID)

	before := len(sess.Messages)
	_, err = app.RunPrompt(context.Background(), sess, "hello", RunPromptOpts{})
	if !errors.Is(err, ErrTurnActive) {
		t.Fatalf("RunPrompt = %v, want ErrTurnActive", err)
	}
	if len(sess.Messages) != before {
		t.Errorf("refused turn wrote %d messages to history", len(sess.Messages)-before)
	}
}
