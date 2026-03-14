// Package conversation contains the low-level Tier 1 LLM loop used by app/conversation.
// It owns message persistence, tool execution, and optional streaming for one turn.
package conversation

import (
	"context"
	"encoding/json"
	"fmt"

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

// conversationBuffer implements agent.MessageBuffer by persisting each Append to the message store.
type conversationBuffer struct {
	ctx             context.Context
	conversationID  string
	msgStore        entity.ConversationMessageStore
	msgs            []llm.Message
}

func (b *conversationBuffer) Messages() []llm.Message {
	return b.msgs
}

func (b *conversationBuffer) Append(m llm.Message) error {
	b.msgs = append(b.msgs, m)
	var toolCallID *string
	if m.Role == "tool" && m.ToolCallID != "" {
		toolCallID = &m.ToolCallID
	}
	var toolCallsJSON *string
	if m.Role == "assistant" && len(m.ToolCalls) > 0 {
		js, err := marshalToolCalls(m.ToolCalls)
		if err != nil {
			return fmt.Errorf("marshal tool calls: %w", err)
		}
		toolCallsJSON = js
	}
	_, err := b.msgStore.AppendMessage(b.ctx, b.conversationID, m.Role, m.Content, nil, toolCallID, toolCallsJSON)
	return err
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

	buf := &conversationBuffer{ctx: ctx, conversationID: conversationID, msgStore: msgStore, msgs: llmMsgs}
	reply, _, err = agent.RunLoop(ctx, agent.RunLoopOpts{
		Caller:        caller,
		SystemPrompt:  systemPrompt,
		ToolDefs:      defs,
		ToolsByName:   toolsByName,
		MaxIter:       maxIterations,
		Buffer:        buf,
		StreamSink:    nil,
	})
	if err != nil {
		return "", err
	}
	if firstRound && userContent != "" && titleGenerator != nil {
		if title, genErr := titleGenerator.GenerateTitleFromInput(ctx, userContent); genErr == nil && title != "" {
			_ = convStore.UpdateConversationTitle(ctx, conversationID, title)
		}
	}
	return reply, nil
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

	buf := &conversationBuffer{ctx: ctx, conversationID: conversationID, msgStore: msgStore, msgs: llmMsgs}
	reply, _, err = agent.RunLoop(ctx, agent.RunLoopOpts{
		Caller:        caller,
		SystemPrompt:  systemPrompt,
		ToolDefs:      defs,
		ToolsByName:   toolsByName,
		MaxIter:       maxIterations,
		Buffer:        buf,
		StreamSink:    sink,
	})
	if err != nil {
		return "", err
	}
	if firstRound && userContent != "" && titleGenerator != nil {
		if title, genErr := titleGenerator.GenerateTitleFromInput(ctx, userContent); genErr == nil && title != "" {
			_ = convStore.UpdateConversationTitle(ctx, conversationID, title)
		}
	}
	return reply, nil
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
