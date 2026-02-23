package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"buildmax/internal/storage/entity"
)

func TestGetRunStreamHandler_RequiresAuth(t *testing.T) {
	runID := "r_run1"
	chatID := "c1"
	workspaceID := "ws1"
	cfg := Config{
		JWTSecret:      "secret",
		WorkspaceStore: &mockWorkspaceStore{list: []entity.Workspace{{WorkspaceID: workspaceID, OwnerUserID: "u1", Name: "Default", CreatedAt: 1}}},
		ChatStore:      &mockChatStore{list: []entity.Chat{{ChatID: chatID, WorkspaceID: workspaceID}}},
		ChatRunStore:   &mockChatRunStore{runs: []entity.ChatRun{{ChatRunID: runID, ChatID: chatID}}, chatList: []entity.Chat{{ChatID: chatID, WorkspaceID: workspaceID}}},
	}
	s := New(cfg)
	path := "/api/workspaces/" + workspaceID + "/chats/" + chatID + "/runs/" + runID + "/stream"
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.SetPathValue("workspace_id", workspaceID)
	req.SetPathValue("chat_id", chatID)
	req.SetPathValue("run_id", runID)
	w := httptest.NewRecorder()
	s.getRunStreamHandler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("without auth: got status %d, want 401", w.Code)
	}
}

func TestGetRunStreamHandler_RunNotFound(t *testing.T) {
	workspaceID := "ws1"
	chatID := "c1"
	runID := "r_nonexistent"
	cfg := Config{
		JWTSecret:      "secret",
		WorkspaceStore: &mockWorkspaceStore{list: []entity.Workspace{{WorkspaceID: workspaceID, OwnerUserID: "u1", Name: "Default", CreatedAt: 1}}},
		ChatStore:      &mockChatStore{list: []entity.Chat{{ChatID: chatID, WorkspaceID: workspaceID}}},
		ChatRunStore:   &mockChatRunStore{runs: []entity.ChatRun{}}, // no run
	}
	s := New(cfg)
	path := "/api/workspaces/" + workspaceID + "/chats/" + chatID + "/runs/" + runID + "/stream"
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.SetPathValue("workspace_id", workspaceID)
	req.SetPathValue("chat_id", chatID)
	req.SetPathValue("run_id", runID)
	req.Header.Set("Authorization", "Bearer "+signJWT("u1", "secret"))
	w := httptest.NewRecorder()
	s.getRunStreamHandler(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("run not found: got status %d, want 404", w.Code)
	}
}

func TestGetRunStreamHandler_ReturnsStreamWithBuffer(t *testing.T) {
	runID := "r_stream1"
	chatID := "c1"
	workspaceID := "ws1"
	cfg := Config{
		JWTSecret:      "secret",
		WorkspaceStore: &mockWorkspaceStore{list: []entity.Workspace{{WorkspaceID: workspaceID, OwnerUserID: "u1", Name: "Default", CreatedAt: 1}}},
		ChatStore:      &mockChatStore{list: []entity.Chat{{ChatID: chatID, WorkspaceID: workspaceID}}},
		ChatRunStore:   &mockChatRunStore{runs: []entity.ChatRun{{ChatRunID: runID, ChatID: chatID}}, chatList: []entity.Chat{{ChatID: chatID, WorkspaceID: workspaceID}}},
	}
	s := New(cfg)
	// Hub is keyed by chat_id; run-scoped handler subscribes with chatID from path.
	s.hub.Append(chatID, "hello stream")

	path := "/api/workspaces/" + workspaceID + "/chats/" + chatID + "/runs/" + runID + "/stream"
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx)
	req.SetPathValue("workspace_id", workspaceID)
	req.SetPathValue("chat_id", chatID)
	req.SetPathValue("run_id", runID)
	req.Header.Set("Authorization", "Bearer "+signJWT("u1", "secret"))
	w := httptest.NewRecorder()

	go s.getRunStreamHandler(w, req)
	time.Sleep(30 * time.Millisecond)
	cancel()
	time.Sleep(20 * time.Millisecond)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "data:") {
		t.Errorf("body should contain SSE data: %q", body)
	}
	if !strings.Contains(body, "hello stream") {
		t.Errorf("body should contain buffered content: %q", body)
	}
}
