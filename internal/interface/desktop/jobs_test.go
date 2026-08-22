package desktop

import (
	"context"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/agentapp/job"
)

// recordingEmitter captures frontend events for assertions.
type recordingEmitter struct {
	mu     sync.Mutex
	events []struct {
		name string
		data any
	}
}

func (r *recordingEmitter) emit(_ context.Context, name string, data any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, struct {
		name string
		data any
	}{name, data})
}

func (r *recordingEmitter) jobUpdates() []JobPayload {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []JobPayload
	for _, e := range r.events {
		if e.name == eventJobUpdate {
			if p, ok := e.data.(JobPayload); ok {
				out = append(out, p)
			}
		}
	}
	return out
}

func shellJobSpecForTest(command string) job.CommandSpec {
	if runtime.GOOS == "windows" {
		return job.CommandSpec{Command: command, Name: "cmd", Args: []string{"/c", command}}
	}
	return job.CommandSpec{Command: command, Name: "sh", Args: []string{"-c", command}}
}

// newJobsTestApp builds an App with one saved project and a recording emitter.
func newJobsTestApp(t *testing.T) (*App, *recordingEmitter, string) {
	t.Helper()
	t.Setenv("BUILDMAX_HOME", t.TempDir())
	rec := &recordingEmitter{}
	app := NewApp()
	app.emit = rec.emit
	app.Startup(context.Background())
	t.Cleanup(func() { app.Shutdown(context.Background()) })

	const projectID = "p_jobs"
	if err := writeProjects([]Project{{ID: projectID, Name: "jobs", FolderPath: t.TempDir()}}); err != nil {
		t.Fatal(err)
	}
	return app, rec, projectID
}

func TestDesktopJobBridge(t *testing.T) {
	app, rec, projectID := newJobsTestApp(t)

	ag, err := app.agentAppForProject(projectID)
	if err != nil {
		t.Fatal(err)
	}
	if ag.Jobs() == nil {
		t.Fatal("desktop AgentApp has no job manager")
	}
	j, err := ag.Jobs().StartCommand(shellJobSpecForTest("echo desktop"), job.Provenance{SessionID: "s1"})
	if err != nil {
		t.Fatal(err)
	}

	// The lifecycle pump forwards the terminal transition to the frontend.
	deadline := time.Now().Add(15 * time.Second)
	for {
		updates := rec.jobUpdates()
		done := false
		for _, u := range updates {
			if u.ID == j.ID && !u.Running && u.State == string(job.StateSucceeded) && u.ProjectID == projectID {
				done = true
			}
		}
		if done {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no terminal job-update event; got %+v", rec.jobUpdates())
		}
		time.Sleep(10 * time.Millisecond)
	}

	list, err := app.ListJobs(projectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != j.ID {
		t.Fatalf("ListJobs = %+v", list)
	}

	out, err := app.GetJobOutput(projectID, j.ID, "stdout", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Data, "desktop") || out.Running {
		t.Fatalf("GetJobOutput = %+v", out)
	}
}

func TestDesktopStopJob(t *testing.T) {
	app, _, projectID := newJobsTestApp(t)
	ag, err := app.agentAppForProject(projectID)
	if err != nil {
		t.Fatal(err)
	}
	command := "sleep 30"
	if runtime.GOOS == "windows" {
		command = "ping -n 31 127.0.0.1 > NUL"
	}
	j, err := ag.Jobs().StartCommand(shellJobSpecForTest(command), job.Provenance{})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.StopJob(projectID, j.ID); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		if snap, ok := ag.Jobs().Get(j.ID); ok && !snap.Running() {
			if snap.State != job.StateCanceled {
				t.Fatalf("state = %s", snap.State)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("job never stopped")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
