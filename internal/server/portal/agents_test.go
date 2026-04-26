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

const agentTestSecret = "agent-test-secret"

func TestPatchAgentHandler(t *testing.T) {
	personalTeamID := "tm_personal_u1"
	agentStore := &testutil.MockAgentStore{
		Agents: []entity.Agent{
			{AgentID: "a_1", UserID: "u1", TeamID: personalTeamID, Name: "Old", Description: "d1", Instructions: "i1", CreatedAt: 100},
		},
	}
	teamStore := &testutil.MockTeamStore{
		Teams:   []entity.Team{{TeamID: personalTeamID, Name: "My Space", PersonalForUserID: testutil.PtrString("u1"), CreatedBy: "u1"}},
		Members: []entity.TeamMember{{TeamID: personalTeamID, UserID: "u1", Role: entity.TeamRoleOwner}, {TeamID: personalTeamID, UserID: "u2", Role: entity.TeamRoleMember}},
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
			authHeader:  "Bearer " + testutil.SignJWT("u1", agentTestSecret),
			wantStatus:  http.StatusOK,
			wantBodyHas: "Updated",
		},
		{
			name:        "PATCH empty name returns 400",
			method:      http.MethodPatch,
			url:         "/api/teams/" + personalTeamID + "/agents/a_1",
			body:        `{"name":""}`,
			authHeader:  "Bearer " + testutil.SignJWT("u1", agentTestSecret),
			wantStatus:  http.StatusBadRequest,
			wantBodyHas: "name required",
		},
		{
			name:        "PATCH forbidden for member",
			method:      http.MethodPatch,
			url:         "/api/teams/" + personalTeamID + "/agents/a_1",
			body:        `{"name":"Updated","description":"d2","instructions":"i2"}`,
			authHeader:  "Bearer " + testutil.SignJWT("u2", agentTestSecret),
			wantStatus:  http.StatusForbidden,
			wantBodyHas: "forbidden",
		},
		{
			name:        "PATCH non-existent agent returns 404",
			method:      http.MethodPatch,
			url:         "/api/teams/" + personalTeamID + "/agents/a_999",
			body:        `{"name":"X","description":"","instructions":""}`,
			authHeader:  "Bearer " + testutil.SignJWT("u1", agentTestSecret),
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
			authHeader: "Bearer " + testutil.SignJWT("u1", agentTestSecret),
			wantStatus: http.StatusNoContent,
		},
		{
			name:        "DELETE non-existent agent returns 404",
			url:         "/api/teams/" + personalTeamID + "/agents/a_999",
			authHeader:  "Bearer " + testutil.SignJWT("u1", agentTestSecret),
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
			store := &testutil.MockAgentStore{
				Agents: []entity.Agent{
					{AgentID: "a_1", UserID: "u1", TeamID: personalTeamID, Name: "ToDelete", CreatedAt: 100},
				},
			}
			teamStore := &testutil.MockTeamStore{
				Teams:   []entity.Team{{TeamID: personalTeamID, Name: "My Space", PersonalForUserID: testutil.PtrString("u1"), CreatedBy: "u1"}},
				Members: []entity.TeamMember{{TeamID: personalTeamID, UserID: "u1", Role: entity.TeamRoleOwner}, {TeamID: personalTeamID, UserID: "u2", Role: entity.TeamRoleMember}},
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
			if tt.wantStatus == http.StatusNoContent && len(store.Agents) != 0 {
				t.Errorf("after DELETE, list has %d agents, want 0", len(store.Agents))
			}
		})
	}
}
