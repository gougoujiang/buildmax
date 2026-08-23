package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDrainingServerIsNotReadyButIsStillAlive(t *testing.T) {
	s := New(Config{
		Addr:      ":0",
		Readiness: []ReadinessCheck{{Name: "database", Probe: func(context.Context) error { return nil }}},
	})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	body, code := getJSON(t, ts.URL+"/readyz")
	if code != http.StatusOK {
		t.Fatalf("before draining: /readyz = %d, want 200 (%s)", code, body)
	}

	s.Drain()

	body, code = getJSON(t, ts.URL+"/readyz")
	if code != http.StatusServiceUnavailable {
		t.Errorf("draining: /readyz = %d, want 503 (%s)", code, body)
	}
	var ready struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(body), &ready); err != nil {
		t.Fatalf("decode /readyz: %v", err)
	}
	if ready.Status != "draining" {
		t.Errorf("/readyz status = %q, want %q", ready.Status, "draining")
	}

	// Liveness must not fail while stopping, or the kubelet restarts a process
	// that is already exiting.
	if _, code := getJSON(t, ts.URL+"/healthz"); code != http.StatusOK {
		t.Errorf("draining: /healthz = %d, want 200", code)
	}
}

func TestDrainIsIdempotent(t *testing.T) {
	s := New(Config{Addr: ":0"})
	s.Drain()
	s.Drain()
	if !s.Draining() {
		t.Fatal("Draining() = false after Drain()")
	}
}

func TestShutdownDrainsBeforeClosingTheListener(t *testing.T) {
	s := New(Config{Addr: "127.0.0.1:0"})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("Shutdown: %v", err)
	}
	if !s.Draining() {
		t.Error("Shutdown did not drain")
	}
}

func TestShutdownBudgetSplitsTheGrace(t *testing.T) {
	b := NewShutdownBudget(20 * time.Second)
	if b.Workers != 12*time.Second {
		t.Errorf("Workers = %v, want 12s", b.Workers)
	}
	if b.Streams != time.Second {
		t.Errorf("Streams = %v, want 1s", b.Streams)
	}
	if b.Requests != 5*time.Second {
		t.Errorf("Requests = %v, want 5s", b.Requests)
	}
	if b.Background != 2*time.Second {
		t.Errorf("Background = %v, want 2s", b.Background)
	}
	if b.Total() != 20*time.Second {
		t.Errorf("Total = %v, want 20s", b.Total())
	}
}

func TestShutdownBudgetFloorsUnusableValues(t *testing.T) {
	// A deployment that configured nothing still gets a usable ladder.
	if got := NewShutdownBudget(0).Total(); got != DefaultShutdownGrace {
		t.Errorf("zero grace: Total = %v, want %v", got, DefaultShutdownGrace)
	}
	// So does one that configured something too small to divide.
	small := NewShutdownBudget(10 * time.Millisecond)
	if small.Workers <= 0 || small.Streams <= 0 || small.Requests <= 0 || small.Background <= 0 {
		t.Errorf("tiny grace produced an empty phase: %+v", small)
	}
}
