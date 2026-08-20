package desktop

import (
	"context"
	"errors"
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/agent"
)

func TestApp_Startup_sets_data_dir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BUILDMAX_HOME", dir)
	app := NewApp()
	ctx := context.Background()
	app.Startup(ctx)
	got := config.DataDir()
	if got != dir {
		t.Errorf("DataDir() = %q, want %q", got, dir)
	}
}

func TestApp_Shutdown_noop(t *testing.T) {
	app := NewApp()
	app.Shutdown(context.Background())
}

func TestApp_CancelRun_requires_project_id(t *testing.T) {
	app := NewApp()
	if err := app.CancelRun(""); err == nil {
		t.Fatal("CancelRun(\"\") = nil, want error")
	}
}

func TestApp_CancelRun_no_inflight_run_is_noop(t *testing.T) {
	app := NewApp()
	if err := app.CancelRun("p_does_not_exist"); err != nil {
		t.Fatalf("CancelRun on idle project: %v", err)
	}
}

func TestApp_CancelRun_cancels_registered_context(t *testing.T) {
	app := NewApp()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Simulate an in-flight run by registering a cancel func, then verify
	// CancelRun fires it and clears the entry. This mirrors what
	// SendMessageStream does without spinning up an AgentApp.
	runCtx, runCancel := context.WithCancel(ctx)
	app.mu.Lock()
	app.runCancels["p_test"] = runCancel
	app.mu.Unlock()

	if err := app.CancelRun("p_test"); err != nil {
		t.Fatalf("CancelRun: %v", err)
	}
	if runCtx.Err() == nil {
		t.Fatal("run context not cancelled after CancelRun")
	}

	// CancelRun does not remove the entry — the run goroutine's defer does.
	// Verify a second CancelRun on the still-registered entry remains a no-op
	// (idempotent) by simply not panicking and returning nil.
	if err := app.CancelRun("p_test"); err != nil {
		t.Fatalf("second CancelRun: %v", err)
	}
}

// A prompt sent while a run is in flight is queued rather than rejected. The run is
// simulated the same way the cancel tests do it: registering a cancel func is what
// "busy" means to SendMessageStream.
func TestApp_SendMessageStream_queues_while_busy(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BUILDMAX_HOME", dir)
	app := NewApp()
	app.Startup(context.Background())

	_, runCancel := context.WithCancel(context.Background())
	t.Cleanup(runCancel)
	app.mu.Lock()
	app.runCancels["p_busy"] = runCancel
	app.mu.Unlock()

	pos, err := app.SendMessageStream("p_busy", "", "follow-up")
	if err != nil {
		t.Fatalf("SendMessageStream while busy: %v", err)
	}
	if pos != 1 {
		t.Errorf("first queued position = %d, want 1", pos)
	}
	pos, err = app.SendMessageStream("p_busy", "", "and another")
	if err != nil {
		t.Fatalf("second SendMessageStream while busy: %v", err)
	}
	if pos != 2 {
		t.Errorf("second queued position = %d, want 2", pos)
	}

	got := app.QueuedMessages("p_busy")
	want := []string{"follow-up", "and another"}
	if len(got) != len(want) {
		t.Fatalf("QueuedMessages = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("QueuedMessages[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// Stopping a run discards what was queued behind it: those prompts were written for
// work the user just called off.
func TestApp_CancelRun_drops_queued_messages(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BUILDMAX_HOME", dir)
	app := NewApp()
	app.Startup(context.Background())

	_, runCancel := context.WithCancel(context.Background())
	t.Cleanup(runCancel)
	app.mu.Lock()
	app.runCancels["p_busy"] = runCancel
	app.mu.Unlock()

	if _, err := app.SendMessageStream("p_busy", "", "queued behind the run"); err != nil {
		t.Fatalf("SendMessageStream while busy: %v", err)
	}
	if err := app.CancelRun("p_busy"); err != nil {
		t.Fatalf("CancelRun: %v", err)
	}
	if got := app.QueuedMessages("p_busy"); len(got) != 0 {
		t.Errorf("QueuedMessages after cancel = %v, want empty", got)
	}
}

func TestApp_SendMessageStream_queue_has_a_cap(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BUILDMAX_HOME", dir)
	app := NewApp()
	app.Startup(context.Background())

	_, runCancel := context.WithCancel(context.Background())
	t.Cleanup(runCancel)
	app.mu.Lock()
	app.runCancels["p_busy"] = runCancel
	app.mu.Unlock()

	for i := 0; i < agent.DefaultMaxQueuedMessages; i++ {
		if _, err := app.SendMessageStream("p_busy", "", "filler"); err != nil {
			t.Fatalf("SendMessageStream #%d: %v", i, err)
		}
	}
	_, err := app.SendMessageStream("p_busy", "", "one too many")
	if err == nil {
		t.Fatal("SendMessageStream past the cap = nil, want error")
	}
	if !errors.Is(err, agent.ErrQueueFull) {
		t.Errorf("error = %v, want it to wrap ErrQueueFull", err)
	}
}
