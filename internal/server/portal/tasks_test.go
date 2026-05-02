package portal

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"buildmax/internal/quota"
	"buildmax/internal/storage/entity"
	"buildmax/internal/testutil"
)

func TestListConversationTasksHandler(t *testing.T) {
	secret := "test-tasks-secret"
	conversationID := "conv1"
	teamID := "tm_personal_u1"
	mockConversations := &testutil.MockConversationStore{
		Conversations: []entity.Conversation{
			{ConversationID: conversationID, UserID: "u1", TeamID: teamID, Channel: "portal", CreatedBy: "u1", CreatedAt: 123},
		},
	}
	task1 := entity.Task{TaskID: "t1", ConversationID: conversationID, TeamID: teamID, Status: "PENDING", Input: "Do something", CreatedBy: "u1", CreatedAt: 1000}
	task2 := entity.Task{TaskID: "t2", ConversationID: conversationID, TeamID: teamID, Status: "PENDING", Input: "Explore", CreatedBy: "u1", CreatedAt: 1001}

	tests := []struct {
		name         string
		taskStore    entity.TaskStore
		authHeader   string
		path         string
		wantStatus   int
		wantBodyHas  string
		wantArrayLen int
	}{
		{
			name:         "no auth returns 401",
			taskStore:    &testutil.MockTaskStore{},
			authHeader:   "",
			path:         "/api/teams/" + teamID + "/conversations/" + conversationID + "/tasks",
			wantStatus:   http.StatusUnauthorized,
			wantBodyHas:  "unauthorized",
			wantArrayLen: -1,
		},
		{
			name:         "conversation not owned returns 404",
			taskStore:    &testutil.MockTaskStore{},
			authHeader:   "Bearer " + testutil.SignJWT("u1", secret),
			path:         "/api/teams/" + teamID + "/conversations/conv-other/tasks",
			wantStatus:   http.StatusNotFound,
			wantBodyHas:  "conversation not found",
			wantArrayLen: -1,
		},
		{
			name:         "owned conversation empty list returns 200",
			taskStore:    &testutil.MockTaskStore{List: []entity.Task{}},
			authHeader:   "Bearer " + testutil.SignJWT("u1", secret),
			path:         "/api/teams/" + teamID + "/conversations/" + conversationID + "/tasks",
			wantStatus:   http.StatusOK,
			wantBodyHas:  "[]",
			wantArrayLen: 0,
		},
		{
			name:         "owned conversation with tasks returns 200",
			taskStore:    &testutil.MockTaskStore{List: []entity.Task{task1, task2}},
			authHeader:   "Bearer " + testutil.SignJWT("u1", secret),
			path:         "/api/teams/" + teamID + "/conversations/" + conversationID + "/tasks",
			wantStatus:   http.StatusOK,
			wantBodyHas:  "t1",
			wantArrayLen: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(Config{
				JWTSecret:         secret,
				TeamStore:         &testutil.MockTeamStore{Teams: []entity.Team{{TeamID: teamID, Name: "My Space", PersonalForUserID: testutil.PtrString("u1"), CreatedBy: "u1"}}, Members: []entity.TeamMember{{TeamID: teamID, UserID: "u1", Role: entity.TeamRoleOwner}}},
				TaskStore:         tt.taskStore,
				ConversationStore: mockConversations,
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
					t.Fatalf("decode body: %v", err)
				}
				if len(arr) != tt.wantArrayLen {
					t.Errorf("array len = %d, want %d", len(arr), tt.wantArrayLen)
				}
			}
		})
	}
}

