package tool

import (
	"context"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/agentapp/job"
	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/session"
)

func backgroundTaskArgs() map[string]any {
	return map[string]any{
		"description":       "background research",
		"prompt":            "find the flaky test",
		"subagent_type":     "general",
		"run_in_background": true,
	}
}

func TestTaskRunInBackground(t *testing.T) {
	m := newJobManager(t)
	runner := &mockRunner{reply: "flaky test is TestX"}
	task, err := NewTask(runner, testAgentTypes())
	if err != nil {
		t.Fatal(err)
	}
	task = task.WithJobs(m, "/ws")

	ctx := session.CtxWithSessionID(context.Background(), "sess-1")
	ctx = agent.CtxWithRunID(ctx, "rt_parent")
	ctx = agent.CtxWithToolCall(ctx, "call_task")
	out, err := task.Execute(ctx, backgroundTaskArgs())
	if err != nil {
		t.Fatal(err)
	}
	idx := strings.Index(out, "jb_")
	if idx < 0 {
		t.Fatalf("no job ID in output: %q", out)
	}
	id := out[idx:]
	if end := strings.IndexAny(id, " \n"); end > 0 {
		id = id[:end]
	}

	j := waitJobDone(t, m, id)
	if j.State != job.StateSucceeded || j.Kind != job.KindSubagent {
		t.Fatalf("job = %+v", j)
	}
	if j.Provenance.SessionID != "sess-1" || j.Provenance.ParentTraceID != "rt_parent" ||
		j.Provenance.ParentToolCallID != "call_task" || j.Provenance.Workspace != "/ws" {
		t.Fatalf("provenance = %+v", j.Provenance)
	}

	reply, err := NewJobOutput(m).Execute(context.Background(), map[string]any{"job_id": id})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply, "flaky test is TestX") || !strings.Contains(reply, "final reply") {
		t.Fatalf("JobOutput = %q", reply)
	}
}

func TestTaskBackgroundRefusedInSubagent(t *testing.T) {
	m := newJobManager(t)
	task, err := NewTask(&mockRunner{reply: "x"}, testAgentTypes())
	if err != nil {
		t.Fatal(err)
	}
	task = task.WithJobs(m, "/ws")
	ctx := agent.CtxMarkSubagent(context.Background())
	out, err := task.Execute(ctx, backgroundTaskArgs())
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

func TestTaskBackgroundUnavailableWithoutManager(t *testing.T) {
	task, err := NewTask(&mockRunner{reply: "x"}, testAgentTypes())
	if err != nil {
		t.Fatal(err)
	}
	out, err := task.Execute(context.Background(), backgroundTaskArgs())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "not available on this surface") {
		t.Fatalf("output = %q", out)
	}
	schema := task.Parameters().(map[string]any)["properties"].(map[string]any)
	if _, ok := schema["run_in_background"]; ok {
		t.Fatal("run_in_background advertised without a job manager")
	}
	withJobs := task.WithJobs(job.NewManager(), "/ws")
	schema = withJobs.Parameters().(map[string]any)["properties"].(map[string]any)
	if _, ok := schema["run_in_background"]; !ok {
		t.Fatal("run_in_background missing with a job manager")
	}
}
