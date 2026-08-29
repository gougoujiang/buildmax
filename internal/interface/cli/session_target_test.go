package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/session"
	"github.com/gougoujiang/buildmax/internal/infra/sessionstore"
)

// seedProjectSession writes a session bundle belonging to projectID. It goes straight
// to the store because the point of these tests is what resolution does with
// membership that already exists, not how it comes to be recorded.
func seedProjectSession(t *testing.T, projectID, workspace string, createdAt time.Time) string {
	t.Helper()
	meta := session.NewMeta(session.NewID(), session.KindUser, createdAt)
	meta.ProjectID = projectID
	meta.Workspace = workspace
	if err := sessionstore.NewFileStore(config.SessionsDir()).Create(context.Background(), meta); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	return meta.ID
}

// projectDir makes a plain directory and returns it with the Project id it
// resolves to. A directory rather than a checkout keeps these tests about
// membership rather than about Git.
func projectDir(t *testing.T, name string) (string, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p, err := currentProject(context.Background(), dir)
	if err != nil {
		t.Fatalf("resolve project for %s: %v", dir, err)
	}
	return dir, p.ID
}

// The behaviour --continue was changed for: the newest session on the machine
// is almost never the one wanted, because it follows whichever repository was
// touched last.
func TestContinuePicksThisProjectsNewestSession(t *testing.T) {
	here, hereID := projectDir(t, "here")
	_, elsewhereID := projectDir(t, "elsewhere")

	now := time.Now()
	older := seedProjectSession(t, hereID, here, now.Add(-2*time.Hour))
	wanted := seedProjectSession(t, hereID, here, now.Add(-time.Hour))
	newestAnywhere := seedProjectSession(t, elsewhereID, here, now)

	target, err := resolveSessionTarget(context.Background(), "", true, here, false)
	if err != nil {
		t.Fatalf("resolveSessionTarget: %v", err)
	}
	switch target.SessionID {
	case wanted:
	case newestAnywhere:
		t.Fatal("--continue chose the newest session on the machine, not this project's")
	case older:
		t.Fatal("--continue chose an older session of this project")
	default:
		t.Fatalf("--continue chose %s, want %s", target.SessionID, wanted)
	}
}

func TestContinueWithNoSessionsHereIsRefused(t *testing.T) {
	empty, _ := projectDir(t, "empty")
	other, otherID := projectDir(t, "other")
	seedProjectSession(t, otherID, other, time.Now())

	if _, err := resolveSessionTarget(context.Background(), "", true, empty, false); err == nil {
		t.Fatal("--continue succeeded in a project with no sessions")
	}
}

// Refused rather than allowed with a warning: the session would go on to read
// this project's context and record its work here, and neither is undone by
// noticing afterwards.
func TestResumeAcrossProjectsIsRefused(t *testing.T) {
	owner, ownerID := projectDir(t, "owner")
	stranger, _ := projectDir(t, "stranger")
	id := seedProjectSession(t, ownerID, owner, time.Now())

	_, err := resolveSessionTarget(context.Background(), id, false, stranger, true)
	if err == nil {
		t.Fatal("resuming another project's session was allowed")
	}
	if !strings.Contains(err.Error(), id) {
		t.Errorf("error does not name the session: %v", err)
	}
	if !strings.Contains(err.Error(), ownerID) {
		t.Errorf("error does not name the project that owns it: %v", err)
	}
}

func TestResumeWithinTheSameProjectIsAllowed(t *testing.T) {
	dir, id := projectDir(t, "same")
	sessionID := seedProjectSession(t, id, dir, time.Now())

	target, err := resolveSessionTarget(context.Background(), sessionID, false, dir, true)
	if err != nil {
		t.Fatalf("resolveSessionTarget: %v", err)
	}
	if target.SessionID != sessionID {
		t.Errorf("session = %s, want %s", target.SessionID, sessionID)
	}
	if target.Workspace != dir {
		t.Errorf("workspace = %s, want the explicit override %s", target.Workspace, dir)
	}
}

// With no --workspace, a resumed session continues where the conversation was
// rather than wherever the terminal happens to be.
func TestResumeWithoutAWorkspaceReturnsToTheRecordedRoot(t *testing.T) {
	recorded, id := projectDir(t, "recorded")
	sessionID := seedProjectSession(t, id, recorded, time.Now())
	elsewhere, _ := projectDir(t, "elsewhere")

	target, err := resolveSessionTarget(context.Background(), sessionID, false, elsewhere, false)
	if err != nil {
		t.Fatalf("resolveSessionTarget: %v", err)
	}
	if target.Workspace != recorded {
		t.Errorf("workspace = %s, want the session's recorded root %s", target.Workspace, recorded)
	}
}

