package portal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"buildmax/internal/storage/entity"
	"buildmax/internal/testutil"
)

const workflowTestSecret = "workflow-test-secret"

func TestWorkflowHandlers(t *testing.T) {
	teamID := "tm_personal_u1"
	workflowStore := &testutil.MockWorkflowStore{
		Workflows: []entity.Workflow{{
			WorkflowID:  "w_1",
			TeamID:      teamID,
			Name:        "WF",
			Description: "desc",
			Definition:  `{"steps":[{"step_id":"s1","type":"agent_task","target_agent_id":"a_1","prompt":"do it"}]}`,
			CreatedBy:   "u1",
			CreatedAt:   100,
			UpdatedAt:   100,
		}},
	}
	agentStore := &testutil.MockAgentStore{
		Agents: []entity.Agent{{AgentID: "a_1", UserID: "u1", TeamID: teamID, Name: "Agent 1", Instructions: "Do things"}},
	}
	teamStore := &testutil.MockTeamStore{
		Teams:   []entity.Team{{TeamID: teamID, Name: "My Space", PersonalForUserID: testutil.PtrString("u1"), CreatedBy: "u1"}},
		Members: []entity.TeamMember{{TeamID: teamID, UserID: "u1", Role: entity.TeamRoleOwner}},
	}
	taskStore := &testutil.MockTaskStore{}
	issueStore := &testutil.MockIssueStore{
		Issues: []entity.Issue{{
			IssueID:      "i_1",
			UserID:       "u1",
			TeamID:       teamID,
			Title:        "Issue",
			Description:  "Desc",
			Status:       entity.IssueStatusTodo,
			AssigneeKind: testutil.PtrString(entity.IssueAssigneeWorkflow),
			AssigneeID:   testutil.PtrString("w_1"),
			CreatedBy:    "u1",
		}},
	}
	h := NewHandler(Config{
		JWTSecret:         workflowTestSecret,
		TeamStore:         teamStore,
		WorkflowStore:     workflowStore,
		AgentStore:        agentStore,
		TaskStore:         taskStore,
		IssueStore:        issueStore,
		ConversationStore: &testutil.MockConversationStore{},
	})
	mux := http.NewServeMux()
	h.Register(mux)

	t.Run("GET list workflows", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/teams/"+teamID+"/workflows", nil)
		req.Header.Set("Authorization", "Bearer "+testutil.SignJWT("u1", workflowTestSecret))
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
		req.Header.Set("Authorization", "Bearer "+testutil.SignJWT("u1", workflowTestSecret))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusCreated, rec.Body.String())
		}
	})

	t.Run("POST direct workflow run", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/teams/"+teamID+"/workflows/w_1/runs", nil)
		req.Header.Set("Authorization", "Bearer "+testutil.SignJWT("u1", workflowTestSecret))
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

	t.Run("POST issue workflow run", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/teams/"+teamID+"/issues/i_1/workflow-runs", nil)
		req.Header.Set("Authorization", "Bearer "+testutil.SignJWT("u1", workflowTestSecret))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusCreated, rec.Body.String())
		}
	})

	t.Run("GET issue flow", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/teams/"+teamID+"/issues/i_1/flow", nil)
		req.Header.Set("Authorization", "Bearer "+testutil.SignJWT("u1", workflowTestSecret))
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
