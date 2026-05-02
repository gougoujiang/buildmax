package agent

import (
	"context"
	"strings"
	"sync"
	"testing"

	"buildmax/internal/core/model"
)

// TestBuildToolDefs asserts BuildToolDefs builds one model.ToolDef per tool with correct name/description/parameters.
func TestBuildToolDefs(t *testing.T) {
	mock := &mockTool{
		name:        "test_tool",
		description: "A test tool",
		params:      map[string]any{"type": "object", "properties": map[string]any{"x": map[string]any{"type": "string"}}},
	}
	defs := BuildToolDefs([]model.Tool{mock})
	if len(defs) != 1 {
		t.Fatalf("len(defs) = %d, want 1", len(defs))
	}
	if defs[0].Name != "test_tool" {
		t.Errorf("defs[0].Name = %q, want test_tool", defs[0].Name)
	}
	if defs[0].Description != "A test tool" {
		t.Errorf("defs[0].Description = %q, want A test tool", defs[0].Description)
	}
	if defs[0].Parameters == nil {
		t.Error("defs[0].Parameters is nil")
	}
	// Ensure mockTool implements model.Tool
	var _ model.Tool = (*mockTool)(nil)
	// Empty tools
	empty := BuildToolDefs(nil)
	if len(empty) != 0 {
		t.Errorf("BuildToolDefs(nil) length = %d, want 0", len(empty))
	}
}

// TestExecuteTool asserts ExecuteTool parses arguments, calls Execute, and returns result or error string.
func TestExecuteTool(t *testing.T) {
	ctx := context.Background()
	mock := &mockTool{name: "echo", result: "ok"}
	t.Run("success", func(t *testing.T) {
		out := ExecuteTool(ctx, mock, model.ToolCall{ID: "1", Name: "echo", Arguments: `{"a":1}`})
		if out != "ok" {
			t.Errorf("ExecuteTool = %q, want ok", out)
		}
	})
	t.Run("invalid_json", func(t *testing.T) {
		out := ExecuteTool(ctx, mock, model.ToolCall{ID: "2", Name: "echo", Arguments: `{invalid`})
		if out == "" || !strings.HasPrefix(out, "error:") {
			t.Errorf("ExecuteTool = %q, want error prefix", out)
		}
	})
	t.Run("empty_arguments", func(t *testing.T) {
		out := ExecuteTool(ctx, mock, model.ToolCall{ID: "3", Name: "echo", Arguments: ""})
		if out != "ok" {
			t.Errorf("ExecuteTool = %q, want ok", out)
		}
	})
}

type testBuffer struct {
	messages []model.Message
}

func newTestBuffer() *testBuffer { return &testBuffer{} }

func (b *testBuffer) Messages() []model.Message {
	if len(b.messages) == 0 {
		return nil
	}
	out := make([]model.Message, len(b.messages))
	copy(out, b.messages)
	return out
}

func (b *testBuffer) Append(msg model.Message) error {
	b.messages = append(b.messages, msg)
	return nil
}

// mockLLMClient is a fake LLM that returns configured content and tool calls.
type mockLLMClient struct {
	mu        sync.Mutex
	calls     int
	responses []mockResponse // per-call response
}

type mockResponse struct {
	content   string
	toolCalls []model.ToolCall
	usage     model.Usage
}

func (m *mockLLMClient) ChatCompletionBlocking(ctx context.Context, messages []model.Message, tools []model.ToolDef) (content string, toolCalls []model.ToolCall, usage model.Usage, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.calls >= len(m.responses) {
		return "", nil, model.Usage{}, nil
	}
	r := m.responses[m.calls]
	m.calls++
	return r.content, r.toolCalls, r.usage, nil
}

func (m *mockLLMClient) ChatCompletionStreaming(ctx context.Context, messages []model.Message, tools []model.ToolDef, onDelta func(string)) (content string, toolCalls []model.ToolCall, usage model.Usage, err error) {
	content, toolCalls, usage, err = m.ChatCompletionBlocking(ctx, messages, tools)
	if err == nil && onDelta != nil && content != "" {
		onDelta(content)
	}
	return content, toolCalls, usage, err
}

// Ensure mockLLMClient implements model.LLMClient.
var _ model.LLMClient = (*mockLLMClient)(nil)

