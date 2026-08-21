package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/testsupport"
	"github.com/gougoujiang/buildmax/internal/util"
)

const agentTestSecret = "agent-test-secret"

// TestAgentRevisionEndpoints walks one agent through an edit, its history, and a
// restore, which is the loop the endpoints exist to support.
func TestAgentRevisionEndpoints(t *testing.T) {
	teamID := "tm_1"
	agentStore := &mock.MockAgentStore{}
	teamStore := &mock.MockTeamStore{
		Teams:   []model.Team{{TeamID: teamID, Name: "Team", CreatedBy: "u1"}},
		Members: []model.TeamMember{{TeamID: teamID, UserID: "u1", Role: model.TeamRoleOwner}},
	}
	created, err := agentStore.CreateAgentInTeam(t.Context(), teamID, "u1", "Collector", "collects", "collect carefully")
	if err != nil {
		t.Fatalf("CreateAgentInTeam: %v", err)
	}
	if _, err := agentStore.UpdateAgentInTeam(t.Context(), created.AgentID, teamID, "u1", "Collector", "collects", "collect faster"); err != nil {
		t.Fatalf("UpdateAgentInTeam: %v", err)
	}

	h := NewHandler(Config{JWTSecret: agentTestSecret, TeamStore: teamStore, AgentStore: agentStore})
	mux := http.NewServeMux()
	h.Register(mux)
	token := "Bearer " + testsupport.SignJWT("u1", agentTestSecret)
	base := "/api/teams/" + teamID + "/agents/" + created.AgentID

	req := httptest.NewRequest(http.MethodGet, base+"/revisions", nil)
	req.Header.Set("Authorization", token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list revisions status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var list struct {
		Revisions []struct {
			Revision     int    `json:"revision"`
			Instructions string `json:"instructions"`
			CreatedBy    string `json:"created_by"`
		} `json:"revisions"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode revisions: %v", err)
	}
	if list.Total != 2 {
		t.Fatalf("total revisions = %d, want 2", list.Total)
	}
	if list.Revisions[0].Revision != 2 || list.Revisions[0].Instructions != "collect faster" {
		t.Fatalf("newest revision = %d %q, want 2 \"collect faster\"", list.Revisions[0].Revision, list.Revisions[0].Instructions)
	}

	req = httptest.NewRequest(http.MethodPost, base+"/revisions/1/restore", nil)
	req.Header.Set("Authorization", token)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("restore status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var restored struct {
		Instructions string `json:"instructions"`
		Revision     int    `json:"revision"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &restored); err != nil {
		t.Fatalf("decode restore: %v", err)
	}
	if restored.Instructions != "collect carefully" {
		t.Fatalf("restored instructions = %q, want the revision 1 text", restored.Instructions)
	}
	if restored.Revision != 3 {
		t.Fatalf("restored revision = %d, want 3 — a restore appends", restored.Revision)
	}

	req = httptest.NewRequest(http.MethodPost, base+"/revisions/99/restore", nil)
	req.Header.Set("Authorization", token)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("restore of missing revision status = %d, want 404", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, base+"/revisions/abc/restore", nil)
	req.Header.Set("Authorization", token)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("restore with non-numeric revision status = %d, want 400", rec.Code)
	}
}

