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

// Doctor is where a person finds the project id and the memory path: neither is
// guessable from the repository, because both live under BUILDMAX_HOME.
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
		t.Fatalf("memory row = %+v, want it to report an empty document", memory)
	}

	const doc = "# Project Memory\n\n- A stable preference.\n"
	store := agentapp.NewProjectManager(config.ProjectsDir()).Store()
	if _, err := store.WriteMemory(context.Background(), id, localproject.MemoryWrite{Content: doc}); err != nil {
		t.Fatalf("WriteMemory: %v", err)
	}

	memory = checkByTitle(checkProject(context.Background(), dir), "project memory")
	if memory == nil {
		t.Fatal("no memory row after writing")
	}
	for _, want := range []string{"revision 1", "MEMORY.md"} {
		if !strings.Contains(memory.Detail, want) {
			t.Errorf("memory detail %q does not report %q", memory.Detail, want)
		}
	}
	// Doctor output is the kind of thing people paste into an issue, so it
	// names the document and never quotes it.
	if strings.Contains(memory.Detail, "A stable preference") {
		t.Errorf("doctor printed the memory content: %q", memory.Detail)
	}
}

// A document left over the limit is skipped for the run. Nothing else tells the
// user their memory stopped being loaded, so this is where they find out.
func TestCheckProjectMemoryFailsOnAnUnusableDocument(t *testing.T) {
	dir, id := projectDir(t, "unusable")
	store := agentapp.NewProjectManager(config.ProjectsDir()).Store()
	if _, err := store.WriteMemory(context.Background(), id, localproject.MemoryWrite{Content: "- Fine.\n"}); err != nil {
		t.Fatalf("WriteMemory: %v", err)
	}
	path := filepath.Join(config.ProjectsDir(), id, "memory", "MEMORY.md")
	if err := os.WriteFile(path, []byte(strings.Repeat("a", localproject.MaxMemoryChars+1)), 0o600); err != nil {
		t.Fatalf("hand edit: %v", err)
	}

	memory := checkByTitle(checkProject(context.Background(), dir), "project memory")
	if memory == nil || memory.Severity != doctorFail {
		t.Fatalf("memory row = %+v, want a failure", memory)
	}
	if memory.Next == "" {
		t.Error("the failure does not say how to repair it")
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
