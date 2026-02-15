package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"buildmax/internal/store"

	"github.com/golang-jwt/jwt/v5"
)

// mockWorkspaceStore is an in-memory WorkspaceStore for tests.
type mockWorkspaceStore struct {
	ensureErr error
	list      []store.Workspace
	listErr   error
}

func (m *mockWorkspaceStore) EnsureDefaultWorkspaceForUser(_ context.Context, _ string) error {
	return m.ensureErr
}

func (m *mockWorkspaceStore) ListWorkspacesByOwner(_ context.Context, _ string) ([]store.Workspace, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.list, nil
}

func signJWT(sub, secret string) string {
	now := time.Now()
	claims := jwtClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		Sub: sub,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, _ := token.SignedString([]byte(secret))
	return s
}

func TestWorkspacesHandler(t *testing.T) {
	secret := "test-workspaces-secret"
	oneWorkspace := []store.Workspace{
		{WorkspaceID: "ws1", OwnerUserID: "u1", Name: "Default", CreatedAt: 123},
	}

	tests := []struct {
		name          string
		workspaceStore store.WorkspaceStore
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
