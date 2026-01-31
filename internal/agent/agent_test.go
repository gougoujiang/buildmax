package agent

import (
	"context"
	"sync"
	"testing"

	"buildmax/internal/llm"
)

// mockLLMCaller is a fake LLM that returns configured content and tool calls.
type mockLLMCaller struct {
	mu         sync.Mutex
	calls      int
	responses  []mockResponse // per-call response
}

type mockResponse struct {
	content   string
	toolCalls []llm.ToolCall
}

func (m *mockLLMCaller) ChatWithTools(ctx context.Context, messages []llm.Message, tools []llm.ToolDef) (content string, toolCalls []llm.ToolCall, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.calls >= len(m.responses) {
		return "", nil, nil
	}
	r := m.responses[m.calls]
	m.calls++
	return r.content, r.toolCalls, nil
}

func (m *mockLLMCaller) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

// mockTool is a fake tool that records executions and returns a fixed string.
type mockTool struct {
	name        string
	description string
	params      any
	result      string
	executed    int
	mu          sync.Mutex
}

func (t *mockTool) Name() string        { return t.name }
func (t *mockTool) Description() string { return t.description }
func (t *mockTool) Parameters() any     { return t.params }
func (t *mockTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	t.mu.Lock()
	t.executed++
	t.mu.Unlock()
	return t.result, nil
}

func (t *mockTool) executionCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.executed
}

// TestProcess_NoToolCall asserts that when the LLM returns final content with no tool_calls,
// Process returns that content and no tool is executed.
func TestProcess_NoToolCall(t *testing.T) {
	ctx := context.Background()
	mock := &mockLLMCaller{
		responses: []mockResponse{
			{content: "Hello, I am the final reply.", toolCalls: nil},
		},
	}
	tool := &mockTool{
		name:        "echo",
		description: "Echoes input",
		params:      map[string]any{"type": "object"},
		result:      "echoed",
	}
	a := NewAgent(mock, []Tool{tool})
	reply, err := a.Process(ctx, "Hi")
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if reply != "Hello, I am the final reply." {
		t.Errorf("reply = %q, want %q", reply, "Hello, I am the final reply.")
	}
	if tool.executionCount() != 0 {
		t.Errorf("tool executed %d times, want 0", tool.executionCount())
	}
	if mock.callCount() != 1 {
		t.Errorf("LLM called %d times, want 1", mock.callCount())
	}
}

// TestProcess_WithToolCall asserts that when the LLM returns a tool_call, the agent executes
// the tool, sends results back, and returns the final content from the next LLM call.
func TestProcess_WithToolCall(t *testing.T) {
	ctx := context.Background()
	toolResult := "the tool result"
	mock := &mockLLMCaller{
		responses: []mockResponse{
			{
				content: "",
				toolCalls: []llm.ToolCall{
					{ID: "call-1", Name: "get_weather", Arguments: `{"location":"Boston"}`},
				},
			},
			{content: "The weather in Boston is nice.", toolCalls: nil},
		},
	}
	tool := &mockTool{
		name:        "get_weather",
		description: "Get weather for a location",
		params:      map[string]any{"type": "object", "properties": map[string]any{"location": map[string]any{"type": "string"}}},
		result:      toolResult,
	}
	a := NewAgent(mock, []Tool{tool})
	reply, err := a.Process(ctx, "What is the weather in Boston?")
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if reply != "The weather in Boston is nice." {
		t.Errorf("reply = %q, want %q", reply, "The weather in Boston is nice.")
	}
	if tool.executionCount() != 1 {
		t.Errorf("tool executed %d times, want 1", tool.executionCount())
	}
	if mock.callCount() != 2 {
		t.Errorf("LLM called %d times, want 2", mock.callCount())
	}
}

// recordingLLMCaller wraps an LLMCaller and records the messages from the first ChatWithTools call.
type recordingLLMCaller struct {
	inner    *mockLLMCaller
	firstMsg []llm.Message
	once     sync.Once
}

func (r *recordingLLMCaller) ChatWithTools(ctx context.Context, messages []llm.Message, tools []llm.ToolDef) (content string, toolCalls []llm.ToolCall, err error) {
	r.once.Do(func() {
		r.firstMsg = make([]llm.Message, len(messages))
		copy(r.firstMsg, messages)
	})
	return r.inner.ChatWithTools(ctx, messages, tools)
}

// TestProcess_SystemPromptPrepend verifies that the first ChatWithTools call receives
// a system message with DefaultSystemPrompt first, then the user message.
func TestProcess_SystemPromptPrepend(t *testing.T) {
	ctx := context.Background()
	userMsg := "Hello, assistant."
	inner := &mockLLMCaller{
		responses: []mockResponse{
			{content: "Hi there.", toolCalls: nil},
		},
	}
	rec := &recordingLLMCaller{inner: inner}
	a := NewAgent(rec, nil)
	_, err := a.Process(ctx, userMsg)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(rec.firstMsg) < 2 {
		t.Fatalf("first call: len(messages) = %d, want at least 2", len(rec.firstMsg))
	}
	if rec.firstMsg[0].Role != "system" {
		t.Errorf("messages[0].Role = %q, want \"system\"", rec.firstMsg[0].Role)
	}
	if rec.firstMsg[0].Content != DefaultSystemPrompt {
		t.Errorf("messages[0].Content = %q, want %q", rec.firstMsg[0].Content, DefaultSystemPrompt)
	}
	if rec.firstMsg[1].Role != "user" {
		t.Errorf("messages[1].Role = %q, want \"user\"", rec.firstMsg[1].Role)
	}
	if rec.firstMsg[1].Content != userMsg {
		t.Errorf("messages[1].Content = %q, want %q", rec.firstMsg[1].Content, userMsg)
	}
}

// TestProcess_MaxIterationsExceeded asserts that when the LLM keeps returning tool_calls
// without ever returning final content, Process returns an error after max iterations.
func TestProcess_MaxIterationsExceeded(t *testing.T) {
	ctx := context.Background()
	// Always return a tool call, never final content
	var responses []mockResponse
	for i := 0; i < 15; i++ {
		responses = append(responses, mockResponse{
			content: "",
			toolCalls: []llm.ToolCall{
				{ID: "call-1", Name: "ping", Arguments: "{}"},
			},
		})
	}
	mock := &mockLLMCaller{responses: responses}
	tool := &mockTool{
		name:        "ping",
		description: "Ping",
		params:      map[string]any{"type": "object"},
		result:      "pong",
	}
	a := NewAgent(mock, []Tool{tool}, MaxIterations(3))
	_, err := a.Process(ctx, "ping")
	if err == nil {
		t.Fatal("Process: expected error for max iterations exceeded")
	}
	if mock.callCount() != 3 {
		t.Errorf("LLM called %d times, want 3 (max iter)", mock.callCount())
	}
}
