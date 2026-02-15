package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"buildmax/internal/store"
)

func TestListTasksHandler(t *testing.T) {
	secret := "test-tasks-secret"
	userWorkspaces := []store.Workspace{
		{WorkspaceID: "ws1", OwnerUserID: "u1", Name: "Default", CreatedAt: 123},
	}
	projInWs1 := store.Project{ProjectID: "proj1", WorkspaceID: "ws1", Name: "Proj", Description: "", CreatedAt: 100}
	projOtherWs := store.Project{ProjectID: "proj-other", WorkspaceID: "ws-other", Name: "Other", Description: "", CreatedAt: 200}
	mockWS := &mockWorkspaceStore{list: userWorkspaces}

	task1 := store.Task{
		TaskID: "t1", ProjectID: "proj1", Status: "PENDING", Input: "Do something",
		CreatedBy: "u1", CreatedAt: 1000,
	}

	tests := []struct {
		name           string
		workspaceStore store.WorkspaceStore
		projectStore   store.ProjectStore
		taskStore      store.TaskStore
		authHeader     string
		pathSuffix     string // /proj1 -> /api/projects/proj1/tasks
		jwtSecret      string
		wantStatus     int
		wantBodyHas    string
		wantArrayLen   int
	}{
		{
			name:           "no auth returns 401",
			workspaceStore: mockWS,
			projectStore:   &mockProjectStore{list: []store.Project{projInWs1}},
			taskStore:      &mockTaskStore{},
			authHeader:     "",
			pathSuffix:     "/proj1",
			jwtSecret:      secret,
			wantStatus:     http.StatusUnauthorized,
			wantBodyHas:    "unauthorized",
		},
		{
			name:           "project not found returns 404",
			workspaceStore: mockWS,
			projectStore:   &mockProjectStore{list: []store.Project{}}, // GetProject returns nil for any id
			taskStore:      &mockTaskStore{},
			authHeader:     "Bearer " + signJWT("u1", secret),
			pathSuffix:     "/nonexistent",
			jwtSecret:      secret,
			wantStatus:     http.StatusNotFound,
			wantBodyHas:    "not found",
		},
		{
			name:           "project not owned returns 403",
			workspaceStore: mockWS,
			projectStore:   &mockProjectStore{list: []store.Project{projOtherWs}},
			taskStore:      &mockTaskStore{},
			authHeader:     "Bearer " + signJWT("u1", secret),
			pathSuffix:     "/proj-other",
			jwtSecret:      secret,
			wantStatus:     http.StatusForbidden,
			wantBodyHas:    "forbidden",
		},
		{
			name:           "owned project empty list returns 200",
			workspaceStore: mockWS,
			projectStore:   &mockProjectStore{list: []store.Project{projInWs1}},
			taskStore:      &mockTaskStore{list: []store.Task{}},
			authHeader:     "Bearer " + signJWT("u1", secret),
			pathSuffix:     "/proj1",
			jwtSecret:      secret,
			wantStatus:     http.StatusOK,
			wantBodyHas:    "[]",
			wantArrayLen:   0,
		},
		{
			name:           "owned project with tasks returns 200",
			workspaceStore: mockWS,
			projectStore:   &mockProjectStore{list: []store.Project{projInWs1}},
			taskStore:      &mockTaskStore{list: []store.Task{task1}},
			authHeader:     "Bearer " + signJWT("u1", secret),
			pathSuffix:     "/proj1",
			jwtSecret:      secret,
			wantStatus:     http.StatusOK,
			wantBodyHas:    "t1",
			wantArrayLen:   1,
		},
		{
			name:           "task store nil returns 503",
			workspaceStore: mockWS,
			projectStore:   &mockProjectStore{list: []store.Project{projInWs1}},
			taskStore:      nil,
			authHeader:     "Bearer " + signJWT("u1", secret),
			pathSuffix:     "/proj1",
			jwtSecret:      secret,
			wantStatus:     http.StatusServiceUnavailable,
			wantBodyHas:    "tasks not configured",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				WorkspaceStore: tt.workspaceStore,
				ProjectStore:   tt.projectStore,
				JWTSecret:      tt.jwtSecret,
			}
			if tt.taskStore != nil {
				cfg.TaskStore = tt.taskStore
			}
			s := New(cfg)
			path := "/api/projects" + tt.pathSuffix + "/tasks"
			req := httptest.NewRequest(http.MethodGet, path, nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			body := rec.Body.String()
			if tt.wantBodyHas != "" && !strings.Contains(body, tt.wantBodyHas) {
				t.Errorf("body %q does not contain %q", body, tt.wantBodyHas)
			}
			if tt.wantArrayLen >= 0 {
				var arr []map[string]interface{}
				if err := json.Unmarshal([]byte(body), &arr); err != nil {
					if tt.wantArrayLen > 0 {
						t.Fatalf("decode body: %v", err)
					}
					return
				}
				if len(arr) != tt.wantArrayLen {
					t.Errorf("array len = %d, want %d", len(arr), tt.wantArrayLen)
				}
			}
		})
	}
}

