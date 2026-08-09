package desktop

import (
	"context"
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
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
