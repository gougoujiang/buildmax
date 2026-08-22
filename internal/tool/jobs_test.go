package tool

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/agentapp/job"
	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/session"
)

func sleepCommandForTest() string {
	if runtime.GOOS == "windows" {
		return "ping -n 31 127.0.0.1 > NUL"
	}
	return "sleep 30"
}

func newJobManager(t *testing.T) *job.Manager {
	t.Helper()
	m := job.NewManager()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = m.Close(ctx)
	})
	return m
}

// startBackground runs command through Bash's background path and returns the
// job ID parsed from the tool output.
func startBackground(t *testing.T, b *Bash, m *job.Manager, command string) string {
	t.Helper()
	ctx := session.CtxWithSessionID(context.Background(), "sess-1")
	out, err := b.Execute(ctx, map[string]any{"command": command, "run_in_background": true})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	idx := strings.Index(out, "jb_")
	if idx < 0 {
		t.Fatalf("no job ID in output: %q", out)
	}
	id := out[idx:]
	if end := strings.IndexAny(id, " \n"); end > 0 {
		id = id[:end]
	}
	if _, ok := m.Get(id); !ok {
		t.Fatalf("job %q not in manager", id)
	}
	return id
}

func waitJobDone(t *testing.T, m *job.Manager, id string) job.Job {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if j, ok := m.Get(id); ok && !j.Running() {
			return j
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("job %s never finished", id)
	return job.Job{}
}

func TestBashRunInBackground(t *testing.T) {
	m := newJobManager(t)
	b := NewBash(t.TempDir()).WithJobs(m)

	id := startBackground(t, b, m, "echo detached")
	j := waitJobDone(t, m, id)
	if j.State != job.StateSucceeded {
		t.Fatalf("job = %+v", j)
	}
	if j.Provenance.SessionID != "sess-1" {
		t.Fatalf("provenance = %+v", j.Provenance)
	}

	out, err := NewJobOutput(m).Execute(context.Background(), map[string]any{"job_id": id})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "detached") || !strings.Contains(out, "succeeded") || !strings.Contains(out, "next_cursor:") {
		t.Fatalf("JobOutput = %q", out)
	}

	list, err := NewJobList(m).Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(list, id) {
		t.Fatalf("JobList = %q", list)
	}
}

func TestBashBackgroundRefusedInSubagent(t *testing.T) {
	m := newJobManager(t)
	b := NewBash(t.TempDir()).WithJobs(m)
	ctx := agent.CtxMarkSubagent(session.CtxWithSessionID(context.Background(), "sub-sess"))
	out, err := b.Execute(ctx, map[string]any{"command": "echo hi", "run_in_background": true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "not available inside a subagent") {
		t.Fatalf("output = %q", out)
	}
	if jobs := m.List(); len(jobs) != 0 {
		t.Fatalf("job started despite refusal: %+v", jobs)
	}
}

func TestBashBackgroundUnavailableWithoutManager(t *testing.T) {
	b := NewBash(t.TempDir())
	out, err := b.Execute(context.Background(), map[string]any{"command": "echo hi", "run_in_background": true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "not available on this surface") {
		t.Fatalf("output = %q", out)
	}
	// The parameter is only advertised when jobs are available.
	schema := b.Parameters().(map[string]any)["properties"].(map[string]any)
	if _, ok := schema["run_in_background"]; ok {
		t.Fatal("run_in_background advertised without a job manager")
	}
	withJobs := NewBash(t.TempDir()).WithJobs(job.NewManager())
	schema = withJobs.Parameters().(map[string]any)["properties"].(map[string]any)
	if _, ok := schema["run_in_background"]; !ok {
		t.Fatal("run_in_background missing with a job manager")
	}
}

func TestJobStopTool(t *testing.T) {
	m := newJobManager(t)
	b := NewBash(t.TempDir()).WithJobs(m)
	id := startBackground(t, b, m, sleepCommandForTest())

	out, err := NewJobStop(m).Execute(context.Background(), map[string]any{"job_id": id})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Stop requested") {
		t.Fatalf("JobStop = %q", out)
	}
	j := waitJobDone(t, m, id)
	if j.State != job.StateCanceled || j.StopReason != job.StopUser {
		t.Fatalf("job = %+v", j)
	}
	// Stopping again reports the terminal state instead of erroring.
	out, err = NewJobStop(m).Execute(context.Background(), map[string]any{"job_id": id})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "already finished") {
		t.Fatalf("JobStop = %q", out)
	}
}

func TestJobToolsUnknownID(t *testing.T) {
	m := newJobManager(t)
	out, err := NewJobOutput(m).Execute(context.Background(), map[string]any{"job_id": "jb_nope"})
	if err != nil || !strings.Contains(out, "No such job") {
		t.Fatalf("JobOutput = %q, %v", out, err)
	}
	out, err = NewJobStop(m).Execute(context.Background(), map[string]any{"job_id": "jb_nope"})
	if err != nil || !strings.Contains(out, "No such job") {
		t.Fatalf("JobStop = %q, %v", out, err)
	}
	out, err = NewJobList(m).Execute(context.Background(), map[string]any{})
	if err != nil || out != "No background jobs." {
		t.Fatalf("JobList = %q, %v", out, err)
	}
}
