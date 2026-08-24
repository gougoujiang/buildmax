package cli

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/agentapp"
	"github.com/gougoujiang/buildmax/internal/agentapp/job"
	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/llm"
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

func deliverableJob(m *Model, kind job.Kind) job.Job {
	return job.Job{
		ID: "jb_deliver", Kind: kind, State: job.StateSucceeded, Deliver: true,
		Command:    "npm test",
		Provenance: job.Provenance{SessionID: m.opts.Session.ID()},
	}
}

func currentParked(m *Model) []agentapp.BackgroundEvent {
	return m.parkedJobEvents[m.opts.Session.ID()]
}

// A requested delivery parks a wake-up under the owning session; another
// session's job parks for that session and waits, and non-requested
// completions only notify.
func TestJobEventParksRequestedDelivery(t *testing.T) {
	m := NewModel(TUIOpts{Session: testSessionContext(), Workspace: t.TempDir()})

	_, cmd := handleJobEvent(m, jobEventMsg{Event: job.Event{Job: deliverableJob(m, job.KindCommand)}})
	if len(currentParked(m)) != 1 {
		t.Fatalf("parked = %d, want 1", len(currentParked(m)))
	}
	if cmd == nil {
		t.Fatal("idle model should get a drain command")
	}
	ev := currentParked(m)[0]
	if ev.Source != llm.MessageSourceCommandResult || ev.JobID != "jb_deliver" {
		t.Fatalf("event = %+v", ev)
	}
	if got, ok := m.nextParkedJobEvent(); !ok || got.JobID != "jb_deliver" {
		t.Fatalf("pop = %+v, %v", got, ok)
	}
	if _, ok := m.nextParkedJobEvent(); ok {
		t.Fatal("pop from empty parking succeeded")
	}

	// Subagent kind maps to subagent_result.
	_, _ = handleJobEvent(m, jobEventMsg{Event: job.Event{Job: deliverableJob(m, job.KindSubagent)}})
	if currentParked(m)[0].Source != llm.MessageSourceSubagentResult {
		t.Fatalf("event = %+v", currentParked(m)[0])
	}
	m.parkedJobEvents = nil

	// Another session's job parks under that session and does not wake this
	// one — until that session comes back on screen.
	other := deliverableJob(m, job.KindCommand)
	other.Provenance.SessionID = "someone-else"
	_, _ = handleJobEvent(m, jobEventMsg{Event: job.Event{Job: other}})
	if len(currentParked(m)) != 0 {
		t.Fatalf("current parked = %d, want 0", len(currentParked(m)))
	}
	if len(m.parkedJobEvents["someone-else"]) != 1 {
		t.Fatal("other session's delivery not parked")
	}
	if _, ok := m.nextParkedJobEvent(); ok {
		t.Fatal("popped another session's delivery")
	}

	// A completion nobody asked to deliver only notifies.
	quiet := deliverableJob(m, job.KindCommand)
	quiet.Deliver = false
	_, _ = handleJobEvent(m, jobEventMsg{Event: job.Event{Job: quiet}})
	if len(currentParked(m)) != 0 {
		t.Fatalf("parked = %d, want 0 without deliver", len(currentParked(m)))
	}
}

func TestJobEventParksReactMonitorLine(t *testing.T) {
	m := NewModel(TUIOpts{Session: testSessionContext(), Workspace: t.TempDir()})
	watcher := deliverableJob(m, job.KindMonitor)
	watcher.State = job.StateRunning

	_, _ = handleJobEvent(m, jobEventMsg{Event: job.Event{
		Job: watcher, Type: job.EventMonitorLine, Line: "ERROR boom", DroppedLines: 3,
	}})
	if len(currentParked(m)) != 1 {
		t.Fatalf("parked = %d, want 1", len(currentParked(m)))
	}
	ev := currentParked(m)[0]
	if ev.Source != llm.MessageSourceMonitorEvent || !strings.Contains(ev.Payload, "ERROR boom") || !strings.Contains(ev.Payload, "3 earlier lines") {
		t.Fatalf("event = %+v", ev)
	}

	// A notify-only monitor line parks nothing.
	m.parkedJobEvents = nil
	watcher.Deliver = false
	_, _ = handleJobEvent(m, jobEventMsg{Event: job.Event{Job: watcher, Type: job.EventMonitorLine, Line: "info"}})
	if len(currentParked(m)) != 0 {
		t.Fatalf("parked = %d, want 0 for notify-only", len(currentParked(m)))
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