func TestCreateConversationTaskHandler(t *testing.T) {
	secret := "test-create-task-secret"
	conversationID := "conv1"
	teamID := "tm_personal_u1"
	mockConversations := &testutil.MockConversationStore{
		Conversations: []entity.Conversation{
			{ConversationID: conversationID, UserID: "u1", TeamID: teamID, Channel: "portal", CreatedBy: "u1", CreatedAt: 123},
		},
	}

	tests := []struct {
		name         string
		taskStore    entity.TaskStore
		agentStore   entity.AgentStore
		authHeader   string
		path         string
		body         string
		wantStatus   int
		wantBodyHas  string
		checkCreated bool
	}{
		{
			name:        "no auth returns 401",
			taskStore:   &testutil.MockTaskStore{},
			authHeader:  "",
			path:        "/api/teams/" + teamID + "/conversations/" + conversationID + "/tasks",
			body:        `{"input":"Do X"}`,
			wantStatus:  http.StatusUnauthorized,
			wantBodyHas: "unauthorized",
		},
		{
			name:        "conversation not owned returns 404",
			taskStore:   &testutil.MockTaskStore{},
			authHeader:  "Bearer " + testutil.SignJWT("u1", secret),
			path:        "/api/teams/" + teamID + "/conversations/conv-other/tasks",
			body:        `{"input":"Do X"}`,
			wantStatus:  http.StatusNotFound,
			wantBodyHas: "conversation not found",
		},
		{
			name:        "missing input returns 400",
			taskStore:   &testutil.MockTaskStore{},
			authHeader:  "Bearer " + testutil.SignJWT("u1", secret),
			path:        "/api/teams/" + teamID + "/conversations/" + conversationID + "/tasks",
			body:        `{}`,
			wantStatus:  http.StatusBadRequest,
			wantBodyHas: "input",
		},
		{
			name: "valid body returns 201",
			taskStore: &testutil.MockTaskStore{
				Create: &entity.Task{
					TaskID: "new-task-id", ConversationID: conversationID, TeamID: teamID, Status: "PENDING",
					Input: "Do X", CreatedBy: "u1", CreatedAt: 99999,
				},
			},
			authHeader:   "Bearer " + testutil.SignJWT("u1", secret),
			path:         "/api/teams/" + teamID + "/conversations/" + conversationID + "/tasks",
			body:         `{"input":"Do X"}`,
			wantStatus:   http.StatusCreated,
			wantBodyHas:  "new-task-id",
			checkCreated: true,
		},
		{
			name:      "create with agent_id composes input and returns 201",
			taskStore: &testutil.MockTaskStore{},
			agentStore: &testutil.MockAgentStore{
				Agents: []entity.Agent{
					{AgentID: "a_1", UserID: "u1", TeamID: teamID, Name: "TestAgent", Description: "A desc", Instructions: "Do things", CreatedAt: 100},
				},
			},
			authHeader:   "Bearer " + testutil.SignJWT("u1", secret),
			path:         "/api/teams/" + teamID + "/conversations/" + conversationID + "/tasks",
			body:         `{"agent_id":"a_1"}`,
			wantStatus:   http.StatusCreated,
			wantBodyHas:  "TestAgent",
			checkCreated: true,
		},
	}
	denyChecker := quota.NewChecker(
		&testutil.DenyQuotaTeamStore{Team: &entity.Team{TeamID: teamID, QuotaTier: "free_trial"}},
		&testutil.DenyQuotaUsageReader{RunCount: 10, TotalTokens: 0},
		&testutil.DenyQuotaTierStore{Tier: &entity.QuotaTier{TierName: "free_trial", MaxRunsPerPeriod: 10, MaxTokensPerPeriod: 100000, PeriodDays: 30}},
		"free_trial",
	)
	tests = append(tests, struct {
		name         string
		taskStore    entity.TaskStore
		agentStore   entity.AgentStore
		authHeader   string
		path         string
		body         string
		wantStatus   int
		wantBodyHas  string
		checkCreated bool
	}{
		name:        "quota exceeded returns 429",
		taskStore:   &testutil.MockTaskStore{},
		authHeader:  "Bearer " + testutil.SignJWT("u1", secret),
		path:        "/api/teams/" + teamID + "/conversations/" + conversationID + "/tasks",
		body:        `{"input":"Do X"}`,
		wantStatus:  http.StatusTooManyRequests,
		wantBodyHas: "quota exceeded",
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				JWTSecret:         secret,
				TeamStore:         &testutil.MockTeamStore{Teams: []entity.Team{{TeamID: teamID, Name: "My Space", PersonalForUserID: testutil.PtrString("u1"), CreatedBy: "u1"}}, Members: []entity.TeamMember{{TeamID: teamID, UserID: "u1", Role: entity.TeamRoleOwner}}},
				TaskStore:         tt.taskStore,
				AgentStore:        tt.agentStore,
				ConversationStore: mockConversations,
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
				for _, key := range []string{"id", "conversation_id", "status", "input", "created_by", "created_at"} {
					if _, ok := out[key]; !ok {
						t.Errorf("response missing key %q", key)
					}
				}
			}
		})
	}
}
