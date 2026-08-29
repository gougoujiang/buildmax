package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/agentapp"
	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/localproject"
)

func checkByTitle(checks []doctorCheck, title string) *doctorCheck {
	for i := range checks {
		if checks[i].Title == title {
			return &checks[i]
		}
	}
	return nil
}

// Doctor diagnoses; the first run registers. A command that reported which
// project a directory belongs to by deciding it would answer its own question.
func TestCheckProjectRegistersNothing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "never-run-here")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	got := checkProject(context.Background(), dir)
	if len(got) != 1 || got[0].Severity != doctorOK {
		t.Fatalf("checks = %+v, want one OK row for an unregistered directory", got)
	}
	if !strings.Contains(got[0].Detail, "no project registered") {
		t.Errorf("detail = %q, want it to say nothing is registered here", got[0].Detail)
	}

	if _, err := agentapp.NewProjectManager(config.ProjectsDir()).Lookup(context.Background(), dir); err == nil {
		t.Error("doctor registered a project for a directory it was only asked about")
	}
}

// Doctor is where a person finds the project id and where its memories live:
// neither is guessable from the repository, because both are under
// BUILDMAX_HOME.
func TestCheckProjectReportsTheProjectAndItsMemory(t *testing.T) {
	dir, id := projectDir(t, "reported")

	got := checkProject(context.Background(), dir)
	if len(got) != 2 {
		t.Fatalf("checks = %+v, want a project row and a memory row", got)
	}
	if !strings.Contains(got[0].Detail, id) {
		t.Errorf("project detail = %q, want the project id", got[0].Detail)
	}
	memory := checkByTitle(got, "project memory")
	if memory == nil || !strings.Contains(memory.Detail, "empty") {
		t.Fatalf("memory row = %+v, want it to report an empty store", memory)
	}

	store := agentapp.NewProjectManager(config.ProjectsDir()).Store()
	if _, err := store.WriteMemory(context.Background(), id, localproject.MemoryWrite{
		Name:        "merge-commit",
		Description: "merge commits, not squash",
		Type:        localproject.MemoryTypeProject,
		Body:        "Use merge commits.\n\n**Why:** per-commit revert.",
	}); err != nil {
		t.Fatalf("WriteMemory: %v", err)
	}

	memory = checkByTitle(checkProject(context.Background(), dir), "project memory")
	if memory == nil {
		t.Fatal("no memory row after writing")
	}
	for _, want := range []string{"1/20 memories", "index ", "memory"} {
		if !strings.Contains(memory.Detail, want) {
			t.Errorf("memory detail %q does not report %q", memory.Detail, want)
		}
	}
	// Doctor output is the kind of thing people paste into an issue, so it
	// counts and locates memories and never quotes one.
	if strings.Contains(memory.Detail, "per-commit revert") {
		t.Errorf("doctor printed a memory body: %q", memory.Detail)
	}
}

// A file left unusable by a hand edit is silently absent from every run.
// Nothing else tells the user, so this is where they find out.
func TestCheckProjectMemoryFailsOnASkippedFile(t *testing.T) {
	dir, id := projectDir(t, "unusable")
	store := agentapp.NewProjectManager(config.ProjectsDir()).Store()
	if _, err := store.WriteMemory(context.Background(), id, localproject.MemoryWrite{
		Name: "good", Description: "fine", Type: localproject.MemoryTypeProject, Body: "Still fine.",
	}); err != nil {
		t.Fatalf("WriteMemory: %v", err)
	}
	broken := filepath.Join(config.ProjectsDir(), id, "memory", "broken.md")
	if err := os.WriteFile(broken, []byte("no frontmatter here\n"), 0o600); err != nil {
		t.Fatalf("write a broken file: %v", err)
	}

	checks := checkProject(context.Background(), dir)
	var failed *doctorCheck
	for i := range checks {
		if checks[i].Title == "project memory" && checks[i].Severity == doctorFail {
			failed = &checks[i]
		}
	}
	if failed == nil {
		t.Fatalf("checks = %+v, want a failure naming the skipped file", checks)
	}
	if !strings.Contains(failed.Detail, "broken.md") || failed.Next == "" {
		t.Errorf("failure = %+v, want it to name the file and how to repair it", failed)
	}
}

