package cli

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/agentapp"
	"github.com/gougoujiang/buildmax/internal/core/agent"
)

func TestClassifyExit(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		policyDenied  bool
		userCancelled bool
		want          int
	}{
		{"ok", nil, false, false, ExitOK},
		{"ok with prior policy deny does not fail run", nil, true, false, ExitOK},
		{"user cancel overrides err", errors.New("boom"), false, true, ExitUserCancelled},
		{"policy denial with err", errors.New("boom"), true, false, ExitPolicyDenied},
		{"plain model error", errors.New("agent: llm: 500"), false, false, ExitModelError},
		{"cancel beats policy", errors.New("boom"), true, true, ExitUserCancelled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyExit(tt.err, tt.policyDenied, tt.userCancelled)
			if got != tt.want {
				t.Fatalf("classifyExit() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestExitCodeFor(t *testing.T) {
	if ExitCodeFor(nil) != ExitOK {
		t.Errorf("nil err: want %d", ExitOK)
	}
	if ExitCodeFor(errors.New("plain")) != ExitGeneric {
		t.Errorf("plain err: want %d", ExitGeneric)
	}
	if got := ExitCodeFor(&ExitError{Code: ExitPolicyDenied}); got != ExitPolicyDenied {
		t.Errorf("ExitError: got %d, want %d", got, ExitPolicyDenied)
	}
}

func TestErrorKindForExitCode(t *testing.T) {
	cases := map[int]string{
		ExitPolicyDenied:  "policy_denied",
		ExitModelError:    "model_error",
		ExitToolError:     "tool_error",
		ExitUserCancelled: "cancelled",
		ExitUsage:         "usage",
		ExitGeneric:       "error",
	}
	for code, want := range cases {
		if got := errorKindForExitCode(code); got != want {
			t.Errorf("errorKindForExitCode(%d) = %q, want %q", code, got, want)
		}
	}
}

func TestBuildResultEnvelope_FieldsAndShape(t *testing.T) {
	out := agentapp.RunResult{
		Reply:                 "hello",
		Duration:              250 * time.Millisecond,
		ToolCalls:             2,
		PromptTokens:          10,
		CompletionTokens:      20,
		TotalPromptTokens:     100,
		TotalCompletionTokens: 200,
		CacheReadTokens:       4,
		CacheWriteTokens:      6,
		TotalCacheReadTokens:  40,
		TotalCacheWriteTokens: 60,
		ContextTokens:         5000,
		ContextWindow:         32000,
		SessionID:             "sess-1",
		Workspace:             "/tmp/ws",
		ModelName:             "test-model",
		TraceID:               "run-1",
		TracePath:             "/tmp/home/traces/sess-1/run-1.jsonl",
	}
	env := buildResultEnvelope(out, ExitOK, nil, false, false)
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	wantSubs := []string{
		`"session_id":"sess-1"`,
		`"trace_id":"run-1"`,
		`"trace_path":"/tmp/home/traces/sess-1/run-1.jsonl"`,
		`"model":"test-model"`,
		`"workspace":"/tmp/ws"`,
		`"reply":"hello"`,
		`"tool_calls":2`,
		`"duration_ms":250`,
		`"usage":{"prompt":10,"completion":20,"total_prompt":100,"total_completion":200,` +
			`"cache_read":4,"cache_write":6,"total_cache_read":40,"total_cache_write":60}`,
		`"context":{"tokens":5000,"window":32000}`,
		`"exit_code":0`,
		`"error":null`,
	}
	for _, sub := range wantSubs {
		if !strings.Contains(got, sub) {
			t.Errorf("envelope missing %s\nin: %s", sub, got)
		}
	}
}

// A run with tracing off must still carry the trace fields, empty. Omitting
// them would leave a caller unable to tell "this build wrote no trace" from
// "this build is too old to report one".
func TestBuildResultEnvelope_TraceFieldsPresentWhenUntraced(t *testing.T) {
	env := buildResultEnvelope(agentapp.RunResult{SessionID: "s"}, ExitOK, nil, false, false)
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, sub := range []string{`"trace_id":""`, `"trace_path":""`} {
		if !strings.Contains(string(b), sub) {
			t.Errorf("envelope missing %s\nin: %s", sub, b)
		}
	}
}

func TestBuildResultEnvelope_JSONLAddsType(t *testing.T) {
	env := buildResultEnvelope(agentapp.RunResult{SessionID: "s"}, ExitOK, nil, false, true)
	if env.Type != "result" {
		t.Errorf("jsonl envelope.Type = %q, want %q", env.Type, "result")
	}
}

func TestBuildResultEnvelope_ErrorObject(t *testing.T) {
	env := buildResultEnvelope(agentapp.RunResult{}, ExitPolicyDenied, errors.New("denied"), true, false)
	if env.Error == nil {
		t.Fatal("error object missing")
	}
	if env.Error.Kind != "policy_denied" {
		t.Errorf("kind = %q", env.Error.Kind)
	}
	if env.Error.Message != "denied" {
		t.Errorf("message = %q", env.Error.Message)
	}
	if !env.PolicyDenied {
		t.Errorf("policy_denied flag not set")
	}
}

func TestEventToJSON_SkipsAndIncludes(t *testing.T) {
	// IterStart and Delta (without flag) suppressed.
	if _, ok := eventToJSON(agent.Event{Kind: agent.EventIterStart}, false); ok {
		t.Errorf("iter_start should be suppressed")
	}
	if _, ok := eventToJSON(agent.Event{Kind: agent.EventLLMDelta, Content: "x"}, false); ok {
		t.Errorf("delta should be suppressed without includeDeltas")
	}
	if _, ok := eventToJSON(agent.Event{Kind: agent.EventLLMDelta, Content: "x"}, true); !ok {
		t.Errorf("delta should be included with includeDeltas")
	}

	// Tool denied includes reason.
	b, ok := eventToJSON(agent.Event{
		Kind:       agent.EventToolDenied,
		ToolName:   "bash",
		ToolCallID: "id1",
		DenyReason: agent.DenyReasonPolicy,
	}, false)
	if !ok {
		t.Fatal("tool_denied should emit")
	}
	if !strings.Contains(string(b), `"type":"tool_denied"`) || !strings.Contains(string(b), `"reason":"policy"`) {
		t.Errorf("tool_denied payload missing fields: %s", string(b))
	}

	// Run end carries stats.
	b, ok = eventToJSON(agent.Event{
		Kind:  agent.EventRunEnd,
		Stats: agent.RunStats{ToolCalls: 3, PromptTokens: 11, CompletionTokens: 22},
	}, false)
	if !ok {
		t.Fatal("run_end should emit")
	}
	if !strings.Contains(string(b), `"tool_calls":3`) {
		t.Errorf("run_end missing tool_calls: %s", string(b))
	}
}

func TestParseOutputFormat(t *testing.T) {
	cases := map[string]OutputFormat{
		"":      OutputText,
		"text":  OutputText,
		"json":  OutputJSON,
		"jsonl": OutputJSONL,
	}
	for in, want := range cases {
		got, err := parseOutputFormat(in)
		if err != nil {
			t.Errorf("parseOutputFormat(%q) err: %v", in, err)
		}
		if got != want {
			t.Errorf("parseOutputFormat(%q) = %q, want %q", in, got, want)
		}
	}
	if _, err := parseOutputFormat("yaml"); err == nil {
		t.Errorf("parseOutputFormat(yaml) should error")
	}
}