// TestDeleteAgentRefusedWhilePublishedWorkflowUsesIt keeps deletion from quietly
// breaking automation: a published workflow that names the agent has to be fixed
// or archived first, and the response says which one.
func TestDeleteAgentRefusedWhilePublishedWorkflowUsesIt(t *testing.T) {
	teamID := "tm_1"
	agentStore := &mock.MockAgentStore{
		Agents: []model.Agent{{AgentID: "a_1", UserID: "u1", TeamID: teamID, Name: "Collector", Revision: 1}},
	}
	workflowStore := &mock.MockWorkflowStore{
		Workflows: []model.Workflow{{
			WorkflowID: "w_1",
			TeamID:     teamID,
			Name:       "Nightly report",
			Definition: `{"steps":[{"step_id":"s","type":"agent_task","target_agent_id":"a_1","prompt":"p"}]}`,
			Status:     model.WorkflowStatusPublished,
		}},
	}
	teamStore := &mock.MockTeamStore{
		Teams:   []model.Team{{TeamID: teamID, Name: "Team", CreatedBy: "u1"}},
		Members: []model.TeamMember{{TeamID: teamID, UserID: "u1", Role: model.TeamRoleOwner}},
	}
	h := NewHandler(Config{
		JWTSecret:     agentTestSecret,
		TeamStore:     teamStore,
		AgentStore:    agentStore,
		WorkflowStore: workflowStore,
	})
	mux := http.NewServeMux()
	h.Register(mux)
	token := "Bearer " + testsupport.SignJWT("u1", agentTestSecret)
	url := "/api/teams/" + teamID + "/agents/a_1"

	req := httptest.NewRequest(http.MethodDelete, url, nil)
	req.Header.Set("Authorization", token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Nightly report (w_1)") {
		t.Errorf("body %q should name the workflow blocking the delete", rec.Body.String())
	}
	live, err := agentStore.ListAgentsByTeam(t.Context(), teamID)
	if err != nil {
		t.Fatalf("ListAgentsByTeam: %v", err)
	}
	if len(live) != 1 {
		t.Fatalf("agents after refused delete = %d, want 1", len(live))
	}

	// Archiving the workflow releases the agent.
	archived := model.WorkflowStatusArchived
	if _, err := workflowStore.UpdateWorkflow(t.Context(), "w_1", teamID, model.UpdateWorkflowInput{Status: &archived}); err != nil {
		t.Fatalf("archive workflow: %v", err)
	}
	req = httptest.NewRequest(http.MethodDelete, url, nil)
	req.Header.Set("Authorization", token)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status after archiving = %d, want 204: %s", rec.Code, rec.Body.String())
	}
}

