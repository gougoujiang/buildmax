package work

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	agentdef "github.com/gougoujiang/buildmax/internal/core/agentdef"
	coreissue "github.com/gougoujiang/buildmax/internal/core/issue"
	coreteam "github.com/gougoujiang/buildmax/internal/core/team"
	coreworkflow "github.com/gougoujiang/buildmax/internal/core/workflow"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/testsupport"
	"github.com/gougoujiang/buildmax/internal/util"
)

const workflowTestSecret = "workflow-test-secret"

func TestWorkflowHandlers(t *testing.T) {
	teamID := "tm_personal_u1"
	workflowStore := &mock.MockWorkflowStore{
		Workflows: []coreworkflow.Workflow{{
			ID:          "w_1",
			TeamID:      teamID,
			Name:        "WF",
			Description: "desc",
			Definition:  `{"steps":[{"step_id":"s1","type":"agent_task","target_agent_id":"a_1","prompt":"do it"}]}`,
			Status:      coreworkflow.StatusPublished,
			CreatedBy:   "u1",
			CreatedAt:   time.Unix(100, 0).UTC(),
			UpdatedAt:   time.Unix(100, 0).UTC(),
		}},
	}
	agentStore := &mock.MockAgentStore{
		Agents: []agentdef.Agent{{ID: "a_1", UserID: "u1", TeamID: teamID, Name: "Agent 1", Instructions: "Do things"}},
	}
	teamStore := &mock.MockTeamStore{
		Teams:   []coreteam.Team{{ID: teamID, Name: "My Space", PersonalForUserID: util.Ptr("u1"), CreatedBy: "u1"}},
		Members: []coreteam.Member{{TeamID: teamID, UserID: "u1", Role: coreteam.RoleOwner}, {TeamID: teamID, UserID: "u2", Role: coreteam.RoleMember}, {TeamID: teamID, UserID: "u3", Role: coreteam.RoleAdmin}},
	}
	taskStore := &mock.MockTaskStore{}
	issueStore := &mock.MockIssueStore{
		Issues: []coreissue.Issue{{
			ID:           "i_1",
			UserID:       "u1",
			TeamID:       teamID,
			Title:        "Issue",
			Description:  "Desc",
			Status:       coreissue.StatusTodo,
			AssigneeKind: util.Ptr(coreissue.AssigneeWorkflow),
			AssigneeID:   util.Ptr("w_1"),
			CreatedBy:    "u1",
		}},
	}
	h := New(Config{
		JWTSecret:     workflowTestSecret,
		Teams:         teamStore,
		Workflows:     workflowStore,
		Agents:        agentStore,
		Tasks:         taskStore,
		Issues:        issueStore,
		Conversations: &mock.MockConversationStore{},
	})
	mux := http.NewServeMux()
	h.Register(mux)

	t.Run("GET list workflows", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/teams/"+teamID+"/workflows", nil)
		req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT("u1", workflowTestSecret))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		var out workflowListResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(out.Workflows) != 1 || out.Workflows[0].ID != "w_1" {
			t.Fatalf("workflows = %+v", out.Workflows)
		}
	})

	t.Run("POST create workflow", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/teams/"+teamID+"/workflows", strings.NewReader(`{"name":"WF 2","description":"Desc","definition":"{\"steps\":[{\"step_id\":\"s1\",\"type\":\"agent_task\",\"target_agent_id\":\"a_1\",\"prompt\":\"do it\"}]}"}`))
		req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT("u1", workflowTestSecret))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusCreated, rec.Body.String())
		}
		var out workflowResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if out.Status != coreworkflow.StatusDraft {
			t.Fatalf("workflow status = %q, want %q", out.Status, coreworkflow.StatusDraft)
		}
	})

	t.Run("POST direct workflow run", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/teams/"+teamID+"/workflows/w_1/runs", nil)
		req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT("u1", workflowTestSecret))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusCreated, rec.Body.String())
		}
		var out workflowRunDetailResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if out.Run.ID == "" || len(out.Steps) != 1 {
			t.Fatalf("run detail = %+v", out)
		}
	})

	t.Run("POST create workflow forbidden for member", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/teams/"+teamID+"/workflows", strings.NewReader(`{"name":"WF 3","description":"Desc","definition":"{\"steps\":[{\"step_id\":\"s1\",\"type\":\"agent_task\",\"target_agent_id\":\"a_1\",\"prompt\":\"do it\"}]}"}`))
		req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT("u2", workflowTestSecret))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
		}
	})

	t.Run("PATCH publish workflow by admin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPatch, "/api/teams/"+teamID+"/workflows/w_1", strings.NewReader(`{"status":"published"}`))
		req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT("u3", workflowTestSecret))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
	})

	t.Run("POST issue workflow run", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/teams/"+teamID+"/issues/i_1/workflow-runs", nil)
		req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT("u1", workflowTestSecret))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusCreated, rec.Body.String())
		}
	})

	t.Run("GET issue flow", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/teams/"+teamID+"/issues/i_1/flow", nil)
		req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT("u1", workflowTestSecret))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		var out issueFlowResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if out.Issue.ID != "i_1" || out.Workflow == nil || out.Workflow.ID != "w_1" || len(out.Runs) == 0 {
			t.Fatalf("issue flow = %+v", out)
		}
	})
}
