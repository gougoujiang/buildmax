// Conversation agent loop: load messages, call LLM with tools, persist each turn.
package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"buildmax/internal/llm"
	"buildmax/internal/storage/entity"
)

const maxIterations = 10
const systemPrompt = "You are a helpful assistant. You can call get_current_date to get today's date. Reply concisely."

// LLMCaller can perform chat with tools for the conversation loop.
type LLMCaller interface {
	ChatWithTools(ctx context.Context, messages []llm.Message, tools []llm.ToolDef) (content string, toolCalls []llm.ToolCall, usage llm.Usage, err error)
}

// StreamLLMCaller can perform chat with tools and stream content deltas.
type StreamLLMCaller interface {
	ChatWithToolsStream(ctx context.Context, messages []llm.Message, tools []llm.ToolDef, onDelta func(string)) (content string, toolCalls []llm.ToolCall, usage llm.Usage, err error)
}

// StreamSink receives content deltas during streaming. Implementations write to the response or buffer.
type StreamSink interface {
	OnDelta(delta string)
}

// RunLoop loads conversation messages, appends the new user message, runs the LLM loop with the given
// tools, and persists every assistant and tool message to the store. Returns the final assistant text reply.
// If tools is nil, DefaultTools() is used.
func RunLoop(
	ctx context.Context,
	convStore entity.ConversationStore,
	msgStore entity.ConversationMessageStore,
	llmCaller LLMCaller,
	conversationID string,
	userContent string,
	channel string,
	tools []Tool,
) (reply string, err error) {
	if llmCaller == nil {
		return "", fmt.Errorf("conversation LLM not configured")
	}
	msgs, err := msgStore.ListMessages(ctx, conversationID)
	if err != nil {
		return "", fmt.Errorf("list messages: %w", err)
	}
	// Append user message
	channelPtr := &channel
	if _, err := msgStore.AppendMessage(ctx, conversationID, "user", userContent, channelPtr, nil, nil); err != nil {
		return "", fmt.Errorf("append user message: %w", err)
	}
	// Build LLM message list from stored messages + new user message
	llmMsgs := make([]llm.Message, 0, len(msgs)+2)
	for _, m := range msgs {
		toolCallID := ""
		if m.ToolCallID != nil {
			toolCallID = *m.ToolCallID
		}
		msg := llm.Message{Role: m.Role, Content: m.Content, ToolCallID: toolCallID}
		if m.ToolCallsJSON != nil && *m.ToolCallsJSON != "" {
			var toolCalls []llm.ToolCall
			if err := json.Unmarshal([]byte(*m.ToolCallsJSON), &toolCalls); err == nil {
				msg.ToolCalls = toolCalls
			}
		}
		llmMsgs = append(llmMsgs, msg)
	}
	llmMsgs = append(llmMsgs, llm.Message{Role: "user", Content: userContent})

	if tools == nil {
		tools = DefaultTools()
	}
	defs := toolDefs(tools)
	toolsByName := make(map[string]Tool, len(tools))
	for _, t := range tools {
		toolsByName[t.Name()] = t
	}

	for i := 0; i < maxIterations; i++ {
		slog.Debug("conversation iteration", "iter", i+1, "conversation_id", conversationID)
		messages := append([]llm.Message{{Role: "system", Content: systemPrompt}}, llmMsgs...)
		content, toolCalls, _, callErr := llmCaller.ChatWithTools(ctx, messages, defs)
		if callErr != nil {
			return "", fmt.Errorf("llm call: %w", callErr)
		}
		if len(toolCalls) == 0 {
			// Final reply
			if _, err := msgStore.AppendMessage(ctx, conversationID, "assistant", content, nil, nil, nil); err != nil {
				return "", fmt.Errorf("append assistant message: %w", err)
			}
			return content, nil
		}
		// Append assistant message with tool calls (persist id, name, arguments)
		toolCallsJSON, err := marshalToolCalls(toolCalls)
		if err != nil {
			return "", fmt.Errorf("marshal tool calls: %w", err)
		}
		if _, err := msgStore.AppendMessage(ctx, conversationID, "assistant", content, nil, nil, toolCallsJSON); err != nil {
			return "", fmt.Errorf("append assistant message: %w", err)
		}
		llmMsgs = append(llmMsgs, llm.Message{Role: "assistant", Content: content, ToolCalls: toolCalls})

		for _, tc := range toolCalls {
			var toolResult string
			if tool, ok := toolsByName[tc.Name]; ok {
				toolResult = executeTool(ctx, tool, tc)
			} else {
				toolResult = fmt.Sprintf("error: unknown tool %q", tc.Name)
			}
			tcID := tc.ID
			if _, err := msgStore.AppendMessage(ctx, conversationID, "tool", toolResult, nil, &tcID, nil); err != nil {
				return "", fmt.Errorf("append tool message: %w", err)
			}
			llmMsgs = append(llmMsgs, llm.Message{Role: "tool", Content: toolResult, ToolCallID: tc.ID})
		}
	}
	return "", fmt.Errorf("conversation: max iterations (%d) exceeded", maxIterations)
}