func TestPatchAgentHandler(t *testing.T) {
	personalTeamID := "tm_personal_u1"
	agentStore := &mock.MockAgentStore{
		Agents: []model.Agent{
			{AgentID: "a_1", UserID: "u1", TeamID: personalTeamID, Name: "Old", Description: "d1", Instructions: "i1", CreatedAt: 100},
		},
	}
	teamStore := &mock.MockTeamStore{
		Teams:   []model.Team{{TeamID: personalTeamID, Name: "My Space", PersonalForUserID: util.Ptr("u1"), CreatedBy: "u1"}},
		Members: []model.TeamMember{{TeamID: personalTeamID, UserID: "u1", Role: model.TeamRoleOwner}, {TeamID: personalTeamID, UserID: "u2", Role: model.TeamRoleMember}},
	}

	tests := []struct {
		name        string
		method      string
		url         string
		body        string
		authHeader  string
		wantStatus  int
		wantBodyHas string
	}{
		{
			name:        "PATCH success",
			method:      http.MethodPatch,
			url:         "/api/teams/" + personalTeamID + "/agents/a_1",
			body:        `{"name":"Updated","description":"d2","instructions":"i2"}`,
			authHeader:  "Bearer " + testsupport.SignJWT("u1", agentTestSecret),
			wantStatus:  http.StatusOK,
			wantBodyHas: "Updated",
		},
		{
			name:        "PATCH empty name returns 400",
			method:      http.MethodPatch,
			url:         "/api/teams/" + personalTeamID + "/agents/a_1",
			body:        `{"name":""}`,
			authHeader:  "Bearer " + testsupport.SignJWT("u1", agentTestSecret),
			wantStatus:  http.StatusBadRequest,
			wantBodyHas: "name required",
		},
		{
			name:        "PATCH forbidden for member",
			method:      http.MethodPatch,
			url:         "/api/teams/" + personalTeamID + "/agents/a_1",
			body:        `{"name":"Updated","description":"d2","instructions":"i2"}`,
			authHeader:  "Bearer " + testsupport.SignJWT("u2", agentTestSecret),
			wantStatus:  http.StatusForbidden,
			wantBodyHas: "forbidden",
		},
		{
			name:        "PATCH non-existent agent returns 404",
			method:      http.MethodPatch,
			url:         "/api/teams/" + personalTeamID + "/agents/a_999",
			body:        `{"name":"X","description":"","instructions":""}`,
			authHeader:  "Bearer " + testsupport.SignJWT("u1", agentTestSecret),
			wantStatus:  http.StatusNotFound,
			wantBodyHas: "not found",
		},
		{
			name:       "PATCH no auth returns 401",
			method:     http.MethodPatch,
			url:        "/api/teams/" + personalTeamID + "/agents/a_1",
			body:       `{"name":"X"}`,
			authHeader: "",
			wantStatus: http.StatusUnauthorized,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(Config{
				JWTSecret:  agentTestSecret,
				TeamStore:  teamStore,
				AgentStore: agentStore,
			})
			mux := http.NewServeMux()
			h.Register(mux)
			req := httptest.NewRequest(tt.method, tt.url, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantBodyHas != "" && !strings.Contains(rec.Body.String(), tt.wantBodyHas) {
				t.Errorf("body %q does not contain %q", rec.Body.String(), tt.wantBodyHas)
			}
			if tt.wantStatus == http.StatusOK {
				var out struct {
					ID           string `json:"id"`
					Name         string `json:"name"`
					Description  string `json:"description"`
					Instructions string `json:"instructions"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
					t.Fatalf("decode body: %v", err)
				}
				if out.ID != "a_1" || out.Name != "Updated" || out.Description != "d2" || out.Instructions != "i2" {
					t.Errorf("response id=%q name=%q description=%q instructions=%q", out.ID, out.Name, out.Description, out.Instructions)
				}
			}
		})
	}
}

func TestDeleteAgentHandler(t *testing.T) {
	personalTeamID := "tm_personal_u1"
	tests := []struct {
		name        string
		url         string
		authHeader  string
		wantStatus  int
		wantBodyHas string
	}{
		{
			name:       "DELETE success returns 204",
			url:        "/api/teams/" + personalTeamID + "/agents/a_1",
			authHeader: "Bearer " + testsupport.SignJWT("u1", agentTestSecret),
			wantStatus: http.StatusNoContent,
		},
		{
			name:        "DELETE non-existent agent returns 404",
			url:         "/api/teams/" + personalTeamID + "/agents/a_999",
			authHeader:  "Bearer " + testsupport.SignJWT("u1", agentTestSecret),
			wantStatus:  http.StatusNotFound,
			wantBodyHas: "not found",
		},
		{
			name:       "DELETE no auth returns 401",
			url:        "/api/teams/" + personalTeamID + "/agents/a_1",
			authHeader: "",
			wantStatus: http.StatusUnauthorized,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mock.MockAgentStore{
				Agents: []model.Agent{
					{AgentID: "a_1", UserID: "u1", TeamID: personalTeamID, Name: "ToDelete", CreatedAt: 100},
				},
			}
			teamStore := &mock.MockTeamStore{
				Teams:   []model.Team{{TeamID: personalTeamID, Name: "My Space", PersonalForUserID: util.Ptr("u1"), CreatedBy: "u1"}},
				Members: []model.TeamMember{{TeamID: personalTeamID, UserID: "u1", Role: model.TeamRoleOwner}, {TeamID: personalTeamID, UserID: "u2", Role: model.TeamRoleMember}},
			}
			h := NewHandler(Config{
				JWTSecret:  agentTestSecret,
				TeamStore:  teamStore,
				AgentStore: store,
			})
			mux := http.NewServeMux()
			h.Register(mux)
			req := httptest.NewRequest(http.MethodDelete, tt.url, nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantBodyHas != "" && !strings.Contains(rec.Body.String(), tt.wantBodyHas) {
				t.Errorf("body %q does not contain %q", rec.Body.String(), tt.wantBodyHas)
			}
			if tt.wantStatus == http.StatusNoContent {
				// The row survives so records that name the agent still resolve;
				// what must go is its visibility as something to use.
				live, err := store.ListAgentsByTeam(t.Context(), personalTeamID)
				if err != nil {
					t.Fatalf("ListAgentsByTeam: %v", err)
				}
				if len(live) != 0 {
					t.Errorf("after DELETE, team lists %d agents, want 0", len(live))
				}
				kept, err := store.GetAgentIncludingDeleted(t.Context(), "a_1")
				if err != nil {
					t.Fatalf("GetAgentIncludingDeleted: %v", err)
				}
				if kept == nil || kept.DeletedAt == nil {
					t.Error("after DELETE, the agent row should remain with deleted_at set")
				}
			}
		})
	}
}
