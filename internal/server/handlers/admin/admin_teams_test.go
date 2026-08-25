package admin

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	coreidentity "github.com/gougoujiang/buildmax/internal/core/identity"
	coreteam "github.com/gougoujiang/buildmax/internal/core/team"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/service/audit"
)

func TestAdminTeamsList(t *testing.T) {
	mux := adminTeamsMux(t)
	rec := adminRequestAs(t, mux, adminCase{"GET", "/api/admin/teams"}, adminUser)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	var out AdminTeamsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Total != 2 || len(out.Teams) != 2 {
		t.Fatalf("got %d of %d teams: %+v", len(out.Teams), out.Total, out.Teams)
	}
	byID := map[string]AdminTeam{}
	for _, team := range out.Teams {
		byID[team.ID] = team
	}
	if got := byID["tm_shared"]; got.MemberCount != 2 || got.Personal || got.QuotaTier != "free_trial" {
		t.Errorf("shared team = %+v", got)
	}
	// A personal team is marked as one, so an operator counting "teams" is not
	// counting every account twice without knowing it.
	if got := byID["tm_personal"]; !got.Personal || got.MemberCount != 1 {
		t.Errorf("personal team = %+v", got)
	}
}

func TestAdminTeamDetailShowsMembershipNotContent(t *testing.T) {
	mux := adminTeamsMux(t)
	rec := adminRequestAs(t, mux, adminCase{"GET", "/api/admin/teams/tm_shared"}, adminUser)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	var detail AdminTeamDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(detail.Members) != 2 {
		t.Fatalf("members = %+v", detail.Members)
	}
	var owner AdminTeamMember
	for _, m := range detail.Members {
		if m.UserID == "u_alice" {
			owner = m
		}
	}
	if owner.Role != coreteam.RoleOwner || owner.Email != "alice@example.com" {
		t.Errorf("owner = %+v", owner)
	}
	// A membership naming an account the store does not have still lists, with
	// the id: refusing to describe the team would be worse.
	for _, m := range detail.Members {
		if m.UserID == "u_bob" && m.Email != "" {
			t.Errorf("u_bob has no account seeded; email should be empty: %+v", m)
		}
	}

	// The rule the design states: if a member wrote it or an agent produced
	// it, it is content — and none of it is here.
	body := strings.ToLower(rec.Body.String())
	for _, forbidden := range []string{"issue", "conversation", "artifact", "task", "workflow", "trace"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("the team detail mentions %q: %s", forbidden, rec.Body.String())
		}
	}
}

// TestAdminTeamDetailOnAnUnknownTeam is 404, not an empty team.
func TestAdminTeamDetailOnAnUnknownTeam(t *testing.T) {
	mux := adminTeamsMux(t)
	if got := adminRequestAs(t, mux, adminCase{"GET", "/api/admin/teams/tm_nobody"}, adminUser).Code; got != http.StatusNotFound {
		t.Errorf("got %d, want 404", got)
	}
}

// TestAdminTeamRoutesAreNotAWayIntoATeam: the routes describe a team from the
// outside, and reaching a team's own resources still needs membership. This is
// the same claim TestSystemGrantIsNotATeamKey makes, restated at the surface
// that most looks like an exception to it.

func adminTeamsMux(t *testing.T) *http.ServeMux {
	t.Helper()
	users := &mock.MockUserStore{}
	seedUser(t, users, adminUser, "admin@example.com")
	seedUser(t, users, "u_alice", "alice@example.com")
	grants := &mock.MockSystemGrantStore{}
	grants.GrantForTest(adminUser, coreidentity.SystemRoleAdmin)

	personalOf := "u_alice"
	teams := &mock.MockTeamStore{
		Teams: []coreteam.Team{
			{ID: "tm_shared", Name: "Platform", CreatedBy: "u_alice", QuotaTier: "free_trial", CreatedAt: time.Unix(200, 0).UTC()},
			{ID: "tm_personal", Name: "My Space", PersonalForUserID: &personalOf, CreatedBy: "u_alice", CreatedAt: time.Unix(100, 0).UTC()},
		},
		Members: []coreteam.Member{
			{TeamID: "tm_shared", UserID: "u_alice", Role: coreteam.RoleOwner},
			{TeamID: "tm_shared", UserID: "u_bob", Role: coreteam.RoleMember},
			{TeamID: "tm_personal", UserID: "u_alice", Role: coreteam.RoleOwner},
		},
	}
	audits := &mock.MockAuditStore{}
	h := New(Config{
		JWTSecret: testSecret,
		Grants:    grants,
		Users:     users,
		Teams:     teams,
		// Wired because a nil store answers 503 before any authorization check
		// runs, which would make TestAdminTeamRoutesAreNotAWayIntoATeam pass
		// for the wrong reason.
		Audits: audits,
		Audit:  audit.NewRecorder(audits),
	})
	mux := http.NewServeMux()
	h.Register(mux)
	return mux
}
