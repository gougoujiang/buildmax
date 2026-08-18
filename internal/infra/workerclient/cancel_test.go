package workerclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// cancelServer answers the worker's run route, reporting a cancel only after
// the given number of polls.
func cancelServer(t *testing.T, cancelAfter int32, polls *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := polls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if cancelAfter > 0 && n >= cancelAfter {
			_, _ = w.Write([]byte(`{"run":{"task_run_id":"r1","status":"RUNNING","cancel_requested":true},"task":{}}`))
			return
		}
		_, _ = w.Write([]byte(`{"run":{"task_run_id":"r1","status":"RUNNING"},"task":{}}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestIsCancelRequestedReadsTheRunsFlag(t *testing.T) {
	var polls atomic.Int32
	srv := cancelServer(t, 2, &polls)
	cfg := WorkerAPIClientConfig{BaseURL: srv.URL, Token: "run-token"}

	first, err := IsCancelRequested(context.Background(), cfg, "r1")
	if err != nil {
		t.Fatalf("IsCancelRequested: %v", err)
	}
	if first {
		t.Error("a run nobody canceled reports a cancel")
	}
	second, err := IsCancelRequested(context.Background(), cfg, "r1")
	if err != nil {
		t.Fatalf("IsCancelRequested: %v", err)
	}
	if !second {
		t.Error("a canceled run does not report its cancel")
	}
}

// A run that no longer exists is not a reason to stop mid-turn: the worker is
// executing real work and a 404 says nothing about whether it should.
func TestIsCancelRequestedTreatsAMissingRunAsNoCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	got, err := IsCancelRequested(context.Background(), WorkerAPIClientConfig{BaseURL: srv.URL}, "r1")
	if err != nil || got {
		t.Errorf("IsCancelRequested = %v, %v; want false, nil", got, err)
	}
}

func TestWatchCancelFiresOnceWhenTheRunIsCanceled(t *testing.T) {
	var polls atomic.Int32
	srv := cancelServer(t, 2, &polls)

	fired := make(chan struct{}, 4)
	go WatchCancel(context.Background(), WorkerAPIClientConfig{BaseURL: srv.URL}, "r1", time.Millisecond, func() {
		fired <- struct{}{}
	})

	select {
	case <-fired:
	case <-time.After(5 * time.Second):
		t.Fatal("the watcher never reported the cancel")
	}
	// It stops after the first: the run has one outcome, and a second call
	// would race whatever the first one started.
	select {
	case <-fired:
		t.Error("the watcher fired more than once")
	case <-time.After(50 * time.Millisecond):
	}
}

// A server the worker cannot reach is the same server that cannot have been
// told to stop this run. Ending someone's work on a network blip would destroy
// more than it protects, so a failing poll is retried, not obeyed.
func TestWatchCancelIgnoresPollFailures(t *testing.T) {
	var polls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		polls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	fired := make(chan struct{}, 1)
	go WatchCancel(t.Context(), WorkerAPIClientConfig{BaseURL: srv.URL}, "r1", time.Millisecond, func() {
		fired <- struct{}{}
	})

	select {
	case <-fired:
		t.Fatal("a failing poll was treated as a cancel")
	case <-time.After(100 * time.Millisecond):
	}
	if polls.Load() < 2 {
		t.Errorf("polls = %d, want the watcher to keep trying", polls.Load())
	}
}

func TestWatchCancelStopsWithItsContext(t *testing.T) {
	var polls atomic.Int32
	srv := cancelServer(t, 0, &polls)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		WatchCancel(ctx, WorkerAPIClientConfig{BaseURL: srv.URL}, "r1", time.Millisecond, func() {})
		close(done)
	}()
	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the watcher outlived its context")
	}
}
