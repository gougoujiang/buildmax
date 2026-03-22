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

// ConversationToolRunners holds optional runners for Tier 1 task tools. Nil means do not add that tool.
type ConversationToolRunners struct {
	StartTask      tools.StartTaskRunner
	ListTasks      tools.ListTasksRunner
	GetTask        tools.GetTaskRunner
	ContinueTask   tools.ContinueTaskRunner
	AgentSummaries []tools.AgentSummary
}

// RunInput configures one conversation turn execution.
type RunInput struct {
	ConversationID     string
	UserContent        string
	Channel            string
	ToolsList          []core.Tool
	ScopeID            string
	UserID             string
	Runners            *ConversationToolRunners
	TitleGenerator     ConversationTitleGenerator
	RecentChatsSnippet string
	StreamSink         llm.StreamSink
}

const maxIterations = 10
const systemPrompt = `You are the user's assistant. You coordinate between the user and background tasks. You can call GetCurrentDate to get today's date. Reply concisely.

# Decision order
First evaluate whether the user's request should continue an existing task (use ContinueTask) rather than creating a new one (StartTask). When the user refers to an existing task (e.g. "add to that task", "try again", "what about the last run?"), prefer ContinueTask. Use the injected "Recent tasks" context or ListTasks/GetTask to decide.

# Tools
- GetCurrentDate: today's date when needed.
- StartTask: create and schedule a new background task (long-running job, analysis). Tell the user you have started a task and will report back when it completes. Do not provide internal task or run IDs. Do not tell the user to check a task detail page — you will deliver the result directly.
- ListTasks: list recent tasks in the current conversation (up to 10). Use when the user asks what tasks they have or for recent activity.
- GetTask: get detail for one task by task_id. Use when the user asks about a specific task's status or result.
- ContinueTask: add a follow-up message to an existing task (new run). Use when the user wants to continue, retry, or add to an existing task.

When starting or continuing a task, tell the user you are working on it and will get back to them with results. Do not expose internal IDs.

# Task results
When you receive a message starting with "[Task Result]", a background task has completed. Read the status and output, then:
- If succeeded: summarize the result clearly and concisely for the user. Present key findings naturally.
- If failed: explain what went wrong and suggest next steps (e.g. retry, provide more info).
Do not mention task IDs, run IDs, or internal system details. Speak to the user as their assistant.`

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
func buildConversationTools(scopeID, userID string, runners *ConversationToolRunners) []core.Tool {
	toolList := DefaultConversationTools()
	if runners == nil {
		return toolList
	}
	if runners.StartTask != nil {
		toolList = append(toolList, tools.NewStartTaskTool(scopeID, userID, runners.StartTask, runners.AgentSummaries))
	}
	if runners.ListTasks != nil {
		toolList = append(toolList, tools.NewListTasksTool(scopeID, runners.ListTasks))
	}
	if runners.GetTask != nil {
		toolList = append(toolList, tools.NewGetTaskTool(scopeID, runners.GetTask))
	}
	if runners.ContinueTask != nil {
		toolList = append(toolList, tools.NewContinueTaskTool(scopeID, userID, runners.ContinueTask))
	}
	return toolList
}

