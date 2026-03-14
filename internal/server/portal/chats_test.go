package portal

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"buildmax/internal/quota"
	"buildmax/internal/testutil"
	"buildmax/internal/storage/entity"
)

func TestListWorkspaceChatsHandler(t *testing.T) {
	secret := "test-chats-secret"
	userWorkspaces := []entity.Workspace{
		{WorkspaceID: "ws1", OwnerUserID: "u1", Name: "Default", CreatedAt: 123},
	}
	mockWS := &testutil.MockWorkspaceStore{List: userWorkspaces}

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
			chatStore:    &testutil.MockChatStore{},
			authHeader:   "",
			path:         "/api/workspaces/ws1/chats",
			jwtSecret:    secret,
			wantStatus:   http.StatusUnauthorized,
			wantBodyHas:  "unauthorized",
			wantArrayLen: -1,
		},
		{
			name:         "workspace not owned returns 403",
			chatStore:    &testutil.MockChatStore{},
			authHeader:   "Bearer " + testutil.SignJWT("u1", secret),
			path:         "/api/workspaces/ws-other/chats",
			jwtSecret:    secret,
			wantStatus:   http.StatusForbidden,
			wantBodyHas:  "forbidden",
			wantArrayLen: -1,
		},
		{
			name:         "owned workspace empty list returns 200",
			chatStore:    &testutil.MockChatStore{List: []entity.Chat{}},
			authHeader:   "Bearer " + testutil.SignJWT("u1", secret),
			path:         "/api/workspaces/ws1/chats",
			jwtSecret:    secret,
			wantStatus:   http.StatusOK,
			wantBodyHas:  "[]",
			wantArrayLen: 0,
		},
		{
			name:         "owned workspace with chats returns 200",
			chatStore:    &testutil.MockChatStore{List: []entity.Chat{chat1, chat2}},
			authHeader:   "Bearer " + testutil.SignJWT("u1", secret),
			path:         "/api/workspaces/ws1/chats",
			jwtSecret:    secret,
			wantStatus:   http.StatusOK,
			wantBodyHas:  "c1",
			wantArrayLen: 2,
		},
		{
			name:         "chat store nil returns 503",
			chatStore:    nil,
			authHeader:   "Bearer " + testutil.SignJWT("u1", secret),
			path:         "/api/workspaces/ws1/chats",
			jwtSecret:    secret,
			wantStatus:   http.StatusServiceUnavailable,
			wantBodyHas:  "chats not configured",
			wantArrayLen: -1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(Config{
				JWTSecret:      tt.jwtSecret,
				WorkspaceStore: mockWS,
				ChatStore:      tt.chatStore,
			})
			mux := http.NewServeMux()
			h.Register(mux)
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
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
	mockWS := &testutil.MockWorkspaceStore{List: userWorkspaces}

	tests := []struct {
		name         string
		chatStore    entity.ChatStore
		agentStore   entity.AgentStore
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
			chatStore:   &testutil.MockChatStore{},
			agentStore:  nil,
			authHeader:  "",
			path:        "/api/workspaces/ws1/chats",
			body:        `{"input":"Do X"}`,
			jwtSecret:   secret,
			wantStatus:  http.StatusUnauthorized,
			wantBodyHas: "unauthorized",
		},
		{
			name:        "workspace not owned returns 403",
			chatStore:   &testutil.MockChatStore{},
			agentStore:  nil,
			authHeader:  "Bearer " + testutil.SignJWT("u1", secret),
			path:        "/api/workspaces/ws-other/chats",
			body:        `{"input":"Do X"}`,
			jwtSecret:   secret,
			wantStatus:  http.StatusForbidden,
			wantBodyHas: "forbidden",
		},
		{
			name:        "missing input returns 400",
			chatStore:   &testutil.MockChatStore{},
			agentStore:  nil,
			authHeader:  "Bearer " + testutil.SignJWT("u1", secret),
			path:        "/api/workspaces/ws1/chats",
			body:        `{}`,
			jwtSecret:   secret,
			wantStatus:  http.StatusBadRequest,
			wantBodyHas: "input",
		},
		{
			name:        "empty input returns 400",
			chatStore:   &testutil.MockChatStore{},
			agentStore:  nil,
			authHeader:  "Bearer " + testutil.SignJWT("u1", secret),
			path:        "/api/workspaces/ws1/chats",
			body:        `{"input":""}`,
			jwtSecret:   secret,
			wantStatus:  http.StatusBadRequest,
			wantBodyHas: "input",
		},
		{
			name: "valid body returns 201",
			chatStore: &testutil.MockChatStore{
				Create: &entity.Chat{
					ChatID: "new-chat-id", WorkspaceID: "ws1", Status: "PENDING",
					Input: "Do X", CreatedBy: "u1", CreatedAt: 99999,
				},
			},
			agentStore:   nil,
			authHeader:   "Bearer " + testutil.SignJWT("u1", secret),
			path:         "/api/workspaces/ws1/chats",
			body:         `{"input":"Do X"}`,
			jwtSecret:    secret,
			wantStatus:   http.StatusCreated,
			wantBodyHas:  "new-chat-id",
			checkCreated: true,
		},
		{
			name: "create with agent_id composes input and returns 201",
			chatStore: &testutil.MockChatStore{},
			agentStore: &testutil.MockAgentStore{
				Agents: []entity.Agent{
					{AgentID: "a_1", WorkspaceID: "ws1", Name: "TestAgent", Description: "A desc", Instructions: "Do things", CreatedAt: 100},
				},
			},
			authHeader:   "Bearer " + testutil.SignJWT("u1", secret),
			path:         "/api/workspaces/ws1/chats",
			body:         `{"agent_id":"a_1"}`,
			jwtSecret:    secret,
			wantStatus:   http.StatusCreated,
			wantBodyHas:  "TestAgent",
			checkCreated: true,
		},
		{
			name: "create with agent_id and non-empty input uses input directly",
			chatStore: &testutil.MockChatStore{},
			agentStore: &testutil.MockAgentStore{
				Agents: []entity.Agent{
					{AgentID: "a_1", WorkspaceID: "ws1", Name: "TestAgent", Description: "D", Instructions: "I", CreatedAt: 100},
				},
			},
			authHeader:   "Bearer " + testutil.SignJWT("u1", secret),
			path:         "/api/workspaces/ws1/chats",
			body:         `{"agent_id":"a_1","input":"My custom prompt for this run"}`,
			jwtSecret:    secret,
			wantStatus:   http.StatusCreated,
			wantBodyHas:  "My custom prompt for this run",
			checkCreated: true,
		},
	}
	denyChecker := quota.NewChecker(
		&testutil.DenyQuotaUserStore{User: &entity.User{UserID: "u1", QuotaTier: "free_trial"}},
		&testutil.DenyQuotaUsageReader{RunCount: 10, TotalTokens: 0},
		&testutil.DenyQuotaTierStore{Tier: &entity.QuotaTier{TierName: "free_trial", MaxRunsPerPeriod: 10, MaxTokensPerPeriod: 100000, PeriodDays: 30}},
		"free_trial",
	)
	tests = append(tests, struct {
		name         string
		chatStore    entity.ChatStore
		agentStore   entity.AgentStore
		authHeader   string
		path         string
		body         string
		jwtSecret    string
		wantStatus   int
		wantBodyHas  string
		checkCreated bool
	}{
		name:        "quota exceeded returns 429",
		chatStore:   &testutil.MockChatStore{},
		agentStore:  nil,
		authHeader:  "Bearer " + testutil.SignJWT("u1", secret),
		path:        "/api/workspaces/ws1/chats",
		body:        `{"input":"Do X"}`,
		jwtSecret:   secret,
		wantStatus:  http.StatusTooManyRequests,
		wantBodyHas: "quota exceeded",
	})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				JWTSecret:      tt.jwtSecret,
				WorkspaceStore: mockWS,
				ChatStore:      tt.chatStore,
				AgentStore:     tt.agentStore,
			}
			if tt.name == "quota exceeded returns 429" {
				cfg.QuotaChecker = denyChecker
			}
			h := NewHandler(cfg)
			mux := http.NewServeMux()
			h.Register(mux)
			req := httptest.NewRequest(http.MethodPost, tt.path, bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
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
