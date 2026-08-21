package mockllm_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/infra/llm"
	"github.com/gougoujiang/buildmax/internal/testsupport/mockllm"
)

// The suites this harness serves drive the real adapters, so the tests do too:
// a mock validated against a hand-written parser would only prove the mock
// agrees with itself.

func start(t *testing.T, scenario mockllm.Scenario) *mockllm.Server {
	t.Helper()
	server, err := mockllm.Start(scenario)
	if err != nil {
		t.Fatalf("start mockllm: %v", err)
	}
	t.Cleanup(server.Close)
	return server
}

func client(t *testing.T, server *mockllm.Server, protocol string) *llm.LLMClient {
	t.Helper()
	c, err := llm.NewClient(llm.Config{
		Provider: protocol,
		APIKey:   "mock-key",
		BaseURL:  server.BaseURL(protocol),
		Model:    "mock-model",
	})
	if err != nil {
		t.Fatalf("new client for %s: %v", protocol, err)
	}
	return c
}

var protocols = []string{
	mockllm.ProtocolOpenAIChat,
	mockllm.ProtocolOpenAIResponses,
	mockllm.ProtocolAnthropic,
}

func TestEveryProtocolReplaysTheScenario(t *testing.T) {
	scenario := mockllm.Scenario{Steps: []mockllm.Step{
		{
			Text:      "writing it now",
			ToolCalls: []mockllm.ToolCall{{Name: "Write", Args: map[string]any{"file_path": "notes.txt", "content": "hello"}}},
			Usage:     &mockllm.Usage{PromptTokens: 11, CompletionTokens: 7},
		},
		{Text: "done", Usage: &mockllm.Usage{PromptTokens: 13, CompletionTokens: 2}},
	}}

	for _, protocol := range protocols {
		t.Run(protocol, func(t *testing.T) {
			server := start(t, scenario)
			c := client(t, server, protocol)
			history := []cllm.Message{{Role: "user", Content: "write notes.txt"}}
			tools := []cllm.ToolDef{{Name: "Write", Description: "write a file", Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{"file_path": map[string]any{"type": "string"}, "content": map[string]any{"type": "string"}},
				"required":   []string{"file_path", "content"},
			}}}

			first, err := c.ChatCompletionBlocking(context.Background(), history, tools)
			if err != nil {
				t.Fatalf("first call: %v", err)
			}
			if first.Content != "writing it now" {
				t.Fatalf("content = %q, want %q", first.Content, "writing it now")
			}
			if len(first.ToolCalls) != 1 {
				t.Fatalf("tool calls = %d, want 1", len(first.ToolCalls))
			}
			call := first.ToolCalls[0]
			if call.Name != "Write" || call.ID == "" {
				t.Fatalf("tool call = %+v, want a named call with an id", call)
			}
			var args map[string]any
			if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
				t.Fatalf("arguments %q: %v", call.Arguments, err)
			}
			if args["file_path"] != "notes.txt" || args["content"] != "hello" {
				t.Fatalf("arguments = %v, want the scripted ones", args)
			}
			if first.Usage.PromptTokens != 11 || first.Usage.CompletionTokens != 7 {
				t.Fatalf("usage = %+v, want the scripted counts", first.Usage)
			}

			history = append(history,
				cllm.Message{Role: "assistant", Content: first.Content, ToolCalls: first.ToolCalls, ProviderState: first.ProviderState},
				cllm.Message{Role: "tool", ToolCallID: call.ID, Content: "wrote notes.txt"},
			)
			second, err := c.ChatCompletionBlocking(context.Background(), history, tools)
			if err != nil {
				t.Fatalf("second call: %v", err)
			}
			if second.Content != "done" || len(second.ToolCalls) != 0 {
				t.Fatalf("second completion = %+v, want the final text and no calls", second)
			}
			if server.Remaining() != 0 {
				t.Fatalf("remaining steps = %d, want 0", server.Remaining())
			}
			calls := server.Requests()
			if len(calls) != 2 {
				t.Fatalf("recorded calls = %d, want 2", len(calls))
			}
			if calls[0].Protocol != protocol {
				t.Fatalf("recorded protocol = %q, want %q", calls[0].Protocol, protocol)
			}
			// The second request has to carry the result the tool produced, or
			// the run under test never closed the loop.
			if !strings.Contains(string(calls[1].Body), "wrote notes.txt") {
				t.Fatalf("second request did not carry the tool result:\n%s", calls[1].Body)
			}
		})
	}
}

func TestOneTurnCarriesEveryScriptedCall(t *testing.T) {
	scenario := mockllm.Scenario{Steps: []mockllm.Step{{
		ToolCalls: []mockllm.ToolCall{
			{Name: "Read", Args: map[string]any{"file_path": "a.txt"}},
			{Name: "Read", Args: map[string]any{"file_path": "b.txt"}},
			{Name: "Read", Args: map[string]any{"file_path": "c.txt"}},
		},
	}}}
	for _, protocol := range protocols {
		t.Run(protocol, func(t *testing.T) {
			server := start(t, scenario)
			completion, err := client(t, server, protocol).ChatCompletionBlocking(
				context.Background(), []cllm.Message{{Role: "user", Content: "read them"}}, nil)
			if err != nil {
				t.Fatalf("call: %v", err)
			}
			if len(completion.ToolCalls) != 3 {
				t.Fatalf("tool calls = %d, want 3", len(completion.ToolCalls))
			}
			seen := map[string]bool{}
			for i, call := range completion.ToolCalls {
				if call.ID == "" || seen[call.ID] {
					t.Fatalf("call %d has a missing or repeated id: %q", i, call.ID)
				}
				seen[call.ID] = true
			}
			if !strings.Contains(completion.ToolCalls[0].Arguments, "a.txt") ||
				!strings.Contains(completion.ToolCalls[2].Arguments, "c.txt") {
				t.Fatalf("calls arrived out of order: %+v", completion.ToolCalls)
			}
		})
	}
}