func (m *mockLLMClient) callCount() int {
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
	mock := &mockLLMClient{
		responses: []mockResponse{
			{content: "Hello, I am the final reply.", toolCalls: nil},
		},
	}
	mockTool := &mockTool{
		name:        "echo",
		description: "Echoes input",
		params:      map[string]any{"type": "object"},
		result:      "echoed",
	}
	a := NewAgent(mock, []model.Tool{mockTool})
	sess := newTestBuffer()
	reply, stats, err := a.Process(ctx, sess, "Hi")
	if err != nil {
		t.Fatalf("ProcessWithSession: %v", err)
	}
	if reply != "Hello, I am the final reply." {
		t.Errorf("reply = %q, want %q", reply, "Hello, I am the final reply.")
	}
	if stats.ToolCalls != 0 {
		t.Errorf("stats.ToolCalls = %d, want 0", stats.ToolCalls)
	}
	if mockTool.executionCount() != 0 {
		t.Errorf("tool executed %d times, want 0", mockTool.executionCount())
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
	mock := &mockLLMClient{
		responses: []mockResponse{
			{
				content: "",
				toolCalls: []model.ToolCall{
					{ID: "call-1", Name: "get_weather", Arguments: `{"location":"Boston"}`},
				},
			},
			{content: "The weather in Boston is nice.", toolCalls: nil},
		},
	}
	mockTool := &mockTool{
		name:        "get_weather",
		description: "Get weather for a location",
		params:      map[string]any{"type": "object", "properties": map[string]any{"location": map[string]any{"type": "string"}}},
		result:      toolResult,
	}
	a := NewAgent(mock, []model.Tool{mockTool})
	sess := newTestBuffer()
	reply, stats, err := a.Process(ctx, sess, "What is the weather in Boston?")
	if err != nil {
		t.Fatalf("ProcessWithSession: %v", err)
	}
	if reply != "The weather in Boston is nice." {
		t.Errorf("reply = %q, want %q", reply, "The weather in Boston is nice.")
	}
	if stats.ToolCalls != 1 {
		t.Errorf("stats.ToolCalls = %d, want 1", stats.ToolCalls)
	}
	if mockTool.executionCount() != 1 {
		t.Errorf("tool executed %d times, want 1", mockTool.executionCount())
	}
	if mock.callCount() != 2 {
		t.Errorf("LLM called %d times, want 2", mock.callCount())
	}
}

// TestProcessWithSession_AccumulatesUsage asserts that RunStats accumulates prompt and completion tokens across LLM calls.
func TestProcessWithSession_AccumulatesUsage(t *testing.T) {
	ctx := context.Background()
	mock := &mockLLMClient{
		responses: []mockResponse{
			{content: "", toolCalls: []model.ToolCall{{ID: "1", Name: "echo", Arguments: "{}"}}, usage: model.Usage{PromptTokens: 10, CompletionTokens: 5}},
			{content: "Done.", toolCalls: nil, usage: model.Usage{PromptTokens: 20, CompletionTokens: 8}},
		},
	}
	mockTool := &mockTool{name: "echo", description: "Echo", params: map[string]any{"type": "object"}, result: "ok"}
	a := NewAgent(mock, []model.Tool{mockTool})
	sess := newTestBuffer()
	reply, stats, err := a.Process(ctx, sess, "Hi")
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if reply != "Done." {
		t.Errorf("reply = %q, want %q", reply, "Done.")
	}
	if stats.ToolCalls != 1 {
		t.Errorf("stats.ToolCalls = %d, want 1", stats.ToolCalls)
	}
	if stats.PromptTokens != 30 {
		t.Errorf("stats.PromptTokens = %d, want 30", stats.PromptTokens)
	}
	if stats.CompletionTokens != 13 {
		t.Errorf("stats.CompletionTokens = %d, want 13", stats.CompletionTokens)
	}
}

// recordingLLMClient wraps an model.LLMClient and records the messages from the first ChatCompletionBlocking call.
type recordingLLMClient struct {
	inner    *mockLLMClient
	firstMsg []model.Message
	once     sync.Once
}

func (r *recordingLLMClient) ChatCompletionBlocking(ctx context.Context, messages []model.Message, tools []model.ToolDef) (content string, toolCalls []model.ToolCall, usage model.Usage, err error) {
	r.once.Do(func() {
		r.firstMsg = make([]model.Message, len(messages))
		copy(r.firstMsg, messages)
	})
	return r.inner.ChatCompletionBlocking(ctx, messages, tools)
}

func (r *recordingLLMClient) ChatCompletionStreaming(ctx context.Context, messages []model.Message, tools []model.ToolDef, onDelta func(string)) (content string, toolCalls []model.ToolCall, usage model.Usage, err error) {
	return r.inner.ChatCompletionStreaming(ctx, messages, tools, onDelta)
}

// lastCallLLMClient records the messages from the last ChatCompletionBlocking call (for ProcessWithSession tests).
type lastCallLLMClient struct {
	inner   *mockLLMClient
	lastMsg []model.Message
	lastMu  sync.Mutex
}

func (r *lastCallLLMClient) ChatCompletionBlocking(ctx context.Context, messages []model.Message, tools []model.ToolDef) (content string, toolCalls []model.ToolCall, usage model.Usage, err error) {
	r.lastMu.Lock()
	r.lastMsg = make([]model.Message, len(messages))
	copy(r.lastMsg, messages)
	r.lastMu.Unlock()
	return r.inner.ChatCompletionBlocking(ctx, messages, tools)
}

func (r *lastCallLLMClient) ChatCompletionStreaming(ctx context.Context, messages []model.Message, tools []model.ToolDef, onDelta func(string)) (content string, toolCalls []model.ToolCall, usage model.Usage, err error) {
	return r.inner.ChatCompletionStreaming(ctx, messages, tools, onDelta)
}

func (r *lastCallLLMClient) lastMessages() []model.Message {
	r.lastMu.Lock()
	defer r.lastMu.Unlock()
	return r.lastMsg
}

// TestProcessWithSession_SystemPromptPrepend verifies that the first ChatCompletionBlocking call receives
// a system message with DefaultSystemPrompt first, then the user message.
func TestProcessWithSession_SystemPromptPrepend(t *testing.T) {
	ctx := context.Background()
	userMsg := "Hello, assistant."
	inner := &mockLLMClient{
		responses: []mockResponse{
			{content: "Hi there.", toolCalls: nil},
		},
	}
	rec := &recordingLLMClient{inner: inner}
	a := NewAgent(rec, nil)
	sess := newTestBuffer()
	_, _, err := a.Process(ctx, sess, userMsg)
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
			toolCalls: []model.ToolCall{
				{ID: "call-1", Name: "ping", Arguments: "{}"},
			},
		})
	}
	mock := &mockLLMClient{responses: responses}
	mockTool := &mockTool{
		name:        "ping",
		description: "Ping",
		params:      map[string]any{"type": "object"},
		result:      "pong",
	}
	a := NewAgent(mock, []model.Tool{mockTool}, MaxIterations(3))
	sess := newTestBuffer()
	_, stats, err := a.Process(ctx, sess, "ping")
	if err == nil {
		t.Fatal("ProcessWithSession: expected error for max iterations exceeded")
	}
	if stats.ToolCalls != 3 {
		t.Errorf("stats.ToolCalls = %d, want 3", stats.ToolCalls)
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
	inner := &mockLLMClient{
		responses: []mockResponse{
			{content: firstReply, toolCalls: nil},
			{content: secondReply, toolCalls: nil},
		},
	}
	rec := &lastCallLLMClient{inner: inner}
	a := NewAgent(rec, nil)
	sess := newTestBuffer()

	reply1, _, err := a.Process(ctx, sess, "First message")
	if err != nil {
		t.Fatalf("first ProcessWithSession: %v", err)
	}
	if reply1 != firstReply {
		t.Errorf("first reply = %q, want %q", reply1, firstReply)
	}

	reply2, _, err := a.Process(ctx, sess, "Second message")
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

// recordingStreamSink records each OnDelta call for tests.
var _ model.StreamSink = (*recordingStreamSink)(nil)

type recordingStreamSink struct {
	deltas []string
	mu     sync.Mutex
}

func (r *recordingStreamSink) OnDelta(delta string) {
	r.mu.Lock()
	r.deltas = append(r.deltas, delta)
	r.mu.Unlock()
}

func (r *recordingStreamSink) getDeltas() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.deltas...)
}

