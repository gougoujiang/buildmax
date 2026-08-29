package desktop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/agentapp"
	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/localproject"
	"github.com/gougoujiang/buildmax/internal/infra/localprojectstore"
)

func TestApp_Startup_sets_data_dir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BUILDMAX_HOME", dir)
	app := NewApp()
	ctx := context.Background()
	app.Startup(ctx)
	got := config.DataDir()
	if got != dir {
		t.Errorf("DataDir() = %q, want %q", got, dir)
	}
}

func TestApp_Shutdown_noop(t *testing.T) {
	app := NewApp()
	app.Shutdown(context.Background())
}

func TestApp_CancelRun_requires_project_id(t *testing.T) {
	app := NewApp()
	if err := app.CancelRun(""); err == nil {
		t.Fatal("CancelRun(\"\") = nil, want error")
	}
}

func TestApp_CancelRun_no_inflight_run_is_noop(t *testing.T) {
	app := NewApp()
	if err := app.CancelRun("p_does_not_exist"); err != nil {
		t.Fatalf("CancelRun on idle project: %v", err)
	}
}

func TestApp_CancelRun_cancels_registered_context(t *testing.T) {
	app := NewApp()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Simulate an in-flight run by registering a cancel func, then verify
	// CancelRun fires it and clears the entry. This mirrors what
	// SendMessageStream does without spinning up an AgentApp.
	runCtx, runCancel := context.WithCancel(ctx)
	app.mu.Lock()
	app.runCancels["p_test"] = runCancel
	app.mu.Unlock()

	if err := app.CancelRun("p_test"); err != nil {
		t.Fatalf("CancelRun: %v", err)
	}
	if runCtx.Err() == nil {
		t.Fatal("run context not cancelled after CancelRun")
	}

	// CancelRun does not remove the entry — the run goroutine's defer does.
	// Verify a second CancelRun on the still-registered entry remains a no-op
	// (idempotent) by simply not panicking and returning nil.
	if err := app.CancelRun("p_test"); err != nil {
		t.Fatalf("second CancelRun: %v", err)
	}
}

