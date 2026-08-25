package work

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
	corequota "github.com/gougoujiang/buildmax/internal/core/quota"
	coreteam "github.com/gougoujiang/buildmax/internal/core/team"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/service/quota"
	"github.com/gougoujiang/buildmax/internal/testsupport"
	"github.com/gougoujiang/buildmax/internal/util"
)

func TestListConversationTasksHandler(t *testing.T) {
	secret := "test-tasks-secret"
	conversationID := "conv1"
	teamID := "tm_personal_u1"
	mockConversations := &mock.MockConversationStore{
		Conversations: []model.Conversation{
			{ID: conversationID, UserID: "u1", TeamID: teamID, Channel: "portal", CreatedBy: "u1", CreatedAt: time.Unix(123, 0).UTC()},
		},
	}
	task1 := model.Task{ID: "t1", ConversationID: conversationID, TeamID: teamID, Status: "PENDING", Input: "Do something", CreatedBy: "u1", CreatedAt: time.Unix(1000, 0).UTC()}
	task2 := model.Task{ID: "t2", ConversationID: conversationID, TeamID: teamID, Status: "PENDING", Input: "Explore", CreatedBy: "u1", CreatedAt: time.Unix(1001, 0).UTC()}

	tests := []struct {
		name         string
		taskStore    model.TaskStore
		authHeader   string
		path         string
		wantStatus   int
		wantBodyHas  string
		wantArrayLen int
	}{
		{
			name:         "no auth returns 401",
			taskStore:    &mock.MockTaskStore{},
			authHeader:   "",
			path:         "/api/teams/" + teamID + "/conversations/" + conversationID + "/tasks",
			wantStatus:   http.StatusUnauthorized,
			wantBodyHas:  "unauthorized",
			wantArrayLen: -1,
		},
		{
			name:         "conversation not owned returns 404",
			taskStore:    &mock.MockTaskStore{},
			authHeader:   "Bearer " + testsupport.SignJWT("u1", secret),
			path:         "/api/teams/" + teamID + "/conversations/conv-other/tasks",
			wantStatus:   http.StatusNotFound,
			wantBodyHas:  "conversation not found",
			wantArrayLen: -1,
		},
		{
			name:         "owned conversation empty list returns 200",
			taskStore:    &mock.MockTaskStore{List: []model.Task{}},
			authHeader:   "Bearer " + testsupport.SignJWT("u1", secret),
			path:         "/api/teams/" + teamID + "/conversations/" + conversationID + "/tasks",
			wantStatus:   http.StatusOK,
			wantBodyHas:  "[]",
			wantArrayLen: 0,
		},
		{
			name:         "owned conversation with tasks returns 200",
			taskStore:    &mock.MockTaskStore{List: []model.Task{task1, task2}},
			authHeader:   "Bearer " + testsupport.SignJWT("u1", secret),
			path:         "/api/teams/" + teamID + "/conversations/" + conversationID + "/tasks",
			wantStatus:   http.StatusOK,
			wantBodyHas:  "t1",
			wantArrayLen: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := New(Config{
				JWTSecret:     secret,
				Teams:         &mock.MockTeamStore{Teams: []coreteam.Team{{ID: teamID, Name: "My Space", PersonalForUserID: util.Ptr("u1"), CreatedBy: "u1"}}, Members: []coreteam.Member{{TeamID: teamID, UserID: "u1", Role: coreteam.RoleOwner}}},
				Tasks:         tt.taskStore,
				Conversations: mockConversations,
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
	mockConversations := &mock.MockConversationStore{
		Conversations: []model.Conversation{
			{ID: conversationID, UserID: "u1", TeamID: teamID, Channel: "portal", CreatedBy: "u1", CreatedAt: time.Unix(123, 0).UTC()},
		},
	}

	tests := []struct {
		name         string
		taskStore    model.TaskStore
		agentStore   model.AgentStore
		authHeader   string
		path         string
		body         string
		wantStatus   int
		wantBodyHas  string
		checkCreated bool
	}{
		{
			name:        "no auth returns 401",
			taskStore:   &mock.MockTaskStore{},
			authHeader:  "",
			path:        "/api/teams/" + teamID + "/conversations/" + conversationID + "/tasks",
			body:        `{"input":"Do X"}`,
			wantStatus:  http.StatusUnauthorized,
			wantBodyHas: "unauthorized",
		},
		{
			name:        "conversation not owned returns 404",
			taskStore:   &mock.MockTaskStore{},
			authHeader:  "Bearer " + testsupport.SignJWT("u1", secret),
			path:        "/api/teams/" + teamID + "/conversations/conv-other/tasks",
			body:        `{"input":"Do X"}`,
			wantStatus:  http.StatusNotFound,
			wantBodyHas: "conversation not found",
		},
		{
			name:        "missing input returns 400",
			taskStore:   &mock.MockTaskStore{},
			authHeader:  "Bearer " + testsupport.SignJWT("u1", secret),
			path:        "/api/teams/" + teamID + "/conversations/" + conversationID + "/tasks",
			body:        `{}`,
			wantStatus:  http.StatusBadRequest,
			wantBodyHas: "input",
		},
		{
			name: "valid body returns 201",
			taskStore: &mock.MockTaskStore{
				Create: &model.Task{
					ID: "new-task-id", ConversationID: conversationID, TeamID: teamID, Status: "PENDING",
					Input: "Do X", CreatedBy: "u1", CreatedAt: time.Unix(99999, 0).UTC(),
				},
			},
			authHeader:   "Bearer " + testsupport.SignJWT("u1", secret),
			path:         "/api/teams/" + teamID + "/conversations/" + conversationID + "/tasks",
			body:         `{"input":"Do X"}`,
			wantStatus:   http.StatusCreated,
			wantBodyHas:  "new-task-id",
			checkCreated: true,
		},
		{
			name:      "create with agent_id composes input and returns 201",
			taskStore: &mock.MockTaskStore{},
			agentStore: &mock.MockAgentStore{
				Agents: []model.Agent{
					{ID: "a_1", UserID: "u1", TeamID: teamID, Name: "TestAgent", Description: "A desc", Instructions: "Do things", CreatedAt: time.Unix(100, 0).UTC()},
				},
			},
			authHeader:   "Bearer " + testsupport.SignJWT("u1", secret),
			path:         "/api/teams/" + teamID + "/conversations/" + conversationID + "/tasks",
			body:         `{"agent_id":"a_1"}`,
			wantStatus:   http.StatusCreated,
			wantBodyHas:  "TestAgent",
			checkCreated: true,
		},
	}
	denyChecker := &quota.Service{
		TeamStore:   &mock.DenyQuotaTeamStore{Team: &coreteam.Team{ID: teamID, QuotaTier: "free_trial"}},
		UsageReader: &mock.DenyQuotaUsageReader{RunCount: 10, TotalTokens: 0},
		TierStore:   &mock.DenyQuotaTierStore{Tier: &corequota.Tier{TierName: "free_trial", MaxRunsPerPeriod: 10, MaxTokensPerPeriod: 100000, PeriodDays: 30}},
		DefaultTier: "free_trial",
	}
	tests = append(tests, struct {
		name         string
		taskStore    model.TaskStore
		agentStore   model.AgentStore
		authHeader   string
		path         string
		body         string
		wantStatus   int
		wantBodyHas  string
		checkCreated bool
	}{
		name:        "quota exceeded returns 429",
		taskStore:   &mock.MockTaskStore{},
		authHeader:  "Bearer " + testsupport.SignJWT("u1", secret),
		path:        "/api/teams/" + teamID + "/conversations/" + conversationID + "/tasks",
		body:        `{"input":"Do X"}`,
		wantStatus:  http.StatusTooManyRequests,
		wantBodyHas: "quota exceeded",
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				JWTSecret:     secret,
				Teams:         &mock.MockTeamStore{Teams: []coreteam.Team{{ID: teamID, Name: "My Space", PersonalForUserID: util.Ptr("u1"), CreatedBy: "u1"}}, Members: []coreteam.Member{{TeamID: teamID, UserID: "u1", Role: coreteam.RoleOwner}}},
				Tasks:         tt.taskStore,
				Agents:        tt.agentStore,
				Conversations: mockConversations,
			}
			if tt.name == "quota exceeded returns 429" {
				cfg.Quota = denyChecker
			}
			h := New(cfg)
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

// A card in the conversation needs to reach what its task did without a second
// round trip per task: the run behind the status, and the runs that left files.
func TestListConversationTasksCarriesRunAndArtifacts(t *testing.T) {
	secret := "test-task-cards-secret"
	conversationID := "conv1"
	teamID := "tm_personal_u1"
	task1 := model.Task{ID: "t1", ConversationID: conversationID, TeamID: teamID, Status: "SUCCEEDED", Input: "Do something", CreatedBy: "u1", CreatedAt: time.Unix(1000, 0).UTC(), LastRunID: util.Ptr("tr_1")}
	task2 := model.Task{ID: "t2", ConversationID: conversationID, TeamID: teamID, Status: "PENDING", Input: "Explore", CreatedBy: "u1", CreatedAt: time.Unix(1001, 0).UTC()}

	h := New(Config{
		JWTSecret: secret,
		Teams:     &mock.MockTeamStore{Teams: []coreteam.Team{{ID: teamID, Name: "My Space", PersonalForUserID: util.Ptr("u1"), CreatedBy: "u1"}}, Members: []coreteam.Member{{TeamID: teamID, UserID: "u1", Role: coreteam.RoleOwner}}},
		Tasks:     &mock.MockTaskStore{List: []model.Task{task1, task2}},
		Conversations: &mock.MockConversationStore{
			Conversations: []model.Conversation{{ID: conversationID, UserID: "u1", TeamID: teamID, Channel: "portal", CreatedBy: "u1", CreatedAt: time.Unix(123, 0).UTC()}},
		},
		RunOutputs: &mock.MockRunOutputLister{List: []model.ArtifactWithTask{
			{ArtifactID: "tr_1", TaskID: "t1", TaskRunID: "tr_1", ConversationID: conversationID},
		}},
	})
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/teams/"+teamID+"/conversations/"+conversationID+"/tasks", nil)
	req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT("u1", secret))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}

	var out []TaskResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("len(tasks) = %d, want 2", len(out))
	}
	byID := map[string]TaskResponse{}
	for _, task := range out {
		byID[task.ID] = task
	}
	if got := byID["t1"]; got.LastRunID == nil || *got.LastRunID != "tr_1" {
		t.Errorf("last_run_id = %v, want tr_1", got.LastRunID)
	}
	if got := byID["t1"].ArtifactRunIDs; len(got) != 1 || got[0] != "tr_1" {
		t.Errorf("artifact_run_ids = %v, want [tr_1]", got)
	}
	if got := byID["t2"].ArtifactRunIDs; len(got) != 0 {
		t.Errorf("task with no artifacts got %v", got)
	}
}
