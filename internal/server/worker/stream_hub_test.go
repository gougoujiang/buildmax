package worker

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"buildmax/internal/testutil"
	"buildmax/internal/streamhub"
	"buildmax/internal/storage/entity"
)

func TestPostWorkerStreamHandler_AppendsToHub(t *testing.T) {
	chatRunID := "r_run1"
	chatID := "c_chat1"
	hub := streamhub.NewStreamHub()
	cfg := Config{
		Token:        "worker-tok",
		ChatRunStore: &testutil.MockChatRunStore{
			Runs:     []entity.ChatRun{{ChatRunID: chatRunID, ChatID: chatID}},
			ChatList: []entity.Chat{{ChatID: chatID}},
		},
		Hub: hub,
	}
	h := NewHandler(cfg)
	mux := http.NewServeMux()
	h.Register(mux)

	body := bytes.NewReader([]byte(`{"delta":"hello "}`))
	req := httptest.NewRequest(http.MethodPost, "/api/worker/chat-runs/"+chatRunID+"/stream", body)
	req.SetPathValue("chat_run_id", chatRunID)
	req.Header.Set("Authorization", "Bearer worker-tok")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("POST stream: got status %d, want 200", w.Code)
	}
	if got := hub.Buffer(chatID); got != "hello " {
		t.Errorf("hub buffer: got %q, want \"hello \"", got)
	}

	body2 := bytes.NewReader([]byte(`{"delta":"world"}`))
	req2 := httptest.NewRequest(http.MethodPost, "/api/worker/chat-runs/"+chatRunID+"/stream", body2)
	req2.SetPathValue("chat_run_id", chatRunID)
	req2.Header.Set("Authorization", "Bearer worker-tok")
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("POST stream second: got status %d, want 200", w2.Code)
	}
	if got := hub.Buffer(chatID); got != "hello world" {
		t.Errorf("hub buffer after second: got %q, want \"hello world\"", got)
	}
}
