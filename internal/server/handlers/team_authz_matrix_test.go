package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	coreidentity "github.com/gougoujiang/buildmax/internal/core/identity"
	coreteam "github.com/gougoujiang/buildmax/internal/core/team"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/testsupport"
)

// Every route under /api/teams/{team_id} is a team-scoped resource, and Team is
// the authorization boundary for all of them. These tests drive real requests
// through the mux for each role and for two kinds of stranger, because the
// checks live in handler helpers rather than in one middleware — so a route
// that forgets to call one is not a compile error, and would otherwise be
// caught by nobody.
//
// Driving real requests rather than unit-testing core/team.Allows is
// deliberate, and stays deliberate now that Allows is the only implementation
// of the rules. What this proves is not what a role may do -- core/team's own
// table proves that -- but that each route asks. A rule with one owner that
// nobody consults on a route is still an open route.

const (
	matrixSecret    = "matrix-secret"
	matrixTeam      = "tm_matrix"
	matrixOther     = "tm_other"
	matrixOwner     = "u_owner"
	matrixAdmin     = "u_admin"
	matrixMember    = "u_member"
	matrixUnsetRole = "u_unset_role"
	matrixOutside   = "u_outsider"
)

// authzCase is one route and the least privileged role that may reach its
// handler.
type authzCase struct {
	method string
	path   string
	// minRole is TeamRoleMember, TeamRoleAdmin, or TeamRoleOwner.
	minRole string
	// tokenInQuery marks a route that reads its credential from ?token=
	// instead of the Authorization header. A browser cannot set headers on a
	// WebSocket handshake, so the upgrade route has no other option.
	tokenInQuery bool
}

