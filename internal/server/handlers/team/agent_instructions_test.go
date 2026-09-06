package team

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	coreteam "github.com/gougoujiang/buildmax/internal/core/team"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/testsupport"
)

const agentInstructionsSecret = "agent-instructions-test-secret"

func TestTeamAgentInstructionsRoundTripAndPermissions(t *testing.T) {
	teamID := "tm_1"
	teams := &mock.MockTeamStore{
		Teams: []coreteam.Team{{ID: teamID, Name: "Team", CreatedBy: "u_owner"}},
		Members: []coreteam.Member{
			{TeamID: teamID, UserID: "u_owner", Role: coreteam.RoleOwner},
			{TeamID: teamID, UserID: "u_admin", Role: coreteam.RoleAdmin},
			{TeamID: teamID, UserID: "u_member", Role: coreteam.RoleMember},
		},
	}
	h := New(Config{JWTSecret: agentInstructionsSecret, Teams: teams})
	mux := http.NewServeMux()
	h.Register(mux)
	call := func(method, userID, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/teams/"+teamID+"/agent-instructions", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT(userID, agentInstructionsSecret))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	if rec := call(http.MethodPut, "u_member", `{"instructions":"no"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("member PUT status = %d, want 403: %s", rec.Code, rec.Body.String())
	}
	if rec := call(http.MethodPut, "u_admin", `{"instructions":"  Use British English.  "}`); rec.Code != http.StatusOK {
		t.Fatalf("admin PUT status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	rec := call(http.MethodGet, "u_member", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("member GET status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var got agentInstructionsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Instructions != "Use British English." || got.Revision != 1 {
		t.Fatalf("response = %+v, want trimmed instructions at revision 1", got)
	}

	if rec := call(http.MethodPut, "u_owner", `{"instructions":"Use British English."}`); rec.Code != http.StatusOK {
		t.Fatalf("owner no-op PUT status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if teams.Teams[0].AgentInstructionsRevision != 1 {
		t.Fatalf("no-op revision = %d, want 1", teams.Teams[0].AgentInstructionsRevision)
	}
}

func TestTeamAgentInstructionsRejectsOversizeText(t *testing.T) {
	teamID := "tm_1"
	teams := &mock.MockTeamStore{
		Teams:   []coreteam.Team{{ID: teamID, Name: "Team", CreatedBy: "u_owner"}},
		Members: []coreteam.Member{{TeamID: teamID, UserID: "u_owner", Role: coreteam.RoleOwner}},
	}
	h := New(Config{JWTSecret: agentInstructionsSecret, Teams: teams})
	mux := http.NewServeMux()
	h.Register(mux)
	body := `{"instructions":"` + strings.Repeat("界", 8193) + `"}`
	req := httptest.NewRequest(http.MethodPut, "/api/teams/"+teamID+"/agent-instructions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT("u_owner", agentInstructionsSecret))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}
