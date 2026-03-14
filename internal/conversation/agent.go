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

// ConversationToolRunners holds optional runners for Tier 1 chat tools. Nil means do not add that tool.
type ConversationToolRunners struct {
	StartChat    tools.StartChatRunner
	ListChats    tools.ListChatsRunner
	GetChat      tools.GetChatRunner
	ContinueChat tools.ContinueChatRunner
}

const maxIterations = 10
const systemPrompt = `You are a coordinator between the user and background chat tasks. You can call GetCurrentDate to get today's date. Reply concisely.

# Decision order
First evaluate whether the user's request should continue an existing chat (use ContinueChat) rather than creating a new one (StartChat). When the user refers to an existing chat (e.g. "add to that chat", "try again for c_xyz", "what about the last run?"), prefer ContinueChat. Use the injected "Recent chats" context or ListChats/GetChat to decide.

# Tools
- GetCurrentDate: today's date when needed.
- StartChat: create and schedule a new background chat task (long-running job, analysis). Always tell the user the chat_id and run_id and where to check progress (Activity or chat detail). Do not claim the work is done immediately—the task runs in the background.
- ListChats: list recent chats in the workspace (up to 10). Use when the user asks what chats they have or for recent activity.
- GetChat: get detail for one chat by chat_id. Use when the user asks about a specific chat's status or result.
- ContinueChat: add a follow-up message to an existing chat (new run). Use when the user wants to continue, retry, or add to an existing chat.

When starting or continuing a task, always tell the user the chat id (and run id) so they can check progress or results later.`

// effectiveSystemPrompt returns the base prompt; if recentChatsSnippet is non-empty, appends it.
func effectiveSystemPrompt(basePrompt, recentChatsSnippet string) string {
	if recentChatsSnippet == "" {
		return basePrompt
	}
	return basePrompt + "\n\n" + recentChatsSnippet
}

// DefaultConversationTools returns the default tool set for the conversation loop (GetCurrentDate only).
func DefaultConversationTools() []core.Tool {
	return []core.Tool{tools.GetCurrentDate{}}
}

// buildConversationTools returns default tools plus any tools whose runner is set in runners.
func buildConversationTools(workspaceID, userID string, runners *ConversationToolRunners) []core.Tool {
	toolList := DefaultConversationTools()
	if runners == nil {
		return toolList
	}
	if runners.StartChat != nil {
		toolList = append(toolList, tools.NewStartChatTool(workspaceID, userID, runners.StartChat))
	}
	if runners.ListChats != nil {
		toolList = append(toolList, tools.NewListChatsTool(workspaceID, runners.ListChats))
	}
	if runners.GetChat != nil {
		toolList = append(toolList, tools.NewGetChatTool(workspaceID, runners.GetChat))
	}
	if runners.ContinueChat != nil {
		toolList = append(toolList, tools.NewContinueChatTool(workspaceID, userID, runners.ContinueChat))
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
// If toolsList is nil, the list is built from buildConversationTools(workspaceID, userID, runners).
// recentChatsSnippet, when non-empty, is appended to the system prompt (e.g. latest 5 chats).
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
	runners *ConversationToolRunners,
	titleGenerator ConversationTitleGenerator,
	recentChatsSnippet string,
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
		toolsList = buildConversationTools(workspaceID, userID, runners)
	}
	defs := agent.ToolDefs(toolsList)
	toolsByName := make(map[string]core.Tool, len(toolsList))
	for _, t := range toolsList {
		toolsByName[t.Name()] = t
	}

	effectivePrompt := effectiveSystemPrompt(systemPrompt, recentChatsSnippet)
	buf := &conversationBuffer{ctx: ctx, conversationID: conversationID, msgStore: msgStore, msgs: llmMsgs}
	reply, _, err = agent.RunLoop(ctx, agent.RunLoopOpts{
		Caller:        caller,
		SystemPrompt:  effectivePrompt,
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
// If toolsList is nil, the list is built from buildConversationTools(workspaceID, userID, runners).
// recentChatsSnippet, when non-empty, is appended to the system prompt.
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
	runners *ConversationToolRunners,
	titleGenerator ConversationTitleGenerator,
	sink llm.StreamSink,
	recentChatsSnippet string,
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
		toolsList = buildConversationTools(workspaceID, userID, runners)
	}
	defs := agent.ToolDefs(toolsList)
	toolsByName := make(map[string]core.Tool, len(toolsList))
	for _, t := range toolsList {
		toolsByName[t.Name()] = t
	}

	effectivePrompt := effectiveSystemPrompt(systemPrompt, recentChatsSnippet)
	buf := &conversationBuffer{ctx: ctx, conversationID: conversationID, msgStore: msgStore, msgs: llmMsgs}
	reply, _, err = agent.RunLoop(ctx, agent.RunLoopOpts{
		Caller:        caller,
		SystemPrompt:  effectivePrompt,
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