// conversationBuffer implements agent.MessageBuffer by persisting each Append to the message store.
type conversationBuffer struct {
	ctx            context.Context
	conversationID string
	msgStore       entity.ConversationMessageStore
	msgs           []llm.Message
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

type preparedRun struct {
	firstRound bool
	buffer     *conversationBuffer
	toolsList  []core.Tool
}

func prepareRun(ctx context.Context, msgStore entity.ConversationMessageStore, in RunInput) (*preparedRun, error) {
	msgs, err := msgStore.ListMessages(ctx, in.ConversationID)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	firstRound := len(msgs) == 0
	channelPtr := &in.Channel
	if _, err := msgStore.AppendMessage(ctx, in.ConversationID, "user", in.UserContent, channelPtr, nil, nil); err != nil {
		return nil, fmt.Errorf("append user message: %w", err)
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
	llmMsgs = append(llmMsgs, llm.Message{Role: "user", Content: in.UserContent})

	toolsList := in.ToolsList
	if toolsList == nil {
		toolsList = buildConversationTools(in.ScopeID, in.UserID, in.Runners)
	}

	return &preparedRun{
		firstRound: firstRound,
		buffer: &conversationBuffer{
			ctx:            ctx,
			conversationID: in.ConversationID,
			msgStore:       msgStore,
			msgs:           llmMsgs,
		},
		toolsList: toolsList,
	}, nil
}

func executeRun(ctx context.Context, caller llm.LLMCaller, in RunInput, prepared *preparedRun) (string, error) {
	defs := agent.ToolDefs(prepared.toolsList)
	toolsByName := make(map[string]core.Tool, len(prepared.toolsList))
	for _, t := range prepared.toolsList {
		toolsByName[t.Name()] = t
	}

	reply, _, err := agent.RunLoop(ctx, agent.RunLoopOpts{
		Caller:       caller,
		SystemPrompt: effectiveSystemPrompt(systemPrompt, in.RecentChatsSnippet),
		ToolDefs:     defs,
		ToolsByName:  toolsByName,
		MaxIter:      maxIterations,
		Buffer:       prepared.buffer,
		StreamSink:   in.StreamSink,
	})
	if err != nil {
		return "", err
	}
	return reply, nil
}

func maybeUpdateTitle(ctx context.Context, convStore entity.ConversationStore, in RunInput, prepared *preparedRun) {
	if !prepared.firstRound || in.UserContent == "" || in.TitleGenerator == nil {
		return
	}
	if title, err := in.TitleGenerator.GenerateTitleFromInput(ctx, in.UserContent); err == nil && title != "" {
		_ = convStore.UpdateConversationTitle(ctx, in.ConversationID, title)
	}
}

func runLoop(ctx context.Context, convStore entity.ConversationStore, msgStore entity.ConversationMessageStore, caller llm.LLMCaller, in RunInput) (string, error) {
	prepared, err := prepareRun(ctx, msgStore, in)
	if err != nil {
		return "", err
	}
	reply, err := executeRun(ctx, caller, in, prepared)
	if err != nil {
		return "", err
	}
	maybeUpdateTitle(ctx, convStore, in, prepared)
	return reply, nil
}

// Run executes one conversation turn. Streaming is enabled when in.StreamSink is non-nil.
func Run(ctx context.Context, convStore entity.ConversationStore, msgStore entity.ConversationMessageStore, caller llm.LLMCaller, in RunInput) (string, error) {
	if caller == nil {
		if in.StreamSink != nil {
			return "", fmt.Errorf("conversation stream LLM not configured")
		}
		return "", fmt.Errorf("conversation LLM not configured")
	}
	return runLoop(ctx, convStore, msgStore, caller, in)
}

// RunLoop loads conversation messages, appends the new user message, runs the LLM loop with the given
// tools, and persists every assistant and tool message to the store. Returns the final assistant text reply.
// If toolsList is nil, the list is built from buildConversationTools(scopeID, userID, runners).
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
	scopeID, userID string,
	runners *ConversationToolRunners,
	titleGenerator ConversationTitleGenerator,
	recentChatsSnippet string,
) (reply string, err error) {
	return Run(ctx, convStore, msgStore, caller, RunInput{
		ConversationID:     conversationID,
		UserContent:        userContent,
		Channel:            channel,
		ToolsList:          toolsList,
		ScopeID:            scopeID,
		UserID:             userID,
		Runners:            runners,
		TitleGenerator:     titleGenerator,
		RecentChatsSnippet: recentChatsSnippet,
	})
}

// RunLoopStream is like RunLoop but streams assistant content deltas via sink.
// When the model returns tool calls, those turns are not streamed; only the final (or intermediate) text content is streamed.
// If toolsList is nil, the list is built from buildConversationTools(scopeID, userID, runners).
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
	scopeID, userID string,
	runners *ConversationToolRunners,
	titleGenerator ConversationTitleGenerator,
	sink llm.StreamSink,
	recentChatsSnippet string,
) (reply string, err error) {
	return Run(ctx, convStore, msgStore, caller, RunInput{
		ConversationID:     conversationID,
		UserContent:        userContent,
		Channel:            channel,
		ToolsList:          toolsList,
		ScopeID:            scopeID,
		UserID:             userID,
		Runners:            runners,
		TitleGenerator:     titleGenerator,
		RecentChatsSnippet: recentChatsSnippet,
		StreamSink:         sink,
	})
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
