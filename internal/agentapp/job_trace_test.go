package agentapp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/agentapp/job"
	"github.com/gougoujiang/buildmax/internal/config"
)

// A background job leaves a durable event log: job_start with launch
// provenance, job_end with the terminal state.
func TestJobEventsReachDurableTrace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("BUILDMAX_HOME", home)
	if err := os.WriteFile(filepath.Join(home, "settings.yaml"), []byte("log_level: error\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	app, err := NewAgentApp(AppConfig{WorkspaceDir: t.TempDir(), EnableBackgroundJobs: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })

	spec := job.CommandSpec{Command: "echo traced", Name: "sh", Args: []string{"-c", "echo traced"}}
	if runtime.GOOS == "windows" {
		spec.Name, spec.Args = "cmd", []string{"/c", "echo traced"}
	}
	j, err := app.Jobs().StartCommand(spec, job.Provenance{
		SessionID: "sess-t", ParentTraceID: "rt_p", ParentToolCallID: "call_p", Sandboxed: false,
	})
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(config.TracesDir(), "jobs", j.ID+".jsonl")
	deadline := time.Now().Add(15 * time.Second)
	var records []map[string]any
	for {
		records = records[:0]
		if data, err := os.ReadFile(path); err == nil {
			for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
				var rec map[string]any
				if json.Unmarshal([]byte(line), &rec) == nil {
					records = append(records, rec)
				}
			}
		}
		if len(records) >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job log incomplete: %v", records)
		}
		time.Sleep(10 * time.Millisecond)
	}

	start := records[0]
	if start["type"] != "job_start" || start["job_id"] != j.ID || start["kind"] != "command" ||
		start["parent_run_id"] != "rt_p" || start["parent_tool_call_id"] != "call_p" ||
		start["session_id"] != "sess-t" || start["sandboxed"] != false {
		t.Fatalf("job_start = %v", start)
	}
	end := records[len(records)-1]
	if end["type"] != "job_end" || end["state"] != "succeeded" {
		t.Fatalf("job_end = %v", end)
	}
}