// A session naming a project this machine no longer has still opens, and
// nothing re-attaches it: the point of reporting it is that a person decides.
func TestCheckDetachedSessions(t *testing.T) {
	clean := checkDetachedSessions(context.Background())
	if clean.Severity != doctorOK {
		t.Fatalf("sessions row = %+v, want OK before any session is detached", clean)
	}

	seedProjectSession(t, "hyzc3kqxa2vw7m4t9pbn", t.TempDir(), time.Now())

	got := checkDetachedSessions(context.Background())
	if got.Severity != doctorWarn {
		t.Fatalf("sessions row = %+v, want a warning", got)
	}
	if !strings.Contains(got.Next, "resume") {
		t.Errorf("the warning does not say the session still opens: %q", got.Next)
	}
}

// A moved repository misses lookup, so a second project with empty memory is
// created before the user could have asked for anything — and the duplicate
// looks like the feature working unless creation says otherwise.
func TestCreatingBesideAnUnresolvedProjectAnnouncesItself(t *testing.T) {
	gone := filepath.Join(t.TempDir(), "moved-away")
	if err := os.MkdirAll(gone, 0o755); err != nil {
		t.Fatal(err)
	}
	// Its own catalog: the shared test home accumulates projects whose temp
	// directories later tests remove, so "nothing is unresolved" is only a
	// stable question inside a catalog this test owns.
	manager := agentapp.NewProjectManager(filepath.Join(t.TempDir(), "projects"))
	stale, err := manager.Resolve(context.Background(), gone)
	if err != nil {
		t.Fatalf("resolve the project that will move: %v", err)
	}
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}

	fresh := filepath.Join(t.TempDir(), "moved-here")
	if err := os.MkdirAll(fresh, 0o755); err != nil {
		t.Fatal(err)
	}
	created, report, err := manager.ResolveReporting(context.Background(), fresh)
	if err != nil {
		t.Fatalf("ResolveReporting: %v", err)
	}
	if created.ID == stale.ID {
		t.Fatal("the moved project was silently reused; nothing may join two memory domains by guess")
	}
	lines := strings.Join(report.Lines(relinkCommandHint), "\n")
	if !strings.Contains(lines, stale.ID) {
		t.Errorf("the notice does not name the unresolved project: %q", lines)
	}
	if !strings.Contains(lines, relinkCommandHint) {
		t.Errorf("the notice does not name the relink command: %q", lines)
	}

	// Relinking is what the notice is for, and it keeps the project's id.
	relinked, err := manager.Relink(context.Background(), stale.ID, fresh)
	if err == nil {
		t.Fatalf("relinked onto a directory another project already holds: %+v", relinked)
	}
	if err := manager.Store().Delete(context.Background(), created.ID); err != nil {
		t.Fatalf("remove the duplicate: %v", err)
	}
	relinked, err = manager.Relink(context.Background(), stale.ID, fresh)
	if err != nil {
		t.Fatalf("Relink: %v", err)
	}
	if relinked.ID != stale.ID {
		t.Errorf("relink changed the id: %s -> %s", stale.ID, relinked.ID)
	}
	back, err := manager.Lookup(context.Background(), fresh)
	if err != nil || back.ID != stale.ID {
		t.Errorf("the relinked project does not resolve from %s: %v", fresh, err)
	}
}

// Only when both things are true: a run that registers the first project on a
// machine is not reporting damage.
func TestNoAnnouncementWhenNothingIsUnresolved(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ordinary")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	_, report, err := agentapp.NewProjectManager(filepath.Join(t.TempDir(), "projects")).
		ResolveReporting(context.Background(), dir)
	if err != nil {
		t.Fatalf("ResolveReporting: %v", err)
	}
	if !report.Empty() {
		t.Errorf("report = %+v, want nothing to say", report)
	}
}