func TestCreateTaskHandler(t *testing.T) {
	secret := "test-create-task-secret"
	userWorkspaces := []store.Workspace{
		{WorkspaceID: "ws1", OwnerUserID: "u1", Name: "Default", CreatedAt: 123},
	}
	projInWs1 := store.Project{ProjectID: "proj1", WorkspaceID: "ws1", Name: "Proj", Description: "", CreatedAt: 100}
	mockWS := &mockWorkspaceStore{list: userWorkspaces}

	tests := []struct {
		name           string
		workspaceStore store.WorkspaceStore
		projectStore   store.ProjectStore
		taskStore      store.TaskStore
		authHeader     string
		pathSuffix     string
		body           string
		jwtSecret      string
		wantStatus     int
		wantBodyHas    string
		checkCreated   bool
	}{
		{
			name:           "no auth returns 401",
			workspaceStore: mockWS,
			projectStore:   &mockProjectStore{list: []store.Project{projInWs1}},
			taskStore:      &mockTaskStore{},
			authHeader:     "",
			pathSuffix:     "/proj1",
			body:           `{"input":"Do X"}`,
			jwtSecret:      secret,
			wantStatus:     http.StatusUnauthorized,
			wantBodyHas:    "unauthorized",
		},
		{
			name:           "project not found returns 404",
			workspaceStore: mockWS,
			projectStore:   &mockProjectStore{list: []store.Project{}},
			taskStore:      &mockTaskStore{},
			authHeader:     "Bearer " + signJWT("u1", secret),
			pathSuffix:     "/nonexistent",
			body:           `{"input":"Do X"}`,
			jwtSecret:      secret,
			wantStatus:     http.StatusNotFound,
			wantBodyHas:    "not found",
		},
		{
			name:           "project not owned returns 403",
			workspaceStore: mockWS,
			projectStore:   &mockProjectStore{list: []store.Project{
				{ProjectID: "proj-other", WorkspaceID: "ws-other", Name: "Other", Description: "", CreatedAt: 200},
			}},
			taskStore:   &mockTaskStore{},
			authHeader:  "Bearer " + signJWT("u1", secret),
			pathSuffix:  "/proj-other",
			body:        `{"input":"Do X"}`,
			jwtSecret:   secret,
			wantStatus:  http.StatusForbidden,
			wantBodyHas: "forbidden",
		},
		{
			name:           "missing input returns 400",
			workspaceStore: mockWS,
			projectStore:   &mockProjectStore{list: []store.Project{projInWs1}},
			taskStore:      &mockTaskStore{},
			authHeader:     "Bearer " + signJWT("u1", secret),
			pathSuffix:     "/proj1",
			body:           `{}`,
			jwtSecret:      secret,
			wantStatus:     http.StatusBadRequest,
			wantBodyHas:    "input",
		},
		{
			name:           "empty input returns 400",
			workspaceStore: mockWS,
			projectStore:   &mockProjectStore{list: []store.Project{projInWs1}},
			taskStore:      &mockTaskStore{},
			authHeader:     "Bearer " + signJWT("u1", secret),
			pathSuffix:     "/proj1",
			body:           `{"input":""}`,
			jwtSecret:      secret,
			wantStatus:     http.StatusBadRequest,
			wantBodyHas:    "input",
		},
		{
			name:           "valid body returns 201",
			workspaceStore: mockWS,
			projectStore:   &mockProjectStore{list: []store.Project{projInWs1}},
			taskStore: &mockTaskStore{
				create: &store.Task{
					TaskID: "new-task-id", ProjectID: "proj1", Status: "PENDING",
					Input: "Do X", CreatedBy: "u1", CreatedAt: 99999,
				},
			},
			authHeader:   "Bearer " + signJWT("u1", secret),
			pathSuffix:   "/proj1",
			body:         `{"input":"Do X"}`,
			jwtSecret:    secret,
			wantStatus:   http.StatusCreated,
			wantBodyHas:  "new-task-id",
			checkCreated: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				WorkspaceStore: tt.workspaceStore,
				ProjectStore:   tt.projectStore,
				TaskStore:      tt.taskStore,
				JWTSecret:      tt.jwtSecret,
			}
			s := New(cfg)
			path := "/api/projects" + tt.pathSuffix + "/tasks"
			req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			body := rec.Body.String()
			if tt.wantBodyHas != "" && !strings.Contains(body, tt.wantBodyHas) {
				t.Errorf("body %q does not contain %q", body, tt.wantBodyHas)
			}
			if tt.checkCreated {
				var out map[string]interface{}
				if err := json.Unmarshal([]byte(body), &out); err != nil {
					t.Fatalf("decode body: %v", err)
				}
				for _, key := range []string{"id", "project_id", "status", "input", "created_by", "created_at"} {
					if _, ok := out[key]; !ok {
						t.Errorf("response missing key %q", key)
					}
				}
				if s, _ := out["status"].(string); s != "PENDING" {
					t.Errorf("status = %q, want PENDING", s)
				}
			}
		})
	}
}