// TestProcess_Streaming asserts that when WithStreamSink is used, the sink receives content deltas
// and the session ends with the full assistant message.
func TestProcess_Streaming(t *testing.T) {
	ctx := context.Background()
	fullReply := "Hello, I am the streamed reply."
	mock := &mockLLMClient{
		responses: []mockResponse{
			{content: fullReply, toolCalls: nil},
		},
	}
	sink := &recordingStreamSink{}
	a := NewAgent(mock, nil)
	sess := newTestBuffer()
	reply, _, err := a.Process(ctx, sess, "Hi", WithStreamSink(sink))
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if reply != fullReply {
		t.Errorf("reply = %q, want %q", reply, fullReply)
	}
	deltas := sink.getDeltas()
	if len(deltas) == 0 {
		t.Error("streaming sink received no deltas")
	}
	if got := strings.Join(deltas, ""); got != fullReply {
		t.Errorf("deltas joined = %q, want %q", got, fullReply)
	}
	msgs := sess.Messages()
	if len(msgs) != 2 {
		t.Fatalf("session has %d messages, want 2", len(msgs))
	}
	if msgs[1].Role != "assistant" || msgs[1].Content != fullReply {
		t.Errorf("session last message = %q %q", msgs[1].Role, msgs[1].Content)
	}
}

