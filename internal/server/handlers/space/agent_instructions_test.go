package space

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	corespace "github.com/icloudbb/buildmax/internal/core/space"
	"github.com/icloudbb/buildmax/internal/mock"
	"github.com/icloudbb/buildmax/internal/testsupport"
)

const agentInstructionsSecret = "agent-instructions-test-secret"

func TestSpaceAgentInstructionsRoundTripAndPermissions(t *testing.T) {
	spaceID := "tm_1"
	spaces := &mock.MockSpaceStore{
		Spaces: []corespace.Space{{ID: spaceID, Name: "Space", CreatedBy: "u_owner"}},
		Members: []corespace.Member{
			{SpaceID: spaceID, UserID: "u_owner", Role: corespace.RoleOwner},
			{SpaceID: spaceID, UserID: "u_admin", Role: corespace.RoleAdmin},
			{SpaceID: spaceID, UserID: "u_member", Role: corespace.RoleMember},
		},
	}
	h := New(Config{JWTSecret: agentInstructionsSecret, Spaces: spaces})
	mux := http.NewServeMux()
	h.Register(mux)
	call := func(method, userID, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/spaces/"+spaceID+"/agent-instructions", strings.NewReader(body))
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
	if spaces.Spaces[0].AgentInstructionsRevision != 1 {
		t.Fatalf("no-op revision = %d, want 1", spaces.Spaces[0].AgentInstructionsRevision)
	}
}

func TestSpaceAgentInstructionsRejectsOversizeText(t *testing.T) {
	spaceID := "tm_1"
	spaces := &mock.MockSpaceStore{
		Spaces:  []corespace.Space{{ID: spaceID, Name: "Space", CreatedBy: "u_owner"}},
		Members: []corespace.Member{{SpaceID: spaceID, UserID: "u_owner", Role: corespace.RoleOwner}},
	}
	h := New(Config{JWTSecret: agentInstructionsSecret, Spaces: spaces})
	mux := http.NewServeMux()
	h.Register(mux)
	body := `{"instructions":"` + strings.Repeat("界", 8193) + `"}`
	req := httptest.NewRequest(http.MethodPut, "/api/spaces/"+spaceID+"/agent-instructions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT("u_owner", agentInstructionsSecret))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}