// A prompt sent while a run is in flight is queued rather than rejected. The run is
// simulated the same way the cancel tests do it: registering a cancel func is what
// "busy" means to SendMessageStream.
func TestApp_SendMessageStream_queues_while_busy(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BUILDMAX_HOME", dir)
	app := NewApp()
	app.Startup(context.Background())

	_, runCancel := context.WithCancel(context.Background())
	t.Cleanup(runCancel)
	app.mu.Lock()
	app.runCancels["p_busy"] = runCancel
	app.mu.Unlock()

	pos, err := app.SendMessageStream("p_busy", "", "follow-up")
	if err != nil {
		t.Fatalf("SendMessageStream while busy: %v", err)
	}
	if pos != 1 {
		t.Errorf("first queued position = %d, want 1", pos)
	}
	pos, err = app.SendMessageStream("p_busy", "", "and another")
	if err != nil {
		t.Fatalf("second SendMessageStream while busy: %v", err)
	}
	if pos != 2 {
		t.Errorf("second queued position = %d, want 2", pos)
	}

	got := app.QueuedMessages("p_busy")
	want := []string{"follow-up", "and another"}
	if len(got) != len(want) {
		t.Fatalf("QueuedMessages = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("QueuedMessages[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// Stopping a run discards what was queued behind it: those prompts were written for
// work the user just called off.
func TestApp_CancelRun_drops_queued_messages(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BUILDMAX_HOME", dir)
	app := NewApp()
	app.Startup(context.Background())

	_, runCancel := context.WithCancel(context.Background())
	t.Cleanup(runCancel)
	app.mu.Lock()
	app.runCancels["p_busy"] = runCancel
	app.mu.Unlock()

	if _, err := app.SendMessageStream("p_busy", "", "queued behind the run"); err != nil {
		t.Fatalf("SendMessageStream while busy: %v", err)
	}
	if err := app.CancelRun("p_busy"); err != nil {
		t.Fatalf("CancelRun: %v", err)
	}
	if got := app.QueuedMessages("p_busy"); len(got) != 0 {
		t.Errorf("QueuedMessages after cancel = %v, want empty", got)
	}
}

func TestApp_SendMessageStream_queue_has_a_cap(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BUILDMAX_HOME", dir)
	app := NewApp()
	app.Startup(context.Background())

	_, runCancel := context.WithCancel(context.Background())
	t.Cleanup(runCancel)
	app.mu.Lock()
	app.runCancels["p_busy"] = runCancel
	app.mu.Unlock()

	for i := 0; i < agent.DefaultMaxQueuedMessages; i++ {
		if _, err := app.SendMessageStream("p_busy", "", "filler"); err != nil {
			t.Fatalf("SendMessageStream #%d: %v", i, err)
		}
	}
	_, err := app.SendMessageStream("p_busy", "", "one too many")
	if err == nil {
		t.Fatal("SendMessageStream past the cap = nil, want error")
	}
	if !errors.Is(err, agent.ErrQueueFull) {
		t.Errorf("error = %v, want it to wrap ErrQueueFull", err)
	}
}

// The desktop is where a person looks at their own memories, so the payload
// carries bodies -- unlike the index a model is given, which is bounded because
// it is sent on every call.
func TestProjectMemoryCarriesBodiesAndWhatTheIndexCosts(t *testing.T) {
	t.Setenv("BUILDMAX_HOME", t.TempDir())
	app := NewApp()
	app.Startup(t.Context())
	t.Cleanup(func() { app.Shutdown(t.Context()) })

	project, err := app.OpenProject(t.TempDir(), "memory probe")
	if err != nil {
		t.Fatalf("OpenProject: %v", err)
	}

	empty, err := app.ProjectMemory(project.ID)
	if err != nil {
		t.Fatalf("ProjectMemory: %v", err)
	}
	if len(empty.Memories) != 0 || empty.Directory == "" {
		t.Errorf("empty payload = %+v, want no memories and a directory to open", empty)
	}

	store := agentapp.NewProjectManager(config.ProjectsDir()).Store()
	if _, err := store.WriteMemory(t.Context(), project.ID, localproject.MemoryWrite{
		Name:        "merge-commit",
		Description: "merge commits, not squash",
		Type:        localproject.MemoryTypeFeedback,
		Body:        "Use merge commits.\n\n**Why:** per-commit revert.",
	}); err != nil {
		t.Fatalf("WriteMemory: %v", err)
	}

	got, err := app.ProjectMemory(project.ID)
	if err != nil {
		t.Fatalf("ProjectMemory: %v", err)
	}
	if len(got.Memories) != 1 {
		t.Fatalf("memories = %+v, want the one written", got.Memories)
	}
	m := got.Memories[0]
	if m.Name != "merge-commit" || m.Type != "feedback" {
		t.Errorf("memory = %+v, want its name and type", m)
	}
	// The body is the half a description cannot carry, and it is why this
	// payload exists rather than the index.
	if !strings.Contains(m.Body, "per-commit revert") {
		t.Errorf("body = %q, want the reason", m.Body)
	}
	// What the index costs on every call is the number a person prunes
	// against, not the count.
	if got.IndexChars == 0 || got.IndexBudget == 0 {
		t.Errorf("payload = %+v, want the index size against its budget", got)
	}
	if got.ProjectName != "memory probe" {
		t.Errorf("ProjectName = %q, want the project's", got.ProjectName)
	}
}

// A file that never loads is silently absent from every run, so the drawer a
// person opens to look at their memories has to name it.
func TestProjectMemoryNamesSkippedFiles(t *testing.T) {
	t.Setenv("BUILDMAX_HOME", t.TempDir())
	app := NewApp()
	app.Startup(t.Context())
	t.Cleanup(func() { app.Shutdown(t.Context()) })

	project, err := app.OpenProject(t.TempDir(), "skips")
	if err != nil {
		t.Fatalf("OpenProject: %v", err)
	}
	dir := filepath.Join(config.ProjectsDir(), project.ID, localprojectstore.MemoryDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.md"), []byte("no frontmatter\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := app.ProjectMemory(project.ID)
	if err != nil {
		t.Fatalf("ProjectMemory: %v", err)
	}
	if len(got.Skipped) != 1 || got.Skipped[0].File != "broken.md" {
		t.Errorf("skipped = %+v, want the unusable file named", got.Skipped)
	}
	if got.Skipped[0].Reason == "" {
		t.Error("the skipped file carries no reason")
	}
}

func TestProjectMemoryNeedsAProject(t *testing.T) {
	t.Setenv("BUILDMAX_HOME", t.TempDir())
	app := NewApp()
	app.Startup(t.Context())
	t.Cleanup(func() { app.Shutdown(t.Context()) })

	if _, err := app.ProjectMemory(""); err == nil {
		t.Error("ProjectMemory accepted an empty project id")
	}
	if _, err := app.ProjectMemory("no-such-project"); err == nil {
		t.Error("ProjectMemory accepted an unknown project id")
	}
}
