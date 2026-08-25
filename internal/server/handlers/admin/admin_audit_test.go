package admin

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	coreaudit "github.com/gougoujiang/buildmax/internal/core/audit"
	coreidentity "github.com/gougoujiang/buildmax/internal/core/identity"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/service/audit"
)

func auditSearchMux(t *testing.T) (*http.ServeMux, *mock.MockAuditStore) {
	t.Helper()
	users := &mock.MockUserStore{}
	seedUser(t, users, adminUser, "admin@example.com")
	grants := &mock.MockSystemGrantStore{}
	grants.GrantForTest(adminUser, coreidentity.SystemRoleAdmin)

	audits := &mock.MockAuditStore{Events: []coreaudit.Event{
		// Deployment-scoped: no team-scoped reader can ever see these.
		{ID: "ae_1", ActorType: coreaudit.ActorUser, ActorID: "u_alice", Action: coreaudit.UserLogin, CreatedAt: time.Unix(100, 0).UTC()},
		{ID: "ae_2", ActorType: coreaudit.ActorSystem, ActorID: coreaudit.ActorOperator, Action: coreaudit.SystemAdminGranted, TargetID: "u_bob", CreatedAt: time.Unix(200, 0).UTC()},
		// Team-scoped, in two different teams.
		{ID: "ae_3", TeamID: "tm_one", ActorType: coreaudit.ActorUser, ActorID: "u_alice", Action: coreaudit.TeamMemberAdded, CreatedAt: time.Unix(300, 0).UTC()},
		{ID: "ae_4", TeamID: "tm_two", ActorType: coreaudit.ActorUser, ActorID: "u_carol", Action: coreaudit.AccessDenied, CreatedAt: time.Unix(400, 0).UTC()},
	}}

	h := New(Config{
		JWTSecret: testSecret,
		Grants:    grants,
		Users:     users,
		Teams:     &mock.MockTeamStore{},
		Audits:    audits,
		Audit:     audit.NewRecorder(audits),
	})
	mux := http.NewServeMux()
	h.Register(mux)
	return mux, audits
}

func searchAudit(t *testing.T, mux *http.ServeMux, query string) AdminAuditEventsResponse {
	t.Helper()
	rec := adminRequestAs(t, mux, adminCase{"GET", "/api/admin/audit-events" + query}, adminUser)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s got %d: %s", query, rec.Code, rec.Body.String())
	}
	var out AdminAuditEventsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

// TestAdminAuditSearchSeesWhatTheTeamRouteCannot is the reason this route
// exists. A login and a grant have no team, so the team-scoped read can never
// return them however it is called.
func TestAdminAuditSearchSeesWhatTheTeamRouteCannot(t *testing.T) {
	mux, audits := auditSearchMux(t)

	all := searchAudit(t, mux, "")
	if all.Total != 4 {
		t.Errorf("unfiltered search returned %d of 4", all.Total)
	}

	// The same store, asked the team-scoped question, cannot reach the
	// deployment-scoped events at all.
	teamOnly, _, err := audits.ListAuditEvents(t.Context(), "tm_one", 50, 0)
	if err != nil {
		t.Fatalf("ListAuditEvents: %v", err)
	}
	if len(teamOnly) != 1 || teamOnly[0].ID != "ae_3" {
		t.Errorf("the team-scoped read should see only its own team: %+v", teamOnly)
	}
}

// rfc3339 renders a seeded instant the way the route reads its bounds. The
// query carries RFC 3339, not epoch seconds.
func rfc3339(unix int64) string {
	return time.Unix(unix, 0).UTC().Format(time.RFC3339)
}

func TestAdminAuditSearchFilters(t *testing.T) {
	mux, _ := auditSearchMux(t)

	for _, tc := range []struct {
		name  string
		query string
		want  []string
	}{
		{"by team", "?team_id=tm_two", []string{"ae_4"}},
		{"by actor across teams", "?actor_id=u_alice", []string{"ae_1", "ae_3"}},
		{"by action", "?action=" + coreaudit.SystemAdminGranted, []string{"ae_2"}},
		{"since", "?since=" + rfc3339(300), []string{"ae_3", "ae_4"}},
		{"until", "?until=" + rfc3339(300), []string{"ae_1", "ae_2"}},
		{"a window", "?since=" + rfc3339(200) + "&until=" + rfc3339(400), []string{"ae_2", "ae_3"}},
		// The events no team-scoped reader can see, asked for on purpose. An
		// empty team_id already means "any team", so this needs its own word.
		{"deployment-scoped only", "?team_id=none", []string{"ae_1", "ae_2"}},
		{"a filter matching nothing", "?actor_id=u_nobody", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := searchAudit(t, mux, tc.query)
			if got.Total != len(tc.want) {
				t.Fatalf("total = %d, want %d: %+v", got.Total, len(tc.want), got.Events)
			}
			ids := make(map[string]bool, len(got.Events))
			for _, e := range got.Events {
				ids[e.ID] = true
			}
			for _, want := range tc.want {
				if !ids[want] {
					t.Errorf("%s missing from the result: %+v", want, got.Events)
				}
			}
		})
	}
}

// TestAdminAuditSearchIgnoresAMalformedTimestamp: a filter that rejects the
// whole request because a bound was unparseable makes an investigation harder
// than one that returns a wider window.
func TestAdminAuditSearchIgnoresAMalformedTimestamp(t *testing.T) {
	mux, _ := auditSearchMux(t)
	if got := searchAudit(t, mux, "?since=yesterday"); got.Total != 4 {
		t.Errorf("total = %d, want the unbounded 4", got.Total)
	}
}

// TestAdminAuditSearchCarriesNoContent: the trail holds who did what to which
// object, and searching it must not become a way to read across teams.
func TestAdminAuditSearchCarriesNoContent(t *testing.T) {
	mux, _ := auditSearchMux(t)
	rec := adminRequestAs(t, mux, adminCase{"GET", "/api/admin/audit-events"}, adminUser)
	body := rec.Body.String()
	for _, forbidden := range []string{"prompt", "message", "output", "content", "input"} {
		if strings.Contains(strings.ToLower(body), `"`+forbidden+`"`) {
			t.Errorf("the response carries a %q field: %s", forbidden, body)
		}
	}
}
