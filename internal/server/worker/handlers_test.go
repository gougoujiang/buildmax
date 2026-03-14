package worker

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"buildmax/internal/server/testutil"
	"buildmax/internal/storage/entity"
)

func TestGetWorkerChatRunHandler_RequiresWorkerAuth(t *testing.T) {
	chatRunID := "run-1"
	run := entity.ChatRun{ChatRunID: chatRunID, ChatID: "chat-1", Input: "input", Status: "SCHEDULED", CreatedAt: 1}
	chat := entity.Chat{ChatID: "chat-1", WorkspaceID: "ws1", CreatedBy: "u1"}
	mockRun := &testutil.MockChatRunStore{Runs: []entity.ChatRun{run}, ChatList: []entity.Chat{chat}}
	cfg := Config{
		Token:        "worker-token-123",
		ChatRunStore: mockRun,
	}
	h := NewHandler(cfg)
	mux := http.NewServeMux()
	h.Register(mux)

	// Without token: 401
	req := httptest.NewRequest(http.MethodGet, "/api/worker/chat-runs/"+chatRunID, nil)
	req.SetPathValue("chat_run_id", chatRunID)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("without token: got status %d, want 401", w.Code)
	}

	// With correct token: 200 and run+chat body
	req = httptest.NewRequest(http.MethodGet, "/api/worker/chat-runs/"+chatRunID, nil)
	req.SetPathValue("chat_run_id", chatRunID)
	req.Header.Set("Authorization", "Bearer worker-token-123")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("with token: got status %d, want 200", w.Code)
	}
	if body := w.Body.String(); body == "" || len(body) < 10 {
		t.Errorf("with token: body too short: %q", body)
	}
}

func TestGetWorkerChatRunHandler_NotFound(t *testing.T) {
	cfg := Config{
		Token:        "token",
		ChatRunStore: &testutil.MockChatRunStore{Runs: []entity.ChatRun{}},
	}
	h := NewHandler(cfg)
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/worker/chat-runs/nonexistent", nil)
	req.SetPathValue("chat_run_id", "nonexistent")
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("got status %d, want 404", w.Code)
	}
}

func TestPatchWorkerChatRun_RUNNING_WhenScheduled_Returns200(t *testing.T) {
	chatRunID := "run-scheduled"
	run := entity.ChatRun{ChatRunID: chatRunID, ChatID: "chat1", Input: "input", Status: "SCHEDULED", CreatedAt: 1}
	chat := entity.Chat{ChatID: "chat1", WorkspaceID: "ws1"}
	mockRun := &testutil.MockChatRunStore{Runs: []entity.ChatRun{run}, ChatList: []entity.Chat{chat}}
	cfg := Config{
		Token:        "token",
		ChatRunStore: mockRun,
	}
	h := NewHandler(cfg)
	mux := http.NewServeMux()
	h.Register(mux)

	body := map[string]interface{}{"status": "RUNNING", "session_id": "sess-1", "started_at": int64(123)}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/api/worker/chat-runs/"+chatRunID, bytes.NewReader(raw))
	req.SetPathValue("chat_run_id", chatRunID)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("PATCH RUNNING when SCHEDULED: got status %d, want 200", w.Code)
	}
	if len(mockRun.Runs) != 1 || mockRun.Runs[0].Status != "RUNNING" {
		t.Errorf("PATCH RUNNING when SCHEDULED: run status = %q, want RUNNING", mockRun.Runs[0].Status)
	}
}

func TestPatchWorkerChatRun_RUNNING_WhenPending_Returns409(t *testing.T) {
	chatRunID := "run-pending"
	run := entity.ChatRun{ChatRunID: chatRunID, ChatID: "chat1", Input: "input", Status: "PENDING", CreatedAt: 1}
	chat := entity.Chat{ChatID: "chat1", WorkspaceID: "ws1"}
	mockRun := &testutil.MockChatRunStore{Runs: []entity.ChatRun{run}, ChatList: []entity.Chat{chat}}
	cfg := Config{
		Token:        "token",
		ChatRunStore: mockRun,
	}
	h := NewHandler(cfg)
	mux := http.NewServeMux()
	h.Register(mux)

	body := map[string]interface{}{"status": "RUNNING", "session_id": "sess-1"}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/api/worker/chat-runs/"+chatRunID, bytes.NewReader(raw))
	req.SetPathValue("chat_run_id", chatRunID)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("PATCH RUNNING when PENDING: got status %d, want 409", w.Code)
	}
	if len(mockRun.Runs) != 1 || mockRun.Runs[0].Status != "PENDING" {
		t.Errorf("PATCH RUNNING when PENDING: run status = %q, want PENDING (unchanged)", mockRun.Runs[0].Status)
	}
}
