package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMemStreamHub_AppendBufferDone(t *testing.T) {
	hub := NewStreamHub().(*memStreamHub)
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
	hub := NewStreamHub().(*memStreamHub)
	runID := "r_xyz"
	hub.Append(runID, "")
	hub.Append(runID, "a")
	if got := hub.Buffer(runID); got != "a" {
		t.Errorf("Buffer: got %q, want \"a\"", got)
	}
}

func TestMemStreamHub_SubscribeReceivesDeltasAndDone(t *testing.T) {
	hub := NewStreamHub().(*memStreamHub)
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

func TestPostWorkerStreamHandler_AppendsToHub(t *testing.T) {
	chatRunID := "r_run1"
	cfg := Config{
		JWTSecret:   "secret",
		WorkerToken: "worker-tok",
	}
	s := New(cfg)
	handler := s.workerAuthMiddleware(http.HandlerFunc(s.postWorkerStreamHandler))

	body := bytes.NewReader([]byte(`{"delta":"hello "}`))
	req := httptest.NewRequest(http.MethodPost, "/api/worker/chat-runs/"+chatRunID+"/stream", body)
	req.SetPathValue("chat_run_id", chatRunID)
	req.Header.Set("Authorization", "Bearer worker-tok")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("POST stream: got status %d, want 200", w.Code)
	}
	if got := s.hub.Buffer(chatRunID); got != "hello " {
		t.Errorf("hub buffer: got %q, want \"hello \"", got)
	}

	body2 := bytes.NewReader([]byte(`{"delta":"world"}`))
	req2 := httptest.NewRequest(http.MethodPost, "/api/worker/chat-runs/"+chatRunID+"/stream", body2)
	req2.SetPathValue("chat_run_id", chatRunID)
	req2.Header.Set("Authorization", "Bearer worker-tok")
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("POST stream second: got status %d, want 200", w2.Code)
	}
	if got := s.hub.Buffer(chatRunID); got != "hello world" {
		t.Errorf("hub buffer after second: got %q, want \"hello world\"", got)
	}
}
