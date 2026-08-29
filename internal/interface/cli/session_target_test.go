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

// gitRepoWithWorktree returns a checkout and a linked worktree of it. They are
// one Project with two Workspaces, which is the only shape in which "same
// project, different directory" can be tested at all.
func gitRepoWithWorktree(t *testing.T) (string, string) {
	t.Helper()
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
	return repo, linked
}

// --continue answers "which conversation was I just having", which is a
// question about this directory. Not the newest session on the machine, and
// not a newer one in a sibling worktree of the same project.
func TestContinuePicksThisWorkspacesNewestSession(t *testing.T) {
	repo, linked := gitRepoWithWorktree(t)
	project, err := currentProject(context.Background(), repo)
	if err != nil {
		t.Fatalf("resolve project: %v", err)
	}
	_, otherID := projectDir(t, "elsewhere")

	now := time.Now()
	older := seedProjectSession(t, project.ID, repo, now.Add(-2*time.Hour))
	wanted := seedProjectSession(t, project.ID, repo, now.Add(-time.Hour))
	inSibling := seedProjectSession(t, project.ID, linked, now.Add(-time.Minute))
	newestAnywhere := seedProjectSession(t, otherID, repo, now)

	target, err := resolveSessionTarget(context.Background(), "", true, false, repo, false)
	if err != nil {
		t.Fatalf("resolveSessionTarget: %v", err)
	}
	switch target.SessionID {
	case wanted:
	case inSibling:
		t.Fatal("--continue reached into a sibling worktree of the same project")
	case newestAnywhere:
		t.Fatal("--continue chose a session belonging to another project")
	case older:
		t.Fatal("--continue chose an older session of this workspace")
	default:
		t.Fatalf("--continue chose %s, want %s", target.SessionID, wanted)
	}
	if target.Workspace != repo {
		t.Errorf("workspace = %s, want %s", target.Workspace, repo)
	}
}

func TestContinueWithNoSessionsHereIsRefused(t *testing.T) {
	empty, _ := projectDir(t, "empty")
	other, otherID := projectDir(t, "other")
	seedProjectSession(t, otherID, other, time.Now())

	if _, err := resolveSessionTarget(context.Background(), "", true, false, empty, false); err == nil {
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

	_, err := resolveSessionTarget(context.Background(), id, false, false, stranger, true)
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

	target, err := resolveSessionTarget(context.Background(), sessionID, false, false, dir, true)
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

	target, err := resolveSessionTarget(context.Background(), sessionID, false, false, elsewhere, false)
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

	target, err := resolveSessionTarget(context.Background(), sessionID, false, false, dir, false)
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
	target, err := resolveSessionTarget(context.Background(), "", false, false, "/some/dir", false)
	if err != nil {
		t.Fatalf("resolveSessionTarget: %v", err)
	}
	if target != (sessionTarget{Workspace: "/some/dir"}) {
		t.Errorf("target = %+v, want the workspace alone", target)
	}
}

// A checkout and its linked worktree are one Project, and --continue still does
// not cross between them: shared memory and resume scope are different
// questions. Widening is explicit and prints the root it will run in. See
// docs/design/local-project-memory.md §11.2.
func TestContinueDoesNotCrossWorktreesOfOneRepository(t *testing.T) {
	repo, linked := gitRepoWithWorktree(t)
	project, err := currentProject(context.Background(), repo)
	if err != nil {
		t.Fatalf("resolve project: %v", err)
	}
	fromWorktree, err := currentProject(context.Background(), linked)
	if err != nil {
		t.Fatalf("resolve project from the worktree: %v", err)
	}
	if fromWorktree.ID != project.ID {
		t.Fatalf("the worktree resolved to project %s, want the checkout's %s", fromWorktree.ID, project.ID)
	}
	sessionID := seedProjectSession(t, project.ID, repo, time.Now())

	if _, err := resolveSessionTarget(context.Background(), "", true, false, linked, false); err == nil {
		t.Fatal("--continue in the worktree took the checkout's session")
	}

	target, err := resolveSessionTarget(context.Background(), "", true, true, linked, false)
	if err != nil {
		t.Fatalf("--continue --project from the worktree: %v", err)
	}
	if target.SessionID != sessionID {
		t.Errorf("widened --continue chose %s, want %s", target.SessionID, sessionID)
	}
	if target.Workspace != repo {
		t.Errorf("workspace = %s, want the checkout %s", target.Workspace, repo)
	}
}