func TestStreamingDeliversDeltasAndUsage(t *testing.T) {
	scenario := mockllm.Scenario{Steps: []mockllm.Step{{
		Text:      "streamed answer",
		ToolCalls: []mockllm.ToolCall{{Name: "Read", Args: map[string]any{"file_path": "a.txt"}}},
		Usage:     &mockllm.Usage{PromptTokens: 5, CompletionTokens: 9},
	}}}
	server := start(t, scenario)
	var deltas []string
	completion, err := client(t, server, mockllm.ProtocolOpenAIChat).ChatCompletionStreaming(
		context.Background(),
		[]cllm.Message{{Role: "user", Content: "answer"}},
		nil,
		func(delta string) { deltas = append(deltas, delta) },
	)
	if err != nil {
		t.Fatalf("streaming call: %v", err)
	}
	if len(deltas) < 2 {
		t.Fatalf("deltas = %v, want the text split across chunks", deltas)
	}
	if strings.Join(deltas, "") != "streamed answer" || completion.Content != "streamed answer" {
		t.Fatalf("streamed content = %q / %q, want %q", strings.Join(deltas, ""), completion.Content, "streamed answer")
	}
	if len(completion.ToolCalls) != 1 || completion.ToolCalls[0].Name != "Read" {
		t.Fatalf("tool calls = %+v, want the scripted Read", completion.ToolCalls)
	}
	if completion.Usage.PromptTokens != 5 || completion.Usage.CompletionTokens != 9 {
		t.Fatalf("usage = %+v, want the scripted counts", completion.Usage)
	}
	if !server.Requests()[0].Stream {
		t.Fatal("the recorded call should be marked as streaming")
	}
}

func TestUnscriptedStreamingFailsLoudly(t *testing.T) {
	// The other two protocols stream through their own event shapes, which this
	// harness does not script yet. Answering them with a blocking body would
	// fail somewhere far less informative.
	for _, protocol := range []string{mockllm.ProtocolOpenAIResponses, mockllm.ProtocolAnthropic} {
		t.Run(protocol, func(t *testing.T) {
			server := start(t, mockllm.Scenario{Steps: []mockllm.Step{{Text: "unused"}}})
			_, err := client(t, server, protocol).ChatCompletionStreaming(
				context.Background(), []cllm.Message{{Role: "user", Content: "hi"}}, nil, nil)
			if err == nil {
				t.Fatal("streaming should fail while it is unscripted")
			}
			if server.Remaining() != 1 {
				t.Fatalf("remaining steps = %d, want the step left unconsumed", server.Remaining())
			}
		})
	}
}

func TestExhaustedScenarioFailsTheCall(t *testing.T) {
	server := start(t, mockllm.Scenario{Steps: []mockllm.Step{{Text: "only reply"}}})
	c := client(t, server, mockllm.ProtocolOpenAIChat)
	history := []cllm.Message{{Role: "user", Content: "hi"}}
	if _, err := c.ChatCompletionBlocking(context.Background(), history, nil); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := c.ChatCompletionBlocking(context.Background(), history, nil); err == nil {
		t.Fatal("a call past the end of the scenario should fail")
	}
}

func TestUnconsumedStepsAreVisible(t *testing.T) {
	server := start(t, mockllm.Scenario{Steps: []mockllm.Step{{Text: "one"}, {Text: "two"}}})
	if _, err := client(t, server, mockllm.ProtocolOpenAIChat).ChatCompletionBlocking(
		context.Background(), []cllm.Message{{Role: "user", Content: "hi"}}, nil); err != nil {
		t.Fatalf("call: %v", err)
	}
	if server.Remaining() != 1 {
		t.Fatalf("remaining steps = %d, want 1", server.Remaining())
	}
}

func TestScriptedProviderErrorReachesTheCaller(t *testing.T) {
	server := start(t, mockllm.Scenario{Steps: []mockllm.Step{{Status: 400, Error: "scripted refusal"}}})
	_, err := client(t, server, mockllm.ProtocolOpenAIChat).ChatCompletionBlocking(
		context.Background(), []cllm.Message{{Role: "user", Content: "hi"}}, nil)
	if err == nil {
		t.Fatal("a scripted provider error should fail the call")
	}
	if !strings.Contains(err.Error(), "scripted refusal") {
		t.Fatalf("error = %v, want it to carry the scripted message", err)
	}
}

func TestLoadScenarioReadsACommittedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scenario.json")
	body := `{"name":"write once","steps":[{"tool_calls":[{"name":"Write","args":{"file_path":"a.txt","content":"hi"}}]},{"text":"done"}]}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write scenario: %v", err)
	}
	scenario, err := mockllm.LoadScenario(path)
	if err != nil {
		t.Fatalf("load scenario: %v", err)
	}
	if scenario.Name != "write once" || len(scenario.Steps) != 2 {
		t.Fatalf("scenario = %+v, want the two scripted steps", scenario)
	}
	if scenario.Steps[0].ToolCalls[0].Args["file_path"] != "a.txt" {
		t.Fatalf("arguments = %v, want the scripted path", scenario.Steps[0].ToolCalls[0].Args)
	}

	empty := filepath.Join(t.TempDir(), "empty.json")
	if err := os.WriteFile(empty, []byte(`{"steps":[]}`), 0o600); err != nil {
		t.Fatalf("write scenario: %v", err)
	}
	if _, err := mockllm.LoadScenario(empty); err == nil {
		t.Fatal("a scenario with no steps should not load")
	}
}
