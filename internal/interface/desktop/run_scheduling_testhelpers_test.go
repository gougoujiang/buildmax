package desktop

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/icloudbb/buildmax/internal/agentapp"
)

// blockingHost is a RunHost whose turn blocks until released or its context is
// cancelled, so a test can hold a project's run slot busy without a model.
type blockingHost struct {
	release chan struct{}
	ctxCh   chan context.Context
}

func (h *blockingHost) OpenSession(string) (*agentapp.SessionContext, error) {
	return &agentapp.SessionContext{}, nil
}
func (h *blockingHost) CloseSession(*agentapp.SessionContext) {}

func (h *blockingHost) RunPrompt(ctx context.Context, _ *agentapp.SessionContext, _ string, _ agentapp.RunPromptOpts) (agentapp.RunResult, error) {
	select {
	case h.ctxCh <- ctx:
	default:
	}
	select {
	case <-ctx.Done():
		return agentapp.RunResult{}, ctx.Err()
	case <-h.release:
		return agentapp.RunResult{}, nil
	}
}

func (h *blockingHost) RunBackgroundEvent(ctx context.Context, s *agentapp.SessionContext, _ agentapp.BackgroundEvent, o agentapp.RunPromptOpts) (agentapp.RunResult, error) {
	return h.RunPrompt(ctx, s, "", o)
}

// noopLifecycle is a RunLifecycle that ignores every callback.
type noopLifecycle struct{}

func (noopLifecycle) RunOpts() agentapp.RunPromptOpts  { return agentapp.RunPromptOpts{} }
func (noopLifecycle) OnStart(*agentapp.SessionContext) {}
func (noopLifecycle) TurnDone(agentapp.RunResult)      {}
func (noopLifecycle) TurnError(error)                  {}
func (noopLifecycle) Dequeued(string, []string)        {}
func (noopLifecycle) Done(agentapp.RunResult, error)   {}

// occupyProject holds key's run slot busy with a blocking fake run and returns
// that run's context plus a release func. The run is released at test cleanup if
// the test did not release it. It needs no AgentApp: SendMessageStream on a busy
// key only enqueues, and the scheduler never resolves the host.
func occupyProject(t *testing.T, app *App, key string) (context.Context, func()) {
	t.Helper()
	host := &blockingHost{release: make(chan struct{}), ctxCh: make(chan context.Context, 1)}
	host2 := host
	if _, err := app.scheduler.Submit(context.Background(), key, "", "occupy", func() (agentapp.RunHost, error) { return host2, nil }, noopLifecycle{}); err != nil {
		t.Fatalf("occupy %s: %v", key, err)
	}
	var runCtx context.Context
	select {
	case runCtx = <-host.ctxCh:
	case <-time.After(2 * time.Second):
		t.Fatal("occupying run did not start")
	}
	var once sync.Once
	release := func() { once.Do(func() { close(host.release) }) }
	t.Cleanup(release)
	return runCtx, release
}

// waitNotBusy blocks until key has no run in flight.
func waitNotBusy(t *testing.T, app *App, key string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for app.scheduler.Busy(key) {
		if time.Now().After(deadline) {
			t.Fatalf("run slot for %s never released", key)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
