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

const issueTestSecret = "issue-test-secret"

func TestIssueHandlers(t *testing.T) {
	agentID := "a_1"
	store := &testutil.MockIssueStore{
		Issues: []entity.Issue{
			{
				IssueID:      "i_1",
				UserID:       "u1",
				Title:        "Initial issue",
				Description:  "Initial description",
				Status:       entity.IssueStatusTodo,
				CreatedBy:    "u1",
				CreatedAt:    100,
				UpdatedAt:    100,
				AssigneeKind: nil,
				AssigneeID:   nil,
			},
		},
	}
	agents := &testutil.MockAgentStore{
		Agents: []entity.Agent{{AgentID: agentID, UserID: "u1", Name: "Agent 1"}},
	}
	h := NewHandler(Config{
		JWTSecret: issueTestSecret,
		IssueStore: store,
		AgentStore: agents,
	})
	mux := http.NewServeMux()
	h.Register(mux)

	t.Run("GET list issues", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/issues", nil)
		req.Header.Set("Authorization", "Bearer "+testutil.SignJWT("u1", issueTestSecret))
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
		req := httptest.NewRequest(http.MethodPost, "/api/issues", strings.NewReader(`{"title":"New issue","description":"Desc"}`))
		req.Header.Set("Authorization", "Bearer "+testutil.SignJWT("u1", issueTestSecret))
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
		if out.Title != "New issue" || out.Status != entity.IssueStatusTodo {
			t.Fatalf("created = %+v", out)
		}
	})

	t.Run("POST create issue missing title returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/issues", strings.NewReader(`{"title":"","description":"Desc"}`))
		req.Header.Set("Authorization", "Bearer "+testutil.SignJWT("u1", issueTestSecret))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
		}
	})

	t.Run("GET issue detail", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/issues/i_1", nil)
		req.Header.Set("Authorization", "Bearer "+testutil.SignJWT("u1", issueTestSecret))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	})

	t.Run("PATCH issue assign to agent", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPatch, "/api/issues/i_1", strings.NewReader(`{"status":"in_progress","assignee_kind":"agent","assignee_id":"a_1"}`))
		req.Header.Set("Authorization", "Bearer "+testutil.SignJWT("u1", issueTestSecret))
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
		if out.Status != entity.IssueStatusInProgress || out.AssigneeKind == nil || *out.AssigneeKind != entity.IssueAssigneeAgent {
			t.Fatalf("patched = %+v", out)
		}
	})

	t.Run("PATCH invalid status returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPatch, "/api/issues/i_1", strings.NewReader(`{"status":"blocked"}`))
		req.Header.Set("Authorization", "Bearer "+testutil.SignJWT("u1", issueTestSecret))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
		}
	})

	t.Run("GET issue unauthorized returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/issues/i_1", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})
}
