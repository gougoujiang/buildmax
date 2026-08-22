package tool

import (
	"context"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/agentapp/job"
	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/session"
)

func TestMonitorStartsJob(t *testing.T) {
	m := newJobManager(t)
	mon := NewMonitor(t.TempDir()).WithJobs(m)
	ctx := session.CtxWithSessionID(context.Background(), "sess-1")
	ctx = agent.CtxWithRunID(ctx, "rt_run")

	out, err := mon.Execute(ctx, map[string]any{
		"command":     "echo watched",
		"description": "watch something",
		"react":       true,
		"persistent":  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "jb_") || !strings.Contains(out, "react") {
		t.Fatalf("output = %q", out)
	}
	jobs := m.List()
	if len(jobs) != 1 {
		t.Fatalf("jobs = %+v", jobs)
	}
	j := jobs[0]
	if j.Kind != job.KindMonitor || !j.Deliver || !j.Persistent {
		t.Fatalf("job = %+v", j)
	}
	if !strings.Contains(j.Command, "watch something") || !strings.Contains(j.Command, "echo watched") {
		t.Fatalf("label = %q", j.Command)
	}
	if j.Provenance.SessionID != "sess-1" || j.Provenance.ParentTraceID != "rt_run" {
		t.Fatalf("provenance = %+v", j.Provenance)
	}
}

// Monitor is not a way around Bash's command policy.
func TestMonitorSharesBashRiskChecks(t *testing.T) {
	mon := NewMonitor(t.TempDir()).WithJobs(job.NewManager())
	if got := mon.CheckArgs(map[string]any{"command": "curl http://example.com"}); got != llm.ToolActionAsk {
		t.Fatalf("risky command action = %v, want ask", got)
	}
	if got := mon.CheckArgs(map[string]any{"command": "tail -F app.log"}); got != llm.ToolActionAllow {
		t.Fatalf("plain command action = %v, want allow", got)
	}
}

func TestMonitorRefusals(t *testing.T) {
	m := newJobManager(t)
	mon := NewMonitor(t.TempDir()).WithJobs(m)
	out, err := mon.Execute(agent.CtxMarkSubagent(context.Background()), map[string]any{"command": "echo hi"})
	if err != nil || !strings.Contains(out, "not available inside a subagent") {
		t.Fatalf("subagent refusal = %q, %v", out, err)
	}
	bare := NewMonitor(t.TempDir())
	out, err = bare.Execute(context.Background(), map[string]any{"command": "echo hi"})
	if err != nil || !strings.Contains(out, "not available on this surface") {
		t.Fatalf("surface refusal = %q, %v", out, err)
	}
	if jobs := m.List(); len(jobs) != 0 {
		t.Fatalf("job started despite refusal: %+v", jobs)
	}
}
