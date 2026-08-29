package agentapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/localproject"
	tools "github.com/gougoujiang/buildmax/internal/tool"
	"github.com/gougoujiang/buildmax/internal/util"
)

// memoryApp is the smallest AgentApp the memory seam reads through: a resolved
// Project and the catalog behind it.
func memoryApp(t *testing.T) *AgentApp {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "notes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	projects := NewProjectManager(filepath.Join(t.TempDir(), "projects"))
	project, err := projects.Resolve(context.Background(), dir)
	if err != nil {
		t.Fatalf("resolve project: %v", err)
	}
	return &AgentApp{project: project, projects: projects}
}

func TestProjectMemoryRoundTripsThroughTheSeam(t *testing.T) {
	app := memoryApp(t)
	mem := app.projectMemoryFor("session-1", "run-1")
	if mem == nil {
		t.Fatal("a run in a project has no memory seam")
	}

	if got := mem.Memory(); got.Content != "" || got.Digest != "" {
		t.Fatalf("fresh memory = %+v, want empty", got)
	}

	const doc = "# Project Memory\n\n- Prefer narrow table-driven tests.\n"
	written, err := mem.WriteMemory(context.Background(), doc, "")
	if err != nil {
		t.Fatalf("WriteMemory: %v", err)
	}
	if written.Revision != 1 || written.ScopeID != app.project.ID {
		t.Errorf("write returned %+v, want revision 1 scoped to the project", written)
	}

	// Read again rather than trusting the returned value: the point of a shared
	// document is that the next model call goes back to the store.
	read := mem.Memory()
	if read.Content != doc {
		t.Errorf("Content = %q, want %q", read.Content, doc)
	}
	if read.Digest != written.Digest || read.Revision != 1 {
		t.Errorf("read = %+v, want the digest and revision just written", read)
	}
	if read.Scope != "project" {
		t.Errorf("Scope = %q, want project", read.Scope)
	}
}

// A write from one session is visible to the next call of another, which is the
// whole reason the document lives on the Project rather than in a session.
func TestProjectMemoryIsSharedBetweenSessions(t *testing.T) {
	app := memoryApp(t)
	first := app.projectMemoryFor("session-1", "run-1")
	second := app.projectMemoryFor("session-2", "run-2")

	if _, err := first.WriteMemory(context.Background(), "- Learned in session one.\n", ""); err != nil {
		t.Fatalf("WriteMemory: %v", err)
	}
	if got := second.Memory(); !strings.Contains(got.Content, "Learned in session one.") {
		t.Errorf("the second session read %q, want the first's memory", got.Content)
	}
}

func TestProjectMemoryConflictSurfacesTheStoredRevision(t *testing.T) {
	app := memoryApp(t)
	mem := app.projectMemoryFor("session-1", "run-1")
	if _, err := mem.WriteMemory(context.Background(), "- First.\n", ""); err != nil {
		t.Fatalf("WriteMemory: %v", err)
	}

	stored, err := mem.WriteMemory(context.Background(), "- Blind overwrite.\n", "sha256:stale")
	if !errors.Is(err, localproject.ErrDigestMismatch) {
		t.Fatalf("WriteMemory = %v, want ErrDigestMismatch", err)
	}
	if stored.Revision != 1 || !strings.Contains(stored.Content, "First.") {
		t.Errorf("the conflict returned %+v, want what is actually stored", stored)
	}
}

// A hand edit can leave the file over the limit or not text at all. Sending a
// prefix would be worse than sending none, so the run goes without.
func TestUnusableMemoryIsNotRendered(t *testing.T) {
	app := memoryApp(t)
	mem := app.projectMemoryFor("session-1", "run-1")
	if _, err := mem.WriteMemory(context.Background(), "- Fine for now.\n", ""); err != nil {
		t.Fatalf("WriteMemory: %v", err)
	}

	path := filepath.Join(app.projects.Dir(), app.project.ID, "memory", "MEMORY.md")
	oversize := strings.Repeat("a", localproject.MaxMemoryChars+1)
	if err := os.WriteFile(path, []byte(oversize), 0o600); err != nil {
		t.Fatalf("hand edit: %v", err)
	}

	if got := mem.Memory(); got.Content != "" {
		t.Errorf("an oversize document was loaded: %d characters", len(got.Content))
	}
	// It is refused, not truncated: the file is still exactly what its author
	// left, waiting to be repaired.
	after, err := os.ReadFile(path)
	if err != nil || string(after) != oversize {
		t.Errorf("the unusable document was rewritten: %v", err)
	}
}

// Two separate reasons a run has no memory, and both have to produce the same
// nothing: a surface that never had a Project, and a user who turned it off.
func TestNoMemorySeamWithoutAProjectOrWhenDisabled(t *testing.T) {
	if seam := (&AgentApp{}).projectMemoryFor("session-1", "run-1"); seam != nil {
		t.Error("a projectless run has a memory seam")
	}

	disabled := memoryApp(t)
	disabled.memoryDisabled = true
	if seam := disabled.projectMemoryFor("session-1", "run-1"); seam != nil {
		t.Error("a run with memory turned off has a memory seam")
	}

	var nilApp *AgentApp
	if seam := nilApp.projectMemoryFor("session-1", "run-1"); seam != nil {
		t.Error("a nil app has a memory seam")
	}
}

// Registration follows the same rule as reading, and from the same condition:
// a run that may not look at the document must not be able to replace it.
func TestWriteToolFollowsWhetherTheRunHasMemory(t *testing.T) {
	tests := []struct {
		name string
		cfg  AppConfig
		want bool
	}{
		{name: "project memory on", cfg: AppConfig{EnableLocalProject: true}, want: true},
		{name: "memory turned off for this run", cfg: AppConfig{EnableLocalProject: true, DisableProjectMemory: true}},
		{name: "no local project at all", cfg: AppConfig{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.cfg
			cfg.WorkspaceDir = t.TempDir()
			app, err := NewAgentApp(cfg)
			if err != nil {
				t.Fatalf("NewAgentApp: %v", err)
			}
			t.Cleanup(func() { _ = app.Close() })

			var found bool
			for _, entry := range app.ToolEntries() {
				if entry.Name == tools.ToolNameProjectMemoryWrite {
					found = true
				}
			}
			if found != tt.want {
				t.Errorf("%s registered = %v, want %v", tools.ToolNameProjectMemoryWrite, found, tt.want)
			}
		})
	}
}

// A delegate reads the same memory and does not curate it, so the tool must not
// be resolvable from the set an agent definition draws on.
func TestBaseToolsExcludeTheMemoryWriteTool(t *testing.T) {
	base := buildBaseTools(nil, util.FixedRoot(t.TempDir()), stubTool{}, agent.NoopSandbox{}, nil, nil)
	for _, tl := range base {
		if tl.Name() == tools.ToolNameProjectMemoryWrite {
			t.Fatal("the write tool is in the base set, so a subagent definition can name it")
		}
	}
}
