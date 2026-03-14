package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"buildmax/internal/storage/entity"
)

func TestListWorkspaceArtifactsHandler(t *testing.T) {
	secret := "test-secret"
	userID := "user-1"
	workspaceID := "ws-1"
	token := signJWT(userID, secret)

	mockWS := &mockWorkspaceStore{
		list: []entity.Workspace{
			{WorkspaceID: workspaceID, OwnerUserID: userID, Name: "Default", CreatedAt: 1},
		},
	}
	mockLister := &mockRunOutputLister{
		list: []entity.ArtifactWithChat{
			{
				ArtifactID:       "run-1",
				ChatID:           "chat-1",
				ChatRunID:        "run-1",
				WorkspaceID:      workspaceID,
				CreatedAt:        100,
				ChatInputSnippet: "input snippet",
			},
		},
	}

	tests := []struct {
		name            string
		runOutputLister RunOutputLister
		auth            string
		wantStatus      int
		wantBodyArray   bool
	}{
		{
			name:            "200 with auth and RunOutputLister",
			runOutputLister: mockLister,
			auth:            "Bearer " + token,
			wantStatus:      http.StatusOK,
			wantBodyArray:   true,
		},
		{
			name:            "503 without RunOutputLister",
			runOutputLister: nil,
			auth:            "Bearer " + token,
			wantStatus:      http.StatusServiceUnavailable,
		},
		{
			name:            "401 without auth",
			runOutputLister: mockLister,
			auth:            "",
			wantStatus:      http.StatusUnauthorized,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				Stores: StoresConfig{WorkspaceStore: mockWS, RunOutputLister: tt.runOutputLister},
				Auth:   AuthConfig{JWTSecret: secret},
			}
			s := New(cfg)
			req := httptest.NewRequest(http.MethodGet, "/api/workspaces/"+workspaceID+"/artifacts", nil)
			if tt.auth != "" {
				req.Header.Set("Authorization", tt.auth)
			}
			req.SetPathValue("workspace_id", workspaceID)
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantBodyArray && rec.Code == http.StatusOK {
				ct := rec.Header().Get("Content-Type")
				if ct != "application/json" {
					t.Errorf("Content-Type = %q, want application/json", ct)
				}
				body := rec.Body.String()
				if body != "[" && len(body) < 10 {
					t.Errorf("body should be JSON array, got %q", body)
				}
			}
		})
	}
}

func TestListArtifactItemsHandler(t *testing.T) {
	secret := "test-secret"
	userID := "user-1"
	workspaceID := "ws-1"
	chatRunID := "run-1"
	token := signJWT(userID, secret)

	mockWS := &mockWorkspaceStore{
		list: []entity.Workspace{
			{WorkspaceID: workspaceID, OwnerUserID: userID, Name: "Default", CreatedAt: 1},
		},
	}
	mockChatRun := &mockChatRunStore{
		runs:     []entity.ChatRun{{ChatRunID: chatRunID, ChatID: "chat-1", Status: "SUCCEEDED", CreatedAt: 1}},
		chatList: []entity.Chat{{ChatID: "chat-1", WorkspaceID: workspaceID, Status: "SUCCEEDED", Input: "in", CreatedBy: userID, CreatedAt: 1}},
	}
	mockLister := &mockRunOutputLister{
		outputFiles: map[string][]entity.ChatRunArtifact{
			chatRunID: {{ChatRunID: chatRunID, RelativePath: "result-chat1.md"}},
		},
	}

	cfg := Config{
		Stores: StoresConfig{WorkspaceStore: mockWS, ChatRunStore: mockChatRun, RunOutputLister: mockLister},
		Auth:   AuthConfig{JWTSecret: secret},
	}
	s := New(cfg)
	req := httptest.NewRequest(http.MethodGet, "/api/workspaces/"+workspaceID+"/artifacts/"+chatRunID+"/items", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.SetPathValue("workspace_id", workspaceID)
	req.SetPathValue("chat_run_id", chatRunID)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "relative_path") || !strings.Contains(body, "result-chat1.md") {
		t.Errorf("body should contain relative_path and result-chat1.md, got %q", body)
	}
}

func TestArtifactContentHandler(t *testing.T) {
	secret := "test-secret"
	userID := "user-1"
	workspaceID := "ws-1"
	chatRunID := "run-1"
	chatID := "chat-1"
	token := signJWT(userID, secret)

	mockWS := &mockWorkspaceStore{
		list: []entity.Workspace{
			{WorkspaceID: workspaceID, OwnerUserID: userID, Name: "Default", CreatedAt: 1},
		},
	}
	mockChatRunNotFound := &mockChatRunStore{runs: nil, chatList: nil}
	mockChatRunFound := &mockChatRunStore{
		runs:     []entity.ChatRun{{ChatRunID: chatRunID, ChatID: chatID, Status: "SUCCEEDED", CreatedAt: 1}},
		chatList: []entity.Chat{{ChatID: chatID, WorkspaceID: workspaceID, Status: "SUCCEEDED", Input: "in", CreatedBy: userID, CreatedAt: 1}},
	}
	mockListerFound := &mockRunOutputLister{
		outputFiles: map[string][]entity.ChatRunArtifact{chatRunID: {{ChatRunID: chatRunID, RelativePath: "result.md"}}},
	}

	tests := []struct {
		name            string
		chatRunStore    entity.ChatRunStore
		runOutputLister RunOutputLister
		auth            string
		wantStatus      int
	}{
		{
			name:            "404 when run not found",
			chatRunStore:    mockChatRunNotFound,
			runOutputLister: mockListerFound,
			auth:            "Bearer " + token,
			wantStatus:      http.StatusNotFound,
		},
		{
			name:            "503 without RunOutputLister",
			chatRunStore:    mockChatRunFound,
			runOutputLister: nil,
			auth:            "Bearer " + token,
			wantStatus:      http.StatusServiceUnavailable,
		},
		{
			name:            "401 without auth",
			chatRunStore:    mockChatRunFound,
			runOutputLister: mockListerFound,
			auth:            "",
			wantStatus:      http.StatusUnauthorized,
		},
	}
	artifactStorage := newMockArtifactStorage()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				Stores:  StoresConfig{WorkspaceStore: mockWS, ChatRunStore: tt.chatRunStore, RunOutputLister: tt.runOutputLister},
				Storage: StorageConfig{ArtifactStorage: artifactStorage},
				Auth:    AuthConfig{JWTSecret: secret},
			}
			s := New(cfg)
			req := httptest.NewRequest(http.MethodGet, "/api/workspaces/"+workspaceID+"/artifacts/"+chatRunID+"/content", nil)
			if tt.auth != "" {
				req.Header.Set("Authorization", tt.auth)
			}
			req.SetPathValue("workspace_id", workspaceID)
			req.SetPathValue("chat_run_id", chatRunID)
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}
