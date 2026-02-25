package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"buildmax/internal/quota"
	"buildmax/internal/storage/entity"
)

func ptrStr(s string) *string { return &s }

// denyQuotaUserStore is used by quota 429 test to supply a user with tier.
type denyQuotaUserStore struct {
	user *entity.User
}

func (d *denyQuotaUserStore) UserByEmail(_ context.Context, _ string) (*entity.User, error) { return nil, nil }
func (d *denyQuotaUserStore) GetUser(_ context.Context, _ string) (*entity.User, error)     { return d.user, nil }
func (d *denyQuotaUserStore) CreateUser(_ context.Context, _, _ string) (*entity.User, error) {
	return nil, nil
}

type denyQuotaUsageReader struct {
	runCount, totalTokens int
}

func (d *denyQuotaUsageReader) UserUsageInWindow(_ context.Context, _ string, _, _ int64) (int, int, error) {
	return d.runCount, d.totalTokens, nil
}

type denyQuotaTierStore struct {
	tier *entity.QuotaTier
}

func (d *denyQuotaTierStore) GetQuotaTier(_ context.Context, _ string) (*entity.QuotaTier, error) {
	return d.tier, nil
}

func TestListWorkspaceChatsHandler(t *testing.T) {
	secret := "test-chats-secret"
	userWorkspaces := []entity.Workspace{
		{WorkspaceID: "ws1", OwnerUserID: "u1", Name: "Default", CreatedAt: 123},
	}
	mockWS := &mockWorkspaceStore{list: userWorkspaces}

	chat1 := entity.Chat{
		ChatID: "c1", WorkspaceID: "ws1", Status: "PENDING", Input: "Do something",
		CreatedBy: "u1", CreatedAt: 1000,
	}
	chat2 := entity.Chat{
		ChatID: "c2", WorkspaceID: "ws1", Status: "PENDING", Input: "Explore",
		CreatedBy: "u1", CreatedAt: 1001,
	}

	tests := []struct {
		name         string
		chatStore    entity.ChatStore
		authHeader   string
		path         string
		jwtSecret    string
		wantStatus   int
		wantBodyHas  string
		wantArrayLen int
	}{
		{
			name:         "no auth returns 401",
			chatStore:    &mockChatStore{},
			authHeader:   "",
			path:         "/api/workspaces/ws1/chats",
			jwtSecret:    secret,
			wantStatus:   http.StatusUnauthorized,
			wantBodyHas:  "unauthorized",
			wantArrayLen: -1,
		},
		{
			name:         "workspace not owned returns 403",
			chatStore:    &mockChatStore{},
			authHeader:   "Bearer " + signJWT("u1", secret),
			path:         "/api/workspaces/ws-other/chats",
			jwtSecret:    secret,
			wantStatus:   http.StatusForbidden,
			wantBodyHas:  "forbidden",
			wantArrayLen: -1,
		},
		{
			name:         "owned workspace empty list returns 200",
			chatStore:    &mockChatStore{list: []entity.Chat{}},
			authHeader:   "Bearer " + signJWT("u1", secret),
			path:         "/api/workspaces/ws1/chats",
			jwtSecret:    secret,
			wantStatus:   http.StatusOK,
			wantBodyHas:  "[]",
			wantArrayLen: 0,
		},
		{
			name:         "owned workspace with chats returns 200",
			chatStore:    &mockChatStore{list: []entity.Chat{chat1, chat2}},
			authHeader:   "Bearer " + signJWT("u1", secret),
			path:         "/api/workspaces/ws1/chats",
			jwtSecret:    secret,
			wantStatus:   http.StatusOK,
			wantBodyHas:  "c1",
			wantArrayLen: 2,
		},
		{
			name:         "chat store nil returns 503",
			chatStore:    nil,
			authHeader:   "Bearer " + signJWT("u1", secret),
			path:         "/api/workspaces/ws1/chats",
			jwtSecret:    secret,
			wantStatus:   http.StatusServiceUnavailable,
			wantBodyHas:  "chats not configured",
			wantArrayLen: -1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				WorkspaceStore: mockWS,
				JWTSecret:      tt.jwtSecret,
			}
			if tt.chatStore != nil {
				cfg.ChatStore = tt.chatStore
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

func TestCreateWorkspaceChatHandler(t *testing.T) {
	secret := "test-create-chat-secret"
	userWorkspaces := []entity.Workspace{
		{WorkspaceID: "ws1", OwnerUserID: "u1", Name: "Default", CreatedAt: 123},
	}
	mockWS := &mockWorkspaceStore{list: userWorkspaces}

	tests := []struct {
		name         string
		chatStore    entity.ChatStore
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
			chatStore:   &mockChatStore{},
			authHeader:  "",
			path:        "/api/workspaces/ws1/chats",
			body:        `{"input":"Do X"}`,
			jwtSecret:   secret,
			wantStatus:  http.StatusUnauthorized,
			wantBodyHas: "unauthorized",
		},
		{
			name:        "workspace not owned returns 403",
			chatStore:   &mockChatStore{},
			authHeader:  "Bearer " + signJWT("u1", secret),
			path:        "/api/workspaces/ws-other/chats",
			body:        `{"input":"Do X"}`,
			jwtSecret:   secret,
			wantStatus:  http.StatusForbidden,
			wantBodyHas: "forbidden",
		},
		{
			name:        "missing input returns 400",
			chatStore:   &mockChatStore{},
			authHeader:  "Bearer " + signJWT("u1", secret),
			path:        "/api/workspaces/ws1/chats",
			body:        `{}`,
			jwtSecret:   secret,
			wantStatus:  http.StatusBadRequest,
			wantBodyHas: "input",
		},
		{
			name:        "empty input returns 400",
			chatStore:   &mockChatStore{},
			authHeader:  "Bearer " + signJWT("u1", secret),
			path:        "/api/workspaces/ws1/chats",
			body:        `{"input":""}`,
			jwtSecret:   secret,
			wantStatus:  http.StatusBadRequest,
			wantBodyHas: "input",
		},
		{
			name: "valid body returns 201",
			chatStore: &mockChatStore{
				create: &entity.Chat{
					ChatID: "new-chat-id", WorkspaceID: "ws1", Status: "PENDING",
					Input: "Do X", CreatedBy: "u1", CreatedAt: 99999,
				},
			},
			authHeader:   "Bearer " + signJWT("u1", secret),
			path:         "/api/workspaces/ws1/chats",
			body:         `{"input":"Do X"}`,
			jwtSecret:    secret,
			wantStatus:   http.StatusCreated,
			wantBodyHas:  "new-chat-id",
			checkCreated: true,
		},
	}
	// Quota 429 test: use a checker that denies (user at run limit).
	denyChecker := quota.NewChecker(
		&denyQuotaUserStore{user: &entity.User{UserID: "u1", QuotaTier: "free_trial"}},
		&denyQuotaUsageReader{runCount: 10, totalTokens: 0},
		&denyQuotaTierStore{tier: &entity.QuotaTier{TierName: "free_trial", MaxRunsPerPeriod: 10, MaxTokensPerPeriod: 100000, PeriodDays: 30}},
		"free_trial",
	)
	tests = append(tests, struct {
		name         string
		chatStore    entity.ChatStore
		authHeader   string
		path         string
		body         string
		jwtSecret    string
		wantStatus   int
		wantBodyHas  string
		checkCreated bool
	}{
		name:        "quota exceeded returns 429",
		chatStore:   &mockChatStore{},
		authHeader:  "Bearer " + signJWT("u1", secret),
		path:        "/api/workspaces/ws1/chats",
		body:        `{"input":"Do X"}`,
		jwtSecret:   secret,
		wantStatus:  http.StatusTooManyRequests,
		wantBodyHas: "quota exceeded",
	})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				WorkspaceStore: mockWS,
				ChatStore:      tt.chatStore,
				JWTSecret:      tt.jwtSecret,
			}
			if tt.name == "quota exceeded returns 429" {
				cfg.QuotaChecker = denyChecker
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
