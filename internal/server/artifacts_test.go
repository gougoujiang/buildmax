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
	mockArt := &mockArtifactStore{
		list: []entity.ArtifactWithTask{
			{
				ArtifactID:       "art-1",
				TaskID:           "task-1",
				WorkspaceID:      workspaceID,
				CreatedAt:        100,
				Seq:              1,
				TaskInputSnippet: "input snippet",
			},
		},
	}

	tests := []struct {
		name           string
		artifactStore  entity.ArtifactStore
		auth           string
		wantStatus     int
		wantBodyArray  bool
	}{
		{
			name:          "200 with auth and ArtifactStore",
			artifactStore: mockArt,
			auth:          "Bearer " + token,
			wantStatus:    http.StatusOK,
			wantBodyArray: true,
		},
		{
			name:          "503 without ArtifactStore",
			artifactStore: nil,
			auth:          "Bearer " + token,
			wantStatus:    http.StatusServiceUnavailable,
		},
		{
			name:          "401 without auth",
			artifactStore: mockArt,
			auth:          "",
			wantStatus:    http.StatusUnauthorized,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				WorkspaceStore: mockWS,
				ArtifactStore:  tt.artifactStore,
				JWTSecret:      secret,
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
	artifactID := "art-1"
	token := signJWT(userID, secret)

	mockWS := &mockWorkspaceStore{
		list: []entity.Workspace{
			{WorkspaceID: workspaceID, OwnerUserID: userID, Name: "Default", CreatedAt: 1},
		},
	}
	mockTask := &mockTaskStore{
		list: []entity.Task{
			{TaskID: "task-1", WorkspaceID: workspaceID, Status: "SUCCEEDED", Input: "in", CreatedBy: userID, CreatedAt: 1},
		},
	}
	mockArt := &mockArtifactStore{
		get: map[string]*entity.Artifact{
			artifactID: {ArtifactID: artifactID, TaskID: "task-1", CreatedAt: 1, Seq: 1},
		},
		listItems: map[string][]entity.ArtifactItem{
			artifactID: {{ArtifactID: artifactID, RelativePath: "result-task1.md"}},
		},
	}

	cfg := Config{
		WorkspaceStore: mockWS,
		ArtifactStore:  mockArt,
		TaskStore:      mockTask,
		JWTSecret:      secret,
	}
	s := New(cfg)
	req := httptest.NewRequest(http.MethodGet, "/api/workspaces/"+workspaceID+"/artifacts/"+artifactID+"/items", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.SetPathValue("workspace_id", workspaceID)
	req.SetPathValue("artifact_id", artifactID)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "relative_path") || !strings.Contains(body, "result-task1.md") {
		t.Errorf("body should contain relative_path and result-task1.md, got %q", body)
	}
}

func TestArtifactContentHandler(t *testing.T) {
	secret := "test-secret"
	userID := "user-1"
	workspaceID := "ws-1"
	artifactID := "art-1"
	taskID := "task-1"
	token := signJWT(userID, secret)

	mockWS := &mockWorkspaceStore{
		list: []entity.Workspace{
			{WorkspaceID: workspaceID, OwnerUserID: userID, Name: "Default", CreatedAt: 1},
		},
	}
	mockTask := &mockTaskStore{
		list: []entity.Task{
			{TaskID: taskID, WorkspaceID: workspaceID, Status: "SUCCEEDED", Input: "in", CreatedBy: userID, CreatedAt: 1},
		},
	}
	mockArtNotFound := &mockArtifactStore{get: map[string]*entity.Artifact{}}
	mockArtFound := &mockArtifactStore{
		get: map[string]*entity.Artifact{
			artifactID: {ArtifactID: artifactID, TaskID: taskID, CreatedAt: 1, Seq: 1},
		},
	}

	tests := []struct {
		name          string
		artifactStore entity.ArtifactStore
		taskStore     entity.TaskStore
		auth          string
		wantStatus    int
	}{
		{
			name:          "404 when artifact not found",
			artifactStore: mockArtNotFound,
			taskStore:     mockTask,
			auth:          "Bearer " + token,
			wantStatus:    http.StatusNotFound,
		},
		{
			name:          "503 without ArtifactStore",
			artifactStore: nil,
			taskStore:     mockTask,
			auth:          "Bearer " + token,
			wantStatus:    http.StatusServiceUnavailable,
		},
		{
			name:          "401 without auth",
			artifactStore: mockArtFound,
			taskStore:     mockTask,
			auth:          "",
			wantStatus:    http.StatusUnauthorized,
		},
	}
	artifactStorage := newMockArtifactStorage()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				WorkspaceStore:   mockWS,
				ArtifactStore:    tt.artifactStore,
				ArtifactStorage:  artifactStorage,
				TaskStore:        tt.taskStore,
				JWTSecret:        secret,
			}
			s := New(cfg)
			req := httptest.NewRequest(http.MethodGet, "/api/workspaces/"+workspaceID+"/artifacts/"+artifactID+"/content", nil)
			if tt.auth != "" {
				req.Header.Set("Authorization", tt.auth)
			}
			req.SetPathValue("workspace_id", workspaceID)
			req.SetPathValue("artifact_id", artifactID)
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}
