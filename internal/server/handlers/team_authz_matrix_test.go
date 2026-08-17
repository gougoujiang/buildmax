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

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/util"
)

// Every route under /api/teams/{team_id} is a team-scoped resource, and Team is
// the authorization boundary for all of them. These tests drive real requests
// through the mux for each role and for two kinds of stranger, because the
// checks live in handler helpers rather than in one middleware — so a route
// that forgets to call one is not a compile error, and would otherwise be
// caught by nobody.
//
// Driving real requests rather than unit-testing isRoleAllowed is deliberate:
// the role rules have two implementations. Most routes go through
// authorizeTeamAction, while member add and remove check for owner inline in
// teams.go. A test of the shared helper would pass while those two drifted.

const (
	matrixSecret  = "matrix-secret"
	matrixTeam    = "tm_matrix"
	matrixOther   = "tm_other"
	matrixOwner   = "u_owner"
	matrixAdmin   = "u_admin"
	matrixMember  = "u_member"
	matrixOutside = "u_outsider"
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

// teamRoutes is the authorization matrix. Every team-scoped route in routes.go
// must appear here — TestAuthzMatrixCoversEveryTeamRoute fails otherwise, so a
// new route cannot ship without someone deciding who may call it.
var teamRoutes = []authzCase{
	{"GET", "/api/teams/{team_id}/agents", model.TeamRoleMember, false},
	{"POST", "/api/teams/{team_id}/agents", model.TeamRoleAdmin, false},
	{"GET", "/api/teams/{team_id}/agents/{agent_id}", model.TeamRoleMember, false},
	{"PATCH", "/api/teams/{team_id}/agents/{agent_id}", model.TeamRoleAdmin, false},
	{"DELETE", "/api/teams/{team_id}/agents/{agent_id}", model.TeamRoleAdmin, false},

	{"GET", "/api/teams/{team_id}/members", model.TeamRoleMember, false},
	{"POST", "/api/teams/{team_id}/members", model.TeamRoleOwner, false},
	{"DELETE", "/api/teams/{team_id}/members/{user_id}", model.TeamRoleOwner, false},

	{"GET", "/api/teams/{team_id}/conversations", model.TeamRoleMember, false},
	{"POST", "/api/teams/{team_id}/conversations", model.TeamRoleMember, false},
	{"GET", "/api/teams/{team_id}/conversations/{conversation_id}/messages", model.TeamRoleMember, false},
	{"POST", "/api/teams/{team_id}/conversations/{conversation_id}/messages", model.TeamRoleMember, false},
	{"GET", "/api/teams/{team_id}/conversations/{conversation_id}/tasks", model.TeamRoleMember, false},
	{"POST", "/api/teams/{team_id}/conversations/{conversation_id}/tasks", model.TeamRoleMember, false},

	{"GET", "/api/teams/{team_id}/issues", model.TeamRoleMember, false},
	{"POST", "/api/teams/{team_id}/issues", model.TeamRoleMember, false},
	{"GET", "/api/teams/{team_id}/issues/{issue_id}", model.TeamRoleMember, false},
	{"PATCH", "/api/teams/{team_id}/issues/{issue_id}", model.TeamRoleMember, false},
	{"GET", "/api/teams/{team_id}/issues/{issue_id}/flow", model.TeamRoleMember, false},
	// Commenting is collaboration, so every member may do it. Moderation —
	// deleting a comment you did not write — is owner-only, but that is decided
	// inside the handler from the comment's author, not by the route: a member
	// deleting their own comment reaches the same endpoint.
	{"GET", "/api/teams/{team_id}/issues/{issue_id}/comments", model.TeamRoleMember, false},
	{"POST", "/api/teams/{team_id}/issues/{issue_id}/comments", model.TeamRoleMember, false},
	{"PATCH", "/api/teams/{team_id}/issues/{issue_id}/comments/{comment_id}", model.TeamRoleMember, false},
	{"DELETE", "/api/teams/{team_id}/issues/{issue_id}/comments/{comment_id}", model.TeamRoleMember, false},
	{"POST", "/api/teams/{team_id}/issues/{issue_id}/agent-runs", model.TeamRoleMember, false},
	{"POST", "/api/teams/{team_id}/issues/{issue_id}/workflow-runs", model.TeamRoleMember, false},

	{"GET", "/api/teams/{team_id}/workflows", model.TeamRoleMember, false},
	{"POST", "/api/teams/{team_id}/workflows", model.TeamRoleAdmin, false},
	{"GET", "/api/teams/{team_id}/workflows/{workflow_id}", model.TeamRoleMember, false},
	{"PATCH", "/api/teams/{team_id}/workflows/{workflow_id}", model.TeamRoleAdmin, false},
	{"GET", "/api/teams/{team_id}/workflows/{workflow_id}/runs", model.TeamRoleMember, false},
	{"POST", "/api/teams/{team_id}/workflows/{workflow_id}/runs", model.TeamRoleMember, false},
	{"GET", "/api/teams/{team_id}/workflow-runs/{workflow_run_id}", model.TeamRoleMember, false},

	{"GET", "/api/teams/{team_id}/tasks/{task_id}", model.TeamRoleMember, false},
	{"POST", "/api/teams/{team_id}/tasks/{task_id}/runs", model.TeamRoleMember, false},
	{"GET", "/api/teams/{team_id}/tasks/{task_id}/artifacts", model.TeamRoleMember, false},
	{"GET", "/api/teams/{team_id}/tasks/{task_id}/conversation", model.TeamRoleMember, false},
	{"GET", "/api/teams/{team_id}/tasks/{task_id}/stream", model.TeamRoleMember, false},

	{"GET", "/api/teams/{team_id}/task-runs/{task_run_id}/artifacts/items", model.TeamRoleMember, false},
	{"GET", "/api/teams/{team_id}/task-runs/{task_run_id}/artifacts/content", model.TeamRoleMember, false},
	{"GET", "/api/teams/{team_id}/task-runs/{task_run_id}/trace", model.TeamRoleMember, false},

	{"GET", "/api/teams/{team_id}/files", model.TeamRoleMember, false},
	{"GET", "/api/teams/{team_id}/files/{path...}", model.TeamRoleMember, false},
	{"POST", "/api/teams/{team_id}/upload", model.TeamRoleMember, false},

	{"GET", "/api/teams/{team_id}/usage", model.TeamRoleMember, false},
	// Owner only: the trail names who did what, including who was refused,
	// which is administrative rather than collaborative information.
	{"GET", "/api/teams/{team_id}/audit-events", model.TeamRoleOwner, false},
	{"GET", "/api/teams/{team_id}/llm/models", model.TeamRoleMember, false},
	{"POST", "/api/teams/{team_id}/llm/completions", model.TeamRoleMember, false},

	{"GET", "/api/teams/{team_id}/ws", model.TeamRoleMember, true},
}

// matrixMux builds a handler with every store wired.
//
// A nil store answers 503 before any authorization check runs, which would turn
// a missing denial into a passing test. Wiring them all is what makes a 403
// assertion mean what it says.
func matrixMux(t *testing.T) *http.ServeMux {
	t.Helper()
	teams := &mock.MockTeamStore{
		Teams: []model.Team{
			{TeamID: matrixTeam, Name: "Matrix", CreatedBy: matrixOwner},
			{TeamID: matrixOther, Name: "Other", CreatedBy: matrixOutside},
		},
		Members: []model.TeamMember{
			{TeamID: matrixTeam, UserID: matrixOwner, Role: model.TeamRoleOwner},
			{TeamID: matrixTeam, UserID: matrixAdmin, Role: model.TeamRoleAdmin},
			{TeamID: matrixTeam, UserID: matrixMember, Role: model.TeamRoleMember},
			// The stranger owns a different team, which is the case that
			// separates "is a member of something" from "is a member of this".
			{TeamID: matrixOther, UserID: matrixOutside, Role: model.TeamRoleOwner},
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
		ConversationMessageStore: &mockConversationMessageStore{},
		AuditStore:               &mock.MockAuditStore{},
		PersistStorage:           mock.NewMockPersistStorage(),
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
		path += "?token=" + util.SignJWT(userID, matrixSecret)
	}
	req := httptest.NewRequest(c.method, path, strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	if userID != "" && !c.tokenInQuery {
		req.Header.Set("Authorization", "Bearer "+util.SignJWT(userID, matrixSecret))
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec.Code
}

func allowedFor(minRole, role string) bool {
	switch minRole {
	case model.TeamRoleOwner:
		return role == model.TeamRoleOwner
	case model.TeamRoleAdmin:
		return role == model.TeamRoleOwner || role == model.TeamRoleAdmin
	default:
		return true
	}
}

// TestTeamAuthzMatrix drives every team-scoped route as an owner, an admin, a
// member, a member of a different team, and an anonymous caller.
func TestTeamAuthzMatrix(t *testing.T) {
	mux := matrixMux(t)

	roles := map[string]string{
		matrixOwner:  model.TeamRoleOwner,
		matrixAdmin:  model.TeamRoleAdmin,
		matrixMember: model.TeamRoleMember,
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

// TestAuthzMatrixCoversEveryTeamRoute reads routes.go and fails when a
// team-scoped route has no entry above.
//
// Without this the matrix silently stops being a matrix: a new route ships,
// nobody notices it was never assigned a minimum role, and the gap looks
// exactly like coverage.
func TestAuthzMatrixCoversEveryTeamRoute(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("routes.go"))
	if err != nil {
		t.Fatalf("read routes.go: %v", err)
	}
	found := regexp.MustCompile(`"(GET|POST|PATCH|PUT|DELETE) (/api/teams/\{team_id\}[^"]*)"`).
		FindAllStringSubmatch(string(body), -1)
	if len(found) == 0 {
		t.Fatal("no team-scoped routes found in routes.go; the pattern this test relies on has changed")
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
			t.Errorf("%s has an authorization case but is not registered in routes.go", key)
		}
	}
}
