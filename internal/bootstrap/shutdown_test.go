package bootstrap

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	httpserver "github.com/gougoujiang/buildmax/internal/server"
)

// recordingServer records the ladder's calls in order.
type recordingServer struct {
	mu    sync.Mutex
	steps *[]string
	// shutdownBlocks makes Shutdown outlast its budget, the way an unfinished
	// request would.
	shutdownBlocks bool
}

func (r *recordingServer) record(step string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	*r.steps = append(*r.steps, step)
}

func (r *recordingServer) Drain() { r.record("drain") }

func (r *recordingServer) Shutdown(ctx context.Context) error {
	r.record("shutdown")
	if r.shutdownBlocks {
		<-ctx.Done()
		return ctx.Err()
	}
	return nil
}

func (r *recordingServer) StopBackground(context.Context) { r.record("stop-background") }

// TestShutdownLadderStopsTheSchedulerBeforeTheListener is the ordering this
// whole design turns on. A worker reports its outcome over the server's own
// API, so closing the listener first is what used to leave a finished run
// unable to say so.
func TestShutdownLadderStopsTheSchedulerBeforeTheListener(t *testing.T) {
	var steps []string
	srv := &recordingServer{steps: &steps}
	targets := shutdownTargets{
		server:    srv,
		scheduler: namedStop{name: "scheduler", stop: func() { srv.record("stop-scheduler") }},
		loops: []namedStop{
			{name: "reaper", stop: func() { srv.record("stop-reaper") }},
			{name: "cleaner", stop: func() { srv.record("stop-cleaner") }},
		},
	}

	shutdownServer(context.Background(), targets, httpserver.NewShutdownBudget(2*time.Second))

	want := []string{"drain", "stop-scheduler", "shutdown", "stop-background", "stop-reaper", "stop-cleaner"}
	if !slices.Equal(steps, want) {
		t.Fatalf("ladder ran %v, want %v", steps, want)
	}
}

// A component that never stops must not hold the process open. The design
// prefers losing a little work to a shutdown that hangs.
func TestShutdownLadderIsBoundedByItsBudget(t *testing.T) {
	var steps []string
	srv := &recordingServer{steps: &steps, shutdownBlocks: true}
	blocked := make(chan struct{})
	defer close(blocked)

	targets := shutdownTargets{
		server: srv,
		scheduler: namedStop{name: "scheduler", stop: func() {
			srv.record("stop-scheduler")
			<-blocked // a local worker still running its task
		}},
		loops: []namedStop{{name: "reaper", stop: func() { srv.record("stop-reaper") }}},
	}

	budget := httpserver.NewShutdownBudget(2 * time.Second)
	start := time.Now()
	shutdownServer(context.Background(), targets, budget)
	elapsed := time.Since(start)

	if elapsed > budget.Total()*3 {
		t.Fatalf("shutdown took %v, far past its %v budget", elapsed, budget.Total())
	}
	// Everything below the blocked rung still ran.
	if !slices.Contains(steps, "shutdown") || !slices.Contains(steps, "stop-reaper") {
		t.Errorf("ladder stopped at the blocked component: %v", steps)
	}
}

// A shutdown whose own context is already cancelled — a second Ctrl-C —
// abandons the ladder rather than waiting out every phase.
func TestShutdownLadderGivesUpOnACancelledContext(t *testing.T) {
	var steps []string
	srv := &recordingServer{steps: &steps}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	targets := shutdownTargets{
		server:    srv,
		scheduler: namedStop{name: "scheduler", stop: func() { srv.record("stop-scheduler") }},
	}

	start := time.Now()
	shutdownServer(ctx, targets, httpserver.NewShutdownBudget(time.Hour))
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("cancelled shutdown took %v", elapsed)
	}
}

func TestStopWithinReturnsAtItsLimit(t *testing.T) {
	never := make(chan struct{})
	defer close(never)

	start := time.Now()
	stopWithin(context.Background(), 100*time.Millisecond, "test", func() { <-never })
	elapsed := time.Since(start)

	if elapsed < 100*time.Millisecond {
		t.Errorf("stopWithin returned after %v, before its limit", elapsed)
	}
	if elapsed > 2*time.Second {
		t.Errorf("stopWithin waited %v past its limit", elapsed)
	}
}
