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

func TestListProjectsHandler(t *testing.T) {
	secret := "test-projects-secret"
	userWorkspaces := []store.Workspace{
		{WorkspaceID: "ws1", OwnerUserID: "u1", Name: "Default", CreatedAt: 123},
	}
	ws1Projects := []store.Project{
		{ProjectID: "p1", WorkspaceID: "ws1", Name: "Proj One", Description: "d1", CreatedAt: 100},
	}
	mockWS := &mockWorkspaceStore{list: userWorkspaces}
	mockPS := &mockProjectStore{list: ws1Projects}

	tests := []struct {
		name           string
		workspaceStore store.WorkspaceStore
		projectStore   store.ProjectStore
		authHeader     string
		pathSuffix     string // e.g. "/ws1" so path is /api/workspaces/ws1/projects
		jwtSecret      string
		wantStatus     int
		wantBodyHas    string
		wantArrayLen   int
	}{
		{
			name:           "no auth returns 401",
			workspaceStore: mockWS,
			projectStore:   mockPS,
			authHeader:     "",
			pathSuffix:     "/ws1",
			jwtSecret:      secret,
			wantStatus:     http.StatusUnauthorized,
			wantBodyHas:    "unauthorized",
		},
		{
			name:           "invalid token returns 401",
			workspaceStore: mockWS,
			projectStore:   mockPS,
			authHeader:     "Bearer bad-token",
			pathSuffix:     "/ws1",
			jwtSecret:      secret,
			wantStatus:     http.StatusUnauthorized,
		},
		{
			name:           "workspace not owned returns 403",
			workspaceStore: mockWS,
			projectStore:   mockPS,
			authHeader:     "Bearer " + signJWT("u1", secret),
			pathSuffix:     "/ws-other",
			jwtSecret:      secret,
			wantStatus:     http.StatusForbidden,
			wantBodyHas:    "forbidden",
		},
		{
			name:           "owned workspace empty list returns 200",
			workspaceStore: mockWS,
			projectStore:   &mockProjectStore{list: nil},
			authHeader:     "Bearer " + signJWT("u1", secret),
			pathSuffix:     "/ws1",
			jwtSecret:      secret,
			wantStatus:     http.StatusOK,
			wantBodyHas:    "[]",
			wantArrayLen:   0,
		},
		{
			name:           "owned workspace one project returns 200",
			workspaceStore: mockWS,
			projectStore:   mockPS,
			authHeader:     "Bearer " + signJWT("u1", secret),
			pathSuffix:     "/ws1",
			jwtSecret:      secret,
			wantStatus:     http.StatusOK,
			wantBodyHas:    "Proj One",
			wantArrayLen:   1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(Config{
				WorkspaceStore: tt.workspaceStore,
				ProjectStore:   tt.projectStore,
				JWTSecret:      tt.jwtSecret,
			})
			req := httptest.NewRequest(http.MethodGet, "/api/workspaces"+tt.pathSuffix+"/projects", nil)
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

func TestCreateProjectHandler(t *testing.T) {
	secret := "test-create-secret"
	userWorkspaces := []store.Workspace{
		{WorkspaceID: "ws1", OwnerUserID: "u1", Name: "Default", CreatedAt: 123},
	}
	mockWS := &mockWorkspaceStore{list: userWorkspaces}

	tests := []struct {
		name           string
		workspaceStore store.WorkspaceStore
		projectStore   store.ProjectStore
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
			projectStore:   &mockProjectStore{},
			authHeader:     "",
			pathSuffix:     "/ws1",
			body:           `{"name":"My Project"}`,
			jwtSecret:      secret,
			wantStatus:     http.StatusUnauthorized,
			wantBodyHas:    "unauthorized",
		},
		{
			name:           "workspace not owned returns 403",
			workspaceStore: mockWS,
			projectStore:   &mockProjectStore{},
			authHeader:     "Bearer " + signJWT("u1", secret),
			pathSuffix:     "/ws-other",
			body:           `{"name":"My Project"}`,
			jwtSecret:      secret,
			wantStatus:     http.StatusForbidden,
			wantBodyHas:    "forbidden",
		},
		{
			name:           "missing name returns 400",
			workspaceStore: mockWS,
			projectStore:   &mockProjectStore{},
			authHeader:     "Bearer " + signJWT("u1", secret),
			pathSuffix:     "/ws1",
			body:           `{}`,
			jwtSecret:      secret,
			wantStatus:     http.StatusBadRequest,
			wantBodyHas:    "name",
		},
		{
			name:           "empty name returns 400",
			workspaceStore: mockWS,
			projectStore:   &mockProjectStore{},
			authHeader:     "Bearer " + signJWT("u1", secret),
			pathSuffix:     "/ws1",
			body:           `{"name":""}`,
			jwtSecret:      secret,
			wantStatus:     http.StatusBadRequest,
			wantBodyHas:    "name",
		},
		{
			name:           "valid body returns 201",
			workspaceStore: mockWS,
			projectStore:   &mockProjectStore{
				create: &store.Project{
					ProjectID:   "new-id",
					WorkspaceID: "ws1",
					Name:        "My Project",
					Description: "desc",
					CreatedAt:   999,
				},
			},
			authHeader:     "Bearer " + signJWT("u1", secret),
			pathSuffix:     "/ws1",
			body:           `{"name":"My Project","description":"desc"}`,
			jwtSecret:      secret,
			wantStatus:     http.StatusCreated,
			wantBodyHas:    "new-id",
			checkCreated:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(Config{
				WorkspaceStore: tt.workspaceStore,
				ProjectStore:   tt.projectStore,
				JWTSecret:      tt.jwtSecret,
			})
			req := httptest.NewRequest(http.MethodPost, "/api/workspaces"+tt.pathSuffix+"/projects", bytes.NewBufferString(tt.body))
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
				for _, key := range []string{"id", "workspace_id", "name", "description", "created_at"} {
					if _, ok := out[key]; !ok {
						t.Errorf("response missing key %q", key)
					}
				}
			}
		})
	}
}
