package client

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The inbox asks each team the same question and keeps the team alongside each
// issue, because a local surface has no current team to put back later.
func TestListAssignedIssuesCarriesTheTeam(t *testing.T) {
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/teams":
			_, _ = w.Write([]byte(`[{"id":"tm_1","name":"Platform"},{"id":"tm_2","name":"Data"}]`))
		case strings.HasSuffix(r.URL.Path, "/issues"):
			asked = append(asked, r.URL.Path+"?"+r.URL.RawQuery)
			if strings.Contains(r.URL.Path, "tm_1") {
				_, _ = w.Write([]byte(`{"issues":[{"id":"i_1","title":"Ship it","status":"todo"}],"total":1}`))
				return
			}
			_, _ = w.Write([]byte(`{"issues":[],"total":0}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	issues, problems := NewClient(srv.URL).ListAssignedIssues(t.Context(), "tok", "todo", 25)
	if len(problems) != 0 {
		t.Fatalf("problems = %v", problems)
	}
	if len(issues) != 1 || issues[0].Issue.ID != "i_1" {
		t.Fatalf("issues = %+v", issues)
	}
	if issues[0].TeamID != "tm_1" || issues[0].TeamName != "Platform" {
		t.Fatalf("team lost: %+v", issues[0])
	}
	if len(asked) != 2 {
		t.Fatalf("asked %d teams, want 2: %v", len(asked), asked)
	}
	for _, url := range asked {
		if !strings.Contains(url, "assignee=me") || !strings.Contains(url, "status=todo") {
			t.Fatalf("query lost a filter: %s", url)
		}
	}
}

// One unreadable team must not empty the inbox. The caller is told which team
// failed and still sees the rest.
func TestListAssignedIssuesSkipsATeamItCannotRead(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/teams":
			_, _ = w.Write([]byte(`[{"id":"tm_1","name":"Platform"},{"id":"tm_gone","name":"Archived"}]`))
		case strings.Contains(r.URL.Path, "tm_gone"):
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"not a member"}`))
		default:
			_, _ = w.Write([]byte(`{"issues":[{"id":"i_1","title":"Ship it","status":"todo"}],"total":1}`))
		}
	}))
	defer srv.Close()

	issues, problems := NewClient(srv.URL).ListAssignedIssues(t.Context(), "tok", "", 0)
	if len(issues) != 1 {
		t.Fatalf("one team failing emptied the inbox: %+v", issues)
	}
	if len(problems) != 1 || !strings.Contains(problems[0].Error(), "Archived") {
		t.Fatalf("problems = %v, want one naming the team", problems)
	}
}

// A server that cannot even list teams has no inbox to show, and says so rather
// than reporting an empty one.
func TestListAssignedIssuesReportsATeamListingFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"token expired"}`))
	}))
	defer srv.Close()

	issues, problems := NewClient(srv.URL).ListAssignedIssues(t.Context(), "tok", "", 0)
	if len(issues) != 0 || len(problems) != 1 {
		t.Fatalf("issues = %v, problems = %v", issues, problems)
	}
	if !strings.Contains(problems[0].Error(), "list teams") {
		t.Fatalf("problem does not say what failed: %v", problems[0])
	}
}
