package agent

import (
	"context"
	"sync"
	"testing"

	"buildmax/internal/llm"
	"buildmax/internal/session"
)

// mockLLMCaller is a fake LLM that returns configured content and tool calls.
type mockLLMCaller struct {
	mu        sync.Mutex
	calls     int
	responses []mockResponse // per-call response
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

// TestProcessWithSession_NoToolCall asserts that when the LLM returns final content with no tool_calls,
// ProcessWithSession returns that content and no tool is executed.
func TestProcessWithSession_NoToolCall(t *testing.T) {
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
	sess := session.NewSession("")
	reply, err := a.Process(ctx, sess, "Hi")
	if err != nil {
		t.Fatalf("ProcessWithSession: %v", err)
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

// TestProcessWithSession_WithToolCall asserts that when the LLM returns a tool_call, the agent
// executes the tool, sends results back, and returns the final content from the next LLM call.
func TestProcessWithSession_WithToolCall(t *testing.T) {
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
	sess := session.NewSession("")
	reply, err := a.Process(ctx, sess, "What is the weather in Boston?")
	if err != nil {
		t.Fatalf("ProcessWithSession: %v", err)
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

// lastCallLLMCaller records the messages from the last ChatWithTools call (for ProcessWithSession tests).
type lastCallLLMCaller struct {
	inner   *mockLLMCaller
	lastMsg []llm.Message
	lastMu  sync.Mutex
}

func (r *lastCallLLMCaller) ChatWithTools(ctx context.Context, messages []llm.Message, tools []llm.ToolDef) (content string, toolCalls []llm.ToolCall, err error) {
	r.lastMu.Lock()
	r.lastMsg = make([]llm.Message, len(messages))
	copy(r.lastMsg, messages)
	r.lastMu.Unlock()
	return r.inner.ChatWithTools(ctx, messages, tools)
}

func (r *lastCallLLMCaller) lastMessages() []llm.Message {
	r.lastMu.Lock()
	defer r.lastMu.Unlock()
	return r.lastMsg
}

// TestProcessWithSession_SystemPromptPrepend verifies that the first ChatWithTools call receives
// a system message with DefaultSystemPrompt first, then the user message.
func TestProcessWithSession_SystemPromptPrepend(t *testing.T) {
	ctx := context.Background()
	userMsg := "Hello, assistant."
	inner := &mockLLMCaller{
		responses: []mockResponse{
			{content: "Hi there.", toolCalls: nil},
		},
	}
	rec := &recordingLLMCaller{inner: inner}
	a := NewAgent(rec, nil)
	sess := session.NewSession("")
	_, err := a.Process(ctx, sess, userMsg)
	if err != nil {
		t.Fatalf("ProcessWithSession: %v", err)
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

// TestProcessWithSession_MaxIterationsExceeded asserts that when the LLM keeps returning tool_calls
// without ever returning final content, ProcessWithSession returns an error after max iterations.
func TestProcessWithSession_MaxIterationsExceeded(t *testing.T) {
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
	sess := session.NewSession("")
	_, err := a.Process(ctx, sess, "ping")
	if err == nil {
		t.Fatal("ProcessWithSession: expected error for max iterations exceeded")
	}
	if mock.callCount() != 3 {
		t.Errorf("LLM called %d times, want 3 (max iter)", mock.callCount())
	}
}

// TestProcessWithSession verifies that two turns with the same session result in session
// history containing both user messages and assistant reply(ies) in order, and that the
// second LLM call receives the full history (system + first user + first assistant + second user).
func TestProcessWithSession(t *testing.T) {
	ctx := context.Background()
	firstReply := "First reply."
	secondReply := "Second reply."
	inner := &mockLLMCaller{
		responses: []mockResponse{
			{content: firstReply, toolCalls: nil},
			{content: secondReply, toolCalls: nil},
		},
	}
	rec := &lastCallLLMCaller{inner: inner}
	a := NewAgent(rec, nil)
	sess := session.NewSession("")

	reply1, err := a.Process(ctx, sess, "First message")
	if err != nil {
		t.Fatalf("first ProcessWithSession: %v", err)
	}
	if reply1 != firstReply {
		t.Errorf("first reply = %q, want %q", reply1, firstReply)
	}

	reply2, err := a.Process(ctx, sess, "Second message")
	if err != nil {
		t.Fatalf("second ProcessWithSession: %v", err)
	}
	if reply2 != secondReply {
		t.Errorf("second reply = %q, want %q", reply2, secondReply)
	}

	msgs := sess.Messages()
	// Expect: user1, assistant1, user2, assistant2 (4 messages)
	if len(msgs) != 4 {
		t.Fatalf("session Messages() length = %d, want 4", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "First message" {
		t.Errorf("msgs[0] = %q %q", msgs[0].Role, msgs[0].Content)
	}
	if msgs[1].Role != "assistant" || msgs[1].Content != firstReply {
		t.Errorf("msgs[1] = %q %q", msgs[1].Role, msgs[1].Content)
	}
	if msgs[2].Role != "user" || msgs[2].Content != "Second message" {
		t.Errorf("msgs[2] = %q %q", msgs[2].Role, msgs[2].Content)
	}
	if msgs[3].Role != "assistant" || msgs[3].Content != secondReply {
		t.Errorf("msgs[3] = %q %q", msgs[3].Role, msgs[3].Content)
	}

	// Second LLM call should receive: system, first user, first assistant, second user (4 messages)
	lastMsg := rec.lastMessages()
	if len(lastMsg) != 4 {
		t.Fatalf("last LLM call messages length = %d, want 4 (system + user1 + assistant1 + user2)", len(lastMsg))
	}
	if lastMsg[0].Role != "system" || lastMsg[0].Content != DefaultSystemPrompt {
		t.Errorf("lastMsg[0]: role=%q content mismatch", lastMsg[0].Role)
	}
	if lastMsg[1].Role != "user" || lastMsg[1].Content != "First message" {
		t.Errorf("lastMsg[1] = %q %q", lastMsg[1].Role, lastMsg[1].Content)
	}
	if lastMsg[2].Role != "assistant" || lastMsg[2].Content != firstReply {
		t.Errorf("lastMsg[2] = %q %q", lastMsg[2].Role, lastMsg[2].Content)
	}
	if lastMsg[3].Role != "user" || lastMsg[3].Content != "Second message" {
		t.Errorf("lastMsg[3] = %q %q", lastMsg[3].Role, lastMsg[3].Content)
	}
}
