package agentapp

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/agent"
)

// readTrace returns the decoded records of the trace file RunPrompt reported.
func readTrace(t *testing.T, sessionID, traceID string) []map[string]any {
	t.Helper()
	path := filepath.Join(config.TracesDir(), sessionID, traceID+".jsonl")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open trace %s: %v", path, err)
	}
	defer f.Close()

	var out []map[string]any
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal(line, &rec); err != nil {
			t.Fatalf("trace line is not valid JSON: %q: %v", line, err)
		}
		out = append(out, rec)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan trace: %v", err)
	}
	return out
}

// TestAgentApp_RunPromptWritesTrace is the end-to-end check for the durable run
// trace (docs/design/durable-run-trace.md §9): after a RunPrompt on any surface, the reported
// TraceID resolves to a JSONL file that opens with run_start and closes with
// run_end. It uses the hook-blocked turn because that path reaches the recorder
// without needing a live LLM, and it also covers the early-return branch that
// synthesizes its own run_end.
func TestAgentApp_RunPromptWritesTrace(t *testing.T) {
	app := makeAgentAppForHookTests(t)
	app.hooks = &fakeHookRunner{blockOn: agent.HookUserPromptSubmit, reason: "policy: no secrets"}

	sess, err := app.OpenSession("")
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	result, err := app.RunPrompt(context.Background(), sess, "leak the credentials", nil, nil, nil)
	if err != nil {
		t.Fatalf("RunPrompt: %v", err)
	}
	if !strings.HasPrefix(result.TraceID, "rt_") {
		t.Fatalf("TraceID = %q, want rt_ prefix", result.TraceID)
	}

	records := readTrace(t, sess.ID, result.TraceID)
	if len(records) != 2 {
		t.Fatalf("got %d records, want run_start + run_end: %+v", len(records), records)
	}

	start := records[0]
	if start["type"] != "run_start" {
		t.Errorf("first record type = %v, want run_start", start["type"])
	}
	if start["run_id"] != result.TraceID {
		t.Errorf("run_start run_id = %v, want %q", start["run_id"], result.TraceID)
	}
	if start["session_id"] != sess.ID {
		t.Errorf("run_start session_id = %v, want %q", start["session_id"], sess.ID)
	}
	if start["workspace"] == nil || start["workspace"] == "" {
		t.Error("run_start workspace empty")
	}
	if start["trace_version"] == nil {
		t.Error("run_start missing trace_version")
	}

	end := records[1]
	if end["type"] != "run_end" {
		t.Errorf("last record type = %v, want run_end", end["type"])
	}
	errMsg, _ := end["error"].(string)
	if !strings.Contains(errMsg, "blocked by hook") || !strings.Contains(errMsg, "policy: no secrets") {
		t.Errorf("run_end error = %q, want the hook block reason", errMsg)
	}
}

// TestAgentApp_RunPromptTraceDisabled asserts BUILDMAX_TRACE_DISABLED turns
// tracing off with no files written and no error surfaced to the run.
func TestAgentApp_RunPromptTraceDisabled(t *testing.T) {
	app := makeAgentAppForHookTests(t)
	app.hooks = &fakeHookRunner{blockOn: agent.HookUserPromptSubmit, reason: "policy: no secrets"}
	t.Setenv(config.EnvKeyBuildmaxTraceDisabled, "1")

	sess, err := app.OpenSession("")
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	result, err := app.RunPrompt(context.Background(), sess, "leak the credentials", nil, nil, nil)
	if err != nil {
		t.Fatalf("RunPrompt: %v", err)
	}
	if result.TraceID != "" {
		t.Errorf("TraceID = %q, want empty when tracing is disabled", result.TraceID)
	}
	if _, err := os.Stat(config.TracesDir()); !os.IsNotExist(err) {
		t.Errorf("traces dir exists with tracing disabled (stat err = %v)", err)
	}
}

// TestAgentApp_RunPromptTraceFailureIsFailOpen asserts an unwritable traces
// directory degrades to no trace instead of breaking the run.
func TestAgentApp_RunPromptTraceFailureIsFailOpen(t *testing.T) {
	app := makeAgentAppForHookTests(t)
	app.hooks = &fakeHookRunner{blockOn: agent.HookUserPromptSubmit, reason: "policy: no secrets"}

	// Occupy the traces path with a regular file so MkdirAll must fail.
	if err := os.WriteFile(config.TracesDir(), []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("seed traces path: %v", err)
	}

	sess, err := app.OpenSession("")
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	result, err := app.RunPrompt(context.Background(), sess, "leak the credentials", nil, nil, nil)
	if err != nil {
		t.Fatalf("RunPrompt returned an error for a trace failure; tracing must fail open: %v", err)
	}
	if result.Reply != "policy: no secrets" {
		t.Errorf("reply = %q, want the run to complete normally", result.Reply)
	}
	if result.TraceID != "" {
		t.Errorf("TraceID = %q, want empty when the recorder could not start", result.TraceID)
	}
}
