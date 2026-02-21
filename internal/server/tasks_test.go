package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"buildmax/internal/storage/entity"
)

func ptrStr(s string) *string { return &s }

func TestListWorkspaceTasksHandler(t *testing.T) {
	secret := "test-tasks-secret"
	userWorkspaces := []entity.Workspace{
		{WorkspaceID: "ws1", OwnerUserID: "u1", Name: "Default", CreatedAt: 123},
	}
	mockWS := &mockWorkspaceStore{list: userWorkspaces}

	task1 := entity.Task{
		TaskID: "t1", WorkspaceID: "ws1", Status: "PENDING", Input: "Do something",
		CreatedBy: "u1", CreatedAt: 1000,
	}
	task2 := entity.Task{
		TaskID: "t2", WorkspaceID: "ws1", Status: "PENDING", Input: "Explore",
		CreatedBy: "u1", CreatedAt: 1001,
	}

	tests := []struct {
		name         string
		taskStore    entity.TaskStore
		authHeader   string
		path         string
		jwtSecret    string
		wantStatus   int
		wantBodyHas  string
		wantArrayLen int
	}{
		{
			name:         "no auth returns 401",
			taskStore:    &mockTaskStore{},
			authHeader:   "",
			path:         "/api/workspaces/ws1/tasks",
			jwtSecret:    secret,
			wantStatus:   http.StatusUnauthorized,
			wantBodyHas:  "unauthorized",
			wantArrayLen: -1,
		},
		{
			name:         "workspace not owned returns 403",
			taskStore:    &mockTaskStore{},
			authHeader:   "Bearer " + signJWT("u1", secret),
			path:         "/api/workspaces/ws-other/tasks",
			jwtSecret:    secret,
			wantStatus:   http.StatusForbidden,
			wantBodyHas:  "forbidden",
			wantArrayLen: -1,
		},
		{
			name:         "owned workspace empty list returns 200",
			taskStore:    &mockTaskStore{list: []entity.Task{}},
			authHeader:   "Bearer " + signJWT("u1", secret),
			path:         "/api/workspaces/ws1/tasks",
			jwtSecret:    secret,
			wantStatus:   http.StatusOK,
			wantBodyHas:  "[]",
			wantArrayLen: 0,
		},
		{
			name:         "owned workspace with tasks returns 200",
			taskStore:    &mockTaskStore{list: []entity.Task{task1, task2}},
			authHeader:   "Bearer " + signJWT("u1", secret),
			path:         "/api/workspaces/ws1/tasks",
			jwtSecret:    secret,
			wantStatus:   http.StatusOK,
			wantBodyHas:  "t1",
			wantArrayLen: 2,
		},
		{
			name:         "task store nil returns 503",
			taskStore:    nil,
			authHeader:   "Bearer " + signJWT("u1", secret),
			path:         "/api/workspaces/ws1/tasks",
			jwtSecret:    secret,
			wantStatus:   http.StatusServiceUnavailable,
			wantBodyHas:  "tasks not configured",
			wantArrayLen: -1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				WorkspaceStore: mockWS,
				JWTSecret:      tt.jwtSecret,
			}
			if tt.taskStore != nil {
				cfg.TaskStore = tt.taskStore
			}
			s := New(cfg)
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
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

func TestCreateWorkspaceTaskHandler(t *testing.T) {
	secret := "test-create-task-secret"
	userWorkspaces := []entity.Workspace{
		{WorkspaceID: "ws1", OwnerUserID: "u1", Name: "Default", CreatedAt: 123},
	}
	mockWS := &mockWorkspaceStore{list: userWorkspaces}

	tests := []struct {
		name         string
		taskStore    entity.TaskStore
		authHeader   string
		path         string
		body         string
		jwtSecret    string
		wantStatus   int
		wantBodyHas  string
		checkCreated bool
	}{
		{
			name:        "no auth returns 401",
			taskStore:   &mockTaskStore{},
			authHeader:  "",
			path:        "/api/workspaces/ws1/tasks",
			body:        `{"input":"Do X"}`,
			jwtSecret:   secret,
			wantStatus:  http.StatusUnauthorized,
			wantBodyHas: "unauthorized",
		},
		{
			name:        "workspace not owned returns 403",
			taskStore:   &mockTaskStore{},
			authHeader:  "Bearer " + signJWT("u1", secret),
			path:        "/api/workspaces/ws-other/tasks",
			body:        `{"input":"Do X"}`,
			jwtSecret:   secret,
			wantStatus:  http.StatusForbidden,
			wantBodyHas: "forbidden",
		},
		{
			name:        "missing input returns 400",
			taskStore:   &mockTaskStore{},
			authHeader:  "Bearer " + signJWT("u1", secret),
			path:        "/api/workspaces/ws1/tasks",
			body:        `{}`,
			jwtSecret:   secret,
			wantStatus:  http.StatusBadRequest,
			wantBodyHas: "input",
		},
		{
			name:        "empty input returns 400",
			taskStore:   &mockTaskStore{},
			authHeader:  "Bearer " + signJWT("u1", secret),
			path:        "/api/workspaces/ws1/tasks",
			body:        `{"input":""}`,
			jwtSecret:   secret,
			wantStatus:  http.StatusBadRequest,
			wantBodyHas: "input",
		},
		{
			name: "valid body returns 201",
			taskStore: &mockTaskStore{
				create: &entity.Task{
					TaskID: "new-task-id", WorkspaceID: "ws1", Status: "PENDING",
					Input: "Do X", CreatedBy: "u1", CreatedAt: 99999,
				},
			},
			authHeader:   "Bearer " + signJWT("u1", secret),
			path:         "/api/workspaces/ws1/tasks",
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
				WorkspaceStore: mockWS,
				TaskStore:      tt.taskStore,
				JWTSecret:      tt.jwtSecret,
			}
			s := New(cfg)
			req := httptest.NewRequest(http.MethodPost, tt.path, bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
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
				for _, key := range []string{"id", "workspace_id", "status", "input", "created_by", "created_at"} {
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
