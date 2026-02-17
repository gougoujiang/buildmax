package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"buildmax/internal/storage/entity"
)

func TestWorkspacesHandler(t *testing.T) {
	secret := "test-workspaces-secret"
	oneWorkspace := []entity.Workspace{
		{WorkspaceID: "ws1", OwnerUserID: "u1", Name: "Default", CreatedAt: 123},
	}

	tests := []struct {
		name          string
		workspaceStore entity.WorkspaceStore
		authHeader    string
		jwtSecret     string
		wantStatus    int
		wantBodyHas   string
		wantArrayLen  int
	}{
		{
			name:          "no auth returns 401",
			workspaceStore: &mockWorkspaceStore{list: oneWorkspace},
			authHeader:    "",
			jwtSecret:     secret,
			wantStatus:    http.StatusUnauthorized,
			wantBodyHas:   "unauthorized",
		},
		{
			name:          "invalid bearer returns 401",
			workspaceStore: &mockWorkspaceStore{list: oneWorkspace},
			authHeader:    "Bearer invalid-token",
			jwtSecret:     secret,
			wantStatus:    http.StatusUnauthorized,
		},
		{
			name:          "valid JWT returns 200 and workspace list",
			workspaceStore: &mockWorkspaceStore{list: oneWorkspace},
			authHeader:    "Bearer " + signJWT("u1", secret),
			jwtSecret:     secret,
			wantStatus:    http.StatusOK,
			wantBodyHas:   "Default",
			wantArrayLen:  1,
		},
		{
			name:          "no WorkspaceStore returns 503",
			workspaceStore: nil,
			authHeader:    "Bearer " + signJWT("u1", secret),
			jwtSecret:     secret,
			wantStatus:    http.StatusServiceUnavailable,
			wantBodyHas:   "not configured",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(Config{
				WorkspaceStore: tt.workspaceStore,
				JWTSecret:      tt.jwtSecret,
			})
			req := httptest.NewRequest(http.MethodGet, "/api/workspaces", nil)
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
			if tt.wantArrayLen > 0 {
				var arr []map[string]interface{}
				if err := json.Unmarshal([]byte(body), &arr); err != nil {
					t.Fatalf("decode body: %v", err)
				}
				if len(arr) != tt.wantArrayLen {
					t.Errorf("array len = %d, want %d", len(arr), tt.wantArrayLen)
				}
				if len(arr) > 0 {
					if arr[0]["id"] != "ws1" || arr[0]["name"] != "Default" {
						t.Errorf("first workspace id=%v name=%v", arr[0]["id"], arr[0]["name"])
					}
				}
			}
		})
	}
}

func TestCreateWorkspaceHandler(t *testing.T) {
	secret := "test-create-ws-secret"

	tests := []struct {
		name           string
		workspaceStore entity.WorkspaceStore
		authHeader     string
		body           string
		wantStatus     int
		wantBodyHas    string
	}{
		{
			name:           "no auth returns 401",
			workspaceStore: &mockWorkspaceStore{list: nil},
			authHeader:     "",
			body:           `{"name":"My workspace"}`,
			wantStatus:     http.StatusUnauthorized,
			wantBodyHas:    "unauthorized",
		},
		{
			name:           "empty name returns 400",
			workspaceStore: &mockWorkspaceStore{list: nil},
			authHeader:     "Bearer " + signJWT("u1", secret),
			body:           `{"name":""}`,
			wantStatus:     http.StatusBadRequest,
			wantBodyHas:    "name required",
		},
		{
			name:           "valid request returns 201 and workspace",
			workspaceStore: &mockWorkspaceStore{list: nil},
			authHeader:     "Bearer " + signJWT("u1", secret),
			body:           `{"name":"My workspace"}`,
			wantStatus:     http.StatusCreated,
			wantBodyHas:    "My workspace",
		},
		{
			name:           "no WorkspaceStore returns 503",
			workspaceStore: nil,
			authHeader:     "Bearer " + signJWT("u1", secret),
			body:           `{"name":"x"}`,
			wantStatus:     http.StatusServiceUnavailable,
			wantBodyHas:    "not configured",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(Config{
				WorkspaceStore: tt.workspaceStore,
				JWTSecret:      secret,
			})
			req := httptest.NewRequest(http.MethodPost, "/api/workspaces", strings.NewReader(tt.body))
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
			if tt.wantStatus == http.StatusCreated {
				var out struct {
					ID          string `json:"id"`
					Name        string `json:"name"`
					OwnerUserID string `json:"owner_user_id"`
					CreatedAt   int64  `json:"created_at"`
				}
				if err := json.Unmarshal([]byte(body), &out); err != nil {
					t.Fatalf("decode body: %v", err)
				}
				if out.Name != "My workspace" || out.OwnerUserID != "u1" || out.ID == "" || out.CreatedAt == 0 {
					t.Errorf("response id=%q name=%q owner_user_id=%q created_at=%d", out.ID, out.Name, out.OwnerUserID, out.CreatedAt)
				}
			}
		})
	}
}
