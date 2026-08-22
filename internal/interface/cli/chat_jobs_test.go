package cli

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/agentapp"
	"github.com/gougoujiang/buildmax/internal/agentapp/job"
	"github.com/gougoujiang/buildmax/internal/config"
)

func testAgentAppWithJobs(t *testing.T, workspace string) *agentapp.AgentApp {
	t.Helper()
	app, err := agentapp.NewAgentApp(agentapp.AppConfig{
		WorkspaceDir:         workspace,
		EnableBackgroundJobs: true,
	})
	if err != nil {
		t.Fatalf("NewAgentApp: %v", err)
	}
	t.Cleanup(func() {
		if err := app.Close(); err != nil {
			t.Fatalf("AgentApp.Close: %v", err)
		}
	})
	return app
}

func testShellJobSpec(command string) job.CommandSpec {
	if runtime.GOOS == "windows" {
		return job.CommandSpec{Command: command, Name: "cmd", Args: []string{"/c", command}}
	}
	return job.CommandSpec{Command: command, Name: "sh", Args: []string{"-c", command}}
}

func TestSlashTasksUnavailableWithoutJobs(t *testing.T) {
	m := NewModel(TUIOpts{Session: testSessionContext(), Workspace: t.TempDir()})
	got, _ := dispatchSlashCommand(m, "/tasks")
	mod := got.(*Model)
	if mod.err == "" {
		t.Fatal("expected an error message when no job manager exists")
	}
	if mod.activePanel != nil {
		t.Fatal("panel should not open without a job manager")
	}
}

func TestSlashTasksPanelListsAndStops(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(config.EnvKeyBuildmaxHome, tmp)
	writeTestSettings(t, `{"models":[{"model":"m","name":"m","api_key":"x"}]}`)
	app := testAgentAppWithJobs(t, t.TempDir())

	command := "sleep 30"
	if runtime.GOOS == "windows" {
		command = "ping -n 31 127.0.0.1 > NUL"
	}
	j, err := app.Jobs().StartCommand(testShellJobSpec(command), job.Provenance{SessionID: "s1"})
	if err != nil {
		t.Fatal(err)
	}

	m := NewModel(TUIOpts{App: app, Session: testSessionContext(), Workspace: t.TempDir()})
	got, _ := dispatchSlashCommand(m, "/tasks")
	mod := got.(*Model)
	panel, ok := mod.activePanel.(*slashJobsPanel)
	if !ok {
		t.Fatalf("active panel = %T, want *slashJobsPanel", mod.activePanel)
	}
	rendered := panel.Render(mod, 120)
	if !strings.Contains(rendered, j.ID) || !strings.Contains(rendered, "running") {
		t.Fatalf("render = %q", rendered)
	}

	// The stop action goes through the manager, and the panel reflects it.
	if err := app.Jobs().Stop(j.ID); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		if snap, ok := app.Jobs().Get(j.ID); ok && !snap.Running() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("job never stopped")
		}
		time.Sleep(10 * time.Millisecond)
	}
	rendered = panel.Render(mod, 120)
	if !strings.Contains(rendered, "canceled") {
		t.Fatalf("render after stop = %q", rendered)
	}
}

func TestJobEventLineFormatsTerminalStates(t *testing.T) {
	line := formatJobEventForScrollback(job.Job{
		ID: "jb_x", State: job.StateFailed, Err: "exit code 2",
		CreatedAt: time.Now().Add(-3 * time.Second), EndedAt: time.Now(),
		Command: "npm test",
	})
	for _, want := range []string{"jb_x", "failed", "exit code 2", "npm test"} {
		if !strings.Contains(line, want) {
			t.Fatalf("line %q missing %q", line, want)
		}
	}
}
