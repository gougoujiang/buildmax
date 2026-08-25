package team

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	coreaudit "github.com/gougoujiang/buildmax/internal/core/audit"
	coreteam "github.com/gougoujiang/buildmax/internal/core/team"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/service/audit"
	"github.com/gougoujiang/buildmax/internal/testsupport"
)

// auditFixture returns a mux plus the store the handlers write into.
func auditFixture(t *testing.T) (*http.ServeMux, *mock.MockAuditStore, *mock.MockUserStore) {
	t.Helper()
	users := &mock.MockUserStore{}
	store := &mock.MockAuditStore{}
	h := New(Config{
		JWTSecret: matrixSecret,
		Teams: &mock.MockTeamStore{
			Teams: []coreteam.Team{{ID: matrixTeam, Name: "Matrix", CreatedBy: matrixOwner}},
			Members: []coreteam.Member{
				{TeamID: matrixTeam, UserID: matrixOwner, Role: coreteam.RoleOwner},
				{TeamID: matrixTeam, UserID: matrixMember, Role: coreteam.RoleMember},
			},
		},
		Users:  users,
		Agents: &mock.MockAgentStore{},
		Audits: store,
		Audit:  audit.NewRecorder(store),
	})
	mux := http.NewServeMux()
	h.Register(mux)
	return mux, store, users
}

// TestDenialIsRecorded covers the one action written on failure. A denial is
// what shows someone probing at a boundary, so it is the event most worth
// having and the easiest to forget.
func TestDenialIsRecorded(t *testing.T) {
	mux, store, _ := auditFixture(t)

	// A member attempting an admin-only action.
	req := httptest.NewRequest(http.MethodPost, "/api/teams/"+matrixTeam+"/agents", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT(matrixMember, matrixSecret))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}

	if len(store.Events) != 1 {
		t.Fatalf("got %d events, want the denial recorded: %+v", len(store.Events), store.Events)
	}
	got := store.Events[0]
	if got.Action != coreaudit.AccessDenied || got.ActorID != matrixMember || got.TeamID != matrixTeam {
		t.Errorf("denial event wrong: %+v", got)
	}
}

// TestAuditTrailIsOwnerOnly guards the read side. The trail names who was
// refused, which a colleague does not need to see.
func TestAuditTrailIsOwnerOnly(t *testing.T) {
	mux, store, _ := auditFixture(t)
	store.Events = []coreaudit.Event{
		{ID: "ae_1", TeamID: matrixTeam, ActorType: coreaudit.ActorUser, ActorID: matrixOwner, Action: coreaudit.TeamMemberAdded, CreatedAt: time.Unix(1, 0).UTC()},
	}

	for _, tc := range []struct {
		user string
		want int
	}{
		{matrixOwner, http.StatusOK},
		{matrixMember, http.StatusForbidden},
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/teams/"+matrixTeam+"/audit-events", nil)
		req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT(tc.user, matrixSecret))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("%s got %d, want %d", tc.user, rec.Code, tc.want)
		}
	}
}

// TestAuditTrailCarriesNoContent is the privacy boundary. This table is
// evidence that an action occurred, not a copy of what was said or produced —
// those have different retention answers and live elsewhere.
func TestAuditTrailCarriesNoContent(t *testing.T) {
	mux, store, _ := auditFixture(t)
	store.Events = []coreaudit.Event{{
		ID: "ae_1", TeamID: matrixTeam, ActorType: coreaudit.ActorUser,
		ActorID: matrixOwner, Action: coreaudit.ModelCreated,
		TargetType: "llm_model", TargetID: "lm_1", Detail: "fast", CreatedAt: time.Unix(1, 0).UTC(),
	}}

	req := httptest.NewRequest(http.MethodGet, "/api/teams/"+matrixTeam+"/audit-events", nil)
	req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT(matrixOwner, matrixSecret))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var got AuditEventsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Events) != 1 || got.Total != 1 {
		t.Fatalf("got %+v", got)
	}
	// The event type has no field for a prompt, an endpoint, or a credential,
	// and this asserts the wire shape keeps it that way.
	for _, banned := range []string{"prompt", "content", "api_key", "endpoint", "credential"} {
		if strings.Contains(strings.ToLower(rec.Body.String()), banned) {
			t.Errorf("audit response carries a %q field: %s", banned, rec.Body.String())
		}
	}
}