// teamRoutes is the authorization matrix. Every team-scoped route the server registers
// must appear here — TestAuthzMatrixCoversEveryTeamRoute fails otherwise, so a
// new route cannot ship without someone deciding who may call it.
var teamRoutes = []authzCase{
	{"GET", "/api/teams/{team_id}/agents", coreteam.RoleMember, false},
	{"POST", "/api/teams/{team_id}/agents", coreteam.RoleAdmin, false},
	{"GET", "/api/teams/{team_id}/agents/{agent_id}", coreteam.RoleMember, false},
	{"PATCH", "/api/teams/{team_id}/agents/{agent_id}", coreteam.RoleAdmin, false},
	{"DELETE", "/api/teams/{team_id}/agents/{agent_id}", coreteam.RoleAdmin, false},
	{"GET", "/api/teams/{team_id}/agents/{agent_id}/revisions", coreteam.RoleMember, false},
	{"POST", "/api/teams/{team_id}/agents/{agent_id}/revisions/{revision}/restore", coreteam.RoleAdmin, false},

	// Reading what a team activated answers "why did this run have this
	// plugin", which is any member's question. Changing an activation is the
	// same authority the team's other shared automation needs.
	{"GET", "/api/teams/{team_id}/plugin-activations", coreteam.RoleMember, false},
	{"POST", "/api/teams/{team_id}/plugin-activations", coreteam.RoleAdmin, false},
	{"PATCH", "/api/teams/{team_id}/plugin-activations/{plugin_name}", coreteam.RoleAdmin, false},
	{"PUT", "/api/teams/{team_id}/plugin-curation", coreteam.RoleAdmin, false},

	{"GET", "/api/teams/{team_id}/members", coreteam.RoleMember, false},
	{"DELETE", "/api/teams/{team_id}/members/{user_id}", coreteam.RoleOwner, false},
	// Role change (including the ownership transfer that results from
	// setting a target's role to owner) and team-scoped access recovery are
	// both owner-only -- see docs/design/team-membership-lifecycle.md §5.2,
	// §5.3, §5.4, and §7.
	{"PATCH", "/api/teams/{team_id}/members/{user_id}", coreteam.RoleOwner, false},
	{"POST", "/api/teams/{team_id}/members/{user_id}/login-code", coreteam.RoleOwner, false},

	// Invitation is the one membership action admin holds, at member role
	// only -- see docs/design/team-membership-lifecycle.md §5.1 and §7. That
	// role-content restriction is enforced by the service, not the route, so
	// this matrix -- which drives one representative request per case -- still
	// reads minRole as admin: an admin's request with no role in the body
	// defaults to member and succeeds.
	{"POST", "/api/teams/{team_id}/invitations", coreteam.RoleAdmin, false},
	{"GET", "/api/teams/{team_id}/invitations", coreteam.RoleAdmin, false},
	{"DELETE", "/api/teams/{team_id}/invitations/{invitation_id}", coreteam.RoleAdmin, false},

	{"GET", "/api/teams/{team_id}/conversations", coreteam.RoleMember, false},
	{"POST", "/api/teams/{team_id}/conversations", coreteam.RoleMember, false},
	{"GET", "/api/teams/{team_id}/conversations/{conversation_id}/messages", coreteam.RoleMember, false},
	{"POST", "/api/teams/{team_id}/conversations/{conversation_id}/messages", coreteam.RoleMember, false},
	{"GET", "/api/teams/{team_id}/conversations/{conversation_id}/tasks", coreteam.RoleMember, false},
	{"POST", "/api/teams/{team_id}/conversations/{conversation_id}/tasks", coreteam.RoleMember, false},

	{"GET", "/api/teams/{team_id}/issues", coreteam.RoleMember, false},
	{"POST", "/api/teams/{team_id}/issues", coreteam.RoleMember, false},
	{"GET", "/api/teams/{team_id}/issues/{issue_id}", coreteam.RoleMember, false},
	{"PATCH", "/api/teams/{team_id}/issues/{issue_id}", coreteam.RoleMember, false},
	{"GET", "/api/teams/{team_id}/issues/{issue_id}/flow", coreteam.RoleMember, false},
	// Commenting is collaboration, so every member may do it. Moderation —
	// deleting a comment you did not write — is owner-only, but that is decided
	// inside the handler from the comment's author, not by the route: a member
	// deleting their own comment reaches the same endpoint.
	{"GET", "/api/teams/{team_id}/issues/{issue_id}/comments", coreteam.RoleMember, false},
	{"POST", "/api/teams/{team_id}/issues/{issue_id}/comments", coreteam.RoleMember, false},
	{"PATCH", "/api/teams/{team_id}/issues/{issue_id}/comments/{comment_id}", coreteam.RoleMember, false},
	{"DELETE", "/api/teams/{team_id}/issues/{issue_id}/comments/{comment_id}", coreteam.RoleMember, false},
	{"POST", "/api/teams/{team_id}/issues/{issue_id}/agent-runs", coreteam.RoleMember, false},
	{"POST", "/api/teams/{team_id}/issues/{issue_id}/workflow-runs", coreteam.RoleMember, false},

	{"GET", "/api/teams/{team_id}/workflows", coreteam.RoleMember, false},
	{"POST", "/api/teams/{team_id}/workflows", coreteam.RoleAdmin, false},
	{"GET", "/api/teams/{team_id}/workflows/{workflow_id}", coreteam.RoleMember, false},
	{"PATCH", "/api/teams/{team_id}/workflows/{workflow_id}", coreteam.RoleAdmin, false},
	{"GET", "/api/teams/{team_id}/workflows/{workflow_id}/revisions", coreteam.RoleMember, false},
	{"POST", "/api/teams/{team_id}/workflows/{workflow_id}/revisions/{revision}/restore", coreteam.RoleAdmin, false},
	{"GET", "/api/teams/{team_id}/workflows/{workflow_id}/runs", coreteam.RoleMember, false},
	{"POST", "/api/teams/{team_id}/workflows/{workflow_id}/runs", coreteam.RoleMember, false},
	{"GET", "/api/teams/{team_id}/workflow-runs/{workflow_run_id}", coreteam.RoleMember, false},

	{"GET", "/api/teams/{team_id}/tasks/{task_id}", coreteam.RoleMember, false},
	{"POST", "/api/teams/{team_id}/tasks/{task_id}/runs", coreteam.RoleMember, false},
	// Starting a run and stopping one are the same level of act on the same
	// resource. A member who may spend the team's budget may also stop
	// spending it, including on a run somebody else started.
	{"POST", "/api/teams/{team_id}/tasks/{task_id}/cancel", coreteam.RoleMember, false},
	// Retry starts a run, so it sits at the same level as starting one.
	{"POST", "/api/teams/{team_id}/tasks/{task_id}/retry", coreteam.RoleMember, false},
	{"GET", "/api/teams/{team_id}/tasks/{task_id}/artifacts", coreteam.RoleMember, false},
	// Unified artifacts. Any member may keep a file for the team and see what
	// the team holds; removing one is decided per artifact rather than per
	// role, so it is not on a team-scoped route -- see the artifact package.
	{"GET", "/api/teams/{team_id}/artifacts", coreteam.RoleMember, false},
	{"POST", "/api/teams/{team_id}/artifacts", coreteam.RoleMember, false},
	{"GET", "/api/teams/{team_id}/tasks/{task_id}/conversation", coreteam.RoleMember, false},
	{"GET", "/api/teams/{team_id}/tasks/{task_id}/stream", coreteam.RoleMember, false},

	{"GET", "/api/teams/{team_id}/task-runs/{task_run_id}", coreteam.RoleMember, false},
	{"GET", "/api/teams/{team_id}/task-runs/{task_run_id}/artifacts/items", coreteam.RoleMember, false},
	{"GET", "/api/teams/{team_id}/task-runs/{task_run_id}/artifacts/content", coreteam.RoleMember, false},
	{"GET", "/api/teams/{team_id}/task-runs/{task_run_id}/trace", coreteam.RoleMember, false},
	// A member may read what their team's run spent, the same as its trace and
	// artifacts. The ledger carries no prompts, and hiding a team's own usage
	// from the people producing it would make quota unexplainable.
	{"GET", "/api/teams/{team_id}/task-runs/{task_run_id}/llm-calls", coreteam.RoleMember, false},

	{"GET", "/api/teams/{team_id}/files", coreteam.RoleMember, false},
	{"GET", "/api/teams/{team_id}/files/{path...}", coreteam.RoleMember, false},
	{"POST", "/api/teams/{team_id}/upload", coreteam.RoleMember, false},

	{"GET", "/api/teams/{team_id}/usage", coreteam.RoleMember, false},
	// Owner only: the trail names who did what, including who was refused,
	// which is administrative rather than collaborative information.
	{"GET", "/api/teams/{team_id}/audit-events", coreteam.RoleOwner, false},
	// The export is the same read in a file, so it is the same reader.
	{"GET", "/api/teams/{team_id}/audit-events/export", coreteam.RoleOwner, false},
	// The managed gateway is deliberately absent: it is not team-scoped. Every
	// catalog model is available to every signed-in user, so its routes carry no
	// team and are covered by the gateway's own tests.

	{"GET", "/api/teams/{team_id}/ws", coreteam.RoleMember, true},
}

