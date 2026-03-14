// Package conversation contains the low-level Tier 1 LLM loop used by app/conversation.
// It owns message persistence, tool execution, and optional streaming for one turn.
package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"buildmax/internal/agent"
	"buildmax/internal/core"
	"buildmax/internal/llm"
	"buildmax/internal/storage/entity"
	"buildmax/internal/tools"
)

// ConversationTitleGenerator generates a short title from the first user message (e.g. via LLM). Optional for RunLoop/RunLoopStream.
type ConversationTitleGenerator interface {
	GenerateTitleFromInput(ctx context.Context, input string) (string, error)
}

const maxIterations = 10
const systemPrompt = `You are a helpful assistant. You can call GetCurrentDate to get today's date. Reply concisely.

You can manage background chat tasks: use the StartChat tool to create and schedule a long-running task (e.g. analysis, a multi-step job). When you start a background task, you must tell the user clearly that a background task was started, and give them the chat id (and optionally run id) so they can check progress or results later (e.g. in Activity or chat detail). Do not claim the work is done immediately—the task runs in the background.`

// DefaultConversationTools returns the default tool set for the conversation loop (GetCurrentDate only).
// Callers (e.g. server) may append more tools (e.g. StartChat) when building the list for RunLoop/RunLoopStream.
func DefaultConversationTools() []core.Tool {
	return []core.Tool{tools.GetCurrentDate{}}
}

// buildConversationTools returns default tools and, when startChatRunner is non-nil, the StartChat tool.
func buildConversationTools(workspaceID, userID string, startChatRunner tools.StartChatRunner) []core.Tool {
	toolList := DefaultConversationTools()
	if startChatRunner != nil {
		toolList = append(toolList, tools.NewStartChatTool(workspaceID, userID, startChatRunner))
	}
	return toolList
}

// RunLoop loads conversation messages, appends the new user message, runs the LLM loop with the given
// tools, and persists every assistant and tool message to the store. Returns the final assistant text reply.
// If toolsList is nil, the list is built from DefaultConversationTools() and, when startChatRunner is non-nil, the StartChat tool.
// If titleGenerator is non-nil and this is the first round (no messages before), a title is generated from userContent and saved.
func RunLoop(
	ctx context.Context,
	convStore entity.ConversationStore,
	msgStore entity.ConversationMessageStore,
	caller llm.LLMCaller,
	conversationID string,
	userContent string,
	channel string,
	toolsList []core.Tool,
	workspaceID, userID string,
	startChatRunner tools.StartChatRunner,
	titleGenerator ConversationTitleGenerator,
) (reply string, err error) {
	if caller == nil {
		return "", fmt.Errorf("conversation LLM not configured")
	}
	msgs, err := msgStore.ListMessages(ctx, conversationID)
	if err != nil {
		return "", fmt.Errorf("list messages: %w", err)
	}
	firstRound := len(msgs) == 0
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

	if toolsList == nil {
		toolsList = buildConversationTools(workspaceID, userID, startChatRunner)
	}
	defs := agent.ToolDefs(toolsList)
	toolsByName := make(map[string]core.Tool, len(toolsList))
	for _, t := range toolsList {
		toolsByName[t.Name()] = t
	}

	for i := 0; i < maxIterations; i++ {
		slog.Debug("conversation iteration", "iter", i+1, "conversation_id", conversationID)
		messages := append([]llm.Message{{Role: "system", Content: systemPrompt}}, llmMsgs...)
		content, toolCalls, _, callErr := caller.ChatWithTools(ctx, messages, defs)
		if callErr != nil {
			return "", fmt.Errorf("llm call: %w", callErr)
		}
		if len(toolCalls) == 0 {
			// Final reply
			if _, err := msgStore.AppendMessage(ctx, conversationID, "assistant", content, nil, nil, nil); err != nil {
				return "", fmt.Errorf("append assistant message: %w", err)
			}
			if firstRound && userContent != "" && titleGenerator != nil {
				if title, genErr := titleGenerator.GenerateTitleFromInput(ctx, userContent); genErr == nil && title != "" {
					_ = convStore.UpdateConversationTitle(ctx, conversationID, title)
				}
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
				toolResult = agent.ExecuteTool(ctx, tool, tc)
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

// RunLoopStream is like RunLoop but streams assistant content deltas via sink.
// When the model returns tool calls, those turns are not streamed; only the final (or intermediate) text content is streamed.
// If toolsList is nil, the list is built from DefaultConversationTools() and, when startChatRunner is non-nil, the StartChat tool.
// If titleGenerator is non-nil and this is the first round, a title is generated from userContent and saved.
func RunLoopStream(
	ctx context.Context,
	convStore entity.ConversationStore,
	msgStore entity.ConversationMessageStore,
	caller llm.LLMCaller,
	conversationID string,
	userContent string,
	channel string,
	toolsList []core.Tool,
	workspaceID, userID string,
	startChatRunner tools.StartChatRunner,
	titleGenerator ConversationTitleGenerator,
	sink llm.StreamSink,
) (reply string, err error) {
	if caller == nil {
		return "", fmt.Errorf("conversation stream LLM not configured")
	}
	msgs, err := msgStore.ListMessages(ctx, conversationID)
	if err != nil {
		return "", fmt.Errorf("list messages: %w", err)
	}
	firstRound := len(msgs) == 0
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

	if toolsList == nil {
		toolsList = buildConversationTools(workspaceID, userID, startChatRunner)
	}
	defs := agent.ToolDefs(toolsList)
	toolsByName := make(map[string]core.Tool, len(toolsList))
	for _, t := range toolsList {
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
		content, toolCalls, _, callErr := caller.ChatWithToolsStream(ctx, messages, defs, onDelta)
		if callErr != nil {
			return "", fmt.Errorf("llm call: %w", callErr)
		}
		if len(toolCalls) == 0 {
			if _, err := msgStore.AppendMessage(ctx, conversationID, "assistant", content, nil, nil, nil); err != nil {
				return "", fmt.Errorf("append assistant message: %w", err)
			}
			if firstRound && userContent != "" && titleGenerator != nil {
				if title, genErr := titleGenerator.GenerateTitleFromInput(ctx, userContent); genErr == nil && title != "" {
					_ = convStore.UpdateConversationTitle(ctx, conversationID, title)
				}
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
				toolResult = agent.ExecuteTool(ctx, tool, tc)
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
