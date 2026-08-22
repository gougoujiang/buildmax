package agentapp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/infra/trace"
	tools "github.com/gougoujiang/buildmax/internal/tool"
)

type traceScriptClient struct {
	mu          sync.Mutex
	completions []llm.Completion
}

func (c *traceScriptClient) ChatCompletionBlocking(_ context.Context, _ []llm.Message, _ []llm.ToolDef) (llm.Completion, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.completions) == 0 {
		return llm.Completion{}, fmt.Errorf("unexpected model call")
	}
	completion := c.completions[0]
	c.completions = c.completions[1:]
	return completion, nil
}

func (c *traceScriptClient) ChatCompletionStreaming(ctx context.Context, messages []llm.Message, tools []llm.ToolDef, onDelta func(string)) (llm.Completion, error) {
	completion, err := c.ChatCompletionBlocking(ctx, messages, tools)
	if err == nil && onDelta != nil && completion.Content != "" {
		onDelta(completion.Content)
	}
	return completion, err
}

func (c *traceScriptClient) ContextWindow() int { return 0 }

type traceAllowPolicy struct{}

func (traceAllowPolicy) Check(string, string, map[string]any) (llm.ToolAction, bool) {
	return llm.ToolActionAllow, true
}

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
	result, err := app.RunPrompt(context.Background(), sess, "leak the credentials", RunPromptOpts{})
	if err != nil {
		t.Fatalf("RunPrompt: %v", err)
	}
	if !strings.HasPrefix(result.TraceID, "rt_") {
		t.Fatalf("TraceID = %q, want rt_ prefix", result.TraceID)
	}

	records := readTrace(t, sess.ID, result.TraceID)
	if len(records) != 5 {
		t.Fatalf("got %d records, want run_start + sandbox_boundary + prompt_layers + plugins + run_end: %+v",
			len(records), records)
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

	// The boundary record is written even on a run the hook blocked before the
	// first tool call: what confined a run is not conditional on how far it got.
	boundary := records[1]
	if boundary["type"] != "sandbox_boundary" {
		t.Errorf("second record type = %v, want sandbox_boundary", boundary["type"])
	}
	if sandboxed, ok := boundary["sandboxed"].(bool); !ok || sandboxed {
		t.Errorf("sandboxed = %v, want an explicit false on this unsandboxed surface", boundary["sandboxed"])
	}

	// The prompt layers are recorded for the same reason: what the run was told before the
	// conversation started does not depend on how far it got.
	layers := records[2]
	if layers["type"] != "prompt_layers" {
		t.Errorf("third record type = %v, want prompt_layers", layers["type"])
	}
	if got, ok := layers["layers"].([]any); !ok || len(got) == 0 {
		t.Errorf("prompt_layers records no layers: %+v", layers)
	}

	end := records[len(records)-1]
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
	result, err := app.RunPrompt(context.Background(), sess, "leak the credentials", RunPromptOpts{})
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
	result, err := app.RunPrompt(context.Background(), sess, "leak the credentials", RunPromptOpts{})
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

// TestAgentApp_SubagentTraceLinksToImmediateParent proves both ends of the
// relationship: the top-level trace has no stray parent field, while the child
// records its immediate parent's run id and remains a complete trace itself.
func TestAgentApp_SubagentTraceLinksToImmediateParent(t *testing.T) {
	app := makeAgentAppForHookTests(t)
	app.policy = traceAllowPolicy{}
	app.llmClients.clients["stub"] = &traceScriptClient{completions: []llm.Completion{
		{ToolCalls: []llm.ToolCall{{
			ID:        "call_parent",
			Name:      "Task",
			Arguments: `{"description":"inspect traces","prompt":"reply with child result","subagent_type":"general"}`,
		}}},
		{Content: "child result"},
		{Content: "parent result"},
	}}

	sess, err := app.OpenSession("")
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	result, err := app.RunPrompt(context.Background(), sess, "delegate this", RunPromptOpts{})
	if err != nil {
		t.Fatalf("RunPrompt: %v", err)
	}
	if result.Reply != "parent result" {
		t.Fatalf("Reply = %q, want parent result", result.Reply)
	}

	parent := readTrace(t, sess.ID, result.TraceID)
	if _, exists := parent[0]["parent_run_id"]; exists {
		t.Errorf("top-level run_start has parent_run_id = %v, want absent", parent[0]["parent_run_id"])
	}

	var child []map[string]any
	sessionDirs, err := os.ReadDir(config.TracesDir())
	if err != nil {
		t.Fatalf("read traces dir: %v", err)
	}
	for _, sessionDir := range sessionDirs {
		if !sessionDir.IsDir() || sessionDir.Name() == sess.ID {
			continue
		}
		files, err := os.ReadDir(filepath.Join(config.TracesDir(), sessionDir.Name()))
		if err != nil {
			t.Fatalf("read trace session %s: %v", sessionDir.Name(), err)
		}
		for _, file := range files {
			if file.IsDir() || !strings.HasSuffix(file.Name(), ".jsonl") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(config.TracesDir(), sessionDir.Name(), file.Name()))
			if err != nil {
				t.Fatalf("read child trace %s: %v", file.Name(), err)
			}
			var records []map[string]any
			for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
				var record map[string]any
				if err := json.Unmarshal([]byte(line), &record); err != nil {
					t.Fatalf("decode child trace %s: %v", file.Name(), err)
				}
				records = append(records, record)
			}
			if len(records) > 0 && records[0]["parent_run_id"] == result.TraceID {
				child = records
			}
		}
	}
	if len(child) == 0 {
		t.Fatalf("no child trace linked to parent run %q", result.TraceID)
	}
	if child[0]["is_subagent"] != true {
		t.Errorf("child is_subagent = %v, want true", child[0]["is_subagent"])
	}
	if child[0]["parent_tool_call_id"] != "call_parent" {
		t.Errorf("child parent_tool_call_id = %v, want call_parent", child[0]["parent_tool_call_id"])
	}
	if child[len(child)-1]["type"] != "run_end" {
		t.Errorf("child trace ends with %v, want run_end", child[len(child)-1]["type"])
	}
}

// A background subagent runs on a manager-owned context that never saw this
// package's trace key; the child trace must still link through the explicit
// core-level provenance stamped at launch.
func TestAgentApp_SubagentTraceFallsBackToCoreRunID(t *testing.T) {
	app := makeAgentAppForHookTests(t)

	ctx := agent.CtxWithRunID(context.Background(), "rt_launcher")
	ctx = agent.CtxWithToolCall(ctx, "call_bg_task")
	rec := app.newSubAgentTrace(ctx, "sub-sess", tools.SubAgentRunOpts{Description: "bg"})
	if rec == nil {
		t.Fatal("no child trace despite core-level run identity on ctx")
	}
	tr, ok := rec.(*trace.Recorder)
	if !ok {
		t.Fatalf("trace factory returned %T", rec)
	}
	if err := tr.Close(); err != nil {
		t.Fatalf("close trace: %v", err)
	}
	data, err := os.ReadFile(tr.Path())
	if err != nil {
		t.Fatal(err)
	}
	var first map[string]any
	if err := json.Unmarshal([]byte(strings.SplitN(strings.TrimSpace(string(data)), "\n", 2)[0]), &first); err != nil {
		t.Fatal(err)
	}
	if first["parent_run_id"] != "rt_launcher" || first["parent_tool_call_id"] != "call_bg_task" {
		t.Fatalf("run_start = %v", first)
	}
}