// matrixMux builds a handler with every store wired.
//
// A nil store answers 503 before any authorization check runs, which would turn
// a missing denial into a passing test. Wiring them all is what makes a 403
// assertion mean what it says.
func matrixMux(t *testing.T) *http.ServeMux {
	t.Helper()
	return matrixMuxWithGrants(t, nil)
}

// matrixMuxWithGrants is matrixMux with a deployment-scoped grant store, so
// the team matrix can also be driven by a system administrator. That caller
// must be refused by every route here — see TestSystemGrantIsNotATeamKey.
func matrixMuxWithGrants(t *testing.T, grants coreidentity.SystemGrantStore) *http.ServeMux {
	t.Helper()
	teams := &mock.MockTeamStore{
		Teams: []coreteam.Team{
			{ID: matrixTeam, Name: "Matrix", CreatedBy: matrixOwner},
			{ID: matrixOther, Name: "Other", CreatedBy: matrixOutside},
		},
		Members: []coreteam.Member{
			{TeamID: matrixTeam, UserID: matrixOwner, Role: coreteam.RoleOwner},
			{TeamID: matrixTeam, UserID: matrixAdmin, Role: coreteam.RoleAdmin},
			{TeamID: matrixTeam, UserID: matrixMember, Role: coreteam.RoleMember},
			// A membership row that never got a role. Nothing writes one now --
			// the team service defaults an unset role before storing it -- so
			// this stands in for a legacy row, and pins what such a row may do.
			{TeamID: matrixTeam, UserID: matrixUnsetRole, Role: ""},
			// The stranger owns a different team, which is the case that
			// separates "is a member of something" from "is a member of this".
			{TeamID: matrixOther, UserID: matrixOutside, Role: coreteam.RoleOwner},
		},
	}
	conversations := &mock.MockConversationStore{}
	h := NewHandler(Config{
		JWTSecret:                matrixSecret,
		LLMGateway:               llmTestService(t, &llmStubClient{content: "ok"}, nil),
		TeamStore:                teams,
		UserStore:                &mock.MockUserStore{},
		AgentStore:               &mock.MockAgentStore{},
		IssueStore:               &mock.MockIssueStore{},
		IssueCommentStore:        &mock.MockIssueCommentStore{},
		WorkflowStore:            &mock.MockWorkflowStore{},
		TaskStore:                &mock.MockTaskStore{},
		TaskRunStore:             &mock.MockTaskRunStore{},
		RunOutputLister:          &mock.MockRunOutputLister{},
		ConversationStore:        conversations,
		ConversationMessageStore: &mock.MockConversationMessageStore{},
		AuditStore:               &mock.MockAuditStore{},
		SystemGrantStore:         grants,
		LoginCodeStore:           &mock.MockLoginCodeStore{},
		PersistStorage:           mock.NewMockPersistStorage(),
		RunOutputStorage:         mock.NewMockRunOutputStorage(),
		ArtifactStore:            &mock.MockArtifactStore{},
		ArtifactStorage:          mock.NewMockArtifactStorage(),
		WorkspacesDir:            t.TempDir(),
	})
	mux := http.NewServeMux()
	h.Register(mux)
	return mux
}

