package work

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	agentdef "github.com/gougoujiang/buildmax/internal/core/agentdef"
	"github.com/gougoujiang/buildmax/internal/core/model"
	coreteam "github.com/gougoujiang/buildmax/internal/core/team"
	coreworkflow "github.com/gougoujiang/buildmax/internal/core/workflow"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/testsupport"
	"github.com/gougoujiang/buildmax/internal/util"
)

const issueTestSecret = "issue-test-secret"

func TestIssueHandlers(t *testing.T) {
	agentID := "a_1"
	personalTeamID := "tm_personal_u1"
	otherTeamID := "tm_other"
	workflowID := "w_1"
	store := &mock.MockIssueStore{
		Issues: []model.Issue{
			{
				ID:           "i_1",
				UserID:       "u1",
				TeamID:       personalTeamID,
				Title:        "Initial issue",
				Description:  "Initial description",
				Status:       model.IssueStatusTodo,
				CreatedBy:    "u1",
				CreatedAt:    time.Unix(100, 0).UTC(),
				UpdatedAt:    time.Unix(100, 0).UTC(),
				AssigneeKind: nil,
				AssigneeID:   nil,
			},
		},
	}
	agents := &mock.MockAgentStore{
		Agents: []agentdef.Agent{{ID: agentID, UserID: "u1", TeamID: personalTeamID, Name: "Agent 1"}},
	}
	workflows := &mock.MockWorkflowStore{
		Workflows: []coreworkflow.Workflow{{ID: workflowID, TeamID: personalTeamID, Name: "Workflow 1", Definition: `{"steps":[{"step_id":"s1","type":"agent_task","target_agent_id":"a_1","prompt":"do it"}]}`, Status: coreworkflow.StatusPublished}},
	}
	tasks := &mock.MockTaskStore{}
	teams := &mock.MockTeamStore{
		Teams: []coreteam.Team{
			{ID: personalTeamID, Name: "My Space", PersonalForUserID: util.Ptr("u1"), CreatedBy: "u1"},
			{ID: otherTeamID, Name: "Other", CreatedBy: "u2"},
		},
		Members: []coreteam.Member{
			{TeamID: personalTeamID, UserID: "u1", Role: coreteam.RoleOwner},
			{TeamID: personalTeamID, UserID: "u2", Role: coreteam.RoleMember},
			{TeamID: personalTeamID, UserID: "u3", Role: coreteam.RoleAdmin},
			{TeamID: otherTeamID, UserID: "u2", Role: coreteam.RoleOwner},
		},
	}
	h := New(Config{
		JWTSecret:     issueTestSecret,
		Teams:         teams,
		Issues:        store,
		Agents:        agents,
		Workflows:     workflows,
		Tasks:         tasks,
		Conversations: &mock.MockConversationStore{},
	})
	mux := http.NewServeMux()
	h.Register(mux)

	t.Run("GET list issues", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/teams/"+personalTeamID+"/issues", nil)
		req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT("u1", issueTestSecret))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		var out issueListResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode list: %v", err)
		}
		if len(out.Issues) != 1 || out.Issues[0].ID != "i_1" {
			t.Fatalf("issues = %+v", out.Issues)
		}
	})

	t.Run("POST create issue", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/teams/"+personalTeamID+"/issues", strings.NewReader(`{"title":"New issue","description":"Desc"}`))
		req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT("u1", issueTestSecret))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusCreated, rec.Body.String())
		}
		var out IssueResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode create: %v", err)
		}
		if out.Title != "New issue" || out.Status != model.IssueStatusTodo {
			t.Fatalf("created = %+v", out)
		}
		if out.TeamID != personalTeamID {
			t.Fatalf("created team_id = %q, want %q", out.TeamID, personalTeamID)
		}
	})

	t.Run("POST create issue missing title returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/teams/"+personalTeamID+"/issues", strings.NewReader(`{"title":"","description":"Desc"}`))
		req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT("u1", issueTestSecret))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
		}
	})

	t.Run("GET issue detail", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/teams/"+personalTeamID+"/issues/i_1", nil)
		req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT("u1", issueTestSecret))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	})

	t.Run("PATCH issue assign to agent", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPatch, "/api/teams/"+personalTeamID+"/issues/i_1", strings.NewReader(`{"status":"in_progress","assignee_kind":"agent","assignee_id":"a_1"}`))
		req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT("u1", issueTestSecret))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		var out IssueResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode patch: %v", err)
		}
		if out.Status != model.IssueStatusInProgress || out.AssigneeKind == nil || *out.AssigneeKind != model.IssueAssigneeAgent {
			t.Fatalf("patched = %+v", out)
		}
	})

	t.Run("POST issue agent run", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/teams/"+personalTeamID+"/issues/i_1/agent-runs", nil)
		req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT("u1", issueTestSecret))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusCreated, rec.Body.String())
		}
		var out TaskResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode agent run: %v", err)
		}
		if out.IssueID == nil || *out.IssueID != "i_1" || out.AgentID == nil || *out.AgentID != agentID {
			t.Fatalf("agent run = %+v", out)
		}
		if len(tasks.List) == 0 {
			t.Fatal("expected created task to be persisted")
		}
		created := tasks.List[len(tasks.List)-1]
		if created.TeamID != personalTeamID {
			t.Fatalf("created task team_id = %q, want %q", created.TeamID, personalTeamID)
		}
		if created.IssueID == nil || *created.IssueID != "i_1" {
			t.Fatalf("created task issue_id = %v, want i_1", created.IssueID)
		}
		flowReq := httptest.NewRequest(http.MethodGet, "/api/teams/"+personalTeamID+"/issues/i_1/flow", nil)
		flowReq.Header.Set("Authorization", "Bearer "+testsupport.SignJWT("u1", issueTestSecret))
		flowRec := httptest.NewRecorder()
		mux.ServeHTTP(flowRec, flowReq)
		if flowRec.Code != http.StatusOK {
			t.Fatalf("flow status = %d, want %d body=%s", flowRec.Code, http.StatusOK, flowRec.Body.String())
		}
		var flow issueFlowResponse
		if err := json.Unmarshal(flowRec.Body.Bytes(), &flow); err != nil {
			t.Fatalf("decode flow: %v", err)
		}
		if len(flow.AgentTasks) == 0 {
			t.Fatal("expected issue flow to include created agent task")
		}
	})

	t.Run("PATCH invalid status returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPatch, "/api/teams/"+personalTeamID+"/issues/i_1", strings.NewReader(`{"status":"blocked"}`))
		req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT("u1", issueTestSecret))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
		}
	})

	t.Run("PATCH issue assign to workflow", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPatch, "/api/teams/"+personalTeamID+"/issues/i_1", strings.NewReader(`{"assignee_kind":"workflow","assignee_id":"w_1"}`))
		req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT("u1", issueTestSecret))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		var out IssueResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode patch: %v", err)
		}
		if out.AssigneeKind == nil || *out.AssigneeKind != model.IssueAssigneeWorkflow {
			t.Fatalf("patched = %+v", out)
		}
	})

	t.Run("PATCH issue assign to workflow forbidden for member", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPatch, "/api/teams/"+personalTeamID+"/issues/i_1", strings.NewReader(`{"assignee_kind":"workflow","assignee_id":"w_1"}`))
		req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT("u2", issueTestSecret))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
		}
	})

	t.Run("GET issue unauthorized returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/teams/"+personalTeamID+"/issues/i_1", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})

	t.Run("GET issues forbidden for non-member explicit team", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/teams/"+otherTeamID+"/issues", nil)
		req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT("u1", issueTestSecret))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
		}
	})
}
