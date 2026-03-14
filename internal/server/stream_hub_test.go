package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"buildmax/internal/storage/entity"
)

func TestPostWorkerStreamHandler_AppendsToHub(t *testing.T) {
	chatRunID := "r_run1"
	chatID := "c_chat1"
	cfg := Config{
		Auth:   AuthConfig{JWTSecret: "secret"},
		Worker: WorkerConfig{WorkerToken: "worker-tok"},
		Stores: StoresConfig{ChatRunStore: &mockChatRunStore{runs: []entity.ChatRun{{ChatRunID: chatRunID, ChatID: chatID}}, chatList: []entity.Chat{{ChatID: chatID}}}},
	}
	s := New(cfg)
	body := bytes.NewReader([]byte(`{"delta":"hello "}`))
	req := httptest.NewRequest(http.MethodPost, "/api/worker/chat-runs/"+chatRunID+"/stream", body)
	req.SetPathValue("chat_run_id", chatRunID)
	req.Header.Set("Authorization", "Bearer worker-tok")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("POST stream: got status %d, want 200", w.Code)
	}
	// Hub is keyed by chat_id, not run_id.
	if got := s.hub.Buffer(chatID); got != "hello " {
		t.Errorf("hub buffer: got %q, want \"hello \"", got)
	}

	body2 := bytes.NewReader([]byte(`{"delta":"world"}`))
	req2 := httptest.NewRequest(http.MethodPost, "/api/worker/chat-runs/"+chatRunID+"/stream", body2)
	req2.SetPathValue("chat_run_id", chatRunID)
	req2.Header.Set("Authorization", "Bearer worker-tok")
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	s.Handler().ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("POST stream second: got status %d, want 200", w2.Code)
	}
	if got := s.hub.Buffer(chatID); got != "hello world" {
		t.Errorf("hub buffer after second: got %q, want \"hello world\"", got)
	}
}