// RunLoopStream is like RunLoop but streams assistant content deltas via sink. Use streamCaller for LLM calls.
// When the model returns tool calls, those turns are not streamed; only the final (or intermediate) text content is streamed.
func RunLoopStream(
	ctx context.Context,
	convStore entity.ConversationStore,
	msgStore entity.ConversationMessageStore,
	streamCaller StreamLLMCaller,
	conversationID string,
	userContent string,
	channel string,
	tools []Tool,
	sink StreamSink,
) (reply string, err error) {
	if streamCaller == nil {
		return "", fmt.Errorf("conversation stream LLM not configured")
	}
	msgs, err := msgStore.ListMessages(ctx, conversationID)
	if err != nil {
		return "", fmt.Errorf("list messages: %w", err)
	}
	channelPtr := &channel
	if _, err := msgStore.AppendMessage(ctx, conversationID, "user", userContent, channelPtr, nil, nil); err != nil {
		return "", fmt.Errorf("append user message: %w", err)
	}
	llmMsgs := make([]llm.Message, 0, len(msgs)+2)
	for _, m := range msgs {
		toolCallID := ""
		if m.ToolCallID != nil {
			toolCallID = *m.ToolCallID
		}
		msg := llm.Message{Role: m.Role, Content: m.Content, ToolCallID: toolCallID}
		if m.ToolCallsJSON != nil && *m.ToolCallsJSON != "" {
			var toolCalls []llm.ToolCall
			if err := json.Unmarshal([]byte(*m.ToolCallsJSON), &toolCalls); err == nil {
				msg.ToolCalls = toolCalls
			}
		}
		llmMsgs = append(llmMsgs, msg)
	}
	llmMsgs = append(llmMsgs, llm.Message{Role: "user", Content: userContent})

	if tools == nil {
		tools = DefaultTools()
	}
	defs := toolDefs(tools)
	toolsByName := make(map[string]Tool, len(tools))
	for _, t := range tools {
		toolsByName[t.Name()] = t
	}

	onDelta := func(delta string) {
		if sink != nil && delta != "" {
			sink.OnDelta(delta)
		}
	}

	for i := 0; i < maxIterations; i++ {
		slog.Debug("conversation iteration", "iter", i+1, "conversation_id", conversationID)
		messages := append([]llm.Message{{Role: "system", Content: systemPrompt}}, llmMsgs...)
		content, toolCalls, _, callErr := streamCaller.ChatWithToolsStream(ctx, messages, defs, onDelta)
		if callErr != nil {
			return "", fmt.Errorf("llm call: %w", callErr)
		}
		if len(toolCalls) == 0 {
			if _, err := msgStore.AppendMessage(ctx, conversationID, "assistant", content, nil, nil, nil); err != nil {
				return "", fmt.Errorf("append assistant message: %w", err)
			}
			return content, nil
		}
		toolCallsJSON, err := marshalToolCalls(toolCalls)
		if err != nil {
			return "", fmt.Errorf("marshal tool calls: %w", err)
		}
		if _, err := msgStore.AppendMessage(ctx, conversationID, "assistant", content, nil, nil, toolCallsJSON); err != nil {
			return "", fmt.Errorf("append assistant message: %w", err)
		}
		llmMsgs = append(llmMsgs, llm.Message{Role: "assistant", Content: content, ToolCalls: toolCalls})

		for _, tc := range toolCalls {
			var toolResult string
			if tool, ok := toolsByName[tc.Name]; ok {
				toolResult = executeTool(ctx, tool, tc)
			} else {
				toolResult = fmt.Sprintf("error: unknown tool %q", tc.Name)
			}
			tcID := tc.ID
			if _, err := msgStore.AppendMessage(ctx, conversationID, "tool", toolResult, nil, &tcID, nil); err != nil {
				return "", fmt.Errorf("append tool message: %w", err)
			}
			llmMsgs = append(llmMsgs, llm.Message{Role: "tool", Content: toolResult, ToolCallID: tc.ID})
		}
	}
	return "", fmt.Errorf("conversation: max iterations (%d) exceeded", maxIterations)
}

// marshalToolCalls serializes tool calls to JSON for storage. Returns (nil, nil) for empty slice.
func marshalToolCalls(toolCalls []llm.ToolCall) (*string, error) {
	if len(toolCalls) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(toolCalls)
	if err != nil {
		return nil, err
	}
	s := string(b)
	return &s, nil
}