// TestProcessAfterUserAppended asserts that when the session already has a user message appended,
// ProcessAfterUserAppended runs the loop without appending again and returns the assistant reply.
func TestProcessAfterUserAppended(t *testing.T) {
	ctx := context.Background()
	mock := &mockLLMClient{
		responses: []mockResponse{
			{content: "Reply to you.", toolCalls: nil},
		},
	}
	a := NewAgent(mock, nil)
	sess := newTestBuffer()
	sess.Append(model.Message{Role: "user", Content: "Hello"})

	reply, _, err := a.ProcessAfterUserAppended(ctx, sess)
	if err != nil {
		t.Fatalf("ProcessAfterUserAppended: %v", err)
	}
	if reply != "Reply to you." {
		t.Errorf("reply = %q, want %q", reply, "Reply to you.")
	}
	msgs := sess.Messages()
	if len(msgs) != 2 {
		t.Fatalf("session has %d messages, want 2 (user + assistant)", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "Hello" {
		t.Errorf("msgs[0] = %q %q", msgs[0].Role, msgs[0].Content)
	}
	if msgs[1].Role != "assistant" || msgs[1].Content != "Reply to you." {
		t.Errorf("msgs[1] = %q %q", msgs[1].Role, msgs[1].Content)
	}
	if mock.callCount() != 1 {
		t.Errorf("LLM called %d times, want 1", mock.callCount())
	}
}

// TestProcessAfterUserAppended_EmptySession returns an error when session has no messages.
func TestProcessAfterUserAppended_EmptySession(t *testing.T) {
	ctx := context.Background()
	a := NewAgent(&mockLLMClient{}, nil)
	sess := newTestBuffer()
	_, _, err := a.ProcessAfterUserAppended(ctx, sess)
	if err == nil {
		t.Fatal("ProcessAfterUserAppended: expected error for empty session")
	}
}

// TestProcessAfterUserAppended_LastNotUser returns an error when last message is not user.
func TestProcessAfterUserAppended_LastNotUser(t *testing.T) {
	ctx := context.Background()
	a := NewAgent(&mockLLMClient{}, nil)
	sess := newTestBuffer()
	sess.Append(model.Message{Role: "assistant", Content: "Hi"})
	_, _, err := a.ProcessAfterUserAppended(ctx, sess)
	if err == nil {
		t.Fatal("ProcessAfterUserAppended: expected error when last message is assistant")
	}
}

// TestSystemPromptOption verifies that the SystemPrompt option overrides the default system prompt
// used in processLoop, so the first message sent to ChatCompletionBlocking uses the custom prompt.
func TestSystemPromptOption(t *testing.T) {
	ctx := context.Background()
	customPrompt := "You are a specialized sub-agent for code exploration."
	inner := &mockLLMClient{
		responses: []mockResponse{
			{content: "Done.", toolCalls: nil},
		},
	}
	rec := &recordingLLMClient{inner: inner}
	a := NewAgent(rec, nil, SystemPrompt(customPrompt))
	sess := newTestBuffer()
	_, _, err := a.Process(ctx, sess, "explore the code")
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(rec.firstMsg) < 2 {
		t.Fatalf("first call: len(messages) = %d, want at least 2", len(rec.firstMsg))
	}
	if rec.firstMsg[0].Role != "system" {
		t.Errorf("messages[0].Role = %q, want \"system\"", rec.firstMsg[0].Role)
	}
	if rec.firstMsg[0].Content != customPrompt {
		t.Errorf("messages[0].Content = %q, want custom prompt", rec.firstMsg[0].Content)
	}
}