// A session from before Projects existed, or one a worker wrote, belongs
// nowhere local. Attaching it to whatever directory it is resumed in would be
// the path-coincidence membership this design removes.
func TestResumeOfAProjectlessSessionIsNotReassigned(t *testing.T) {
	dir, _ := projectDir(t, "unrelated")
	sessionID := seedProjectSession(t, "", "", time.Now())

	target, err := resolveSessionTarget(context.Background(), sessionID, false, dir, false)
	if err != nil {
		t.Fatalf("resolveSessionTarget: %v", err)
	}
	if target.SessionID != sessionID {
		t.Errorf("session = %s, want %s", target.SessionID, sessionID)
	}
	if target.Workspace != dir {
		t.Errorf("workspace = %s, want the current directory %s", target.Workspace, dir)
	}
}

// Neither flag given: nothing is resolved and nothing is refused, so starting a
// fresh session never depends on the session store being readable.
func TestNoResumeFlagsLeavesTheTargetAlone(t *testing.T) {
	target, err := resolveSessionTarget(context.Background(), "", false, "/some/dir", false)
	if err != nil {
		t.Fatalf("resolveSessionTarget: %v", err)
	}
	if target != (sessionTarget{Workspace: "/some/dir"}) {
		t.Errorf("target = %+v, want the workspace alone", target)
	}
}

// A checkout and its linked worktree are one Project, so a session started in
// one continues in the other.
func TestContinueSpansWorktreesOfOneRepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init"}, {"config", "user.email", "t@example.com"}, {"config", "user.name", "T"},
		{"commit", "--allow-empty", "-m", "initial"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	linked := filepath.Join(base, "linked")
	if out, err := exec.Command("git", "-C", repo, "worktree", "add", "-b", "wt", linked, "HEAD").CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v\n%s", err, out)
	}

	p, err := currentProject(context.Background(), repo)
	if err != nil {
		t.Fatalf("resolve project: %v", err)
	}
	sessionID := seedProjectSession(t, p.ID, repo, time.Now())

	target, err := resolveSessionTarget(context.Background(), "", true, linked, false)
	if err != nil {
		t.Fatalf("--continue from the worktree: %v", err)
	}
	if target.SessionID != sessionID {
		t.Errorf("--continue from the worktree chose %s, want the checkout's session %s", target.SessionID, sessionID)
	}
}

// The panel's scope is the picker half of the same rule --continue follows: a
// list that spans every repository on the machine is not a list of this
// project's work.
func TestSessionPanelScope(t *testing.T) {
	const mine, theirs = "hyzc3kqxa2vw7m4t9pbn", "q7wd4mnbz3vk8t2yjxs5"
	everything := []session.ItemSummary{
		{ID: "a", ProjectID: mine, Title: "mine one"},
		{ID: "b", ProjectID: theirs, Title: "theirs"},
		{ID: "c", ProjectID: mine, Title: "mine two"},
		{ID: "d", Title: "projectless"},
	}

	scoped := &slashSessionState{Everything: everything, ProjectID: mine, ProjectName: "repo"}
	applySessionScope(scoped)
	if got := len(scoped.Filtered); got != 2 {
		t.Errorf("scoped view holds %d sessions, want this project's 2: %+v", got, scoped.Filtered)
	}

	scoped.AllProjects = true
	applySessionScope(scoped)
	if got := len(scoped.Filtered); got != len(everything) {
		t.Errorf("all-projects view holds %d sessions, want %d", got, len(everything))
	}

	// A run with no Project has nothing to narrow to, so it opens on everything
	// rather than on an empty list it cannot explain.
	projectless := &slashSessionState{Everything: everything, AllProjects: true}
	applySessionScope(projectless)
	if got := len(projectless.Filtered); got != len(everything) {
		t.Errorf("projectless view holds %d sessions, want %d", got, len(everything))
	}
}

// Project name is searchable only where it is shown. In the scoped view every
// row shares one project, so matching on it would return the whole list for a
// query that looks like it found something.
func TestSessionPanelSearchesProjectNamesOnlyAcrossProjects(t *testing.T) {
	const mine, theirs = "hyzc3kqxa2vw7m4t9pbn", "q7wd4mnbz3vk8t2yjxs5"
	st := &slashSessionState{
		Everything: []session.ItemSummary{
			{ID: "a", ProjectID: mine, Title: "one"},
			{ID: "b", ProjectID: theirs, Title: "two"},
		},
		ProjectID:    mine,
		ProjectName:  "buildmax",
		ProjectNames: map[string]string{mine: "buildmax", theirs: "notes"},
		AllProjects:  true,
		Query:        "notes",
	}
	applySessionScope(st)
	if len(st.Filtered) != 1 || st.Filtered[0].ID != "b" {
		t.Errorf("searching a project name matched %+v, want the session in that project", st.Filtered)
	}

	st.AllProjects = false
	applySessionScope(st)
	if len(st.Filtered) != 0 {
		t.Errorf("scoped search on another project's name matched %+v, want nothing", st.Filtered)
	}
}