// requestAs issues one request for the matrix. An empty user means no token.
func requestAs(t *testing.T, mux *http.ServeMux, c authzCase, teamID, userID string) int {
	t.Helper()
	path := strings.ReplaceAll(c.path, "{team_id}", teamID)
	// Remaining path parameters are filled with ids that do not exist. The
	// resource is irrelevant: authorization is decided before it is looked up,
	// and a 404 for a missing object still proves the caller got past the gate.
	path = regexp.MustCompile(`\{[^}]+\}`).ReplaceAllString(path, "nonexistent")
	if c.tokenInQuery && userID != "" {
		path += "?token=" + testsupport.SignJWT(userID, matrixSecret)
	}
	req := httptest.NewRequest(c.method, path, strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	if userID != "" && !c.tokenInQuery {
		req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT(userID, matrixSecret))
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec.Code
}

func allowedFor(minRole, role string) bool {
	switch minRole {
	case coreteam.RoleOwner:
		return role == coreteam.RoleOwner
	case coreteam.RoleAdmin:
		return role == coreteam.RoleOwner || role == coreteam.RoleAdmin
	default:
		return true
	}
}

// TestTeamAuthzMatrix drives every team-scoped route as an owner, an admin, a
// member, a member of a different team, and an anonymous caller.
func TestTeamAuthzMatrix(t *testing.T) {
	mux := matrixMux(t)

	roles := map[string]string{
		matrixOwner:  coreteam.RoleOwner,
		matrixAdmin:  coreteam.RoleAdmin,
		matrixMember: coreteam.RoleMember,
		// A row with no role is a member, so it is driven through every route
		// against the same expectations. It used to be refused everything.
		matrixUnsetRole: coreteam.RoleMember,
	}

	for _, c := range teamRoutes {
		t.Run(c.method+" "+c.path, func(t *testing.T) {
			// Nobody outside the team gets in, whatever the route does.
			if got := requestAs(t, mux, c, matrixTeam, matrixOutside); got != http.StatusForbidden {
				t.Errorf("a member of another team got %d, want 403", got)
			}
			if got := requestAs(t, mux, c, matrixTeam, ""); got != http.StatusUnauthorized {
				t.Errorf("an anonymous caller got %d, want 401", got)
			}
			// Naming someone else's team must not work either, even for a user
			// who is a legitimate member of their own.
			if got := requestAs(t, mux, c, matrixOther, matrixMember); got != http.StatusForbidden {
				t.Errorf("a member reaching into another team got %d, want 403", got)
			}

			for userID, role := range roles {
				got := requestAs(t, mux, c, matrixTeam, userID)
				if allowedFor(c.minRole, role) {
					if got == http.StatusForbidden || got == http.StatusUnauthorized {
						t.Errorf("%s may call this route but got %d", role, got)
					}
					continue
				}
				if got != http.StatusForbidden {
					t.Errorf("%s must not call this route (needs %s) but got %d", role, c.minRole, got)
				}
			}
		})
	}
}

// TestAuthzMatrixCoversEveryTeamRoute reads every route registration under
// this package and fails when a team-scoped route has no entry above.
//
// Without this the matrix silently stops being a matrix: a new route ships,
// nobody notices it was never assigned a minimum role, and the gap looks
// exactly like coverage.
func TestAuthzMatrixCoversEveryTeamRoute(t *testing.T) {
	// Team-scoped routes are registered by each context package's own Register
	// method, so the matrix reads every one of them rather than the file that
	// happened to hold them all before the split.
	var body []byte
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		part, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		body = append(body, part...)
		return nil
	})
	if err != nil {
		t.Fatalf("read the route tables: %v", err)
	}
	found := regexp.MustCompile(`"(GET|POST|PATCH|PUT|DELETE) (/api/teams/\{team_id\}[^"]*)"`).
		FindAllStringSubmatch(string(body), -1)
	if len(found) == 0 {
		t.Fatal("no team-scoped routes found; the pattern this test relies on has changed")
	}

	covered := make(map[string]bool, len(teamRoutes))
	for _, c := range teamRoutes {
		covered[c.method+" "+c.path] = true
	}
	var missing []string
	registered := make(map[string]bool, len(found))
	for _, m := range found {
		key := m[1] + " " + m[2]
		registered[key] = true
		if !covered[key] {
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)
	for _, key := range missing {
		t.Errorf("%s is registered but has no authorization case; decide who may call it", key)
	}

	// The reverse: an entry for a route that no longer exists is dead weight
	// that reads as coverage.
	for _, c := range teamRoutes {
		key := c.method + " " + c.path
		if !registered[key] {
			t.Errorf("%s has an authorization case but is registered nowhere", key)
		}
	}
}
